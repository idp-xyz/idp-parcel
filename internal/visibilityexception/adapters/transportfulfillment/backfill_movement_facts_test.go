package transportfulfillment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// 本文件证控制事实信封上的移动事实补派生（票 demo-chain-gaps/53，ADR-0224）：对象凭这一版`已交接`或场外揽收进段，声明了这一段、
// 发生那一刻对象在控的在册移动事实就在它身上补派生；与移动事实那一路同一判据、同一幂等键，两路谁先谁后都只留一份。派生一侧用真的派生
// 编排接替身端口，与移动事实那一份用例共用同包的替身。

type objectSegmentsDouble struct {
	keys    []tfports.FulfillmentSegmentKey
	listErr error
	records map[tfports.FulfillmentSegmentKey]tfports.FulfillmentSegmentRecord
	findErr error
	finds   int
}

func (double *objectSegmentsDouble) FindSegmentsForObject(
	context.Context, tfdomain.TenantID, tfdomain.CarriedObjectReference,
) ([]tfports.FulfillmentSegmentKey, error) {
	return double.keys, double.listErr
}

func (double *objectSegmentsDouble) FindByKey(
	_ context.Context, key tfports.FulfillmentSegmentKey,
) (tfports.FulfillmentSegmentRecord, bool, error) {
	double.finds++
	if double.findErr != nil {
		return tfports.FulfillmentSegmentRecord{}, false, double.findErr
	}
	record, found := double.records[key]
	return record, found, nil
}

type segmentFactsDouble struct {
	bySegment map[tfports.FulfillmentSegmentKey][]tfports.MovementFactRecord
	err       error
	calls     int
}

func (double *segmentFactsDouble) FindBySegment(
	_ context.Context, key tfports.FulfillmentSegmentKey,
) ([]tfports.MovementFactRecord, error) {
	double.calls++
	return double.bySegment[key], double.err
}

// backfillSegment 立 segment-1：handover 那一件凭这一版交接进段，起点 enteredAt；parcel-2 凭另一条依据在出发前一小时进段。
func backfillSegment(
	t *testing.T,
	handover tfdomain.TransportHandover,
	enteredAt time.Time,
) tfports.FulfillmentSegmentRecord {
	t.Helper()
	basis, ok := handover.ParticipationBasis()
	if !ok {
		t.Fatal("这一版交接给不出参与依据")
	}
	tenant := movementValue(t, tfdomain.NewTenantID, "tenant-1")
	reference := movementValue(t, tfdomain.NewFulfillmentSegmentReference, "segment-1")
	segment, err := tfdomain.RehydrateActualFulfillmentSegment(tfdomain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenant,
		Segment:  reference,
		Participations: []tfdomain.RehydrateParticipationSpec{
			{Object: handover.Object(), EntryKind: tfdomain.EnteredByTransportHandover, EntryBasis: basis, EnteredAt: enteredAt},
			{
				Object:     movementValue(t, tfdomain.NewCarriedObjectReference, "parcel-2"),
				EntryKind:  tfdomain.EnteredByTransportHandover,
				EntryBasis: movementValue(t, tfdomain.NewParticipationBasisReference, "HO/parcel-2"),
				EnteredAt:  movementDepartedAt.Add(-time.Hour),
			},
		},
	})
	if err != nil {
		t.Fatalf("重建段：%v", err)
	}
	return tfports.FulfillmentSegmentRecord{Key: tfports.FulfillmentSegmentKey{TenantID: tenant, Segment: reference}, Segment: segment}
}

type backfillFixture struct {
	subject   *adapter.BackfillMovementFactsAdapter
	handovers *handoverFinderDouble
	pickups   *pickupRegistryDouble
	segments  *objectSegmentsDouble
	facts     *segmentFactsDouble
	members   *membershipDouble
	derive    *veapplication.DeriveProjectionHandler
	store     *handoverFactStoreDouble
	views     *handoverProjectionStoreDouble
}

// newBackfillFixture 装一份「事实先登、对象后凭交接进段」的现场：parcel-1 凭 handover-result/v1 在出发前进 segment-1，
// segment-1 上声明着 move-1 / v1 一条出发。
func newBackfillFixture(t *testing.T, mapping handoverMappingViewDouble) *backfillFixture {
	t.Helper()
	handover := registeredHandover(t, tfdomain.ObjectHandedOver)
	segment := backfillSegment(t, handover.Handover, handover.Handover.JudgedAt())
	derive, store, views := handoverDeriveHandler(t, mapping, handoverProjectionDownstreamDouble{})
	fixture := &backfillFixture{
		handovers: &handoverFinderDouble{record: handover, found: true},
		pickups:   &pickupRegistryDouble{record: registeredPickup(t, "parcel-1"), found: true},
		segments: &objectSegmentsDouble{
			keys:    []tfports.FulfillmentSegmentKey{segment.Key},
			records: map[tfports.FulfillmentSegmentKey]tfports.FulfillmentSegmentRecord{segment.Key: segment},
		},
		facts: &segmentFactsDouble{bySegment: map[tfports.FulfillmentSegmentKey][]tfports.MovementFactRecord{
			segment.Key: {movementRecord(t, tfdomain.DepartureFact)},
		}},
		members: &membershipDouble{},
		derive:  derive,
		store:   store,
		views:   views,
	}
	subject, err := adapter.NewBackfillMovementFactsAdapter(
		fixture.handovers, fixture.pickups, fixture.segments, fixture.facts, fixture.members, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	fixture.subject = subject
	return fixture
}

func (fixture *backfillFixture) factsOf(parcel string) int {
	count := 0
	for _, record := range fixture.store.byKey {
		if record.Fact.Parcel().String() == parcel {
			count++
		}
	}
	return count
}

func (fixture *backfillFixture) entriesOf(t *testing.T, parcel string) int {
	t.Helper()
	projection, found := fixture.views.byKey["tenant-1/"+parcel]
	if !found {
		return 0
	}
	return len(projection.Entries())
}

// Covers: AT-VE-178——事实先于段登记、对象后凭`已交接`进段：交接那一封补出与正常路径同一份事实——类型词、带对象维的引用、版本与
// 发生/有效/接收时间都同移动事实那一路；只补这一件，同段别的对象不在这一封里补。
func TestAHandoverIntoTheSegmentBackfillsTheFactRecordedBeforeIt(t *testing.T) {
	fixture := newBackfillFixture(t, handoverMappingViewDouble{})

	if err := fixture.subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	var found bool
	for _, record := range fixture.store.byKey {
		fact := record.Fact
		if fact.Parcel().String() != "parcel-1" {
			t.Fatalf("补到了别的对象上：%s", fact.Parcel())
		}
		if fact.Kind().String() != "movement-departure" || fact.Fact().String() != "movement-fact/parcel-1/move-1" ||
			fact.Version().String() != "v1" {
			t.Fatalf("补派生的事实 = %s %s %s", fact.Kind(), fact.Fact(), fact.Version())
		}
		if !fact.OccurredAt().Equal(movementDepartedAt) || !fact.EffectiveAt().Equal(movementDepartedAt) ||
			!fact.ReceivedAt().Equal(movementRecordedAt) {
			t.Fatalf("发生/有效/接收时间 = %s / %s / %s", fact.OccurredAt(), fact.EffectiveAt(), fact.ReceivedAt())
		}
		found = true
	}
	if !found || fixture.entriesOf(t, "parcel-1") != 1 {
		t.Fatalf("parcel-1 没补出出发：事实 %d 份、条目 %d 个", fixture.factsOf("parcel-1"), fixture.entriesOf(t, "parcel-1"))
	}
}

// Covers: AT-VE-179——补派生与移动事实那一路交进派生编排的是同一把键：补过再来正常路径、正常路径派生过再补、补派生重投，
// 谁先谁后都只留一份事实、一个条目。
func TestTheBackfillAndTheNormalPathLeaveOneEntryInEitherOrder(t *testing.T) {
	normalPath := func(t *testing.T, fixture *backfillFixture) {
		t.Helper()
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
	}
	backfill := func(t *testing.T, fixture *backfillFixture) {
		t.Helper()
		if err := fixture.subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); err != nil {
			t.Fatalf("补派生：%v", err)
		}
	}

	for name, steps := range map[string][]func(*testing.T, *backfillFixture){
		"先补后正常":   {backfill, normalPath},
		"先正常后补":   {normalPath, backfill},
		"补派生重投两次": {backfill, backfill},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newBackfillFixture(t, handoverMappingViewDouble{})
			for _, step := range steps {
				step(t, fixture)
			}
			if facts, entries := fixture.factsOf("parcel-1"), fixture.entriesOf(t, "parcel-1"); facts != 1 || entries != 1 {
				t.Fatalf("parcel-1 事实 %d 份、条目 %d 个，want 各 1", facts, entries)
			}
		})
	}
}

// Covers: AT-VE-180——这一版交接没让对象在事实那一刻落在声明的段里，就没有可补的：拒收与待确认不转出控制（连段都不读）；对象不在
// 任何段；有参与的段不是这一版交接进的（不列事实）；进的段上没有声明它的事实；对象进段晚于事实发生。都入账不派生。
func TestAHandoverThatDoesNotPutTheObjectUnderControlAtTheFactBackfillsNothing(t *testing.T) {
	assertNothing := func(t *testing.T, fixture *backfillFixture) {
		t.Helper()
		if err := fixture.subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.store.byKey) != 0 {
			t.Fatalf("不该补却派生了 %d 份", len(fixture.store.byKey))
		}
	}

	for _, verdict := range []tfdomain.HandoverVerdict{tfdomain.HandoverRefused, tfdomain.HandoverPendingConfirmation} {
		t.Run(verdict.String(), func(t *testing.T) {
			fixture := newBackfillFixture(t, handoverMappingViewDouble{})
			fixture.handovers.record = registeredHandover(t, verdict)
			assertNothing(t, fixture)
			if fixture.segments.finds != 0 || fixture.facts.calls != 0 {
				t.Fatalf("不转出控制的交接读了段 %d 次、列了事实 %d 次", fixture.segments.finds, fixture.facts.calls)
			}
		})
	}
	t.Run("对象不在任何段", func(t *testing.T) {
		fixture := newBackfillFixture(t, handoverMappingViewDouble{})
		fixture.segments.keys = nil
		assertNothing(t, fixture)
	})
	t.Run("有参与的段不是这一版交接进的", func(t *testing.T) {
		fixture := newBackfillFixture(t, handoverMappingViewDouble{})
		other := registeredHandover(t, tfdomain.ObjectHandedOver)
		corrected, err := other.Handover.Correct(tfdomain.HandoverCorrection{
			Verdict:           tfdomain.ObjectHandedOver,
			ReleasingEvidence: movementValue(t, tfdomain.NewHandoverEvidenceReference, "seal-out"),
			ReceivingEvidence: movementValue(t, tfdomain.NewHandoverEvidenceReference, "seal-in"),
			Rule:              movementValue(t, tfdomain.NewHandoverRuleReference, "rule/v1"),
			Version:           movementValue(t, tfdomain.NewHandoverResultVersion, "handover-result/v2"),
			CorrectedAt:       handoverJudgedAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("更正交接：%v", err)
		}
		segment := backfillSegment(t, corrected, corrected.JudgedAt())
		fixture.segments.records[segment.Key] = segment
		assertNothing(t, fixture)
		if fixture.facts.calls != 0 {
			t.Fatalf("段不是这一版进的，却按段列了 %d 次事实", fixture.facts.calls)
		}
	})
	t.Run("进的段上没有声明它的事实", func(t *testing.T) {
		fixture := newBackfillFixture(t, handoverMappingViewDouble{})
		fixture.facts.bySegment = nil
		assertNothing(t, fixture)
	})
	t.Run("进段晚于事实发生", func(t *testing.T) {
		fixture := newBackfillFixture(t, handoverMappingViewDouble{})
		late := backfillSegment(t, fixture.handovers.record.Handover, movementDepartedAt.Add(time.Minute))
		fixture.segments.records[late.Key] = late
		assertNothing(t, fixture)
	})
}

// Covers: AT-VE-181——各格失败落进各自的哨兵：交接读不回、对象的段列不出或按键读不动、按段列事实读不动、派生停在未决是续办；
// 按对象列得出的段按键取不到、列出的事实没声明这一段、某一行装不回是仓储不变量已破，不当未决。
func TestBackfillFailuresKeepTheirSentinels(t *testing.T) {
	unreadable := errors.New("connection refused")
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, fixture *backfillFixture)
		want  error
	}{
		{"交接读不回", func(_ *testing.T, fixture *backfillFixture) { fixture.handovers.found = false }, adapter.ErrHandoverNotVisible},
		{"对象的段列不出", func(_ *testing.T, fixture *backfillFixture) { fixture.segments.listErr = unreadable },
			adapter.ErrObjectSegmentsNotVisible},
		{"段按键读不动", func(_ *testing.T, fixture *backfillFixture) { fixture.segments.findErr = unreadable },
			adapter.ErrObjectSegmentsNotVisible},
		{"按段列事实读不动", func(_ *testing.T, fixture *backfillFixture) { fixture.facts.err = unreadable },
			adapter.ErrMovementFactsOfSegmentNotVisible},
		{"按段列出的某一行装不回", func(_ *testing.T, fixture *backfillFixture) {
			fixture.facts.err = fmt.Errorf("find movement facts of segment: %w", tfports.ErrMovementFactRecordUnreadable)
		}, adapter.ErrMovementFactRecordInconsistent},
		{"派生停在未决", nil, adapter.ErrProjectionUndecided},
		{"列得出的段按键取不到", func(_ *testing.T, fixture *backfillFixture) {
			fixture.segments.records = map[tfports.FulfillmentSegmentKey]tfports.FulfillmentSegmentRecord{}
		}, adapter.ErrMovementFactRecordInconsistent},
		{"列出的事实没声明这一段", func(t *testing.T, fixture *backfillFixture) {
			key := fixture.segments.keys[0]
			elsewhere := tfports.FulfillmentSegmentKey{TenantID: key.TenantID,
				Segment: movementValue(t, tfdomain.NewFulfillmentSegmentReference, "segment-9")}
			fixture.segments.keys = []tfports.FulfillmentSegmentKey{elsewhere}
			moved := fixture.segments.records[key]
			segment, err := tfdomain.RehydrateActualFulfillmentSegment(tfdomain.RehydrateActualFulfillmentSegmentSpec{
				TenantID: key.TenantID, Segment: elsewhere.Segment, Participations: participationSpecs(moved.Segment),
			})
			if err != nil {
				t.Fatalf("重建段：%v", err)
			}
			fixture.segments.records = map[tfports.FulfillmentSegmentKey]tfports.FulfillmentSegmentRecord{
				elsewhere: {Key: elsewhere, Segment: segment},
			}
			fixture.facts.bySegment = map[tfports.FulfillmentSegmentKey][]tfports.MovementFactRecord{
				elsewhere: fixture.facts.bySegment[key],
			}
		}, adapter.ErrMovementFactRecordInconsistent},
	} {
		t.Run(test.name, func(t *testing.T) {
			mapping := handoverMappingViewDouble{}
			if test.setup == nil {
				mapping.err = unreadable
			}
			fixture := newBackfillFixture(t, mapping)
			if test.setup != nil {
				test.setup(t, fixture)
			}
			err := fixture.subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef())
			if !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
		})
	}
}

// participationSpecs 把段上的参与原样抄成重建规格，给「同一批参与挂到另一段号下」那一格用。
func participationSpecs(segment tfdomain.ActualFulfillmentSegment) []tfdomain.RehydrateParticipationSpec {
	var specs []tfdomain.RehydrateParticipationSpec
	for _, participation := range segment.Participations() {
		specs = append(specs, tfdomain.RehydrateParticipationSpec{
			Object:     participation.Object(),
			EntryKind:  participation.EntryKind(),
			EntryBasis: participation.EntryBasis(),
			EnteredAt:  participation.EnteredAt(),
		})
	}
	return specs
}

// Covers: AT-VE-182——段上的事实带更正链时逐版补，次序同登记：更正版带替代关系回指前版，与正常路径逐封收到两版同形；补派生
// 不交信号——适配器根本没有信号那一口，读数是登记那一刻问的，补问归补问机制。
func TestABackfilledCorrectionChainKeepsItsSupersession(t *testing.T) {
	fixture := newBackfillFixture(t, handoverMappingViewDouble{})
	original := movementRecord(t, tfdomain.DepartureFact)
	corrected, err := original.Fact.Correct(original.Fact.Location(), movementDepartedAt.Add(time.Minute), original.Fact.Source(),
		movementValue(t, tfdomain.NewMovementFactVersion, "v2"), movementRecordedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	key := fixture.segments.keys[0]
	fixture.facts.bySegment[key] = []tfports.MovementFactRecord{original, {
		Key:        tfports.MovementFactKey{TenantID: original.Key.TenantID, Fact: original.Key.Fact, Version: corrected.Version()},
		Fact:       corrected,
		RecordedAt: movementRecordedAt.Add(time.Hour),
	}}

	if err := fixture.subject.HandleRegisteredTransportHandover(t.Context(), registeredHandoverRef()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if n := fixture.factsOf("parcel-1"); n != 2 {
		t.Fatalf("parcel-1 事实 %d 份, want 两版各一份", n)
	}
	for _, record := range fixture.store.byKey {
		if record.Fact.Version().String() != "v2" {
			continue
		}
		if supersedes, has := record.Fact.Supersedes(); !has || supersedes.String() != "v1" {
			t.Fatalf("更正版替代关系 = %q（%v），want 回指 v1", supersedes, has)
		}
	}
	if n := fixture.entriesOf(t, "parcel-1"); n != 1 {
		t.Fatalf("parcel-1 条目 %d 个, want 1——被替代的前版不参与派生", n)
	}
}

func TestTheBackfillAdapterRefusesMissingDependencies(t *testing.T) {
	derive, _, _ := handoverDeriveHandler(t, handoverMappingViewDouble{}, handoverProjectionDownstreamDouble{})
	handovers, pickups := &handoverFinderDouble{}, &pickupRegistryDouble{}
	segments, facts, members := &objectSegmentsDouble{}, &segmentFactsDouble{}, &membershipDouble{}
	for name, build := range map[string]func() error{
		"缺交接读口": func() error {
			_, err := adapter.NewBackfillMovementFactsAdapter(nil, pickups, segments, facts, members, derive)
			return err
		},
		"缺揽收读口": func() error {
			_, err := adapter.NewBackfillMovementFactsAdapter(handovers, nil, segments, facts, members, derive)
			return err
		},
		"缺段读口": func() error {
			_, err := adapter.NewBackfillMovementFactsAdapter(handovers, pickups, nil, facts, members, derive)
			return err
		},
		"缺按段列事实": func() error {
			_, err := adapter.NewBackfillMovementFactsAdapter(handovers, pickups, segments, nil, members, derive)
			return err
		},
		"缺成员关系读口": func() error {
			_, err := adapter.NewBackfillMovementFactsAdapter(handovers, pickups, segments, facts, nil, derive)
			return err
		},
		"缺派生": func() error {
			_, err := adapter.NewBackfillMovementFactsAdapter(handovers, pickups, segments, facts, members, nil)
			return err
		},
	} {
		if build() == nil {
			t.Fatalf("%s 构造成功了", name)
		}
	}
}

// pickupFixture 把现场换成「事实先登、对象后凭场外揽收立段」：parcel-1 凭 pickup-result/v1 立 segment-1，parcel-2 凭另一次收寄
// 加入。
func pickupFixture(t *testing.T) *backfillFixture {
	t.Helper()
	fixture := newBackfillFixture(t, handoverMappingViewDouble{})
	key := fixture.segments.keys[0]
	established, err := tfdomain.EstablishSegmentWithPickup(key.Segment, fixture.pickups.record.Pickup, tfdomain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	joined, err := established.JoinWithPickup(registeredPickup(t, "parcel-2").Pickup, tfdomain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("加入：%v", err)
	}
	fixture.segments.records[key] = tfports.FulfillmentSegmentRecord{Key: key, Segment: joined}
	return fixture
}

// Covers: AT-VE-178（揽收）——对象凭场外揽收立段时，揽收那一封补出同一份事实；只补这一件。
func TestAPickupIntoTheSegmentBackfillsTheFactRecordedBeforeIt(t *testing.T) {
	fixture := pickupFixture(t)

	if err := fixture.subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if facts, entries := fixture.factsOf("parcel-1"), fixture.entriesOf(t, "parcel-1"); facts != 1 || entries != 1 {
		t.Fatalf("parcel-1 事实 %d 份、条目 %d 个，want 各 1", facts, entries)
	}
	if n := fixture.factsOf("parcel-2"); n != 0 {
		t.Fatalf("同段别的对象也在这一封里补了 %d 份", n)
	}
	for _, record := range fixture.store.byKey {
		if record.Fact.Fact().String() != "movement-fact/parcel-1/move-1" || record.Fact.Kind().String() != "movement-departure" {
			t.Fatalf("补派生的事实 = %s %s", record.Fact.Kind(), record.Fact.Fact())
		}
	}
}

// Covers: AT-VE-180、AT-VE-181（揽收）——收寄没带段（对象不在任何段）入账不补；有参与的段是凭交接进的、不是这一次收寄进的，不补；
// 揽收读不回是续办。
func TestAPickupThatPutsNoObjectIntoTheSegmentBackfillsNothing(t *testing.T) {
	t.Run("对象不在任何段", func(t *testing.T) {
		fixture := pickupFixture(t)
		fixture.segments.keys = nil
		if err := fixture.subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.store.byKey) != 0 {
			t.Fatalf("不该补却派生了 %d 份", len(fixture.store.byKey))
		}
	})
	t.Run("段是凭交接进的", func(t *testing.T) {
		fixture := newBackfillFixture(t, handoverMappingViewDouble{})
		if err := fixture.subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef()); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.store.byKey) != 0 || fixture.facts.calls != 0 {
			t.Fatalf("段不是这一次收寄进的，却列了 %d 次事实、派生了 %d 份", fixture.facts.calls, len(fixture.store.byKey))
		}
	})
	t.Run("揽收读不回", func(t *testing.T) {
		fixture := pickupFixture(t)
		fixture.pickups.found = false
		err := fixture.subject.HandleRegisteredOffsitePickup(t.Context(), registeredPickupRef())
		if !errors.Is(err, adapter.ErrPickupNotVisible) {
			t.Fatalf("err = %v, want ErrPickupNotVisible", err)
		}
	})
}
