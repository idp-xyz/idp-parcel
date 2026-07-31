package domain

import (
	"errors"
	"time"
)

// RequestOutcome is the per request result inside a submission batch. This
// slice only ever records Submitted or NotAdmitted; Accepted and Rejected
// belong to a later slice.
type RequestOutcome string

const (
	// OutcomeSubmitted means the request was recorded and no business decision
	// has been made yet.
	OutcomeSubmitted RequestOutcome = "SUBMITTED"
	// OutcomeNotAdmitted means no minimal identity could be established, so no
	// shipment request exists.
	OutcomeNotAdmitted RequestOutcome = "NOT_ADMITTED"
)

// ErrBatchIncomplete reports a batch result that cannot be attributed.
var ErrBatchIncomplete = errors.New("parcelshipment: batch result requires a scope, a batch id and a recording time")

// BatchRequestRef links one batch to one shipment request result. It holds a
// reference and an outcome, never the request content.
type BatchRequestRef struct {
	ShipmentRequestID ShipmentRequestID
	Outcome           RequestOutcome
	Reason            string
}

// BatchResult groups the per request results of one intake call. A batch is a
// processing grouping: one request failing never rolls back a sibling request
// that legitimately reached Submitted.
type BatchResult struct {
	Scope      Scope
	BatchID    SubmissionBatchID
	SourceKey  SourceKey
	Requests   []BatchRequestRef
	RecordedAt time.Time
}

// Validate rejects an unattributable batch result.
func (b BatchResult) Validate() error {
	if err := b.Scope.Validate(); err != nil {
		return err
	}
	if b.BatchID == "" || b.RecordedAt.IsZero() {
		return ErrBatchIncomplete
	}
	for _, ref := range b.Requests {
		switch ref.Outcome {
		case OutcomeSubmitted:
			if ref.ShipmentRequestID == "" {
				return ErrShipmentRequestIDMissing
			}
		case OutcomeNotAdmitted:
			if ref.Reason == "" {
				return ErrNotAdmittedNeedsReason
			}
		default:
			return ErrBatchIncomplete
		}
	}
	return nil
}
