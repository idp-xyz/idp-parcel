package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func TestRateTableUsesLeftClosedRightOpenIntervals(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	first, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "r-1"), "Z1",
		weight(t, "0", domain.WeightUnitKilogram), weight(t, "1", domain.WeightUnitKilogram), money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("first entry: %v", err)
	}
	second, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "r-2"), "Z1",
		weight(t, "1", domain.WeightUnitKilogram), weight(t, "2", domain.WeightUnitKilogram), money(t, "20", currency),
	)
	if err != nil {
		t.Fatalf("second entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-boundary", "v1"),
		domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{second, first},
	)
	if err != nil {
		t.Fatalf("table: %v", err)
	}
	for _, test := range []struct {
		weight string
		wantID string
	}{
		{"0", "r-1"},
		{"0.999", "r-1"},
		{"1", "r-2"},
	} {
		entry, lookupErr := table.Lookup("Z1", weight(t, test.weight, domain.WeightUnitKilogram))
		if lookupErr != nil {
			t.Fatalf("lookup %s: %v", test.weight, lookupErr)
		}
		if entry.ID().String() != test.wantID {
			t.Fatalf("lookup %s = %s, want %s", test.weight, entry.ID(), test.wantID)
		}
	}
	if _, err := table.Lookup("Z1", weight(t, "2", domain.WeightUnitKilogram)); !errors.Is(err, domain.ErrNoMatchingRate) {
		t.Fatalf("upper boundary error = %v", err)
	}
}

func TestRateTableRejectsOverlapButAllowsGapsAndZoneIsolation(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry := func(id, zone, minimum, maximum string) domain.RateEntry {
		t.Helper()
		result, err := domain.NewRateEntry(
			mustValue(t, domain.NewRateEntryID, id), zone,
			weight(t, minimum, domain.WeightUnitKilogram), weight(t, maximum, domain.WeightUnitKilogram), money(t, "10", currency),
		)
		if err != nil {
			t.Fatalf("entry %s: %v", id, err)
		}
		return result
	}
	_, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-overlap", "v1"),
		domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{entry("r-1", "Z1", "0", "2"), entry("r-2", "Z1", "1", "3")},
	)
	if !errors.Is(err, domain.ErrRateIntervalOverlap) {
		t.Fatalf("overlap error = %v", err)
	}

	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-gap", "v1"),
		domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{entry("r-1", "Z1", "0", "1"), entry("r-2", "Z1", "2", "3"), entry("r-3", "Z2", "0", "3")},
	)
	if err != nil {
		t.Fatalf("gap table: %v", err)
	}
	if _, err := table.Lookup("Z1", weight(t, "1.5", domain.WeightUnitKilogram)); !errors.Is(err, domain.ErrNoMatchingRate) {
		t.Fatalf("gap lookup error = %v", err)
	}
	if _, err := table.Lookup("Z2", weight(t, "1.5", domain.WeightUnitKilogram)); err != nil {
		t.Fatalf("zone isolation lookup: %v", err)
	}
}

func TestRateTableCopiesEntrySlices(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "r-1"), "Z1",
		weight(t, "0", domain.WeightUnitKilogram), weight(t, "1", domain.WeightUnitKilogram), money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	entries := []domain.RateEntry{entry}
	table, err := domain.NewRateTableVersion(versionReference(t, domain.ArtifactRateTable, "table-copy", "v1"), domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram, effectivePeriod(t), entries)
	if err != nil {
		t.Fatalf("table: %v", err)
	}
	entries[0] = domain.RateEntry{}
	returned := table.Entries()
	returned[0] = domain.RateEntry{}
	if _, err := table.Lookup("Z1", weight(t, "0.5", domain.WeightUnitKilogram)); err != nil {
		t.Fatalf("table changed through slice: %v", err)
	}
}

func TestRateTableSupportsOnlyTerminalOpenEndedInterval(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	finite, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "r-finite"), "Z1",
		weight(t, "0", domain.WeightUnitKilogram), weight(t, "10", domain.WeightUnitKilogram), money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("finite entry: %v", err)
	}
	open, err := domain.NewOpenEndedRateEntry(
		mustValue(t, domain.NewRateEntryID, "r-open"), "Z1",
		weight(t, "10", domain.WeightUnitKilogram), money(t, "20", currency),
	)
	if err != nil {
		t.Fatalf("open entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-open", "v1"), domain.RateTableFamilyWeightZone,
		currency, domain.WeightUnitKilogram, effectivePeriod(t), []domain.RateEntry{open, finite},
	)
	if err != nil {
		t.Fatalf("open-ended table: %v", err)
	}
	matched, err := table.Lookup("Z1", weight(t, "1000000", domain.WeightUnitKilogram))
	if err != nil || matched.ID().String() != "r-open" {
		t.Fatalf("open-ended lookup = %s, %v", matched.ID(), err)
	}
	if _, hasMaximum := open.Maximum(); hasMaximum {
		t.Fatal("open-ended entry unexpectedly has a maximum")
	}

	lateFinite, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "r-late"), "Z1",
		weight(t, "20", domain.WeightUnitKilogram), weight(t, "30", domain.WeightUnitKilogram), money(t, "30", currency),
	)
	if err != nil {
		t.Fatalf("late entry: %v", err)
	}
	if _, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-open-overlap", "v1"), domain.RateTableFamilyWeightZone,
		currency, domain.WeightUnitKilogram, effectivePeriod(t), []domain.RateEntry{open, lateFinite},
	); !errors.Is(err, domain.ErrRateIntervalOverlap) {
		t.Fatalf("open-ended overlap error = %v", err)
	}
}
