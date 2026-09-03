package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// closedSegmentFixture 造一个已按声明关闭的两成员段，再让三条控制事实编排各自往里送一个新对象。
type closedSegmentFixture struct {
	*closureFixture
}

func newClosedSegmentFixture(t *testing.T) *closedSegmentFixture {
	t.Helper()
	fixture := &closedSegmentFixture{closureFixture: newClosureFixture(t)}
	fixture.twoMemberSegment(t)
	fixture.endBoth(t)
	result, err := fixture.closer.Close(t.Context(), closeCommand(t))
	if err != nil || result.Outcome() != application.SegmentClosedNow {
		t.Fatalf("关段：outcome=%s err=%v", result.Outcome(), err)
	}
	return fixture
}

func (fixture *closedSegmentFixture) handoverHandler() *application.RegisterTransportHandoverHandler {
	return application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.handovers,
		Segments:   fixture.segments,
		Downstream: &handoverHandoffDouble{},
		Clock:      handoverClock{at: handoverRegisteredAt},
	})
}

// Covers: 票 tf-segment-lifecycle-closure/03 裁决 3——「段已关闭」**单开一格答给调用方**（含义：另立
// 新段）。此前段已关闭后新对象凭正当控制事实来到同段，可观察结果只有「交接在册、段里没它」，与
// 正常入段在调用方眼里同形，没有任何一格告诉它该另立新段。
//
// 三条约束一起钉：来源保全照常成立（交接 / 收寄 / 到访都登上了）；这一格**不是欠账**——段没关
// 时进段那一半的失败才是欠账，段关了是领域正当拒绝，留续办引用会让调用方反复重试一件本就不该
// 发生的事；段的成员数与关闭状态一动不动（票 01 的判据，这里不重钉，只核成员数）。
func TestArrivingAtAClosedSegmentIsAnsweredAsSegmentClosedNotAsDebt(t *testing.T) {
	t.Run("by transport handover", func(t *testing.T) {
		fixture := newClosedSegmentFixture(t)
		command := registerHandoverCommand(t)
		command.Object = "parcel-3"
		command.Version = "handover-result/parcel-3/v1"
		command.JudgedAt = segmentClosureDeclaredAt.Add(time.Hour)
		command.Segment = "segment-1"

		result, err := fixture.handoverHandler().Register(t.Context(), command)
		if err != nil {
			t.Fatalf("交接：%v", err)
		}
		if result.Outcome() != application.HandoverRegistered {
			t.Fatalf("outcome = %s, want HANDOVER_REGISTERED——段关了抹不掉已经发生的交接", result.Outcome())
		}
		if result.SegmentEntryRefusal() != application.SegmentEntryRefusedSegmentClosed {
			t.Fatalf("segmentEntryRefusal = %s, want SEGMENT_CLOSED", result.SegmentEntryRefusal())
		}
		if reference := result.SegmentContinuationReference(); reference != "" {
			t.Fatalf("段已关闭是正当拒绝不是欠账，却留了续办引用 %q", reference)
		}
		if members := len(fixture.segments.saved(t, "tenant-1", "segment-1").Segment.Participations()); members != 2 {
			t.Fatalf("关闭后的段收下了新对象：成员 %d，want 2", members)
		}
	})

	t.Run("by offsite pickup registration", func(t *testing.T) {
		fixture := newClosedSegmentFixture(t)
		handler := application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
			Pickups:    newPickupRegistry(),
			Segments:   fixture.segments,
			Versions:   &pickupRegVersionFactory{},
			Downstream: &pickupRegHandoffDouble{},
			Clock:      pickupRegClock{at: pickupRegisteredAt},
		})
		command := pickupRegistrationCommand(t)
		command.Object = "parcel-4"
		command.OccurredAt = segmentClosureDeclaredAt.Add(time.Hour)
		command.Segment = "segment-1"

		result, err := handler.Register(t.Context(), command)
		if err != nil {
			t.Fatalf("收寄：%v", err)
		}
		if result.Outcome() != application.PickupRegistered {
			t.Fatalf("outcome = %s, want PICKUP_REGISTERED", result.Outcome())
		}
		if result.SegmentEntryRefusal() != application.SegmentEntryRefusedSegmentClosed {
			t.Fatalf("segmentEntryRefusal = %s, want SEGMENT_CLOSED", result.SegmentEntryRefusal())
		}
		if reference := result.SegmentContinuationReference(); reference != "" {
			t.Fatalf("段已关闭是正当拒绝不是欠账，却留了续办引用 %q", reference)
		}
	})

	t.Run("by a multi-object pickup attempt", func(t *testing.T) {
		fixture := newClosedSegmentFixture(t)
		handler := application.NewPerformOffsitePickupHandler(application.PerformOffsitePickupDeps{
			Attempts:   newPickupStore(),
			Segments:   fixture.segments,
			Versions:   &versionFactoryDouble{},
			Downstream: &pickupHandoffDouble{},
			Clock:      fixedClock{at: recordedAt},
		})
		command := pickupCommand(t, "source-9", "attempt-9",
			successSubmission(t, "parcel-5", "TRANSPORT-CONTROL/TF-5"),
			successSubmission(t, "parcel-6", "TRANSPORT-CONTROL/TF-6"),
		)
		command.Segment = "segment-1"

		result, err := handler.Handle(t.Context(), command)
		if err != nil {
			t.Fatalf("到访：%v", err)
		}
		if result.Outcome() != application.PickupAttemptRecorded {
			t.Fatalf("outcome = %s, want ATTEMPT_RECORDED", result.Outcome())
		}
		if result.SegmentEntryRefusal() != application.SegmentEntryRefusedSegmentClosed {
			t.Fatalf("segmentEntryRefusal = %s, want SEGMENT_CLOSED", result.SegmentEntryRefusal())
		}
		if entries := result.SegmentEntries(); len(entries) != 0 {
			t.Fatalf("段已关闭是正当拒绝不是欠账，却报了逐对象欠账：%+v", entries)
		}
	})
}

// 段没关时这一格必须是空的：它只答「段已关闭」，不替「对象已在段内」「没要求进段」之类的正当结果
// 说话——那些各有自己的可观察形状（成员数不动、段登记册不被触及）。
func TestEnteringAnOpenSegmentCarriesNoRefusal(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)
	command := registerHandoverCommand(t)
	command.Segment = "segment-1"

	result, err := fixture.handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("交接：%v", err)
	}
	if result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("segmentEntryRefusal = %s, want none", result.SegmentEntryRefusal())
	}
	if result.SegmentEntryRefusal().String() != "" {
		t.Fatalf("空格的 String 应为空串，得到 %q", result.SegmentEntryRefusal().String())
	}
}
