package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// R24 and R32 both raise the plan-level pricing weight from inside a surcharge
// clause: "计费重量不足 90LB 依然按 90LB 计". The raise decides the base freight
// band, not just the surcharge, so a light parcel that trips the clause must be
// priced in the heavier band.
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
	// 50 is the heavy band the raise reached; the extra 1 is the surcharge the
	// same clause declares, which fires on its own condition.
	if total, _ := evaluation.Total(); total.Amount().String() != "51" {
		t.Fatalf("total = %s, want 51 = 50 heavy band + 1 surcharge", total.Amount().String())
	}
}

// A clause that did not trip must leave the weight alone; otherwise every
// parcel would be priced at the floor.
func TestConditionalMinimumThatMissesLeavesTheWeightAlone(t *testing.T) {
	plan := bandedPlan(t, minimumStructures(t, "96", "oversize-minimum", "2"))
	evaluation := evaluateBanded(t, plan, "eval-minimum-miss", "50", "0.5")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	// Neither the raise nor the surcharge fires, so only the light band remains.
	if total, _ := evaluation.Total(); total.Amount().String() != "10" {
		t.Fatalf("total = %s, want 10 from the light band", total.Amount().String())
	}
}

// CONTEXT: 同一评价可存在多条，同时触发时取其中最高者. The card carries two — 40 LB
// under R24 and 90 LB under R32 — so this is the card's own shape.
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

// CONTEXT orders the steps: derive, then raise, then round. A floor of 1.2 kg
// under a whole-kilogram ceiling must therefore bill 2 kg. Raising after
// rounding would bill 1.2, which is not a whole increment at all — the order is
// observable, not a formality.
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

// CONTEXT: 抬高前后的值都要进解释. A reader checking the bill against the card has
// to see that the weight was not the parcel's own.
func TestConditionalMinimumRaiseIsExplained(t *testing.T) {
	plan := bandedPlan(t, minimumStructures(t, "48", "oversize-minimum", "2"))
	evaluation := evaluateBanded(t, plan, "eval-minimum-explained", "50", "0.5")

	if !explanationMentions(evaluation, "oversize-minimum") {
		t.Fatalf("explanation = %#v, want the raise recorded", evaluation.Explanation())
	}
}

// The clause is decided by a predicate over the package's dimensions, so
// without them it can be neither confirmed nor excluded.
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
