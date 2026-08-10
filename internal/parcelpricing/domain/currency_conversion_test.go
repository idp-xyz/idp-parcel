package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「换算步骤」—「一次评价把原币金额换算为结算币种金额的过程记录」：卡按
// USD 定价而锚点货主客户按 CNY 结算，换算落在评价之内。丢给结算侧去做，PricingEvaluation
// 就不再是那个可复算的最终价格了。
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

// Covers: CONTEXT「换算必须保留原币金额与所引用的汇率序列版本，只保留结算币种金额视为解释
// 不完整」— 争议是按原币争的，所以这一对必须活在评价上，而不是只活在说明文字里。
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

// Covers: CONTEXT「汇率口径——牌价类型、取值时点规则和加点规则——由商业价格政策版本化
// 声明；不接受未声明口径的裸汇率」— 一个没有口径的数字日后没法争：谁也说不出它本该是哪
// 一个汇率。
func TestBareExchangeRateWithoutADeclaredQuoteBasisIsRefused(t *testing.T) {
	// 在构造期而不是使用期拒绝：裸汇率本来就不该作为一个取值存在。
	if _, err := domain.NewReferenceSeriesValue(
		domain.ReferenceSeriesExchangeRate,
		versionReference(t, domain.ArtifactReferenceSeries, "fx-daily", "v1"),
		decimal(t, "7.2"),
	); err == nil {
		t.Fatal("a bare exchange rate with no declared quote basis was accepted")
	}
	// 燃油费率不需要这类声明：它的折扣系数在卡上，不在商业价格政策里。
	if _, err := domain.NewReferenceSeriesValue(
		domain.ReferenceSeriesFuelRate,
		versionReference(t, domain.ArtifactReferenceSeries, "fuel-weekly", "v1"),
		decimal(t, "20"),
	); err != nil {
		t.Fatalf("fuel reading rejected: %v", err)
	}
}

// 按卡本来就定价所用的币种结算，不算一次换算。硬记一笔会把一个 1 的汇率塞进每一次评价，
// 引着读的人以为真的用过某个汇率。
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

// 要一个评价没拿到汇率的结算币种，是证据缺失，不是退回卡币种的理由：退回会把一个结算侧
// 没有要过的币种金额交给它，而它分不出这与一个换算过的金额有何不同。
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

// Covers: CONTEXT「摘要携带产生它的规范化版本，只在同一规范化版本内可比」— 从不换算的评价
// 必须与从前一模一样地规范化，这样本切片之前记下的每一个摘要都仍然可比。
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
