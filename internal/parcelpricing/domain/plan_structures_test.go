package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// The content digest is what tells a replay that a version reference still
// points at the same executable rules. CONTEXT fixes it over 附加费规则 and
// 判定条件, so a plan that declares a surcharge rule must not share a digest
// with an otherwise identical plan that declares none — otherwise a released
// version could gain a chargeable rule without the digest moving.
func TestPricingPlanContentDigestCoversDeclaredSurchargeRules(t *testing.T) {
	bare := planWithStructures(t, domain.PricingPlanStructures{})
	declared := planWithStructures(t, structuresWithSurcharge(t, "48"))
	if bare.ContentDigest() == declared.ContentDigest() {
		t.Fatal("declared surcharge rules were omitted from the plan content digest")
	}
}

// The threshold lives inside the condition, so two plans whose only difference
// is where the rule triggers must not agree either.
func TestPricingPlanContentDigestCoversConditionThresholds(t *testing.T) {
	lower := planWithStructures(t, structuresWithSurcharge(t, "48"))
	higher := planWithStructures(t, structuresWithSurcharge(t, "60"))
	if lower.ContentDigest() == higher.ContentDigest() {
		t.Fatal("condition threshold was omitted from the plan content digest")
	}
}

// 形态决定五 selects within an exclusivity group by priority first and amount
// second, so group membership and priority both change which single rule the
// card charges. A digest that ignored them would let a released version move a
// rule between groups without reporting a content conflict.
func TestPricingPlanContentDigestCoversExclusivityGroupAndPriority(t *testing.T) {
	ungrouped := planWithStructures(t, structuresWithSurcharge(t, "48"))
	grouped := planWithStructures(t, structuresWithGroupedSurcharge(t, "48", "AHS", 1))
	if ungrouped.ContentDigest() == grouped.ContentDigest() {
		t.Fatal("exclusivity group was omitted from the plan content digest")
	}
	reprioritised := planWithStructures(t, structuresWithGroupedSurcharge(t, "48", "AHS", 2))
	if grouped.ContentDigest() == reprioritised.ContentDigest() {
		t.Fatal("exclusivity priority was omitted from the plan content digest")
	}
}

// A priority only means something relative to the other members of a group, so
// a rule outside every group must not carry one.
func TestSurchargeRuleRejectsPriorityWithoutAnExclusivityGroup(t *testing.T) {
	rule := surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10")
	if _, err := rule.InExclusivityGroup("", 1); !errors.Is(err, domain.ErrInvalidSurchargeRule) {
		t.Fatalf("ungrouped priority error = %v", err)
	}
	if _, err := rule.InExclusivityGroup("AHS", 0); !errors.Is(err, domain.ErrInvalidSurchargeRule) {
		t.Fatalf("grouped rule without priority error = %v", err)
	}
}

// 条件最低计价重量 is declared inside a surcharge clause but raises the
// plan-level pricing weight, so it changes the base freight lookup as well as
// the surcharge. It has to be inside the digest.
func TestPricingPlanContentDigestCoversConditionalMinimumWeight(t *testing.T) {
	plain := planWithStructures(t, structuresWithSurcharge(t, "48"))
	raised := planWithStructures(t, structuresWithMinimumWeight(t, "48", "40"))
	if plain.ContentDigest() == raised.ContentDigest() {
		t.Fatal("conditional minimum weight was omitted from the plan content digest")
	}
	higher := planWithStructures(t, structuresWithMinimumWeight(t, "48", "90"))
	if raised.ContentDigest() == higher.ContentDigest() {
		t.Fatal("conditional minimum weight value was omitted from the plan content digest")
	}
}

// The card's fuel basis is "every other charge except the freight audit fee",
// so the exclusion set is what decides the amount. Two plans that exclude
// different codes are different rules and must not share a digest.
func TestPricingPlanContentDigestCoversChargeDependencies(t *testing.T) {
	plain := planWithStructures(t, structuresWithSurcharge(t, "48"))
	dependent := planWithStructures(t, structuresWithDependency(t, "FREIGHT_AUDIT_FEE"))
	if plain.ContentDigest() == dependent.ContentDigest() {
		t.Fatal("charge dependencies were omitted from the plan content digest")
	}
	other := planWithStructures(t, structuresWithDependency(t, "RESIDENTIAL_DELIVERY"))
	if dependent.ContentDigest() == other.ContentDigest() {
		t.Fatal("dependency exclusion set was omitted from the plan content digest")
	}
}

// A plan that resolves a fuel rate or an exchange rate is bound to a registered
// series; swapping the bound series changes what the plan charges even when
// every other rule is identical.
func TestPricingPlanContentDigestCoversReferenceSeriesBindings(t *testing.T) {
	unbound := planWithStructures(t, structuresWithSurcharge(t, "48"))
	bound := planWithStructures(t, structuresWithReferenceSeries(t, "fuel-weekly", "v1"))
	if unbound.ContentDigest() == bound.ContentDigest() {
		t.Fatal("reference series bindings were omitted from the plan content digest")
	}
	rebound := planWithStructures(t, structuresWithReferenceSeries(t, "fuel-weekly", "v2"))
	if bound.ContentDigest() == rebound.ContentDigest() {
		t.Fatal("reference series version was omitted from the plan content digest")
	}
}

// A replay has to reach the same series values it originally used, and the
// version manifest is where it looks. A binding the manifest does not carry
// would be resolvable only against whatever the series holds today.
func TestPricingPlanManifestCarriesBoundReferenceSeries(t *testing.T) {
	plan := planWithStructures(t, structuresWithReferenceSeries(t, "fuel-weekly", "v1"))
	for _, reference := range plan.Manifest().References() {
		if reference.Kind() == domain.ArtifactReferenceSeries && reference.ID() == "fuel-weekly" {
			return
		}
	}
	t.Fatal("bound reference series is missing from the plan version manifest")
}

func structuresWithReferenceSeries(t testing.TB, id, version string) domain.PricingPlanStructures {
	t.Helper()
	binding, err := domain.NewReferenceSeriesBinding(
		domain.ReferenceSeriesFuelRate,
		versionReference(t, domain.ArtifactReferenceSeries, id, version),
	)
	if err != nil {
		t.Fatalf("reference series binding: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{standaloneRule(t, surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10"))},
		nil,
		[]domain.ReferenceSeriesBinding{binding},
	)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

// Whether a surcharge stands beside the others or competes with them is the
// carrier's rule, not the engine's: UPS puts its large-package charge in the
// same exclusive set as additional handling, FedEx charges both. Reading an
// unset group as "stands alone" would silently apply one carrier's rule to the
// other's traffic, so silence is refused instead of defaulted.
func TestPlanStructuresRefuseASurchargeRuleThatNeverDeclaredItsExclusivityStance(t *testing.T) {
	undeclared := surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10")
	if _, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{undeclared}, nil, nil); !errors.Is(err, domain.ErrUndeclaredExclusivity) {
		t.Fatalf("undeclared exclusivity error = %v", err)
	}
}

// Standing alone is itself a declaration — the card saying this charge may be
// collected alongside the others — so it must be expressible and must not read
// the same as never having said.
func TestSurchargeRuleMayDeclareThatItStandsAlone(t *testing.T) {
	standalone, err := surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10").Standalone()
	if err != nil {
		t.Fatalf("standalone: %v", err)
	}
	if _, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{standalone}, nil, nil); err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	if group, grouped := standalone.ExclusivityGroup(); grouped {
		t.Fatalf("a standalone rule reported group %q", group)
	}
}

// A charge can never be part of its own basis; accepting that declaration would
// record a cycle as if it were a valid rule.
func TestChargeDependencyRejectsSelfReference(t *testing.T) {
	fuel := mustValue(t, domain.NewChargeCode, "FUEL")
	if _, err := domain.NewListedChargeDependency("fuel", fuel, []domain.ChargeCode{fuel}, nil); !errors.Is(err, domain.ErrInvalidChargeDependency) {
		t.Fatalf("self-referencing dependency error = %v", err)
	}
}

// CONTEXT closes the calculation set at four methods, so all four sets of
// parameters belong in the canonical form now. If only the fixed amount had a
// slot, declaring the card's max(1.782, freight × 12%) later would move the
// digest of every plan that never used it.
func TestPricingPlanContentDigestCoversEveryCalculationMethod(t *testing.T) {
	digests := make(map[string]string, 4)
	for name, calculation := range map[string]domain.SurchargeCalculation{
		"fixed amount":     fixedAmountCalculation(t, "10"),
		"table lookup":     tableLookupCalculation(t),
		"percent of basis": percentOfBasisCalculation(t, "12"),
		"greater of":       greaterOfCalculation(t, "1.782", "12"),
	} {
		plan := planWithStructures(t, structuresWithCalculation(t, calculation))
		for existing, digest := range digests {
			if digest == plan.ContentDigest() {
				t.Fatalf("%q and %q share a content digest", name, existing)
			}
		}
		digests[name] = plan.ContentDigest()
	}
}

// Taking the greater of two amounts is a choice between two other methods, not
// a third thing that can nest without end.
func TestGreaterOfSurchargeRejectsANestedGreaterOfOperand(t *testing.T) {
	nested := greaterOfCalculation(t, "1.782", "12")
	if _, err := domain.NewGreaterOfSurcharge(nested, fixedAmountCalculation(t, "10")); !errors.Is(err, domain.ErrInvalidSurchargeRule) {
		t.Fatalf("nested greater-of error = %v", err)
	}
}

// A percentage means nothing without the basis it applies to, and the basis is
// a declared dependency rather than whatever charges happen to precede it.
func TestPlanStructuresRejectPercentSurchargeWithoutItsDeclaredBasis(t *testing.T) {
	_, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{standaloneRule(t, ruleWithCalculation(t, percentOfBasisCalculation(t, "12")))},
		nil,
		nil,
	)
	if !errors.Is(err, domain.ErrInvalidChargeDependency) {
		t.Fatalf("unresolved basis error = %v", err)
	}
}

func fixedAmountCalculation(t testing.TB, amount string) domain.SurchargeCalculation {
	t.Helper()
	calculation, err := domain.NewFixedAmountSurcharge(money(t, amount, mustValue(t, domain.NewCurrency, "USD")))
	if err != nil {
		t.Fatalf("fixed amount calculation: %v", err)
	}
	return calculation
}

func tableLookupCalculation(t testing.TB) domain.SurchargeCalculation {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-surcharge"),
		"Z2",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, "7", currency),
	)
	if err != nil {
		t.Fatalf("surcharge rate entry: %v", err)
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
		t.Fatalf("surcharge rate table: %v", err)
	}
	calculation, err := domain.NewTableLookupSurcharge(table)
	if err != nil {
		t.Fatalf("table lookup calculation: %v", err)
	}
	return calculation
}

func percentOfBasisCalculation(t testing.TB, percentage string) domain.SurchargeCalculation {
	t.Helper()
	calculation, err := domain.NewPercentOfBasisSurcharge(decimal(t, percentage), "fuel")
	if err != nil {
		t.Fatalf("percent of basis calculation: %v", err)
	}
	return calculation
}

func greaterOfCalculation(t testing.TB, amount, percentage string) domain.SurchargeCalculation {
	t.Helper()
	calculation, err := domain.NewGreaterOfSurcharge(
		fixedAmountCalculation(t, amount),
		percentOfBasisCalculation(t, percentage),
	)
	if err != nil {
		t.Fatalf("greater of calculation: %v", err)
	}
	return calculation
}

func ruleWithCalculation(t testing.TB, calculation domain.SurchargeCalculation) domain.SurchargeRule {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, "48", domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	rule, err := domain.NewSurchargeRule(
		"ahs-dimension",
		mustValue(t, domain.NewChargeCode, "AHS_DIMENSION"),
		"ahs-dimension",
		domain.ChargeEffectAdd,
		leafTrigger(t, condition),
		calculation,
	)
	if err != nil {
		t.Fatalf("surcharge rule: %v", err)
	}
	return rule
}

// structuresWithCalculation always declares the fuel dependency so that the
// percentage methods have the basis they name; the dependency itself is held
// constant across the digest comparison.
func structuresWithCalculation(t testing.TB, calculation domain.SurchargeCalculation) domain.PricingPlanStructures {
	t.Helper()
	dependency, err := domain.NewAllChargesDependency(
		"fuel",
		mustValue(t, domain.NewChargeCode, "FUEL"),
		[]domain.ChargeCode{mustValue(t, domain.NewChargeCode, "FREIGHT_AUDIT_FEE")},
	)
	if err != nil {
		t.Fatalf("charge dependency: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{standaloneRule(t, ruleWithCalculation(t, calculation))},
		[]domain.ChargeDependency{dependency},
		nil,
	)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

// The shape is fixed ahead of the behaviour, so a plan can declare rules this
// evaluator cannot yet execute. Charging only the base rate in that case would
// under-bill silently and still look like a completed evaluation, so the
// evaluation must not form at all.
// A fixed-amount surcharge is executed as of the surcharge slice, so the
// fixture moved to a reference series binding, which still has no executor.
// The gate narrows as capabilities land; it must never be removed while any
// declared structure remains unexecuted.
func TestEvaluationDoesNotCompleteWhenPlanDeclaresUnexecutableStructures(t *testing.T) {
	plan := planWithStructures(t, structuresWithReferenceSeries(t, "fuel-weekly", "v1"))
	evaluation := evaluate(t, "eval-declared-structures", plan, syntheticInput(t, "1", "Z1"))
	if evaluation.Status() == domain.EvaluationCompleted {
		t.Fatal("evaluation completed while the plan declared rules the evaluator cannot execute")
	}
	if _, formed := evaluation.Total(); formed {
		t.Fatal("evaluation produced a total from the base rate alone")
	}
	issues := evaluation.Issues()
	if len(issues) != 1 || issues[0].Code() != "PLAN_STRUCTURES_NOT_EXECUTABLE" {
		t.Fatalf("issues = %#v", issues)
	}
}

func structuresWithDependency(t testing.TB, excluded string) domain.PricingPlanStructures {
	t.Helper()
	dependency, err := domain.NewAllChargesDependency(
		"fuel",
		mustValue(t, domain.NewChargeCode, "FUEL"),
		[]domain.ChargeCode{mustValue(t, domain.NewChargeCode, excluded)},
	)
	if err != nil {
		t.Fatalf("charge dependency: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{standaloneRule(t, surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", "48", "10"))}, []domain.ChargeDependency{dependency},
		nil,
	)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func structuresWithGroupedSurcharge(t testing.TB, threshold, group string, priority int) domain.PricingPlanStructures {
	t.Helper()
	rule, err := surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", threshold, "10").InExclusivityGroup(group, priority)
	if err != nil {
		t.Fatalf("exclusivity group: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, nil, nil)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func structuresWithMinimumWeight(t testing.TB, threshold, minimum string) domain.PricingPlanStructures {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, threshold, domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	conditionalMinimum, err := domain.NewConditionalMinimumWeight(
		"oversize-minimum",
		leafTrigger(t, condition),
		weight(t, minimum, domain.WeightUnitKilogram),
	)
	if err != nil {
		t.Fatalf("conditional minimum weight: %v", err)
	}
	rule, err := standaloneRule(t, surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", threshold, "10")).
		WithConditionalMinimumWeight(conditionalMinimum)
	if err != nil {
		t.Fatalf("attach conditional minimum: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, nil, nil)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func structuresWithSurcharge(t testing.TB, threshold string) domain.PricingPlanStructures {
	t.Helper()
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{standaloneRule(t, surchargeRule(t, "ahs-dimension", "AHS_DIMENSION", threshold, "10"))},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func standaloneRule(t testing.TB, rule domain.SurchargeRule) domain.SurchargeRule {
	t.Helper()
	declared, err := rule.Standalone()
	if err != nil {
		t.Fatalf("standalone: %v", err)
	}
	return declared
}

func surchargeRule(t testing.TB, id, code, threshold, amount string) domain.SurchargeRule {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, threshold, domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	currency := mustValue(t, domain.NewCurrency, "USD")
	calculation, err := domain.NewFixedAmountSurcharge(money(t, amount, currency))
	if err != nil {
		t.Fatalf("calculation: %v", err)
	}
	rule, err := domain.NewSurchargeRule(
		id,
		mustValue(t, domain.NewChargeCode, code),
		id,
		domain.ChargeEffectAdd,
		leafTrigger(t, condition),
		calculation,
	)
	if err != nil {
		t.Fatalf("surcharge rule %s: %v", id, err)
	}
	return rule
}

// planWithStructures keeps every declared version reference identical across
// calls so a digest difference can only come from the structures under test.
func planWithStructures(t testing.TB, structures domain.PricingPlanStructures) domain.PricingPlanVersion {
	t.Helper()
	plan, err := newPlanWithStructures(t, structures)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}

func newPlanWithStructures(t testing.TB, structures domain.PricingPlanStructures) (domain.PricingPlanVersion, error) {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-structures"),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-structures", "v1"),
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
		versionReference(t, domain.ArtifactWeightPolicy, "weight-structures", "v1"),
		domain.PricingWeightActualOnly,
		rounding,
		nil,
	)
	if err != nil {
		t.Fatalf("pricing weight policy: %v", err)
	}
	return domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-structures", "v1"),
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
}
