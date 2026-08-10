package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「燃油费率是承运商当周公布费率与价卡折扣系数的乘积」— L5 把燃油定为基数
// 乘以承运商每周公布的费率，F1 又把该费率打到 80%。两个数都必须走到金额里：只用公布费率
// 会多收四分之一，只用系数则根本不是一个费率。
func TestFuelChargesThePublishedRateTimesTheCardFactor(t *testing.T) {
	plan := seriesPlan(t, "0.8")
	evaluation := evaluateWithSeries(t, plan, "eval-series-fuel", "20")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	// 基数是 10 的基础运费；实际费率为 20% × 0.8 = 16%。
	if total, _ := evaluation.Total(); total.Amount().String() != "11.6" {
		t.Fatalf("total = %s, want 11.6 = 10 + 10 × 16%%", total.Amount().String())
	}
}

// 序列取值按每次评价解析并冻结进快照，所以绑定了序列却没拿到该取值的方案算不出价。那是
// 证据缺失——费率是存在的，只是这次评价没被递到。
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

// 快照带的序列版本与方案所绑的不同，这不是空档而是分歧：拿碰巧递过来的那个去重放，等于
// 悄悄按一个方案从未声明过的费率计价。
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

// Covers: CONTEXT「燃油费率是承运商当周公布费率与价卡折扣系数的乘积，两者都必须写入版本
// 清单，只保留乘积结果视为解释不完整」。
func TestFuelExplanationKeepsBothTheRateAndTheFactor(t *testing.T) {
	plan := seriesPlan(t, "0.8")
	evaluation := evaluateWithSeries(t, plan, "eval-series-explained", "20")

	if !explanationMentions(evaluation, "20") || !explanationMentions(evaluation, "0.8") {
		t.Fatalf("explanation = %#v, want both the published rate and the card factor", evaluation.Explanation())
	}
}

// 新增一个可选的序列字段不得惊动任何不带它的评价：字段缺席时规范化字节原样不变，因而本
// 切片之前记下的每一个摘要仍然可比，也不欠一个新的规范化版本。
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
