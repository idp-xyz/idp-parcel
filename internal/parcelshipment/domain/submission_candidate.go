package domain

import "errors"

var (
	ErrInvalidSubmissionCandidate = errors.New("parcel shipment: invalid submission candidate")
	ErrNoDeclaredParcels          = errors.New("parcel shipment: no declared parcels")
	ErrDuplicateDeclaredParcel    = errors.New("parcel shipment: duplicate declared parcel")
)

type SubmissionCandidate struct {
	sourceSubmission  SourceSubmissionFingerprint
	batchID           SubmissionBatchID
	shipmentRequestID ShipmentRequestID
	declaredParcelIDs []DeclaredParcelID
}

func NewSubmissionCandidate(
	sourceSubmission SourceSubmissionFingerprint,
	batchID SubmissionBatchID,
	shipmentRequestID ShipmentRequestID,
	declaredParcelIDs []DeclaredParcelID,
) (SubmissionCandidate, error) {
	if !sourceSubmission.valid() || !batchID.valid() || !shipmentRequestID.valid() {
		return SubmissionCandidate{}, ErrInvalidSubmissionCandidate
	}
	if len(declaredParcelIDs) == 0 {
		return SubmissionCandidate{}, ErrNoDeclaredParcels
	}

	parcels := make([]DeclaredParcelID, len(declaredParcelIDs))
	seen := make(map[DeclaredParcelID]struct{}, len(declaredParcelIDs))
	for index, parcelID := range declaredParcelIDs {
		if !parcelID.valid() {
			return SubmissionCandidate{}, ErrInvalidSubmissionCandidate
		}
		if _, exists := seen[parcelID]; exists {
			return SubmissionCandidate{}, ErrDuplicateDeclaredParcel
		}
		seen[parcelID] = struct{}{}
		parcels[index] = parcelID
	}

	return SubmissionCandidate{
		sourceSubmission:  sourceSubmission,
		batchID:           batchID,
		shipmentRequestID: shipmentRequestID,
		declaredParcelIDs: parcels,
	}, nil
}

func (candidate SubmissionCandidate) SourceSubmission() SourceSubmissionFingerprint {
	return candidate.sourceSubmission
}

func (candidate SubmissionCandidate) BatchID() SubmissionBatchID {
	return candidate.batchID
}

func (candidate SubmissionCandidate) ShipmentRequestID() ShipmentRequestID {
	return candidate.shipmentRequestID
}

func (candidate SubmissionCandidate) DeclaredParcelIDs() []DeclaredParcelID {
	return append([]DeclaredParcelID(nil), candidate.declaredParcelIDs...)
}

func (candidate SubmissionCandidate) valid() bool {
	if !candidate.sourceSubmission.valid() ||
		!candidate.batchID.valid() ||
		!candidate.shipmentRequestID.valid() ||
		len(candidate.declaredParcelIDs) == 0 {
		return false
	}

	seen := make(map[DeclaredParcelID]struct{}, len(candidate.declaredParcelIDs))
	for _, parcelID := range candidate.declaredParcelIDs {
		if !parcelID.valid() {
			return false
		}
		if _, exists := seen[parcelID]; exists {
			return false
		}
		seen[parcelID] = struct{}{}
	}
	return true
}
