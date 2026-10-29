package domain_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件钉 ADR-0348 决定一、二：价卡对单票合计声明下限与上限，评价在各行收齐之后、换算与合计取整之前施加；
// 低于下限补一条 MINIMUM_CHARGE 补差行，高于上限补一条 MAXIMUM_CHARGE 冲减行（DEDUCT，金额仍为正），
// 各行相加仍等于合计。取值全是 SYN（只记 S）；基础运费一律 USD 10。

func boundsOf(t testing.TB, minimum, maximum string) domain.TotalChargeBounds {
	t.Helper()
	usd := mustValue(t, domain.NewCurrency, "USD")
	var low, high *domain.Money
	if minimum != "" {
		value := money(t, minimum, usd)
		low = &value
	}
	if maximum != "" {
		value := money(t, maximum, usd)
		high = &value
	}
	bounds, err := domain.NewTotalChargeBounds(low, high)
	if err != nil {
		t.Fatalf("total charge bounds [%s,%s]: %v", minimum, maximum, err)
	}
	return bounds
}

func structuresWithBounds(t testing.TB, base domain.PricingPlanStructures, bounds domain.TotalChargeBounds) domain.PricingPlanStructures {
	t.Helper()
	declared, err := base.WithTotalChargeBounds(bounds)
	if err != nil {
		t.Fatalf("with total charge bounds: %v", err)
	}
	return declared
}

func boundedPlan(t *testing.T, minimum, maximum string, rules ...domain.FixedChargeRule) domain.PricingPlanVersion {
	t.Helper()
	return planWithStructuresAndFixedRules(t, structuresWithBounds(t, domain.PricingPlanStructures{}, boundsOf(t, minimum, maximum)), rules...)
}

func lineAmounts(evaluation domain.PricingEvaluation) map[string]string {
	amounts := map[string]string{}
	for _, line := range evaluation.ChargeLines() {
		amounts[line.Code().String()] = line.Amount().Amount().String()
	}
	return amounts
}

// 把各行按效果折成合计：补差行与冲减行也是行，行之和必须等于合计（取整之前），这是「补差行」口径的理由。
func signedLineSum(t testing.TB, evaluation domain.PricingEvaluation) domain.Decimal {
	t.Helper()
	sum := decimal(t, "0")
	for _, line := range evaluation.ChargeLines() {
		var err error
		if line.Effect() == domain.ChargeEffectDeduct {
			sum, err = sum.Sub(line.Amount().Amount())
		} else {
			sum, err = sum.Add(line.Amount().Amount())
		}
		if err != nil {
			t.Fatalf("行求和：%v", err)
		}
	}
	return sum
}

// Covers: AT-PP-030
func TestATPP030ATotalBelowTheDeclaredMinimumIsLiftedByASeparateSupplementLine(t *testing.T) {
	plan := boundedPlan(t, "25", "")
	evaluation := evaluate(t, "eval-min-lifted", plan, syntheticInput(t, "5", "Z1"))

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("状态 %s，问题项 %v", evaluation.Status(), evaluation.Issues())
	}
	if got := totalOf(t, evaluation); got != "25" {
		t.Fatalf("合计 %s，手算 25（基础 10 低于下限 25）", got)
	}
	lines := evaluation.ChargeLines()
	if len(lines) != 2 {
		t.Fatalf("费用行 %d 条，手算 2（基础运费 + 补差行）：%v", len(lines), lineAmounts(evaluation))
	}
	supplement := lines[1]
	if supplement.Code() != domain.MinimumChargeCode() || supplement.Effect() != domain.ChargeEffectAdd ||
		supplement.Amount().Amount().String() != "15" || supplement.Kind() != domain.ChargeLineTotalBound {
		t.Fatalf("补差行 = %s %s %s %s，手算 MINIMUM_CHARGE ADD 15 TOTAL_BOUND",
			supplement.Code(), supplement.Effect(), supplement.Amount().Amount(), supplement.Kind())
	}
	// 补的是差额而不是整笔：各行相加仍等于合计，基础运费那一行没被改写。
	if lineAmounts(evaluation)["BASE_FREIGHT"] != "10" {
		t.Fatalf("基础运费行被改写了：%v", lineAmounts(evaluation))
	}
	if sum := signedLineSum(t, evaluation); sum.String() != "25" {
		t.Fatalf("行之和 %s，应等于合计 25", sum)
	}
	if !explanationMentions(evaluation, "below the declared minimum 25 USD") || !explanationMentions(evaluation, "MINIMUM_CHARGE supplement of 15 USD") {
		t.Fatalf("解释没有如实记下最低收费补足：%v", evaluation.Explanation())
	}
}

// Covers: AT-PP-031
func TestATPP031ATotalAtOrAboveTheMinimumIsNotAdjustedAndTheJudgementIsStillExplained(t *testing.T) {
	cases := []struct {
		name    string
		rules   []domain.FixedChargeRule
		total   string
		mention string
	}{
		{"恰好等于下限", []domain.FixedChargeRule{fixedRule(t, "handling", domain.ChargeEffectAdd, "5", 1)}, "15", "equal to the declared minimum 15 USD"},
		{"高于下限", []domain.FixedChargeRule{fixedRule(t, "handling", domain.ChargeEffectAdd, "9", 1)}, "19", "within the declared bounds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evaluation := evaluate(t, "eval-min-"+tc.name, boundedPlan(t, "15", "", tc.rules...), syntheticInput(t, "5", "Z1"))
			if got := totalOf(t, evaluation); got != tc.total {
				t.Fatalf("合计 %s，手算 %s", got, tc.total)
			}
			if hasChargeCode(evaluation, "MINIMUM_CHARGE") {
				t.Fatalf("不该补足却有补差行：%v", lineAmounts(evaluation))
			}
			if !explanationMentions(evaluation, tc.mention) {
				t.Fatalf("不调整也要留痕（%q）：%v", tc.mention, evaluation.Explanation())
			}
		})
	}
}

// Covers: AT-PP-032
func TestATPP032ATotalAboveTheDeclaredMaximumIsCappedByADeductLineWithAPositiveAmount(t *testing.T) {
	plan := boundedPlan(t, "", "30", fixedRule(t, "handling", domain.ChargeEffectAdd, "35", 1))
	evaluation := evaluate(t, "eval-max-capped", plan, syntheticInput(t, "5", "Z1"))

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("状态 %s，问题项 %v", evaluation.Status(), evaluation.Issues())
	}
	if got := totalOf(t, evaluation); got != "30" {
		t.Fatalf("合计 %s，手算 30（10 + 35 = 45 封顶到 30）", got)
	}
	lines := evaluation.ChargeLines()
	reduction := lines[len(lines)-1]
	if reduction.Code() != domain.MaximumChargeCode() || reduction.Effect() != domain.ChargeEffectDeduct ||
		reduction.Amount().Amount().String() != "15" {
		t.Fatalf("冲减行 = %s %s %s，手算 MAXIMUM_CHARGE DEDUCT 15（金额是正数，方向由 DEDUCT 表达）",
			reduction.Code(), reduction.Effect(), reduction.Amount().Amount())
	}
	if sum := signedLineSum(t, evaluation); sum.String() != "30" {
		t.Fatalf("行之和 %s，应等于合计 30", sum)
	}
	if !explanationMentions(evaluation, "above the declared maximum 30 USD") {
		t.Fatalf("解释没有如实记下最高收费冲减：%v", evaluation.Explanation())
	}
}

// 上下限同时声明时至多触发一端，且上限不会把已被下限抬起的合计再压回去（构造期已保证下限不高于上限）。
func TestBothBoundsDeclaredOnlyOneSideCanFire(t *testing.T) {
	plan := boundedPlan(t, "20", "40", fixedRule(t, "handling", domain.ChargeEffectAdd, "50", 1))
	evaluation := evaluate(t, "eval-both-high", plan, syntheticInput(t, "5", "Z1"))
	if got := totalOf(t, evaluation); got != "40" || hasChargeCode(evaluation, "MINIMUM_CHARGE") || !hasChargeCode(evaluation, "MAXIMUM_CHARGE") {
		t.Fatalf("合计 %s，行 %v，手算只触发上限、合计 40", got, lineAmounts(evaluation))
	}
	low := evaluate(t, "eval-both-low", boundedPlan(t, "20", "40"), syntheticInput(t, "5", "Z1"))
	if got := totalOf(t, low); got != "20" || !hasChargeCode(low, "MINIMUM_CHARGE") || hasChargeCode(low, "MAXIMUM_CHARGE") {
		t.Fatalf("合计 %s，行 %v，手算只触发下限、合计 20", got, lineAmounts(low))
	}
}

// Covers: AT-PP-033
func TestATPP033BoundsApplyAfterDeductLinesAndAfterEveryOtherChargeWasSettled(t *testing.T) {
	// 折扣类的 DEDUCT 行先落，下限对的是扣完之后的合计：10 + 5 − 8 = 7，低于下限 12，补差 5。
	plan := boundedPlan(t, "12", "",
		fixedRule(t, "handling", domain.ChargeEffectAdd, "5", 1),
		fixedRule(t, "discount", domain.ChargeEffectDeduct, "8", 2),
	)
	evaluation := evaluate(t, "eval-min-after-discount", plan, syntheticInput(t, "5", "Z1"))
	if got := totalOf(t, evaluation); got != "12" {
		t.Fatalf("合计 %s，手算 12", got)
	}
	if lineAmounts(evaluation)["MINIMUM_CHARGE"] != "5" {
		t.Fatalf("补差行 %v，手算 5 = 12 − 7（对折扣之后的合计补）", lineAmounts(evaluation))
	}
	lines := evaluation.ChargeLines()
	if lines[len(lines)-1].Code() != domain.MinimumChargeCode() {
		t.Fatalf("补差行必须是最后一行：%v", lineAmounts(evaluation))
	}
	for index := 1; index < len(lines); index++ {
		if lines[index].Order() <= lines[index-1].Order() {
			t.Fatalf("费用行序号不严格递增：%d 在 %d 之后", lines[index].Order(), lines[index-1].Order())
		}
	}
}

func TestBoundsApplyToTheOutcomeOfTheExclusivityGroupNotToTheSuppressedRule(t *testing.T) {
	// 同组两条同级规则只收金额高者：30 胜 20，合计 10 + 30 = 40；上限 35 冲减 5。被压下的那条不进合计，也不进上限的判断。
	lower := groupedSurcharge(t, surchargeRuleFor(t, "ahs-a", "AHS_A", "48", "20"), "AHS", 1)
	higher := groupedSurcharge(t, surchargeRuleFor(t, "ahs-b", "AHS_B", "48", "30"), "AHS", 1)
	structures := structuresWithBounds(t, declaredSurcharges(t, lower, higher), boundsOf(t, "", "35"))
	evaluation := evaluateWithSides(t, planWithStructures(t, structures), "eval-bound-after-group", "50")

	if got := totalOf(t, evaluation); got != "35" {
		t.Fatalf("合计 %s，手算 35（10 + 30 封顶到 35）；行 %v", got, lineAmounts(evaluation))
	}
	if hasChargeCode(evaluation, "AHS_A") || lineAmounts(evaluation)["MAXIMUM_CHARGE"] != "5" {
		t.Fatalf("行 %v，手算只留 AHS_B 与冲减 5", lineAmounts(evaluation))
	}
}

func TestTheSupplementIsNotPartOfAnyPercentBasisSoFuelIsComputedWithoutIt(t *testing.T) {
	// 燃油 = 基础运费 10 的 10% = 1，合计 11 低于下限 20，补差 9。补差行在所有费用行之后产生，不进任何费用依赖的基数：
	// 燃油不会因为补差而变成 20 的 10%。
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{percentRule(t, "fuel", "FUEL", "48", "10", "fuel")},
		[]domain.ChargeDependency{listedChargesBasis(t, "fuel", "FUEL", "BASE_FREIGHT")},
		nil,
	)
	if err != nil {
		t.Fatalf("结构：%v", err)
	}
	structures = structuresWithBounds(t, structures, boundsOf(t, "20", ""))
	evaluation := evaluateWithSides(t, planWithStructures(t, structures), "eval-fuel-not-on-supplement", "50")

	if got := totalOf(t, evaluation); got != "20" {
		t.Fatalf("合计 %s，手算 20", got)
	}
	amounts := lineAmounts(evaluation)
	if amounts["FUEL"] != "1" || amounts["MINIMUM_CHARGE"] != "9" {
		t.Fatalf("行 %v，手算燃油 1（基数只有基础运费）与补差 9", amounts)
	}
}

func TestBoundsComposeWithAmountRoundingPerLineAndTotal(t *testing.T) {
	// 逐行 + 合计取整到 0.01：基础 10、固定 5.555 取整成 5.56，行之和 15.56；下限 20 对的是取整后的行之和，
	// 补差 4.44 恰在进位单位上，合计 20。补差行也是一条逐行取整的行，留下自己的逐行留痕。
	rounding := amountPolicy(t, domain.RoundingHalfUp, "0.01", domain.AmountRoundingPerLine, domain.AmountRoundingTotal)
	structures := structuresWithBounds(t, structuresWithRounding(t, domain.PricingPlanStructures{}, rounding), boundsOf(t, "20", ""))
	plan := planWithStructuresAndFixedRules(t, structures, fixedRule(t, "handling", domain.ChargeEffectAdd, "5.555", 1))
	evaluation := evaluate(t, "eval-bounds-rounding", plan, syntheticInput(t, "5", "Z1"))

	if got := totalOf(t, evaluation); got != "20" {
		t.Fatalf("合计 %s，手算 20", got)
	}
	if lineAmounts(evaluation)["MINIMUM_CHARGE"] != "4.44" || lineAmounts(evaluation)["RULE_HANDLING"] != "5.56" {
		t.Fatalf("行 %v，手算固定 5.56、补差 4.44", lineAmounts(evaluation))
	}
	var subjects []string
	for _, step := range evaluation.AmountRounding() {
		subjects = append(subjects, string(step.Point())+":"+step.Subject())
	}
	want := []string{"PER_LINE:base:entry-mixed", "PER_LINE:fixed:handling", "PER_LINE:bound:minimum", "TOTAL:"}
	if strings.Join(subjects, "|") != strings.Join(want, "|") {
		t.Fatalf("取整留痕 %v，手算 %v", subjects, want)
	}
}

func TestBoundsOnlyTotalRoundingLeavesTheSupplementExactAndTheTotalOnTheMinimum(t *testing.T) {
	// 只声明合计取整：各行不取整，行之和 15.555，补差 4.445（精确差额），合计 20 取整后仍是 20。
	rounding := amountPolicy(t, domain.RoundingHalfUp, "0.01", domain.AmountRoundingTotal)
	structures := structuresWithBounds(t, structuresWithRounding(t, domain.PricingPlanStructures{}, rounding), boundsOf(t, "20", ""))
	plan := planWithStructuresAndFixedRules(t, structures, fixedRule(t, "handling", domain.ChargeEffectAdd, "5.555", 1))
	evaluation := evaluate(t, "eval-bounds-total-only", plan, syntheticInput(t, "5", "Z1"))

	if got := totalOf(t, evaluation); got != "20" || lineAmounts(evaluation)["MINIMUM_CHARGE"] != "4.445" {
		t.Fatalf("合计 %s，行 %v，手算合计 20、补差 4.445", got, lineAmounts(evaluation))
	}
}

func TestBoundsAreInTheCardCurrencyAndComeBeforeTheSettlementConversion(t *testing.T) {
	// 卡 USD、结算 CNY：下限 25 USD 先补足，再按汇率换算整个合计。补差行仍是 USD 的行，行之和（USD）等于换算前的合计。
	plan := planWithStructuresAndFixedRules(t, structuresWithBounds(t, fxStructures(t), boundsOf(t, "25", "")))
	evaluation := evaluateConverted(t, "eval-bounds-conversion", plan, "7")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("状态 %s，问题项 %v", evaluation.Status(), evaluation.Issues())
	}
	step, converted := evaluation.ConversionStep()
	if !converted || step.Original().Amount().String() != "25" || step.Original().Currency().String() != "USD" {
		t.Fatalf("换算前的原币合计应已补足到 25 USD：%+v", step)
	}
	if got := totalOf(t, evaluation); got != "175" {
		t.Fatalf("合计 %s，手算 175 CNY = 25 × 7", got)
	}
	if lineAmounts(evaluation)["MINIMUM_CHARGE"] != "15" {
		t.Fatalf("补差行 %v，手算 15 USD", lineAmounts(evaluation))
	}
}

// Covers: AT-PP-034
func TestATPP034BoundsThatDoNotHoldAreRefused(t *testing.T) {
	usd := mustValue(t, domain.NewCurrency, "USD")
	cny := mustValue(t, domain.NewCurrency, "CNY")
	ten, twenty, zero := money(t, "10", usd), money(t, "20", usd), money(t, "0", usd)
	cnyTen := money(t, "10", cny)

	for name, build := range map[string]func() error{
		"一端也没声明":  func() error { _, err := domain.NewTotalChargeBounds(nil, nil); return err },
		"下限高于上限":  func() error { _, err := domain.NewTotalChargeBounds(&twenty, &ten); return err },
		"零下限":     func() error { _, err := domain.NewTotalChargeBounds(&zero, nil); return err },
		"零上限":     func() error { _, err := domain.NewTotalChargeBounds(nil, &zero); return err },
		"上下限币种不同": func() error { _, err := domain.NewTotalChargeBounds(&ten, &cnyTen); return err },
	} {
		if err := build(); !errors.Is(err, domain.ErrInvalidTotalChargeBounds) {
			t.Fatalf("%s：%v，应拒为 ErrInvalidTotalChargeBounds", name, err)
		}
	}
	if equalBounds, err := domain.NewTotalChargeBounds(&ten, &ten); err != nil || equalBounds == (domain.TotalChargeBounds{}) {
		t.Fatalf("下限等于上限是定价一口价，应成立：%v", err)
	}

	// 币种与方案计价币种不一致：方案层拒。
	foreign, err := domain.NewTotalChargeBounds(&cnyTen, nil)
	if err != nil {
		t.Fatalf("外币下限本身成立：%v", err)
	}
	structures := structuresWithBounds(t, domain.PricingPlanStructures{}, foreign)
	if _, err := newPlanWithStructures(t, structures); !errors.Is(err, domain.ErrCurrencyMismatch) {
		t.Fatalf("外币下限的方案：%v，应拒为 ErrCurrencyMismatch", err)
	}

	// 卡声明了金额取整，上下限须落在进位单位的整数倍上，否则合计取整会把补足后的合计改回下限之外。
	offGrid := money(t, "10.005", usd)
	offGridBounds, err := domain.NewTotalChargeBounds(&offGrid, nil)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	rounded := structuresWithRounding(t, domain.PricingPlanStructures{}, amountPolicy(t, domain.RoundingHalfUp, "0.01", domain.AmountRoundingTotal))
	if _, err := newPlanWithStructures(t, structuresWithBounds(t, rounded, offGridBounds)); !errors.Is(err, domain.ErrInvalidTotalChargeBounds) {
		t.Fatalf("不在进位单位整数倍上的下限：%v，应拒为 ErrInvalidTotalChargeBounds", err)
	}
	// 同一个下限在没有声明金额取整的卡上成立：没有取整就没有被改回去的风险。
	if _, err := newPlanWithStructures(t, structuresWithBounds(t, domain.PricingPlanStructures{}, offGridBounds)); err != nil {
		t.Fatalf("没有金额取整的卡上，任意精度的下限都应成立：%v", err)
	}

	// 费用代码被占用：声明了下限的方案里，别的规则不能再用 MINIMUM_CHARGE。
	clash, err := domain.NewFixedChargeRule("clash", domain.MinimumChargeCode(), "clash", domain.ChargeEffectAdd, ten, 1)
	if err != nil {
		t.Fatalf("固定规则：%v", err)
	}
	lowOnly := structuresWithBounds(t, domain.PricingPlanStructures{}, boundsOf(t, "15", ""))
	if _, err := newPlanWithStructuresAndRules(t, lowOnly, clash); !errors.Is(err, domain.ErrDuplicateChargeCode) {
		t.Fatalf("占用 MINIMUM_CHARGE 的固定规则：%v，应拒为 ErrDuplicateChargeCode", err)
	}
	// 没声明下限的方案里同名的码照旧可用（既有价卡不因这一版而失效）。
	if _, err := newPlanWithStructuresAndRules(t, domain.PricingPlanStructures{}, clash); err != nil {
		t.Fatalf("没有声明下限的方案里，MINIMUM_CHARGE 作为普通费用代码应照旧成立：%v", err)
	}
}

// 补足行 / 冲减行读回时的自洽门：码与方向配对、只能是最后一行。
func TestATPP034TheBoundLineContractSurvivesTheEvaluationSnapshot(t *testing.T) {
	evaluation := evaluate(t, "eval-bound-snapshot", boundedPlan(t, "25", ""), syntheticInput(t, "5", "Z1"))
	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("折装评价快照：%v", err)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("重建评价：%v", err)
	}
	if restored.SemanticDigest() != evaluation.SemanticDigest() || restored.PlanCanonicalizationVersion() != "PPC-6" {
		t.Fatalf("读回的评价摘要或规范化版本变了：%s / %s", restored.SemanticDigest(), restored.PlanCanonicalizationVersion())
	}

	// 码与方向不配对（MINIMUM_CHARGE 却是 DEDUCT）：读回门拒绝。
	if _, err := domain.RehydrateEvaluationSnapshot(tamperedDocument(t, raw, func(document map[string]any) {
		lines := document["chargeLines"].([]any)
		lines[len(lines)-1].(map[string]any)["effect"] = "DEDUCT"
	})); err == nil {
		t.Fatal("码与方向不配对的补差行应被读回门拒绝")
	}
	// 总额上下限行不在最后：把它挪到基础运费前面，序号单调与首行必为基础运费两道都会拒。
	if _, err := domain.RehydrateEvaluationSnapshot(tamperedDocument(t, raw, func(document map[string]any) {
		lines := document["chargeLines"].([]any)
		lines[0], lines[1] = lines[1], lines[0]
	})); err == nil {
		t.Fatal("补差行挪到基础运费前应被读回门拒绝")
	}
}

// Covers: AT-PP-035
func TestATPP035APlanWithoutBoundsEvaluatesAsItAlwaysDidAndStaysOnTheBaseCanonicalization(t *testing.T) {
	plan := planWithStructuresAndFixedRules(t, domain.PricingPlanStructures{}, fixedRule(t, "handling", domain.ChargeEffectAdd, "5", 1))
	if plan.CanonicalizationVersion() != "PPC-5" {
		t.Fatalf("不带总额上下限的方案写 %s，应仍是 PPC-5", plan.CanonicalizationVersion())
	}
	evaluation := evaluate(t, "eval-no-bounds", plan, syntheticInput(t, "5", "Z1"))
	if got := totalOf(t, evaluation); got != "15" || explanationMentions(evaluation, "total bounds") {
		t.Fatalf("合计 %s，解释 %v：未声明时不多一行解释、金额如旧", got, evaluation.Explanation())
	}
	if evaluation.PlanCanonicalizationVersion() != "PPC-5" {
		t.Fatalf("评价记的规范化版本 %s，应仍是 PPC-5", evaluation.PlanCanonicalizationVersion())
	}
	// 声明了上下限但没触发的方案也是新结构，写 PPC-6——版本由方案声明了什么定，不由这一票触没触发定。
	bounded := boundedPlan(t, "5", "", fixedRule(t, "handling", domain.ChargeEffectAdd, "5", 1))
	if bounded.CanonicalizationVersion() != "PPC-6" {
		t.Fatalf("声明了下限的方案写 %s，应是 PPC-6", bounded.CanonicalizationVersion())
	}
	if bounded.ContentDigest() == plan.ContentDigest() {
		t.Fatal("声明下限与否的两张卡摘要必须不同")
	}
}

func newPlanWithStructuresAndRules(t *testing.T, structures domain.PricingPlanStructures, rules ...domain.FixedChargeRule) (domain.PricingPlanVersion, error) {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-bounds-rules"), "Z1",
		weight(t, "0", domain.WeightUnitKilogram), weight(t, "10", domain.WeightUnitKilogram), money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-bounds-rules", "v1"),
		domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram, effectivePeriod(t), []domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingNone, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("weight rounding: %v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-bounds-rules", "v1"), domain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	return domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-bounds-rules", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t), table, weightPolicy, rules, structures,
	)
}

// Covers: AT-PP-035
// 下面两个摘要是在引入总额上下限与偏远档位查档价表之前的 origin/main 上算出来记下的：同一份夹具在本构建上必须
// 得到同一串。它们钉的是「不改变任何今天已算对的评价」——方案内容摘要、评价语义摘要、规范化版本三样都不动
// （ADR-0348 决定五；跨树的整份对拍见票面完成记录）。
func TestATPP035DigestsOfOldShapeFixturesAreTheOnesRecordedBeforeBoundsExisted(t *testing.T) {
	structures := structuresWithRounding(t, structuresWithCatalogues(t, catalogueLink(t, domain.CatalogueKindZone, "SYN-CAT-ZONE")),
		amountPolicy(t, domain.RoundingHalfUp, "0.01", domain.AmountRoundingPerLine, domain.AmountRoundingTotal))
	plan := planWithStructuresAndFixedRules(t, structures, fixedRule(t, "handling", domain.ChargeEffectAdd, "5.555", 1))
	evaluation := evaluate(t, "eval-golden", plan, postalInput(t, "098123", catalogueReading(t, domain.CatalogueKindZone, "SYN-CAT-ZONE", "v1", "Z1")))
	if plan.ContentDigest() != "72f90962068dcc928942dd4c710be0dff02ed15a63485ae5370918b060d1ea0e" ||
		evaluation.SemanticDigest() != "04cee7c7d60708756a117796b93a2fd4097e3c013a70fb8c601da3b0d60791c6" ||
		plan.CanonicalizationVersion() != "PPC-5" || evaluation.PlanCanonicalizationVersion() != "PPC-5" {
		t.Fatalf("旧形夹具的摘要变了：方案 %s、评价 %s、版本 %s/%s", plan.ContentDigest(), evaluation.SemanticDigest(),
			plan.CanonicalizationVersion(), evaluation.PlanCanonicalizationVersion())
	}

	full := fullyDeclaredSyntheticPlan(t)
	sides := dimensions(t, "50", "10", "10", domain.LengthUnitInch)
	fullEvaluation := evaluate(t, "eval-golden-full", full, syntheticInputWithDimensions(t, "5", "Z1", sides))
	if full.ContentDigest() != "bb0139455cadba47f6d34b143be9a99b954eb421f9da4c121330db2f0c8339d1" ||
		fullEvaluation.SemanticDigest() != "c596d142e2badfae51fd40368d53bbdc12e376be60e11b14bdaa1687250e78ce" {
		t.Fatalf("声明了全部旧结构的夹具摘要变了：方案 %s、评价 %s", full.ContentDigest(), fullEvaluation.SemanticDigest())
	}
}
