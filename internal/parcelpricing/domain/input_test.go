package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Features are derived from the input snapshot so that every rule in one
// evaluation decides against the same frozen values. If each rule reached for
// the raw sides instead, two rules in the same evaluation could disagree about
// the same package.
func TestPricingInputSnapshotDerivesFeaturesFromItsDimensions(t *testing.T) {
	input := syntheticInputWithDimensions(t, "1", "Z1", dimensions(t, "70", "8", "9", domain.LengthUnitInch))

	packageFeatures, err := input.Features()
	if err != nil {
		t.Fatalf("features: %v", err)
	}

	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, "48", domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	matched, err := condition.Matches(packageFeatures)
	if err != nil {
		t.Fatalf("matches: %v", err)
	}
	if !matched {
		t.Fatalf("matched = false, want a 70 inch longest side to clear a 48 inch threshold")
	}
}

// A snapshot that never carried dimensions is a missing fact, which can be
// supplied and re-evaluated. Measurements that are present but unusable are an
// illegal request. The context keeps those two outcomes apart on purpose, so
// the two must not arrive as the same error.
func TestPricingInputSnapshotWithoutDimensionsReportsThemMissingNotUnusable(t *testing.T) {
	input := syntheticInput(t, "1", nil, "Z1")

	_, err := input.Features()
	if !errors.Is(err, domain.ErrMissingDimensions) {
		t.Fatalf("err = %v, want %v", err, domain.ErrMissingDimensions)
	}
	if errors.Is(err, domain.ErrInvalidDimensions) {
		t.Fatalf("err = %v, must not also read as unusable measurements", err)
	}
}

// Dimensions decide which rules hit, so two evaluations differing only in the
// package's sides are different evaluations. Sharing a semantic digest would
// let a replay of one pass as a faithful replay of the other, which is the one
// thing the digest exists to prevent.
func TestEvaluationDigestSeparatesInputsThatDifferOnlyInDimensions(t *testing.T) {
	plan := syntheticPlan(t, "digest-dimensions", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.BillableWeightActualOnly, nil)

	compact := evaluate(t, "eval-compact", plan,
		syntheticInputWithDimensions(t, "1", "Z1", dimensions(t, "10", "10", "10", domain.LengthUnitInch)))
	elongated := evaluate(t, "eval-elongated", plan,
		syntheticInputWithDimensions(t, "1", "Z1", dimensions(t, "70", "8", "9", domain.LengthUnitInch)))

	if compact.SemanticDigest() == elongated.SemanticDigest() {
		t.Fatalf("evaluations differing only in dimensions share digest %s", compact.SemanticDigest())
	}
}
