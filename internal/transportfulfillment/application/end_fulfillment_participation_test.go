package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

type participationFixture struct {
	segments   *segmentRegistryDouble
	handovers  *handoverRegistryDouble
	deliveries *deliveryStoreDouble
	handler    *application.EndFulfillmentParticipationHandler
	tenant     domain.TenantID
}

// joinEarlyMember 让一个起点早于交付到场时刻的对象进段。
//
// 交付夹具的到场时刻（`deliveryArrivedAt`）比 `handoverJudgedTime` 早几个小时，而领域拒绝终点
// 早于起点——所以验交付结束要用一个进段更早的成员，不能拿两成员段里那两个。**那条拒绝本身另有
// 一条用例专钉**，不是绕开它。
func (fixture *participationFixture) joinEarlyMember(t *testing.T, object string) {
	t.Helper()
	handler := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.handovers,
		Segments:   fixture.segments,
		Downstream: &handoverHandoffDouble{},
		Clock:      handoverClock{at: handoverRegisteredAt},
	})
	command := registerHandoverCommand(t)
	command.Object = object
	command.Version = "handover-result/" + object + "/v1"
	command.JudgedAt = deliveryArrivedAt.Add(-2 * time.Hour)
	command.Segment = "segment-1"
	if _, err := handler.Register(t.Context(), command); err != nil {
		t.Fatalf("%s 进段：%v", object, err)
	}
}

// registerDelivery 把一条真实的交付登记进册——结束参与要凭读得回来的控制事实，测试因此不能
// 手搓一条记录塞进替身，得走生产登记路径。
func (fixture *participationFixture) registerDelivery(t *testing.T, object, attempt string) {
	t.Helper()
	handler := application.NewRegisterEffectiveDeliveryHandler(application.RegisterEffectiveDeliveryDeps{
		Attempts:   &deliveryViewDouble{outcome: domain.ObjectDelivered, found: true},
		Deliveries: fixture.deliveries,
		Versions:   &deliveryVersionFactory{},
		Downstream: &deliveryHandoffDouble{},
		Clock:      deliveryClock{at: deliveryRecordedAt},
	})
	command := registerCommand(t)
	command.Object = object
	command.Attempt = attempt
	if _, err := handler.Register(t.Context(), command); err != nil {
		t.Fatalf("登记交付：%v", err)
	}
}

func newParticipationFixture(t *testing.T) *participationFixture {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	fixture := &participationFixture{
		segments:   newSegmentRegistry(),
		handovers:  newHandoverRegistry(),
		deliveries: newDeliveryStore(),
		tenant:     tenant,
	}
	fixture.handler = application.NewEndFulfillmentParticipationHandler(
		application.EndFulfillmentParticipationDeps{
			Segments:   fixture.segments,
			Handovers:  fixture.handovers,
			Deliveries: fixture.deliveries,
			Clock:      handoverClock{at: handoverRegisteredAt},
		})
	return fixture
}

// twoMemberSegment 造一个两成员的段：两个对象各凭自己的交接进段，起点各自不同。
func (fixture *participationFixture) twoMemberSegment(t *testing.T) {
	t.Helper()
	handler := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.handovers,
		Segments:   fixture.segments,
		Downstream: &handoverHandoffDouble{},
		Clock:      handoverClock{at: handoverRegisteredAt},
	})
	for index, object := range []string{"parcel-1", "parcel-2"} {
		command := registerHandoverCommand(t)
		command.Object = object
		command.Version = "handover-result/" + object + "/v1"
		command.JudgedAt = handoverJudgedTime.Add(time.Duration(index) * time.Hour)
		command.Segment = "segment-1"
		if _, err := handler.Register(t.Context(), command); err != nil {
			t.Fatalf("%s 进段：%v", object, err)
		}
	}
}

func (fixture *participationFixture) participation(
	t *testing.T,
	object string,
) domain.FulfillmentParticipation {
	t.Helper()
	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	reference, err := domain.NewCarriedObjectReference(object)
	if err != nil {
		t.Fatalf("对象引用：%v", err)
	}
	found, ok := record.Segment.ParticipationFor(reference)
	if !ok {
		t.Fatalf("段里没有 %s", object)
	}
	return found
}

func terminationCommand(t *testing.T, object string) application.EndFulfillmentParticipationCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.EndFulfillmentParticipationCommand{
		TenantID: tenant,
		Segment:  "segment-1",
		Object:   object,
		Source:   application.ParticipationEndedByTermination,
		Basis:    "control-termination/case-1",
		EndedAt:  handoverJudgedTime.Add(6 * time.Hour),
	}
}

// **本票的头号红线**：同段两成员结果不同时互不覆盖（CONTEXT「共享实际履约段中的每个载运对象
// 分别成立、结束和更正，不能由整段结果覆盖成员差异」）。
//
// 一个走控制终止、一个走下一次权威交接，两种结果并存；而且**结束一个不动另一个的起点**。
func TestTwoMembersEndWithDifferentResultsWithoutOverwritingEachOther(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)

	if _, err := fixture.handler.End(t.Context(), terminationCommand(t, "parcel-1")); err != nil {
		t.Fatalf("终止 parcel-1：%v", err)
	}

	// parcel-2 走下一次权威交接：它要凭一条真实登记过的交接，不能自报依据。
	next := registerHandoverCommand(t)
	next.Object = "parcel-2"
	next.Scope = "handover-scope-2"
	next.Version = "handover-result/parcel-2/v2"
	next.JudgedAt = handoverJudgedTime.Add(8 * time.Hour)
	registrar := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.handovers,
		Downstream: &handoverHandoffDouble{},
		Clock:      handoverClock{at: handoverRegisteredAt},
	})
	if _, err := registrar.Register(t.Context(), next); err != nil {
		t.Fatalf("登记下一次交接：%v", err)
	}

	handoverEnd := terminationCommand(t, "parcel-2")
	handoverEnd.Source = application.ParticipationEndedByNextHandover
	handoverEnd.Basis = ""
	handoverEnd.Scope = "handover-scope-2"
	handoverEnd.Version = "handover-result/parcel-2/v2"
	if _, err := fixture.handler.End(t.Context(), handoverEnd); err != nil {
		t.Fatalf("交接结束 parcel-2：%v", err)
	}

	first := fixture.participation(t, "parcel-1")
	kind, basis, endedAt, ended := first.End()
	if !ended || kind != domain.EndedByControlTermination {
		t.Fatalf("parcel-1 结果 = %q ended=%v, want CONTROL_TERMINATED", kind, ended)
	}
	if basis.String() != "control-termination/case-1" {
		t.Fatalf("parcel-1 依据 = %q", basis)
	}
	if !endedAt.Equal(handoverJudgedTime.Add(6 * time.Hour)) {
		t.Fatalf("parcel-1 终点时刻 = %s", endedAt)
	}

	second := fixture.participation(t, "parcel-2")
	secondKind, _, secondEndedAt, secondEnded := second.End()
	if !secondEnded || secondKind != domain.EndedByNextHandover {
		t.Fatalf("parcel-2 结果 = %q ended=%v, want NEXT_HANDOVER", secondKind, secondEnded)
	}
	// 交接结束取裁决的业务时间，不取终止那一条的时刻——两成员各有各的终点。
	if !secondEndedAt.Equal(handoverJudgedTime.Add(8 * time.Hour)) {
		t.Fatalf("parcel-2 终点时刻 = %s", secondEndedAt)
	}
	// 起点也没被对方的结束动过。
	if !first.EnteredAt().Equal(handoverJudgedTime) {
		t.Fatalf("parcel-1 起点被改写成 %s", first.EnteredAt())
	}
	if !second.EnteredAt().Equal(handoverJudgedTime.Add(time.Hour)) {
		t.Fatalf("parcel-2 起点被改写成 %s", second.EnteredAt())
	}
}

// 结束一条已经离场的参与是业务答案不是错误：已结束的参与不重复结束也不改写，更正走新的判断
// 版本。**它与「对象不在段内」分开**，因为调用方要做的事不同。
func TestEndingAnAlreadyEndedParticipationIsItsOwnAnswer(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)

	if _, err := fixture.handler.End(t.Context(), terminationCommand(t, "parcel-1")); err != nil {
		t.Fatalf("首次终止：%v", err)
	}
	result, err := fixture.handler.End(t.Context(), terminationCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("重复终止不该上抛：%v", err)
	}
	if result.Outcome() != application.ParticipationAlreadyEnded {
		t.Fatalf("outcome = %q, want PARTICIPATION_ALREADY_ENDED", result.Outcome())
	}

	// 原结果一字未改——重复结束不是覆盖。
	kind, basis, _, _ := fixture.participation(t, "parcel-1").End()
	if kind != domain.EndedByControlTermination || basis.String() != "control-termination/case-1" {
		t.Fatalf("重复结束改写了原结果：kind=%q basis=%q", kind, basis)
	}
}

// 不在段内的对象结束不了，且**不因此顺手把它加进去**。
func TestEndingAnObjectThatNeverJoinedIsRefused(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)

	result, err := fixture.handler.End(t.Context(), terminationCommand(t, "parcel-9"))
	if err != nil {
		t.Fatalf("不在段内不该上抛：%v", err)
	}
	if result.Outcome() != application.ParticipationObjectNotInSegment {
		t.Fatalf("outcome = %q, want OBJECT_NOT_IN_SEGMENT", result.Outcome())
	}
	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	if len(record.Segment.Participations()) != 2 {
		t.Fatalf("结束一个不在段内的对象把它加进去了：%d 个成员", len(record.Segment.Participations()))
	}
}

// **凭一条不存在的交接结束参与要被拒。** 结束的依据必须是一条读得回来的控制事实，不是调用方
// 自报的引用串——否则任何人都能宣称一次并未发生的交接把对象移出本段。
func TestEndingByAHandoverThatWasNeverRegisteredIsRefused(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)

	command := terminationCommand(t, "parcel-1")
	command.Source = application.ParticipationEndedByNextHandover
	command.Basis = ""
	command.Scope = "handover-scope-2"
	command.Version = "handover-result/parcel-1/never"

	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("交接不在册不该上抛：%v", err)
	}
	if result.Outcome() != application.ParticipationControlFactNotFound {
		t.Fatalf("outcome = %q, want CONTROL_FACT_NOT_FOUND", result.Outcome())
	}
	if _, _, _, ended := fixture.participation(t, "parcel-1").End(); ended {
		t.Fatal("凭一条不存在的交接把参与结束掉了")
	}
}

// 段不在册与对象不在段内分开：前者说这个段从未成立，后者说段在但这个对象不是它的成员。
func TestEndingInASegmentThatWasNeverEstablishedIsRefused(t *testing.T) {
	fixture := newParticipationFixture(t)

	command := terminationCommand(t, "parcel-1")
	command.Segment = "segment-never"
	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("段不在册不该上抛：%v", err)
	}
	if result.Outcome() != application.ParticipationSegmentNotFound {
		t.Fatalf("outcome = %q, want SEGMENT_NOT_FOUND", result.Outcome())
	}
}

// 缺依据的控制终止不受理：没有有效控制结束事实时，停止移动、异常案件或计划取消均不结束控制。
func TestATerminationWithoutABasisIsNotAccepted(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)

	command := terminationCommand(t, "parcel-1")
	command.Basis = ""
	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("缺依据不该上抛：%v", err)
	}
	if result.Outcome() != application.ParticipationEndNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
	}
}

// 有效交付结束参与：CONTEXT「有效交付同时形成该对象向收件方的控制转移并结束相应履约参与
// 关系」。依据同样凭一条读得回来的交付登记，不收自报引用。
func TestAnEffectiveDeliveryEndsTheParticipation(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)
	fixture.joinEarlyMember(t, "parcel-3")
	fixture.registerDelivery(t, "parcel-3", "attempt-1")

	command := terminationCommand(t, "parcel-3")
	command.Source = application.ParticipationEndedByDelivery
	command.Basis = ""
	command.Attempt = "attempt-1"

	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("交付结束：%v", err)
	}
	if result.Outcome() != application.ParticipationEndedNow {
		t.Fatalf("outcome = %q, want PARTICIPATION_ENDED", result.Outcome())
	}
	kind, _, _, ended := fixture.participation(t, "parcel-3").End()
	if !ended || kind != domain.EndedByEffectiveDelivery {
		t.Fatalf("结果 = %q ended=%v, want EFFECTIVE_DELIVERY", kind, ended)
	}
	// 同段另外两个成员一动没动——结束一条不碰别人。
	if _, _, _, otherEnded := fixture.participation(t, "parcel-1").End(); otherEnded {
		t.Fatal("交付结束 parcel-3 顺手把 parcel-1 也结束了")
	}
}

// **终点早于起点要被拒**，而这一条是写上一条用例时撞出来的：交付夹具的到场时刻比两成员段的
// 参与起点早几个小时，于是领域当场拒了。它不是夹具凑巧，是 CONTEXT 那条「实际控制起止」的
// 直接后果——一次发生在对象进段之前的交付，结束不了它此后才成立的参与。
func TestADeliveryEarlierThanTheParticipationStartIsRefused(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)
	fixture.registerDelivery(t, "parcel-1", "attempt-1")

	command := terminationCommand(t, "parcel-1")
	command.Source = application.ParticipationEndedByDelivery
	command.Basis = ""
	command.Attempt = "attempt-1"

	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("不该上抛：%v", err)
	}
	if result.Outcome() != application.ParticipationEndNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
	}
	if _, _, _, ended := fixture.participation(t, "parcel-1").End(); ended {
		t.Fatal("一条早于起点的交付把参与结束了")
	}
}

// 凭一条不存在的交付结束参与要被拒——与交接那一路同一条判据。
func TestEndingByADeliveryThatWasNeverRegisteredIsRefused(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)

	command := terminationCommand(t, "parcel-1")
	command.Source = application.ParticipationEndedByDelivery
	command.Basis = ""
	command.Attempt = "attempt-never"

	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("交付不在册不该上抛：%v", err)
	}
	if result.Outcome() != application.ParticipationControlFactNotFound {
		t.Fatalf("outcome = %q, want CONTROL_FACT_NOT_FOUND", result.Outcome())
	}
}

// **控制边界变化：结束原关系并形成下一段。** CONTEXT「可验证的实际承运责任或运输控制边界
// 发生变化时，结束原载运对象的履约参与关系并形成下一实际履约段」。
//
// 下一段的段引用由调用方显式给（与票 02 同一条裁定），缺席则只结束不立新段。
func TestAControlBoundaryChangeEndsHereAndEntersTheNextSegment(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)

	next := registerHandoverCommand(t)
	next.Object = "parcel-1"
	next.Scope = "handover-scope-2"
	next.Version = "handover-result/parcel-1/v2"
	next.JudgedAt = handoverJudgedTime.Add(9 * time.Hour)
	registrar := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.handovers,
		Downstream: &handoverHandoffDouble{},
		Clock:      handoverClock{at: handoverRegisteredAt},
	})
	if _, err := registrar.Register(t.Context(), next); err != nil {
		t.Fatalf("登记下一次交接：%v", err)
	}

	command := terminationCommand(t, "parcel-1")
	command.Source = application.ParticipationEndedByNextHandover
	command.Basis = ""
	command.Scope = "handover-scope-2"
	command.Version = "handover-result/parcel-1/v2"
	command.NextSegment = "segment-2"

	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("边界变化：%v", err)
	}
	if result.Outcome() != application.ParticipationEndedNow {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	if reference := result.NextSegmentContinuationReference(); reference != "" {
		t.Fatalf("下一段没进去却报了成功：%q", reference)
	}

	// 原段那一条已离场，新段里同一对象凭同一条交接成立了新的参与起点。
	if _, _, _, ended := fixture.participation(t, "parcel-1").End(); !ended {
		t.Fatal("原段那一条没结束")
	}
	nextRecord := fixture.segments.saved(t, "tenant-1", "segment-2")
	object, err := domain.NewCarriedObjectReference("parcel-1")
	if err != nil {
		t.Fatalf("对象引用：%v", err)
	}
	joined, present := nextRecord.Segment.ParticipationFor(object)
	if !present {
		t.Fatal("下一段里没有这个对象")
	}
	if joined.EntryKind() != domain.EnteredByTransportHandover || !joined.Active() {
		t.Fatalf("下一段的参与起点不对：kind=%q active=%v", joined.EntryKind(), joined.Active())
	}
	// 原段那一条的终点与新段那一条的起点取同一条交接的裁决时刻——它们是同一件事的两面。
	if !joined.EnteredAt().Equal(handoverJudgedTime.Add(9 * time.Hour)) {
		t.Fatalf("下一段起点 = %s", joined.EnteredAt())
	}
}

// **交付或终止不得带下一段。** 有效交付把控制转给收件方、终止是控制结束——两者之后都没有
// 「下一段」。允许它就等于能表达「已交付但又进了下一段」，而那在 CONTEXT 里不成立。
func TestADeliveryOrTerminationCannotCarryANextSegment(t *testing.T) {
	for name, source := range map[string]application.ParticipationEndSource{
		"终止带下一段": application.ParticipationEndedByTermination,
		"交付带下一段": application.ParticipationEndedByDelivery,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newParticipationFixture(t)
			fixture.twoMemberSegment(t)
			fixture.joinEarlyMember(t, "parcel-3")
			fixture.registerDelivery(t, "parcel-3", "attempt-1")

			command := terminationCommand(t, "parcel-3")
			command.Source = source
			command.Attempt = "attempt-1"
			command.NextSegment = "segment-2"

			result, err := fixture.handler.End(t.Context(), command)
			if err != nil {
				t.Fatalf("不该上抛：%v", err)
			}
			if result.Outcome() != application.ParticipationEndNotAccepted {
				t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
			}
			if _, _, _, ended := fixture.participation(t, "parcel-3").End(); ended {
				t.Fatal("被拒的命令仍然把参与结束了")
			}
		})
	}
}

// 段登记册故障形成未决并带续办引用。
func TestASegmentRegistryFailureEndingAParticipationIsUndecided(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)
	fixture.segments.findErr = assignmentRegistryError

	result, err := fixture.handler.End(t.Context(), terminationCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("登记册故障不该上抛：%v", err)
	}
	if result.Outcome() != application.ParticipationEndUndecided {
		t.Fatalf("outcome = %q, want PARTICIPATION_END_UNDECIDED", result.Outcome())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决没有续办引用")
	}
}
