package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: AT-PP-033
// 总额上下限对的是评价主体的合计：逐委托的方案上，按件计收的行与每主体一次的行同在各行之和里，补差行标主体那一级的单位，
// 读回门认这种单位（ADR-0348 决定二）。
func TestBoundsApplyToTheShipmentAggregateTotalAndTheSupplementIsScopedToTheSubject(t *testing.T) {
	base := mustAggregatePlan(t, domain.AggregationPerShipment, domain.PricingWeightActualOnly, perPiece(t, fixedRule(t, "handling", domain.ChargeEffectAdd, "2", 1)))
	structures := structuresWithBounds(t, domain.PricingPlanStructures{}, boundsOf(t, "30", ""))
	plan, err := domain.NewAggregatePricingPlanVersion(domain.AggregationPerShipment, base.Reference(), base.Scope(), base.Direction(),
		base.Purpose(), base.BaseChargeCode(), base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), base.Rules(), structures)
	if err != nil {
		t.Fatalf("带总额下限的逐委托方案：%v", err)
	}

	input := aggregateInput(t, shipmentSubject(t, "SYN-SHIPMENT-1"), members(t, "5", nil, "pkg-1", "pkg-2", "pkg-3"))
	evaluation := evaluate(t, "eval-bounds-shipment", plan, input)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("状态 %s，问题项 %v", evaluation.Status(), evaluation.Issues())
	}
	// 基础运费 10（合计 5 kg 查表）+ 按件 2 × 3 件 = 16，低于下限 30，补差 14。
	if got := totalOf(t, evaluation); got != "30" || lineAmounts(evaluation)["MINIMUM_CHARGE"] != "14" {
		t.Fatalf("合计 %s，行 %v，手算合计 30、补差 14", got, lineAmounts(evaluation))
	}
	lines := evaluation.ChargeLines()
	if lines[len(lines)-1].Scope() != domain.ChargeScopeShipment {
		t.Fatalf("补差行的聚合单位 %s，应是主体那一级 SHIPMENT", lines[len(lines)-1].Scope())
	}

	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("折装评价快照：%v", err)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil || restored.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatalf("评价快照往返：%v", err)
	}
}
