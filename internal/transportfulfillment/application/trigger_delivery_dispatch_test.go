package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	deliveryWindowFrom  = time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	deliveryWindowTo    = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	deliveryTriggeredAt = time.Date(2026, 8, 13, 15, 0, 0, 0, time.UTC)
)

// 三条派送要求端口的替身。每条都能扮演所有者的三种回答：给了、说没有、读不回来——三种在执行器里
// 各有各的续办，替身把它们分开才测得出执行器有没有把「没有」当成「读不到」。
type deliveryPlaceStub struct {
	place      string
	resolution ports.RequirementResolution
	err        error
	calls      int
}

func (stub *deliveryPlaceStub) LoadDeliveryPlace(
	_ context.Context, _ domain.TenantID, object domain.CarriedObjectReference,
) (string, ports.RequirementResolution, error) {
	stub.calls++
	if stub.err != nil {
		return "", ports.RequirementResolutionInvalid, stub.err
	}
	if stub.resolution == ports.RequirementMissing {
		return "", ports.RequirementMissing, nil
	}
	return stub.place + object.String(), ports.RequirementResolved, nil
}

type deliveryWindowStub struct {
	from, to   time.Time
	resolution ports.RequirementResolution
	err        error
	calls      int
	// planned / present 记下执行器交出来的计划履约段引用与在场标记：执行器该原样转交参与关系上的那一格，不自己判。
	planned domain.PlannedSegmentReference
	present bool
}

func (stub *deliveryWindowStub) LoadDeliveryWindow(
	_ context.Context, _ domain.TenantID, _ domain.CarriedObjectReference,
) (time.Time, time.Time, ports.RequirementResolution, error) {
	return stub.answer()
}

func (stub *deliveryWindowStub) LoadDeliveryWindowByPlannedSegment(
	_ context.Context, _ domain.TenantID, planned domain.PlannedSegmentReference, present bool,
) (time.Time, time.Time, ports.RequirementResolution, error) {
	stub.planned, stub.present = planned, present
	return stub.answer()
}

func (stub *deliveryWindowStub) answer() (time.Time, time.Time, ports.RequirementResolution, error) {
	stub.calls++
	if stub.err != nil {
		return time.Time{}, time.Time{}, ports.RequirementResolutionInvalid, stub.err
	}
	if stub.resolution == ports.RequirementMissing {
		return time.Time{}, time.Time{}, ports.RequirementMissing, nil
	}
	return stub.from, stub.to, ports.RequirementResolved, nil
}

type deliveryConditionStub struct {
	conditions string
	resolution ports.RequirementResolution
	err        error
	calls      int
}

func (stub *deliveryConditionStub) LoadDeliveryConditions(
	_ context.Context, _ domain.TenantID, _ domain.CarriedObjectReference,
) (string, ports.RequirementResolution, error) {
	stub.calls++
	if stub.err != nil {
		return "", ports.RequirementResolutionInvalid, stub.err
	}
	if stub.resolution == ports.RequirementMissing {
		return "", ports.RequirementMissing, nil
	}
	return stub.conditions, ports.RequirementResolved, nil
}

type dispatchTriggerFixture struct {
	handovers  *handoverSegmentFixture
	tasks      *dispatchTaskRegistryDouble
	places     *deliveryPlaceStub
	windows    *deliveryWindowStub
	conditions *deliveryConditionStub
	deps       application.TriggerDeliveryDispatchDeps
}

func newDispatchTriggerFixture(t *testing.T) *dispatchTriggerFixture {
	t.Helper()
	fixture := &dispatchTriggerFixture{
		handovers:  newHandoverSegmentFixture(t),
		tasks:      newDispatchTaskRegistry(),
		places:     &deliveryPlaceStub{place: "delivery-place/"},
		windows:    &deliveryWindowStub{from: deliveryWindowFrom, to: deliveryWindowTo},
		conditions: &deliveryConditionStub{conditions: "delivery-conditions/contract-1/v3"},
	}
	fixture.deps = application.TriggerDeliveryDispatchDeps{
		Segments:   fixture.handovers.segments,
		Places:     fixture.places,
		Windows:    fixture.windows,
		Conditions: fixture.conditions,
		Dispatch:   newDispatchTaskHandler(fixture.tasks),
	}
	return fixture
}

func (fixture *dispatchTriggerFixture) handler() *application.TriggerDeliveryDispatchHandler {
	return application.NewTriggerDeliveryDispatchHandler(fixture.deps)
}

// enterByHandover 让一个对象凭`已交接`进入 segment-1；首个对象成立段时随带服务动作声明。
func (fixture *dispatchTriggerFixture) enterByHandover(t *testing.T, object, serviceAction string, judgedAt time.Time) {
	t.Helper()
	command := registerHandoverCommand(t)
	command.Object = object
	command.Version = "handover-result/" + object + "/v1"
	command.JudgedAt = judgedAt
	command.Segment = "segment-1"
	command.SegmentServiceAction = serviceAction
	result, err := fixture.handovers.handler.Register(t.Context(), command)
	if err != nil || result.Outcome() != application.HandoverRegistered {
		t.Fatalf("%s 进段：%v %q", object, err, result.Outcome())
	}
	if result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || result.SegmentContinuationReference() != "" {
		t.Fatalf("%s 进段被拒或欠账：%s %q", object, result.SegmentEntryRefusal(), result.SegmentContinuationReference())
	}
}

// enterByHandoverWithPlan 同 enterByHandover，但对象带着登记方关联的计划履约段引用进段（不透明串，TF 只搬运不解读）。
func (fixture *dispatchTriggerFixture) enterByHandoverWithPlan(t *testing.T, object, serviceAction, plannedSegment string, judgedAt time.Time) {
	t.Helper()
	command := registerHandoverCommand(t)
	command.Object = object
	command.Version = "handover-result/" + object + "/v1"
	command.JudgedAt = judgedAt
	command.Segment = "segment-1"
	command.SegmentServiceAction = serviceAction
	command.PlannedSegment = plannedSegment
	result, err := fixture.handovers.handler.Register(t.Context(), command)
	if err != nil || result.Outcome() != application.HandoverRegistered {
		t.Fatalf("%s 带计划段进段：%v %q", object, err, result.Outcome())
	}
	if result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || result.SegmentContinuationReference() != "" {
		t.Fatalf("%s 进段被拒或欠账：%s %q", object, result.SegmentEntryRefusal(), result.SegmentContinuationReference())
	}
}

// Covers: 票 tf-segment-lifecycle-closure/13 做法第 4 步 / ADR-0131 决定一 — 时间窗那一缝按参与关系上登记方关联的计划
// 履约段引用问，不按对象问：执行器把引用与在场标记原样转交（拼写不拆不解读），缺席也交出去由适配器答（决定三），
// 执行器不自己判缺席。
func TestTheWindowSeamIsAskedByThePlannedSegmentReferenceOnTheParticipation(t *testing.T) {
	t.Run("参与关系带计划段：引用原样交出", func(t *testing.T) {
		fixture := newDispatchTriggerFixture(t)
		fixture.enterByHandoverWithPlan(t, "parcel-1", "FINAL_DELIVERY", "RPV-000000000007#2", handoverJudgedTime)

		result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
		if err != nil || result.Outcome() != application.DeliveryDispatchTaskFormed {
			t.Fatalf("触发：%v %q", err, result.Outcome())
		}
		if fixture.windows.calls != 1 || !fixture.windows.present || fixture.windows.planned.String() != "RPV-000000000007#2" {
			t.Fatalf("时间窗缝收到的问法 = calls %d present %v planned %q，want 参与关系上那个引用原样到达",
				fixture.windows.calls, fixture.windows.present, fixture.windows.planned)
		}
	})

	t.Run("参与关系无计划段：仍问、在场标记为无，缺席由适配器答", func(t *testing.T) {
		fixture := newDispatchTriggerFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		fixture.windows.resolution = ports.RequirementMissing

		result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
		if err != nil || result.Outcome() != application.DeliveryDispatchRequirementMissing {
			t.Fatalf("触发：%v %q，want REQUIREMENT_MISSING（由适配器答的缺席）", err, result.Outcome())
		}
		if fixture.windows.calls != 1 || fixture.windows.present || fixture.windows.planned.String() != "" {
			t.Fatalf("时间窗缝收到的问法 = calls %d present %v planned %q，want 交出去且在场标记为无",
				fixture.windows.calls, fixture.windows.present, fixture.windows.planned)
		}
		if missing := result.Missing(); len(missing) != 1 || missing[0] != application.DeliveryWindowRequirement {
			t.Fatalf("缺件名单 = %v，want 只有时间窗一件", missing)
		}
	})
}

func triggerCommand(t *testing.T, object string) application.TriggerDeliveryDispatchCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.TriggerDeliveryDispatchCommand{TenantID: tenant, Segment: "segment-1", Object: object, OccurredAt: deliveryTriggeredAt}
}

// Covers: ADR-0114 决定二 — 触发事实是「对象凭`已交接`进入派送段」，输出是调既有 OpenDispatchTask；一拍一对象一任务，
// 任务的对象集是该对象自己（同段另一个对象各有目的地，合并规则是运营政策，不代拟）；七件里地点、时间窗、条件三件
// 按派送要求从三条端口取，落进任务的 Place 是引用；成立时间取这一拍的业务时间——CONTEXT「任务在下一拍形成」，进段是
// 触发事实不是成立时刻，写库时刻更不是。
func TestEnteringADeliverySegmentByHandoverOpensOneTaskForThatObject(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.enterByHandover(t, "parcel-2", "", handoverJudgedTime.Add(time.Hour))

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("触发：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("outcome = %q, want DISPATCH_TASK_FORMED（refusal=%s undecided=%s）", result.Outcome(), result.Refusal(), result.UndecidedReason())
	}
	record, recorded := result.Record()
	if !recorded {
		t.Fatal("任务建立了却没交回记录")
	}
	if record.Task.Kind() != domain.DeliveryDispatch {
		t.Fatalf("任务种类 = %q, want DELIVERY", record.Task.Kind())
	}
	objects := record.Task.Objects()
	if len(objects) != 1 || objects[0].String() != "parcel-1" {
		t.Fatalf("任务对象集 = %v, want 只有 parcel-1——一拍一对象一任务，同段的 parcel-2 不并进来", objects)
	}
	if record.Task.Place().String() != "delivery-place/parcel-1" {
		t.Fatalf("任务地点 = %q, want parcel-shipment 交回的地点引用", record.Task.Place())
	}
	from, to := record.Task.Window()
	if !from.Equal(deliveryWindowFrom) || !to.Equal(deliveryWindowTo) {
		t.Fatalf("任务时间窗 = [%s, %s], want network-routing 交回的计划窗口", from, to)
	}
	if record.Task.Conditions().String() != "delivery-conditions/contract-1/v3" {
		t.Fatalf("任务条件 = %q, want party-commercial 交回的条件引用", record.Task.Conditions())
	}
	if !record.Task.OpenedAt().Equal(deliveryTriggeredAt) {
		t.Fatalf("任务成立时间 = %s, want 这一拍的业务时间 %s，不是进段时刻也不是写库时刻", record.Task.OpenedAt(), deliveryTriggeredAt)
	}

	second, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-2"))
	if err != nil || second.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("同段第二个对象的那一拍：%v %q", err, second.Outcome())
	}
	secondRecord, _ := second.Record()
	if secondRecord.Key.Task == record.Key.Task {
		t.Fatal("两个对象铸出了同一个任务引用——任务引用按（段，对象，入场依据）铸，对象不同引用必须不同")
	}
	if objects := secondRecord.Task.Objects(); len(objects) != 1 || objects[0].String() != "parcel-2" {
		t.Fatalf("第二个任务对象集 = %v, want 只有 parcel-2", objects)
	}
	if fixture.tasks.saves != 2 {
		t.Fatalf("任务登记册写了 %d 次, want 2", fixture.tasks.saves)
	}
}

// Covers: ADR-0114 决定二 — 「任务引用由执行器按（段，对象，入场依据）确定性铸出，重跑同一拍撞`已在册`而不重建」；
// CONTEXT「形成失败重跑本拍」的另一半：重跑成功过的那一拍不得长出第二个任务。
func TestRerunningTheSameBeatFindsTheTaskAlreadyOpen(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)

	first, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil || first.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("首拍：%v %q", err, first.Outcome())
	}
	// 重跑那一拍的业务时间可以不同（重跑本来就晚于首拍）；撞`已在册`看的是任务引用，不是时间。
	rerunCommand := triggerCommand(t, "parcel-1")
	rerunCommand.OccurredAt = deliveryTriggeredAt.Add(10 * time.Minute)
	rerun, err := fixture.handler().Trigger(t.Context(), rerunCommand)
	if err != nil {
		t.Fatalf("重跑：%v", err)
	}
	if rerun.Outcome() != application.DeliveryDispatchTaskAlreadyFormed {
		t.Fatalf("重跑 outcome = %q, want DISPATCH_TASK_ALREADY_FORMED", rerun.Outcome())
	}
	firstRecord, _ := first.Record()
	rerunRecord, recorded := rerun.Record()
	if !recorded || rerunRecord.Key.Task != firstRecord.Key.Task {
		t.Fatalf("重跑交回的任务 = %v, want 首拍铸出的同一个任务 %s", rerunRecord.Key.Task, firstRecord.Key.Task)
	}
	if fixture.tasks.saves != 1 {
		t.Fatalf("重跑又写了任务登记册：saves=%d, want 1", fixture.tasks.saves)
	}
}

// Covers: CONTEXT「段服务动作」——「未声明是一种答案而不是缺陷，这样的段照常成立与结束，只是不触发依赖服务动作的派生」；
// 「声明为末端派送的段即派送段」。未声明、节点间运输、场外揽收三格都不是派送段，一件派送要求都不去拉，一个任务都不开。
func TestOnlyADeclaredDeliverySegmentTriggers(t *testing.T) {
	for name, serviceAction := range map[string]string{
		"未声明":   "",
		"节点间运输": "LINEHAUL",
		"场外揽收":  "OFFSITE_PICKUP",
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDispatchTriggerFixture(t)
			fixture.enterByHandover(t, "parcel-1", serviceAction, handoverJudgedTime)

			result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("触发：%v", err)
			}
			if result.Outcome() != application.DeliveryDispatchNotTriggered {
				t.Fatalf("outcome = %q, want NOT_A_DELIVERY_TRIGGER", result.Outcome())
			}
			if result.Refusal() != application.DeliveryTriggerRefusedSegmentNotDelivery {
				t.Fatalf("refusal = %s, want SEGMENT_NOT_DELIVERY", result.Refusal())
			}
			if fixture.tasks.saves != 0 {
				t.Fatalf("不是派送段却开了任务：saves=%d", fixture.tasks.saves)
			}
			if fixture.places.calls+fixture.windows.calls+fixture.conditions.calls != 0 {
				t.Fatal("不是派送段却去拉派送要求——不依赖服务动作的段不触发任何派生")
			}
		})
	}
}

// Covers: CONTEXT「末端派送任务的内部触发只有一种事实：载运对象凭`已交接`的权威交接进入派送段」。凭有效收寄进入的对象
// 不是那一种事实——哪怕登记方把揽收成立的段声明成了末端派送（ADR-0114 越权风险点 2 的那一格）。
func TestEnteringByOffsitePickupIsNotTheTriggerFact(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	pickups := newPickupSegmentFixture(t)
	fixture.deps.Segments = pickups.segments
	command := pickupRegistrationCommand(t)
	command.Segment = "segment-1"
	command.SegmentServiceAction = "FINAL_DELIVERY"
	if result, err := pickups.handler.Register(t.Context(), command); err != nil || result.Outcome() != application.PickupRegistered {
		t.Fatalf("收寄进段：%v %q", err, result.Outcome())
	}

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("触发：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchNotTriggered || result.Refusal() != application.DeliveryTriggerRefusedEntryNotByHandover {
		t.Fatalf("outcome = %q refusal = %s, want NOT_A_DELIVERY_TRIGGER / ENTRY_NOT_BY_HANDOVER", result.Outcome(), result.Refusal())
	}
	if fixture.tasks.saves != 0 {
		t.Fatalf("凭收寄进段开了派送任务：saves=%d", fixture.tasks.saves)
	}
}

// Covers: ADR-0114 决定二 — 「对象必须是在场参与——凭链尾判，被替代或已离场的参与不触发」。对象已凭明确控制终止离场，
// 这一拍再来就不再是一个可派送的对象。
func TestAnObjectThatAlreadyLeftTheSegmentDoesNotTrigger(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	ender := application.NewEndFulfillmentParticipationHandler(application.EndFulfillmentParticipationDeps{
		Segments: fixture.handovers.segments,
		Clock:    dispatchTaskClock{at: taskRecordedAt},
	})
	ended, err := ender.End(t.Context(), application.EndFulfillmentParticipationCommand{
		TenantID: triggerCommand(t, "parcel-1").TenantID,
		Segment:  "segment-1",
		Object:   "parcel-1",
		Source:   application.ParticipationEndedByTermination,
		Basis:    "control-termination/parcel-1",
		EndedAt:  handoverJudgedTime.Add(30 * time.Minute),
	})
	if err != nil || ended.Outcome() != application.ParticipationEndedNow {
		t.Fatalf("离场：%v %q", err, ended.Outcome())
	}

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("触发：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchNotTriggered || result.Refusal() != application.DeliveryTriggerRefusedParticipationNotActive {
		t.Fatalf("outcome = %q refusal = %s, want NOT_A_DELIVERY_TRIGGER / PARTICIPATION_NOT_ACTIVE", result.Outcome(), result.Refusal())
	}
	if fixture.tasks.saves != 0 {
		t.Fatalf("已离场的对象开了派送任务：saves=%d", fixture.tasks.saves)
	}
}

// Covers: 票 tf-segment-lifecycle-closure/11 裁决 4 在派送触发一路——对象凭以进段的`已交接`被更正为拒收，链尾是失效版本：
// 对象在本段当前无有效参与，答的是 OBJECT_NOT_IN_SEGMENT 而不是 PARTICIPATION_NOT_ACTIVE——后者说的是「已离场、这一拍
// 来晚了」，而失效的参与从未离场，也没有一个终点在册。
func TestAnObjectWhoseEntryWasWithdrawnDoesNotTrigger(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	withdrawing := correctHandoverStillHandedOver(t, "handover-result/parcel-1/v1", "handover-result/parcel-1/v2")
	withdrawing.Verdict = domain.HandoverRefused
	withdrawing.ReleasingEvidence, withdrawing.ReceivingEvidence, withdrawing.Rule = "", "", ""
	withdrawing.Basis = "handover-basis/withdrawn-1"
	corrected, err := fixture.handovers.handler.Correct(t.Context(), withdrawing)
	if err != nil || corrected.Outcome() != application.HandoverCorrected || corrected.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("撤回控制的更正：%v %q %s", err, corrected.Outcome(), corrected.SegmentEntryRefusal())
	}

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("触发：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchNotTriggered || result.Refusal() != application.DeliveryTriggerRefusedObjectNotInSegment {
		t.Fatalf("outcome = %q refusal = %s, want NOT_A_DELIVERY_TRIGGER / OBJECT_NOT_IN_SEGMENT", result.Outcome(), result.Refusal())
	}
	if fixture.tasks.saves != 0 {
		t.Fatalf("已失效的参与开了派送任务：saves=%d", fixture.tasks.saves)
	}
}

// 段不在册、对象不在段里：两格都是形成了的答案（重试不会变），各自单开一格而不并成「找不到」——续办动作不同：
// 一个去查段有没有立起来，一个去查对象进没进段。
func TestAMissingSegmentOrObjectIsRefusedNotUndecided(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)

	unknownSegment := triggerCommand(t, "parcel-1")
	unknownSegment.Segment = "segment-9"
	result, err := fixture.handler().Trigger(t.Context(), unknownSegment)
	if err != nil || result.Outcome() != application.DeliveryDispatchNotTriggered || result.Refusal() != application.DeliveryTriggerRefusedSegmentNotFound {
		t.Fatalf("段不在册：%v %q %s, want NOT_A_DELIVERY_TRIGGER / SEGMENT_NOT_FOUND", err, result.Outcome(), result.Refusal())
	}

	result, err = fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-7"))
	if err != nil || result.Outcome() != application.DeliveryDispatchNotTriggered || result.Refusal() != application.DeliveryTriggerRefusedObjectNotInSegment {
		t.Fatalf("对象不在段里：%v %q %s, want NOT_A_DELIVERY_TRIGGER / OBJECT_NOT_IN_SEGMENT", err, result.Outcome(), result.Refusal())
	}
	if fixture.tasks.saves != 0 {
		t.Fatalf("开了任务：saves=%d", fixture.tasks.saves)
	}
}

// Covers: ADR-0114 决定三 — 「任一端口未接线时执行器答`未决`并点名哪条缝，不造替身、不填默认」。三条缝今天一条都没接，
// 执行器对每一条各答一格，不并成一句「要求未就位」。
func TestAnUnwiredRequirementSeamStopsHonestlyAndNamesTheSeam(t *testing.T) {
	cases := map[string]struct {
		unwire func(*application.TriggerDeliveryDispatchDeps)
		reason application.DeliveryDispatchUndecidedReason
	}{
		"收件地点引用（parcel-shipment）":    {func(deps *application.TriggerDeliveryDispatchDeps) { deps.Places = nil }, application.DeliveryPlaceSourceNotWired},
		"计划履约段时间窗口（network-routing）": {func(deps *application.TriggerDeliveryDispatchDeps) { deps.Windows = nil }, application.DeliveryWindowSourceNotWired},
		"交付条件引用（party-commercial）":   {func(deps *application.TriggerDeliveryDispatchDeps) { deps.Conditions = nil }, application.DeliveryConditionSourceNotWired},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDispatchTriggerFixture(t)
			fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
			testCase.unwire(&fixture.deps)

			result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("缝未接不该上抛技术错误：%v", err)
			}
			if result.Outcome() != application.DeliveryDispatchUndecided {
				t.Fatalf("outcome = %q, want DELIVERY_DISPATCH_UNDECIDED", result.Outcome())
			}
			if result.UndecidedReason() != testCase.reason {
				t.Fatalf("reason = %s, want %s——未决要点名是哪条缝", result.UndecidedReason(), testCase.reason)
			}
			if result.ContinuationReference() == "" {
				t.Fatal("未决没有续办引用——接上线之后这一拍要能重跑")
			}
			if fixture.tasks.saves != 0 {
				t.Fatalf("缝未接却开了任务——那是替身或默认值：saves=%d", fixture.tasks.saves)
			}
		})
	}
}

// Covers: ADR-0114 决定三 — 「接了线而所有者答『没有』时任务保持待形成（REQUIREMENT_MISSING），也不填默认」。「没有」是
// 业务答案不是故障：对象没有计划段就没有时间窗。缺几件就点名几件。
func TestAnOwnerAnsweringMissingKeepsTheTaskUnformed(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.windows.resolution = ports.RequirementMissing

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("触发：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchRequirementMissing {
		t.Fatalf("outcome = %q, want REQUIREMENT_MISSING", result.Outcome())
	}
	if result.ContinuationReference() != "" {
		t.Fatal("所有者答「没有」是业务答案不是欠账，不该留续办引用")
	}
	if missing := result.Missing(); len(missing) != 1 || missing[0] != application.DeliveryWindowRequirement {
		t.Fatalf("missing = %v, want [DELIVERY_WINDOW]", missing)
	}
	if fixture.tasks.saves != 0 {
		t.Fatalf("所有者说没有时间窗却开了任务——填了默认窗口：saves=%d", fixture.tasks.saves)
	}

	fixture.places.resolution = ports.RequirementMissing
	fixture.conditions.resolution = ports.RequirementMissing
	result, _ = fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if missing := result.Missing(); len(missing) != 3 {
		t.Fatalf("三件都缺却只点名了 %v", missing)
	}
}

// 所有者那一侧读不回来是欠账，不是「没有」：等它恢复重跑同一拍就过。执行器对三条缝与段登记册各答一格。
func TestASourceFailureIsUndecidedAndTheSameBeatRerunsToSuccess(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.places.err = errors.New("parcel-shipment 读面不可用")

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("读不回不该上抛技术错误：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchUndecided || result.UndecidedReason() != application.DeliveryPlaceSourceUnavailable {
		t.Fatalf("outcome = %q reason = %s, want DELIVERY_DISPATCH_UNDECIDED / DELIVERY_PLACE_SOURCE_UNAVAILABLE", result.Outcome(), result.UndecidedReason())
	}
	if fixture.tasks.saves != 0 {
		t.Fatalf("地点读不回却开了任务：saves=%d", fixture.tasks.saves)
	}

	fixture.places.err = nil
	rerun, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil || rerun.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("恢复后重跑同一拍：%v %q, want DISPATCH_TASK_FORMED", err, rerun.Outcome())
	}

	fixture.handovers.segments.findErr = errors.New("段登记册不可用")
	result, err = fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil || result.Outcome() != application.DeliveryDispatchUndecided || result.UndecidedReason() != application.DeliverySegmentRegistryUnavailable {
		t.Fatalf("段读不回：%v %q %s, want DELIVERY_DISPATCH_UNDECIDED / SEGMENT_REGISTRY_UNAVAILABLE", err, result.Outcome(), result.UndecidedReason())
	}
}

// Covers: CONTEXT「控制事实先如实落库，任务在下一拍形成，形成失败重跑本拍，不回滚交接」。任务登记册写不进时执行器答未决，
// 段与交接那一侧一行不动；恢复后重跑同一拍开出任务。
func TestADispatchRegistryFailureLeavesTheHandoverUntouchedAndRerunsCleanly(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	segmentSaves, segmentJoins := fixture.handovers.segments.saves, fixture.handovers.segments.joins
	handoverSaves := fixture.handovers.handovers.saves
	fixture.tasks.saveErr = errors.New("任务登记册不可用")

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("写不进不该上抛技术错误：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchUndecided || result.UndecidedReason() != application.DeliveryDispatchTaskUndecided {
		t.Fatalf("outcome = %q reason = %s, want DELIVERY_DISPATCH_UNDECIDED / DISPATCH_TASK_UNDECIDED", result.Outcome(), result.UndecidedReason())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决没有续办引用")
	}
	if fixture.handovers.segments.saves != segmentSaves || fixture.handovers.segments.joins != segmentJoins || fixture.handovers.handovers.saves != handoverSaves {
		t.Fatal("任务没形成却动了段或交接——形成失败重跑本拍，不回滚交接、也不补写任何控制事实")
	}

	fixture.tasks.saveErr = nil
	rerun, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
	if err != nil || rerun.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("恢复后重跑同一拍：%v %q, want DISPATCH_TASK_FORMED", err, rerun.Outcome())
	}
}

// 不成形的输入不受理，且一次库都不碰：空租户、空段、空对象、没有这一拍的业务时间，各是一格构造门的事，不是「找不到」。
func TestAMalformedTriggerIsNotAccepted(t *testing.T) {
	cases := map[string]func(*application.TriggerDeliveryDispatchCommand){
		"空段":    func(command *application.TriggerDeliveryDispatchCommand) { command.Segment = "  " },
		"空对象":   func(command *application.TriggerDeliveryDispatchCommand) { command.Object = "" },
		"空租户":   func(command *application.TriggerDeliveryDispatchCommand) { command.TenantID = domain.TenantID{} },
		"缺业务时间": func(command *application.TriggerDeliveryDispatchCommand) { command.OccurredAt = time.Time{} },
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDispatchTriggerFixture(t)
			fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
			command := triggerCommand(t, "parcel-1")
			breakIt(&command)

			result, err := fixture.handler().Trigger(t.Context(), command)
			if err != nil {
				t.Fatalf("不受理不该上抛技术错误：%v", err)
			}
			if result.Outcome() != application.DeliveryDispatchNotAccepted {
				t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
			}
			if fixture.tasks.saves != 0 || fixture.places.calls+fixture.windows.calls+fixture.conditions.calls != 0 {
				t.Fatal("不受理却碰了任务登记册或派送要求端口")
			}
		})
	}
}
