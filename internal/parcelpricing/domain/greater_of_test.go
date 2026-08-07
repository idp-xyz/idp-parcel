package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// CHG-016 is max(1.782, 运费 × 12%): a floor stated as a flat amount against a
// share of the freight. Whichever is larger is what the card charges, so both
// operands have to be valued before either can be discarded.
func TestGreaterOfChargesTheLargerOperand(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	small, err := domain.NewFixedAmountSurcharge(money(t, "2", currency))
	if err != nil {
		t.Fatalf("small operand: %v", err)
	}
	large, err := domain.NewFixedAmountSurcharge(money(t, "9", currency))
	if err != nil {
		t.Fatalf("large operand: %v", err)
	}
	calculation, err := domain.NewGreaterOfSurcharge(small, large)
	if err != nil {
		t.Fatalf("greater-of: %v", err)
	}
	rule := standaloneRule(t, surchargeRuleWithCalculation(t, "handling", "HANDLING", "48", calculation))
	plan := planWithStructures(t, declaredSurcharges(t, rule))
	evaluation := evaluateWithSides(t, plan, "eval-greater-fixed", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "19" {
		t.Fatalf("total = %s, want 19 = 10 base + 9 larger operand", total.Amount().String())
	}
}

// The card's own shape has a percent on one side, so a greater-of can depend on
// a basis just as a bare percent does. Valuing it in the first pass would read
// a zero for the percent side and always pick the flat floor.
func TestGreaterOfWithAPercentOperandWaitsForItsBasis(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	floor, err := domain.NewFixedAmountSurcharge(money(t, "1", currency))
	if err != nil {
		t.Fatalf("floor operand: %v", err)
	}
	// 50% of the 10 base freight is 5, which must beat the 1 floor.
	share, err := domain.NewPercentOfBasisSurcharge(decimal(t, "50"), "freight")
	if err != nil {
		t.Fatalf("share operand: %v", err)
	}
	calculation, err := domain.NewGreaterOfSurcharge(floor, share)
	if err != nil {
		t.Fatalf("greater-of: %v", err)
	}
	rule := standaloneRule(t, surchargeRuleWithCalculation(t, "handling", "HANDLING", "48", calculation))
	plan := dependencyPlanWith(t,
		nil,
		[]domain.ChargeDependency{listedChargesBasis(t, "freight", "HANDLING", "BASE_FREIGHT")},
		rule,
	)
	evaluation := evaluateWithSides(t, plan, "eval-greater-percent", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "15" {
		t.Fatalf("total = %s, want 15 = 10 base + 5 (the percent side beating the 1 floor)", total.Amount().String())
	}
}
