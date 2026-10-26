package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证先于段登记的移动事实经控制事实那一封补进全程追踪（票 demo-chain-gaps/53，ADR-0224），走的是生产 wireDispatcher：出发
// 先登、段不在册，移动事实那一路入账不派生；随后一次`已交接`或一次对象级场外揽收经生产登记编排把对象送进那一段、立起段，那一封
// 信上的补派生在对象身上补出出发，按演示种子的里程碑映射归 DEPARTED。另证补派生两路的失败分区。证据只记 S。

const (
	backfillConsumerName         = "visibility-exception/backfill-movement-facts-on-transport-handover"
	backfillOnPickupConsumerName = "visibility-exception/backfill-movement-facts-on-offsite-pickup"
	backfillPickupAttempt        = "SYN-PICKUP-ATTEMPT-DEP-01"
	backfillScope                = "SYN-SCOPE-SHA-HUB-DISPATCH"
	backfillVersion              = "SYN-HANDOVER-RESULT/DEP-01/v1"
	backfillFact                 = "SYN-MOVE-BACKFILL-01"
)

// Covers: AT-VE-178 真库半边、票面判据「先登移动事实、后立段，追踪里出现该事实派生的里程碑」（证据 S）——出发先于段登记时投影里
// 没有它；对象经生产登记编排交接入段、立起段之后，生产路由一拍：交接那一件得到交接与出发两个条目，出发归 DEPARTED；没交接入段的
// 那一件仍然没有投影。
//
// Covers: AT-VE-179 真库半边——补派生之后，移动事实信封与交接信封各以新 ID 再投一次（越过 inbox 的重投），两路都答已有结果：条目
// 不多出第二个，事实库里这一件的这一版仍只有一份。
func TestADepartureRecordedBeforeItsSegmentReachesTrackingOnceTheHandoverEstablishesIt(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)

	factEvent := recordDepartureThroughProduction(t, fixture, backfillFact)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("出发那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, factEvent))
	}
	if entries := projectionEntries(t, fixture, movementParcelOnBoard); len(entries) != 0 {
		t.Fatalf("段不在册时投影里已有 %d 个条目——前提不成立", len(entries))
	}

	handoverEvent := registerHandoverIntoMovementSegment(t, fixture)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("交接那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, handoverEvent))
	}
	if n := fixture.countInbox(t, backfillConsumerName, handoverEvent); n != 1 {
		t.Fatalf("补派生 inbox 行数 = %d, want 1——路由表没把交接信封投给补派生那一路", n)
	}
	assertBackfilledDeparture(t, fixture)
	if entries := projectionEntries(t, fixture, movementParcelBlocked); len(entries) != 0 {
		t.Fatalf("没交接入段的那一件也补出了 %d 个条目", len(entries))
	}

	redeliverMovementFact(t, fixture)
	redeliverHandoverToBackfill(t, fixture)
	assertBackfilledDeparture(t, fixture)
	if n := acceptedFactVersions(t, fixture, "movement-fact/"+movementParcelOnBoard+"/"+backfillFact); n != 1 {
		t.Fatalf("事实库里这一版有 %d 份, want 1", n)
	}
}

// Covers: AT-VE-181 派发半边——补派生两路的失败分格：控制事实读不回、对象的段读不动、按段列事实读不动、派生编排未决落
// dispatch.consumer_undecided，其余保持 dispatch.publish_failed。名单取各路的哨兵变量本身，理由同移动事实那一份。
// Covers: AT-VE-198 派发半边（补派生两路）——集运成员读不动落未决，答复不一致保持发布失败。
func TestMovementFactBackfillFailuresLandInTheRightPartition(t *testing.T) {
	type failure struct {
		name     string
		err      error
		wantCode string
	}
	shared := []failure{
		{"对象的段读不动", vetf.ErrObjectSegmentsNotVisible, "dispatch.consumer_undecided"},
		{"按段列事实读不动", vetf.ErrMovementFactsOfSegmentNotVisible, "dispatch.consumer_undecided"},
		{"派生编排未决", vetf.ErrProjectionUndecided, "dispatch.consumer_undecided"},
		{"集运成员读不动", vetf.ErrConsolidationMembershipNotVisible, "dispatch.consumer_undecided"},
		{"段册或事实不变量已破", vetf.ErrMovementFactRecordInconsistent, "dispatch.publish_failed"},
		{"集运成员答复不一致", vetf.ErrConsolidationMembershipInconsistent, "dispatch.publish_failed"},
		{"事实引用译不出", vetf.ErrMovementFactUntranslatableAnswer, "dispatch.publish_failed"},
		{"投影意图待发", vetf.ErrProjectionHandoffPending, "dispatch.publish_failed"},
		{"投影封闭集合外", veconsume.ErrUnexpectedProjectionOutcome, "dispatch.publish_failed"},
	}
	lanes := []struct {
		name      string
		event     eventing.EventType
		sentinels []error
		own       []failure
	}{
		{"交接", veinbox.TransportHandoverRegisteredEventType, veMovementFactBackfillOnHandoverUndecidedSentinels, []failure{
			{"交接读不回", vetf.ErrHandoverNotVisible, "dispatch.consumer_undecided"},
			{"交接记录不变量已破", vetf.ErrHandoverRecordInconsistent, "dispatch.publish_failed"},
			{"交接引用译不出", vetf.ErrHandoverUntranslatableAnswer, "dispatch.publish_failed"},
		}},
		{"揽收", veinbox.OffsitePickupRegisteredEventType, veMovementFactBackfillOnPickupUndecidedSentinels, []failure{
			{"揽收读不回", vetf.ErrPickupNotVisible, "dispatch.consumer_undecided"},
			{"揽收记录不变量已破", vetf.ErrPickupRecordInconsistent, "dispatch.publish_failed"},
			{"揽收引用译不出", vetf.ErrPickupUntranslatableAnswer, "dispatch.publish_failed"},
		}},
	}
	for _, lane := range lanes {
		for _, test := range append(lane.own, shared...) {
			t.Run(lane.name+"/"+test.name, func(t *testing.T) {
				beat, db, store := beatWithSentinelledConsumer(t, &stallingConsumer{err: test.err}, lane.event, lane.sentinels)
				enqueueForBeat(t, db, store, "control-fact-1", lane.event, `{}`)

				published, err := beat.DispatchOnce(t.Context())
				if err != nil {
					t.Fatalf("一拍：%v", err)
				}
				if published != 0 {
					t.Fatalf("失败的投递被定稿了 %d 条", published)
				}
				if got := recordedFailureCode(t, db, "control-fact-1"); got != test.wantCode {
					t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
				}
			})
		}
	}
}

// Covers: AT-VE-178 真库半边（揽收）——出发先于段登记；对象随后经生产登记编排凭场外揽收立起那一段，生产路由一拍：揽收那一封上的
// 补派生在对象身上补出出发，归 DEPARTED。同一封信上 PS 来源采用那一路在本夹具里停在未决，不影响排在它前面的追踪两路落账。
func TestADepartureRecordedBeforeItsSegmentReachesTrackingOnceAPickupEstablishesIt(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)

	factEvent := recordDepartureThroughProduction(t, fixture, backfillFact)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("出发那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, factEvent))
	}

	pickupEvent := registerPickupIntoMovementSegment(t, fixture)
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("揽收那一拍：%v", err)
	}
	if n := fixture.countInbox(t, backfillOnPickupConsumerName, pickupEvent); n != 1 {
		t.Fatalf("补派生 inbox 行数 = %d, want 1——路由表没把揽收信封投给补派生那一路；失败码 = %q",
			n, recordedFailureCode(t, fixture.db, pickupEvent))
	}
	entries := projectionEntries(t, fixture, movementParcelOnBoard)
	var departed int
	for _, entry := range entries {
		if entry.Fact().Fact().String() != "movement-fact/"+movementParcelOnBoard+"/"+backfillFact {
			continue
		}
		departed++
		if milestone, classified := entry.Milestone(); !classified || milestone.String() != "DEPARTED" {
			t.Fatalf("补出的出发归类 %q（%v），want DEPARTED", milestone, classified)
		}
	}
	if departed != 1 {
		t.Fatalf("出发条目 %d 个, want 1（条目共 %d 个）", departed, len(entries))
	}
}

// Covers: AT-VE-204 真库半边（票 demo-chain-gaps/136，ADR-0302，证据 S）——出发先于段登记；对象随后不经任何一封控制事实信封进了那一段
// （控制事实进段那一半欠了账、日后续办补进段，或存量迁移、直写段册：这里直写段册），那一刻没有东西唤醒补派生，投影里仍没有这次
// 出发；这件对象此后的下一次`已交接`经生产登记编排把它交进下一段，生产路由一拍：那一封上的补派生扫对象有参与的每一段，在前一段上
// 补出这次出发，归 DEPARTED。此前补派生只看这一次交接进的那一段，这次出发就永远丢在追踪外面。
func TestTheNextHandoverBackfillsADepartureOnASegmentTheObjectEnteredWithoutAnEnvelope(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)

	factEvent := recordDepartureThroughProduction(t, fixture, backfillFact)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("出发那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, factEvent))
	}
	saveDepartureSegment(t, fixture)
	if entries := projectionEntries(t, fixture, movementParcelOnBoard); len(entries) != 0 {
		t.Fatalf("对象进段时没有信封，投影里却已有 %d 个条目——前提不成立", len(entries))
	}

	onward := registerOnwardHandover(t, fixture, movementParcelOnBoard)
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("交接那一拍：%v", err)
	}
	if n := fixture.countInbox(t, backfillConsumerName, onward); n != 1 {
		t.Fatalf("补派生 inbox 行数 = %d, want 1；失败码 = %q", n, recordedFailureCode(t, fixture.db, onward))
	}
	var departed int
	for _, entry := range projectionEntries(t, fixture, movementParcelOnBoard) {
		if entry.Fact().Fact().String() != "movement-fact/"+movementParcelOnBoard+"/"+backfillFact {
			continue
		}
		departed++
		if milestone, classified := entry.Milestone(); !classified || milestone.String() != "DEPARTED" {
			t.Fatalf("补出的出发归类 %q（%v），want DEPARTED", milestone, classified)
		}
	}
	if departed != 1 {
		t.Fatalf("前一段上的出发条目 %d 个, want 1——下一次交接没有扫到对象先前进的那一段", departed)
	}
	if entries := projectionEntries(t, fixture, movementParcelBlocked); len(entries) != 0 {
		t.Fatalf("没有下一份控制事实的那一件也补出了 %d 个条目——补派生只补这一件", len(entries))
	}
}

// registerOnwardHandover 经生产登记编排登 object 出发两小时后的下一次`已交接`：交进下一段（结束它在出发那一段的参与、立起下一段）。
// 交回交接信封的事件 ID。
func registerOnwardHandover(t *testing.T, fixture *synVerticalFixture, object string) string {
	t.Helper()
	const (
		onwardScope   = "SYN-SCOPE-SZX-GATE-ONWARD"
		onwardVersion = "SYN-HANDOVER-RESULT/DEP-ONWARD/v1"
	)
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	register := newHandoverRegistration(t, fixture.db, store)
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	command := tfapplication.RegisterTransportHandoverCommand{
		TenantID:          tenant,
		Object:            object,
		Scope:             onwardScope,
		ReleasedBy:        "SYN-OWN-FLEET",
		ReceivedBy:        "SYN-NODE-SZX-GATE",
		Verdict:           tfdomain.ObjectHandedOver,
		ReleasingEvidence: "SYN-EVIDENCE-RELEASE-ONWARD",
		ReceivingEvidence: "SYN-EVIDENCE-RECEIVE-ONWARD",
		Rule:              "SYN-HANDOVER-RULE/v1",
		Version:           onwardVersion,
		JudgedAt:          movementDepartedAt.Add(2 * time.Hour),
		Segment:           "SYN-SEG-SZX-ONWARD",
	}
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var registerErr error
		result, registerErr = register.Register(txCtx, command)
		return registerErr
	})
	if result.Outcome() != tfapplication.HandoverRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("登记下一次交接：outcome=%s debt=%q", result.Outcome(), result.SegmentContinuationReference())
	}
	return tenant.String() + "/" + object + "/" + onwardScope + "/" + onwardVersion
}

// registerHandoverIntoMovementSegment 经生产登记编排（真交接册、真段册、真 outbox 交出、真结束参与）登一次`已交接`：在控那一件
// 在出发前一小时交进出发声明的那一段，段由这一次交接立起。交回交接信封的事件 ID。
func registerHandoverIntoMovementSegment(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()
	return registerHandoverOfObjectIntoMovementSegment(t, fixture, movementParcelOnBoard)
}

// registerHandoverOfObjectIntoMovementSegment 同 registerHandoverIntoMovementSegment，交进段的是 object。
func registerHandoverOfObjectIntoMovementSegment(t *testing.T, fixture *synVerticalFixture, object string) string {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	register := newHandoverRegistration(t, fixture.db, store)
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	command := tfapplication.RegisterTransportHandoverCommand{
		TenantID:          tenant,
		Object:            object,
		Scope:             backfillScope,
		ReleasedBy:        "SYN-NODE-SHA-HUB",
		ReceivedBy:        "SYN-OWN-FLEET",
		Verdict:           tfdomain.ObjectHandedOver,
		ReleasingEvidence: "SYN-EVIDENCE-RELEASE-DEP-01",
		ReceivingEvidence: "SYN-EVIDENCE-RECEIVE-DEP-01",
		Rule:              "SYN-HANDOVER-RULE/v1",
		Version:           backfillVersion,
		JudgedAt:          movementHandedOverAt,
		Segment:           movementSegment,
	}
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var registerErr error
		result, registerErr = register.Register(txCtx, command)
		return registerErr
	})
	if result.Outcome() != tfapplication.HandoverRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("登记交接：outcome=%s debt=%q", result.Outcome(), result.SegmentContinuationReference())
	}
	return tenant.String() + "/" + object + "/" + backfillScope + "/" + backfillVersion
}

// registerPickupIntoMovementSegment 经生产登记编排（真揽收登记册、真段册、真版本签发、真 outbox 交出）登一次场外揽收：在控那一件
// 在出发前一小时被揽走，段由这一次收寄立起。交回对象级揽收信封的事件 ID。
func registerPickupIntoMovementSegment(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	pickups, err := tfpostgres.NewOffsitePickupRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("揽收登记库：%v", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(fixture.db)
	if err != nil {
		t.Fatalf("段登记库：%v", err)
	}
	judgments, err := tfpostgres.NewActualCarrierJudgments(fixture.db)
	if err != nil {
		t.Fatalf("实际承运商判断库：%v", err)
	}
	versions, err := tfpostgres.NewResultVersions(fixture.db)
	if err != nil {
		t.Fatalf("版本签发：%v", err)
	}
	handoff, err := tfpostgres.NewOutboxOffsitePickupRegistrationHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("揽收登记交出：%v", err)
	}
	register := tfapplication.NewRegisterOffsitePickupHandler(tfapplication.RegisterOffsitePickupDeps{
		Pickups: pickups, Segments: segments, Judgments: judgments, Versions: versions, Downstream: handoff, Clock: systemClock{},
	})
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	command := tfapplication.RegisterOffsitePickupCommand{
		TenantID:   tenant,
		Object:     movementParcelOnBoard,
		Task:       "SYN-PICKUP-TASK-DEP-01",
		Attempt:    backfillPickupAttempt,
		Place:      "SYN-PLACE-DEP-01",
		Control:    "SYN-CONTROL-DEP-01",
		ExecutedBy: "SYN-COURIER-DEP-01",
		OccurredAt: movementHandedOverAt,
		Segment:    movementSegment,
	}
	var result tfapplication.RegisterOffsitePickupResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var registerErr error
		result, registerErr = register.Register(txCtx, command)
		return registerErr
	})
	if result.Outcome() != tfapplication.PickupRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("登记揽收：outcome=%s debt=%q", result.Outcome(), result.SegmentContinuationReference())
	}
	return tenant.String() + "/" + movementParcelOnBoard + "/" + backfillPickupAttempt + "/offsite-pickup-registration"
}

func assertBackfilledDeparture(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	entries := projectionEntries(t, fixture, movementParcelOnBoard)
	if len(entries) != 2 {
		t.Fatalf("%s 条目 %d 个, want 交接与出发各一个", movementParcelOnBoard, len(entries))
	}
	var departed int
	for _, entry := range entries {
		if entry.Fact().Fact().String() != "movement-fact/"+movementParcelOnBoard+"/"+backfillFact {
			continue
		}
		departed++
		milestone, classified := entry.Milestone()
		if entry.Fact().Kind().String() != "movement-departure" || !classified || milestone.String() != "DEPARTED" {
			t.Fatalf("补出的出发 = %s 归类 %q（%v），want movement-departure 归 DEPARTED", entry.Fact().Kind(), milestone, classified)
		}
		if !entry.Fact().OccurredAt().Equal(movementDepartedAt) {
			t.Fatalf("补出的出发发生时间 = %s, want 事实的业务发生时间", entry.Fact().OccurredAt())
		}
	}
	if departed != 1 {
		t.Fatalf("出发条目 %d 个, want 1", departed)
	}
}

func projectionEntries(t *testing.T, fixture *synVerticalFixture, parcel string) []vedomain.MilestoneClassification {
	t.Helper()
	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(),
		mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String()), mustVE(t, vedomain.NewTrackedParcelReference, parcel))
	if err != nil {
		t.Fatalf("%s 当前投影：%v", parcel, err)
	}
	if !found {
		return nil
	}
	return projection.Entries()
}

func acceptedFactVersions(t *testing.T, fixture *synVerticalFixture, factRef string) int {
	t.Helper()
	return fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.accepted_fact
		WHERE tenant_id = '`+fixture.identity.TenantID().String()+`' AND fact_ref = '`+factRef+`'`)
}

// redeliverMovementFact 以新 ID 把同一条移动事实交给生产的移动事实那一路：段此刻已在册，正常路径照常派生，答已有结果。
func redeliverMovementFact(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	derive, inboxStore := redeliveryParts(t, fixture)
	consumer, err := deriveMovementFactConsumer(fixture.db, inboxStore, derive)
	if err != nil {
		t.Fatalf("装配移动事实那一路：%v", err)
	}
	tenant := fixture.identity.TenantID().String()
	if err := consumer.Consume(t.Context(), redeliveryEnvelope(tenant, "redeliver-movement-fact",
		veinbox.MovementFactRecordedEventType, backfillFact,
		`{"tenantId":"`+tenant+`","fact":"`+backfillFact+`","version":"v1"}`)); err != nil {
		t.Fatalf("移动事实重投：%v", err)
	}
}

// redeliverHandoverToBackfill 以新 ID 把同一版交接交给补派生那一路。
func redeliverHandoverToBackfill(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	derive, inboxStore := redeliveryParts(t, fixture)
	consumer, err := backfillMovementFactsOnHandoverConsumer(fixture.db, inboxStore, derive)
	if err != nil {
		t.Fatalf("装配补派生那一路：%v", err)
	}
	tenant := fixture.identity.TenantID().String()
	if err := consumer.Consume(t.Context(), redeliveryEnvelope(tenant, "redeliver-handover",
		veinbox.TransportHandoverRegisteredEventType, movementParcelOnBoard,
		`{"tenantId":"`+tenant+`","object":"`+movementParcelOnBoard+`","scope":"`+backfillScope+
			`","version":"`+backfillVersion+`"}`)); err != nil {
		t.Fatalf("交接重投：%v", err)
	}
}

func redeliveryParts(t *testing.T, fixture *synVerticalFixture) (*tenantBoundProjectionDerive, *inbox.Store) {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	derive, err := newTenantBoundProjectionDerive(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造派生：%v", err)
	}
	inboxStore, err := inbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	return derive, inboxStore
}

func redeliveryEnvelope(tenant, id string, eventType eventing.EventType, subject, payload string) eventing.Envelope {
	at := beatInstant()
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(id),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         eventType,
		Version:      1,
		Scope:        tenant,
		Subject:      subject,
		PartitionKey: tenant + "/" + subject,
		OccurredAt:   at,
		RecordedAt:   at,
		ContentType:  eventing.JSONContentType,
		Payload:      []byte(payload),
	}
}
