package parcelpricing

import (
	"context"
	"testing"
	"time"

	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

type fixedTrialClock struct{ at time.Time }

func (clock fixedTrialClock) Now() time.Time { return clock.at }

// evaluatedFor 经计价真入口算出两件试算评价，各按给定的服务产品记在输入快照上；空串即没申报。各件按不同产品算出来，正常的选卡
// 路径给不出，只能在这里直接问给定卡的那一口。
func evaluatedFor(t *testing.T, products ...string) []ppapplication.PreAcceptanceParcelEstimate {
	t.Helper()
	at := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	plan := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-PLAN-SELL", "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-TABLE-SELL", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-WEIGHT", "v1"),
		Scope:                 "SYN-SCOPE-SELL",
		Direction:             ppdomain.PricingDirectionSell,
		Purpose:               ppdomain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "rate-z1",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "12.50",
		WeightRounding:        ppdomain.RoundingCeiling,
		WeightStepKilograms:   "1",
	})
	handler, err := ppapplication.NewFormPlanGivenEstimateEvaluationsHandler(ppapplication.FormPlanGivenEstimateEvaluationsDeps{
		Clock: fixedTrialClock{at: at.Add(time.Minute)},
	})
	if err != nil {
		t.Fatalf("plan-given handler: %v", err)
	}
	weight, err := ppdomain.NewWeightFromString("1", ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("weight: %v", err)
	}
	declarations := make([]ppapplication.PlanGivenEstimateDeclaration, 0, len(products))
	for index, product := range products {
		declaration := ppapplication.PlanGivenEstimateDeclaration{
			Subject: "SHR-SYN-01/SV-SYN-01/P-" + string(rune('1'+index)), BasisAt: at, Weight: weight, Zone: "Z1",
		}
		if product != "" {
			declaration.ServiceProduct = pptest.Value(t, ppdomain.NewServiceProductReference, product)
		}
		declarations = append(declarations, declaration)
	}
	formed, err := handler.Handle(context.Background(), ppapplication.FormPlanGivenEstimateEvaluationsCommand{
		Tenant:       pptest.Value(t, ppdomain.NewTenantID, "SYN-T1"),
		Plan:         plan,
		Evidence:     ppdomain.EvidenceSynthetic,
		Declarations: declarations,
	})
	if err != nil || formed.Outcome != ppapplication.PlanGivenEstimatesFormed || len(formed.Estimates) != len(products) {
		t.Fatalf("plan-given = %+v err = %v", formed, err)
	}
	estimates := make([]ppapplication.PreAcceptanceParcelEstimate, 0, len(formed.Estimates))
	for index, estimate := range formed.Estimates {
		estimates = append(estimates, ppapplication.PreAcceptanceParcelEstimate{
			Parcel: "P-" + string(rune('1'+index)), Answer: estimate.Answer, Evaluation: estimate.Evaluation,
		})
	}
	return estimates
}

// Covers: 票 declared-goods-lines/19 评审、ADR-0379 决定一「补记不得挡住估价形成」——各件评价记的服务产品不同时不记产品、也不报错，
// 每一件照样译得出有合计的试算；各件相同才记那一个，各件都没申报即空。
func TestParcelsPricedForDifferentServiceProductsRecordNoProduct(t *testing.T) {
	disagreeing := evaluatedFor(t, "SYN-PRODUCT-A", "SYN-PRODUCT-B")
	if product := serviceProductOf(disagreeing); product != "" {
		t.Fatalf("各件不同却记了 %q", product)
	}
	for _, estimate := range disagreeing {
		trial, err := trialOf(estimate)
		if err != nil {
			t.Fatalf("trial: %v", err)
		}
		if _, _, present := trial.Total(); !present {
			t.Fatalf("试算 %+v 没有合计", trial)
		}
	}
	if product := serviceProductOf(evaluatedFor(t, "SYN-PRODUCT-A", "SYN-PRODUCT-A")); product != "SYN-PRODUCT-A" {
		t.Fatalf("各件相同记成 %q", product)
	}
	if product := serviceProductOf(evaluatedFor(t, "", "")); product != "" {
		t.Fatalf("没申报记成 %q", product)
	}
	if product := serviceProductOf(evaluatedFor(t, "SYN-PRODUCT-A", "")); product != "" {
		t.Fatalf("一件申报一件没有却记了 %q", product)
	}
}
