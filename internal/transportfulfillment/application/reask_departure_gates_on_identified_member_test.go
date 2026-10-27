package application_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证一件作业实物在袋出发之后才识别成功时，运输履约按原出发时点只替这一件包裹补问关务（ADR-0331）：向节点作业问它曾被装进哪几件
// 单元，在那些单元有参与的段上按段列出的原版出发里，出发那一刻单元在控、且这件包裹正随单元走的，追加一版以这件包裹为键的补问读数。
// 节点作业与成员关系由替身答，真库与真适配器在 cmd/parcel-dispatch 的用例里证。

// identifiedMemberDouble 是节点作业收寄与「曾装它的单元」的替身：identity 是信封指的那份收寄，holders 按成员列它曾被装入的单元。
type identifiedMemberDouble struct {
	identity    ports.IdentifiedIntake
	identityErr error
	holders     map[string][]string
	holdersErr  map[string]error
	sources     []string
	holderReads []string
	// runaway 非零时，反向问到这个次数之后一律报读不动：给成环的册一个出口，去重被拿掉时用例红在断言上而不是挂住。
	runaway int
}

func (double *identifiedMemberDouble) FormedIntakeIdentity(
	_ context.Context,
	_ domain.TenantID,
	source string,
) (ports.IdentifiedIntake, error) {
	double.sources = append(double.sources, source)
	return double.identity, double.identityErr
}

func (double *identifiedMemberDouble) UnitsEverHolding(_ context.Context, _ domain.TenantID, member string) ([]string, error) {
	double.holderReads = append(double.holderReads, member)
	if double.runaway > 0 && len(double.holderReads) > double.runaway {
		return nil, errors.New("反向问没有停")
	}
	if err := double.holdersErr[member]; err != nil {
		return nil, err
	}
	return double.holders[member], nil
}

type memberReaskFixture struct {
	*reaskFixture
	identified *identifiedMemberDouble
}

// newMemberReaskFixture 立 segment-1：units 各在出发前一小时凭`已交接`进段。作业实物 hu-1 已识别成 parcel-1，曾被装进 bag-1。
func newMemberReaskFixture(t *testing.T, units ...string) *memberReaskFixture {
	t.Helper()
	return &memberReaskFixture{
		reaskFixture: newReaskFixture(t, units...),
		identified: &identifiedMemberDouble{
			identity:   ports.IdentifiedIntake{Identified: true, Handling: "hu-1", Parcel: mustRefValue(t, domain.NewCarriedObjectReference, "parcel-1")},
			holders:    map[string][]string{"hu-1": {"bag-1"}},
			holdersErr: map[string]error{},
		},
	}
}

func (fixture *memberReaskFixture) handler(t *testing.T) *application.ReaskDepartureGatesOnIdentifiedMemberHandler {
	t.Helper()
	handler, err := application.NewReaskDepartureGatesOnIdentifiedMemberHandler(application.ReaskDepartureGatesOnIdentifiedMemberDeps{
		Identified: fixture.identified,
		Segments:   fixture.segments,
		Facts:      fixture.facts,
		Reasks:     fixture.reasks,
		Gate:       fixture.gate.reader(t),
		Handoff:    fixture.handoff,
		Clock:      fixedClock{at: movementRecordedAt.Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("装成员补问编排：%v", err)
	}
	return handler
}

func (fixture *memberReaskFixture) plan(t *testing.T) application.DepartureGateReaskResult {
	t.Helper()
	result, err := fixture.handler(t).Plan(t.Context(), onNodeIntakeOf(t, "intake-1"))
	if err != nil {
		t.Fatalf("补问：%v", err)
	}
	return result
}

func onNodeIntakeOf(t *testing.T, source string) application.ReaskOnNodeIntakeCommand {
	t.Helper()
	return application.ReaskOnNodeIntakeCommand{TenantID: mustRefValue(t, domain.NewTenantID, "tenant-1"), Source: source}
}

// bagFirstReading 是袋出发时问关务的首版读数：袋里 members 各有一格，袋那一格取它们里最重的一格，unresolved 是出发那一刻身份不成立、
// 没有自己一格的成员，让袋那一格答未决（ADR-0225 决定三）。
func bagFirstReading(t *testing.T, bag string, members []domain.DepartureGateFinding, unresolved ...string) domain.DepartureGateReading {
	t.Helper()
	var gaps []domain.ConsolidationMemberGap
	if len(unresolved) > 0 {
		gaps = append(gaps, domain.ConsolidationMemberGap{Stop: domain.ConsolidationMemberIdentityUnresolved, Sources: unresolved})
	}
	unit, err := domain.FormConsolidationUnitGateFinding(mustRefValue(t, domain.NewCarriedObjectReference, bag), members, gaps)
	if err != nil {
		t.Fatalf("袋那一格：%v", err)
	}
	return askedFirstReading(t, append([]domain.DepartureGateFinding{unit}, members...)...)
}

// departedWithUnresolvedMember 在 segment-1 上登一条袋出发：首版读数里 hu-1 身份未决、没有自己一格。袋里另有 hu-2（parcel-2）已识别。
func (fixture *memberReaskFixture) departedWithUnresolvedMember(t *testing.T, fact string) ports.MovementFactRecord {
	t.Helper()
	fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "parcel-1"), parcelMember(t, "hu-2", "parcel-2")}
	return fixture.departOn(t, fact, "segment-1", bagFirstReading(t, "bag-1",
		[]domain.DepartureGateFinding{cell(t, "parcel-2", domain.DepartureGateNotGuarded, "OUTSIDE_CUSTOMS_BOOKS")}, "hu-1"))
}

func recordedReask(t *testing.T, departure ports.MovementFactRecord, object string, sequence int, reading domain.DepartureGateReading) ports.DepartureGateReaskRecord {
	t.Helper()
	return ports.DepartureGateReaskRecord{
		Key: ports.DepartureGateReaskKey{
			TenantID: departure.Key.TenantID, Fact: departure.Key.Fact, Version: departure.Key.Version,
			Object: mustRefValue(t, domain.NewCarriedObjectReference, object), Sequence: sequence,
		},
		Reading: reading, Trigger: "earlier", RecordedAt: movementRecordedAt,
	}
}

// Covers: AT-TF-364 编排半边（ADR-0331 决定一至三）——袋出发时 hu-1 身份未决、袋那一格答未决、关务没有被问过它；之后 hu-1 识别成 parcel-1，
// 节点收寄形成信封上的补问向节点作业问它曾装进 bag-1，bag-1 在那条原版出发上在控、出发那一刻 parcel-1 正随它走：按原出发时点（班次起讫、
// 出发的业务时间）只替 parcel-1 问关务，袋与同袋的 parcel-2 一概不问，追加第 1 次补问并同一笔交一封信，键是 parcel-1；首版读数不改。
func TestAMemberIdentifiedAfterItsBagDepartedIsAskedAtTheDepartureMomentAndOnlyThatMember(t *testing.T) {
	fixture := newMemberReaskFixture(t, "bag-1")
	departure := fixture.departedWithUnresolvedMember(t, "move-1")
	fixture.gate.customs.answers["parcel-1"] = domain.DepartureGateBlocked
	handler := fixture.handler(t)

	planned, err := handler.Plan(t.Context(), onNodeIntakeOf(t, "intake-1"))
	if err != nil {
		t.Fatalf("补问：%v", err)
	}
	if planned.Outcome() != application.DepartureGateReaskScanned || len(planned.Drafts()) != 1 {
		t.Fatalf("补问 = %s %s，%d 份", planned.Outcome(), planned.Stop(), len(planned.Drafts()))
	}
	if !slices.Equal(fixture.identified.sources, []string{"intake-1"}) || !slices.Equal(fixture.identified.holderReads, []string{"hu-1", "bag-1"}) {
		t.Fatalf("问节点作业 = 收寄 %v 曾装它的单元 %v，要按信封键读收寄、对这件实物与它的单元逐层各问一次", fixture.identified.sources, fixture.identified.holderReads)
	}
	if len(fixture.gate.customs.queries) != 1 || fixture.gate.customs.queries[0].Object.String() != "parcel-1" ||
		!fixture.gate.customs.queries[0].At.Equal(movedAtFixture) || fixture.gate.customs.queries[0].From.String() != "NODE-SHA" ||
		fixture.gate.customs.queries[0].To.String() != "NODE-SZX" {
		t.Fatalf("问关务 = %+v，只问 parcel-1、按原出发时点与班次方向，袋与同袋的 parcel-2 不问", fixture.gate.customs.queries)
	}
	if len(fixture.reasks.saved) != 0 || len(fixture.handoff.handed) != 0 {
		t.Fatal("问关务那一步就写了册或交了信——写要等进写事务")
	}

	recorded, err := handler.Record(t.Context(), planned)
	if err != nil {
		t.Fatalf("落册：%v", err)
	}
	if recorded.Disposition() != application.DepartureGateReaskSettled || recorded.Recorded() != 1 {
		t.Fatalf("落册 = %s，追加 %d 份", recorded.Disposition(), recorded.Recorded())
	}
	saved := fixture.reasks.saved[0]
	if saved.Key.Departure() != departure.Key || saved.Key.Object.String() != "parcel-1" || saved.Key.Sequence != 1 ||
		saved.Trigger != "node-intake/intake-1" || !saved.RecordedAt.Equal(movementRecordedAt.Add(time.Hour)) {
		t.Fatalf("补问那一行 = %+v", saved)
	}
	findings := saved.Reading.Findings()
	if saved.Reading.Answer() != domain.DepartureGateBlocked || len(findings) != 1 || findings[0].Object().String() != "parcel-1" {
		t.Fatalf("补问读数 = %s %+v，只该有这一件的一格", saved.Reading.Answer(), findings)
	}
	if _, unit := findings[0].ConsolidationUnit(); unit {
		t.Fatal("补问读数里不该有袋那一格")
	}
	if len(fixture.handoff.handed) != 1 || fixture.handoff.handed[0].Key != saved.Key {
		t.Fatalf("补问信封 = %+v", fixture.handoff.handed)
	}
	if first, _ := departure.Fact.GateReading(); first.Answer() != domain.DepartureGateUndecided {
		t.Fatalf("首版读数被改成了 %s", first.Answer())
	}
}

// Covers: AT-TF-364 编排半边（ADR-0331 决定二）——嵌套：段里在控的是笼，笼装着袋、袋装着 hu-1；候选单元逐层向上（袋、笼，各问一次），
// 在笼有参与的段上用同一口成员关系逐层往下验证 parcel-1 随笼走，追加一份。
func TestANestedBagIsFoundThroughTheCageAndVerifiedLayerByLayer(t *testing.T) {
	fixture := newMemberReaskFixture(t, "cage-1")
	fixture.identified.holders["bag-1"] = []string{"cage-1"}
	fixture.gate.memberships.units["cage-1"] = []ports.ConsolidationMember{nestedMember(t, "bag-1")}
	fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "parcel-1")}
	fixture.departOn(t, "move-1", "segment-1", bagFirstReading(t, "cage-1", nil, "hu-1"))

	planned := fixture.plan(t)
	drafts := planned.Drafts()
	if len(drafts) != 1 || drafts[0].Key.Object.String() != "parcel-1" || drafts[0].Key.Sequence != 1 {
		t.Fatalf("补问 %s %s：%+v", planned.Outcome(), planned.Stop(), drafts)
	}
	if asked := askedObjects(fixture.gate.customs); !slices.Equal(asked, []string{"parcel-1"}) {
		t.Fatalf("问关务的对象 = %v", asked)
	}
}

// Covers: AT-TF-364 编排半边（ADR-0331 决定三）——候选只放宽、验证才收紧：这件实物曾被装进 bag-1，但出发那一刻已不在袋里（出袋之后才
// 识别成功）、或验证到它的身份此刻确定地不是这件包裹（是另一件、不成立），都不补问、不问关务，这一拍入账收工。目标成员自己的身份读不动
// 不在此列，见 TestATargetMembersOwnUnreadableIdentityIsUndecidedAndAnotherMembersIsSkipped。
func TestAMemberThatWasNotInTheBagAtDepartureOrIsAnotherParcelNowIsNotAsked(t *testing.T) {
	cases := map[string][]ports.ConsolidationMember{
		"出发那一刻已出袋":   {parcelMember(t, "hu-2", "parcel-2")},
		"身份此刻是另一件包裹": {parcelMember(t, "hu-1", "parcel-9")},
		"身份此刻不成立":    {{Kind: ports.ConsolidationMemberIdentityUnresolved, Handling: "hu-1"}},
		"袋里是别的嵌套单元":  {nestedMember(t, "bag-2")},
	}
	for name, members := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newMemberReaskFixture(t, "bag-1")
			fixture.departedWithUnresolvedMember(t, "move-1")
			fixture.gate.memberships.units["bag-1"] = members
			fixture.gate.memberships.units["bag-2"] = []ports.ConsolidationMember{parcelMember(t, "hu-3", "parcel-3")}

			planned := fixture.plan(t)
			if planned.Outcome() != application.DepartureGateReaskScanned || len(planned.Drafts()) != 0 || len(fixture.gate.customs.queries) != 0 {
				t.Fatalf("补问 = %s %s，%d 份，问关务 %d 次", planned.Outcome(), planned.Stop(), len(planned.Drafts()), len(fixture.gate.customs.queries))
			}
		})
	}
}

// Covers: AT-TF-364 编排半边（ADR-0331 决定三，ADR-0029）——目标成员自己的身份此刻读不动，是读不动不是「不在袋里」：这封信是它唯一的唤醒，
// 答不在就入账收工、受限的出发永远没有信号，所以记未决、整封重投，没有补问、不问关务；读回之后重投照常补问。同袋别的成员身份读不动，都不是
// 这件包裹，跳过并继续（不拦住目标成员被找到，也不让目标不在袋里时无谓地重投）。
func TestATargetMembersOwnUnreadableIdentityIsUndecidedAndAnotherMembersIsSkipped(t *testing.T) {
	unreadable := func(handling string) ports.ConsolidationMember {
		return ports.ConsolidationMember{Kind: ports.ConsolidationMemberIdentityUnreadable, Handling: handling}
	}
	t.Run("目标成员自己读不动：未决重投，读回之后补问", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.departedWithUnresolvedMember(t, "move-1")
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{unreadable("hu-1"), parcelMember(t, "hu-2", "parcel-2")}

		planned := fixture.plan(t)
		if planned.Stop() != application.DepartureGateReaskMembershipUnavailable || planned.Disposition() != application.DepartureGateReaskRetry ||
			len(planned.Drafts()) != 0 || len(fixture.gate.customs.queries) != 0 {
			t.Fatalf("补问 = %s %s %s，%d 份，问关务 %d 次，want 成员关系读不动、未决、不问关务", planned.Outcome(), planned.Stop(),
				planned.Disposition(), len(planned.Drafts()), len(fixture.gate.customs.queries))
		}
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "parcel-1"), parcelMember(t, "hu-2", "parcel-2")}
		if drafts := fixture.plan(t).Drafts(); len(drafts) != 1 || drafts[0].Key.Object.String() != "parcel-1" {
			t.Fatalf("读回之后重投 = %+v", drafts)
		}
	})
	t.Run("嵌套的袋里目标成员自己读不动", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "cage-1")
		fixture.identified.holders["bag-1"] = []string{"cage-1"}
		fixture.gate.memberships.units["cage-1"] = []ports.ConsolidationMember{nestedMember(t, "bag-1")}
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{unreadable("hu-1")}
		fixture.departOn(t, "move-1", "segment-1", bagFirstReading(t, "cage-1", nil, "hu-1"))
		if planned := fixture.plan(t); planned.Stop() != application.DepartureGateReaskMembershipUnavailable ||
			planned.Disposition() != application.DepartureGateReaskRetry {
			t.Fatalf("补问 = %s %s %s", planned.Outcome(), planned.Stop(), planned.Disposition())
		}
	})
	t.Run("同袋别的成员读不动：跳过，目标成员照常找到", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.departedWithUnresolvedMember(t, "move-1")
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{unreadable("hu-9"), parcelMember(t, "hu-1", "parcel-1")}
		if drafts := fixture.plan(t).Drafts(); len(drafts) != 1 || drafts[0].Key.Object.String() != "parcel-1" {
			t.Fatalf("补问 = %+v", drafts)
		}
	})
	t.Run("同袋别的成员读不动、目标成员不在袋里：入账收工", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.departedWithUnresolvedMember(t, "move-1")
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{unreadable("hu-9"), parcelMember(t, "hu-2", "parcel-2")}
		planned := fixture.plan(t)
		if planned.Outcome() != application.DepartureGateReaskScanned || planned.Disposition() != application.DepartureGateReaskSettled ||
			len(planned.Drafts()) != 0 || len(fixture.gate.customs.queries) != 0 {
			t.Fatalf("补问 = %s %s %d 份", planned.Outcome(), planned.Disposition(), len(planned.Drafts()))
		}
	})
	t.Run("目标成员自己身份不成立：确定地不随袋走，入账收工", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.departedWithUnresolvedMember(t, "move-1")
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{
			{Kind: ports.ConsolidationMemberIdentityUnresolved, Handling: "hu-1"}, parcelMember(t, "hu-2", "parcel-2")}
		planned := fixture.plan(t)
		if planned.Disposition() != application.DepartureGateReaskSettled || len(planned.Drafts()) != 0 || len(fixture.gate.customs.queries) != 0 {
			t.Fatalf("补问 = %s %s %d 份", planned.Outcome(), planned.Disposition(), len(planned.Drafts()))
		}
	})
}

// Covers: AT-TF-365 编排半边（ADR-0331 决定三）——向上找候选是宽度优先、每件单元只问一次，成环与菱形的册也走得完：hu-1 曾被装进 bag-1 与
// bag-2，两者都曾被装进 cage-1，cage-1 又曾被装进 bag-1。曾装它的单元的反向问每件恰一次、能终止，经 cage-1 补问一份。
func TestUnitsAboveTheMemberAreFoundOnceEachThroughADiamondAndACycle(t *testing.T) {
	fixture := newMemberReaskFixture(t, "cage-1")
	fixture.identified.holders = map[string][]string{
		"hu-1": {"bag-1", "bag-2"}, "bag-1": {"cage-1"}, "bag-2": {"cage-1"}, "cage-1": {"bag-1"},
	}
	// 成环的册一旦不去重就会一直问下去：问到这个数还没停，替身报读不动，让用例红在断言上而不是挂住。
	fixture.identified.runaway = 20
	fixture.gate.memberships.units["cage-1"] = []ports.ConsolidationMember{nestedMember(t, "bag-1")}
	fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "parcel-1")}
	fixture.departOn(t, "move-1", "segment-1", bagFirstReading(t, "cage-1", nil, "hu-1"))

	planned := fixture.plan(t)
	if want := []string{"hu-1", "bag-1", "bag-2", "cage-1"}; !slices.Equal(fixture.identified.holderReads, want) {
		t.Fatalf("反向问了 %v, want 每件恰一次 %v", fixture.identified.holderReads, want)
	}
	if planned.Outcome() != application.DepartureGateReaskScanned || len(planned.Drafts()) != 1 {
		t.Fatalf("补问 = %s %s，%d 份, want 经 cage-1 补问一份", planned.Outcome(), planned.Stop(), len(planned.Drafts()))
	}
}

// Covers: AT-TF-365 编排半边（ADR-0331 决定三）——同一（成员，出发）只在还没有定论时补问：已有定论（放行、不受门禁、阻断、关务答未决，
// 或译不回来）不问；可再问的读不动（关务调用失败）追加下一次，到三次为止；这件成员在袋里的一格已在首版读数里有定论、或在替袋补问
// 的那条链里已有定论，也不问——关务不被重复问同一件的同一次出发。
func TestAMemberIsAskedOnlyWhileItsDepartureHasNoVerdictForIt(t *testing.T) {
	unreadable := func(t *testing.T) domain.DepartureGateReading {
		return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateUnreadable, "CUSTOMS_ADMISSION_UNAVAILABLE"))
	}
	admitted := func(t *testing.T) domain.DepartureGateReading {
		return askedFirstReading(t, cell(t, "parcel-1", domain.DepartureGateAdmitted, "RELEASED"))
	}
	cases := map[string]struct {
		earlier  func(*testing.T) []ports.DepartureGateReaskRecord
		sequence int
		exhaust  int
	}{
		"从没补问过": {func(*testing.T) []ports.DepartureGateReaskRecord { return nil }, 1, 0},
		"上一次已放行": {func(t *testing.T) []ports.DepartureGateReaskRecord {
			return []ports.DepartureGateReaskRecord{{Reading: admitted(t)}}
		}, 0, 0},
		"上一次关务读不动": {func(t *testing.T) []ports.DepartureGateReaskRecord {
			return []ports.DepartureGateReaskRecord{{Reading: unreadable(t)}}
		}, 2, 0},
		"读不动补到第三次": {func(t *testing.T) []ports.DepartureGateReaskRecord {
			return []ports.DepartureGateReaskRecord{{Reading: unreadable(t)}, {Reading: unreadable(t)}}
		}, 3, 0},
		"读不动补满三次": {func(t *testing.T) []ports.DepartureGateReaskRecord {
			return []ports.DepartureGateReaskRecord{{Reading: unreadable(t)}, {Reading: unreadable(t)}, {Reading: unreadable(t)}}
		}, 0, 1},
		"替袋补问的那条链里已有它的定论": {func(t *testing.T) []ports.DepartureGateReaskRecord {
			return []ports.DepartureGateReaskRecord{{Key: ports.DepartureGateReaskKey{Object: mustRefValue(t, domain.NewCarriedObjectReference, "bag-1")},
				Reading: admitted(t)}}
		}, 0, 0},
		"替袋补问的那条链里它只是读不动": {func(t *testing.T) []ports.DepartureGateReaskRecord {
			return []ports.DepartureGateReaskRecord{{Key: ports.DepartureGateReaskKey{Object: mustRefValue(t, domain.NewCarriedObjectReference, "bag-1")},
				Reading: unreadable(t)}}
		}, 1, 0},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newMemberReaskFixture(t, "bag-1")
			departure := fixture.departedWithUnresolvedMember(t, "move-1")
			own := 0
			for _, earlier := range testCase.earlier(t) {
				record := earlier
				record.Key.TenantID, record.Key.Fact, record.Key.Version = departure.Key.TenantID, departure.Key.Fact, departure.Key.Version
				if record.Key.Object == (domain.CarriedObjectReference{}) {
					own++
					record.Key.Object = mustRefValue(t, domain.NewCarriedObjectReference, "parcel-1")
					record.Key.Sequence = own
				} else {
					record.Key.Sequence = 1
				}
				record.Trigger, record.RecordedAt = "earlier", movementRecordedAt
				fixture.reasks.records = append(fixture.reasks.records, record)
			}

			planned := fixture.plan(t)
			drafts := planned.Drafts()
			if testCase.sequence == 0 {
				if len(drafts) != 0 || len(fixture.gate.customs.queries) != 0 {
					t.Fatalf("不该补问却补了 %d 份、问了关务 %d 次", len(drafts), len(fixture.gate.customs.queries))
				}
			} else if len(drafts) != 1 || drafts[0].Key.Sequence != testCase.sequence {
				t.Fatalf("补问 %+v, want 第 %d 次", drafts, testCase.sequence)
			}
			if planned.Exhausted() != testCase.exhaust {
				t.Fatalf("到上限 %d 条, want %d", planned.Exhausted(), testCase.exhaust)
			}
		})
	}
}

// Covers: AT-TF-365 编排半边——首版读数里这件包裹自己已有一格（出发时它的身份已成立，被问过关务）：放行、阻断这类定论不问；那一格
// 是关务调用失败的读不动，才补。首版停在本侧的，可再问的停点补问、不可再问的不补问（同 ADR-0305 决定三）。
func TestAMemberAlreadyAnsweredInTheFirstReadingIsNotAskedAgain(t *testing.T) {
	cases := map[string]struct {
		first func(*testing.T) domain.DepartureGateReading
		due   bool
	}{
		"首版里它已放行": {func(t *testing.T) domain.DepartureGateReading {
			return bagFirstReading(t, "bag-1", []domain.DepartureGateFinding{cell(t, "parcel-1", domain.DepartureGateAdmitted, "RELEASED")})
		}, false},
		"首版里它已阻断": {func(t *testing.T) domain.DepartureGateReading {
			return bagFirstReading(t, "bag-1", []domain.DepartureGateFinding{cell(t, "parcel-1", domain.DepartureGateBlocked, "RESTRICTION_IN_FORCE")})
		}, false},
		"首版里它关务调用失败": {func(t *testing.T) domain.DepartureGateReading {
			return bagFirstReading(t, "bag-1", []domain.DepartureGateFinding{cell(t, "parcel-1", domain.DepartureGateUnreadable, "CUSTOMS_ADMISSION_UNAVAILABLE")})
		}, true},
		"首版停在段册连不上": {func(t *testing.T) domain.DepartureGateReading {
			return stoppedFirstReading(t, domain.DepartureSegmentRegistryUnavailable)
		}, true},
		"首版班次不服务该段": {func(t *testing.T) domain.DepartureGateReading {
			return stoppedFirstReading(t, domain.DepartureScheduleNotServingSegment)
		}, false},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newMemberReaskFixture(t, "bag-1")
			fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "parcel-1")}
			fixture.departOn(t, "move-1", "segment-1", testCase.first(t))

			planned := fixture.plan(t)
			if got := len(planned.Drafts()) == 1; got != testCase.due {
				t.Fatalf("补问 %d 份, due = %v", len(planned.Drafts()), testCase.due)
			}
		})
	}
}

// Covers: AT-TF-365 编排半边——只补原版出发、只补出发那一刻单元在控的那一条：移动与到达、更正版、出发那一刻袋还没进段的出发都不补；
// 同一条出发经几件候选单元（笼与它装着的袋都有参与同一段）到达时只问一次，不会撞同一个次序。
func TestOnlyTheOriginalDeparturesTheUnitWasUnderControlOfAreAskedAndEachOnlyOnce(t *testing.T) {
	t.Run("移动、到达与更正版", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "parcel-1")}
		unresolved := bagFirstReading(t, "bag-1", nil, "hu-1")
		fixture.facts.bySegment["segment-1"] = append(fixture.facts.bySegment["segment-1"],
			departureRecord(t, "move-arrive", "segment-1", domain.ArrivalFact, domain.DepartureGateReading{}))
		original := departureRecord(t, "move-1", "segment-1", domain.DepartureFact, unresolved)
		corrected, err := original.Fact.Correct(original.Fact.Location(), movedAtFixture,
			mustRefValue(t, domain.NewMovementSourceReference, "own-fleet/v2"),
			mustRefValue(t, domain.NewMovementFactVersion, "v2"), movementRecordedAt)
		if err != nil {
			t.Fatalf("更正：%v", err)
		}
		fixture.facts.bySegment["segment-1"] = append(fixture.facts.bySegment["segment-1"], ports.MovementFactRecord{
			Key:  ports.MovementFactKey{TenantID: original.Key.TenantID, Fact: original.Key.Fact, Version: corrected.Version()},
			Fact: corrected, RecordedAt: movementRecordedAt,
		})
		if drafts := fixture.plan(t).Drafts(); len(drafts) != 0 || len(fixture.gate.customs.queries) != 0 {
			t.Fatalf("移动、到达与更正版补问了 %d 份、问关务 %d 次", len(drafts), len(fixture.gate.customs.queries))
		}
	})
	t.Run("出发那一刻袋还不在段里", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "parcel-1")}
		record := departureRecord(t, "move-early", "segment-1", domain.DepartureFact, bagFirstReading(t, "bag-1", nil, "hu-1"))
		early, err := domain.RehydrateMovementFact(domain.RehydrateMovementFactSpec{
			TenantID: record.Key.TenantID, Fact: record.Key.Fact, Version: record.Key.Version,
			Schedule: record.Fact.Schedule(), Kind: domain.DepartureFact, Location: record.Fact.Location(),
			Source: record.Fact.Source(), OccurredAt: movedAtFixture.Add(-2 * time.Hour),
			Segment: mustRefValue(t, domain.NewFulfillmentSegmentReference, "segment-1"), GateReading: bagFirstReading(t, "bag-1", nil, "hu-1"),
		})
		if err != nil {
			t.Fatalf("早于进段的出发：%v", err)
		}
		record.Fact = early
		fixture.facts.bySegment["segment-1"] = []ports.MovementFactRecord{record}
		if drafts := fixture.plan(t).Drafts(); len(drafts) != 0 {
			t.Fatalf("袋那一刻不在控也补问了 %+v", drafts)
		}
	})
	t.Run("笼与袋都在段里、都装着它", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1", "cage-1")
		fixture.identified.holders["hu-1"] = []string{"bag-1", "cage-1"}
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "parcel-1")}
		fixture.gate.memberships.units["cage-1"] = []ports.ConsolidationMember{nestedMember(t, "bag-1")}
		fixture.departOn(t, "move-1", "segment-1", bagFirstReading(t, "bag-1", nil, "hu-1"))

		drafts := fixture.plan(t).Drafts()
		if len(drafts) != 1 || drafts[0].Key.Sequence != 1 || len(fixture.gate.customs.queries) != 1 {
			t.Fatalf("同一条出发经两件单元到达：补问 %+v、问关务 %d 次，要只问一次", drafts, len(fixture.gate.customs.queries))
		}
	})
	t.Run("袋走过两条出发", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "parcel-1")}
		fixture.departOn(t, "move-1", "segment-1", bagFirstReading(t, "bag-1", nil, "hu-1"))
		fixture.departOn(t, "move-2", "segment-1", bagFirstReading(t, "bag-1", nil, "hu-1"))

		drafts := fixture.plan(t).Drafts()
		if len(drafts) != 2 || drafts[0].Key.Fact == drafts[1].Key.Fact {
			t.Fatalf("两条出发各补一份：%+v", drafts)
		}
	})
}

// Covers: AT-TF-364 编排半边——信封指的收寄没有认出包裹（待识别、未形成、形成了却没有关联）：入账收工，不向节点作业问单元、不读段。
// 这件实物从没被装进过任何单元：同样收工。
func TestAnIntakeThatIdentifiedNoParcelOrWasNeverInABagAsksNothing(t *testing.T) {
	t.Run("没认出包裹", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.identified.identity = ports.IdentifiedIntake{}
		planned := fixture.plan(t)
		if planned.Outcome() != application.DepartureGateReaskMemberNotIdentified || planned.Disposition() != application.DepartureGateReaskSettled {
			t.Fatalf("补问 = %s %s", planned.Outcome(), planned.Disposition())
		}
		if len(fixture.identified.holderReads) != 0 || fixture.segments.lists != 0 {
			t.Fatal("没认出包裹却去找单元、读段了")
		}
	})
	t.Run("从没被装进过单元", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.identified.holders = map[string][]string{}
		planned := fixture.plan(t)
		if planned.Outcome() != application.DepartureGateReaskScanned || len(planned.Drafts()) != 0 || fixture.segments.lists != 0 {
			t.Fatalf("补问 = %s，%d 份，读段 %d 次", planned.Outcome(), len(planned.Drafts()), fixture.segments.lists)
		}
	})
}

// Covers: AT-TF-366 编排半边（ADR-0331 决定五，ADR-0029）——失败分格：收寄读不回、曾装它的单元读不动、成员关系读不动、对象的段或事实或补问册
// 读不动是未决、原样重投；收寄与键不符、单元标识立不成载运对象引用、成员关系译不回来、段行或事实行或补问行装不回、按对象列出的段按键答没有、
// 列出的事实没声明这一段是缺陷、重投不会变。关务那一问读不动不是失败：它是补问读数里的一格。
func TestMemberReaskFailuresLandInTheirPartition(t *testing.T) {
	unreadableRow := func(sentinel error) error { return fmt.Errorf("adapter: %w: bad row", sentinel) }
	cases := map[string]struct {
		arrange     func(*testing.T, *memberReaskFixture)
		stop        application.DepartureGateReaskStop
		disposition application.DepartureGateReaskDisposition
	}{
		"收寄读不回": {func(_ *testing.T, f *memberReaskFixture) { f.identified.identityErr = errors.New("册不可用") },
			application.DepartureGateReaskIntakeNotVisible, application.DepartureGateReaskRetry},
		"收寄立不成": {func(_ *testing.T, f *memberReaskFixture) {
			f.identified.identityErr = fmt.Errorf("adapter: %w", ports.ErrFormedIntakeUntranslatable)
		}, application.DepartureGateReaskIntakeInconsistent, application.DepartureGateReaskFailed},
		"曾装它的单元读不动": {func(_ *testing.T, f *memberReaskFixture) {
			f.identified.holdersErr["hu-1"] = errors.New("册不可用")
		},
			application.DepartureGateReaskConsolidationUnitsUnavailable, application.DepartureGateReaskRetry},
		"曾装它的单元译不回来": {func(_ *testing.T, f *memberReaskFixture) {
			f.identified.holdersErr["hu-1"] = fmt.Errorf("adapter: %w", ports.ErrConsolidationMembershipUntranslatable)
		}, application.DepartureGateReaskMembershipUntranslatable, application.DepartureGateReaskFailed},
		"单元标识立不成载运对象引用": {func(_ *testing.T, f *memberReaskFixture) { f.identified.holders["hu-1"] = []string{" "} },
			application.DepartureGateReaskMembershipUntranslatable, application.DepartureGateReaskFailed},
		"成员关系读不动": {func(_ *testing.T, f *memberReaskFixture) { f.gate.memberships.errors["bag-1"] = true },
			application.DepartureGateReaskMembershipUnavailable, application.DepartureGateReaskRetry},
		"目标成员自己的身份读不动": {func(_ *testing.T, f *memberReaskFixture) {
			f.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{{Kind: ports.ConsolidationMemberIdentityUnreadable, Handling: "hu-1"}}
		}, application.DepartureGateReaskMembershipUnavailable, application.DepartureGateReaskRetry},
		"成员关系译不回来": {func(_ *testing.T, f *memberReaskFixture) { f.gate.memberships.untranslatable["bag-1"] = true },
			application.DepartureGateReaskMembershipUntranslatable, application.DepartureGateReaskFailed},
		"成员落在封闭集外": {func(_ *testing.T, f *memberReaskFixture) {
			f.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{{Kind: ports.ConsolidationMemberKindInvalid, Handling: "hu-x"}}
		}, application.DepartureGateReaskMembershipUntranslatable, application.DepartureGateReaskFailed},
		"对象的段列不出": {func(_ *testing.T, f *memberReaskFixture) { f.segments.listErr = errors.New("册不可用") },
			application.DepartureGateReaskSegmentRegistryUnavailable, application.DepartureGateReaskRetry},
		"段读不动": {func(_ *testing.T, f *memberReaskFixture) { f.gate.segments.findErr = errors.New("册不可用") },
			application.DepartureGateReaskSegmentRegistryUnavailable, application.DepartureGateReaskRetry},
		"段那一行装不回": {func(_ *testing.T, f *memberReaskFixture) {
			f.gate.segments.findErr = unreadableRow(ports.ErrFulfillmentSegmentRecordUnreadable)
		}, application.DepartureGateReaskSegmentRecordUnreadable, application.DepartureGateReaskFailed},
		"按段列事实读不动": {func(_ *testing.T, f *memberReaskFixture) { f.facts.err = errors.New("册不可用") },
			application.DepartureGateReaskMovementFactRegistryUnavailable, application.DepartureGateReaskRetry},
		"事实那一行装不回": {func(_ *testing.T, f *memberReaskFixture) {
			f.facts.err = unreadableRow(ports.ErrMovementFactRecordUnreadable)
		}, application.DepartureGateReaskMovementFactRecordUnreadable, application.DepartureGateReaskFailed},
		"补问册读不动": {func(_ *testing.T, f *memberReaskFixture) { f.reasks.findErr = errors.New("册不可用") },
			application.DepartureGateReaskRegistryUnavailable, application.DepartureGateReaskRetry},
		"补问那一行装不回": {func(_ *testing.T, f *memberReaskFixture) {
			f.reasks.findErr = unreadableRow(ports.ErrDepartureGateReaskRecordUnreadable)
		}, application.DepartureGateReaskRecordUnreadable, application.DepartureGateReaskFailed},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newMemberReaskFixture(t, "bag-1")
			fixture.departedWithUnresolvedMember(t, "move-1")
			testCase.arrange(t, fixture)

			planned := fixture.plan(t)
			if planned.Stop() != testCase.stop || planned.Disposition() != testCase.disposition {
				t.Fatalf("补问 = %s %s %s, want %s %s", planned.Outcome(), planned.Stop(), planned.Disposition(), testCase.stop, testCase.disposition)
			}
			if len(planned.Drafts()) != 0 || len(fixture.reasks.saved) != 0 {
				t.Fatal("停下的这一拍不该有补问")
			}
		})
	}
	t.Run("关务读不动是补问读数里的一格", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.departedWithUnresolvedMember(t, "move-1")
		fixture.gate.customs.errors["parcel-1"] = true
		planned := fixture.plan(t)
		drafts := planned.Drafts()
		if planned.Disposition() != application.DepartureGateReaskSettled || len(drafts) != 1 ||
			drafts[0].Reading.Answer() != domain.DepartureGateUnreadable {
			t.Fatalf("关务读不动 = %s %+v", planned.Disposition(), drafts)
		}
	})
	t.Run("两步之间另一方先补了同一次", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.departedWithUnresolvedMember(t, "move-1")
		planned := fixture.plan(t)
		fixture.reasks.saveOutcome = ports.DepartureGateReaskAlreadyRegistered

		recorded, err := fixture.handler(t).Record(t.Context(), planned)
		if err != nil {
			t.Fatalf("落册：%v", err)
		}
		if recorded.Stop() != application.DepartureGateReaskSequenceTaken || recorded.Disposition() != application.DepartureGateReaskRetry ||
			len(fixture.handoff.handed) != 0 {
			t.Fatalf("撞键 = %s %s，交了 %d 封信", recorded.Stop(), recorded.Disposition(), len(fixture.handoff.handed))
		}
	})
	t.Run("补问交不出去", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.departedWithUnresolvedMember(t, "move-1")
		planned := fixture.plan(t)
		fixture.handoff.err = errors.New("交出口不可用")

		recorded, err := fixture.handler(t).Record(t.Context(), planned)
		if err != nil || recorded.Stop() != application.DepartureGateReaskHandoffUnavailable || recorded.Disposition() != application.DepartureGateReaskRetry {
			t.Fatalf("交不出去 = %s %s %v", recorded.Stop(), recorded.Disposition(), err)
		}
	})
}

// Covers: AT-TF-364 编排半边——构造期拒缺件：漏装的一口到第一封信才暴露，会被当成读不动反复重投。
func TestTheMemberReaskHandlerRefusesMissingDependencies(t *testing.T) {
	fixture := newMemberReaskFixture(t, "bag-1")
	reader := fixture.gate.reader(t)
	complete := application.ReaskDepartureGatesOnIdentifiedMemberDeps{
		Identified: fixture.identified, Segments: fixture.segments, Facts: fixture.facts, Reasks: fixture.reasks,
		Gate: reader, Handoff: fixture.handoff, Clock: fixedClock{},
	}
	if _, err := application.NewReaskDepartureGatesOnIdentifiedMemberHandler(complete); err != nil {
		t.Fatalf("装配齐全却被拒：%v", err)
	}
	for name, mutate := range map[string]func(*application.ReaskDepartureGatesOnIdentifiedMemberDeps){
		"收寄与曾装它的单元": func(d *application.ReaskDepartureGatesOnIdentifiedMemberDeps) { d.Identified = nil },
		"段册":        func(d *application.ReaskDepartureGatesOnIdentifiedMemberDeps) { d.Segments = nil },
		"事实册":       func(d *application.ReaskDepartureGatesOnIdentifiedMemberDeps) { d.Facts = nil },
		"补问册":       func(d *application.ReaskDepartureGatesOnIdentifiedMemberDeps) { d.Reasks = nil },
		"读数":        func(d *application.ReaskDepartureGatesOnIdentifiedMemberDeps) { d.Gate = nil },
		"交出":        func(d *application.ReaskDepartureGatesOnIdentifiedMemberDeps) { d.Handoff = nil },
		"时钟":        func(d *application.ReaskDepartureGatesOnIdentifiedMemberDeps) { d.Clock = nil },
	} {
		t.Run(name, func(t *testing.T) {
			deps := complete
			mutate(&deps)
			if _, err := application.NewReaskDepartureGatesOnIdentifiedMemberHandler(deps); err == nil {
				t.Fatal("缺一口也装起来了")
			}
		})
	}
}

// Covers: AT-TF-364 编排半边（ADR-0331 决定二）——读数路径：成员补问的读数只有这一件的一格，班次与段核袋进段的那次交接：交接声明的
// 节点与班次起讫对不上时停在「班次不服务该段」，不问关务；袋不在册的段停在「段不在册」。
func TestTheMemberReadingChecksTheBagsEntryHandoverAndStopsOnTheSameStopsAsADeparture(t *testing.T) {
	t.Run("袋进段的交接声明对不上班次", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.gate.segments = departureSegmentsDeclaring(t, "segment-1", "NODE-OTHER", "NODE-SZX", "bag-1")
		fixture.segments.departureSegmentsDouble = fixture.gate.segments
		fixture.gate.handovers.handovers = fixture.gate.segments.handovers
		fixture.gate.memberships.units["bag-1"] = []ports.ConsolidationMember{parcelMember(t, "hu-1", "parcel-1")}
		fixture.departOn(t, "move-1", "segment-1", bagFirstReading(t, "bag-1", nil, "hu-1"))

		drafts := fixture.plan(t).Drafts()
		if len(drafts) != 1 || !slices.Equal(stopWords(drafts[0].Reading), []string{"SCHEDULE_NOT_SERVING_SEGMENT"}) ||
			len(fixture.gate.customs.queries) != 0 {
			t.Fatalf("补问 %+v，问关务 %d 次，要停在班次不服务该段、不问关务", drafts, len(fixture.gate.customs.queries))
		}
	})
	t.Run("班次不在册", func(t *testing.T) {
		fixture := newMemberReaskFixture(t, "bag-1")
		fixture.departedWithUnresolvedMember(t, "move-1")
		fixture.gate.schedules.registered = false

		drafts := fixture.plan(t).Drafts()
		if len(drafts) != 1 || !slices.Equal(stopWords(drafts[0].Reading), []string{"SCHEDULE_NOT_REGISTERED"}) || len(fixture.gate.customs.queries) != 0 {
			t.Fatalf("补问 %+v，问关务 %d 次，要停在班次不在册、不问关务", drafts, len(fixture.gate.customs.queries))
		}
	})
}

// Covers: AT-TF-364 编排半边——CarriesMember 是出发展开用的那一口成员关系：逐层往下找、每件单元只问一次、成环走得完；身份不成立或读不动的成员
// 不是任何一件包裹。
func TestCarriesMemberWalksTheBagsLayerByLayerOnceEachAndEndsOnACycle(t *testing.T) {
	fixture := newMemberReaskFixture(t, "bag-1")
	memberships := fixture.gate.memberships
	memberships.units["cage-1"] = []ports.ConsolidationMember{nestedMember(t, "bag-1"), nestedMember(t, "bag-2")}
	memberships.units["bag-1"] = []ports.ConsolidationMember{nestedMember(t, "cage-1"), {Kind: ports.ConsolidationMemberIdentityUnresolved, Handling: "hu-p"}}
	memberships.units["bag-2"] = []ports.ConsolidationMember{nestedMember(t, "bag-1"), parcelMember(t, "hu-1", "parcel-1")}
	reader := fixture.gate.reader(t)
	tenant := mustRefValue(t, domain.NewTenantID, "tenant-1")
	cage := mustRefValue(t, domain.NewCarriedObjectReference, "cage-1")

	found, err := reader.CarriesMember(t.Context(), tenant, cage, mustRefValue(t, domain.NewCarriedObjectReference, "parcel-1"), "hu-1", movedAtFixture)
	if err != nil || !found {
		t.Fatalf("成环的册上找 parcel-1 = %v %v", found, err)
	}
	memberships.queries = nil
	missing, err := reader.CarriesMember(t.Context(), tenant, cage, mustRefValue(t, domain.NewCarriedObjectReference, "parcel-absent"), "hu-absent", movedAtFixture)
	if err != nil || missing {
		t.Fatalf("成环的册上找不存在的包裹 = %v %v", missing, err)
	}
	var asked []string
	for _, query := range memberships.queries {
		asked = append(asked, query.Object.String())
		if !query.At.Equal(movedAtFixture) || query.TenantID.String() != "tenant-1" {
			t.Fatalf("成员关系的问法 = %+v，时点要取出发的业务时间", query)
		}
	}
	slices.Sort(asked)
	if !slices.Equal(asked, []string{"bag-1", "bag-2", "cage-1"}) {
		t.Fatalf("问了 %v，每件单元只问一次", asked)
	}
	// 目标成员自己的身份读不动：作为读不动交回（不是译不回来，重投可能就好）；同袋别的成员读不动跳过。
	memberships.units["bag-2"] = []ports.ConsolidationMember{{Kind: ports.ConsolidationMemberIdentityUnreadable, Handling: "hu-1"}}
	_, err = reader.CarriesMember(t.Context(), tenant, cage, mustRefValue(t, domain.NewCarriedObjectReference, "parcel-1"), "hu-1", movedAtFixture)
	if err == nil || errors.Is(err, ports.ErrConsolidationMembershipUntranslatable) {
		t.Fatalf("目标成员自己读不动 err = %v, want 读不动（非译不回来）", err)
	}
	memberships.units["bag-2"] = []ports.ConsolidationMember{{Kind: ports.ConsolidationMemberIdentityUnreadable, Handling: "hu-9"}}
	if found, err := reader.CarriesMember(t.Context(), tenant, cage, mustRefValue(t, domain.NewCarriedObjectReference, "parcel-1"), "hu-1", movedAtFixture); err != nil || found {
		t.Fatalf("同袋别的成员读不动 = %v %v, want 跳过、没找到、不是错", found, err)
	}
}
