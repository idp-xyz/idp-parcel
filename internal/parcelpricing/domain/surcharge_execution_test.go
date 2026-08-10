package domain_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 一条已声明却从未被执行的附加费，比它根本不存在还糟：方案看起来算过价了，卡上那笔钱却
// 没收。本切片是第一次真的把它收上来。
func TestSurchargeThatHitsIsCollectedOnTopOfTheBaseFreight(t *testing.T) {
	plan := planWithStructures(t, standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "25")))
	evaluation := evaluateWithSides(t, plan, "eval-surcharge-hit", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	total, ok := evaluation.Total()
	if !ok || total.Amount().String() != "35" {
		t.Fatalf("total = %v (present=%v), want 35 = 10 base + 25 surcharge", total.Amount().String(), ok)
	}
	if !hasChargeCode(evaluation, "AHS_DIMENSION") {
		t.Fatalf("charge lines = %#v, want one coded AHS_DIMENSION", evaluation.ChargeLines())
	}
}

// Covers: CONTEXT「未命中的规则也要在解释中留痕，只输出命中结果视为解释不完整」— 只列
// 命中项的解释没法拿去对卡，因为读的人分不出「判过但没命中」和「压根没判过」。
func TestSurchargeThatMissesIsExplainedRatherThanSilentlyDropped(t *testing.T) {
	plan := planWithStructures(t, standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "96", "25")))
	evaluation := evaluateWithSides(t, plan, "eval-surcharge-miss", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "10" {
		t.Fatalf("total = %s, want 10 with the surcharge missing", total.Amount().String())
	}
	if hasChargeCode(evaluation, "AHS_DIMENSION") {
		t.Fatal("a missed surcharge produced a charge line")
	}
	if !explanationMentions(evaluation, "ahs-dimension") {
		t.Fatalf("explanation = %#v, want the missed rule recorded", evaluation.Explanation())
	}
}

// Covers: CONTEXT「互斥组内至多一条规则命中；多条同时满足时先按声明优先级取最高级」（推导
// 见计价规则模型最终设计的形态决定五）— UPS 用大件费压住额外处理费正是这个形状。
func TestExclusivityGroupCollectsOnlyTheHighestPriorityRule(t *testing.T) {
	oversize := groupedSurcharge(t, surchargeRuleFor(t, "oversize", "OVERSIZE", "48", "30"), "AHS", 1)
	handling := groupedSurcharge(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "80"), "AHS", 2)
	plan := planWithStructures(t, declaredSurcharges(t, oversize, handling))
	evaluation := evaluateWithSides(t, plan, "eval-exclusive-priority", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	// 优先级 1 压过优先级 2，哪怕它金额更低，所以选取不能是简单的「取最大值」。
	if !hasChargeCode(evaluation, "OVERSIZE") || hasChargeCode(evaluation, "AHS_DIMENSION") {
		t.Fatalf("charge lines = %#v, want only OVERSIZE", evaluation.ChargeLines())
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "40" {
		t.Fatalf("total = %s, want 40 = 10 base + 30 oversize", total.Amount().String())
	}
}

// 同级则落到取金额最高者，卡上三个额外处理费变体之间就是这么分出胜负的。
func TestExclusivityGroupFallsBackToTheHighestAmountAtEqualPriority(t *testing.T) {
	lower := groupedSurcharge(t, surchargeRuleFor(t, "ahs-a", "AHS_A", "48", "20"), "AHS", 1)
	higher := groupedSurcharge(t, surchargeRuleFor(t, "ahs-b", "AHS_B", "48", "45"), "AHS", 1)
	plan := planWithStructures(t, declaredSurcharges(t, lower, higher))
	evaluation := evaluateWithSides(t, plan, "eval-exclusive-amount", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	if !hasChargeCode(evaluation, "AHS_B") || hasChargeCode(evaluation, "AHS_A") {
		t.Fatalf("charge lines = %#v, want only AHS_B", evaluation.ChargeLines())
	}
}

// Covers: CONTEXT「依据不足形成待判断，明确排除形成不可计价，互斥候选形成冲突，请求不合法
// 或计算失败形成未形成；四者不得互相替代」— 判定条件要读包裹尺寸，没有尺寸这条规则既确认
// 不了也排除不掉，那是依据不足而不是请求不合法，所以评价等着，不失败，也不少收。
func TestSurchargeWithoutDimensionsStaysPending(t *testing.T) {
	plan := planWithStructures(t, standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "25")))
	id := mustValue(t, domain.NewEvaluationID, "eval-surcharge-no-sides")
	request, err := domain.NewEvaluationRequest(id, plan, syntheticInput(t, "5", "Z1"), domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	evaluation := domain.EvaluatePricing(request)
	if evaluation.Status() != domain.EvaluationPending {
		t.Fatalf("status = %s, issues = %#v, want PENDING", evaluation.Status(), evaluation.Issues())
	}
}

// 如今每种已声明结构都有执行器，「不可执行」闸门再也不会触发。接替它做保证的是这一条：
// 声明了结构的方案就要带着这些结构计价，绝不只按基础价表算。原先那道闸门是声明与静默少收
// 之间唯一的阻拦；闸门空转之后，少收必须被直接排除掉。
func TestPlanDeclaringStructuresIsNeverPricedOnTheBaseTableAlone(t *testing.T) {
	bare := planWithStructures(t, declaredSurcharges(t))
	declared := planWithStructures(t, standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "25")))

	bareEvaluation := evaluateWithSides(t, bare, "eval-baseline-bare", "50")
	declaredEvaluation := evaluateWithSides(t, declared, "eval-baseline-declared", "50")
	if bareEvaluation.Status() != domain.EvaluationCompleted || declaredEvaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("statuses = %s / %s", bareEvaluation.Status(), declaredEvaluation.Status())
	}
	bareTotal, _ := bareEvaluation.Total()
	declaredTotal, _ := declaredEvaluation.Total()
	if bareTotal.Amount().String() == declaredTotal.Amount().String() {
		t.Fatalf("a declared surcharge did not change the total: both %s", bareTotal.Amount().String())
	}
}

// 卡上按分区给超尺寸费分档（Q32–Q35），所以附加费金额可以来自一张表，而不是一个定额。
// 分档用的是基础运费同一个计价重量，两边因而读到的是同一个一致的重量。
func TestTableLookupSurchargeReadsItsBandWithThePricingWeight(t *testing.T) {
	rule := standaloneRule(t, surchargeRuleWithCalculation(t, "oversize", "OVERSIZE", "48", tableSurcharge(t, "Z1", "18")))
	plan := planWithStructures(t, declaredSurcharges(t, rule))
	evaluation := evaluateWithSides(t, plan, "eval-table-surcharge", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "28" {
		t.Fatalf("total = %s, want 28 = 10 base + 18 banded surcharge", total.Amount().String())
	}
	if !hasChargeCode(evaluation, "OVERSIZE") {
		t.Fatalf("charge lines = %#v, want one coded OVERSIZE", evaluation.ChargeLines())
	}
}

// Covers: CONTEXT「取不到价卡、区间空档、事实缺失属待判断，不属不可计价」— 附加费表在这个
// 分区没有档位，那是卡上的空档，不是规则没命中：条件确实触发了。所以评价等着，而不是一分不收。
func TestTableLookupSurchargeWithNoBandForTheZoneStaysPending(t *testing.T) {
	rule := standaloneRule(t, surchargeRuleWithCalculation(t, "oversize", "OVERSIZE", "48", tableSurcharge(t, "Z9", "18")))
	plan := planWithStructures(t, declaredSurcharges(t, rule))
	evaluation := evaluateWithSides(t, plan, "eval-table-surcharge-gap", "50")

	if evaluation.Status() != domain.EvaluationPending {
		t.Fatalf("status = %s, issues = %#v, want PENDING", evaluation.Status(), evaluation.Issues())
	}
}

func tableSurcharge(t testing.TB, zone, amount string) domain.SurchargeCalculation {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	band, err := domain.NewOpenEndedRateEntry(
		mustValue(t, domain.NewRateEntryID, "surcharge-band-"+zone),
		zone,
		weight(t, "0", domain.WeightUnitKilogram),
		money(t, amount, currency),
	)
	if err != nil {
		t.Fatalf("surcharge band: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-surcharge-"+zone, "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{band},
	)
	if err != nil {
		t.Fatalf("surcharge table: %v", err)
	}
	calculation, err := domain.NewTableLookupSurcharge(table)
	if err != nil {
		t.Fatalf("table calculation: %v", err)
	}
	return calculation
}

// 一次收了附加费却重放不了的评价只算做了一半：争议复核是靠重放来回答的。费用行契约是在
// 进入重放时检查的，不是在评价出口检查，所以一条被契约拒绝的附加费行能顺利完成，只会到
// 后面才失败。
func TestSurchargedEvaluationCanBeReplayed(t *testing.T) {
	plan := planWithStructures(t, standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "25")))
	original := evaluateWithSides(t, plan, "eval-surcharge-replay-source", "50")
	if original.Status() != domain.EvaluationCompleted {
		t.Fatalf("fixture status = %s, issues = %#v", original.Status(), original.Issues())
	}

	replayed, err := domain.ReplayPricingEvaluation(
		mustValue(t, domain.NewEvaluationID, "eval-surcharge-replay"),
		original,
		plan,
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.Status() != domain.EvaluationCompleted {
		t.Fatalf("replay status = %s, issues = %#v", replayed.Status(), replayed.Issues())
	}
	if replayed.SemanticDigest() != original.SemanticDigest() {
		t.Fatal("replaying a surcharged evaluation produced a different semantic digest")
	}
}

// 固定规则各自声明序号且不必连续，所以附加费行必须从已用的最大序号往后接，而不是从已收集
// 的行数往后接。否则一个规则排在序号 5 的方案，会产出一条排序落在它前面的附加费。
func TestSurchargeOrderContinuesPastNonContiguousFixedRules(t *testing.T) {
	structures := standaloneSurcharges(t, surchargeRuleFor(t, "ahs-dimension", "AHS_DIMENSION", "48", "25"))
	plan := planWithStructuresAndFixedRules(t, structures, fixedRule(t, "late", domain.ChargeEffectAdd, "3", 5))
	evaluation := evaluateWithSides(t, plan, "eval-surcharge-order", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	lines := evaluation.ChargeLines()
	for index := 1; index < len(lines); index++ {
		if lines[index].Order() <= lines[index-1].Order() {
			t.Fatalf("charge line orders are not strictly increasing: %d then %d", lines[index-1].Order(), lines[index].Order())
		}
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "38" {
		t.Fatalf("total = %s, want 38 = 10 base + 3 fixed + 25 surcharge", total.Amount().String())
	}
}

func planWithStructuresAndFixedRules(t *testing.T, structures domain.PricingPlanStructures, rules ...domain.FixedChargeRule) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-mixed"),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, "10", currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-mixed", "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingNone, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("rounding policy: %v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-mixed", "v1"),
		domain.PricingWeightActualOnly,
		rounding,
		nil,
	)
	if err != nil {
		t.Fatalf("pricing weight policy: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-mixed", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell,
		domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t),
		table,
		weightPolicy,
		rules,
		structures,
	)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}

func evaluateWithSides(t *testing.T, plan domain.PricingPlanVersion, id, longest string) domain.PricingEvaluation {
	t.Helper()
	sides := dimensions(t, longest, "10", "10", domain.LengthUnitInch)
	request, err := domain.NewEvaluationRequest(
		mustValue(t, domain.NewEvaluationID, id),
		plan,
		syntheticInputWithDimensions(t, "5", "Z1", sides),
		domain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return domain.EvaluatePricing(request)
}

func hasChargeCode(evaluation domain.PricingEvaluation, code string) bool {
	for _, line := range evaluation.ChargeLines() {
		if line.Code().String() == code {
			return true
		}
	}
	return false
}

func explanationMentions(evaluation domain.PricingEvaluation, fragment string) bool {
	for _, entry := range evaluation.Explanation() {
		if strings.Contains(entry, fragment) {
			return true
		}
	}
	return false
}

func standaloneSurcharges(t testing.TB, rules ...domain.SurchargeRule) domain.PricingPlanStructures {
	t.Helper()
	declared := make([]domain.SurchargeRule, 0, len(rules))
	for _, rule := range rules {
		declared = append(declared, standaloneRule(t, rule))
	}
	return declaredSurcharges(t, declared...)
}

func declaredSurcharges(t testing.TB, rules ...domain.SurchargeRule) domain.PricingPlanStructures {
	t.Helper()
	structures, err := domain.NewPricingPlanStructures(rules, nil, nil)
	if err != nil {
		t.Fatalf("plan structures: %v", err)
	}
	return structures
}

func groupedSurcharge(t testing.TB, rule domain.SurchargeRule, group string, priority int) domain.SurchargeRule {
	t.Helper()
	grouped, err := rule.InExclusivityGroup(group, priority)
	if err != nil {
		t.Fatalf("exclusivity group: %v", err)
	}
	return grouped
}

func surchargeRuleFor(t testing.TB, id, code, threshold, amount string) domain.SurchargeRule {
	t.Helper()
	return surchargeRule(t, id, code, threshold, amount)
}

func surchargeRuleWithCalculation(t testing.TB, id, code, threshold string, calculation domain.SurchargeCalculation) domain.SurchargeRule {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, threshold, domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	rule, err := domain.NewSurchargeRule(id, mustValue(t, domain.NewChargeCode, code), id, domain.ChargeEffectAdd, leafTrigger(t, condition), calculation)
	if err != nil {
		t.Fatalf("surcharge rule: %v", err)
	}
	return rule
}
