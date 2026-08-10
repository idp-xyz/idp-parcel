package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// R40 一次按三个量排除——实重超 67.5 KG、最长边超 274 CM，或长加围超 419 CM——R32 的
// 超尺寸条款还加上体积。只实现 LONGEST_SIDE 时，另外三个一个都表述不了，卡上自己的条件
// 因而无法表达。
func TestConditionsCanReadEveryQuantityTheCardDecidesOn(t *testing.T) {
	sides := dimensions(t, "100", "40", "30", domain.LengthUnitInch)
	packageFeatures := featuresWithWeight(t, sides, weight(t, "70", domain.WeightUnitKilogram))

	longest := lengthCondition(t, domain.FeatureLongestSide, "99", domain.LengthUnitInch)
	assertMatches(t, longest, packageFeatures, true, "longest side 100 > 99")

	second := lengthCondition(t, domain.FeatureSecondLongestSide, "39", domain.LengthUnitInch)
	assertMatches(t, second, packageFeatures, true, "second longest side 40 > 39")

	// 长加围 = 100 + 2×40 + 2×30 = 240
	girth := lengthCondition(t, domain.FeatureLengthAndGirth, "239", domain.LengthUnitInch)
	assertMatches(t, girth, packageFeatures, true, "length plus girth 240 > 239")
	girthMiss := lengthCondition(t, domain.FeatureLengthAndGirth, "240", domain.LengthUnitInch)
	assertMatches(t, girthMiss, packageFeatures, false, "length plus girth 240 is not > 240")

	// 体积 = 100 × 40 × 30 = 120000
	volume := volumeCondition(t, "119999", domain.LengthUnitInch)
	assertMatches(t, volume, packageFeatures, true, "volume 120000 > 119999")

	actual := weightCondition(t, "67.5", domain.WeightUnitKilogram)
	assertMatches(t, actual, packageFeatures, true, "actual weight 70 > 67.5")
	actualMiss := weightCondition(t, "70", domain.WeightUnitKilogram)
	assertMatches(t, actualMiss, packageFeatures, false, "actual weight 70 is not > 70")
}

// 阈值只有对着它所计量的那个量才有意义。把重量阈值配到长度特征上，是拿两个不同的物理量
// 相比，而无论结果是命中还是未命中都会被静默给出。
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

// Covers: CONTEXT「特征判定使用的单位与价表单位可以不同，换算规则必须版本化声明，判定中
// 不得隐式换算」— 跨单位比较等于只凭数字定规则。卡上以公制声明阈值、价表却用英制，所以
// 这种不匹配是真实业务，不是理论情形。
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

// 摘要按计量种类各读一个阈值字段。若对所有条件都去读长度字段，两条限值不同的重量条件会
// 得到同一个指纹，一个已发布版本就能挪动限值而不报内容冲突。
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

// conditionDigest 对一个只在被测条件上有差异的方案取指纹，这是在包外观察该条件对内容摘要
// 有何贡献的唯一办法。
func conditionDigest(t *testing.T, condition domain.FeatureCondition) string {
	t.Helper()
	minimum, err := domain.NewConditionalMinimumWeight("threshold-probe", leafTrigger(t, condition), weight(t, "40", domain.WeightUnitKilogram))
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
