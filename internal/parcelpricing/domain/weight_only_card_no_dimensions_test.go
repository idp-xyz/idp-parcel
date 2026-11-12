package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证票 declared-goods-lines/28 的修复：一张不读任何按尺寸派生的量的卡（判定条件不读五种几何 / 体积来源、
// 重量策略是 ACTUAL_ONLY，即 RequiresDeclaredDimensions 为假），在单件输入没申报尺寸时照聚合主体同一形状拿
// 只带重量的特征评价到底，而不是停在 PACKAGE_FEATURES_UNAVAILABLE 等运营登记；真读尺寸的卡照旧停。
// 判据与试算点名缺项 DIMENSIONS 的分流同一把尺子（ADR-0381 决定二）。

// Covers: 票 declared-goods-lines/28——只按实重判附加费的卡：尺寸没申报，评价照常形成，阈值照实重判命中与否。
func TestAWeightOnlyCardJudgesWithoutDeclaredDimensions(t *testing.T) {
	overweight := weightSurcharge(t, "overweight", "OVERWEIGHT_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureActualWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightActualOnly, structures: structuresOf(t, overweight)})

	heavy := evaluate(t, "eval-no-dims-heavy", plan, syntheticInput(t, "25", "Z1"))
	if heavy.Status() != domain.EvaluationCompleted || !hasChargeCode(heavy, "OVERWEIGHT_SURCHARGE") {
		t.Fatalf("status=%s issues=%v; a 25 kg parcel without dimensions must complete and trip the actual-weight surcharge",
			heavy.Status(), issueCodes(heavy))
	}
	if _, ok := heavy.Total(); !ok {
		t.Fatalf("no total on a completed evaluation: issues=%v", issueCodes(heavy))
	}

	light := evaluate(t, "eval-no-dims-light", plan, syntheticInput(t, "10", "Z1"))
	if light.Status() != domain.EvaluationCompleted || hasChargeCode(light, "OVERWEIGHT_SURCHARGE") {
		t.Fatalf("status=%s issues=%v; a 10 kg parcel must complete without the surcharge",
			light.Status(), issueCodes(light))
	}
}

// Covers: 票 declared-goods-lines/28——拒收条款读自身计价重量而重量策略只取实重：尺寸没申报也判得出拒收，
// 答不可计价而不是待判断（对明确拒收的包裹承诺「补上就能有价」是假的，ADR-0347 的拒收先于缺口不变）。
func TestAWeightOnlyExclusionRefusesWithoutDeclaredDimensions(t *testing.T) {
	trigger := ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram)
	refusal, err := domain.NewExclusionRule("overlimit", "over 20 kg chargeable", trigger)
	if err != nil {
		t.Fatalf("exclusion: %v", err)
	}
	structures, err := (domain.PricingPlanStructures{}).WithExclusionRules(refusal)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightActualOnly, structures: structures})

	evaluation := evaluate(t, "eval-no-dims-excluded", plan, syntheticInput(t, "25", "Z1"))
	if evaluation.Status() != domain.EvaluationUnratable || issueCodes(evaluation)[0] != "EXCLUDED_BY_RATE_CARD" {
		t.Fatalf("status=%s issues=%v, want UNRATABLE EXCLUDED_BY_RATE_CARD", evaluation.Status(), issueCodes(evaluation))
	}
}

// Covers: 票 declared-goods-lines/28 的边界——判定条件读最长边：尺寸没申报照旧停 PACKAGE_FEATURES_UNAVAILABLE，
// 这一停经试算分流点名缺项 DIMENSIONS（ADR-0381 决定二），不落运营登记；本票只放开不读尺寸的卡。
func TestADimensionReadingRuleStillStallsWithoutDimensions(t *testing.T) {
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide, domain.ComparisonGreaterThan, length(t, "100", domain.LengthUnitCentimeter))
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	oversize := weightSurchargeWithTrigger(t, "oversize", "OVERSIZE_SURCHARGE", leafTrigger(t, condition), "7")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightActualOnly, structures: structuresOf(t, oversize)})

	evaluation := evaluate(t, "eval-no-dims-longest-side", plan, syntheticInput(t, "25", "Z1"))
	if evaluation.Status() != domain.EvaluationPending || issueCodes(evaluation)[0] != "PACKAGE_FEATURES_UNAVAILABLE" {
		t.Fatalf("status=%s issues=%v, want PENDING PACKAGE_FEATURES_UNAVAILABLE", evaluation.Status(), issueCodes(evaluation))
	}
}

// weightSurchargeWithTrigger 同 weightSurcharge，但触发条件由调用方给（几何条件一类）。
func weightSurchargeWithTrigger(t testing.TB, id, code string, trigger domain.TriggerCondition, amount string) domain.SurchargeRule {
	t.Helper()
	calculation, err := domain.NewFixedAmountSurcharge(money(t, amount, mustValue(t, domain.NewCurrency, "USD")))
	if err != nil {
		t.Fatalf("fixed surcharge: %v", err)
	}
	rule, err := domain.NewSurchargeRule(id, mustValue(t, domain.NewChargeCode, code), id, domain.ChargeEffectAdd, trigger, calculation)
	if err != nil {
		t.Fatalf("surcharge rule %s: %v", id, err)
	}
	return standaloneRule(t, rule)
}
