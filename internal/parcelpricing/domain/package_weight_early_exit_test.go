package domain_test

import (
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证评价在正式计价重量那一遍之前就收尾时，自身重量那一遍做过的换算也留在评价上（ADR-0347 决定一、ADR-0346 决定三）：
// 拒收命中、序列取值缺失待判断、抬高条件求值出错，这些出口都走不到正式计价重量；换算记录丢了，重放与同标识重复请求
// 就会因为「冻结且没有记录」而答单位不一致，把原来的待判断或不可计价判成结果不一致。

// poundParcel 是按磅给实重（50 磅 ≈ 22.68 千克）的件，尺寸按厘米；价卡按千克查表，所以评价必须换算。
func poundParcel(t testing.TB) domain.PricingInputSnapshot {
	t.Helper()
	sides := dimensions(t, "60", "50", "40", domain.LengthUnitCentimeter)
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"), "Z1", weight(t, "50", domain.WeightUnitPound), &sides,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("input: %v", err)
	}
	return input
}

func conversionLines(evaluation domain.PricingEvaluation) int {
	count := 0
	for _, line := range evaluation.Explanation() {
		if strings.HasPrefix(line, "unit conversion") {
			count++
		}
	}
	return count
}

// assertEarlyExitKeepsItsConversions 钉三件事：原评价记下了换算（记录一条、解释一行）；重放得同一个状态与摘要；
// 评价快照往返之后再重放同样。
func assertEarlyExitKeepsItsConversions(t *testing.T, name string, plan domain.PricingPlanVersion, input domain.PricingInputSnapshot, want domain.EvaluationStatus, wantCode string) {
	t.Helper()
	original := evaluate(t, "eval-early-"+name, plan, input)
	if original.Status() != want || len(original.Issues()) == 0 || original.Issues()[0].Code() != wantCode {
		t.Fatalf("%s: status=%s issues=%v, want %s %s", name, original.Status(), issueCodes(original), want, wantCode)
	}
	if _, formed := original.PricingWeight(); formed {
		t.Fatalf("%s: the fixture was meant to end before the pricing weight", name)
	}
	if got := len(original.UnitConversions()); got == 0 {
		t.Fatalf("%s: the evaluation ended early and lost the unit conversion its own-weights pass made", name)
	}
	if lines := conversionLines(original); lines != len(original.UnitConversions()) {
		t.Fatalf("%s: %d conversion records but %d conversion lines in the explanation: %v", name, len(original.UnitConversions()), lines, original.Explanation())
	}

	replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-early-replay-"+name), original, plan, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("%s: replay: %v", name, err)
	}
	if replay.Status() != want || replay.SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("%s: replay = %s %v digest-equal=%v, want the same %s", name, replay.Status(), issueCodes(replay), replay.SemanticDigest() == original.SemanticDigest(), want)
	}

	raw, err := domain.MarshalEvaluationSnapshot(original)
	if err != nil {
		t.Fatalf("%s: marshal: %v", name, err)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("%s: rehydrate: %v", name, err)
	}
	if len(restored.UnitConversions()) != len(original.UnitConversions()) {
		t.Fatalf("%s: the conversion record did not survive the snapshot", name)
	}
	again, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-early-restored-"+name), restored, plan, domain.EvidenceSynthetic)
	if err != nil || again.Status() != want || again.SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("%s: replay of the restored evaluation = %s %v err=%v", name, again.Status(), issueCodes(again), err)
	}
}

func fuelRuleOnTheActualWeight(t testing.TB) (domain.SurchargeRule, domain.ChargeDependency, domain.ReferenceSeriesBinding) {
	t.Helper()
	calculation, err := domain.NewSeriesRateSurcharge(domain.ReferenceSeriesFuelRate, decimal(t, "0.8"), "freight")
	if err != nil {
		t.Fatalf("series calculation: %v", err)
	}
	rule, err := domain.NewSurchargeRule("fuel", mustValue(t, domain.NewChargeCode, "FUEL"), "fuel", domain.ChargeEffectAdd,
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "0", domain.WeightUnitKilogram), calculation)
	if err != nil {
		t.Fatalf("fuel rule: %v", err)
	}
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, "fuel-weekly")
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	return standaloneRule(t, rule), listedChargesBasis(t, "freight", "FUEL", "BASE_FREIGHT"), binding
}

// Covers: AT-PP-020（换算之后才待判断的评价重放仍得同一个待判断）与 ADR-0347 决定一——卡有一条读自身计价重量的超重费和一条绑了
// 燃油序列的规则，输入按磅给而序列取值缺失：评价在序列那一步待判断，还没走到正式计价重量；换算记录必须已经在评价上。
func TestAPendingSeriesAnswerKeepsTheConversionTheOwnWeightsPassMade(t *testing.T) {
	overweight := weightSurcharge(t, "overweight", "OVERWEIGHT_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	fuel, basis, binding := fuelRuleOnTheActualWeight(t)
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{overweight, fuel}, []domain.ChargeDependency{basis}, []domain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structures})

	assertEarlyExitKeepsItsConversions(t, "series", plan, poundParcel(t), domain.EvaluationPending, "REFERENCE_SERIES_UNRESOLVED")
}

// Covers: 同上——拒收条款读自身计价重量，50 磅折 22.68 千克、进位 23 千克，超过 20：评价在拒收那一步不可计价，没走到计价重量。
func TestAnExcludedParcelKeepsTheConversionTheOwnWeightsPassMade(t *testing.T) {
	trigger := ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram)
	refusal, err := domain.NewExclusionRule("overlimit", "over 20 kg chargeable", trigger)
	if err != nil {
		t.Fatalf("exclusion: %v", err)
	}
	structures, err := (domain.PricingPlanStructures{}).WithExclusionRules(refusal)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structures})

	assertEarlyExitKeepsItsConversions(t, "excluded", plan, poundParcel(t), domain.EvaluationUnratable, "EXCLUDED_BY_RATE_CARD")
}

// Covers: 同上——抬高条件求值出错：抬高条款按实重判，阈值写成千克而输入是磅，特征判定不换算阈值（ADR-0346 决定六），评价在
// 抬高那一步答单位不一致的冲突；自身重量那一遍的换算同样留着，重放得同一个冲突。
func TestAMinimumConditionThatErrorsKeepsTheConversionTheOwnWeightsPassMade(t *testing.T) {
	overweight := weightSurcharge(t, "overweight", "OVERWEIGHT_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	trigger := ownWeightTrigger(t, domain.FeatureActualWeight, domain.ComparisonGreaterThan, "30", domain.WeightUnitKilogram)
	floor, err := domain.NewConditionalMinimumWeight("heavy-minimum", trigger, weight(t, "40", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("minimum: %v", err)
	}
	raising := weightSurcharge(t, "heavy-fee", "HEAVY_FEE", trigger, "1")
	raising, err = raising.WithConditionalMinimumWeight(floor)
	if err != nil {
		t.Fatalf("attach minimum: %v", err)
	}
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, overweight, raising)})

	assertEarlyExitKeepsItsConversions(t, "minimum-error", plan, poundParcel(t), domain.EvaluationConflict, "WEIGHT_UNIT_MISMATCH")
}

// Covers: 走到正式计价重量那一遍的评价：记录与解释都只有一份，不因自身重量那一遍当场记过而重复；带抬高下限时以正式那一遍的
// 记录为准（保留位数可能因下限的小数位更细）。
func TestACompletedEvaluationRecordsEachConversionOnceEvenWithAMinimumRaise(t *testing.T) {
	overweight := weightSurcharge(t, "overweight", "OVERWEIGHT_SURCHARGE",
		ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "20", domain.WeightUnitKilogram), "7")
	raising := conditionalMinimumOnLongestSide(t, "oversize-minimum", "50", "30")
	plan := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightMax, structures: structuresOf(t, overweight, raising)})

	evaluation := evaluate(t, "eval-completed-with-raise", plan, poundParcel(t))
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status=%s issues=%v", evaluation.Status(), issueCodes(evaluation))
	}
	pricingWeight, ok := evaluation.PricingWeight()
	if !ok || pricingWeight.RoundedWeight().Value().String() != "30" {
		t.Fatalf("pricing weight = %v, want the raised 30", pricingWeight.RoundedWeight().Value().String())
	}
	if len(evaluation.UnitConversions()) != 1 || conversionLines(evaluation) != 1 {
		t.Fatalf("records=%d lines=%d, want one each; explanation=%v", len(evaluation.UnitConversions()), conversionLines(evaluation), evaluation.Explanation())
	}
	replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-completed-with-raise-2"), evaluation, plan, domain.EvidenceSynthetic)
	if err != nil || replay.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatalf("replay: err=%v digest-equal=%v", err, replay.SemanticDigest() == evaluation.SemanticDigest())
	}
}
