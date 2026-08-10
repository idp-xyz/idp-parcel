package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「它抬高的是用于基础价查表与后续费用基数的同一个计价重量」— R24 与 R32
// 都从附加费条款内部抬高方案级计价重量，卡上原话为「计费重量不足 90LB 依然按 90LB 计」。
// 抬高决定的是基础运费落在哪一档，不只是附加费，所以触发该条款的轻件必须按更重那一档计价。
func TestConditionalMinimumRaisesTheWeightUsedForTheBaseBand(t *testing.T) {
	plan := bandedPlan(t, minimumStructures(t, "48", "oversize-minimum", "2"))
	evaluation := evaluateBanded(t, plan, "eval-minimum-hit", "50", "0.5")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	pricingWeight, ok := evaluation.PricingWeight()
	if !ok || pricingWeight.RoundedWeight().Value().String() != "2" {
		t.Fatalf("pricing weight = %s, want 2 after the raise", pricingWeight.RoundedWeight().Value().String())
	}
	// 50 是抬高后够到的重档；多出的 1 是同一条款声明的附加费，它按自己的条件触发。
	if total, _ := evaluation.Total(); total.Amount().String() != "51" {
		t.Fatalf("total = %s, want 51 = 50 heavy band + 1 surcharge", total.Amount().String())
	}
}

// 没有触发的条款必须让重量原样不动，否则每一件包裹都会按下限计价。
func TestConditionalMinimumThatMissesLeavesTheWeightAlone(t *testing.T) {
	plan := bandedPlan(t, minimumStructures(t, "96", "oversize-minimum", "2"))
	evaluation := evaluateBanded(t, plan, "eval-minimum-miss", "50", "0.5")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	// 抬高与附加费都没有触发，所以只剩轻档。
	if total, _ := evaluation.Total(); total.Amount().String() != "10" {
		t.Fatalf("total = %s, want 10 from the light band", total.Amount().String())
	}
}

// Covers: CONTEXT「同一评价可存在多条，同时触发时取其中最高者」— 卡上就带着两条：R24 的
// 40 LB 与 R32 的 90 LB，所以这是卡自己的形状。
func TestSeveralTriggeredMinimumsTakeTheHighest(t *testing.T) {
	lower := conditionalMinimum(t, "48", "low-minimum", "2")
	higher := conditionalMinimum(t, "48", "high-minimum", "6")
	plan := bandedPlan(t, structuresWithMinimums(t, lower, higher))
	evaluation := evaluateBanded(t, plan, "eval-minimum-highest", "50", "0.5")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	pricingWeight, _ := evaluation.PricingWeight()
	if pricingWeight.RoundedWeight().Value().String() != "6" {
		t.Fatalf("pricing weight = %s, want 6 from the highest floor", pricingWeight.RoundedWeight().Value().String())
	}
}

// Covers: CONTEXT「计价重量」—「按重量策略从计价输入快照的实重与体积重派生、经适用的条件
// 最低重量抬高、再按取整策略进位后用于查表的重量」：所以整千克进位下，1.2 kg 的下限必须
// 计 2 kg。先进位再抬高会计出 1.2，那压根不是一个整进位单位——顺序是看得见的，不是形式。
func TestConditionalMinimumIsAppliedBeforeRounding(t *testing.T) {
	plan := bandedPlanWithCeiling(t, minimumStructures(t, "48", "oversize-minimum", "1.2"))
	evaluation := evaluateBanded(t, plan, "eval-minimum-before-rounding", "50", "0.5")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	pricingWeight, _ := evaluation.PricingWeight()
	if got := pricingWeight.RoundedWeight().Value().String(); got != "2" {
		t.Fatalf("pricing weight = %s, want 2 — the floor must be raised before the increment is applied", got)
	}
}

// Covers: CONTEXT「价表区间、分区、重量策略、附加费、折扣、最低/最高收费、燃油、组合方式、
// 精度和取整顺序必须可解释、可复算」— 拿账单对卡的人必须看得出这个重量不是包裹自己的，
// 所以抬高这件事要在解释里留痕。
func TestConditionalMinimumRaiseIsExplained(t *testing.T) {
	plan := bandedPlan(t, minimumStructures(t, "48", "oversize-minimum", "2"))
	evaluation := evaluateBanded(t, plan, "eval-minimum-explained", "50", "0.5")

	if !explanationMentions(evaluation, "oversize-minimum") {
		t.Fatalf("explanation = %#v, want the raise recorded", evaluation.Explanation())
	}
}

// 这条条款由一个读包裹尺寸的谓词决定，所以没有尺寸就既确认不了也排除不掉。
func TestConditionalMinimumWithoutDimensionsStaysPending(t *testing.T) {
	plan := bandedPlan(t, minimumStructures(t, "48", "oversize-minimum", "2"))
	request, err := domain.NewEvaluationRequest(
		mustValue(t, domain.NewEvaluationID, "eval-minimum-no-sides"),
		plan,
		syntheticInput(t, "0.5", "Z1"),
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if status := domain.EvaluatePricing(request).Status(); status != domain.EvaluationPending {
		t.Fatalf("status = %s, want PENDING", status)
	}
}

func conditionalMinimum(t testing.TB, threshold, id, minimum string) domain.SurchargeRule {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, threshold, domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	trigger, err := domain.NewTrigger(condition)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	raise, err := domain.NewConditionalMinimumWeight(id, trigger, weight(t, minimum, domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("conditional minimum: %v", err)
	}
	rule, err := standaloneRule(t, surchargeRuleFor(t, id, "CODE_"+normaliseCode(id), threshold, "1")).
		WithConditionalMinimumWeight(raise)
	if err != nil {
		t.Fatalf("attach conditional minimum: %v", err)
	}
	return rule
}

func normaliseCode(id string) string {
	out := make([]rune, 0, len(id))
	for _, character := range id {
		if character == '-' {
			out = append(out, '_')
			continue
		}
		if character >= 'a' && character <= 'z' {
			out = append(out, character-32)
			continue
		}
		out = append(out, character)
	}
	return string(out)
}

func minimumStructures(t testing.TB, threshold, id, minimum string) domain.PricingPlanStructures {
	t.Helper()
	return structuresWithMinimums(t, conditionalMinimum(t, threshold, id, minimum))
}

func structuresWithMinimums(t testing.TB, rules ...domain.SurchargeRule) domain.PricingPlanStructures {
	t.Helper()
	structures, err := domain.NewPricingPlanStructures(rules, nil, nil)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func evaluateBanded(t *testing.T, plan domain.PricingPlanVersion, id, longest, actual string) domain.PricingEvaluation {
	t.Helper()
	sides := dimensions(t, longest, "10", "10", domain.LengthUnitInch)
	request, err := domain.NewEvaluationRequest(
		mustValue(t, domain.NewEvaluationID, id),
		plan,
		syntheticInputWithDimensions(t, actual, "Z1", sides),
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return domain.EvaluatePricing(request)
}

func bandedPlan(t *testing.T, structures domain.PricingPlanStructures) domain.PricingPlanVersion {
	t.Helper()
	return newBandedPlan(t, structures, domain.RoundingNone, "1")
}

func bandedPlanWithCeiling(t *testing.T, structures domain.PricingPlanStructures) domain.PricingPlanVersion {
	t.Helper()
	return newBandedPlan(t, structures, domain.RoundingCeiling, "1")
}

func newBandedPlan(t *testing.T, structures domain.PricingPlanStructures, mode domain.RoundingMode, increment string) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	light, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "band-light"),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "1", domain.WeightUnitKilogram),
		money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("light band: %v", err)
	}
	heavy, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "band-heavy"),
		"Z1",
		weight(t, "1", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, "50", currency),
	)
	if err != nil {
		t.Fatalf("heavy band: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-banded", "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{light, heavy},
	)
	if err != nil {
		t.Fatalf("banded table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(mode, weight(t, increment, domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-banded", "v1"),
		domain.PricingWeightActualOnly,
		rounding,
		nil,
	)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-banded", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell,
		domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t),
		table,
		weightPolicy,
		nil,
		structures,
	)
	if err != nil {
		t.Fatalf("banded plan: %v", err)
	}
	return plan
}
