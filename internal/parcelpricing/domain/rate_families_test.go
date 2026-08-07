package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PA-PP-01: a commercial express agent prices a first weight and then charges
// per continuation step. The step is charged whole, so 0.6 kg against a 0.5 kg
// first weight and a 0.5 kg step costs one step, not 0.2 of one.
func TestFirstContinueChargesTheFirstWeightThenWholeSteps(t *testing.T) {
	table := firstContinueTable(t)
	for _, testCase := range []struct {
		weight string
		want   string
	}{
		{"0.4", "30"},
		{"0.5", "30"},
		{"0.6", "38"},
		{"1", "38"},
		{"1.1", "46"},
	} {
		selection, err := table.Lookup("Z1", weight(t, testCase.weight, domain.WeightUnitKilogram))
		if err != nil {
			t.Fatalf("lookup %s kg: %v", testCase.weight, err)
		}
		if got := selection.Amount().Amount().String(); got != testCase.want {
			t.Fatalf("%s kg priced at %s, want %s", testCase.weight, got, testCase.want)
		}
		if selection.Family() != domain.RateTableFamilyFirstContinue {
			t.Fatalf("family = %s", selection.Family())
		}
	}
}

// PA-PP-01: an economy line quotes one price per unit of chargeable weight with
// no brackets at all, so there is no interval to match 鈥?the amount is derived.
func TestUnitPriceMultipliesTheChargeableWeight(t *testing.T) {
	table := unitPriceTable(t)
	selection, err := table.Lookup("Z1", weight(t, "2.5", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got := selection.Amount().Amount().String(); got != "112.5" {
		t.Fatalf("2.5 kg at 45/kg priced at %s, want 112.5", got)
	}
	if selection.Family() != domain.RateTableFamilyUnitPrice {
		t.Fatalf("family = %s", selection.Family())
	}
}

// Every family is zone-keyed; a zone the table does not price must not fall
// back to another zone's rate.
func TestNewFamiliesKeepZonesIsolated(t *testing.T) {
	for name, table := range map[string]domain.RateTableVersion{
		"first-continue": firstContinueTable(t),
		"unit-price":     unitPriceTable(t),
	} {
		if _, err := table.Lookup("Z9", weight(t, "1", domain.WeightUnitKilogram)); !errors.Is(err, domain.ErrNoMatchingRate) {
			t.Fatalf("%s: unknown zone error = %v, want ErrNoMatchingRate", name, err)
		}
	}
}

// The explanation is what a dispute review reads. A derived amount has no
// bracket, so it must state how it was derived rather than borrow the
// weight-zone wording.
func TestDerivedFamiliesExplainHowTheAmountWasReached(t *testing.T) {
	for name, table := range map[string]domain.RateTableVersion{
		"first-continue": firstContinueTable(t),
		"unit-price":     unitPriceTable(t),
	} {
		selection, err := table.Lookup("Z1", weight(t, "1.1", domain.WeightUnitKilogram))
		if err != nil {
			t.Fatalf("%s: lookup: %v", name, err)
		}
		if selection.Explanation() == "" {
			t.Fatalf("%s: derived amount carried no explanation", name)
		}
	}
}

// A table declares one family and carries only that family's rates. Building
// one with no rates leaves a zone unpriceable at evaluation time instead of at
// construction.
func TestNewFamiliesRejectEmptyRateSets(t *testing.T) {
	reference := versionReference(t, domain.ArtifactRateTable, "table-empty", "v1")
	currency := mustValue(t, domain.NewCurrency, "USD")
	if _, err := domain.NewFirstContinueRateTable(reference, currency, domain.WeightUnitKilogram, effectivePeriod(t), nil); !errors.Is(err, domain.ErrInvalidRateTable) {
		t.Fatalf("empty first-continue table error = %v", err)
	}
	if _, err := domain.NewUnitPriceRateTable(reference, currency, domain.WeightUnitKilogram, effectivePeriod(t), nil); !errors.Is(err, domain.ErrInvalidRateTable) {
		t.Fatalf("empty unit-price table error = %v", err)
	}
}

func firstContinueTable(t *testing.T) domain.RateTableVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	rate, err := domain.NewFirstContinueRate(
		mustValue(t, domain.NewRateEntryID, "fc-z1"),
		"Z1",
		weight(t, "0.5", domain.WeightUnitKilogram),
		money(t, "30", currency),
		weight(t, "0.5", domain.WeightUnitKilogram),
		money(t, "8", currency),
	)
	if err != nil {
		t.Fatalf("first-continue rate: %v", err)
	}
	table, err := domain.NewFirstContinueRateTable(
		versionReference(t, domain.ArtifactRateTable, "table-fc", "v1"),
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.FirstContinueRate{rate},
	)
	if err != nil {
		t.Fatalf("first-continue table: %v", err)
	}
	return table
}

func unitPriceTable(t *testing.T) domain.RateTableVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	rate, err := domain.NewUnitPriceRate(
		mustValue(t, domain.NewRateEntryID, "up-z1"),
		"Z1",
		money(t, "45", currency),
	)
	if err != nil {
		t.Fatalf("unit-price rate: %v", err)
	}
	table, err := domain.NewUnitPriceRateTable(
		versionReference(t, domain.ArtifactRateTable, "table-up", "v1"),
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.UnitPriceRate{rate},
	)
	if err != nil {
		t.Fatalf("unit-price table: %v", err)
	}
	return table
}
