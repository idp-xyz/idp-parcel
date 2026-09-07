package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件钉 ADR-0107：金额取整策略是价卡内容（模式 + 进位单位 + 应用点），进内容摘要；评价在已声明
// 的点上按固定先后取整并留痕；未声明即不取整并记问题项，不给默认。取值全是 SYN（只记 S）。

func amountPolicy(t testing.TB, mode domain.RoundingMode, increment string, points ...domain.AmountRoundingPoint) domain.AmountRoundingPolicy {
	t.Helper()
	policy, err := domain.NewAmountRoundingPolicy(mode, money(t, increment, mustValue(t, domain.NewCurrency, "USD")), points)
	if err != nil {
		t.Fatalf("amount rounding policy: %v", err)
	}
	return policy
}

func structuresWithRounding(t testing.TB, base domain.PricingPlanStructures, policy domain.AmountRoundingPolicy) domain.PricingPlanStructures {
	t.Helper()
	declared, err := base.WithAmountRounding(policy)
	if err != nil {
		t.Fatalf("with amount rounding: %v", err)
	}
	return declared
}

// planWithBaseAndStructures 造一张基础运费金额可指定的 SYN 卡：`12.5` 与 `12.50` 两种写法要比得出
// 同一个合计，写法只能从这里进。
func planWithBaseAndStructures(t testing.TB, baseAmount string, structures domain.PricingPlanStructures) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-rounding"),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, baseAmount, currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-rounding", "v1"),
		domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram, effectivePeriod(t),
		[]domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingNone, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("weight rounding: %v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-rounding", "v1"),
		domain.PricingWeightActualOnly, rounding, nil,
	)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-rounding", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t), table, weightPolicy, nil, structures,
	)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}

func fxStructures(t testing.TB) domain.PricingPlanStructures {
	t.Helper()
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesExchangeRate, "fx-daily")
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(nil, nil, []domain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func evaluateConverted(t testing.TB, id string, plan domain.PricingPlanVersion, rate string) domain.PricingEvaluation {
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
	input, err := syntheticInput(t, "5", "Z1").WithReferenceSeries(quoted)
	if err != nil {
		t.Fatalf("attach series: %v", err)
	}
	input, err = input.WithSettlementCurrency(mustValue(t, domain.NewCurrency, "CNY"))
	if err != nil {
		t.Fatalf("settlement currency: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, id), plan, input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return domain.EvaluatePricing(request)
}

func hasIssue(evaluation domain.PricingEvaluation, code string) bool {
	for _, issue := range evaluation.Issues() {
		if issue.Code() == code {
			return true
		}
	}
	return false
}

// Covers: ADR-0107 Decision 二、四——声明合计 HALF_UP / 0.01 的卡，USD 基价 × 四位汇率换算后合计落在
// 两位小数且留痕在解释项与取整记录里；同卡去掉声明，合计仍是精确十进制并带「金额精度未声明」问题
// 项；两格各自重放语义摘要不变。
func TestDeclaredTotalRoundingLandsOnTheIncrementAndUndeclaredCardsStayExact(t *testing.T) {
	declared := planWithBaseAndStructures(t, "10", structuresWithRounding(t, fxStructures(t),
		amountPolicy(t, domain.RoundingHalfUp, "0.01", domain.AmountRoundingTotal)))
	rounded := evaluateConverted(t, "eval-rounded-total", declared, "7.2345")
	if rounded.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", rounded.Status(), rounded.Issues())
	}
	total, _ := rounded.Total()
	if total.Amount().String() != "72.35" || total.Currency().String() != "CNY" {
		t.Fatalf("total = %s %s, want 72.35 CNY（72.345 按 HALF_UP 到 0.01）", total.Amount().String(), total.Currency())
	}
	steps := rounded.AmountRounding()
	if len(steps) != 1 || steps[0].Point() != domain.AmountRoundingTotal ||
		steps[0].Before().Amount().String() != "72.345" || steps[0].After().Amount().String() != "72.35" ||
		steps[0].Mode() != domain.RoundingHalfUp || steps[0].Increment().Amount().String() != "0.01" {
		t.Fatalf("取整留痕 = %#v", steps)
	}
	if !explanationMentions(rounded, "rounded TOTAL 72.345 CNY to 72.35 CNY (HALF_UP to 0.01)") {
		t.Fatalf("解释项没记这次取整：%v", rounded.Explanation())
	}
	if hasIssue(rounded, "AMOUNT_PRECISION_UNDECLARED") {
		t.Fatal("声明了策略的卡不该带「金额精度未声明」")
	}

	undeclared := planWithBaseAndStructures(t, "10", fxStructures(t))
	exact := evaluateConverted(t, "eval-exact-total", undeclared, "7.2345")
	if exact.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", exact.Status(), exact.Issues())
	}
	if total, _ := exact.Total(); total.Amount().String() != "72.345" {
		t.Fatalf("未声明的卡合计 = %s, want 精确的 72.345", total.Amount().String())
	}
	if !hasIssue(exact, "AMOUNT_PRECISION_UNDECLARED") || len(exact.AmountRounding()) != 0 {
		t.Fatalf("未声明的卡该带问题项且无取整留痕：%#v / %#v", exact.Issues(), exact.AmountRounding())
	}

	for name, pair := range map[string]struct {
		plan       domain.PricingPlanVersion
		evaluation domain.PricingEvaluation
	}{"declared": {declared, rounded}, "undeclared": {undeclared, exact}} {
		replayed, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "replay-"+name), pair.evaluation, pair.plan, domain.EvidenceSynthetic)
		if err != nil {
			t.Fatalf("%s replay: %v", name, err)
		}
		if replayed.Status() != domain.EvaluationCompleted || replayed.SemanticDigest() != pair.evaluation.SemanticDigest() {
			t.Fatalf("%s 重放 = %s / %s，想要同一语义摘要 %s；issues=%#v", name, replayed.Status(), replayed.SemanticDigest(), pair.evaluation.SemanticDigest(), replayed.Issues())
		}
	}
}

// Covers: 票 pricing-amount-precision/02 完成判据——`12.5` 与 `12.50` 两种写法的价表在声明合计取整后
// 得到同一个合计（含同一 scale）。
func TestEquivalentDecimalSpellingsShareOneRoundedTotal(t *testing.T) {
	policy := amountPolicy(t, domain.RoundingHalfUp, "0.01", domain.AmountRoundingTotal)
	structures := structuresWithRounding(t, mustStructures(t), policy)
	short := evaluateWithSides(t, planWithBaseAndStructures(t, "12.5", structures), "eval-spelling-short", "5")
	long := evaluateWithSides(t, planWithBaseAndStructures(t, "12.50", structures), "eval-spelling-long", "5")
	shortTotal, _ := short.Total()
	longTotal, _ := long.Total()
	// Decimal 的算术结果是规范形（去尾零），所以两者都写成 12.5；消费方要的 scale 从取整留痕的进位单位取。
	if shortTotal.Amount().String() != "12.5" || longTotal.Amount().String() != "12.5" || !shortTotal.Equal(longTotal) {
		t.Fatalf("totals = %s / %s, want 同一个 12.5", shortTotal.Amount().String(), longTotal.Amount().String())
	}
}

// Covers: ADR-0107 Decision 二的顺序——逐行先于换算后先于合计，三点各留一痕；逐行取整改的是费用行
// 上的金额（费用行之和仍等于换算前的原币金额）。
func TestRoundingPointsApplyInTheFixedOrder(t *testing.T) {
	rules := []domain.FixedChargeRule{fixedRule(t, "handling", domain.ChargeEffectAdd, "1.005", 1)}
	policy := amountPolicy(t, domain.RoundingHalfUp, "0.01",
		domain.AmountRoundingTotal, domain.AmountRoundingPerLine, domain.AmountRoundingAfterConversion)
	if got := policy.Points(); len(got) != 3 || got[0] != domain.AmountRoundingPerLine || got[1] != domain.AmountRoundingAfterConversion || got[2] != domain.AmountRoundingTotal {
		t.Fatalf("应用点没按固定先后排序：%v", got)
	}
	structures := structuresWithRounding(t, fxStructures(t), policy)
	plan := planWithBaseAndStructures(t, "10.004", structures)
	// 基础运费 10.004 与固定规则 1.005 各自逐行取整：10 + 1.01 = 11.01 USD；× 7.2345 = 79.651845，
	// 换算后取整到 0.01，再合计取整（已在 0.01 上，原样）。
	plan, err := domain.NewPricingPlanVersion(
		plan.Reference(), plan.Scope(), plan.Direction(), plan.Purpose(), plan.BaseChargeCode(), plan.EffectivePeriod(),
		plan.RateTable(), plan.WeightPolicy(), rules, structures,
	)
	if err != nil {
		t.Fatalf("plan with rules: %v", err)
	}
	evaluation := evaluateConverted(t, "eval-three-points", plan, "7.2345")
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	lines := evaluation.ChargeLines()
	if len(lines) != 2 || lines[0].Amount().Amount().String() != "10" || lines[1].Amount().Amount().String() != "1.01" {
		t.Fatalf("逐行取整后的费用行 = %#v", lines)
	}
	step, _ := evaluation.ConversionStep()
	if step.Original().Amount().String() != "11.01" {
		t.Fatalf("换算前原币金额 = %s, want 11.01（费用行之和）", step.Original().Amount().String())
	}
	points := make([]domain.AmountRoundingPoint, 0)
	for _, recorded := range evaluation.AmountRounding() {
		points = append(points, recorded.Point())
	}
	want := []domain.AmountRoundingPoint{domain.AmountRoundingPerLine, domain.AmountRoundingPerLine, domain.AmountRoundingAfterConversion, domain.AmountRoundingTotal}
	if len(points) != len(want) {
		t.Fatalf("取整留痕 = %v, want %v", points, want)
	}
	for index := range want {
		if points[index] != want[index] {
			t.Fatalf("取整留痕 = %v, want %v", points, want)
		}
	}
	total, _ := evaluation.Total()
	if total.Amount().String() != "79.65" {
		t.Fatalf("total = %s, want 79.65（11.01 × 7.2345 = 79.651845 → 79.65）", total.Amount().String())
	}
}

// Covers: ADR-0107 Decision 三——策略进内容摘要与快照：声明与不声明的两张卡摘要不同；带策略的卡与带
// 取整留痕的评价各自折装重建后原样且自校通过。
func TestAmountRoundingEntersTheDigestAndRoundTripsThroughSnapshots(t *testing.T) {
	policy := amountPolicy(t, domain.RoundingHalfUp, "0.01", domain.AmountRoundingTotal)
	declared := planWithBaseAndStructures(t, "10", structuresWithRounding(t, mustStructures(t), policy))
	undeclared := planWithBaseAndStructures(t, "10", mustStructures(t))
	if declared.ContentDigest() == undeclared.ContentDigest() {
		t.Fatal("声明了金额取整策略的卡与没声明的卡内容摘要相同——策略没进摘要")
	}

	_, rebuilt := planSnapshotRoundTrip(t, declared)
	rebuiltPolicy, present := rebuilt.Structures().AmountRounding()
	if !present || rebuiltPolicy.Mode() != domain.RoundingHalfUp || rebuiltPolicy.Increment().Amount().String() != "0.01" || len(rebuiltPolicy.Points()) != 1 {
		t.Fatalf("重建后的策略 = %#v present=%v", rebuiltPolicy, present)
	}
	if rebuilt.ContentDigest() != declared.ContentDigest() {
		t.Fatal("重建后的价卡摘要变了")
	}

	evaluation := evaluateWithSides(t, declared, "eval-rounding-snapshot", "5")
	if evaluation.Status() != domain.EvaluationCompleted || len(evaluation.AmountRounding()) != 1 {
		t.Fatalf("评价 = %s / %#v", evaluation.Status(), evaluation.AmountRounding())
	}
	rawEvaluation, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("折装评价：%v", err)
	}
	rebuiltEvaluation, err := domain.RehydrateEvaluationSnapshot(rawEvaluation)
	if err != nil {
		t.Fatalf("重建评价：%v", err)
	}
	if rebuiltEvaluation.SemanticDigest() != evaluation.SemanticDigest() || len(rebuiltEvaluation.AmountRounding()) != 1 {
		t.Fatalf("重建后的评价 = %s / %#v", rebuiltEvaluation.SemanticDigest(), rebuiltEvaluation.AmountRounding())
	}
}

// Covers: 构造门——模式不许空或 NONE、进位单位必须为正、应用点必含合计且在封闭集内；进位单位币种
// 必须是卡币种（在方案构造上判）。不填任何模式取值与进位单位默认。
func TestAmountRoundingPolicyGuards(t *testing.T) {
	usd := mustValue(t, domain.NewCurrency, "USD")
	if _, err := domain.NewAmountRoundingPolicy(domain.RoundingNone, money(t, "0.01", usd), []domain.AmountRoundingPoint{domain.AmountRoundingTotal}); !errors.Is(err, domain.ErrInvalidAmountRoundingPolicy) {
		t.Fatalf("NONE 应拒：%v", err)
	}
	if _, err := domain.NewAmountRoundingPolicy("", money(t, "0.01", usd), []domain.AmountRoundingPoint{domain.AmountRoundingTotal}); !errors.Is(err, domain.ErrInvalidAmountRoundingPolicy) {
		t.Fatalf("空模式应拒：%v", err)
	}
	if _, err := domain.NewAmountRoundingPolicy(domain.RoundingHalfUp, money(t, "0", usd), []domain.AmountRoundingPoint{domain.AmountRoundingTotal}); !errors.Is(err, domain.ErrInvalidAmountRoundingPolicy) {
		t.Fatalf("零进位单位应拒：%v", err)
	}
	if _, err := domain.NewAmountRoundingPolicy(domain.RoundingHalfUp, money(t, "0.01", usd), []domain.AmountRoundingPoint{domain.AmountRoundingPerLine}); !errors.Is(err, domain.ErrInvalidAmountRoundingPolicy) {
		t.Fatalf("缺合计点应拒：%v", err)
	}
	if _, err := domain.NewAmountRoundingPolicy(domain.RoundingHalfUp, money(t, "0.01", usd), []domain.AmountRoundingPoint{domain.AmountRoundingTotal, "SOMEWHERE"}); !errors.Is(err, domain.ErrInvalidAmountRoundingPolicy) {
		t.Fatalf("集合外的应用点应拒：%v", err)
	}

	cny := mustValue(t, domain.NewCurrency, "CNY")
	foreign, err := domain.NewAmountRoundingPolicy(domain.RoundingHalfUp, money(t, "0.01", cny), []domain.AmountRoundingPoint{domain.AmountRoundingTotal})
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	structures, err := mustStructures(t).WithAmountRounding(foreign)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	if _, err := newPlanWithStructures(t, structures); !errors.Is(err, domain.ErrCurrencyMismatch) {
		t.Fatalf("进位单位币种与卡币种不同应拒 ErrCurrencyMismatch，实得 %v", err)
	}
}

// Covers: HALF_UP 进入 RoundingMode 封闭集且两个取整内核都认它：半数远离零，不足半数舍。
func TestHalfUpRoundsHalvesAwayFromZero(t *testing.T) {
	increment := decimal(t, "0.01")
	// 结果是 Decimal 规范形（去尾零）：1.004 落到 1、2 仍是 2。
	for input, want := range map[string]string{"1.005": "1.01", "1.004": "1", "1.015": "1.02", "2": "2", "0.125": "0.13"} {
		got, err := decimal(t, input).RoundToIncrement(increment, domain.RoundingHalfUp)
		if err != nil || got.String() != want {
			t.Fatalf("%s HALF_UP 0.01 = %s / %v, want %s", input, got.String(), err, want)
		}
	}
	quotient, err := decimal(t, "10").DivRoundToIncrement(decimal(t, "3"), decimal(t, "0.01"), domain.RoundingHalfUp)
	if err != nil || quotient.String() != "3.33" {
		t.Fatalf("10 / 3 HALF_UP 0.01 = %s / %v, want 3.33", quotient.String(), err)
	}
}

func mustStructures(t testing.TB) domain.PricingPlanStructures {
	t.Helper()
	structures, err := domain.NewPricingPlanStructures(nil, nil, nil)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}
