package domain

import "errors"

var (
	ErrInvalidSubmissionBatchCandidate = errors.New("parcel shipment: invalid submission batch candidate")
	ErrNoSubmissionCandidates          = errors.New("parcel shipment: no submission candidates")
	ErrMixedTenantSubmissionBatch      = errors.New("parcel shipment: mixed tenant submission batch")
	ErrSubmissionBatchMismatch         = errors.New("parcel shipment: submission batch mismatch")
	ErrDuplicateShipmentCandidate      = errors.New("parcel shipment: duplicate shipment candidate")
)

type SubmissionBatchCandidate struct {
	tenantID   TenantID
	batchID    SubmissionBatchID
	candidates []SubmissionCandidate
}

type shipmentCandidateKey struct {
	customerAccountID CustomerAccountID
	shipmentRequestID ShipmentRequestID
}

func NewSubmissionBatchCandidate(
	tenantID TenantID,
	batchID SubmissionBatchID,
	candidates []SubmissionCandidate,
) (SubmissionBatchCandidate, error) {
	if !tenantID.valid() || !batchID.valid() {
		return SubmissionBatchCandidate{}, ErrInvalidSubmissionBatchCandidate
	}
	if len(candidates) == 0 {
		return SubmissionBatchCandidate{}, ErrNoSubmissionCandidates
	}

	batchCandidates := make([]SubmissionCandidate, len(candidates))
	seen := make(map[shipmentCandidateKey]struct{}, len(candidates))
	for index, candidate := range candidates {
		if !candidate.valid() {
			return SubmissionBatchCandidate{}, ErrInvalidSubmissionBatchCandidate
		}

		identity := candidate.SourceSubmission().Identity()
		if identity.TenantID() != tenantID {
			return SubmissionBatchCandidate{}, ErrMixedTenantSubmissionBatch
		}
		if candidate.BatchID() != batchID {
			return SubmissionBatchCandidate{}, ErrSubmissionBatchMismatch
		}

		key := shipmentCandidateKey{
			customerAccountID: identity.CustomerAccountID(),
			shipmentRequestID: candidate.ShipmentRequestID(),
		}
		if _, exists := seen[key]; exists {
			return SubmissionBatchCandidate{}, ErrDuplicateShipmentCandidate
		}
		seen[key] = struct{}{}
		batchCandidates[index] = candidate
	}

	return SubmissionBatchCandidate{
		tenantID:   tenantID,
		batchID:    batchID,
		candidates: batchCandidates,
	}, nil
}

func (batch SubmissionBatchCandidate) TenantID() TenantID {
	return batch.tenantID
}

func (batch SubmissionBatchCandidate) BatchID() SubmissionBatchID {
	return batch.batchID
}

func (batch SubmissionBatchCandidate) Candidates() []SubmissionCandidate {
	return append([]SubmissionCandidate(nil), batch.candidates...)
}
