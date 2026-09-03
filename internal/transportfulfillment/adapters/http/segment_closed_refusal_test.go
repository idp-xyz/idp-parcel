package tfhttp_test

import (
	"net/http"
	"testing"
	"time"
)

// Covers: 票 tf-segment-lifecycle-closure/03 裁决 3 的传输面——「段已关闭」单开一格答给调用方
// （`segmentEntryRefusal: "SEGMENT_CLOSED"`，含义：另立新段），三条控制事实入口都透出；它不是欠账，
// 所以 `segmentContinuationReference` / `segmentEntries` 同时为空；来源保全照常 201。
//
// 这一格被传输层吞掉，调用方看到的又是「交接在册、段里没它」——与正常入段同形，正是这一格要治的病。
func TestArrivingAtAClosedSegmentIsAnsweredOverHTTP(t *testing.T) {
	closedAt := time.Date(2026, 9, 5, 11, 0, 0, 0, time.UTC)

	t.Run("/transport-fulfillment/handovers", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		first := defaultHandoverBody()
		first.Segment = "segment-1"
		if seed := postTo(t, fixture.register, "/transport-fulfillment/handovers", first); seed.Code != http.StatusCreated {
			t.Fatalf("seed status = %d body = %s", seed.Code, seed.Body.String())
		}
		fixture.segments.closeSegment(t, "tenant-1", "segment-1", closedAt)

		late := defaultHandoverBody()
		late.Object = "parcel-2"
		late.Version = "handover-result/parcel-2/v1"
		late.JudgedAt = "2026-09-05T12:00:00Z"
		late.Segment = "segment-1"
		response := postTo(t, fixture.register, "/transport-fulfillment/handovers", late)

		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201（交接本身登上了）; body = %s", response.Code, response.Body.String())
		}
		view := decodeHandover(t, response)
		if view.Outcome != "HANDOVER_REGISTERED" || view.SegmentEntryRefusal != "SEGMENT_CLOSED" {
			t.Fatalf("view = %+v，want segmentEntryRefusal SEGMENT_CLOSED", view)
		}
		if view.SegmentContinuationReference != "" {
			t.Fatalf("段已关闭不是欠账，却带了 segmentContinuationReference %q", view.SegmentContinuationReference)
		}
	})

	t.Run("/transport-fulfillment/offsite-pickups", func(t *testing.T) {
		fixture := newPickupRegistrationFixture(t)
		first := defaultPickupRegistrationBody()
		first.Segment = "segment-1"
		if seed := postTo(t, fixture.register, "/transport-fulfillment/offsite-pickups", first); seed.Code != http.StatusCreated {
			t.Fatalf("seed status = %d body = %s", seed.Code, seed.Body.String())
		}
		fixture.segments.closeSegment(t, "tenant-1", "segment-1", closedAt)

		late := defaultPickupRegistrationBody()
		late.Object = "parcel-2"
		late.OccurredAt = "2026-09-05T12:00:00Z"
		late.Segment = "segment-1"
		response := postTo(t, fixture.register, "/transport-fulfillment/offsite-pickups", late)

		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
		}
		view := decodePickupRegistration(t, response)
		if view.Outcome != "PICKUP_REGISTERED" || view.SegmentEntryRefusal != "SEGMENT_CLOSED" || view.SegmentContinuationReference != "" {
			t.Fatalf("view = %+v，want segmentEntryRefusal SEGMENT_CLOSED 且无欠账", view)
		}
	})

	t.Run("/transport-fulfillment/offsite-pickup-attempts", func(t *testing.T) {
		fixture := newPickupAttemptFixture(t)
		first := defaultPickupAttemptBody(pickedUp("parcel-1", "TRANSPORT-CONTROL/TF-1", ""))
		first.Segment = "segment-1"
		if seed := postTo(t, fixture.perform, "/transport-fulfillment/offsite-pickup-attempts", first); seed.Code != http.StatusCreated {
			t.Fatalf("seed status = %d body = %s", seed.Code, seed.Body.String())
		}
		fixture.segments.closeSegment(t, "tenant-1", "segment-1", closedAt)

		late := defaultPickupAttemptBody(
			pickedUp("parcel-2", "TRANSPORT-CONTROL/TF-2", ""),
			pickedUp("parcel-3", "TRANSPORT-CONTROL/TF-3", ""),
		)
		late.SourceID = "source-2"
		late.Attempt = "attempt-2"
		late.ArrivedAt = "2026-09-05T12:00:00Z"
		late.PlannedTo = "2026-09-05T14:00:00Z"
		for index := range late.Objects {
			late.Objects[index].OccurredAt = "2026-09-05T12:05:00Z"
		}
		late.Segment = "segment-1"
		response := postTo(t, fixture.perform, "/transport-fulfillment/offsite-pickup-attempts", late)

		if response.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
		}
		view := decodePickupAttempt(t, response)
		if view.Outcome != "ATTEMPT_RECORDED" || view.SegmentEntryRefusal != "SEGMENT_CLOSED" {
			t.Fatalf("view = %+v，want segmentEntryRefusal SEGMENT_CLOSED", view)
		}
		if len(view.SegmentEntries) != 0 {
			t.Fatalf("段已关闭不是欠账，却报了逐对象欠账：%+v", view.SegmentEntries)
		}
	})
}
