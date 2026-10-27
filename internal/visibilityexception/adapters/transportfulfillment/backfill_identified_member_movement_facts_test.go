package transportfulfillment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	venodeops "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/nodeoperations"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证节点收寄形成信封上的移动事实补派生（票 docs-consistency/code/74，ADR-0299）：一件作业实物后来识别成功，就回头找它随袋走过
// 的移动事实，凡事实发生那一刻它在袋里（逐层向上到段里在控的那一件）的，补到它新认出的包裹上；与移动事实那一路同一判据、同一幂等
// 键。派生一侧用真的派生编排接替身端口，段与事实的替身与控制事实信封上的补派生共用同包。

// identifiedMemberDouble 答「这封信指的收寄认出了哪件包裹」与「这个成员曾被装进哪几件单元」；asked 记下被问的成员，用例据此核
// 「没认出就一口都不问」「每件只问一次」。
type identifiedMemberDouble struct {
	identity    veports.FormedIntakeIdentity
	identityErr error
	holders     map[string][]string
	holderErrs  map[string]error
	asked       []string
}

func (double *identifiedMemberDouble) FormedIntakeIdentity(
	context.Context, vedomain.TenantID, string,
) (veports.FormedIntakeIdentity, error) {
	return double.identity, double.identityErr
}

func (double *identifiedMemberDouble) UnitsEverHolding(_ context.Context, _ vedomain.TenantID, member string) ([]string, error) {
	double.asked = append(double.asked, member)
	if err := double.holderErrs[member]; err != nil {
		return nil, err
	}
	return double.holders[member], nil
}

// unitSegmentsDouble 按对象答它有参与的段：登了的才有，没登的答没有；按键取段照 records。
type unitSegmentsDouble struct {
	byObject map[string][]tfports.FulfillmentSegmentKey
	records  map[tfports.FulfillmentSegmentKey]tfports.FulfillmentSegmentRecord
	listErr  error
	listed   []string
}

func (double *unitSegmentsDouble) FindSegmentsForObject(
	_ context.Context, _ tfdomain.TenantID, object tfdomain.CarriedObjectReference,
) ([]tfports.FulfillmentSegmentKey, error) {
	double.listed = append(double.listed, object.String())
	return double.byObject[object.String()], double.listErr
}

func (double *unitSegmentsDouble) FindByKey(
	_ context.Context, key tfports.FulfillmentSegmentKey,
) (tfports.FulfillmentSegmentRecord, bool, error) {
	record, found := double.records[key]
	return record, found, nil
}

// timedMembershipDouble 按（对象，时点）答成员关系：answer 拿到问询，交回那一刻的样子。用例用它演「袋在出发那一刻装着、之后拆了」。
type timedMembershipDouble struct {
	answer func(query veports.ConsolidationMembershipQuery) (veports.ConsolidationMembership, error)
	asked  []veports.ConsolidationMembershipQuery
}

func (double *timedMembershipDouble) MembersAt(
	_ context.Context, query veports.ConsolidationMembershipQuery,
) (veports.ConsolidationMembership, error) {
	double.asked = append(double.asked, query)
	return double.answer(query)
}

func (double *timedMembershipDouble) IsUnitAt(_ context.Context, query veports.ConsolidationMembershipQuery) (bool, error) {
	membership, err := double.MembersAt(context.Background(), query)
	return membership.Unit, err
}

type identifiedMemberFixture struct {
	subject    *adapter.BackfillIdentifiedMemberAdapter
	identified *identifiedMemberDouble
	segments   *unitSegmentsDouble
	facts      *segmentFactsDouble
	members    *membershipDouble
	*backfillFixture
}

// newIdentifiedMemberFixture 装一份「一袋 BAG-1 凭交接在出发前一小时进了 segment-1，出发先登，袋里的 HU-NEW 当时还没识别成功」的现场：
// 这封信说 HU-NEW 现在认出了 PCL-NEW，HU-NEW 曾被装进 BAG-1；袋里另有已识别的 HU-OLD（PCL-OLD）和仍待识别的 HU-PENDING。
func newIdentifiedMemberFixture(t *testing.T) *identifiedMemberFixture {
	t.Helper()
	base := newBackfillFixture(t, handoverMappingViewDouble{})
	segment := base.segments.records[base.segments.keys[0]]
	segments := &unitSegmentsDouble{
		byObject: map[string][]tfports.FulfillmentSegmentKey{"parcel-1": {segment.Key}},
		records:  map[tfports.FulfillmentSegmentKey]tfports.FulfillmentSegmentRecord{segment.Key: segment},
	}
	members := &membershipDouble{units: map[string][]veports.ConsolidationMember{
		"parcel-1": {parcelMember("HU-NEW", "PCL-NEW"), parcelMember("HU-OLD", "PCL-OLD"), unresolvedMember("HU-PENDING")},
	}}
	identified := &identifiedMemberDouble{
		identity: veports.FormedIntakeIdentity{
			Identified: true, Handling: "HU-NEW", Parcel: movementValue(t, vedomain.NewTrackedParcelReference, "PCL-NEW"),
		},
		holders: map[string][]string{"HU-NEW": {"parcel-1"}},
	}
	fixture := &identifiedMemberFixture{
		identified: identified, segments: segments, facts: base.facts, members: members, backfillFixture: base,
	}
	fixture.rebuild(t)
	return fixture
}

// rebuild 按当前的替身重新构造被测适配器：用例改了替身的内容或换了成员关系口之后调用。
func (fixture *identifiedMemberFixture) rebuild(t *testing.T) {
	t.Helper()
	subject, err := adapter.NewBackfillIdentifiedMemberAdapter(
		fixture.identified, fixture.segments, fixture.facts, fixture.members, fixture.derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	fixture.subject = subject
}

func formedNodeIntakeRef() veinbox.FormedNodeIntake {
	return veinbox.FormedNodeIntake{TenantID: "tenant-1", SourceID: "IDENTITY-RECHECK/v2"}
}

// Covers: AT-VE-218——袋出发时袋里的实物还没识别、没有可投的追踪包裹，出发入账不派生；之后这件实物识别成功，节点收寄形成那一封上的
// 补派生把出发补到它新认出的包裹上：类型词、带成员维的引用、版本与三个时间都同移动事实那一路，成员关系按出发那一刻问；只补这一件，
// 同袋已识别的和仍待识别的成员不被这一封碰，袋本身不立投影。
func TestAMemberIdentifiedAfterTheDepartureGetsTheDepartureAndNobodyElseDoes(t *testing.T) {
	fixture := newIdentifiedMemberFixture(t)

	if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	records := fixture.factsOfParcel("PCL-NEW")
	if len(records) != 1 {
		t.Fatalf("PCL-NEW 的事实 %d 份, want 1", len(records))
	}
	fact := records[0].Fact
	if fact.Kind().String() != "movement-departure" || fact.Fact().String() != "movement-fact/PCL-NEW/move-1" || fact.Version().String() != "v1" {
		t.Fatalf("补派生的事实 = %s %s %s", fact.Kind(), fact.Fact(), fact.Version())
	}
	if !fact.OccurredAt().Equal(movementDepartedAt) || !fact.EffectiveAt().Equal(movementDepartedAt) || !fact.ReceivedAt().Equal(movementRecordedAt) {
		t.Fatalf("发生/有效/接收时间 = %s / %s / %s", fact.OccurredAt(), fact.EffectiveAt(), fact.ReceivedAt())
	}
	if fixture.entriesOf(t, "PCL-NEW") != 1 {
		t.Fatalf("PCL-NEW 条目 %d 个, want 1", fixture.entriesOf(t, "PCL-NEW"))
	}
	for _, other := range []string{"PCL-OLD", "HU-PENDING", "parcel-1"} {
		if n := len(fixture.factsOfParcel(other)); n != 0 {
			t.Fatalf("%s 被补了 %d 份，这一封只欠 PCL-NEW", other, n)
		}
	}
	for _, query := range fixture.members.asked {
		if !query.At.Equal(movementDepartedAt) || query.TenantID.String() != "tenant-1" {
			t.Fatalf("成员关系问在 %s / %s，要按出发那一刻、本租户问", query.At, query.TenantID)
		}
	}
}

func (fixture *identifiedMemberFixture) factsOfParcel(parcel string) []veports.FactRecord {
	var found []veports.FactRecord
	for _, record := range fixture.store.byKey {
		if record.Fact.Parcel().String() == parcel {
			found = append(found, record)
		}
	}
	return found
}

// Covers: AT-VE-218——补派生与正常路径交进派生编排的是同一把键：识别之后补、补过的移动事实信封再重投、补派生信封重投，谁先谁后都只留一份
// 事实、一个条目。移动事实那一路先于识别跑时成员身份不成立、跳过；识别之后它再跑（重投）派生同一份，补派生答已有结果。
func TestTheIdentifiedMemberBackfillAndTheNormalPathLeaveOneEntryInEitherOrder(t *testing.T) {
	normalPath := func(t *testing.T, fixture *identifiedMemberFixture) {
		t.Helper()
		segment := fixture.segments.records[fixture.segments.byObject["parcel-1"][0]]
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
	}
	backfill := func(t *testing.T, fixture *identifiedMemberFixture) {
		t.Helper()
		if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
			t.Fatalf("补派生：%v", err)
		}
	}
	identifyLater := func(t *testing.T, fixture *identifiedMemberFixture) {
		t.Helper()
		// 正常路径当时看到的 HU-NEW 还待识别；识别成功之后册上才是 PCL-NEW。
		fixture.members.units["parcel-1"] = []veports.ConsolidationMember{
			unresolvedMember("HU-NEW"), parcelMember("HU-OLD", "PCL-OLD"), unresolvedMember("HU-PENDING"),
		}
	}
	identified := func(t *testing.T, fixture *identifiedMemberFixture) {
		t.Helper()
		fixture.members.units["parcel-1"] = []veports.ConsolidationMember{
			parcelMember("HU-NEW", "PCL-NEW"), parcelMember("HU-OLD", "PCL-OLD"), unresolvedMember("HU-PENDING"),
		}
	}

	for name, steps := range map[string][]func(*testing.T, *identifiedMemberFixture){
		"先正常（成员还待识别）后识别补":  {identifyLater, normalPath, identified, backfill},
		"先识别补后正常重投":        {backfill, normalPath},
		"补派生重投两次":          {backfill, backfill},
		"先正常（成员已识别）后补派生重投": {normalPath, backfill, backfill},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newIdentifiedMemberFixture(t)
			for _, step := range steps {
				step(t, fixture)
			}
			if facts, entries := len(fixture.factsOfParcel("PCL-NEW")), fixture.entriesOf(t, "PCL-NEW"); facts != 1 || entries != 1 {
				t.Fatalf("PCL-NEW 事实 %d 份、条目 %d 个，want 各 1", facts, entries)
			}
		})
	}
}

// Covers: AT-VE-219——袋套袋、袋在笼里：这件实物曾被装进 BAG-IN，BAG-IN 曾被装进 CAGE-1，段里在控的是笼；候选逐层向上找到笼，笼那一刻
// 的成员逐层展开到这件包裹，补上。向上找时每个成员只问一次，成环的册（笼又回装进袋）也走得完。
func TestAMemberIdentifiedInsideANestedBagGetsTheMovementOfTheCageUnderControl(t *testing.T) {
	fixture := newIdentifiedMemberFixture(t)
	fixture.identified.holders = map[string][]string{"HU-NEW": {"BAG-IN"}, "BAG-IN": {"parcel-1"}, "parcel-1": {"BAG-IN"}}
	fixture.members.units = map[string][]veports.ConsolidationMember{
		"parcel-1": {unitMember("BAG-IN")},
		"BAG-IN":   {parcelMember("HU-NEW", "PCL-NEW"), parcelMember("HU-OLD", "PCL-OLD")},
	}

	if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if n := len(fixture.factsOfParcel("PCL-NEW")); n != 1 {
		t.Fatalf("PCL-NEW 的事实 %d 份, want 1", n)
	}
	if n := len(fixture.factsOfParcel("PCL-OLD")); n != 0 {
		t.Fatalf("同袋别的成员被补了 %d 份", n)
	}
	counts := map[string]int{}
	for _, member := range fixture.identified.asked {
		counts[member]++
		if counts[member] > 1 {
			t.Fatalf("向上找时 %s 被问了 %d 次，want 每件只问一次（成环也走得完）", member, counts[member])
		}
	}
	if len(counts) != 3 {
		t.Fatalf("向上找问了 %v，want HU-NEW、BAG-IN、parcel-1 各一次", fixture.identified.asked)
	}
}

// Covers: AT-VE-219——候选只放宽、展开才收紧：按事实发生那一刻问，袋那一刻不装这件实物的不补（出发之后才装进的；出发前已移出的），出发
// 那一刻装着的补，之后拆了也补；段里在控的袋不是候选里的（这件实物从没被装进它）时连段都不读。
func TestOnlyTheMovementsFromTheMomentTheMemberWasInTheBagAreBackfilled(t *testing.T) {
	inBag := func(t *testing.T, from, until time.Time) *identifiedMemberFixture {
		t.Helper()
		fixture := newIdentifiedMemberFixture(t)
		timed := &timedMembershipDouble{answer: func(query veports.ConsolidationMembershipQuery) (veports.ConsolidationMembership, error) {
			if query.Object != "parcel-1" {
				return veports.ConsolidationMembership{}, nil
			}
			if query.At.Before(from) || !query.At.Before(until) {
				return veports.ConsolidationMembership{Unit: true, Members: []veports.ConsolidationMember{unresolvedMember("HU-PENDING")}}, nil
			}
			return veports.ConsolidationMembership{Unit: true, Members: []veports.ConsolidationMember{parcelMember("HU-NEW", "PCL-NEW")}}, nil
		}}
		fixture.members = &membershipDouble{}
		subject, err := adapter.NewBackfillIdentifiedMemberAdapter(
			fixture.identified, fixture.segments, fixture.facts, timed, fixture.derive)
		if err != nil {
			t.Fatalf("构造：%v", err)
		}
		fixture.subject = subject
		return fixture
	}
	for name, test := range map[string]struct {
		from, until time.Time
		want        int
	}{
		"出发那一刻在袋里，之后拆了": {movementDepartedAt.Add(-time.Hour), movementDepartedAt.Add(time.Minute), 1},
		"出发之后才装进":       {movementDepartedAt.Add(time.Minute), movementDepartedAt.Add(time.Hour), 0},
		"出发前已移出":        {movementDepartedAt.Add(-2 * time.Hour), movementDepartedAt.Add(-time.Hour), 0},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := inBag(t, test.from, test.until)
			if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
				t.Fatalf("handle: %v", err)
			}
			if n := len(fixture.factsOfParcel("PCL-NEW")); n != test.want {
				t.Fatalf("PCL-NEW 的事实 %d 份, want %d", n, test.want)
			}
		})
	}
	t.Run("段里在控的袋不在候选里", func(t *testing.T) {
		fixture := newIdentifiedMemberFixture(t)
		fixture.identified.holders = map[string][]string{"HU-NEW": {"BAG-OTHER"}}
		if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.store.byKey) != 0 || fixture.segments.listed[len(fixture.segments.listed)-1] != "BAG-OTHER" {
			t.Fatalf("不该补却派生了 %d 份；列段问的对象 = %v", len(fixture.store.byKey), fixture.segments.listed)
		}
		if fixture.facts.calls != 0 {
			t.Fatalf("候选袋没有参与的段，却列了 %d 次事实", fixture.facts.calls)
		}
	})
}

// Covers: AT-VE-220——实物身份后来被纠正成另一件包裹：这封旧信指的是 PCL-NEW，展开时册上 HU-NEW 此刻认的已是 PCL-OTHER，什么也不补——
// 不替 PCL-NEW 补（它已不是这件实物），也不越俎代庖替 PCL-OTHER 补（它有自己的信封）；先前已补到 PCL-NEW 上的那一份不撤。
func TestAnIdentityCorrectedToAnotherParcelBackfillsNothingForTheStaleEnvelope(t *testing.T) {
	fixture := newIdentifiedMemberFixture(t)
	if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
		t.Fatalf("先前认出 PCL-NEW 时：%v", err)
	}
	if n := len(fixture.factsOfParcel("PCL-NEW")); n != 1 {
		t.Fatalf("前提：PCL-NEW 的事实 %d 份, want 1", n)
	}

	fixture.members.units["parcel-1"] = []veports.ConsolidationMember{parcelMember("HU-NEW", "PCL-OTHER")}
	if err := fixture.subject.HandleFormedNodeIntake(t.Context(), veinbox.FormedNodeIntake{TenantID: "tenant-1", SourceID: "IDENTITY-RECHECK/v1"}); err != nil {
		t.Fatalf("旧信：%v", err)
	}
	if n := len(fixture.factsOfParcel("PCL-OTHER")); n != 0 {
		t.Fatalf("旧信替 PCL-OTHER 补了 %d 份", n)
	}
	if n := len(fixture.factsOfParcel("PCL-NEW")); n != 1 {
		t.Fatalf("先前补到 PCL-NEW 上的被动了：%d 份, want 1", n)
	}
}

// Covers: AT-VE-218 边界——没有可补的就入账收工：这封信没认出包裹（待识别、未形成、形成了却没有关联）时一口都不问；这件实物从没被装进过
// 任何袋时只问反向一口、不读段；候选袋没有参与任何段、段上没有声明它的事实、袋那一刻不装这件实物，都什么也不补。
func TestNothingIsBackfilledWhenThereIsNothingToHeal(t *testing.T) {
	t.Run("没认出包裹", func(t *testing.T) {
		fixture := newIdentifiedMemberFixture(t)
		fixture.identified.identity = veports.FormedIntakeIdentity{}
		if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.identified.asked) != 0 || len(fixture.segments.listed) != 0 || len(fixture.store.byKey) != 0 {
			t.Fatalf("没认出包裹却问了 %v、列了段 %v、派生了 %d 份", fixture.identified.asked, fixture.segments.listed, len(fixture.store.byKey))
		}
	})
	t.Run("从没被装进过任何袋", func(t *testing.T) {
		fixture := newIdentifiedMemberFixture(t)
		fixture.identified.holders = nil
		if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.identified.asked) != 1 || len(fixture.segments.listed) != 0 || len(fixture.store.byKey) != 0 {
			t.Fatalf("问了 %v、列了段 %v、派生了 %d 份", fixture.identified.asked, fixture.segments.listed, len(fixture.store.byKey))
		}
	})
	t.Run("候选袋没有参与任何段", func(t *testing.T) {
		fixture := newIdentifiedMemberFixture(t)
		fixture.segments.byObject = nil
		if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.store.byKey) != 0 || fixture.facts.calls != 0 {
			t.Fatalf("派生了 %d 份、列了 %d 次事实", len(fixture.store.byKey), fixture.facts.calls)
		}
	})
	t.Run("段上没有声明它的事实", func(t *testing.T) {
		fixture := newIdentifiedMemberFixture(t)
		fixture.facts.bySegment = nil
		if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.store.byKey) != 0 {
			t.Fatalf("派生了 %d 份", len(fixture.store.byKey))
		}
	})
}

// Covers: AT-VE-218 更正链——段上声明的事实带更正链时逐版补，次序同登记：更正版带替代关系回指前版，与正常路径逐封收到两版同形。
func TestABackfilledCorrectionChainOfAnIdentifiedMemberKeepsItsSupersession(t *testing.T) {
	fixture := newIdentifiedMemberFixture(t)
	original := movementRecord(t, tfdomain.DepartureFact)
	corrected, err := original.Fact.Correct(original.Fact.Location(), movementDepartedAt.Add(time.Minute), original.Fact.Source(),
		movementValue(t, tfdomain.NewMovementFactVersion, "v2"), movementRecordedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	key := fixture.segments.byObject["parcel-1"][0]
	fixture.facts.bySegment[key] = []tfports.MovementFactRecord{original, {
		Key:        tfports.MovementFactKey{TenantID: original.Key.TenantID, Fact: original.Key.Fact, Version: corrected.Version()},
		Fact:       corrected,
		RecordedAt: movementRecordedAt.Add(time.Hour),
	}}

	if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if n := len(fixture.factsOfParcel("PCL-NEW")); n != 2 {
		t.Fatalf("PCL-NEW 事实 %d 份, want 两版各一份", n)
	}
	for _, record := range fixture.store.byKey {
		if record.Fact.Version().String() != "v2" {
			continue
		}
		if supersedes, has := record.Fact.Supersedes(); !has || supersedes.String() != "v1" {
			t.Fatalf("更正版替代关系 = %q（%v），want 回指 v1", supersedes, has)
		}
	}
}

// Covers: AT-VE-221——各格失败落进各自的哨兵：收寄读不回、反向问节点作业读不动、列段读不动、列事实读不动、成员关系读不动、派生停在未决是
// 续办；收寄与键不符、反向答复立不成单元标识、成员关系答复立不成成员、按对象列得出的段按键取不到是确定性问题，不当未决。
func TestIdentifiedMemberBackfillFailuresKeepTheirSentinels(t *testing.T) {
	unreadable := errors.New("connection refused")
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, fixture *identifiedMemberFixture)
		want  error
	}{
		{"收寄读不回", func(_ *testing.T, f *identifiedMemberFixture) { f.identified.identityErr = unreadable }, adapter.ErrFormedIntakeNotVisible},
		{"收寄与键不符", func(_ *testing.T, f *identifiedMemberFixture) {
			f.identified.identityErr = fmt.Errorf("%w: disagrees", veports.ErrFormedIntakeUntranslatable)
		}, adapter.ErrFormedIntakeInconsistent},
		{"反向问节点作业读不动", func(_ *testing.T, f *identifiedMemberFixture) {
			f.identified.holderErrs = map[string]error{"HU-NEW": unreadable}
		}, adapter.ErrConsolidationMembershipNotVisible},
		{"嵌套单元的反向问读不动", func(_ *testing.T, f *identifiedMemberFixture) {
			f.identified.holders = map[string][]string{"HU-NEW": {"BAG-IN"}}
			f.identified.holderErrs = map[string]error{"BAG-IN": unreadable}
		}, adapter.ErrConsolidationMembershipNotVisible},
		{"反向答复立不成单元标识", func(_ *testing.T, f *identifiedMemberFixture) {
			f.identified.holderErrs = map[string]error{"HU-NEW": fmt.Errorf("%w: blank", veports.ErrConsolidationMembershipUntranslatable)}
		}, adapter.ErrConsolidationMembershipInconsistent},
		{"列段读不动", func(_ *testing.T, f *identifiedMemberFixture) { f.segments.listErr = unreadable }, adapter.ErrObjectSegmentsNotVisible},
		{"列事实读不动", func(_ *testing.T, f *identifiedMemberFixture) { f.facts.err = unreadable }, adapter.ErrMovementFactsOfSegmentNotVisible},
		{"成员关系读不动", func(t *testing.T, f *identifiedMemberFixture) {
			f.members.errs = map[string]error{"parcel-1": fmt.Errorf("read membership: %w", unreadable)}
		}, adapter.ErrConsolidationMembershipNotVisible},
		{"成员关系答复立不成成员", func(t *testing.T, f *identifiedMemberFixture) {
			f.members.errs = map[string]error{"parcel-1": fmt.Errorf("%w: bad", veports.ErrConsolidationMembershipUntranslatable)}
		}, adapter.ErrConsolidationMembershipInconsistent},
		{"列得出的段按键取不到", func(_ *testing.T, f *identifiedMemberFixture) {
			f.segments.records = map[tfports.FulfillmentSegmentKey]tfports.FulfillmentSegmentRecord{}
		}, adapter.ErrMovementFactRecordInconsistent},
		{"派生停在未决", nil, adapter.ErrProjectionUndecided},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newIdentifiedMemberFixture(t)
			if test.setup == nil {
				base := newBackfillFixture(t, handoverMappingViewDouble{err: unreadable})
				fixture.derive = base.derive
				fixture.rebuild(t)
			} else {
				test.setup(t, fixture)
				fixture.rebuild(t)
			}
			err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef())
			if !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
		})
	}
}

func TestTheIdentifiedMemberBackfillAdapterRefusesMissingDependencies(t *testing.T) {
	derive, _, _ := handoverDeriveHandler(t, handoverMappingViewDouble{}, handoverProjectionDownstreamDouble{})
	identified, segments, facts, members := &identifiedMemberDouble{}, &objectSegmentsDouble{}, &segmentFactsDouble{}, &membershipDouble{}
	for name, build := range map[string]func() error{
		"缺反向读口": func() error {
			_, err := adapter.NewBackfillIdentifiedMemberAdapter(nil, segments, facts, members, derive)
			return err
		},
		"缺段读口": func() error {
			_, err := adapter.NewBackfillIdentifiedMemberAdapter(identified, nil, facts, members, derive)
			return err
		},
		"缺按段列事实": func() error {
			_, err := adapter.NewBackfillIdentifiedMemberAdapter(identified, segments, nil, members, derive)
			return err
		},
		"缺成员关系读口": func() error {
			_, err := adapter.NewBackfillIdentifiedMemberAdapter(identified, segments, facts, nil, derive)
			return err
		},
		"缺派生": func() error {
			_, err := adapter.NewBackfillIdentifiedMemberAdapter(identified, segments, facts, members, nil)
			return err
		},
	} {
		if build() == nil {
			t.Fatalf("%s 构造成功了", name)
		}
	}
}

// nodeMembershipsDouble 与 nodeIdentitiesDouble 是节点作业的两本册：袋 parcel-1 装着 HU-NEW 与 HU-OLD，HU-OLD 已识别成 PCL-OLD，HU-NEW 的
// 收寄身份由用例决定。用真的成员关系适配器（venodeops）接在补派生下面，钉住「目标成员自己的身份这一刻读不动」从适配器一路到补派生答什么。
type nodeMembershipsDouble struct{}

func (nodeMembershipsDouble) ReadConsolidationMembershipAt(
	_ context.Context, _ nodomain.TenantID, object string, _ time.Time,
) (noports.ConsolidationMembershipAt, error) {
	if object != "parcel-1" {
		return noports.ConsolidationMembershipAt{}, nil
	}
	member := func(raw string) noports.ConsolidationMemberAt {
		id, err := nodomain.NewHandlingUnitID(raw)
		if err != nil {
			panic(err)
		}
		return noports.ConsolidationMemberAt{Member: id}
	}
	return noports.ConsolidationMembershipAt{Unit: true, Members: []noports.ConsolidationMemberAt{member("HU-NEW"), member("HU-OLD")}}, nil
}

type nodeIdentitiesDouble struct {
	newDown bool
}

func (double nodeIdentitiesDouble) FindIntakeIdentities(
	_ context.Context, _ nodomain.TenantID, unit nodomain.HandlingUnitID,
) ([]nodomain.IntakeIdentityObservation, error) {
	switch unit.String() {
	case "HU-NEW":
		if double.newDown {
			return nil, errors.New("收寄册读不动")
		}
		return nil, nil
	case "HU-OLD":
		version, _ := nodomain.NewIntakeResultVersion("v1")
		association, _ := nodomain.NewParcelAssociationReference("PCL-OLD")
		return []nodomain.IntakeIdentityObservation{{
			SourceID: "scan-old", Version: version, Association: association, ReceivedAt: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC),
		}}, nil
	}
	return nil, nil
}

// Covers: AT-VE-221 适配器半边（ADR-0299 失败分格，与 ADR-0331 决定三同口径）——目标成员自己的收寄身份这一刻读不动，补派生不能把它读成「展开
// 不到这封信指的包裹」而入账收工：这封信是它唯一的唤醒，收工就再没有补派生。成员关系适配器把收寄册读不动原样交回为错误，补派生记成
// 成员关系读不动的未决、整封重投，没有派生任何东西；收寄身份读得回却是不成立（待识别）是确定的答复，展开不到它，入账收工。
func TestTheTargetMembersOwnUnreadableIdentityKeepsTheBackfillUndecidedNotFinished(t *testing.T) {
	build := func(t *testing.T, identities nodeIdentitiesDouble) *identifiedMemberFixture {
		t.Helper()
		fixture := newIdentifiedMemberFixture(t)
		view, err := venodeops.NewConsolidationMembershipView(nodeMembershipsDouble{}, identities)
		if err != nil {
			t.Fatalf("成员关系适配器：%v", err)
		}
		subject, err := adapter.NewBackfillIdentifiedMemberAdapter(fixture.identified, fixture.segments, fixture.facts, view, fixture.derive)
		if err != nil {
			t.Fatalf("构造：%v", err)
		}
		fixture.subject = subject
		return fixture
	}
	t.Run("目标成员的收寄身份读不动：未决重投，什么也不派生", func(t *testing.T) {
		fixture := build(t, nodeIdentitiesDouble{newDown: true})
		err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef())
		if !errors.Is(err, adapter.ErrConsolidationMembershipNotVisible) || errors.Is(err, adapter.ErrConsolidationMembershipInconsistent) {
			t.Fatalf("err = %v, want 成员关系读不动的未决", err)
		}
		if n := len(fixture.factsOfParcel("PCL-NEW")); n != 0 {
			t.Fatalf("读不动的这一拍派生了 %d 份", n)
		}
	})
	t.Run("目标成员的收寄身份读得回却不成立：确定的答复，入账收工", func(t *testing.T) {
		fixture := build(t, nodeIdentitiesDouble{})
		if err := fixture.subject.HandleFormedNodeIntake(t.Context(), formedNodeIntakeRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if n := len(fixture.factsOfParcel("PCL-NEW")); n != 0 {
			t.Fatalf("身份不成立却派生了 %d 份", n)
		}
	})
}
