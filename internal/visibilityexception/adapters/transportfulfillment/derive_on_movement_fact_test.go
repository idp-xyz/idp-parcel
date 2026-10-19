package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证移动事实投影适配器（票 demo-chain-gaps/33，ADR-0205 决定五与文末补记）：按键重读事实，沿声明的段取那一刻在控的对象
// 逐个派生，种类词按出发 / 移动 / 到达分开；阻断的那几件各交一次信号；各格失败落进各自的哨兵。派生一侧用真的派生编排接替身端口。

var (
	movementDepartedAt = time.Date(2026, 10, 21, 14, 5, 0, 0, time.UTC)
	movementRecordedAt = movementDepartedAt.Add(time.Minute)
)

type movementFinderDouble struct {
	records map[tfports.MovementFactKey]tfports.MovementFactRecord
	err     error
}

func (double *movementFinderDouble) FindByKey(_ context.Context, key tfports.MovementFactKey) (tfports.MovementFactRecord, bool, error) {
	if double.err != nil {
		return tfports.MovementFactRecord{}, false, double.err
	}
	record, found := double.records[key]
	return record, found, nil
}

type movementSegmentDouble struct {
	record tfports.FulfillmentSegmentRecord
	found  bool
	err    error
	calls  int
}

func (double *movementSegmentDouble) FindByKey(
	_ context.Context, _ tfports.FulfillmentSegmentKey,
) (tfports.FulfillmentSegmentRecord, bool, error) {
	double.calls++
	return double.record, double.found, double.err
}

// movementSignalDouble 记下每一件被交进来的对象，按 outcome 作答。
type movementSignalDouble struct {
	parcels []string
	answer  func(t *testing.T) veapplication.RaiseDepartureGateSignalResult
	err     error
	t       *testing.T
}

func (double *movementSignalDouble) Handle(
	_ context.Context, command veapplication.RaiseDepartureGateSignalCommand,
) (veapplication.RaiseDepartureGateSignalResult, error) {
	double.parcels = append(double.parcels, command.Parcel.String())
	if double.err != nil {
		return veapplication.RaiseDepartureGateSignalResult{}, double.err
	}
	return double.answer(double.t), nil
}

type movementRuleDouble struct {
	configured bool
	err        error
}

func (double movementRuleDouble) DepartureGateSignalRule(context.Context) (veports.DepartureGateSignalRule, bool, error) {
	return veports.DepartureGateSignalRule{}, double.configured, double.err
}

// signalAnswering 交回一个只按规则视图作答的真编排：规则未配置答 RuleNotConfigured，规则读不动答 Deferred。用真编排而不手造
// 结果，是因为结果类型只有编排能造。
func signalAnswering(rule movementRuleDouble) func(t *testing.T) veapplication.RaiseDepartureGateSignalResult {
	return func(t *testing.T) veapplication.RaiseDepartureGateSignalResult {
		t.Helper()
		handler, err := veapplication.NewRaiseDepartureGateSignalHandler(veapplication.RaiseDepartureGateSignalDeps{
			Rules: rule, Signals: unusedSignalRaiser{t: t}, Clock: handoverClock{at: movementRecordedAt},
		})
		if err != nil {
			t.Fatalf("构造信号编排：%v", err)
		}
		result, err := handler.Handle(t.Context(), veapplication.RaiseDepartureGateSignalCommand{})
		if err != nil {
			t.Fatalf("信号编排：%v", err)
		}
		return result
	}
}

type unusedSignalRaiser struct{ t *testing.T }

func (raiser unusedSignalRaiser) Handle(context.Context, veapplication.RaiseSignalCommand) (veapplication.RaiseSignalResult, error) {
	raiser.t.Fatal("这条用例不该走到分诊")
	return veapplication.RaiseSignalResult{}, nil
}

func movementValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// movementSegment 是 segment-1：parcel-1、parcel-2 出发前一小时交接入段，parcel-late 出发后才入段。
func movementSegment(t *testing.T) tfports.FulfillmentSegmentRecord {
	t.Helper()
	entered := func(name string, at time.Time) tfdomain.RehydrateParticipationSpec {
		return tfdomain.RehydrateParticipationSpec{
			Object:     movementValue(t, tfdomain.NewCarriedObjectReference, name),
			EntryKind:  tfdomain.EnteredByTransportHandover,
			EntryBasis: movementValue(t, tfdomain.NewParticipationBasisReference, "HO/"+name),
			EnteredAt:  at,
		}
	}
	tenant := movementValue(t, tfdomain.NewTenantID, "tenant-1")
	reference := movementValue(t, tfdomain.NewFulfillmentSegmentReference, "segment-1")
	segment, err := tfdomain.RehydrateActualFulfillmentSegment(tfdomain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenant,
		Segment:  reference,
		Participations: []tfdomain.RehydrateParticipationSpec{
			entered("parcel-1", movementDepartedAt.Add(-time.Hour)),
			entered("parcel-2", movementDepartedAt.Add(-time.Hour)),
			entered("parcel-late", movementDepartedAt.Add(time.Minute)),
		},
	})
	if err != nil {
		t.Fatalf("重建段：%v", err)
	}
	return tfports.FulfillmentSegmentRecord{
		Key:     tfports.FulfillmentSegmentKey{TenantID: tenant, Segment: reference},
		Segment: segment,
	}
}

// movementRecord 是一条 move-1 / v1；出发声明 segment-1，读数问到 parcel-1 阻断、parcel-2 不受门禁。
func movementRecord(t *testing.T, kind tfdomain.MovementFactKind) tfports.MovementFactRecord {
	t.Helper()
	tenant := movementValue(t, tfdomain.NewTenantID, "tenant-1")
	spec := tfdomain.MovementFactSpec{
		TenantID:   tenant,
		Fact:       movementValue(t, tfdomain.NewMovementFactReference, "move-1"),
		Schedule:   movementValue(t, tfdomain.NewScheduleReference, "schedule-1"),
		Kind:       kind,
		Location:   movementValue(t, tfdomain.NewMovementLocationReference, "NODE-SHA"),
		Source:     movementValue(t, tfdomain.NewMovementSourceReference, "own-fleet/v1"),
		Version:    movementValue(t, tfdomain.NewMovementFactVersion, "v1"),
		OccurredAt: movementDepartedAt,
		Segment:    movementValue(t, tfdomain.NewFulfillmentSegmentReference, "segment-1"),
	}
	if kind == tfdomain.DepartureFact {
		finding := func(object string, answer tfdomain.DepartureGateAnswer) tfdomain.DepartureGateFinding {
			built, err := tfdomain.NewDepartureGateFinding(movementValue(t, tfdomain.NewCarriedObjectReference, object), answer,
				"AADJ-"+object, []tfdomain.DepartureGateBasis{{Action: "LOADING_DEPARTURE", Cause: "SOME_CAUSE"}})
			if err != nil {
				t.Fatalf("逐对象那一格：%v", err)
			}
			return built
		}
		reading, err := tfdomain.FormDepartureGateReading(tfdomain.DepartureGateReadingSpec{
			From:     movementValue(t, tfdomain.NewNetworkNodeReference, "NODE-SHA"),
			To:       movementValue(t, tfdomain.NewNetworkNodeReference, "NODE-SZX"),
			Findings: []tfdomain.DepartureGateFinding{finding("parcel-1", tfdomain.DepartureGateBlocked), finding("parcel-2", tfdomain.DepartureGateNotGuarded)},
		})
		if err != nil {
			t.Fatalf("读数：%v", err)
		}
		spec.GateReading = reading
	}
	fact, err := tfdomain.RecordMovementFact(spec)
	if err != nil {
		t.Fatalf("事实：%v", err)
	}
	return tfports.MovementFactRecord{
		Key:        tfports.MovementFactKey{TenantID: tenant, Fact: spec.Fact, Version: spec.Version},
		Fact:       fact,
		RecordedAt: movementRecordedAt,
	}
}

func recordedMovementRef() veinbox.RecordedMovementFact {
	return veinbox.RecordedMovementFact{TenantID: "tenant-1", Fact: "move-1", Version: "v1"}
}

type movementFixture struct {
	subject     *adapter.DeriveOnMovementFactAdapter
	facts       *handoverFactStoreDouble
	projections *handoverProjectionStoreDouble
	segments    *movementSegmentDouble
	signals     *movementSignalDouble
}

func newMovementFixture(t *testing.T, record tfports.MovementFactRecord) *movementFixture {
	t.Helper()
	derive, facts, projections := handoverDeriveHandler(t, handoverMappingViewDouble{}, handoverProjectionDownstreamDouble{})
	segments := &movementSegmentDouble{record: movementSegment(t), found: true}
	signals := &movementSignalDouble{t: t, answer: signalAnswering(movementRuleDouble{configured: false})}
	subject, err := adapter.NewDeriveOnMovementFactAdapter(
		&movementFinderDouble{records: map[tfports.MovementFactKey]tfports.MovementFactRecord{record.Key: record}},
		segments, derive, signals)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return &movementFixture{subject: subject, facts: facts, projections: projections, segments: segments, signals: signals}
}

func (fixture *movementFixture) factsOf(parcel string) []veports.FactRecord {
	var found []veports.FactRecord
	for _, record := range fixture.facts.byKey {
		if record.Fact.Parcel().String() == parcel {
			found = append(found, record)
		}
	}
	return found
}

// Covers: AT-TF-157——出发在段里那一刻在控的每件对象上各派生一份事实：类型词 movement-departure，引用带对象维，三个时间各取其主，
// 出发后才入段的那件不投；读数不进投影。阻断的那一件（且只有它）交进信号编排。
func TestADepartureProjectsOntoEveryObjectUnderControlAndSignalsTheBlockedOne(t *testing.T) {
	fixture := newMovementFixture(t, movementRecord(t, tfdomain.DepartureFact))

	if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		records := fixture.factsOf(parcel)
		if len(records) != 1 {
			t.Fatalf("%s 的事实 %d 份, want 1", parcel, len(records))
		}
		fact := records[0].Fact
		if fact.Kind().String() != "movement-departure" || fact.Fact().String() != "movement-fact/"+parcel+"/move-1" ||
			fact.Version().String() != "v1" || fact.Source() != vedomain.SourceTransportFulfillment {
			t.Fatalf("%s：kind=%s fact=%s version=%s source=%v", parcel, fact.Kind(), fact.Fact(), fact.Version(), fact.Source())
		}
		if !fact.OccurredAt().Equal(movementDepartedAt) || !fact.EffectiveAt().Equal(movementDepartedAt) ||
			!fact.ReceivedAt().Equal(movementRecordedAt) {
			t.Fatalf("%s 三个时间 = %s / %s / %s", parcel, fact.OccurredAt(), fact.EffectiveAt(), fact.ReceivedAt())
		}
	}
	if late := fixture.factsOf("parcel-late"); len(late) != 0 {
		t.Fatalf("出发后才入段的那件也投了：%d 份", len(late))
	}
	if len(fixture.signals.parcels) != 1 || fixture.signals.parcels[0] != "parcel-1" {
		t.Fatalf("交进信号编排的 = %v, want 只有阻断的 parcel-1", fixture.signals.parcels)
	}
}

// Covers: AT-TF-157 的类型词——移动与到达各用自己的词，没有读数也就没有信号。
func TestInTransitAndArrivalFactsCarryTheirOwnKindsAndRaiseNoSignal(t *testing.T) {
	for kind, want := range map[tfdomain.MovementFactKind]string{
		tfdomain.InTransitFact: "movement-in-transit",
		tfdomain.ArrivalFact:   "movement-arrival",
	} {
		t.Run(want, func(t *testing.T) {
			fixture := newMovementFixture(t, movementRecord(t, kind))
			if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
				t.Fatalf("handle: %v", err)
			}
			records := fixture.factsOf("parcel-1")
			if len(records) != 1 || records[0].Fact.Kind().String() != want {
				t.Fatalf("事实 = %+v, want 一份 %s", records, want)
			}
			if len(fixture.signals.parcels) != 0 {
				t.Fatalf("没有读数却交了信号：%v", fixture.signals.parcels)
			}
		})
	}
}

// Covers: AT-TF-159——没有可投的对象就入账：没声明段（不读段册）、段不在册、那一刻段里没有在控对象，都不派生、不报错。
func TestAMovementFactWithNoObjectToProjectIsAccountedForWithoutDeriving(t *testing.T) {
	undeclared := movementRecord(t, tfdomain.ArrivalFact)
	spec := tfdomain.MovementFactSpec{
		TenantID: undeclared.Key.TenantID, Fact: undeclared.Key.Fact, Schedule: undeclared.Fact.Schedule(),
		Kind: tfdomain.ArrivalFact, Location: undeclared.Fact.Location(), Source: undeclared.Fact.Source(),
		Version: undeclared.Key.Version, OccurredAt: movementDepartedAt,
	}
	fact, err := tfdomain.RecordMovementFact(spec)
	if err != nil {
		t.Fatalf("事实：%v", err)
	}
	undeclared.Fact = fact

	t.Run("没声明段", func(t *testing.T) {
		fixture := newMovementFixture(t, undeclared)
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if fixture.segments.calls != 0 || len(fixture.facts.byKey) != 0 {
			t.Fatalf("没声明段却读了段册 %d 次、派生了 %d 份", fixture.segments.calls, len(fixture.facts.byKey))
		}
	})
	t.Run("段不在册", func(t *testing.T) {
		fixture := newMovementFixture(t, movementRecord(t, tfdomain.DepartureFact))
		fixture.segments.found = false
		fixture.signals.answer = signalAnswering(movementRuleDouble{configured: false})
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.facts.byKey) != 0 {
			t.Fatalf("段不在册却派生了 %d 份", len(fixture.facts.byKey))
		}
	})
	t.Run("那一刻没有在控对象", func(t *testing.T) {
		late := movementRecord(t, tfdomain.ArrivalFact)
		lateSpec := spec
		lateSpec.Segment = movementValue(t, tfdomain.NewFulfillmentSegmentReference, "segment-1")
		lateSpec.OccurredAt = movementDepartedAt.Add(-2 * time.Hour)
		if late.Fact, err = tfdomain.RecordMovementFact(lateSpec); err != nil {
			t.Fatalf("事实：%v", err)
		}
		fixture := newMovementFixture(t, late)
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.facts.byKey) != 0 {
			t.Fatalf("段里那一刻没人却派生了 %d 份", len(fixture.facts.byKey))
		}
	})
}

// Covers: AT-TF-160——各格失败落进各自的哨兵：读不回事实与段册读不动是续办（未决），信号搁置是续办；键不符、引用译不出是不变量
// 或引用坏了；分诊意图未交另成一格。
func TestMovementFactFailuresKeepTheirSentinels(t *testing.T) {
	t.Run("读不回事实", func(t *testing.T) {
		fixture := newMovementFixture(t, movementRecord(t, tfdomain.DepartureFact))
		missing := recordedMovementRef()
		missing.Version = "v9"
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), missing); !errors.Is(err, adapter.ErrMovementFactNotVisible) {
			t.Fatalf("err = %v, want ErrMovementFactNotVisible", err)
		}
	})
	t.Run("事实册读不动", func(t *testing.T) {
		derive, _, _ := handoverDeriveHandler(t, handoverMappingViewDouble{}, handoverProjectionDownstreamDouble{})
		subject, err := adapter.NewDeriveOnMovementFactAdapter(&movementFinderDouble{err: errors.New("down")},
			&movementSegmentDouble{}, derive, &movementSignalDouble{t: t})
		if err != nil {
			t.Fatalf("构造：%v", err)
		}
		if err := subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); !errors.Is(err, adapter.ErrMovementFactNotVisible) {
			t.Fatalf("err = %v, want ErrMovementFactNotVisible", err)
		}
	})
	t.Run("段册读不动", func(t *testing.T) {
		fixture := newMovementFixture(t, movementRecord(t, tfdomain.DepartureFact))
		fixture.segments.err = errors.New("down")
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); !errors.Is(err, adapter.ErrMovementSegmentNotVisible) {
			t.Fatalf("err = %v, want ErrMovementSegmentNotVisible", err)
		}
	})
	t.Run("段册交回另一个段", func(t *testing.T) {
		fixture := newMovementFixture(t, movementRecord(t, tfdomain.DepartureFact))
		fixture.segments.record.Key.Segment = movementValue(t, tfdomain.NewFulfillmentSegmentReference, "segment-9")
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); !errors.Is(err, adapter.ErrMovementFactRecordInconsistent) {
			t.Fatalf("err = %v, want ErrMovementFactRecordInconsistent", err)
		}
	})
	t.Run("记录键不符", func(t *testing.T) {
		record := movementRecord(t, tfdomain.DepartureFact)
		record.RecordedAt = time.Time{}
		fixture := newMovementFixture(t, record)
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); !errors.Is(err, adapter.ErrMovementFactRecordInconsistent) {
			t.Fatalf("err = %v, want ErrMovementFactRecordInconsistent", err)
		}
	})
	t.Run("引用译不出", func(t *testing.T) {
		fixture := newMovementFixture(t, movementRecord(t, tfdomain.DepartureFact))
		blank := recordedMovementRef()
		blank.TenantID = ""
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), blank); !errors.Is(err, adapter.ErrMovementFactUntranslatableAnswer) {
			t.Fatalf("err = %v, want ErrMovementFactUntranslatableAnswer", err)
		}
	})
	t.Run("信号搁置", func(t *testing.T) {
		fixture := newMovementFixture(t, movementRecord(t, tfdomain.DepartureFact))
		fixture.signals.answer = signalAnswering(movementRuleDouble{err: errors.New("rule down")})
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); !errors.Is(err, adapter.ErrDepartureGateSignalUndecided) {
			t.Fatalf("err = %v, want ErrDepartureGateSignalUndecided", err)
		}
	})
	t.Run("信号编排报错", func(t *testing.T) {
		fixture := newMovementFixture(t, movementRecord(t, tfdomain.DepartureFact))
		boom := errors.New("boom")
		fixture.signals.err = boom
		if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want 原样上抛", err)
		}
	})
	t.Run("派生未决", func(t *testing.T) {
		derive, _, _ := handoverDeriveHandler(t, handoverMappingViewDouble{err: errors.New("mapping down")}, handoverProjectionDownstreamDouble{})
		subject, err := adapter.NewDeriveOnMovementFactAdapter(
			&movementFinderDouble{records: map[tfports.MovementFactKey]tfports.MovementFactRecord{
				movementRecord(t, tfdomain.DepartureFact).Key: movementRecord(t, tfdomain.DepartureFact)}},
			&movementSegmentDouble{record: movementSegment(t), found: true}, derive, &movementSignalDouble{t: t})
		if err != nil {
			t.Fatalf("构造：%v", err)
		}
		if err := subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); !errors.Is(err, veconsume.ErrProjectionUndecided) {
			t.Fatalf("err = %v, want veconsume.ErrProjectionUndecided", err)
		}
	})
}

// Covers: AT-TF-161——更正版沿用原读数（ADR-0205 越权风险点 8）：投影照派生、带替代关系，信号不再交——同一次出发原版已经命中过。
func TestACorrectedDepartureProjectsWithItsSupersessionButRaisesNoSecondSignal(t *testing.T) {
	original := movementRecord(t, tfdomain.DepartureFact)
	corrected, err := original.Fact.Correct(original.Fact.Location(), movementDepartedAt.Add(time.Minute), original.Fact.Source(),
		movementValue(t, tfdomain.NewMovementFactVersion, "v2"), movementRecordedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	record := tfports.MovementFactRecord{
		Key:        tfports.MovementFactKey{TenantID: original.Key.TenantID, Fact: original.Key.Fact, Version: corrected.Version()},
		Fact:       corrected,
		RecordedAt: movementRecordedAt.Add(time.Hour),
	}
	fixture := newMovementFixture(t, record)
	reference := recordedMovementRef()
	reference.Version = "v2"
	if err := fixture.subject.HandleRecordedMovementFact(t.Context(), reference); err != nil {
		t.Fatalf("handle: %v", err)
	}
	records := fixture.factsOf("parcel-1")
	if len(records) != 1 {
		t.Fatalf("事实 %d 份, want 1", len(records))
	}
	if supersedes, has := records[0].Fact.Supersedes(); !has || supersedes.String() != "v1" {
		t.Fatalf("替代关系 = %q（%v），want 回指 v1", supersedes, has)
	}
	if len(fixture.signals.parcels) != 0 {
		t.Fatalf("更正版又交了信号：%v", fixture.signals.parcels)
	}
}

func TestTheMovementFactAdapterRefusesMissingDependencies(t *testing.T) {
	derive, _, _ := handoverDeriveHandler(t, handoverMappingViewDouble{}, handoverProjectionDownstreamDouble{})
	finder, segments, signals := &movementFinderDouble{}, &movementSegmentDouble{}, &movementSignalDouble{t: t}
	for name, build := range map[string]func() error{
		"缺事实读口": func() error {
			_, err := adapter.NewDeriveOnMovementFactAdapter(nil, segments, derive, signals)
			return err
		},
		"缺段读口": func() error {
			_, err := adapter.NewDeriveOnMovementFactAdapter(finder, nil, derive, signals)
			return err
		},
		"缺派生": func() error {
			_, err := adapter.NewDeriveOnMovementFactAdapter(finder, segments, nil, signals)
			return err
		},
		"缺信号": func() error {
			_, err := adapter.NewDeriveOnMovementFactAdapter(finder, segments, derive, nil)
			return err
		},
	} {
		if build() == nil {
			t.Fatalf("%s 构造成功了", name)
		}
	}
}
