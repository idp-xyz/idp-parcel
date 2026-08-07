package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func TestSubmissionCandidatePreservesMemberOrderAndCopiesSlices(t *testing.T) {
	source := sourceFingerprint(t, "tenant-1", "customer-1", "api", "request-1", "digest-1")
	batchID := mustValue(t, domain.NewSubmissionBatchID, "batch-1")
	requestID := mustValue(t, domain.NewShipmentRequestID, "shipment-1")
	parcel1 := mustValue(t, domain.NewDeclaredParcelID, "parcel-1")
	parcel2 := mustValue(t, domain.NewDeclaredParcelID, "parcel-2")
	input := []domain.DeclaredParcelID{parcel2, parcel1}

	candidate, err := domain.NewSubmissionCandidate(source, batchID, requestID, input)
	if err != nil {
		t.Fatalf("new submission candidate: %v", err)
	}
	input[0] = parcel1
	got := candidate.DeclaredParcelIDs()
	if got[0] != parcel2 || got[1] != parcel1 {
		t.Fatalf("member order = %v", stringValues(got))
	}
	got[0] = parcel1
	if candidate.DeclaredParcelIDs()[0] != parcel2 {
		t.Fatal("candidate changed through returned slice")
	}
}

func TestSubmissionCandidateRejectsInvalidMinimumIdentity(t *testing.T) {
	source := sourceFingerprint(t, "tenant-1", "customer-1", "api", "request-1", "digest-1")
	batchID := mustValue(t, domain.NewSubmissionBatchID, "batch-1")
	requestID := mustValue(t, domain.NewShipmentRequestID, "shipment-1")
	parcelID := mustValue(t, domain.NewDeclaredParcelID, "parcel-1")

	tests := []struct {
		name      string
		source    domain.SourceSubmissionFingerprint
		batchID   domain.SubmissionBatchID
		requestID domain.ShipmentRequestID
		parcels   []domain.DeclaredParcelID
		want      error
	}{
		{"missing source", domain.SourceSubmissionFingerprint{}, batchID, requestID, []domain.DeclaredParcelID{parcelID}, domain.ErrInvalidSubmissionCandidate},
		{"missing batch", source, domain.SubmissionBatchID{}, requestID, []domain.DeclaredParcelID{parcelID}, domain.ErrInvalidSubmissionCandidate},
		{"missing request", source, batchID, domain.ShipmentRequestID{}, []domain.DeclaredParcelID{parcelID}, domain.ErrInvalidSubmissionCandidate},
		{"no parcels", source, batchID, requestID, nil, domain.ErrNoDeclaredParcels},
		{"blank parcel", source, batchID, requestID, []domain.DeclaredParcelID{{}}, domain.ErrInvalidSubmissionCandidate},
		{"duplicate parcel", source, batchID, requestID, []domain.DeclaredParcelID{parcelID, parcelID}, domain.ErrDuplicateDeclaredParcel},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := domain.NewSubmissionCandidate(test.source, test.batchID, test.requestID, test.parcels)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}
