package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PP-S03 keeps source-fact references in the pure evaluation contract. The
// settlement context may adopt the reference, but the pricing context does
// not create a fee item or accounting object.
func TestSyntheticContractPreservesFactReferencesAcrossEvaluationAndReplay(t *testing.T) {
	factReference, err := domain.NewVersionedFactReference(versionReference(t, domain.ArtifactKind("measurement"), "synthetic-measurement", "v1"))
	if err != nil {
		t.Fatalf("fact reference: %v", err)
	}
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"),
		"Z1",
		weight(t, "1.2", domain.WeightUnitKilogram),
		nil,
		nil,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
		factReference,
	)
	if err != nil {
		t.Fatalf("input: %v", err)
	}
	plan := syntheticPlan(t, "handoff-facts", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.BillableWeightActualOnly, nil)
	request, err := domain.NewEvaluationRequest(
		mustValue(t, domain.NewEvaluationID, "eval-handoff-facts"),
		plan,
		input,
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	evaluation := domain.EvaluatePricing(request)
	if evaluation.Status() != domain.EvaluationCompleted || evaluation.Evidence() != domain.EvidenceSynthetic {
		t.Fatalf("evaluation status/evidence = %s/%s", evaluation.Status(), evaluation.Evidence())
	}
	if got := evaluation.Input().FactReferences(); len(got) != 1 || got[0].Reference().ID() != "synthetic-measurement" {
		t.Fatalf("evaluation fact references = %#v", got)
	}

	replay, err := domain.ReplayPricingEvaluation(
		mustValue(t, domain.NewEvaluationID, "eval-handoff-facts-replay"),
		evaluation,
		plan,
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Status() != domain.EvaluationCompleted || replay.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatalf("replay status/digest = %s/%s", replay.Status(), replay.SemanticDigest())
	}
	if got := replay.Input().FactReferences(); len(got) != 1 || got[0].Reference().ID() != "synthetic-measurement" {
		t.Fatalf("replay fact references = %#v", got)
	}
}
