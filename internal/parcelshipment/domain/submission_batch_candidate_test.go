package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func TestSubmissionBatchCandidatePreservesOrderAndCopiesSlices(t *testing.T) {
	tenantID := mustValue(t, domain.NewTenantID, "tenant-1")
	batchID := mustValue(t, domain.NewSubmissionBatchID, "batch-1")
	first := submissionCandidate(t, "tenant-1", "customer-1", "api", "source-1", "batch-1", "shipment-1")
	second := submissionCandidate(t, "tenant-1", "customer-2", "file", "source-2", "batch-1", "shipment-2")
	input := []domain.SubmissionCandidate{first, second}

	batch, err := domain.NewSubmissionBatchCandidate(tenantID, batchID, input)
	if err != nil {
		t.Fatalf("new submission batch candidate: %v", err)
	}
	input[0] = second
	got := batch.Candidates()
	if got[0].ShipmentRequestID() != first.ShipmentRequestID() || got[1].ShipmentRequestID() != second.ShipmentRequestID() {
		t.Fatal("candidate order changed")
	}
	got[0] = second
	if batch.Candidates()[0].ShipmentRequestID() != first.ShipmentRequestID() {
		t.Fatal("batch changed through returned slice")
	}
	if batch.TenantID() != tenantID || batch.BatchID() != batchID {
		t.Fatal("batch identity changed")
	}
}

func TestSubmissionBatchCandidateRejectsInvalidBoundaries(t *testing.T) {
	tenantID := mustValue(t, domain.NewTenantID, "tenant-1")
	batchID := mustValue(t, domain.NewSubmissionBatchID, "batch-1")
	valid := submissionCandidate(t, "tenant-1", "customer-1", "api", "source-1", "batch-1", "shipment-1")

	tests := []struct {
		name       string
		tenantID   domain.TenantID
		batchID    domain.SubmissionBatchID
		candidates []domain.SubmissionCandidate
		want       error
	}{
		{"missing tenant", domain.TenantID{}, batchID, []domain.SubmissionCandidate{valid}, domain.ErrInvalidSubmissionBatchCandidate},
		{"missing batch", tenantID, domain.SubmissionBatchID{}, []domain.SubmissionCandidate{valid}, domain.ErrInvalidSubmissionBatchCandidate},
		{"no candidates", tenantID, batchID, nil, domain.ErrNoSubmissionCandidates},
		{"invalid candidate", tenantID, batchID, []domain.SubmissionCandidate{{}}, domain.ErrInvalidSubmissionBatchCandidate},
		{
			"mixed tenant",
			tenantID,
			batchID,
			[]domain.SubmissionCandidate{submissionCandidate(t, "tenant-2", "customer-1", "api", "source-1", "batch-1", "shipment-1")},
			domain.ErrMixedTenantSubmissionBatch,
		},
		{
			"mismatched batch",
			tenantID,
			batchID,
			[]domain.SubmissionCandidate{submissionCandidate(t, "tenant-1", "customer-1", "api", "source-1", "batch-2", "shipment-1")},
			domain.ErrSubmissionBatchMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := domain.NewSubmissionBatchCandidate(test.tenantID, test.batchID, test.candidates)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSubmissionBatchCandidateUsesBusinessIdentityForDuplicates(t *testing.T) {
	tenantID := mustValue(t, domain.NewTenantID, "tenant-1")
	batchID := mustValue(t, domain.NewSubmissionBatchID, "batch-1")

	t.Run("same customer and shipment from different sources is duplicate", func(t *testing.T) {
		first := submissionCandidate(t, "tenant-1", "customer-1", "api", "source-1", "batch-1", "shipment-1")
		second := submissionCandidate(t, "tenant-1", "customer-1", "file", "source-2", "batch-1", "shipment-1")

		_, err := domain.NewSubmissionBatchCandidate(tenantID, batchID, []domain.SubmissionCandidate{first, second})
		if !errors.Is(err, domain.ErrDuplicateShipmentCandidate) {
			t.Fatalf("error = %v, want duplicate shipment candidate", err)
		}
	})

	t.Run("same shipment ID in another customer account remains isolated", func(t *testing.T) {
		first := submissionCandidate(t, "tenant-1", "customer-1", "api", "source-1", "batch-1", "shipment-1")
		second := submissionCandidate(t, "tenant-1", "customer-2", "api", "source-2", "batch-1", "shipment-1")

		_, err := domain.NewSubmissionBatchCandidate(tenantID, batchID, []domain.SubmissionCandidate{first, second})
		if err != nil {
			t.Fatalf("new submission batch candidate: %v", err)
		}
	})
}

func submissionCandidate(
	t *testing.T,
	tenant string,
	customer string,
	source string,
	requestKey string,
	batch string,
	shipment string,
) domain.SubmissionCandidate {
	t.Helper()
	fingerprint := sourceFingerprint(t, tenant, customer, source, requestKey, "digest-1")
	batchID := mustValue(t, domain.NewSubmissionBatchID, batch)
	shipmentID := mustValue(t, domain.NewShipmentRequestID, shipment)
	parcelID := mustValue(t, domain.NewDeclaredParcelID, shipment+"-parcel-1")
	candidate, err := domain.NewSubmissionCandidate(fingerprint, batchID, shipmentID, []domain.DeclaredParcelID{parcelID})
	if err != nil {
		t.Fatalf("new submission candidate: %v", err)
	}
	return candidate
}
