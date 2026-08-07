package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// L5 defines fuel as "(尾程派送费 + 除运费复核费以外的其他全部费用项) × 费率": an
// all-charges basis with a named exclusion. The exclusion is what decides the
// amount, so it has to be read from the declaration rather than assumed.
func TestPercentSurchargeChargesItsDeclaredShareOfTheBasis(t *testing.T) {
	plan := dependencyPlan(t,
		[]domain.FixedChargeRule{
			fixedRule(t, "handling", domain.ChargeEffectAdd, "5", 1),
			fixedRule(t, "audit", domain.ChargeEffectAdd, "3", 2),
		},
		allChargesBasis(t, "fuel", "FUEL", "RULE_AUDIT"),
		percentRule(t, "fuel", "FUEL", "48", "10", "fuel"),
	)
	evaluation := evaluateWithSides(t, plan, "eval-percent-basis", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	// Basis is 10 base + 5 handling = 15; the audit fee is excluded. 10% is 1.5.
	if total, _ := evaluation.Total(); total.Amount().String() != "19.5" {
		t.Fatalf("total = %s, want 19.5 = 10 + 5 + 3 + 1.5", total.Amount().String())
	}
}

// A listed basis names what it includes rather than what it leaves out. The two
// compositions must not collapse into one another: reading a listed basis as
// "everything" would silently widen what the card charges fuel on.
func TestPercentSurchargeOverAListedBasisUsesOnlyTheNamedCodes(t *testing.T) {
	plan := dependencyPlan(t,
		[]domain.FixedChargeRule{
			fixedRule(t, "handling", domain.ChargeEffectAdd, "5", 1),
			fixedRule(t, "audit", domain.ChargeEffectAdd, "3", 2),
		},
		listedChargesBasis(t, "fuel", "FUEL", "RULE_HANDLING"),
		percentRule(t, "fuel", "FUEL", "48", "10", "fuel"),
	)
	evaluation := evaluateWithSides(t, plan, "eval-percent-listed", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	// Only the handling fee is in the basis: 10% of 5 is 0.5.
	if total, _ := evaluation.Total(); total.Amount().String() != "18.5" {
		t.Fatalf("total = %s, want 18.5 = 10 + 5 + 3 + 0.5", total.Amount().String())
	}
}

// CONTEXT: 存在环时评价为冲突. A basis that reaches back to the charge it feeds has
// no fixed point, and picking an evaluation order would invent one.
func TestCircularChargeDependencyIsAConflict(t *testing.T) {
	plan, err := newDependencyPlan(t,
		nil,
		[]domain.ChargeDependency{
			listedChargesBasis(t, "fuel", "FUEL", "SURCHARGE_OTHER"),
			listedChargesBasis(t, "other", "SURCHARGE_OTHER", "FUEL"),
		},
		percentRule(t, "fuel", "FUEL", "48", "10", "fuel"),
		percentRule(t, "other", "SURCHARGE_OTHER", "48", "10", "other"),
	)
	if err != nil {
		// A plan that cannot even be built is an acceptable place to stop a
		// cycle, but it must be stopped somewhere.
		return
	}
	evaluation := evaluateWithSides(t, plan, "eval-percent-cycle", "50")
	if evaluation.Status() != domain.EvaluationConflict {
		t.Fatalf("status = %s, issues = %#v, want CONFLICT for a circular basis", evaluation.Status(), evaluation.Issues())
	}
}

// A percent charge may itself sit in another charge's basis. Resolving in
// declaration order rather than dependency order would read a zero for a charge
// that had not been computed yet, and quietly under-bill.
func TestPercentSurchargeMayFeedAnotherPercentBasis(t *testing.T) {
	plan := dependencyPlanWith(t,
		[]domain.FixedChargeRule{fixedRule(t, "handling", domain.ChargeEffectAdd, "10", 1)},
		[]domain.ChargeDependency{
			listedChargesBasis(t, "first", "FIRST_PCT", "RULE_HANDLING"),
			listedChargesBasis(t, "second", "SECOND_PCT", "FIRST_PCT"),
		},
		percentRule(t, "first", "FIRST_PCT", "48", "50", "first"),
		percentRule(t, "second", "SECOND_PCT", "48", "50", "second"),
	)
	evaluation := evaluateWithSides(t, plan, "eval-percent-chain", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	// 10 base + 10 handling + 5 (50% of handling) + 2.5 (50% of the first percent).
	if total, _ := evaluation.Total(); total.Amount().String() != "27.5" {
		t.Fatalf("total = %s, want 27.5", total.Amount().String())
	}
}

func allChargesBasis(t testing.TB, id, dependent string, excludes ...string) domain.ChargeDependency {
	t.Helper()
	dependency, err := domain.NewAllChargesDependency(id, mustValue(t, domain.NewChargeCode, dependent), chargeCodes(t, excludes...))
	if err != nil {
		t.Fatalf("all-charges dependency: %v", err)
	}
	return dependency
}

func listedChargesBasis(t testing.TB, id, dependent string, includes ...string) domain.ChargeDependency {
	t.Helper()
	dependency, err := domain.NewListedChargeDependency(id, mustValue(t, domain.NewChargeCode, dependent), chargeCodes(t, includes...), nil)
	if err != nil {
		t.Fatalf("listed dependency: %v", err)
	}
	return dependency
}

func chargeCodes(t testing.TB, codes ...string) []domain.ChargeCode {
	t.Helper()
	resolved := make([]domain.ChargeCode, 0, len(codes))
	for _, code := range codes {
		resolved = append(resolved, mustValue(t, domain.NewChargeCode, code))
	}
	return resolved
}

func percentRule(t testing.TB, id, code, threshold, percentage, basis string) domain.SurchargeRule {
	t.Helper()
	calculation, err := domain.NewPercentOfBasisSurcharge(decimal(t, percentage), basis)
	if err != nil {
		t.Fatalf("percent calculation: %v", err)
	}
	return standaloneRule(t, surchargeRuleWithCalculation(t, id, code, threshold, calculation))
}

func dependencyPlan(t *testing.T, rules []domain.FixedChargeRule, dependency domain.ChargeDependency, surcharges ...domain.SurchargeRule) domain.PricingPlanVersion {
	t.Helper()
	return dependencyPlanWith(t, rules, []domain.ChargeDependency{dependency}, surcharges...)
}

func dependencyPlanWith(t *testing.T, rules []domain.FixedChargeRule, dependencies []domain.ChargeDependency, surcharges ...domain.SurchargeRule) domain.PricingPlanVersion {
	t.Helper()
	plan, err := newDependencyPlan(t, rules, dependencies, surcharges...)
	if err != nil {
		t.Fatalf("dependency plan: %v", err)
	}
	return plan
}

func newDependencyPlan(t *testing.T, rules []domain.FixedChargeRule, dependencies []domain.ChargeDependency, surcharges ...domain.SurchargeRule) (domain.PricingPlanVersion, error) {
	t.Helper()
	structures, err := domain.NewPricingPlanStructures(surcharges, dependencies, nil)
	if err != nil {
		return domain.PricingPlanVersion{}, err
	}
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-dependency"),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-dependency", "v1"),
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
		t.Fatalf("rounding: %v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-dependency", "v1"),
		domain.PricingWeightActualOnly,
		rounding,
		nil,
	)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	return domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-dependency", "v1"),
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
}
