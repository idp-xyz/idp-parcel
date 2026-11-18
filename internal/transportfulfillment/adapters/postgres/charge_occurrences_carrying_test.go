package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

func TestListCurrentCarryingKeepsOnlyTheVersionNothingCorrects(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()
	first := occurrenceRecord(t, "SYN-OCC-CARRY", "v1", "SYN-PARCEL-1")
	mustSaveOccurrence(t, transactor, ctx, repository, first)
	invalidated, err := first.Occurrence.ReviseValidity(
		domain.OccurrenceInvalidated,
		occurrenceRef(t, domain.NewOccurrenceValidityVersion, "v2"),
		occurrenceRef(t, domain.NewOccurrenceBasisReference, "CORRECTION/SYN-SRC-2"),
		occurredAtFixture.Add(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	mustSaveOccurrence(t, transactor, ctx, repository, ports.ChargeOccurrenceRecord{
		Key: occurrenceKey(t, "tenant-1", "SYN-OCC-CARRY", "v2"), Occurrence: invalidated, RecordedAt: occurredAtFixture,
	})
	still := occurrenceRecord(t, "SYN-OCC-OPEN", "v1", "SYN-PARCEL-1")
	mustSaveOccurrence(t, transactor, ctx, repository, still)
	other := occurrenceRecord(t, "SYN-OCC-OTHER", "v1", "SYN-PARCEL-2")
	mustSaveOccurrence(t, transactor, ctx, repository, other)

	listed, err := repository.ListCurrentCarrying(ctx, first.Key.TenantID, occurrenceRef(t, domain.NewCarriedObjectReference, "SYN-PARCEL-1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Occurrence.String() != "SYN-OCC-OPEN" || listed[0].Validity.String() != "v1" {
		t.Fatalf("仍有效 %v", listed)
	}

	reestablished, err := invalidated.ReviseValidity(
		domain.OccurrenceSuperseded,
		occurrenceRef(t, domain.NewOccurrenceValidityVersion, "v3"),
		occurrenceRef(t, domain.NewOccurrenceBasisReference, "CORRECTION/SYN-SRC-3"),
		occurredAtFixture.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	mustSaveOccurrence(t, transactor, ctx, repository, ports.ChargeOccurrenceRecord{
		Key: occurrenceKey(t, "tenant-1", "SYN-OCC-CARRY", "v3"), Occurrence: reestablished, RecordedAt: occurredAtFixture,
	})
	listed, err = repository.ListCurrentCarrying(ctx, first.Key.TenantID, occurrenceRef(t, domain.NewCarriedObjectReference, "SYN-PARCEL-1"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, key := range listed {
		got[key.Occurrence.String()] = key.Validity.String()
	}
	if got["SYN-OCC-OPEN"] != "v1" || got["SYN-OCC-CARRY"] != "v3" || len(got) != 2 {
		t.Fatalf("替代之后 %v", got)
	}
}
