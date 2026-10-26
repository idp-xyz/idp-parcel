package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: AT-TF-356（票 demo-chain-gaps/183；ADR-0307 越权风险点 2）——对象此刻在 segment-0 的参与起于 enteredAt，首登`已交接`
// 的业务时间早于它（离线补登、信封晚到）：交接照登，段那一半一笔不写——不结束 segment-0 的参与、不立声明的段，答
// LATER_CONTROL_FACTS、不留续办引用，对象仍只在 segment-0；同一交接原样再投答已有版本，不再碰段。
func TestALateFirstHandoverIsRecordedWithoutEnteringTheSegment(t *testing.T) {
	enteredAt := handoverJudgedTime.Add(3 * time.Hour)
	fixture := newParticipationFixture(t)
	handler := fixture.handoverHandlerEndingParticipations()
	fixture.enterPreviousSegment(t, handler, "parcel-1", enteredAt)

	late := firstJudgment(t, domain.ObjectHandedOver, handoverJudgedTime.Add(time.Hour))
	result, err := handler.Register(t.Context(), late)
	if err != nil || result.Outcome() != application.HandoverRegistered {
		t.Fatalf("晚到首登：%v %s, want 交接照登", err, result.Outcome())
	}
	if result.SegmentEntryRefusal() != application.SegmentEntryRefusedLaterControlFacts ||
		result.SegmentEntryRefusal().String() != "LATER_CONTROL_FACTS" ||
		result.SegmentContinuationReference() != "" || result.ParticipationEnd() != application.ParticipationEndOutcomeInvalid {
		t.Fatalf("refusal=%s debt=%q end=%s, want LATER_CONTROL_FACTS、无欠账、没结束任何参与",
			result.SegmentEntryRefusal(), result.SegmentContinuationReference(), result.ParticipationEnd())
	}
	for _, row := range fixture.segments.rows {
		if row.key.Segment.String() == declaredSegment {
			t.Fatal("声明的段被立起来了")
		}
	}
	if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 1 || active[0] != previousSegment {
		t.Fatalf("在场的段 = %v, want 仍只在 %s", active, previousSegment)
	}
	previous, _ := fixture.tail(t, previousSegment, "parcel-1")
	if _, _, _, ended := previous.End(); ended {
		t.Fatal("先前段的参与被结束了")
	}

	replay, err := handler.Register(t.Context(), late)
	if err != nil || replay.Outcome() != application.HandoverExistingVersion || replay.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("重放：%v %s refusal=%s, want 已有版本、不再碰段", err, replay.Outcome(), replay.SegmentEntryRefusal())
	}
	if _, has := replay.Record(); !has {
		t.Fatal("重放没交回在册那一版")
	}
}
