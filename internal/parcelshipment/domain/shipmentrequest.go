package domain

import (
	"errors"
	"time"

	bentodomain "go.idp.xyz/idp-bento-go/domain"
)

// LifecycleState is the shipment request lifecycle. This slice only ever
// produces Submitted: acceptance and rejection belong to a later slice and must
// never be produced by unimplemented code.
type LifecycleState string

const (
	// StateSubmitted means the customer confirmed the request and Parcel
	// recorded it. It does not mean the operator accepted it.
	StateSubmitted LifecycleState = "SUBMITTED"
)

// Errors raised while building a shipment request.
var (
	ErrNoDeclaredParcel        = errors.New("parcelshipment: a shipment request requires at least one declared parcel")
	ErrDuplicateDeclaredParcel = errors.New("parcelshipment: declared parcel declared twice in one shipment request")
	ErrSubmissionBatchMissing  = errors.New("parcelshipment: submission batch is required")
	ErrSubmissionTimeMissing   = errors.New("parcelshipment: submitted at is required")
)

// DeclaredParcel is one customer declared member of a shipment request. This
// slice records its identity and customer side reference only; measurement,
// goods and declaration content belong to a later slice.
type DeclaredParcel struct {
	ID                DeclaredParcelID
	CustomerReference string
}

// SubmissionVersion is the monotonic version of the customer declared content
// of one shipment request. A controlled correction creates a new version; it
// never overwrites the previous one.
type SubmissionVersion uint32

// ShipmentRequest is the shipment request aggregate. In this slice it owns its
// identity, scope, submission version, declared parcels and the Submitted
// state; it owns no acceptance baseline, expected commitment or financial fact.
type ShipmentRequest struct {
	key ShipmentRequestKey

	submissionBatchID SubmissionBatchID
	sourceKey         SourceKey

	state           LifecycleState
	version         SubmissionVersion
	declaredParcels []DeclaredParcel

	sourceOccurredAt time.Time
	submittedAt      time.Time

	// eventID is generated once and reused by every repeat of the same logical
	// request, so a retry never produces a second semantically equal event.
	eventID string

	revision int64
	events   bentodomain.EventBuffer
}

// SubmitShipmentRequest creates a shipment request in Submitted state and
// records the domain fact. It never concludes acceptance or rejection.
func SubmitShipmentRequest(
	key ShipmentRequestKey,
	batchID SubmissionBatchID,
	sourceKey SourceKey,
	parcels []DeclaredParcel,
	eventID string,
	sourceOccurredAt time.Time,
	submittedAt time.Time,
) (*ShipmentRequest, error) {
	if err := key.Validate(); err != nil {
		return nil, err
	}
	if err := sourceKey.Validate(); err != nil {
		return nil, err
	}
	if batchID == "" {
		return nil, ErrSubmissionBatchMissing
	}
	if submittedAt.IsZero() || sourceOccurredAt.IsZero() {
		return nil, ErrSubmissionTimeMissing
	}
	if eventID == "" {
		return nil, ErrEventIDMissing
	}

	members, err := validateParcels(parcels)
	if err != nil {
		return nil, err
	}

	request := &ShipmentRequest{
		key:               key,
		submissionBatchID: batchID,
		sourceKey:         sourceKey,
		state:             StateSubmitted,
		version:           1,
		declaredParcels:   members,
		sourceOccurredAt:  sourceOccurredAt,
		submittedAt:       submittedAt,
		eventID:           eventID,
		revision:          0,
	}
	request.events.Record(ShipmentRequestSubmitted{
		Key:               key,
		SubmissionBatchID: batchID,
		SubmissionVersion: request.version,
		DeclaredParcelIDs: request.DeclaredParcelIDs(),
		EventID:           eventID,
		At:                sourceOccurredAt,
	})
	return request, nil
}

// ErrEventIDMissing reports a submission without a stable event identity.
var ErrEventIDMissing = errors.New("parcelshipment: event id is required")

// RestoreShipmentRequest rebuilds a persisted aggregate without recording an
// event. Only the persistence adapter calls it.
func RestoreShipmentRequest(
	key ShipmentRequestKey,
	batchID SubmissionBatchID,
	sourceKey SourceKey,
	state LifecycleState,
	version SubmissionVersion,
	parcels []DeclaredParcel,
	eventID string,
	sourceOccurredAt, submittedAt time.Time,
	revision int64,
) *ShipmentRequest {
	return &ShipmentRequest{
		key:               key,
		submissionBatchID: batchID,
		sourceKey:         sourceKey,
		state:             state,
		version:           version,
		declaredParcels:   append([]DeclaredParcel(nil), parcels...),
		eventID:           eventID,
		sourceOccurredAt:  sourceOccurredAt,
		submittedAt:       submittedAt,
		revision:          revision,
	}
}

func validateParcels(parcels []DeclaredParcel) ([]DeclaredParcel, error) {
	if len(parcels) == 0 {
		return nil, ErrNoDeclaredParcel
	}

	seen := make(map[DeclaredParcelID]struct{}, len(parcels))
	members := make([]DeclaredParcel, 0, len(parcels))
	for _, parcel := range parcels {
		if parcel.ID == "" {
			return nil, ErrNoDeclaredParcel
		}
		if _, duplicate := seen[parcel.ID]; duplicate {
			return nil, ErrDuplicateDeclaredParcel
		}
		seen[parcel.ID] = struct{}{}
		members = append(members, parcel)
	}
	return members, nil
}

// PartitionKey is the stable composite key that keeps the events of one
// shipment request ordered relative to each other.
func (r *ShipmentRequest) PartitionKey() string {
	return r.key.Scope.String() + "/" + string(r.key.ShipmentRequestID)
}

// Accessors.

func (r *ShipmentRequest) Key() ShipmentRequestKey              { return r.key }
func (r *ShipmentRequest) SubmissionBatchID() SubmissionBatchID { return r.submissionBatchID }
func (r *ShipmentRequest) SourceKey() SourceKey                 { return r.sourceKey }
func (r *ShipmentRequest) State() LifecycleState                { return r.state }
func (r *ShipmentRequest) SubmissionVersion() SubmissionVersion { return r.version }
func (r *ShipmentRequest) EventID() string                      { return r.eventID }
func (r *ShipmentRequest) SourceOccurredAt() time.Time          { return r.sourceOccurredAt }
func (r *ShipmentRequest) SubmittedAt() time.Time               { return r.submittedAt }
func (r *ShipmentRequest) Revision() int64                      { return r.revision }

// DeclaredParcels returns a copy of the declared members.
func (r *ShipmentRequest) DeclaredParcels() []DeclaredParcel {
	return append([]DeclaredParcel(nil), r.declaredParcels...)
}

// DeclaredParcelIDs returns the member identities in declaration order.
func (r *ShipmentRequest) DeclaredParcelIDs() []DeclaredParcelID {
	ids := make([]DeclaredParcelID, 0, len(r.declaredParcels))
	for _, parcel := range r.declaredParcels {
		ids = append(ids, parcel.ID)
	}
	return ids
}

// Events exposes the buffer so the application can map recorded facts to
// integration events inside the same transaction.
func (r *ShipmentRequest) Events() *bentodomain.EventBuffer { return &r.events }
