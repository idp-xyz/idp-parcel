package domain

import (
	"errors"
	"time"
)

// IntakeOutcome is the intake processing result of one preserved submission.
// It is not a business decision about the shipment request.
type IntakeOutcome string

const (
	// IntakeAccepted means a minimal shipment request identity was established.
	IntakeAccepted IntakeOutcome = "ACCEPTED"
	// IntakeNotAdmitted means the raw content could not establish a customer
	// scope, a shipment request boundary or a minimal member identity. No
	// placeholder shipment request is created.
	IntakeNotAdmitted IntakeOutcome = "NOT_ADMITTED"
	// IntakeConflict means the same logical request identity arrived with
	// different content. The original content is preserved untouched.
	IntakeConflict IntakeOutcome = "CONFLICT"
)

// Known reports whether the outcome is part of the published set.
func (o IntakeOutcome) Known() bool {
	switch o {
	case IntakeAccepted, IntakeNotAdmitted, IntakeConflict:
		return true
	default:
		return false
	}
}

// Errors raised while preserving a source submission.
var (
	ErrRawContentMissing      = errors.New("parcelshipment: raw content reference and digest are required")
	ErrSourceTimesMissing     = errors.New("parcelshipment: source occurrence and system receipt times are required")
	ErrIntakeOutcomeUnknown   = errors.New("parcelshipment: unknown intake outcome")
	ErrNotAdmittedNeedsReason = errors.New("parcelshipment: a not admitted submission requires a deterministic reason")
)

// SourceSubmission is the immutable preservation of one raw intake request. It
// is written before any business processing, and later parsing or dependency
// failures never delete it.
type SourceSubmission struct {
	key SourceKey

	rawContentRef string
	payloadDigest PayloadDigest

	occurredAt time.Time
	receivedAt time.Time

	correlationID string

	outcome          IntakeOutcome
	notAdmitReason   string
	parseRuleVersion string

	// submissionBatchID and shipmentRequestIDs are set only when a minimal
	// identity was established.
	submissionBatchID  SubmissionBatchID
	shipmentRequestIDs []ShipmentRequestID
}

// PreserveSource builds the immutable source record. It never inspects or
// normalises the raw content; that is the intake adapter's responsibility.
func PreserveSource(
	key SourceKey,
	rawContentRef string,
	digest PayloadDigest,
	occurredAt time.Time,
	receivedAt time.Time,
	correlationID string,
) (*SourceSubmission, error) {
	if err := key.Validate(); err != nil {
		return nil, err
	}
	if rawContentRef == "" || digest == "" {
		return nil, ErrRawContentMissing
	}
	if occurredAt.IsZero() || receivedAt.IsZero() {
		return nil, ErrSourceTimesMissing
	}

	return &SourceSubmission{
		key:           key,
		rawContentRef: rawContentRef,
		payloadDigest: digest,
		occurredAt:    occurredAt,
		receivedAt:    receivedAt,
		correlationID: correlationID,
	}, nil
}

// RestoreSource rebuilds a persisted submission. Only the persistence adapter
// calls it; it performs no business transition.
func RestoreSource(
	key SourceKey,
	rawContentRef string,
	digest PayloadDigest,
	occurredAt, receivedAt time.Time,
	correlationID string,
	outcome IntakeOutcome,
	notAdmitReason, parseRuleVersion string,
	batchID SubmissionBatchID,
	requestIDs []ShipmentRequestID,
) *SourceSubmission {
	return &SourceSubmission{
		key:                key,
		rawContentRef:      rawContentRef,
		payloadDigest:      digest,
		occurredAt:         occurredAt,
		receivedAt:         receivedAt,
		correlationID:      correlationID,
		outcome:            outcome,
		notAdmitReason:     notAdmitReason,
		parseRuleVersion:   parseRuleVersion,
		submissionBatchID:  batchID,
		shipmentRequestIDs: requestIDs,
	}
}

// RecordNotAdmitted marks that no minimal shipment request identity could be
// established. It records the deterministic reason and the parse rule version;
// it never creates a placeholder shipment request or a rejection decision.
func (s *SourceSubmission) RecordNotAdmitted(reason, parseRuleVersion string) error {
	if reason == "" || parseRuleVersion == "" {
		return ErrNotAdmittedNeedsReason
	}
	s.outcome = IntakeNotAdmitted
	s.notAdmitReason = reason
	s.parseRuleVersion = parseRuleVersion
	return nil
}

// RecordAdmitted links the preserved submission to the batch and the shipment
// requests it produced.
func (s *SourceSubmission) RecordAdmitted(
	batchID SubmissionBatchID,
	requestIDs []ShipmentRequestID,
	parseRuleVersion string,
) error {
	if batchID == "" || len(requestIDs) == 0 {
		return ErrShipmentRequestIDMissing
	}
	if parseRuleVersion == "" {
		return ErrNotAdmittedNeedsReason
	}
	s.outcome = IntakeAccepted
	s.submissionBatchID = batchID
	s.shipmentRequestIDs = append([]ShipmentRequestID(nil), requestIDs...)
	s.parseRuleVersion = parseRuleVersion
	return nil
}

// SameContent reports whether a repeated request carries the identical digest.
// A matching digest returns the existing result; a differing digest is an
// intake conflict that must not overwrite the preserved content.
func (s *SourceSubmission) SameContent(digest PayloadDigest) bool {
	return s.payloadDigest == digest
}

// Accessors. The aggregate exposes values rather than its fields so a caller
// cannot mutate a preserved fact.

func (s *SourceSubmission) Key() SourceKey                       { return s.key }
func (s *SourceSubmission) RawContentRef() string                { return s.rawContentRef }
func (s *SourceSubmission) PayloadDigest() PayloadDigest         { return s.payloadDigest }
func (s *SourceSubmission) OccurredAt() time.Time                { return s.occurredAt }
func (s *SourceSubmission) ReceivedAt() time.Time                { return s.receivedAt }
func (s *SourceSubmission) CorrelationID() string                { return s.correlationID }
func (s *SourceSubmission) Outcome() IntakeOutcome               { return s.outcome }
func (s *SourceSubmission) NotAdmittedReason() string            { return s.notAdmitReason }
func (s *SourceSubmission) ParseRuleVersion() string             { return s.parseRuleVersion }
func (s *SourceSubmission) SubmissionBatchID() SubmissionBatchID { return s.submissionBatchID }

// ShipmentRequestIDs returns a copy so a caller cannot extend the preserved link.
func (s *SourceSubmission) ShipmentRequestIDs() []ShipmentRequestID {
	return append([]ShipmentRequestID(nil), s.shipmentRequestIDs...)
}
