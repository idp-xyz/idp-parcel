package application_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// closureFixture 在结束参与那套夹具上多挂一个关段编排：关段的前提正是那套夹具造得出来的
// 「全部参与已结束」的段。
type closureFixture struct {
	*participationFixture
	closer *application.CloseFulfillmentSegmentHandler
}

var segmentClosureDeclaredAt = handoverJudgedTime.Add(8 * time.Hour)

func newClosureFixture(t *testing.T) *closureFixture {
	t.Helper()
	inner := newParticipationFixture(t)
	return &closureFixture{
		participationFixture: inner,
		closer: application.NewCloseFulfillmentSegmentHandler(application.CloseFulfillmentSegmentDeps{
			Segments: inner.segments,
		}),
	}
}

// endBoth 让两成员段的两个对象各自以明确控制终止收尾——关段要的是「全部有效参与关系已经结束」。
func (fixture *closureFixture) endBoth(t *testing.T) {
	t.Helper()
	for _, object := range []string{"parcel-1", "parcel-2"} {
		result, err := fixture.handler.End(t.Context(), terminationCommand(t, object))
		if err != nil || result.Outcome() != application.ParticipationEndedNow {
			t.Fatalf("结束 %s：outcome=%s err=%v", object, result.Outcome(), err)
		}
	}
}

// arriveByHandover 让一个新对象凭`已交接`的权威交接来到同一个段——走的是生产进段路径，不是
// 直接往替身里塞。
func (fixture *closureFixture) arriveByHandover(t *testing.T, object string) {
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
	command.JudgedAt = segmentClosureDeclaredAt.Add(time.Hour)
	command.Segment = "segment-1"
	if _, err := handler.Register(t.Context(), command); err != nil {
		t.Fatalf("%s 到场：%v", object, err)
	}
}

func closeCommand(t *testing.T) application.CloseFulfillmentSegmentCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.CloseFulfillmentSegmentCommand{
		TenantID: tenant,
		Segment:  "segment-1",
		ClosedAt: segmentClosureDeclaredAt,
	}
}

// CONTEXT 生命周期④：「全部有效参与关系已经结束且不再接受新对象 → 实际履约段结束」。
//
// 前半由参与关系的状态给，后半是这条命令本身——它就是那个「不再接受新对象」的声明。声明落库后，
// 一个新对象凭正当的控制事实来到同一个段，也进不去：段的成员数不动、关闭状态不动。
func TestASegmentWhoseParticipationsHaveAllEndedClosesAndAcceptsNoNewObjects(t *testing.T) {
	fixture := newClosureFixture(t)
	fixture.twoMemberSegment(t)
	fixture.endBoth(t)

	result, err := fixture.closer.Close(t.Context(), closeCommand(t))
	if err != nil {
		t.Fatalf("关段：%v", err)
	}
	if got := result.Outcome(); got != application.SegmentClosedNow {
		t.Fatalf("outcome = %s, want SegmentClosedNow", got)
	}

	closed := fixture.segments.saved(t, "tenant-1", "segment-1").Segment
	closedAt, isClosed := closed.ClosedAt()
	if !closed.Closed() || !isClosed || !closedAt.Equal(segmentClosureDeclaredAt) {
		t.Fatalf("段没有按声明关闭：closed=%v at=%v", closed.Closed(), closedAt)
	}

	fixture.arriveByHandover(t, "parcel-3")

	after := fixture.segments.saved(t, "tenant-1", "segment-1").Segment
	if members := len(after.Participations()); members != 2 {
		t.Fatalf("关闭后的段收下了新对象：成员 %d，want 2", members)
	}
	if !after.Closed() {
		t.Fatal("新对象到场把段的关闭状态翻回去了")
	}
}

// CONTEXT：「没有有效控制结束事实时，停止移动、异常案件或计划取消均不结束该段」——一个仍在控制中
// 的对象足以让段继续存在。声明来了也关不上，登记册一动不动。
func TestASegmentWithAnActiveParticipationCannotBeClosed(t *testing.T) {
	fixture := newClosureFixture(t)
	fixture.twoMemberSegment(t)
	if _, err := fixture.handler.End(t.Context(), terminationCommand(t, "parcel-1")); err != nil {
		t.Fatalf("结束 parcel-1：%v", err)
	}

	result, err := fixture.closer.Close(t.Context(), closeCommand(t))
	if err != nil {
		t.Fatalf("关段：%v", err)
	}
	if got := result.Outcome(); got != application.SegmentStillHasActiveParticipations {
		t.Fatalf("outcome = %s, want SegmentStillHasActiveParticipations", got)
	}

	segment := fixture.segments.saved(t, "tenant-1", "segment-1").Segment
	if segment.Closed() || segment.ActiveParticipations() != 1 {
		t.Fatalf("关不上的段被动了：closed=%v active=%d", segment.Closed(), segment.ActiveParticipations())
	}
}

// 重放同一条声明是业务答案不是失败；而且第一次声明的时刻不被第二次改写——决定只做一次。
func TestClosingAnAlreadyClosedSegmentIsItsOwnAnswer(t *testing.T) {
	fixture := newClosureFixture(t)
	fixture.twoMemberSegment(t)
	fixture.endBoth(t)
	if result, err := fixture.closer.Close(t.Context(), closeCommand(t)); err != nil || result.Outcome() != application.SegmentClosedNow {
		t.Fatalf("首次关段：outcome=%s err=%v", result.Outcome(), err)
	}

	replay := closeCommand(t)
	replay.ClosedAt = segmentClosureDeclaredAt.Add(3 * time.Hour)
	result, err := fixture.closer.Close(t.Context(), replay)
	if err != nil {
		t.Fatalf("重放关段：%v", err)
	}
	if got := result.Outcome(); got != application.SegmentAlreadyClosed {
		t.Fatalf("outcome = %s, want SegmentAlreadyClosed", got)
	}
	closedAt, _ := fixture.segments.saved(t, "tenant-1", "segment-1").Segment.ClosedAt()
	if !closedAt.Equal(segmentClosureDeclaredAt) {
		t.Fatalf("第二次声明改写了第一次的关闭时刻：%v", closedAt)
	}
}

func TestClosingASegmentThatWasNeverEstablishedIsRefused(t *testing.T) {
	fixture := newClosureFixture(t)

	result, err := fixture.closer.Close(t.Context(), closeCommand(t))
	if err != nil {
		t.Fatalf("关段：%v", err)
	}
	if got := result.Outcome(); got != application.SegmentToCloseNotFound {
		t.Fatalf("outcome = %s, want SegmentToCloseNotFound", got)
	}
}

// 关闭时刻由调用方给，缺了它这条声明就没有业务时间；段引用缺了则不知道在声明谁。两者都在动库之前拒。
func TestAClosureWithoutATimeOrASegmentIsNotAccepted(t *testing.T) {
	fixture := newClosureFixture(t)
	fixture.twoMemberSegment(t)
	fixture.endBoth(t)

	withoutTime := closeCommand(t)
	withoutTime.ClosedAt = time.Time{}
	withoutSegment := closeCommand(t)
	withoutSegment.Segment = "   "

	for name, command := range map[string]application.CloseFulfillmentSegmentCommand{
		"没有关闭时刻": withoutTime,
		"没有段引用":  withoutSegment,
	} {
		result, err := fixture.closer.Close(t.Context(), command)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if got := result.Outcome(); got != application.SegmentCloseNotAccepted {
			t.Fatalf("%s：outcome = %s, want SegmentCloseNotAccepted", name, got)
		}
	}
	if fixture.segments.saved(t, "tenant-1", "segment-1").Segment.Closed() {
		t.Fatal("不受理的声明把段关了")
	}
}

// 登记册读不到或写不进是欠账不是答案：交回续办引用，不假装关了也不假装没关。
func TestASegmentRegistryFailureClosingIsUndecided(t *testing.T) {
	t.Run("reading the segment back fails", func(t *testing.T) {
		fixture := newClosureFixture(t)
		fixture.twoMemberSegment(t)
		fixture.endBoth(t)
		fixture.segments.findErr = errors.New("registry down")

		result, err := fixture.closer.Close(t.Context(), closeCommand(t))
		if err != nil {
			t.Fatalf("关段：%v", err)
		}
		if got := result.Outcome(); got != application.SegmentCloseUndecided {
			t.Fatalf("outcome = %s, want SegmentCloseUndecided", got)
		}
		if result.ContinuationReference() == "" {
			t.Fatal("未决没有交回续办引用")
		}
	})

	t.Run("writing the closure fails", func(t *testing.T) {
		fixture := newClosureFixture(t)
		fixture.twoMemberSegment(t)
		fixture.endBoth(t)
		fixture.segments.closeErr = errors.New("registry down")

		result, err := fixture.closer.Close(t.Context(), closeCommand(t))
		if err != nil {
			t.Fatalf("关段：%v", err)
		}
		if got := result.Outcome(); got != application.SegmentCloseUndecided {
			t.Fatalf("outcome = %s, want SegmentCloseUndecided", got)
		}
		if fixture.segments.saved(t, "tenant-1", "segment-1").Segment.Closed() {
			t.Fatal("写失败的关段在登记册里成了真的")
		}
	})

	t.Run("the outcome set is closed", func(t *testing.T) {
		if got := application.SegmentCloseOutcome(250).String(); got != "" {
			t.Fatalf("未知结果被命了名：%q", got)
		}
	})
}
