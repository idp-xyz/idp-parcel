package main

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 本文件对真实 PostgreSQL 证派送发起随交接信封在派发进程里逐对象发起（ADR-0216）。交接一律经生产登记编排登记（真段、
// 真交接、真 outbox 交出），任务落真任务登记册。只有三条派送要求缝分两种接法：生产路由那一条用真缝——空库里三个所有者都
// 答「没有」；其余几条经 deliveryDispatchOnHandoverConsumerOver 换上替身，好让「给了」「没有」「读不动」三种回答各自出现。
// 夹具全部合成，只记 `S`。

const (
	handoverDispatchConsumerName = "transport-fulfillment/trigger-delivery-dispatch-from-transport-handover"
	handoverDispatchTenant       = "SYN-TENANT-01"
	handoverDispatchScope        = "SYN-HANDOVER-SCOPE-60"
	handoverDispatchSegment      = "SYN-DELIVERY-SEGMENT-60"
)

// handoverDispatchAt 是替身接法下派发这一拍的时钟读数，与交接判断时刻错开：任务的成立时间要取它。
var handoverDispatchAt = time.Date(2026, 10, 21, 4, 0, 0, 0, time.UTC)

type handoverDispatchClock struct{}

func (handoverDispatchClock) Now() time.Time { return handoverDispatchAt }

// requirementStub 一身扮三条派送要求缝，每条都能答给了、没有、读不动。
type requirementStub struct {
	missingWindow bool
	placeErr      error
	calls         int
}

func (stub *requirementStub) LoadDeliveryPlace(
	_ context.Context, _ tfdomain.TenantID, object tfdomain.CarriedObjectReference,
) (string, tfports.RequirementResolution, error) {
	stub.calls++
	if stub.placeErr != nil {
		return "", tfports.RequirementResolutionInvalid, stub.placeErr
	}
	return "SYN-PLACE/" + object.String(), tfports.RequirementResolved, nil
}

func (stub *requirementStub) LoadDeliveryWindow(
	_ context.Context, _ tfdomain.TenantID, _ tfdomain.PlannedSegmentReference, _ bool,
) (time.Time, time.Time, tfports.RequirementResolution, error) {
	stub.calls++
	if stub.missingWindow {
		return time.Time{}, time.Time{}, tfports.RequirementMissing, nil
	}
	return handoverDispatchAt, handoverDispatchAt.Add(4 * time.Hour), tfports.RequirementResolved, nil
}

func (stub *requirementStub) LoadDeliveryConditions(
	_ context.Context, _ tfdomain.TenantID, _ tfdomain.CarriedObjectReference,
) (string, tfports.RequirementResolution, error) {
	stub.calls++
	return "SYN-DELIVERY-CONDITIONS/v1", tfports.RequirementResolved, nil
}

type handoverDispatchFixture struct {
	db          *bentopg.DB
	outboxStore *outbox.Store
	inboxStore  *inbox.Store
	register    *tfapplication.RegisterTransportHandoverHandler
	owners      *requirementStub
	lane        dispatch.Consumer
}

// newHandoverDispatchFixture 接一条与生产同形的派送发起路（同一个 Over、同一份哨兵名单），只把三条要求缝换成替身。
func newHandoverDispatchFixture(t *testing.T) *handoverDispatchFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	fixture := &handoverDispatchFixture{db: db, owners: &requirementStub{}}
	if fixture.outboxStore, err = outbox.NewStore(db); err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	if fixture.inboxStore, err = inbox.NewStore(db); err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	fixture.register = newHandoverRegistration(t, db, fixture.outboxStore)
	consumer, err := deliveryDispatchOnHandoverConsumerOver(
		db, fixture.inboxStore, handoverDispatchClock{}, fixture.owners, fixture.owners, fixture.owners)
	if err != nil {
		t.Fatalf("装配派送发起这一路：%v", err)
	}
	if fixture.lane, err = dispatch.WithUndecidedSentinels(consumer, deliveryDispatchOnHandoverUndecidedSentinels...); err != nil {
		t.Fatalf("包未决哨兵：%v", err)
	}
	return fixture
}

// newHandoverRegistration 照生产接法装交接登记编排：真交接册、真段册、真 outbox 交出、真结束参与。
func newHandoverRegistration(t *testing.T, db *bentopg.DB, store *outbox.Store) *tfapplication.RegisterTransportHandoverHandler {
	t.Helper()
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		t.Fatalf("交接登记库：%v", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatalf("段登记库：%v", err)
	}
	judgments, err := tfpostgres.NewActualCarrierJudgments(db)
	if err != nil {
		t.Fatalf("实际承运商判断库：%v", err)
	}
	deliveries, err := tfpostgres.NewEffectiveDeliveries(db)
	if err != nil {
		t.Fatalf("有效交付库：%v", err)
	}
	handoff, err := tfpostgres.NewOutboxTransportHandoverRegistrationHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("交接交出：%v", err)
	}
	return tfapplication.NewRegisterTransportHandoverHandler(tfapplication.RegisterTransportHandoverDeps{
		Handovers:  handovers,
		Segments:   segments,
		Judgments:  judgments,
		Downstream: handoff,
		Clock:      systemClock{},
		ParticipationEnds: tfapplication.NewEndFulfillmentParticipationHandler(tfapplication.EndFulfillmentParticipationDeps{
			Segments: segments, Judgments: judgments, Handovers: handovers, Deliveries: deliveries, Clock: systemClock{},
		}),
	})
}

// handoverInto 是一次`已交接`的登记命令：segment 空即不进段，action 空即不声明服务动作。版本随对象与序号，好让同一对象
// 的第二次交接是另一版。
func handoverInto(t *testing.T, object string, ordinal int, segment, action string) tfapplication.RegisterTransportHandoverCommand {
	t.Helper()
	return tfapplication.RegisterTransportHandoverCommand{
		TenantID:             mustTF(t, tfdomain.NewTenantID, handoverDispatchTenant),
		Object:               object,
		Scope:                handoverDispatchScope,
		ReleasedBy:           "SYN-NODE-SIN-HUB",
		ReceivedBy:           "SYN-PARTY-LASTMILE-01",
		Verdict:              tfdomain.ObjectHandedOver,
		ReleasingEvidence:    "SYN-EVIDENCE-RELEASE-60",
		ReceivingEvidence:    "SYN-EVIDENCE-RECEIVE-60",
		Rule:                 "SYN-HANDOVER-RULE/v1",
		Version:              handoverDispatchVersion(object, ordinal),
		JudgedAt:             handoverDispatchAt.Add(-2*time.Hour + time.Duration(ordinal)*time.Minute),
		Segment:              segment,
		SegmentServiceAction: action,
	}
}

func handoverDispatchVersion(object string, ordinal int) string {
	return "SYN-HANDOVER-RESULT/" + object + "/v" + strconv.Itoa(ordinal)
}

func (fixture *handoverDispatchFixture) registerHandover(t *testing.T, command tfapplication.RegisterTransportHandoverCommand) {
	t.Helper()
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, fixture.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.register.Register(txCtx, command)
		return err
	})
	if result.Outcome() != tfapplication.HandoverRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("登记交接 %s：outcome=%s debt=%q", command.Version, result.Outcome(), result.SegmentContinuationReference())
	}
}

// handoverDispatchEventID 与生产交出的信封 ID 同一拼法：生产路由那一条用例按它去库里找真交出的那一封，两边对不上就红。
func handoverDispatchEventID(object string, ordinal int) string {
	return handoverDispatchTenant + "/" + object + "/" + handoverDispatchScope + "/" + handoverDispatchVersion(object, ordinal)
}

// deliver 把那一版交接的信封交给派送发起这一路，模拟派发进程投递（含重投）。
func (fixture *handoverDispatchFixture) deliver(t *testing.T, object string, ordinal int) error {
	t.Helper()
	at := beatInstant()
	return fixture.lane.Consume(t.Context(), eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(handoverDispatchEventID(object, ordinal)),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         tfinbox.TransportHandoverRegisteredEventType,
		Version:      1,
		Scope:        handoverDispatchTenant,
		Subject:      object,
		PartitionKey: handoverDispatchTenant + "/" + object + "/transport-handover-registration",
		OccurredAt:   at,
		RecordedAt:   at,
		ContentType:  eventing.JSONContentType,
		Payload: []byte(`{"tenantId":"` + handoverDispatchTenant + `","object":"` + object + `","scope":"` +
			handoverDispatchScope + `","version":"` + handoverDispatchVersion(object, ordinal) + `"}`),
	})
}

func dispatchTaskRows(t *testing.T, db *bentopg.DB) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.dispatch_task WHERE tenant_id = $1`, handoverDispatchTenant,
	).Scan(&count); err != nil {
		t.Fatalf("数派送任务：%v", err)
	}
	return count
}

// handoverDispatchTaskReference 照执行器的 v2 铸法拼出（段，对象，入场依据）那一拍的任务引用：段、对象、入场依据各写
// 「/字节长度:原串」。这里另拼一份而不调执行器，是要让占用者恰好立在执行器会铸的那一串上。
func handoverDispatchTaskReference(object string, ordinal int) string {
	reference := "DELIVERY-DISPATCH/v2"
	for _, part := range []string{handoverDispatchSegment, object, "TRANSPORT-HANDOVER/" + handoverDispatchVersion(object, ordinal)} {
		reference += "/" + strconv.Itoa(len(part)) + ":" + part
	}
	return reference
}

// Covers: AT-TF-195、AT-TF-199（ADR-0216 决定一、三）——生产路由表上交接信封扇出两路：VE 投影与 TF 派送
// 发起各记一行 inbox，信封定稿 PUBLISHED。三条要求缝是生产真缝，空库里所有者都答「没有」，派送发起答`要求缺失`入账：
// 不重投、不形成任务。两路认的事件类型与生产交出的是同一个串——信封是生产交接口真交出的那一封。
func TestAHandoverIntoADeliverySegmentReachesTrackingAndDispatchThroughTheProductionRoute(t *testing.T) {
	if veinbox.TransportHandoverRegisteredEventType != tfinbox.TransportHandoverRegisteredEventType {
		t.Fatalf("VE 认 %q、TF 认 %q——同一封信两路认的不是同一个类型",
			veinbox.TransportHandoverRegisteredEventType, tfinbox.TransportHandoverRegisteredEventType)
	}
	beat, db, store := wiredBeat(t)
	register := newHandoverRegistration(t, db, store)
	var registered tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		registered, err = register.Register(txCtx,
			handoverInto(t, "SYN-PARCEL-60-A", 1, handoverDispatchSegment, tfdomain.SegmentServesFinalDelivery.String()))
		return err
	})
	if registered.Outcome() != tfapplication.HandoverRegistered {
		t.Fatalf("登记交接：%s", registered.Outcome())
	}

	eventID := handoverDispatchEventID("SYN-PARCEL-60-A", 1)
	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if status := outboxStatus(t, db, eventID); status != "PUBLISHED" {
		t.Fatalf("交接信封 status = %s，失败码 = %q, want PUBLISHED——要求缺失该入账而不是重投",
			status, recordedFailureCode(t, db, eventID))
	}
	for _, consumer := range []string{deriveHandoverConsumerName, handoverDispatchConsumerName} {
		if n := inboxRowsOf(t, db, consumer, eventID); n != 1 {
			t.Fatalf("%s 名下 inbox 行 = %d, want 1——FanOut 少了这一路", consumer, n)
		}
	}
	if n := dispatchTaskRows(t, db); n != 0 {
		t.Fatalf("所有者都答没有却形成了 %d 项任务——填了默认值", n)
	}
}

// Covers: AT-TF-191、AT-TF-192、AT-TF-200（ADR-0216 决定一至四）——要求缝都给：交接信封一到形成一项任务，对象恰为该对象，成立
// 时间取派发这一拍的时钟；同一封重投不重建；之后经补拍口对同一（段，对象）再打一拍，执行器答`已在册`并交回同一项。
func TestTheHandoverLaneFormsOneTaskAndNeitherRedeliveryNorARetickRebuildsIt(t *testing.T) {
	fixture := newHandoverDispatchFixture(t)
	fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-60-B", 1, handoverDispatchSegment,
		tfdomain.SegmentServesFinalDelivery.String()))

	for range 2 {
		if err := fixture.deliver(t, "SYN-PARCEL-60-B", 1); err != nil {
			t.Fatalf("投递交接信封：%v", err)
		}
	}
	if n := dispatchTaskRows(t, fixture.db); n != 1 {
		t.Fatalf("任务 = %d 项, want 1——重投不得重建", n)
	}
	if fixture.owners.calls != 3 {
		t.Fatalf("所有者被问 %d 次, want 3——重投在消费门就停下，不再问所有者", fixture.owners.calls)
	}
	tasks, err := tfpostgres.NewDispatchTasks(fixture.db)
	if err != nil {
		t.Fatalf("任务登记册：%v", err)
	}
	reference := handoverDispatchTaskReference("SYN-PARCEL-60-B", 1)
	record, found, err := tasks.FindByKey(t.Context(), tfports.DispatchTaskKey{
		TenantID: mustTF(t, tfdomain.NewTenantID, handoverDispatchTenant),
		Task:     mustTF(t, tfdomain.NewDispatchTaskReference, reference),
	})
	if err != nil || !found {
		t.Fatalf("按执行器铸法读回任务 %s：found=%v err=%v", reference, found, err)
	}
	if objects := record.Task.Objects(); record.Task.Kind() != tfdomain.DeliveryDispatch ||
		len(objects) != 1 || objects[0].String() != "SYN-PARCEL-60-B" {
		t.Fatalf("任务 = %s %v, want DELIVERY 且对象只有 SYN-PARCEL-60-B", record.Task.Kind(), objects)
	}
	if !record.Task.OpenedAt().Equal(handoverDispatchAt) {
		t.Fatalf("任务成立时间 = %s, want 派发这一拍的时钟 %s", record.Task.OpenedAt(), handoverDispatchAt)
	}

	retick := retickDeliveryDispatch(t, fixture)
	result, err := retick.Trigger(t.Context(), tfapplication.TriggerDeliveryDispatchCommand{
		TenantID:   mustTF(t, tfdomain.NewTenantID, handoverDispatchTenant),
		Segment:    handoverDispatchSegment,
		Object:     "SYN-PARCEL-60-B",
		OccurredAt: handoverDispatchAt.Add(time.Hour),
	})
	if err != nil || result.Outcome() != tfapplication.DeliveryDispatchTaskAlreadyFormed {
		t.Fatalf("补拍 = %s err = %v, want DISPATCH_TASK_ALREADY_FORMED", result.Outcome(), err)
	}
	if again, _ := result.Record(); again.Key.Task.String() != reference || dispatchTaskRows(t, fixture.db) != 1 {
		t.Fatalf("补拍交回 %s, want 同一项 %s 且库仍 1 项", again.Key.Task, reference)
	}
}

// retickDeliveryDispatch 是补拍口背后的那个执行器，接法与本路同一组登记册与所有者——补拍与进程内那一拍互相认，靠的就是这个。
func retickDeliveryDispatch(t *testing.T, fixture *handoverDispatchFixture) *tfapplication.TriggerDeliveryDispatchHandler {
	t.Helper()
	segments, err := tfpostgres.NewFulfillmentSegments(fixture.db)
	if err != nil {
		t.Fatalf("段登记库：%v", err)
	}
	tasks, err := tfpostgres.NewDispatchTasks(fixture.db)
	if err != nil {
		t.Fatalf("任务登记库：%v", err)
	}
	return tfapplication.NewTriggerDeliveryDispatchHandler(tfapplication.TriggerDeliveryDispatchDeps{
		Segments: segments, Tasks: tasks, Places: fixture.owners, Windows: fixture.owners, Conditions: fixture.owners,
		Dispatch: tfapplication.NewOpenDispatchTaskHandler(tfapplication.OpenDispatchTaskDeps{Tasks: tasks, Clock: systemClock{}}),
	})
}

// Covers: AT-TF-193、AT-TF-194、AT-TF-195、AT-TF-197（ADR-0216 决定三）——非派送段、已离场、引用被占、要求缺失、拒收：
// 都入账（inbox 一行、交回 nil），都不形成任务。
func TestHandoversThatAreNotATriggerSettleWithoutATask(t *testing.T) {
	t.Run("段不是派送段", func(t *testing.T) {
		fixture := newHandoverDispatchFixture(t)
		fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-60-C", 1, handoverDispatchSegment,
			tfdomain.SegmentServesLinehaul.String()))
		fixture.assertSettledWithTasks(t, "SYN-PARCEL-60-C", 1, 0)
	})
	t.Run("已离场：信封到时对象已被下一次交接带走", func(t *testing.T) {
		fixture := newHandoverDispatchFixture(t)
		fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-60-D", 1, handoverDispatchSegment,
			tfdomain.SegmentServesFinalDelivery.String()))
		fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-60-D", 2, "", ""))
		fixture.assertSettledWithTasks(t, "SYN-PARCEL-60-D", 1, 0)
	})
	t.Run("引用被占：执行器要铸的那一串上立着别的任务", func(t *testing.T) {
		fixture := newHandoverDispatchFixture(t)
		fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-60-E", 1, handoverDispatchSegment,
			tfdomain.SegmentServesFinalDelivery.String()))
		occupy(t, fixture, handoverDispatchTaskReference("SYN-PARCEL-60-E", 1))
		fixture.assertSettledWithTasks(t, "SYN-PARCEL-60-E", 1, 1)
	})
	t.Run("要求缺失", func(t *testing.T) {
		fixture := newHandoverDispatchFixture(t)
		fixture.owners.missingWindow = true
		fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-60-F", 1, handoverDispatchSegment,
			tfdomain.SegmentServesFinalDelivery.String()))
		fixture.assertSettledWithTasks(t, "SYN-PARCEL-60-F", 1, 0)
	})
	t.Run("拒收", func(t *testing.T) {
		fixture := newHandoverDispatchFixture(t)
		refused := handoverInto(t, "SYN-PARCEL-60-G", 1, handoverDispatchSegment, tfdomain.SegmentServesFinalDelivery.String())
		refused.Verdict = tfdomain.HandoverRefused
		refused.Basis = "SYN-REFUSAL-BASIS-60"
		var result tfapplication.RegisterTransportHandoverResult
		mustWithinTX(t, fixture.db.Transactor(), t.Context(), func(txCtx context.Context) error {
			var err error
			result, err = fixture.register.Register(txCtx, refused)
			return err
		})
		if result.Outcome() != tfapplication.HandoverRegistered {
			t.Fatalf("登记拒收：%s", result.Outcome())
		}
		fixture.assertSettledWithTasks(t, "SYN-PARCEL-60-G", 1, 0)
		if fixture.owners.calls != 0 {
			t.Fatalf("拒收的交接问了所有者 %d 次, want 0", fixture.owners.calls)
		}
	})
}

func (fixture *handoverDispatchFixture) assertSettledWithTasks(t *testing.T, object string, ordinal, tasks int) {
	t.Helper()
	if err := fixture.deliver(t, object, ordinal); err != nil {
		t.Fatalf("投递交接信封 = %v, want 入账", err)
	}
	if n := inboxRowsOf(t, fixture.db, handoverDispatchConsumerName, handoverDispatchEventID(object, ordinal)); n != 1 {
		t.Fatalf("inbox 行 = %d, want 1——该入账", n)
	}
	if n := dispatchTaskRows(t, fixture.db); n != tasks {
		t.Fatalf("任务 = %d 项, want %d", n, tasks)
	}
}

// occupy 在执行器要铸的那一串上先立一项别的任务（揽收、对象是别人），扮演存量里指名了这串引用的手工任务。经任务口编排
// 落库，不走手工建任务口：那一口自票 tf-face-algebra/09 起拒指名这一起头。
func occupy(t *testing.T, fixture *handoverDispatchFixture, reference string) {
	t.Helper()
	tasks, err := tfpostgres.NewDispatchTasks(fixture.db)
	if err != nil {
		t.Fatalf("任务登记库：%v", err)
	}
	opener := tfapplication.NewOpenDispatchTaskHandler(tfapplication.OpenDispatchTaskDeps{Tasks: tasks, Clock: systemClock{}})
	var opened tfapplication.OpenDispatchTaskResult
	mustWithinTX(t, fixture.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		opened, err = opener.Open(txCtx, tfapplication.OpenDispatchTaskCommand{
			TenantID:   mustTF(t, tfdomain.NewTenantID, handoverDispatchTenant),
			Task:       reference,
			Kind:       tfdomain.PickupDispatch,
			Objects:    []string{"SYN-PARCEL-60-OTHER"},
			Place:      "SYN-PLACE/OTHER",
			WindowFrom: handoverDispatchAt,
			WindowTo:   handoverDispatchAt.Add(time.Hour),
			Conditions: "SYN-PICKUP-CONDITIONS/v1",
			OpenedAt:   handoverDispatchAt.Add(-time.Hour),
		})
		return err
	})
	if opened.Outcome() != tfapplication.DispatchTaskOpened {
		t.Fatalf("立占用者：%s", opened.Outcome())
	}
}

// Covers: AT-TF-196（ADR-0216 决定三）——要求缝读不动：错误带派发的未决哨兵，inbox 不落行、整笔回滚；缝恢复后重投形成任务。
func TestAnUnreadableRequirementSeamIsRetriedUntilItReads(t *testing.T) {
	fixture := newHandoverDispatchFixture(t)
	fixture.owners.placeErr = errors.New("parcel shipment unavailable")
	fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-60-H", 1, handoverDispatchSegment,
		tfdomain.SegmentServesFinalDelivery.String()))

	err := fixture.deliver(t, "SYN-PARCEL-60-H", 1)
	if !errors.Is(err, dispatch.ErrConsumerUndecided) || !errors.Is(err, tfinbox.ErrDeliveryDispatchOnHandoverUndecided) {
		t.Fatalf("缝读不动 = %v, want 译成派发的未决（ErrConsumerUndecided）并留着本路的哨兵", err)
	}
	if n := inboxRowsOf(t, fixture.db, handoverDispatchConsumerName, handoverDispatchEventID("SYN-PARCEL-60-H", 1)); n != 0 {
		t.Fatalf("未决却入了账：inbox 行 = %d", n)
	}

	fixture.owners.placeErr = nil
	if err := fixture.deliver(t, "SYN-PARCEL-60-H", 1); err != nil {
		t.Fatalf("缝恢复后重投：%v", err)
	}
	if n := dispatchTaskRows(t, fixture.db); n != 1 {
		t.Fatalf("缝恢复后任务 = %d 项, want 1", n)
	}
}
