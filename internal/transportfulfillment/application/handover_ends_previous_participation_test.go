package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func (fixture *participationFixture) handoverHandlerEndingParticipations() *application.RegisterTransportHandoverHandler {
	return application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:         fixture.handovers,
		Segments:          fixture.segments,
		Downstream:        &handoverHandoffDouble{},
		Clock:             handoverClock{at: handoverRegisteredAt},
		ParticipationEnds: fixture.handler,
	})
}

// Covers: 票 tf-segment-lifecycle-closure/06 裁决 (i) 的交接一路——`已交接`落库后**同一编排内**结束该对象在前一段
// 的参与，再进新段（CONTEXT 生命周期③「下一次权威交接」与①②「形成下一实际履约段」）。邻居不动；拒收不结束
// 任何参与；对象的第一次交接答 NO_ACTIVE_PARTICIPATION 而不静默。
func TestARegisteredHandoverEndsThePreviousParticipationThenEntersTheNextSegment(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)
	handler := fixture.handoverHandlerEndingParticipations()

	next := registerHandoverCommand(t)
	next.Object = "parcel-1"
	next.Scope = "handover-scope-2"
	next.Version = "handover-result/parcel-1/v2"
	next.JudgedAt = handoverJudgedTime.Add(3 * time.Hour)
	next.Segment = "segment-2"

	result, err := handler.Register(t.Context(), next)
	if err != nil {
		t.Fatalf("下一次交接：%v", err)
	}
	if result.Outcome() != application.HandoverRegistered || result.ParticipationEnd() != application.ParticipationEndedNow {
		t.Fatalf("outcome = %s participationEnd = %s", result.Outcome(), result.ParticipationEnd())
	}
	if fixture.participation(t, "parcel-1").Active() {
		t.Fatal("parcel-1 在前一段的参与没有结束")
	}
	if !fixture.participation(t, "parcel-2").Active() {
		t.Fatal("邻居 parcel-2 的参与被一起结束了")
	}
	if result.SegmentContinuationReference() != "" || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("进新段那一半：debt=%q refusal=%s", result.SegmentContinuationReference(), result.SegmentEntryRefusal())
	}
	record := fixture.segments.saved(t, "tenant-1", "segment-2").Segment
	participation, present := record.ParticipationFor(value(t, domain.NewCarriedObjectReference, "parcel-1"))
	if !present || !participation.Active() {
		t.Fatal("parcel-1 没有在新段 segment-2 里在场")
	}

	t.Run("a refusal transfers no control and ends nothing", func(t *testing.T) {
		refused := registerHandoverCommand(t)
		refused.Object = "parcel-2"
		refused.Scope = "handover-scope-3"
		refused.Version = "handover-result/parcel-2/v2"
		refused.Verdict = domain.HandoverRefused
		refused.Basis = "refusal-basis-1"
		refused.JudgedAt = handoverJudgedTime.Add(3 * time.Hour)
		result, err := handler.Register(t.Context(), refused)
		if err != nil {
			t.Fatalf("拒收：%v", err)
		}
		if result.Outcome() != application.HandoverRegistered || result.ParticipationEnd() != application.ParticipationEndOutcomeInvalid {
			t.Fatalf("outcome = %s participationEnd = %s", result.Outcome(), result.ParticipationEnd())
		}
		if !fixture.participation(t, "parcel-2").Active() {
			t.Fatal("拒收结束了 parcel-2 的参与")
		}
	})

	t.Run("the first handover of an object is answered as no active participation", func(t *testing.T) {
		first := registerHandoverCommand(t)
		first.Object = "parcel-7"
		first.Version = "handover-result/parcel-7/v1"
		first.Segment = "segment-7"
		result, err := handler.Register(t.Context(), first)
		if err != nil {
			t.Fatalf("首次交接：%v", err)
		}
		if result.ParticipationEnd() != application.ParticipationNoActiveParticipation {
			t.Fatalf("participationEnd = %s, want NO_ACTIVE_PARTICIPATION", result.ParticipationEnd())
		}
		if members := len(fixture.segments.saved(t, "tenant-1", "segment-7").Segment.Participations()); members != 1 {
			t.Fatalf("首次交接立起的新段成员 = %d, want 1", members)
		}
	})
}

// 构造时就看得见：交接那一侧同样漏接即 panic。
func TestConstructingTheHandoverHandlerWithoutAParticipationEnderPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("没有 panic")
		}
	}()
	application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{})
}
