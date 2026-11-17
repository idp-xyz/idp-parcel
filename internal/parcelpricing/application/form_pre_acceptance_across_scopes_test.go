package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// Covers: ADR-0380 决定三在受理前试算上的那一条——两件包裹各自按列表问，可以落在不同的范围和卡上。

func weightBandPlan(t *testing.T, scope, planID, maximum string) domain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, domain.ArtifactPricingPlan, planID, "v1"),
		TableReference:        pptest.IdentityReference(t, domain.ArtifactRateTable, planID+"-table", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, domain.ArtifactWeightPolicy, planID+"-weight", "v1"),
		Scope:                 scope,
		Direction:             domain.PricingDirectionSell,
		Purpose:               domain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "USD",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         planID + "-entry",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    maximum,
		RateAmount:          "12",
		WeightRounding:      domain.RoundingCeiling,
		WeightStepKilograms: "0.5",
	})
}

func TestPreAcceptanceParcelsCanLandOnDifferentSellPriceSources(t *testing.T) {
	light := weightBandPlan(t, "SYN-SCOPE-A", "SYN-PLAN-A", "2")
	heavy := weightBandPlan(t, "SYN-SCOPE-B", "SYN-PLAN-B", "10")
	cards := &scopedPriceCards{byScope: map[string]ports.PriceCardInForceResolution{
		"SYN-SCOPE-A": inForceCard(light),
		"SYN-SCOPE-B": inForceCard(heavy),
	}}
	one := mustWeight(t, "1")
	five := mustWeight(t, "5")
	declarations := &submissionDeclarationDouble{found: true, declaration: ports.SubmissionDeclaration{Parcels: []ports.DeclaredParcel{
		{Parcel: "SYN-PARCEL-1", Weight: &one},
		{Parcel: "SYN-PARCEL-2", Weight: &five},
	}}}
	estimates := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{})
	handler, err := application.NewFormPreAcceptanceEstimatesHandler(application.FormPreAcceptanceEstimatesDeps{
		PriceCards: cards, Declarations: declarations, Estimates: estimates, Evidence: domain.EvidenceSynthetic,
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	command := preAcceptanceCommand(t)
	command.Scope = mustValue(t, domain.NewPricingScopeID, "SYN-SCOPE-A")
	command.Scopes = []domain.PricingScopeID{
		mustValue(t, domain.NewPricingScopeID, "SYN-SCOPE-A"),
		mustValue(t, domain.NewPricingScopeID, "SYN-SCOPE-B"),
	}
	command.Zone = "Z1"

	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.PreAcceptanceEstimatesFormed || len(result.Estimates) != 2 {
		t.Fatalf("outcome = %s estimates = %d", result.Outcome, len(result.Estimates))
	}
	first, second := result.Estimates[0], result.Estimates[1]
	if first.Scope.String() != "SYN-SCOPE-A" || first.Evaluation.PlanReference() != light.Reference() {
		t.Fatalf("轻件落在 %s 卡 %s", first.Scope, first.Evaluation.PlanReference())
	}
	if second.Scope.String() != "SYN-SCOPE-B" || second.Evaluation.PlanReference() != heavy.Reference() {
		t.Fatalf("重件落在 %s 卡 %s trail = %v status = %s issues = %v",
			second.Scope, second.Evaluation.PlanReference(), second.SourceTrail, second.Evaluation.Status(), second.Evaluation.Issues())
	}
	if len(second.SourceTrail) != 2 || second.SourceTrail[0].Kind != application.SellPriceSourceBaseRateMissing {
		t.Fatalf("重件的来源 = %+v，想要先落空再选中后一个", second.SourceTrail)
	}
}
