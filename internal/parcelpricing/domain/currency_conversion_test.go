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

// 与 evaluateWithConversion 同路，只是多声明一个比较币种。
func evaluateWithComparison(t *testing.T, id, rate, comparison string) domain.PricingEvaluation {
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
	input, err = input.WithComparisonCurrency(mustValue(t, domain.NewCurrency, comparison))
	if err != nil {
		t.Fatalf("comparison currency: %v", err)
	}
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesExchangeRate, "fx-daily")
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(nil, nil, []domain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, id), planWithStructures(t, structures), input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return domain.EvaluatePricing(request)
}

// Covers: ADR-0148 决定四——比较币种等于卡币种时不发生换算，比较金额就是原币精确合计；
// 结算侧的换算与取整照旧，比较金额不过卡的取整点。
func TestComparisonCurrencyEqualToTheCardCurrencyKeepsTheExactPricedTotal(t *testing.T) {
	evaluation := evaluateWithComparison(t, "eval-compare-same", "7.2", "USD")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	comparison, ok := evaluation.ComparisonAmount()
	if !ok || comparison.Currency().String() != "USD" || comparison.Amount().String() != "10" {
		t.Fatalf("comparison amount = %s %s, want 10 USD", comparison.Amount().String(), comparison.Currency())
	}
	if _, converted := evaluation.ComparisonStep(); converted {
		t.Fatal("comparing in the card's own currency recorded a comparison conversion")
	}
	total, ok := evaluation.Total()
	if !ok || total.Currency().String() != "CNY" || total.Amount().String() != "72" {
		t.Fatalf("total = %s %s, want 72 CNY（结算侧不受比较币种影响）", total.Amount().String(), total.Currency())
	}
}

// Covers: ADR-0148 决定四——比较币种异于卡币种时，换算一律从原币合计出发，与结算换算
// 同一条汇率读数、互不为基准。
func TestComparisonCurrencyConversionKeepsItsOwnStep(t *testing.T) {
	evaluation := evaluateWithComparison(t, "eval-compare-step", "7.2", "EUR")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	comparison, ok := evaluation.ComparisonAmount()
	if !ok || comparison.Currency().String() != "EUR" || comparison.Amount().String() != "72" {
		t.Fatalf("comparison amount = %s %s, want 72 EUR", comparison.Amount().String(), comparison.Currency())
	}
	step, ok := evaluation.ComparisonStep()
	if !ok {
		t.Fatal("a converted comparison carried no step")
	}
	if step.Original().Currency().String() != "USD" || step.Original().Amount().String() != "10" {
		t.Fatalf("original = %s %s, want 10 USD", step.Original().Amount().String(), step.Original().Currency())
	}
	if step.Rate().String() != "7.2" || step.SeriesReference().ID() != "fx-daily" {
		t.Fatalf("step = rate %s series %s, want 7.2 fx-daily", step.Rate(), step.SeriesReference().ID())
	}
	total, ok := evaluation.Total()
	if !ok || total.Currency().String() != "CNY" || total.Amount().String() != "72" {
		t.Fatalf("total = %s %s, want 72 CNY", total.Amount().String(), total.Currency())
	}
}

// 比较币种要汇率而方案没声明汇率绑定：与结算币种同一格差异，如实落待判断，不退卡币种。
func TestComparisonCurrencyWithoutARateStaysPending(t *testing.T) {
	plan := planWithStructures(t, declaredSurcharges(t))
	input, err := conversionInput(t).WithComparisonCurrency(mustValue(t, domain.NewCurrency, "CNY"))
	if err != nil {
		t.Fatalf("comparison currency: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, "eval-compare-missing"), plan, input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if status := domain.EvaluatePricing(request).Status(); status != domain.EvaluationPending {
		t.Fatalf("status = %s, want PENDING without an exchange rate", status)
	}
}

// 未声明比较币种的评价不答比较格：缺席不是零值。
func TestNoComparisonAmountWithoutADeclaredComparisonCurrency(t *testing.T) {
	evaluation := evaluateWithConversion(t, "eval-no-compare", "7.2")
	if _, ok := evaluation.ComparisonAmount(); ok {
		t.Fatal("an evaluation without a comparison currency still answered a comparison amount")
	}
}

// 比较作答随快照往返：读回的评价答出同一笔比较金额与换算步骤，且整图重验仍成立。
func TestComparisonResultSurvivesSnapshotRoundTrip(t *testing.T) {
	original := evaluateWithComparison(t, "eval-compare-roundtrip", "7.2", "EUR")

	raw, err := domain.MarshalEvaluationSnapshot(original)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	rebuilt, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate snapshot: %v", err)
	}
	amount, ok := rebuilt.ComparisonAmount()
	if !ok || amount.Currency().String() != "EUR" || amount.Amount().String() != "72" {
		t.Fatalf("rebuilt comparison amount = %s %s, want 72 EUR", amount.Amount().String(), amount.Currency())
	}
	step, ok := rebuilt.ComparisonStep()
	if !ok || step.SeriesReference().ID() != "fx-daily" || step.Rate().String() != "7.2" {
		t.Fatalf("rebuilt comparison step = %+v, want the fx-daily step at 7.2", step)
	}
	if rebuilt.SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("rebuilt digest %q differs from original %q", rebuilt.SemanticDigest(), original.SemanticDigest())
	}
}

// 摘要必须盖住比较作答：只差一个比较币种的两次评价，摘要必须不同——比较值进了路由判断的
// 留痕，摘要漏了它，重放就查不出比较口径被换过。
func TestDigestChangesWithTheComparisonResult(t *testing.T) {
	withComparison := evaluateWithComparison(t, "eval-digest-compare", "7.2", "EUR")
	without := evaluateWithConversion(t, "eval-digest-plain", "7.2")
	if withComparison.SemanticDigest() == without.SemanticDigest() {
		t.Fatal("declaring a comparison currency did not change the semantic digest")
	}
}

// Covers: ISO 4217 最小币单位基准——0 位与 3 位的异常集各验一格，2 位是缺省。
func TestCurrencyMinorUnitScale(t *testing.T) {
	cases := []struct {
		code  string
		scale uint8
	}{
		{"CNY", 2}, {"USD", 2}, {"SGD", 2}, {"EUR", 2},
		{"JPY", 0}, {"KRW", 0}, {"VND", 0},
		{"KWD", 3}, {"BHD", 3}, {"JOD", 3},
	}
	for _, test := range cases {
		currency := mustValue(t, domain.NewCurrency, test.code)
		if scale := currency.MinorUnitScale(); scale != test.scale {
			t.Fatalf("%s minor unit scale = %d, want %d", test.code, scale, test.scale)
		}
	}
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
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesExchangeRate, "fx-daily")
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
