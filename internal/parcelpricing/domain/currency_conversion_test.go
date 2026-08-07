package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// The card prices in USD while the anchor customer settles in CNY, and CONTEXT
// puts the conversion inside the evaluation: 计价按结算币种输出. Leaving it to the
// settlement side would make PricingEvaluation stop being the recomputable
// final price.
func TestEvaluationOutputsTheSettlementCurrency(t *testing.T) {
	evaluation := evaluateWithConversion(t, "eval-fx-total", "7.2")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	total, ok := evaluation.Total()
	if !ok || total.Currency().String() != "CNY" || total.Amount().String() != "72" {
		t.Fatalf("total = %s %s, want 72 CNY", total.Amount().String(), total.Currency())
	}
}

// CONTEXT: 换算必须保留原币金额与所引用的汇率序列版本，只保留结算币种金额视为解释
// 不完整. A dispute is argued in the original currency, so the pair has to
// survive on the evaluation rather than only in prose.
func TestConversionStepKeepsTheOriginalAmountAndTheSeriesVersion(t *testing.T) {
	evaluation := evaluateWithConversion(t, "eval-fx-step", "7.2")

	step, ok := evaluation.ConversionStep()
	if !ok {
		t.Fatal("a converted evaluation carried no conversion step")
	}
	if step.Original().Currency().String() != "USD" || step.Original().Amount().String() != "10" {
		t.Fatalf("original = %s %s, want 10 USD", step.Original().Amount().String(), step.Original().Currency())
	}
	if step.Rate().String() != "7.2" {
		t.Fatalf("rate = %s, want 7.2", step.Rate().String())
	}
	if step.SeriesReference().ID() != "fx-daily" {
		t.Fatalf("series reference = %s, want fx-daily", step.SeriesReference().ID())
	}
}

// CONTEXT: 汇率口径——牌价类型、取值时点规则和加点规则——由商业价格政策版本化声明；
// 不接受未声明口径的裸汇率. A number without a stated quote basis cannot be argued
// about later: nobody can say which rate it was supposed to be.
func TestBareExchangeRateWithoutADeclaredQuoteBasisIsRefused(t *testing.T) {
	// Refused at construction rather than at use: a bare rate should not exist
	// as a value in the first place.
	if _, err := domain.NewReferenceSeriesValue(
		domain.ReferenceSeriesExchangeRate,
		versionReference(t, domain.ArtifactReferenceSeries, "fx-daily", "v1"),
		decimal(t, "7.2"),
	); err == nil {
		t.Fatal("a bare exchange rate with no declared quote basis was accepted")
	}
	// A fuel rate needs no such declaration: its discount factor is on the card,
	// not in a commercial policy.
	if _, err := domain.NewReferenceSeriesValue(
		domain.ReferenceSeriesFuelRate,
		versionReference(t, domain.ArtifactReferenceSeries, "fuel-weekly", "v1"),
		decimal(t, "20"),
	); err != nil {
		t.Fatalf("fuel reading rejected: %v", err)
	}
}

// Settling in the currency the card is already priced in is not a conversion.
// Recording one anyway would put a rate of 1 into every evaluation and invite a
// reader to think a rate was applied.
func TestNoConversionStepWhenSettlementMatchesTheCardCurrency(t *testing.T) {
	plan := planWithStructures(t, declaredSurcharges(t))
	input, err := conversionInput(t).WithSettlementCurrency(mustValue(t, domain.NewCurrency, "USD"))
	if err != nil {
		t.Fatalf("settlement currency: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, "eval-fx-same"), plan, input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	evaluation := domain.EvaluatePricing(request)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	if _, converted := evaluation.ConversionStep(); converted {
		t.Fatal("settling in the card's own currency recorded a conversion step")
	}
}

// Asking for a settlement currency the evaluation was given no rate for is
// missing evidence, not a reason to fall back to the card's currency: falling
// back would hand the settlement side an amount in a currency it did not ask
// for and cannot tell apart from a converted one.
func TestSettlementCurrencyWithoutARateStaysPending(t *testing.T) {
	plan := planWithStructures(t, declaredSurcharges(t))
	input, err := conversionInput(t).WithSettlementCurrency(mustValue(t, domain.NewCurrency, "CNY"))
	if err != nil {
		t.Fatalf("settlement currency: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, "eval-fx-missing"), plan, input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if status := domain.EvaluatePricing(request).Status(); status != domain.EvaluationPending {
		t.Fatalf("status = %s, want PENDING without an exchange rate", status)
	}
}

// An evaluation that never converts must canonicalize exactly as before, so
// every digest recorded prior to this slice stays comparable.
func TestEvaluationWithoutConversionKeepsItsCanonicalizationVersion(t *testing.T) {
	plan := planWithStructures(t, declaredSurcharges(t))
	evaluation := evaluateWithSides(t, plan, "eval-fx-neutral", "50")
	if evaluation.PlanCanonicalizationVersion() != domain.CurrentCanonicalizationVersion() {
		t.Fatalf("canonicalization = %q, want the unchanged current version", evaluation.PlanCanonicalizationVersion())
	}
}

func conversionInput(t *testing.T) domain.PricingInputSnapshot {
	t.Helper()
	return syntheticInputWithDimensions(t, "5", "Z1", dimensions(t, "50", "10", "10", domain.LengthUnitInch))
}

func evaluateWithConversion(t *testing.T, id, rate string) domain.PricingEvaluation {
	t.Helper()
	quoted, err := domain.NewQuotedReferenceSeriesValue(
		domain.ReferenceSeriesExchangeRate,
		versionReference(t, domain.ArtifactReferenceSeries, "fx-daily", "v1"),
		decimal(t, rate),
		versionReference(t, domain.ArtifactCommercialPolicy, "fx-quote-basis", "v1"),
	)
	if err != nil {
		t.Fatalf("quoted series value: %v", err)
	}
	input, err := conversionInput(t).WithReferenceSeries(quoted)
	if err != nil {
		t.Fatalf("attach series: %v", err)
	}
	input, err = input.WithSettlementCurrency(mustValue(t, domain.NewCurrency, "CNY"))
	if err != nil {
		t.Fatalf("settlement currency: %v", err)
	}
	binding, err := domain.NewReferenceSeriesBinding(
		domain.ReferenceSeriesExchangeRate,
		versionReference(t, domain.ArtifactReferenceSeries, "fx-daily", "v1"),
	)
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(nil, nil, []domain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	request, err := domain.NewEvaluationRequest(
		mustValue(t, domain.NewEvaluationID, id),
		planWithStructures(t, structures),
		input,
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return domain.EvaluatePricing(request)
}
