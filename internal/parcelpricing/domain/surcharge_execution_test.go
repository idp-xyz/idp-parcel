package domain_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// A declared surcharge that has never been executed is worse than one that does
// not exist: the plan looks priced while the card's charge is missing. This is
// the first slice that actually collects one.
func TestSurchargeThatHitsIsCollectedOnTopOfTheBaseFreight(t *testing.T) {
	plan := planWithStructures(t, standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "25")))
	evaluation := evaluateWithSides(t, plan, "eval-surcharge-hit", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	total, ok := evaluation.Total()
	if !ok || total.Amount().String() != "35" {
		t.Fatalf("total = %v (present=%v), want 35 = 10 base + 25 surcharge", total.Amount().String(), ok)
	}
	if !hasChargeCode(evaluation, "AHS_DIMENSION") {
		t.Fatalf("charge lines = %#v, want one coded AHS_DIMENSION", evaluation.ChargeLines())
	}
}

// CONTEXT requires that a rule which did not fire still leaves a trace: an
// explanation listing only hits cannot be checked against the card, because a
// reader cannot tell a rule that missed from a rule that was never evaluated.
func TestSurchargeThatMissesIsExplainedRatherThanSilentlyDropped(t *testing.T) {
	plan := planWithStructures(t, standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "96", "25")))
	evaluation := evaluateWithSides(t, plan, "eval-surcharge-miss", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "10" {
		t.Fatalf("total = %s, want 10 with the surcharge missing", total.Amount().String())
	}
	if hasChargeCode(evaluation, "AHS_DIMENSION") {
		t.Fatal("a missed surcharge produced a charge line")
	}
	if !explanationMentions(evaluation, "ahs-dimension") {
		t.Fatalf("explanation = %#v, want the missed rule recorded", evaluation.Explanation())
	}
}

// 形态决定五: within one exclusivity group the card collects at most one charge,
// chosen by declared priority first. UPS's large-package charge suppressing
// additional handling is exactly this shape.
func TestExclusivityGroupCollectsOnlyTheHighestPriorityRule(t *testing.T) {
	oversize := groupedSurcharge(t, surchargeRuleFor(t, "oversize", "OVERSIZE", "48", "30"), "AHS", 1)
	handling := groupedSurcharge(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "80"), "AHS", 2)
	plan := planWithStructures(t, declaredSurcharges(t, oversize, handling))
	evaluation := evaluateWithSides(t, plan, "eval-exclusive-priority", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	// Priority 1 outranks priority 2 even though its amount is lower, so the
	// selection must not be a plain "take the largest".
	if !hasChargeCode(evaluation, "OVERSIZE") || hasChargeCode(evaluation, "AHS_DIMENSION") {
		t.Fatalf("charge lines = %#v, want only OVERSIZE", evaluation.ChargeLines())
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "40" {
		t.Fatalf("total = %s, want 40 = 10 base + 30 oversize", total.Amount().String())
	}
}

// Same priority falls through to the highest amount, which is how the card's
// three additional-handling variants resolve against one another.
func TestExclusivityGroupFallsBackToTheHighestAmountAtEqualPriority(t *testing.T) {
	lower := groupedSurcharge(t, surchargeRuleFor(t, "ahs-a", "AHS_A", "48", "20"), "AHS", 1)
	higher := groupedSurcharge(t, surchargeRuleFor(t, "ahs-b", "AHS_B", "48", "45"), "AHS", 1)
	plan := planWithStructures(t, declaredSurcharges(t, lower, higher))
	evaluation := evaluateWithSides(t, plan, "eval-exclusive-amount", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	if !hasChargeCode(evaluation, "AHS_B") || hasChargeCode(evaluation, "AHS_A") {
		t.Fatalf("charge lines = %#v, want only AHS_B", evaluation.ChargeLines())
	}
}

// A condition reads the package's dimensions. Without them the rule can be
// neither confirmed nor excluded, which is missing evidence rather than a
// broken request, so the evaluation waits instead of failing or under-billing.
func TestSurchargeWithoutDimensionsStaysPending(t *testing.T) {
	plan := planWithStructures(t, standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "25")))
	id := mustValue(t, domain.NewEvaluationID, "eval-surcharge-no-sides")
	request, err := domain.NewEvaluationRequest(id, plan, syntheticInput(t, "5", "Z1"), domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	evaluation := domain.EvaluatePricing(request)
	if evaluation.Status() != domain.EvaluationPending {
		t.Fatalf("status = %s, issues = %#v, want PENDING", evaluation.Status(), evaluation.Issues())
	}
}

// The gate that refuses to price a plan carrying structures this build cannot
// execute must narrow as capabilities land, not disappear. A table-lookup
// surcharge still has no executor, so pricing it would under-bill silently.
func TestPlanCarryingAStillUnexecutableStructureDoesNotForm(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewOpenEndedRateEntry(
		mustValue(t, domain.NewRateEntryID, "surcharge-band"),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		money(t, "18", currency),
	)
	if err != nil {
		t.Fatalf("surcharge band: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-surcharge", "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("surcharge table: %v", err)
	}
	calculation, err := domain.NewTableLookupSurcharge(table)
	if err != nil {
		t.Fatalf("table calculation: %v", err)
	}
	rule := standaloneRule(t, surchargeRuleWithCalculation(t, "table-rule", "TABLE_RULE", "48", calculation))
	plan := planWithStructures(t, declaredSurcharges(t, rule))
	evaluation := evaluateWithSides(t, plan, "eval-unexecutable", "50")

	if evaluation.Status() != domain.EvaluationFailed {
		t.Fatalf("status = %s, want FAILED while table-lookup surcharges have no executor", evaluation.Status())
	}
}

// A surcharged evaluation that cannot be replayed is only half formed: replay
// is how a dispute is answered. The charge-line contract is checked on the way
// in to a replay, not on the way out of an evaluation, so a surcharge line the
// contract rejects completes fine and only fails later.
func TestSurchargedEvaluationCanBeReplayed(t *testing.T) {
	plan := planWithStructures(t, standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "25")))
	original := evaluateWithSides(t, plan, "eval-surcharge-replay-source", "50")
	if original.Status() != domain.EvaluationCompleted {
		t.Fatalf("fixture status = %s, issues = %#v", original.Status(), original.Issues())
	}

	replayed, err := domain.ReplayPricingEvaluation(
		mustValue(t, domain.NewEvaluationID, "eval-surcharge-replay"),
		original,
		plan,
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.Status() != domain.EvaluationCompleted {
		t.Fatalf("replay status = %s, issues = %#v", replayed.Status(), replayed.Issues())
	}
	if replayed.SemanticDigest() != original.SemanticDigest() {
		t.Fatal("replaying a surcharged evaluation produced a different semantic digest")
	}
}

// Fixed rules declare their own order and need not be contiguous, so surcharge
// lines have to continue from the highest order in use rather than from the
// number of lines collected. Otherwise a plan with a rule at order 5 would
// produce a surcharge that sorts before it.
func TestSurchargeOrderContinuesPastNonContiguousFixedRules(t *testing.T) {
	structures := standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "25"))
	plan := planWithStructuresAndFixedRules(t, structures, fixedRule(t, "late", domain.ChargeEffectAdd, "3", 5))
	evaluation := evaluateWithSides(t, plan, "eval-surcharge-order", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	lines := evaluation.ChargeLines()
	for index := 1; index < len(lines); index++ {
		if lines[index].Order() <= lines[index-1].Order() {
			t.Fatalf("charge line orders are not strictly increasing: %d then %d", lines[index-1].Order(), lines[index].Order())
		}
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "38" {
		t.Fatalf("total = %s, want 38 = 10 base + 3 fixed + 25 surcharge", total.Amount().String())
	}
}

func planWithStructuresAndFixedRules(t *testing.T, structures domain.PricingPlanStructures, rules ...domain.FixedChargeRule) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-mixed"),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-mixed", "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingNone, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("rounding policy: %v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-mixed", "v1"),
		domain.PricingWeightActualOnly,
		rounding,
		nil,
	)
	if err != nil {
		t.Fatalf("pricing weight policy: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-mixed", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell,
		domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t),
		table,
		weightPolicy,
		rules,
		structures,
	)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}

func evaluateWithSides(t *testing.T, plan domain.PricingPlanVersion, id, longest string) domain.PricingEvaluation {
	t.Helper()
	sides := dimensions(t, longest, "10", "10", domain.LengthUnitInch)
	request, err := domain.NewEvaluationRequest(
		mustValue(t, domain.NewEvaluationID, id),
		plan,
		syntheticInputWithDimensions(t, "5", "Z1", sides),
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return domain.EvaluatePricing(request)
}

func hasChargeCode(evaluation domain.PricingEvaluation, code string) bool {
	for _, line := range evaluation.ChargeLines() {
		if line.Code().String() == code {
			return true
		}
	}
	return false
}

func explanationMentions(evaluation domain.PricingEvaluation, fragment string) bool {
	for _, entry := range evaluation.Explanation() {
		if strings.Contains(entry, fragment) {
			return true
		}
	}
	return false
}

func standaloneSurcharges(t testing.TB, rules ...domain.SurchargeRule) domain.PricingPlanStructures {
	t.Helper()
	declared := make([]domain.SurchargeRule, 0, len(rules))
	for _, rule := range rules {
		declared = append(declared, standaloneRule(t, rule))
	}
	return declaredSurcharges(t, declared...)
}

func declaredSurcharges(t testing.TB, rules ...domain.SurchargeRule) domain.PricingPlanStructures {
	t.Helper()
	structures, err := domain.NewPricingPlanStructures(rules, nil, nil)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func groupedSurcharge(t testing.TB, rule domain.SurchargeRule, group string, priority int) domain.SurchargeRule {
	t.Helper()
	grouped, err := rule.InExclusivityGroup(group, priority)
	if err != nil {
		t.Fatalf("exclusivity group: %v", err)
	}
	return grouped
}

func surchargeRuleFor(t testing.TB, id, code, threshold, amount string) domain.SurchargeRule {
	t.Helper()
	return surchargeRule(t, id, code, threshold, amount)
}

func surchargeRuleWithCalculation(t testing.TB, id, code, threshold string, calculation domain.SurchargeCalculation) domain.SurchargeRule {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, threshold, domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	rule, err := domain.NewSurchargeRule(id, mustValue(t, domain.NewChargeCode, code), id, domain.ChargeEffectAdd, leafTrigger(t, condition), calculation)
	if err != nil {
		t.Fatalf("surcharge rule: %v", err)
	}
	return rule
}
