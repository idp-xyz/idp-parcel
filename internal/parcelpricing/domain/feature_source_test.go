package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// R40 excludes on three quantities at once — actual weight over 67.5 KG,
// longest side over 274 CM, or length-plus-girth over 419 CM — and R32's
// oversize clause adds volume. With only LONGEST_SIDE implemented none of the
// other three could be stated, so the card's own conditions were unexpressible.
func TestConditionsCanReadEveryQuantityTheCardDecidesOn(t *testing.T) {
	sides := dimensions(t, "100", "40", "30", domain.LengthUnitInch)
	packageFeatures := featuresWithWeight(t, sides, weight(t, "70", domain.WeightUnitKilogram))

	longest := lengthCondition(t, domain.FeatureLongestSide, "99", domain.LengthUnitInch)
	assertMatches(t, longest, packageFeatures, true, "longest side 100 > 99")

	second := lengthCondition(t, domain.FeatureSecondLongestSide, "39", domain.LengthUnitInch)
	assertMatches(t, second, packageFeatures, true, "second longest side 40 > 39")

	// 100 + 2×40 + 2×30 = 240
	girth := lengthCondition(t, domain.FeatureLengthAndGirth, "239", domain.LengthUnitInch)
	assertMatches(t, girth, packageFeatures, true, "length plus girth 240 > 239")
	girthMiss := lengthCondition(t, domain.FeatureLengthAndGirth, "240", domain.LengthUnitInch)
	assertMatches(t, girthMiss, packageFeatures, false, "length plus girth 240 is not > 240")

	// 100 × 40 × 30 = 120000
	volume := volumeCondition(t, "119999", domain.LengthUnitInch)
	assertMatches(t, volume, packageFeatures, true, "volume 120000 > 119999")

	actual := weightCondition(t, "67.5", domain.WeightUnitKilogram)
	assertMatches(t, actual, packageFeatures, true, "actual weight 70 > 67.5")
	actualMiss := weightCondition(t, "70", domain.WeightUnitKilogram)
	assertMatches(t, actualMiss, packageFeatures, false, "actual weight 70 is not > 70")
}

// A threshold only means something against the quantity it measures. Pairing a
// weight threshold with a length feature would compare two different physical
// quantities and silently produce a hit or a miss either way.
func TestConditionRefusesAThresholdThatDoesNotMeasureItsFeature(t *testing.T) {
	if _, err := domain.NewWeightFeatureCondition(domain.FeatureLongestSide, domain.ComparisonGreaterThan, weight(t, "1", domain.WeightUnitKilogram)); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("weight threshold on a length feature error = %v", err)
	}
	if _, err := domain.NewLengthFeatureCondition(domain.FeatureActualWeight, domain.ComparisonGreaterThan, length(t, "1", domain.LengthUnitInch)); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("length threshold on a weight feature error = %v", err)
	}
	if _, err := domain.NewVolumeFeatureCondition(domain.FeatureLongestSide, domain.ComparisonGreaterThan, volumeOf(t, "1", domain.LengthUnitInch)); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("volume threshold on a length feature error = %v", err)
	}
}

// Comparing across units would decide the rule on the number alone. The card
// states metric thresholds against an imperial table, so the mismatch is real
// traffic, not a theoretical case.
func TestConditionRefusesAThresholdMeasuredInAnotherUnit(t *testing.T) {
	sides := dimensions(t, "100", "40", "30", domain.LengthUnitInch)
	packageFeatures := featuresWithWeight(t, sides, weight(t, "70", domain.WeightUnitKilogram))

	metric := lengthCondition(t, domain.FeatureLongestSide, "274", domain.LengthUnitCentimeter)
	if _, err := metric.Matches(packageFeatures); !errors.Is(err, domain.ErrLengthUnitMismatch) {
		t.Fatalf("cross-unit length comparison error = %v", err)
	}
	imperialWeight := weightCondition(t, "150", domain.WeightUnitPound)
	if _, err := imperialWeight.Matches(packageFeatures); !errors.Is(err, domain.ErrWeightUnitMismatch) {
		t.Fatalf("cross-unit weight comparison error = %v", err)
	}
}

// The digest reads one threshold field per measure. Reading the length field
// for every condition would give two weight conditions with different limits
// the same fingerprint, and a released version could then move the limit
// without reporting a content conflict.
func TestConditionDigestCoversThresholdsOfEveryMeasure(t *testing.T) {
	lighter := weightCondition(t, "67.5", domain.WeightUnitKilogram)
	heavier := weightCondition(t, "110", domain.WeightUnitKilogram)
	if conditionDigest(t, lighter) == conditionDigest(t, heavier) {
		t.Fatal("weight threshold was omitted from the condition digest")
	}
	smaller := volumeCondition(t, "10368", domain.LengthUnitInch)
	larger := volumeCondition(t, "17280", domain.LengthUnitInch)
	if conditionDigest(t, smaller) == conditionDigest(t, larger) {
		t.Fatal("volume threshold was omitted from the condition digest")
	}
}

// conditionDigest fingerprints a plan that differs only in the condition under
// test, which is the only way to observe the condition's contribution to the
// content digest from outside the package.
func conditionDigest(t *testing.T, condition domain.FeatureCondition) string {
	t.Helper()
	minimum, err := domain.NewConditionalMinimumWeight("threshold-probe", condition, weight(t, "40", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("conditional minimum weight: %v", err)
	}
	rule, err := standaloneRule(t, surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10")).
		WithConditionalMinimumWeight(minimum)
	if err != nil {
		t.Fatalf("attach conditional minimum: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, nil, nil)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return planWithStructures(t, structures).ContentDigest()
}

func assertMatches(t *testing.T, condition domain.FeatureCondition, packageFeatures domain.PackageFeatures, want bool, describe string) {
	t.Helper()
	got, err := condition.Matches(packageFeatures)
	if err != nil {
		t.Fatalf("%s: %v", describe, err)
	}
	if got != want {
		t.Fatalf("%s: matched=%v, want %v", describe, got, want)
	}
}

func lengthCondition(t *testing.T, source domain.FeatureSource, threshold string, unit domain.LengthUnit) domain.FeatureCondition {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(source, domain.ComparisonGreaterThan, length(t, threshold, unit))
	if err != nil {
		t.Fatalf("length condition on %s: %v", source, err)
	}
	return condition
}

func weightCondition(t *testing.T, threshold string, unit domain.WeightUnit) domain.FeatureCondition {
	t.Helper()
	condition, err := domain.NewWeightFeatureCondition(domain.FeatureActualWeight, domain.ComparisonGreaterThan, weight(t, threshold, unit))
	if err != nil {
		t.Fatalf("weight condition: %v", err)
	}
	return condition
}

func volumeCondition(t *testing.T, threshold string, unit domain.LengthUnit) domain.FeatureCondition {
	t.Helper()
	condition, err := domain.NewVolumeFeatureCondition(domain.FeatureVolume, domain.ComparisonGreaterThan, volumeOf(t, threshold, unit))
	if err != nil {
		t.Fatalf("volume condition: %v", err)
	}
	return condition
}

func volumeOf(t *testing.T, value string, unit domain.LengthUnit) domain.Volume {
	t.Helper()
	result, err := domain.NewVolume(decimal(t, value), unit)
	if err != nil {
		t.Fatalf("volume %s: %v", value, err)
	}
	return result
}

func featuresWithWeight(t *testing.T, sides domain.Dimensions, actual domain.Weight) domain.PackageFeatures {
	t.Helper()
	result, err := domain.NewPackageFeatures(sides, actual)
	if err != nil {
		t.Fatalf("package features: %v", err)
	}
	return result
}
