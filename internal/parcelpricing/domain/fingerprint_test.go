package domain

import (
	"errors"
	"testing"
	"time"
)

func TestChargeLineConstructorEnforcesKindContract(t *testing.T) {
	currency, _ := NewCurrency("USD")
	amount, _ := NewMoneyFromString("1", currency)
	code, _ := NewChargeCode("BASE_FREIGHT")
	if _, err := newChargeLine("base:bad", ChargeLineBase, code, ChargeScopePackage, ChargeBasisFixedAmount, ChargeMethodFixed, "Bad base", ChargeEffectAdd, amount, 0, "entry"); !errors.Is(err, ErrInvalidChargeLine) {
		t.Fatalf("mismatched base line error = %v", err)
	}
	if _, err := newChargeLine("fixed:bad", ChargeLineFixed, code, ChargeScopePackage, ChargeBasisFixedAmount, ChargeMethodFixed, "Bad fixed", ChargeEffectAdd, amount, 0, "rule"); !errors.Is(err, ErrInvalidChargeLine) {
		t.Fatalf("zero-order fixed line error = %v", err)
	}
}

func TestPricingEvaluationSemanticDigestIncludesIssueDetails(t *testing.T) {
	first := PricingEvaluation{issues: []EvaluationIssue{newEvaluationIssue("FIRST", "same message")}}
	second := PricingEvaluation{issues: []EvaluationIssue{newEvaluationIssue("SECOND", "same message")}}
	if hashPricingEvaluation(first) == hashPricingEvaluation(second) {
		t.Fatal("issue code was omitted from the semantic digest")
	}
	third := PricingEvaluation{issues: []EvaluationIssue{newEvaluationIssue("FIRST", "different message")}}
	if hashPricingEvaluation(first) == hashPricingEvaluation(third) {
		t.Fatal("issue message was omitted from the semantic digest")
	}
}

func TestPricingEvaluationSemanticDigestIncludesChargeLineCodeAndContract(t *testing.T) {
	plan, input := replayIntegrityFixture(t)
	id, _ := NewEvaluationID("eval-charge-line-digest")
	request, err := NewEvaluationRequest(id, plan, input, EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	evaluation := EvaluatePricing(request)
	if evaluation.status != EvaluationCompleted || len(evaluation.chargeLines) == 0 {
		t.Fatalf("evaluation = %#v", evaluation)
	}
	originalDigest := evaluation.SemanticDigest()
	changedCode, _ := NewChargeCode("BASE_FREIGHT_CHANGED")
	evaluation.chargeLines[0].chargeCode = changedCode
	evaluation.semanticDigest = evaluation.calculateSemanticDigest()
	if originalDigest == evaluation.SemanticDigest() {
		t.Fatal("charge code was omitted from the evaluation semantic digest")
	}
	evaluation.chargeLines[0].chargeCode = plan.baseChargeCode
	evaluation.chargeLines[0].basis = ChargeBasisFixedAmount
	evaluation.semanticDigest = evaluation.calculateSemanticDigest()
	if originalDigest == evaluation.SemanticDigest() {
		t.Fatal("charge basis was omitted from the evaluation semantic digest")
	}
}

func TestReplayPricingEvaluationDetectsChangedSemanticResult(t *testing.T) {
	plan, input := replayIntegrityFixture(t)
	originalID, err := NewEvaluationID("eval-original")
	if err != nil {
		t.Fatalf("original ID: %v", err)
	}
	request, err := NewEvaluationRequest(originalID, plan, input, EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	original := EvaluatePricing(request)

	currency, _ := NewCurrency("USD")
	extraAmount, _ := NewMoneyFromString("1", currency)
	code, _ := NewChargeCode("LEGACY_RUNTIME")
	extraLine, err := newFixedChargeLine("fixed:legacy-runtime", code, "Legacy runtime charge", ChargeEffectAdd, extraAmount, 1, "legacy-runtime")
	if err != nil {
		t.Fatalf("extra line: %v", err)
	}
	original.chargeLines = append(original.chargeLines, extraLine)
	changedTotalAmount, err := original.total.amount.Add(extraAmount.amount)
	if err != nil {
		t.Fatalf("changed total: %v", err)
	}
	changedTotal, err := NewMoney(changedTotalAmount, currency)
	if err != nil {
		t.Fatalf("changed total money: %v", err)
	}
	original.total = &changedTotal
	original.semanticDigest = original.calculateSemanticDigest()
	if !original.valid() {
		t.Fatal("test fixture must remain a structurally valid historical evaluation")
	}

	replayID, _ := NewEvaluationID("eval-replay")
	replayed, err := ReplayPricingEvaluation(replayID, original, plan, EvidenceSynthetic)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.status != EvaluationConflict || replayed.total != nil || len(replayed.issues) != 1 || replayed.issues[0].code != "REPLAY_RESULT_MISMATCH" {
		t.Fatalf("replayed evaluation = %#v", replayed)
	}
}

func TestReplayPricingEvaluationRejectsTamperedOriginalDigest(t *testing.T) {
	plan, input := replayIntegrityFixture(t)
	originalID, _ := NewEvaluationID("eval-tampered-original")
	request, _ := NewEvaluationRequest(originalID, plan, input, EvidenceSynthetic)
	original := EvaluatePricing(request)
	original.semanticDigest = "tampered"

	replayID, _ := NewEvaluationID("eval-tampered-replay")
	if _, err := ReplayPricingEvaluation(replayID, original, plan, EvidenceSynthetic); !errors.Is(err, ErrEvaluationRequestInvalid) {
		t.Fatalf("tampered original error = %v", err)
	}
}

func TestVersionManifestValidityRequiresCanonicalOrder(t *testing.T) {
	first, _ := NewVersionReference(ArtifactKind("custom"), "a", "v1", "digest-a")
	second, _ := NewVersionReference(ArtifactKind("custom"), "b", "v1", "digest-b")
	manifest, err := NewVersionManifest([]VersionReference{first, second})
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	manifest.references[0], manifest.references[1] = manifest.references[1], manifest.references[0]
	if manifest.valid() {
		t.Fatal("non-canonical manifest order was accepted")
	}
}

func replayIntegrityFixture(t *testing.T) (PricingPlanVersion, PricingInputSnapshot) {
	t.Helper()
	currency, _ := NewCurrency("USD")
	period, _ := NewEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	entryID, _ := NewRateEntryID("entry-replay")
	minimum, _ := NewWeightFromString("0", WeightUnitKilogram)
	maximum, _ := NewWeightFromString("10", WeightUnitKilogram)
	amount, _ := NewMoneyFromString("10", currency)
	entry, _ := NewRateEntry(entryID, "Z1", minimum, maximum, amount)
	tableReference, _ := NewVersionReference(ArtifactRateTable, "table-replay", "v1", "digest-table")
	table, _ := NewRateTableVersion(tableReference, RateTableKindWeightZone, currency, WeightUnitKilogram, period, []RateEntry{entry})
	increment, _ := NewWeightFromString("1", WeightUnitKilogram)
	rounding, _ := NewWeightRoundingPolicy(RoundingNone, increment)
	weightReference, _ := NewVersionReference(ArtifactWeightPolicy, "weight-replay", "v1", "digest-weight")
	weightPolicy, _ := NewBillableWeightPolicy(weightReference, BillableWeightActualOnly, rounding)
	planReference, _ := NewVersionReference(ArtifactPricingPlan, "plan-replay", "v1", "digest-plan")
	scope, _ := NewPricingScopeID("scope-replay")
	baseCode, _ := NewChargeCode("BASE_FREIGHT")
	plan, err := NewPricingPlanVersion(planReference, scope, PricingDirectionSell, PricingPurposeCustomerCharge, baseCode, period, table, weightPolicy, nil)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	tenantID, _ := NewTenantID("tenant-replay")
	packageID, _ := NewPackageID("package-replay")
	actualWeight, _ := NewWeightFromString("1", WeightUnitKilogram)
	input, err := NewPricingInputSnapshot(tenantID, scope, packageID, "Z1", actualWeight, nil, nil, time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("input: %v", err)
	}
	return plan, input
}
