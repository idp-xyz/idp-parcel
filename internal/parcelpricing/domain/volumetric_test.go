package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// L4 on the authoritative card gives both the divisor and the units it pairs:
// volumetric pounds are length × width × height in inches over 250. A 12 inch
// cube is 1728 cubic inches, so 6.912 lb before the declared rounding carries
// it to the next whole pound. The divisor is card content, never a constant
// this package holds, so the test supplies it the way a card version would.
func TestVolumetricWeightIsDerivedFromDimensionsAndTheDeclaredDivisor(t *testing.T) {
	policy := maxWeightPolicy(t, "weight-volumetric", "250", domain.LengthUnitInch, "1")
	input := inputWithSides(t, "5", domain.WeightUnitPound, dimensions(t, "12", "12", "12", domain.LengthUnitInch))

	result, err := domain.CalculatePricingWeight(input, policy, domain.WeightUnitPound)
	if err != nil {
		t.Fatalf("pricing weight: %v", err)
	}
	volumetric, derived := result.VolumetricWeight()
	if !derived || volumetric.Value().String() != "7" {
		t.Fatalf("volumetric = %#v, derived=%v, want 7 LB", volumetric, derived)
	}
	if result.RawWeight().Value().String() != "7" {
		t.Fatalf("raw = %s, want the greater of 5 and 7", result.RawWeight().Value())
	}
}

// The divisor pairs one length unit with one weight unit; 250 turns cubic
// inches into pounds and says nothing about centimetres. Converting silently
// would misprice, and the conversion rule would itself have to be versioned.
func TestVolumetricFactorRefusesSidesMeasuredInAnotherUnit(t *testing.T) {
	policy := maxWeightPolicy(t, "weight-unit-clash", "250", domain.LengthUnitInch, "1")
	input := inputWithSides(t, "5", domain.WeightUnitPound, dimensions(t, "30", "30", "30", domain.LengthUnitCentimeter))
	if _, err := domain.CalculatePricingWeight(input, policy, domain.WeightUnitPound); !errors.Is(err, domain.ErrLengthUnitMismatch) {
		t.Fatalf("cross-unit divisor error = %v", err)
	}
}

// CONTEXT keeps MAX waiting rather than falling back to the actual weight: a
// package whose sides never arrived is a missing fact that can still turn up,
// not a package that happens to price on weight alone.
func TestPricingWeightStaysPendingWhenMaxHasNoSides(t *testing.T) {
	policy := maxWeightPolicy(t, "weight-no-sides", "250", domain.LengthUnitInch, "1")
	input := inputWithoutSides(t, "5", domain.WeightUnitPound)
	if _, err := domain.CalculatePricingWeight(input, policy, domain.WeightUnitPound); !errors.Is(err, domain.ErrMissingDimensions) {
		t.Fatalf("missing sides error = %v", err)
	}
}

// A card that changes its divisor charges different money for the same
// package, so the two versions must not share a content digest.
func TestPricingPlanContentDigestCoversTheVolumetricDivisor(t *testing.T) {
	first := planWithWeightPolicy(t, maxWeightPolicy(t, "weight-divisor", "250", domain.LengthUnitInch, "1"))
	second := planWithWeightPolicy(t, maxWeightPolicy(t, "weight-divisor", "139", domain.LengthUnitInch, "1"))
	if first.ContentDigest() == second.ContentDigest() {
		t.Fatal("volumetric divisor was omitted from the plan content digest")
	}
}

// MAX cannot reach a volumetric weight without a divisor, and ACTUAL_ONLY never
// reads one. Either mismatch leaves a declaration nothing acts on.
func TestPricingWeightPolicyPairsTheDivisorWithTheMethodThatNeedsIt(t *testing.T) {
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "1", domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	reference := versionReference(t, domain.ArtifactWeightPolicy, "weight-pairing", "v1")
	factor := volumetricFactor(t, "250", domain.LengthUnitInch, "1")

	if _, err := domain.NewPricingWeightPolicy(reference, domain.PricingWeightMax, rounding, nil); !errors.Is(err, domain.ErrInvalidRoundingPolicy) {
		t.Fatalf("MAX without a divisor error = %v", err)
	}
	if _, err := domain.NewPricingWeightPolicy(reference, domain.PricingWeightActualOnly, rounding, &factor); !errors.Is(err, domain.ErrInvalidRoundingPolicy) {
		t.Fatalf("ACTUAL_ONLY with a divisor error = %v", err)
	}
}

// An exact quotient need not terminate in base 10, so leaving it unrounded
// would mean an undeclared precision decided by whatever the code happened to
// do. The factor has to state where the quotient lands.
func TestVolumetricFactorRequiresADeclaredQuotientPrecision(t *testing.T) {
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingNone, weight(t, "1", domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	if _, err := domain.NewVolumetricFactor(decimal(t, "250"), domain.LengthUnitInch, rounding); !errors.Is(err, domain.ErrInvalidVolumetricFactor) {
		t.Fatalf("unrounded quotient error = %v", err)
	}
}

func volumetricFactor(t testing.TB, divisor string, lengthUnit domain.LengthUnit, increment string) domain.VolumetricFactor {
	t.Helper()
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, increment, domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("volumetric rounding: %v", err)
	}
	factor, err := domain.NewVolumetricFactor(decimal(t, divisor), lengthUnit, rounding)
	if err != nil {
		t.Fatalf("volumetric factor: %v", err)
	}
	return factor
}

func maxWeightPolicy(t testing.TB, id, divisor string, lengthUnit domain.LengthUnit, increment string) domain.PricingWeightPolicy {
	t.Helper()
	factor := volumetricFactor(t, divisor, lengthUnit, increment)
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "1", domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	policy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, id, "v1"),
		domain.PricingWeightMax,
		rounding,
		&factor,
	)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	return policy
}

func inputWithSides(t testing.TB, actual string, unit domain.WeightUnit, sides domain.Dimensions) domain.PricingInputSnapshot {
	t.Helper()
	return newSnapshot(t, actual, unit, &sides)
}

func inputWithoutSides(t testing.TB, actual string, unit domain.WeightUnit) domain.PricingInputSnapshot {
	t.Helper()
	return newSnapshot(t, actual, unit, nil)
}

func newSnapshot(t testing.TB, actual string, unit domain.WeightUnit, sides *domain.Dimensions) domain.PricingInputSnapshot {
	t.Helper()
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"),
		"Z1",
		weight(t, actual, unit),
		sides,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("pricing input: %v", err)
	}
	return input
}

func planWithWeightPolicy(t testing.TB, policy domain.PricingWeightPolicy) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-divisor"),
		"Z1",
		weight(t, "0", domain.WeightUnitPound),
		weight(t, "100", domain.WeightUnitPound),
		money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-divisor", "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		domain.WeightUnitPound,
		effectivePeriod(t),
		[]domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-divisor", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell,
		domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t),
		table,
		policy,
		nil,
		domain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}
