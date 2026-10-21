package transportfulfillment_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// membershipDouble 按对象引用答节点作业的成员关系：units 里登了的是单元、答它登的成员，没登的答不是单元；errs 里登了的那一问报错。
// 每一问的对象与时点都记下，用例据此核「按事实发生那一刻问」与「每件只问一次」。
type membershipDouble struct {
	units map[string][]veports.ConsolidationMember
	errs  map[string]error
	asked []veports.ConsolidationMembershipQuery
}

func (double *membershipDouble) MembersAt(
	_ context.Context, query veports.ConsolidationMembershipQuery,
) (veports.ConsolidationMembership, error) {
	double.asked = append(double.asked, query)
	if err := double.errs[query.Object]; err != nil {
		return veports.ConsolidationMembership{}, err
	}
	members, unit := double.units[query.Object]
	if !unit {
		return veports.ConsolidationMembership{}, nil
	}
	return veports.ConsolidationMembership{Unit: true, Members: append([]veports.ConsolidationMember{}, members...)}, nil
}

func (double *membershipDouble) askedObjects() []string {
	var objects []string
	for _, query := range double.asked {
		objects = append(objects, query.Object)
	}
	return objects
}

func parcelMember(handling, parcel string) veports.ConsolidationMember {
	return veports.ConsolidationMember{Kind: veports.ConsolidationMemberParcel, Handling: handling, Object: parcel}
}

func unitMember(unit string) veports.ConsolidationMember {
	return veports.ConsolidationMember{Kind: veports.ConsolidationMemberUnit, Handling: unit, Object: unit}
}

func unresolvedMember(handling string) veports.ConsolidationMember {
	return veports.ConsolidationMember{Kind: veports.ConsolidationMemberIdentityUnresolved, Handling: handling}
}

// bagHoldsHeldAndClean 让段里的 parcel-1 在节点作业那边是一袋，装着 PCL-HELD 与 PCL-CLEAN——与 bagDepartureRecord 的读数同一现场。
func bagHoldsHeldAndClean(members *membershipDouble) {
	members.units = map[string][]veports.ConsolidationMember{
		"parcel-1": {parcelMember("HU-HELD", "PCL-HELD"), parcelMember("HU-CLEAN", "PCL-CLEAN")},
	}
}

// bagDepartureRecord 是 move-1 / v1 的另一种读数：段里在控的 parcel-1 其实是一袋，袋里装着被扣的 PCL-HELD 与干净的 PCL-CLEAN；
// 同段的 parcel-2 不受门禁。
func bagDepartureRecord(t *testing.T) tfports.MovementFactRecord {
	t.Helper()
	record := movementRecord(t, tfdomain.DepartureFact)
	asked := func(object string, answer tfdomain.GateAnswer) tfdomain.GateFinding {
		built, err := tfdomain.NewGateFinding(movementValue(t, tfdomain.NewCarriedObjectReference, object), answer,
			"AADJ-"+object, []tfdomain.GateBasis{{Action: "LOADING_DEPARTURE", Cause: "SOME_CAUSE"}})
		if err != nil {
			t.Fatalf("逐对象那一格：%v", err)
		}
		return built
	}
	held, clean := asked("PCL-HELD", tfdomain.GateBlocked), asked("PCL-CLEAN", tfdomain.GateNotGuarded)
	bag, err := tfdomain.FormConsolidationUnitGateFinding(movementValue(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
		[]tfdomain.GateFinding{held, clean}, nil)
	if err != nil {
		t.Fatalf("袋那一格：%v", err)
	}
	reading, err := tfdomain.FormDepartureGateReading(tfdomain.DepartureGateReadingSpec{
		From:     movementValue(t, tfdomain.NewNetworkNodeReference, "NODE-SHA"),
		To:       movementValue(t, tfdomain.NewNetworkNodeReference, "NODE-SZX"),
		Findings: []tfdomain.GateFinding{bag, held, clean, asked("parcel-2", tfdomain.GateNotGuarded)},
	})
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	spec := tfdomain.MovementFactSpec{
		TenantID: record.Fact.TenantID(), Fact: record.Fact.Fact(), Schedule: record.Fact.Schedule(), Kind: tfdomain.DepartureFact,
		Location: record.Fact.Location(), Source: record.Fact.Source(), Version: record.Fact.Version(),
		OccurredAt: record.Fact.OccurredAt(), GateReading: reading,
	}
	spec.Segment, _ = record.Fact.Segment()
	fact, err := tfdomain.RecordMovementFact(spec)
	if err != nil {
		t.Fatalf("事实：%v", err)
	}
	record.Fact = fact
	return record
}

// Covers: AT-VE-183（ADR-0225 决定七）——出发读数里袋那一格阻断时，信号落在袋里被关务判为阻断的那一件上，袋那一格不另形成信号：
// 它是由成员推出来的，再立一次就是同一次违规在分诊里记两回。
func TestABlockedBagSignalsTheBlockedMemberNotTheBag(t *testing.T) {
	fixture := newMovementFixture(t, bagDepartureRecord(t))
	bagHoldsHeldAndClean(fixture.members)
	if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(fixture.signals.parcels) != 1 || fixture.signals.parcels[0] != "PCL-HELD" {
		t.Fatalf("交进信号编排的 = %v，要只有袋里被扣的 PCL-HELD", fixture.signals.parcels)
	}
}

// Covers: AT-VE-196——段里那一刻在控的一件是一袋时，移动事实投到袋里那一刻的每件成员身上，不投到袋上：成员各得一份与正常路径
// 同形的事实（类型词、带成员维的引用、版本、三个时间），成员关系按事实的业务发生时间问；同段的散件照投。出发、移动、到达三种同一
// 口径——移动与到达不带读数，展开不靠读数。同一封信重投只答已有结果，事实与条目都不多出第二份。
func TestABagUnderControlProjectsOntoItsMembersAtTheMomentNotOntoTheBag(t *testing.T) {
	for kind, want := range map[tfdomain.MovementFactKind]string{
		tfdomain.DepartureFact: "movement-departure",
		tfdomain.InTransitFact: "movement-in-transit",
		tfdomain.ArrivalFact:   "movement-arrival",
	} {
		t.Run(want, func(t *testing.T) {
			record := movementRecord(t, kind)
			if kind == tfdomain.DepartureFact {
				record = bagDepartureRecord(t)
			}
			fixture := newMovementFixture(t, record)
			bagHoldsHeldAndClean(fixture.members)

			for range 2 {
				if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
					t.Fatalf("handle: %v", err)
				}
			}
			if bag := fixture.factsOf("parcel-1"); len(bag) != 0 {
				t.Fatalf("袋本身被当成一件包裹投了 %d 份", len(bag))
			}
			if _, projected := fixture.projections.byKey["tenant-1/parcel-1"]; projected {
				t.Fatal("袋本身立了一份追踪投影")
			}
			for _, parcel := range []string{"PCL-HELD", "PCL-CLEAN", "parcel-2"} {
				records := fixture.factsOf(parcel)
				if len(records) != 1 {
					t.Fatalf("%s 的事实 %d 份, want 1（重投不多出第二份）", parcel, len(records))
				}
				fact := records[0].Fact
				if fact.Kind().String() != want || fact.Fact().String() != "movement-fact/"+parcel+"/move-1" ||
					fact.Version().String() != "v1" {
					t.Fatalf("%s：kind=%s fact=%s version=%s", parcel, fact.Kind(), fact.Fact(), fact.Version())
				}
				if !fact.OccurredAt().Equal(movementDepartedAt) || !fact.EffectiveAt().Equal(movementDepartedAt) ||
					!fact.ReceivedAt().Equal(movementRecordedAt) {
					t.Fatalf("%s 三个时间 = %s / %s / %s", parcel, fact.OccurredAt(), fact.EffectiveAt(), fact.ReceivedAt())
				}
				if entries := len(fixture.projections.byKey["tenant-1/"+parcel].Entries()); entries != 1 {
					t.Fatalf("%s 条目 %d 个, want 1", parcel, entries)
				}
			}
			for _, query := range fixture.members.asked {
				if !query.At.Equal(movementDepartedAt) || query.TenantID.String() != "tenant-1" {
					t.Fatalf("成员关系问在 %s / %s，要按事实发生那一刻、本租户问", query.At, query.TenantID)
				}
			}
		})
	}
}

// Covers: AT-VE-197——逐层展开到包裹，每件只问一次、只投一次：笼里装着袋、一件散件、一件身份不成立的实物与一件同时直接在段里的
// 包裹，袋里装着一件包裹并（成员关系册成了环）回指笼；身份不成立的那一件没有可投的追踪包裹，入账不派生，不挡其余各件。空单元那一刻
// 什么也不装，没有可投的成员。
func TestNestedUnitsExpandToParcelsOnceAndMembersWithoutIdentityAreSkipped(t *testing.T) {
	t.Run("嵌套、重复与成环", func(t *testing.T) {
		fixture := newMovementFixture(t, movementRecord(t, tfdomain.ArrivalFact))
		fixture.members.units = map[string][]veports.ConsolidationMember{
			"parcel-1": {unitMember("BAG-9"), parcelMember("HU-LOOSE", "PCL-LOOSE"), unresolvedMember("HU-UNKNOWN"),
				parcelMember("HU-2", "parcel-2")},
			"BAG-9": {parcelMember("HU-DEEP", "PCL-DEEP"), unitMember("parcel-1")},
		}
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		for _, parcel := range []string{"PCL-LOOSE", "PCL-DEEP", "parcel-2"} {
			if n := len(fixture.factsOf(parcel)); n != 1 {
				t.Fatalf("%s 的事实 %d 份, want 1", parcel, n)
			}
		}
		for _, notParcel := range []string{"parcel-1", "BAG-9", "HU-UNKNOWN", "HU-LOOSE"} {
			if n := len(fixture.factsOf(notParcel)); n != 0 {
				t.Fatalf("%s 不是可投的追踪包裹，却投了 %d 份", notParcel, n)
			}
		}
		if len(fixture.facts.byKey) != 3 {
			t.Fatalf("事实共 %d 份, want 3", len(fixture.facts.byKey))
		}
		asked := fixture.members.askedObjects()
		seen := map[string]bool{}
		for _, object := range asked {
			if seen[object] {
				t.Fatalf("%s 被问了不止一次：%v", object, asked)
			}
			seen[object] = true
		}
		// 成员认出的包裹也先问一次是不是单元：它与某件单元同串时按单元认（与出发门禁读数同一判据）。
		for _, object := range []string{"parcel-1", "BAG-9", "PCL-LOOSE", "PCL-DEEP", "parcel-2"} {
			if !slices.Contains(asked, object) {
				t.Fatalf("%s 没问过是不是单元：%v", object, asked)
			}
		}
	})
	t.Run("空单元", func(t *testing.T) {
		fixture := newMovementFixture(t, movementRecord(t, tfdomain.ArrivalFact))
		fixture.members.units = map[string][]veports.ConsolidationMember{"parcel-1": {}}
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.factsOf("parcel-1")) != 0 || len(fixture.factsOf("parcel-2")) != 1 || len(fixture.facts.byKey) != 1 {
			t.Fatalf("空单元那一刻没有可投的成员：事实共 %d 份", len(fixture.facts.byKey))
		}
	})
}

// Covers: AT-VE-198——成员关系或成员身份那一问读不动是续办：整封未决、一份也不派生，重投再来；节点作业答了却立不成成员（答复译不
// 回、身份链判不下来、成员种类落在封闭集外）重问还是同一个结果，记成员关系答复不一致，不当未决。
func TestConsolidationMembershipFailuresKeepTheirSentinels(t *testing.T) {
	for name, test := range map[string]struct {
		configure func(*membershipDouble)
		want      error
	}{
		"段里那一件问不动": {
			configure: func(members *membershipDouble) { members.errs = map[string]error{"parcel-2": errors.New("down")} },
			want:      adapter.ErrConsolidationMembershipNotVisible,
		},
		"嵌套那一层问不动": {
			configure: func(members *membershipDouble) {
				bagHoldsHeldAndClean(members)
				members.errs = map[string]error{"PCL-CLEAN": errors.New("intake down")}
			},
			want: adapter.ErrConsolidationMembershipNotVisible,
		},
		"答复译不回": {
			configure: func(members *membershipDouble) {
				members.errs = map[string]error{"parcel-1": veports.ErrConsolidationMembershipUntranslatable}
			},
			want: adapter.ErrConsolidationMembershipInconsistent,
		},
		"成员种类在封闭集外": {
			configure: func(members *membershipDouble) {
				members.units = map[string][]veports.ConsolidationMember{"parcel-1": {{Handling: "HU-?"}}}
			},
			want: adapter.ErrConsolidationMembershipInconsistent,
		},
		"成员引用立不成追踪包裹": {
			configure: func(members *membershipDouble) {
				members.units = map[string][]veports.ConsolidationMember{"parcel-1": {parcelMember("HU-BLANK", " ")}}
			},
			want: adapter.ErrConsolidationMembershipInconsistent,
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newMovementFixture(t, movementRecord(t, tfdomain.ArrivalFact))
			test.configure(fixture.members)
			err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef())
			if !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
			if len(fixture.facts.byKey) != 0 {
				t.Fatalf("成员没展开完就派生了 %d 份", len(fixture.facts.byKey))
			}
		})
	}
}

// Covers: AT-VE-196 补派生半边（ADR-0224 同一判据）——事实先于段登记，一袋后凭`已交接`进段：交接那一封在袋里事实那一刻的成员身上
// 补派生，不补到袋上；成员关系按那条事实的业务发生时间问。与正常路径同一把键：正常路径随后再来，成员身上仍只有一份。
func TestABagHandedIntoTheSegmentBackfillsOntoItsMembers(t *testing.T) {
	fixture := newBackfillFixture(t, handoverMappingViewDouble{})
	bagHoldsHeldAndClean(fixture.members)

	if err := fixture.subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if n := fixture.factsOf("parcel-1"); n != 0 {
		t.Fatalf("袋本身被补了 %d 份", n)
	}
	for _, parcel := range []string{"PCL-HELD", "PCL-CLEAN"} {
		if facts, entries := fixture.factsOf(parcel), fixture.entriesOf(t, parcel); facts != 1 || entries != 1 {
			t.Fatalf("%s 事实 %d 份、条目 %d 个，want 各 1", parcel, facts, entries)
		}
	}
	for _, query := range fixture.members.asked {
		if !query.At.Equal(movementDepartedAt) {
			t.Fatalf("成员关系问在 %s，要按事实发生那一刻问", query.At)
		}
	}

	segment := fixture.segments.records[fixture.segments.keys[0]]
	record := movementRecord(t, tfdomain.DepartureFact)
	normal, err := adapter.NewDeriveOnMovementFactAdapter(
		&movementFinderDouble{records: map[tfports.MovementFactKey]tfports.MovementFactRecord{record.Key: record}},
		&movementSegmentDouble{record: segment, found: true}, fixture.members, fixture.derive,
		&movementSignalDouble{t: t, answer: signalAnswering(movementRuleDouble{configured: false})})
	if err != nil {
		t.Fatalf("构造正常路径：%v", err)
	}
	if err := normal.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
		t.Fatalf("正常路径：%v", err)
	}
	for _, parcel := range []string{"PCL-HELD", "PCL-CLEAN"} {
		if facts, entries := fixture.factsOf(parcel), fixture.entriesOf(t, parcel); facts != 1 || entries != 1 {
			t.Fatalf("正常路径之后 %s 事实 %d 份、条目 %d 个，want 各 1", parcel, facts, entries)
		}
	}
	if n := fixture.factsOf("parcel-1"); n != 0 {
		t.Fatalf("正常路径把袋本身投了 %d 份", n)
	}
}
