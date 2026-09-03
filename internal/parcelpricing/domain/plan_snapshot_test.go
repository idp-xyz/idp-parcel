package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证价卡快照编解码：把 CONTEXT「版本内容摘要」下列出的每一种结构都装满一张
// 合成价卡（S 级，SYN-PRC 系列），折成快照再重建，必须逐字节同答且过领域整图重验；
// 规范化版本不被当前构建支持时拒绝重建（ADR-0014 的「结构上算不出来」），内容被改
// 时以摘要自校暴露，不还回一个看起来合法的价卡。

// fullyDeclaredSyntheticPlan 造一张声明了全部结构件的合成价卡：分段取整、体积系数、
// 三个价表族（基础表重量段阶梯 + 附加费查首重加续重表）、固定规则、互斥组、取较大值、
// 序列费率、条件最低计价重量、费用依赖、序列绑定、拒收条款与额外清单依赖。
func fullyDeclaredSyntheticPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")

	entryLow, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "SYN-PRC-ENTRY-LOW"),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("构造低档费率段：%v", err)
	}
	entryOpen, err := domain.NewOpenEndedRateEntry(
		mustValue(t, domain.NewRateEntryID, "SYN-PRC-ENTRY-OPEN"),
		"Z1",
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, "20", currency),
	)
	if err != nil {
		t.Fatalf("构造开口费率段：%v", err)
	}
	baseTable, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "SYN-PRC-TABLE-BASE", "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{entryLow, entryOpen},
	)
	if err != nil {
		t.Fatalf("构造基础价表：%v", err)
	}

	segmentFine, err := domain.NewWeightRoundingSegment(
		domain.RoundingCeiling, weight(t, "0.1", domain.WeightUnitKilogram), weight(t, "5", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("构造细段取整：%v", err)
	}
	segmentCoarse, err := domain.NewOpenEndedWeightRoundingSegment(
		domain.RoundingCeiling, weight(t, "0.5", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("构造开口取整：%v", err)
	}
	rounding, err := domain.NewSegmentedWeightRoundingPolicy(
		[]domain.WeightRoundingSegment{segmentFine, segmentCoarse})
	if err != nil {
		t.Fatalf("构造分段取整策略：%v", err)
	}
	factorRounding, err := domain.NewWeightRoundingPolicy(
		domain.RoundingCeiling, weight(t, "0.1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("构造体积系数精度：%v", err)
	}
	factor, err := domain.NewVolumetricFactor(decimal(t, "5000"), domain.LengthUnitCentimeter, factorRounding)
	if err != nil {
		t.Fatalf("构造体积系数：%v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "SYN-PRC-WEIGHT", "v1"),
		domain.PricingWeightMax,
		rounding,
		&factor,
	)
	if err != nil {
		t.Fatalf("构造计价重量策略：%v", err)
	}

	oversizeTrigger, err := domain.NewAnyOfTrigger(
		trigger(t, lengthCondition(t, domain.FeatureLongestSide, "100", domain.LengthUnitCentimeter)),
		trigger(t, weightCondition(t, "20", domain.WeightUnitKilogram)),
		trigger(t, volumeCondition(t, "1000", domain.LengthUnitCentimeter)),
	)
	if err != nil {
		t.Fatalf("构造超限触发条件：%v", err)
	}
	oversizeFixed, err := domain.NewFixedAmountSurcharge(money(t, "5", currency))
	if err != nil {
		t.Fatalf("构造定额计算：%v", err)
	}
	oversizePercent, err := domain.NewPercentOfBasisSurcharge(decimal(t, "0.1"), "SYN-PRC-DEP-ALL")
	if err != nil {
		t.Fatalf("构造百分比计算：%v", err)
	}
	oversizeGreater, err := domain.NewGreaterOfSurcharge(oversizeFixed, oversizePercent)
	if err != nil {
		t.Fatalf("构造取较大值计算：%v", err)
	}
	minimum, err := domain.NewConditionalMinimumWeight(
		"SYN-PRC-MIN-OVERSIZE",
		trigger(t, lengthCondition(t, domain.FeatureLengthAndGirth, "300", domain.LengthUnitCentimeter)),
		weight(t, "20", domain.WeightUnitKilogram),
	)
	if err != nil {
		t.Fatalf("构造条件最低计价重量：%v", err)
	}
	oversize, err := domain.NewSurchargeRule(
		"SYN-PRC-SUR-OVERSIZE",
		mustValue(t, domain.NewChargeCode, "OVERSIZE_FEE"),
		"oversize handling",
		domain.ChargeEffectAdd,
		oversizeTrigger,
		oversizeGreater,
	)
	if err != nil {
		t.Fatalf("构造超限附加费：%v", err)
	}
	oversize, err = oversize.InExclusivityGroup("OVERSIZE", 2)
	if err != nil {
		t.Fatalf("声明互斥组：%v", err)
	}
	oversize, err = oversize.WithConditionalMinimumWeight(minimum)
	if err != nil {
		t.Fatalf("挂接条件最低计价重量：%v", err)
	}

	remoteRate, err := domain.NewFirstContinueRate(
		mustValue(t, domain.NewRateEntryID, "SYN-PRC-REMOTE-Z1"),
		"Z1",
		weight(t, "1", domain.WeightUnitKilogram),
		money(t, "3", currency),
		weight(t, "0.5", domain.WeightUnitKilogram),
		money(t, "1", currency),
	)
	if err != nil {
		t.Fatalf("构造首重加续重费率：%v", err)
	}
	remoteTable, err := domain.NewFirstContinueRateTable(
		versionReference(t, domain.ArtifactRateTable, "SYN-PRC-TABLE-REMOTE", "v1"),
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.FirstContinueRate{remoteRate},
	)
	if err != nil {
		t.Fatalf("构造偏远附加费价表：%v", err)
	}
	remoteLookup, err := domain.NewTableLookupSurcharge(remoteTable)
	if err != nil {
		t.Fatalf("构造查表计算：%v", err)
	}
	remote, err := domain.NewSurchargeRule(
		"SYN-PRC-SUR-REMOTE",
		mustValue(t, domain.NewChargeCode, "REMOTE_AREA"),
		"remote area delivery",
		domain.ChargeEffectAdd,
		trigger(t, categoryCondition(t, domain.FeatureZone, "Z1")),
		remoteLookup,
	)
	if err != nil {
		t.Fatalf("构造偏远附加费：%v", err)
	}
	remote, err = remote.Standalone()
	if err != nil {
		t.Fatalf("声明并列计收：%v", err)
	}

	fuelCalculation, err := domain.NewSeriesRateSurcharge(
		domain.ReferenceSeriesFuelRate, decimal(t, "0.8"), "SYN-PRC-DEP-FUEL")
	if err != nil {
		t.Fatalf("构造序列费率计算：%v", err)
	}
	fuel, err := domain.NewSurchargeRule(
		"SYN-PRC-SUR-FUEL",
		mustValue(t, domain.NewChargeCode, "FUEL_SURCHARGE"),
		"fuel surcharge",
		domain.ChargeEffectAdd,
		trigger(t, weightCondition(t, "0", domain.WeightUnitKilogram)),
		fuelCalculation,
	)
	if err != nil {
		t.Fatalf("构造燃油附加费：%v", err)
	}
	fuel, err = fuel.Standalone()
	if err != nil {
		t.Fatalf("声明燃油并列计收：%v", err)
	}

	dependencyAll, err := domain.NewAllChargesDependency(
		"SYN-PRC-DEP-ALL",
		mustValue(t, domain.NewChargeCode, "OVERSIZE_FEE"),
		[]domain.ChargeCode{mustValue(t, domain.NewChargeCode, "REMOTE_AREA")},
	)
	if err != nil {
		t.Fatalf("构造全额基数依赖：%v", err)
	}
	dependencyFuel, err := domain.NewListedChargeDependency(
		"SYN-PRC-DEP-FUEL",
		mustValue(t, domain.NewChargeCode, "FUEL_SURCHARGE"),
		[]domain.ChargeCode{mustValue(t, domain.NewChargeCode, "BASE_FREIGHT")},
		nil,
	)
	if err != nil {
		t.Fatalf("构造列举基数依赖：%v", err)
	}

	fuelBinding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-WEEKLY")
	if err != nil {
		t.Fatalf("构造燃油序列绑定：%v", err)
	}
	fxBinding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesExchangeRate, "SYN-PRC-USD-CNY")
	if err != nil {
		t.Fatalf("构造汇率序列绑定：%v", err)
	}

	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{oversize, remote, fuel},
		[]domain.ChargeDependency{dependencyAll, dependencyFuel},
		[]domain.ReferenceSeriesBinding{fuelBinding, fxBinding},
	)
	if err != nil {
		t.Fatalf("构造方案结构：%v", err)
	}
	exclusion, err := domain.NewExclusionRule(
		"SYN-PRC-EXCL-OVERLIMIT",
		"L12 over-limit refusal",
		trigger(t, weightCondition(t, "150", domain.WeightUnitKilogram)),
	)
	if err != nil {
		t.Fatalf("构造拒收条款：%v", err)
	}
	structures, err = structures.WithExclusionRules(exclusion)
	if err != nil {
		t.Fatalf("挂接拒收条款：%v", err)
	}

	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "SYN-PRC-PLAN-FULL", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionBuy,
		domain.PricingPurposeSupplierCost,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t),
		baseTable,
		weightPolicy,
		[]domain.FixedChargeRule{
			fixedRule(t, "handling", domain.ChargeEffectAdd, "2", 1),
			fixedRule(t, "rebate", domain.ChargeEffectDeduct, "1", 2),
		},
		structures,
		versionReference(t, domain.ArtifactCommercialPolicy, "SYN-PRC-FX-POLICY", "v3"),
	)
	if err != nil {
		t.Fatalf("构造全结构价卡：%v", err)
	}
	return plan
}

func trigger(t *testing.T, condition domain.FeatureCondition) domain.TriggerCondition {
	t.Helper()
	built, err := domain.NewTrigger(condition)
	if err != nil {
		t.Fatalf("构造触发条件：%v", err)
	}
	return built
}

func categoryCondition(t *testing.T, source domain.FeatureSource, expected string) domain.FeatureCondition {
	t.Helper()
	condition, err := domain.NewCategoryFeatureCondition(source, domain.CategoryValue(expected))
	if err != nil {
		t.Fatalf("构造类别判定条件：%v", err)
	}
	return condition
}

// TestPlanSnapshotRoundTripsFullyDeclaredPlan 证全结构价卡折装快照后原样重建：快照
// 逐字节同答、内容摘要与规范化版本原值保留、结构件一件不少。
func TestPlanSnapshotRoundTripsFullyDeclaredPlan(t *testing.T) {
	plan := fullyDeclaredSyntheticPlan(t)

	raw, err := domain.MarshalPricingPlanSnapshot(plan)
	if err != nil {
		t.Fatalf("折装快照：%v", err)
	}
	rebuilt, err := domain.RehydratePricingPlanSnapshot(raw)
	if err != nil {
		t.Fatalf("重建价卡：%v", err)
	}

	again, err := domain.MarshalPricingPlanSnapshot(rebuilt)
	if err != nil {
		t.Fatalf("重建后再折装：%v", err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatalf("快照往返不同答\n首次=%s\n再次=%s", raw, again)
	}
	if rebuilt.ContentDigest() != plan.ContentDigest() {
		t.Fatalf("内容摘要漂移：%s → %s", plan.ContentDigest(), rebuilt.ContentDigest())
	}
	if rebuilt.CanonicalizationVersion() != plan.CanonicalizationVersion() {
		t.Fatalf("规范化版本漂移：%s → %s", plan.CanonicalizationVersion(), rebuilt.CanonicalizationVersion())
	}
	if len(rebuilt.Structures().SurchargeRules()) != 3 ||
		len(rebuilt.Structures().ChargeDependencies()) != 2 ||
		len(rebuilt.Structures().ReferenceSeries()) != 2 ||
		len(rebuilt.Structures().ExclusionRules()) != 1 {
		t.Fatalf("结构件缺失：%+v", rebuilt.Structures())
	}
	if len(rebuilt.Rules()) != 2 || rebuilt.Direction() != domain.PricingDirectionBuy {
		t.Fatalf("固定规则或方向漂移")
	}
	if rebuilt.RateTable().Family() != domain.RateTableFamilyWeightZone {
		t.Fatalf("基础价表族漂移：%s", rebuilt.RateTable().Family())
	}
}

// TestPlanSnapshotRefusesUnsupportedCanonicalization 证按别的规范化版本记录的快照
// 拒绝重建：那是结构上算不出来（ADR-0014），不是版本内容冲突，也不能装作重建成功。
func TestPlanSnapshotRefusesUnsupportedCanonicalization(t *testing.T) {
	raw, err := domain.MarshalPricingPlanSnapshot(fullyDeclaredSyntheticPlan(t))
	if err != nil {
		t.Fatalf("折装快照：%v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("解开快照：%v", err)
	}
	document["canonicalization"] = "PPC-2"
	tampered, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("重封快照：%v", err)
	}

	if _, err := domain.RehydratePricingPlanSnapshot(tampered); !errors.Is(err, domain.ErrCanonicalizationVersionUnsupported) {
		t.Fatalf("err = %v, 想要 ErrCanonicalizationVersionUnsupported", err)
	}
}

// TestPlanSnapshotExposesTampering 证内容被改的快照重建即失败：摘要自校在重建门上，
// 坏写入不会变成一个看起来合法的价卡。
func TestPlanSnapshotExposesTampering(t *testing.T) {
	raw, err := domain.MarshalPricingPlanSnapshot(fullyDeclaredSyntheticPlan(t))
	if err != nil {
		t.Fatalf("折装快照：%v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("解开快照：%v", err)
	}
	document["baseChargeCode"] = "TAMPERED_CODE"
	tampered, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("重封快照：%v", err)
	}

	if _, err := domain.RehydratePricingPlanSnapshot(tampered); !errors.Is(err, domain.ErrPricingPlanSnapshotInvalid) {
		t.Fatalf("err = %v, 想要 ErrPricingPlanSnapshotInvalid", err)
	}
}

// TestPlanSnapshotRefusesInvalidPlan 证折装门与重建门是同一道：立不住的价卡折不出
// 快照。
func TestPlanSnapshotRefusesInvalidPlan(t *testing.T) {
	if _, err := domain.MarshalPricingPlanSnapshot(domain.PricingPlanVersion{}); !errors.Is(err, domain.ErrPricingPlanSnapshotInvalid) {
		t.Fatalf("err = %v, 想要 ErrPricingPlanSnapshotInvalid", err)
	}
}
