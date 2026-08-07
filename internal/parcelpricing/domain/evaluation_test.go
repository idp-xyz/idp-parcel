package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func TestEvaluatePricingCalculatesMaxWeightAndOrderedFixedCharges(t *testing.T) {
	plan := syntheticPlan(
		t, "sell-1", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.BillableWeightMax,
		[]domain.FixedChargeRule{
			fixedRule(t, "discount", domain.ChargeEffectDeduct, "1", 2),
			fixedRule(t, "residential", domain.ChargeEffectAdd, "2", 1),
		},
	)
	input := syntheticInput(t, "1.2", stringPtr("1.6"), "Z1")
	evaluation := evaluate(t, "eval-sell-1", plan, input)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %v", evaluation.Status(), evaluation.Issues())
	}
	total, ok := evaluation.Total()
	if !ok || total.Amount().String() != "11" || total.Currency().String() != "USD" {
		t.Fatalf("total = %#v, present=%v", total, ok)
	}
	billable, ok := evaluation.BillableWeight()
	if !ok || billable.RawWeight().Value().String() != "1.6" || billable.RoundedWeight().Value().String() != "2" {
		t.Fatalf("billable = %#v, present=%v", billable, ok)
	}
	lines := evaluation.ChargeLines()
	if len(lines) != 3 || lines[0].Kind() != domain.ChargeLineBase || lines[0].Code().String() != "BASE_FREIGHT" || lines[1].ID() != "fixed:residential" || lines[1].Code().String() != "RULE_RESIDENTIAL" || lines[2].ID() != "fixed:discount" || lines[2].Code().String() != "RULE_DISCOUNT" {
		t.Fatalf("lines = %#v", lines)
	}
	if lines[0].Scope() != domain.ChargeScopePackage || lines[0].Basis() != domain.ChargeBasisRateEntry || lines[0].Method() != domain.ChargeMethodLookup {
		t.Fatalf("base line semantic fields = %s/%s/%s", lines[0].Scope(), lines[0].Basis(), lines[0].Method())
	}
	if lines[1].Scope() != domain.ChargeScopePackage || lines[1].Basis() != domain.ChargeBasisFixedAmount || lines[1].Method() != domain.ChargeMethodFixed {
		t.Fatalf("fixed line semantic fields = %s/%s/%s", lines[1].Scope(), lines[1].Basis(), lines[1].Method())
	}
	if evaluation.SemanticDigest() == "" {
		t.Fatal("missing semantic digest")
	}
}

func TestBaseChargeCodeIsStableAcrossRateEntries(t *testing.T) {
	currency := mustValue(t, domain.NewCurrency, "USD")
	firstEntry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-light"),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "1", domain.WeightUnitKilogram),
		money(t, "5", currency),
	)
	if err != nil {
		t.Fatalf("first entry: %v", err)
	}
	secondEntry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-heavy"),
		"Z1",
		weight(t, "1", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("second entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-tiered", "v1"),
		domain.RateTableKindWeightZone,
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{firstEntry, secondEntry},
	)
	if err != nil {
		t.Fatalf("table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingNone, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	weightPolicy, err := domain.NewBillableWeightPolicy(versionReference(t, domain.ArtifactWeightPolicy, "weight-tiered", "v1"), domain.BillableWeightActualOnly, rounding)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-tiered", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell,
		domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t),
		table,
		weightPolicy,
		nil,
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	light := evaluate(t, "eval-light", plan, syntheticInput(t, "0.5", nil, "Z1")).ChargeLines()[0]
	heavy := evaluate(t, "eval-heavy", plan, syntheticInput(t, "1.5", nil, "Z1")).ChargeLines()[0]
	if light.Code() != heavy.Code() || light.Code().String() != "BASE_FREIGHT" {
		t.Fatalf("base codes = %s/%s", light.Code().String(), heavy.Code().String())
	}
	if light.ID() == heavy.ID() || light.SourceReference() == heavy.SourceReference() {
		t.Fatalf("rate-entry identity was collapsed: %s/%s and %s/%s", light.ID(), heavy.ID(), light.SourceReference(), heavy.SourceReference())
	}
}

func TestEvaluatePricingReturnsPendingWithoutTotalForMissingFactsOrRate(t *testing.T) {
	maxPlan := syntheticPlan(t, "pending-weight", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, "8", domain.BillableWeightMax, nil)
	evaluation := evaluate(t, "eval-pending-weight", maxPlan, syntheticInput(t, "1", nil, "Z1"))
	if evaluation.Status() != domain.EvaluationPending || hasTotal(evaluation) || evaluation.Issues()[0].Code() != "VOLUMETRIC_WEIGHT_REQUIRED" {
		t.Fatalf("missing volumetric evaluation = %#v", evaluation)
	}

	actualPlan := syntheticPlan(t, "pending-rate", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, "8", domain.BillableWeightActualOnly, nil)
	evaluation = evaluate(t, "eval-pending-rate", actualPlan, syntheticInput(t, "1", nil, "UNKNOWN"))
	if evaluation.Status() != domain.EvaluationPending || hasTotal(evaluation) || evaluation.Issues()[0].Code() != "RATE_NOT_FOUND" {
		t.Fatalf("missing rate evaluation = %#v", evaluation)
	}
}

func TestEvaluatePricingFailsWithoutFormalTotalWhenDeductionExceedsSubtotal(t *testing.T) {
	plan := syntheticPlan(t, "negative", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.BillableWeightActualOnly, []domain.FixedChargeRule{
		fixedRule(t, "too-large-discount", domain.ChargeEffectDeduct, "11", 1),
	})
	evaluation := evaluate(t, "eval-negative", plan, syntheticInput(t, "1", nil, "Z1"))
	if evaluation.Status() != domain.EvaluationFailed || hasTotal(evaluation) {
		t.Fatalf("evaluation = %#v", evaluation)
	}
	if len(evaluation.Issues()) != 1 || evaluation.Issues()[0].Code() != "NEGATIVE_TOTAL" {
		t.Fatalf("issues = %#v", evaluation.Issues())
	}
}

func TestBuyAndSellPlansRemainIndependentAndReplayUsesFrozenManifest(t *testing.T) {
	sellPlan := syntheticPlan(t, "sell-independent", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "15", domain.BillableWeightActualOnly, nil)
	buyPlan := syntheticPlan(t, "buy-independent", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, "10", domain.BillableWeightActualOnly, nil)
	input := syntheticInput(t, "1", nil, "Z1")
	sell := evaluate(t, "eval-sell", sellPlan, input)
	buy := evaluate(t, "eval-buy", buyPlan, input)
	if sell.Direction() != domain.PricingDirectionSell || buy.Direction() != domain.PricingDirectionBuy {
		t.Fatalf("directions = %s/%s", sell.Direction(), buy.Direction())
	}
	sellTotal, _ := sell.Total()
	buyTotal, _ := buy.Total()
	if sellTotal.Amount().String() != "15" || buyTotal.Amount().String() != "10" {
		t.Fatalf("totals = %s/%s", sellTotal.Amount(), buyTotal.Amount())
	}
	if sell.Manifest().Equal(buy.Manifest()) {
		t.Fatal("BUY and SELL unexpectedly share a manifest")
	}

	replay, err := domain.ReplayPricingEvaluation(
		mustValue(t, domain.NewEvaluationID, "eval-sell-replay"), sell, sellPlan, domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Status() != domain.EvaluationCompleted || replay.SemanticDigest() != sell.SemanticDigest() {
		t.Fatalf("replay status/digest = %s/%s", replay.Status(), replay.SemanticDigest())
	}
	if replayID, ok := replay.ReplayOf(); !ok || replayID.String() != "eval-sell" {
		t.Fatalf("replay of = %v/%v", replayID, ok)
	}
	if _, err := domain.ReplayPricingEvaluation(sell.ID(), sell, sellPlan, domain.EvidenceSynthetic); !errors.Is(err, domain.ErrReplayEvaluationIDReuse) {
		t.Fatalf("reused replay ID error = %v", err)
	}
	if _, err := domain.ReplayPricingEvaluation(
		mustValue(t, domain.NewEvaluationID, "eval-sell-invalid-upgrade"), sell, sellPlan, domain.EvidenceReplay,
	); !errors.Is(err, domain.ErrEvaluationRequestInvalid) {
		t.Fatalf("synthetic evidence upgrade error = %v", err)
	}

	changedPlan := syntheticPlan(t, "sell-independent-v2", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "16", domain.BillableWeightActualOnly, nil)
	conflict, err := domain.ReplayPricingEvaluation(
		mustValue(t, domain.NewEvaluationID, "eval-sell-replay-conflict"), sell, changedPlan, domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("conflicting replay: %v", err)
	}
	if conflict.Status() != domain.EvaluationConflict || hasTotal(conflict) || len(conflict.Issues()) != 1 || conflict.Issues()[0].Code() != "VERSION_MANIFEST_MISMATCH" {
		t.Fatalf("conflicting replay = %#v", conflict)
	}
}

func TestEvaluationOutputsAreDefensivelyCopied(t *testing.T) {
	plan := syntheticPlan(t, "copy", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.BillableWeightActualOnly, nil)
	evaluation := evaluate(t, "eval-copy", plan, syntheticInput(t, "1", nil, "Z1"))
	lines := evaluation.ChargeLines()
	lines[0] = domain.ChargeLine{}
	if len(evaluation.ChargeLines()) != 1 || evaluation.ChargeLines()[0].ID() != "base:entry-copy" {
		t.Fatalf("charge lines were mutable")
	}
	references := evaluation.Manifest().References()
	references[0] = domain.VersionReference{}
	if len(evaluation.Manifest().References()) == 0 || evaluation.Manifest().References()[0].ID() == "" {
		t.Fatal("manifest was mutable")
	}
}

func TestReplayDetectsChangedPlanContentBehindSameVersionReferences(t *testing.T) {
	originalPlan := syntheticPlan(t, "content-fingerprint", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "15", domain.BillableWeightActualOnly, nil)
	changedRatePlan := syntheticPlan(t, "content-fingerprint", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "16", domain.BillableWeightActualOnly, nil)
	if !originalPlan.Manifest().Equal(changedRatePlan.Manifest()) {
		t.Fatal("test plans must share the same version manifest")
	}
	if originalPlan.ContentDigest() == changedRatePlan.ContentDigest() {
		t.Fatal("changed rate amount did not change the plan content digest")
	}
	original := evaluate(t, "eval-content-original", originalPlan, syntheticInput(t, "1", nil, "Z1"))
	replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-content-replay"), original, changedRatePlan, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Status() != domain.EvaluationConflict || hasTotal(replay) || len(replay.Issues()) != 1 || replay.Issues()[0].Code() != "PLAN_CONTENT_MISMATCH" {
		t.Fatalf("changed-content replay = %#v", replay)
	}
}

func TestReplayDetectsChangedChargeCodeBehindSameVersionReferences(t *testing.T) {
	originalPlan := syntheticPlanWithBaseCode(t, "charge-code-replay", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "15", "BASE_FREIGHT", domain.BillableWeightActualOnly, nil)
	changedPlan := syntheticPlanWithBaseCode(t, "charge-code-replay", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "15", "TRANSPORTATION", domain.BillableWeightActualOnly, nil)
	if !originalPlan.Manifest().Equal(changedPlan.Manifest()) {
		t.Fatal("test plans must share the same version manifest")
	}
	if originalPlan.ContentDigest() == changedPlan.ContentDigest() {
		t.Fatal("changed charge code did not change the plan content digest")
	}
	original := evaluate(t, "eval-charge-code-original", originalPlan, syntheticInput(t, "1", nil, "Z1"))
	replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-charge-code-replay"), original, changedPlan, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Status() != domain.EvaluationConflict || hasTotal(replay) || len(replay.Issues()) != 1 || replay.Issues()[0].Code() != "PLAN_CONTENT_MISMATCH" {
		t.Fatalf("changed-code replay = %#v", replay)
	}
}

func TestEvaluationRequiresBusinessTimeInsidePlanAndRateTablePeriods(t *testing.T) {
	plan := syntheticPlan(t, "period", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.BillableWeightActualOnly, nil)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		at   time.Time
		want domain.EvaluationStatus
	}{
		{"start inclusive", start, domain.EvaluationCompleted},
		{"end exclusive", end, domain.EvaluationConflict},
		{"before start", start.Add(-time.Nanosecond), domain.EvaluationConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			evaluation := evaluate(t, "eval-period-"+test.name, plan, syntheticInputAt(t, "1", nil, "Z1", test.at))
			if evaluation.Status() != test.want {
				t.Fatalf("status = %s, want %s; issues = %#v", evaluation.Status(), test.want, evaluation.Issues())
			}
			if test.want != domain.EvaluationCompleted && hasTotal(evaluation) {
				t.Fatal("out-of-period evaluation has a formal total")
			}
		})
	}
}

func hasTotal(evaluation domain.PricingEvaluation) bool {
	_, ok := evaluation.Total()
	return ok
}

func stringPtr(value string) *string {
	return &value
}
