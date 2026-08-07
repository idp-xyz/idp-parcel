package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// L5 defines fuel as the basis times a rate the carrier publishes weekly, and
// F1 discounts that rate to 80%. Both numbers have to reach the amount: the
// published rate alone over-bills by a quarter, the factor alone is not a rate
// at all.
func TestFuelChargesThePublishedRateTimesTheCardFactor(t *testing.T) {
	plan := seriesPlan(t, "0.8")
	evaluation := evaluateWithSeries(t, plan, "eval-series-fuel", "20")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	// Basis is the 10 base freight; the effective rate is 20% × 0.8 = 16%.
	if total, _ := evaluation.Total(); total.Amount().String() != "11.6" {
		t.Fatalf("total = %s, want 11.6 = 10 + 10 × 16%%", total.Amount().String())
	}
}

// The series value is resolved per evaluation and frozen into the snapshot, so
// a plan bound to a series it was not given cannot be priced. That is missing
// evidence — the rate exists, this evaluation just was not handed it.
func TestPlanBoundToASeriesTheSnapshotDoesNotCarryStaysPending(t *testing.T) {
	plan := seriesPlan(t, "0.8")
	request, err := domain.NewEvaluationRequest(
		mustValue(t, domain.NewEvaluationID, "eval-series-missing"),
		plan,
		syntheticInputWithDimensions(t, "5", "Z1", dimensions(t, "50", "10", "10", domain.LengthUnitInch)),
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if status := domain.EvaluatePricing(request).Status(); status != domain.EvaluationPending {
		t.Fatalf("status = %s, want PENDING when the bound series was not supplied", status)
	}
}

// A snapshot carrying a different version of the series than the plan bound is
// not a gap but a disagreement: replaying with whichever happened to be handed
// over would silently price against a rate the plan never declared.
func TestSeriesVersionDisagreementIsAConflict(t *testing.T) {
	plan := seriesPlan(t, "0.8")
	input := syntheticInputWithDimensions(t, "5", "Z1", dimensions(t, "50", "10", "10", domain.LengthUnitInch))
	other, err := input.WithReferenceSeries(seriesValue(t, "fuel-weekly", "v2", "20"))
	if err != nil {
		t.Fatalf("attach series: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, "eval-series-version"), plan, other, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if status := domain.EvaluatePricing(request).Status(); status != domain.EvaluationConflict {
		t.Fatalf("status = %s, want CONFLICT when the supplied series version differs from the bound one", status)
	}
}

// CONTEXT: 燃油费率是承运商当周公布费率与价卡折扣系数的乘积，两者都必须写入版本清单，
// 只保留乘积结果视为解释不完整.
func TestFuelExplanationKeepsBothTheRateAndTheFactor(t *testing.T) {
	plan := seriesPlan(t, "0.8")
	evaluation := evaluateWithSeries(t, plan, "eval-series-explained", "20")

	if !explanationMentions(evaluation, "20") || !explanationMentions(evaluation, "0.8") {
		t.Fatalf("explanation = %#v, want both the published rate and the card factor", evaluation.Explanation())
	}
}

// Adding an optional series field must not disturb any evaluation that carries
// none: an absent field leaves the canonical bytes untouched, so every digest
// recorded before this slice stays comparable and no canonicalization version
// is owed.
func TestSnapshotWithoutSeriesKeepsItsDigest(t *testing.T) {
	plan := planWithStructures(t, standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "25")))
	first := evaluateWithSides(t, plan, "eval-series-neutral", "50")
	second := evaluateWithSides(t, plan, "eval-series-neutral", "50")
	if first.SemanticDigest() != second.SemanticDigest() {
		t.Fatal("an evaluation carrying no series is not reproducible")
	}
	if first.PlanCanonicalizationVersion() != domain.CurrentCanonicalizationVersion() {
		t.Fatalf("canonicalization = %q, want the unchanged current version", first.PlanCanonicalizationVersion())
	}
}

func seriesValue(t testing.TB, id, version, rate string) domain.ReferenceSeriesValue {
	t.Helper()
	value, err := domain.NewReferenceSeriesValue(
		domain.ReferenceSeriesFuelRate,
		versionReference(t, domain.ArtifactReferenceSeries, id, version),
		decimal(t, rate),
	)
	if err != nil {
		t.Fatalf("series value: %v", err)
	}
	return value
}

func evaluateWithSeries(t *testing.T, plan domain.PricingPlanVersion, id, rate string) domain.PricingEvaluation {
	t.Helper()
	input := syntheticInputWithDimensions(t, "5", "Z1", dimensions(t, "50", "10", "10", domain.LengthUnitInch))
	withSeries, err := input.WithReferenceSeries(seriesValue(t, "fuel-weekly", "v1", rate))
	if err != nil {
		t.Fatalf("attach series: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, id), plan, withSeries, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return domain.EvaluatePricing(request)
}

func seriesPlan(t *testing.T, factor string) domain.PricingPlanVersion {
	t.Helper()
	calculation, err := domain.NewSeriesRateSurcharge(domain.ReferenceSeriesFuelRate, decimal(t, factor), "freight")
	if err != nil {
		t.Fatalf("series calculation: %v", err)
	}
	rule := standaloneRule(t, surchargeRuleWithCalculation(t, "fuel", "FUEL", "48", calculation))
	binding, err := domain.NewReferenceSeriesBinding(
		domain.ReferenceSeriesFuelRate,
		versionReference(t, domain.ArtifactReferenceSeries, "fuel-weekly", "v1"),
	)
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{rule},
		[]domain.ChargeDependency{listedChargesBasis(t, "freight", "FUEL", "BASE_FREIGHT")},
		[]domain.ReferenceSeriesBinding{binding},
	)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return planWithStructures(t, structures)
}
