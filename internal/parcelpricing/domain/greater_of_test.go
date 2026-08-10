package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// CHG-016 就是 max(1.782, 运费 × 12%)：一个以定额表述的下限，对上运费的一个百分比。卡收
// 的是两者中较大的那个，所以两个操作数都得先求出值，才谈得上丢弃其中之一。
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

// 卡自己的形状就是一侧为百分比，所以「取较大值」可以像裸百分比一样依赖某个基数。在第一遍
// 就给它求值，会给百分比那一侧读到零，于是永远挑中那个定额下限。
func TestGreaterOfWithAPercentOperandWaitsForItsBasis(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	floor, err := domain.NewFixedAmountSurcharge(money(t, "1", currency))
	if err != nil {
		t.Fatalf("floor operand: %v", err)
	}
	// 10 基础运费的 50% 是 5，必须压过 1 的下限。
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
