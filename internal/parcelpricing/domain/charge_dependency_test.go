package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「依赖必须显式给出基数构成与排除集，不由声明顺序隐含」— L5 把燃油定义为
// 「（尾程派送费 + 除运费复核费以外的其他全部费用项）× 费率」：全部费用构成加一个点名的
// 排除项。排除集才是决定金额的那一半，所以它必须从声明里读出来，不能靠假定。
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
	// 基数是 10 基础运费 + 5 处理费 = 15，复核费被排除；10% 即 1.5。
	if total, _ := evaluation.Total(); total.Amount().String() != "19.5" {
		t.Fatalf("total = %s, want 19.5 = 10 + 5 + 3 + 1.5", total.Amount().String())
	}
}

// Covers: CONTEXT「基数构成的取值闭合，只有两种：本票全部其他费用，或逐项列举的费用代码」—
// 逐项列举的基数点的是它包含什么，不是它落下什么。两种构成不得互相坍缩：把列举型基数读成
// 「全部」，会悄悄放宽卡上燃油的计收范围。
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
	// 基数里只有处理费：5 的 10% 是 0.5。
	if total, _ := evaluation.Total(); total.Amount().String() != "18.5" {
		t.Fatalf("total = %s, want 18.5 = 10 + 5 + 3 + 0.5", total.Amount().String())
	}
}

// Covers: CONTEXT「存在环时评价为冲突」— 一个反过来够到它所供养的那笔费用的基数没有不动点，
// 随手挑一个求值顺序就是替它发明一个。
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
		// 连方案都建不起来，也是一个可以接受的截环位置，但环必须在某处被截住。
		return
	}
	evaluation := evaluateWithSides(t, plan, "eval-percent-cycle", "50")
	if evaluation.Status() != domain.EvaluationConflict {
		t.Fatalf("status = %s, issues = %#v, want CONFLICT for a circular basis", evaluation.Status(), evaluation.Issues())
	}
}

// 一笔百分比费用本身可以落在另一笔费用的基数里。按声明顺序而不是依赖顺序求解，会给一笔
// 尚未算出的费用读到零，从而悄悄少收。
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
	// 10 基础运费 + 10 处理费 + 5（处理费的 50%）+ 2.5（第一笔百分比的 50%）。
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
