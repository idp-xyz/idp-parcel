package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

type participationFixture struct {
	segments  *segmentRegistryDouble
	handovers *handoverRegistryDouble
	handler   *application.EndFulfillmentParticipationHandler
	tenant    domain.TenantID
}

func newParticipationFixture(t *testing.T) *participationFixture {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	fixture := &participationFixture{
		segments:  newSegmentRegistry(),
		handovers: newHandoverRegistry(),
		tenant:    tenant,
	}
	fixture.handler = application.NewEndFulfillmentParticipationHandler(
		application.EndFulfillmentParticipationDeps{
			Segments:  fixture.segments,
			Handovers: fixture.handovers,
			Clock:     handoverClock{at: handoverRegisteredAt},
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
