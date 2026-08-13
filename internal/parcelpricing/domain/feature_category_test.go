package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// categoryFeatures 造一份带类别与派生重量的完整特征集。
func categoryFeatures(t *testing.T) domain.PackageFeatures {
	t.Helper()
	base, err := domain.NewPackageFeatures(
		dimensions(t, "60", "40", "30", domain.LengthUnitCentimeter),
		weight(t, "12", domain.WeightUnitKilogram),
	)
	if err != nil {
		t.Fatalf("new package features: %v", err)
	}
	return base.
		WithDerivedWeights(
			weight(t, "14.4", domain.WeightUnitKilogram),
			weight(t, "15", domain.WeightUnitKilogram),
		).
		WithCategories("ZONE-2", "RESIDENTIAL", "SIGNATURE", "SATURDAY_DELIVERY")
}

// Covers: PP CONTEXT「特征」词条「首发的来源集合闭合，只有实重、体积重、计价重量、
// 最长边、次长边、长加围、体积、分区、地址类型和服务选项」——三项类别特征按取值相等
// 判（分区/地址类型单值、服务选项多值含判），命中与未命中都可解释。
func TestCategoricalFeaturesMatchByEquality(t *testing.T) {
	features := categoryFeatures(t)

	zoneHit, err := domain.NewCategoryFeatureCondition(domain.FeatureZone, "ZONE-2")
	if err != nil {
		t.Fatalf("new zone condition: %v", err)
	}
	if matched, err := zoneHit.Matches(features); err != nil || !matched {
		t.Fatalf("zone match = %v err = %v", matched, err)
	}

	zoneMiss, err := domain.NewCategoryFeatureCondition(domain.FeatureZone, "ZONE-8")
	if err != nil {
		t.Fatalf("new zone miss condition: %v", err)
	}
	if matched, err := zoneMiss.Matches(features); err != nil || matched {
		t.Fatalf("zone miss = %v err = %v", matched, err)
	}

	option, err := domain.NewCategoryFeatureCondition(domain.FeatureServiceOption, "SIGNATURE")
	if err != nil {
		t.Fatalf("new option condition: %v", err)
	}
	if matched, err := option.Matches(features); err != nil || !matched {
		t.Fatalf("option match = %v err = %v；多值集合里任一相等即命中", matched, err)
	}

	absentOption, err := domain.NewCategoryFeatureCondition(domain.FeatureServiceOption, "COD")
	if err != nil {
		t.Fatalf("new absent option condition: %v", err)
	}
	if matched, err := absentOption.Matches(features); err != nil || matched {
		t.Fatalf("absent option = %v err = %v", matched, err)
	}
}

// Covers: 派生重量特征——体积重与计价重量由评价器填入后条件才能读；缺席即特征不可用
// （判定条件必须可逐条解释命中与否，缺量不是悄悄未命中）。类别值缺席同理。
func TestAbsentDerivedFeaturesFailLoudly(t *testing.T) {
	bare, err := domain.NewPackageFeatures(
		dimensions(t, "60", "40", "30", domain.LengthUnitCentimeter),
		weight(t, "12", domain.WeightUnitKilogram),
	)
	if err != nil {
		t.Fatalf("new package features: %v", err)
	}

	chargeable, err := domain.NewWeightFeatureCondition(
		domain.FeatureChargeableWeight, domain.ComparisonGreaterThan,
		weight(t, "10", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("new chargeable condition: %v", err)
	}
	if _, err := chargeable.Matches(bare); !errors.Is(err, domain.ErrFeatureUnavailable) {
		t.Fatalf("err = %v; 计价重量还没算出来，条件读它必须响", err)
	}

	enriched := bare.WithDerivedWeights(
		weight(t, "14.4", domain.WeightUnitKilogram),
		weight(t, "15", domain.WeightUnitKilogram),
	)
	if matched, err := chargeable.Matches(enriched); err != nil || !matched {
		t.Fatalf("match = %v err = %v；填入后 15kg > 10kg 命中", matched, err)
	}

	zone, err := domain.NewCategoryFeatureCondition(domain.FeatureZone, "ZONE-2")
	if err != nil {
		t.Fatalf("new zone condition: %v", err)
	}
	if _, err := zone.Matches(bare); !errors.Is(err, domain.ErrFeatureUnavailable) {
		t.Fatalf("err = %v; 类别值缺席不是零值命中", err)
	}
}

// Covers: 量纲与运算的构造期配对——类别没有大小（次序运算配类别在 NewCategoryFeatureCondition
// 的固定 EQ 上封死），数值特征配 EQ 同样拦（CONTEXT 126：比较运算集合扩充属版本内容
// 变化，不在这里顺手放行）；空期望取值拒。
func TestOperatorAndMeasurePairingIsEnforcedAtConstruction(t *testing.T) {
	if _, err := domain.NewWeightFeatureCondition(
		domain.FeatureActualWeight, domain.ComparisonEquals,
		weight(t, "10", domain.WeightUnitKilogram)); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("err = %v; 数值特征配 EQ 被收下了", err)
	}
	if _, err := domain.NewCategoryFeatureCondition(domain.FeatureZone, ""); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("err = %v; 空期望取值被收下了", err)
	}
	if _, err := domain.NewCategoryFeatureCondition(domain.FeatureActualWeight, "ZONE-2"); !errors.Is(err, domain.ErrInvalidFeatureCondition) {
		t.Fatalf("err = %v; 数值特征走类别构造被收下了", err)
	}
	if _, err := domain.NewWeightFeatureCondition(
		domain.FeatureVolumetricWeight, domain.ComparisonLessThanOrEqual,
		weight(t, "20", domain.WeightUnitKilogram)); err != nil {
		t.Fatalf("err = %v; 体积重是合法的重量特征", err)
	}
}
