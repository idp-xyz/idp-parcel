package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「不得以金额为零的已完成评价表达不可计价、不可承运或无适用价格」— 卡对
// 超出最后一个尺寸档的包裹是拒绝，不是计费，CONTEXT 给了它自己的结果：明确排除形成不可
// 计价。报成已完成就得有金额，而唯一拿得出的金额是零——零金额的已完成评价到结算侧读起来
// 就是「我们算过价，收零元」。
func TestExcludedParcelIsUnratableRatherThanPricedAtZero(t *testing.T) {
	plan := planWithStructures(t, exclusionStructures(t, "48"))
	evaluation := evaluateWithSides(t, plan, "eval-excluded", "50")

	if evaluation.Status() != domain.EvaluationUnratable {
		t.Fatalf("status = %s, issues = %#v, want UNRATABLE", evaluation.Status(), evaluation.Issues())
	}
	if _, formed := evaluation.Total(); formed {
		t.Fatal("an excluded evaluation produced a total")
	}
	if lines := evaluation.ChargeLines(); len(lines) != 0 {
		t.Fatalf("charge lines = %#v, want none", lines)
	}
}

// Covers: CONTEXT「结果携带排除依据、版本清单和解释，不含金额与费用行」— 光一个状态没法
// 拿去对卡，所以那条拒绝了包裹的条款必须走进解释。
func TestUnratableEvaluationNamesTheClauseThatRefusedIt(t *testing.T) {
	plan := planWithStructures(t, exclusionStructures(t, "48"))
	evaluation := evaluateWithSides(t, plan, "eval-excluded-explained", "50")

	if !explanationMentions(evaluation, "oversize-refusal") {
		t.Fatalf("explanation = %#v, want the refusing clause named", evaluation.Explanation())
	}
}

// 不成立的条款必须让计价原样进行，否则声明任何一条拒绝就等于拒绝一切。
func TestExclusionRuleThatMissesLeavesPricingAlone(t *testing.T) {
	plan := planWithStructures(t, exclusionStructures(t, "96"))
	evaluation := evaluateWithSides(t, plan, "eval-not-excluded", "50")

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "10" {
		t.Fatalf("total = %s, want the ordinary 10", total.Amount().String())
	}
}

// Covers: CONTEXT「补充事实或重试不改变该结论」— 拒绝是确定结论，所以它必须在任何会报出
// 「缺东西」的判断之前得出。缺一条序列读数是待判断，那等于向调用方承诺补上就能出价；对一个
// 已被排除的包裹，这个承诺是假的，调用方会永远重试下去。
func TestExclusionIsDecidedBeforeAMissingSeriesReadingCanDeferIt(t *testing.T) {
	structures, err := seriesPlan(t, "0.8").Structures().WithExclusionRules(oversizeRefusal(t, "48"))
	if err != nil {
		t.Fatalf("attach exclusion: %v", err)
	}
	plan := planWithStructures(t, structures)
	evaluation := evaluateWithSides(t, plan, "eval-excluded-without-reading", "50")

	if evaluation.Status() != domain.EvaluationUnratable {
		t.Fatalf("status = %s, issues = %#v, want UNRATABLE rather than a deferral", evaluation.Status(), evaluation.Issues())
	}
}

// 拒绝不同包裹的两个方案是两个不同的方案，所以这条条款必须走进内容摘要，不能不计入地搭
// 个便车。
func TestPricingPlanContentDigestCoversExclusionRules(t *testing.T) {
	without := planWithStructures(t, declaredSurcharges(t))
	with := planWithStructures(t, exclusionStructures(t, "48"))
	other := planWithStructures(t, exclusionStructures(t, "96"))

	if without.ContentDigest() == with.ContentDigest() {
		t.Fatal("declaring an exclusion rule left the content digest unchanged")
	}
	if with.ContentDigest() == other.ContentDigest() {
		t.Fatal("changing the exclusion threshold left the content digest unchanged")
	}
}

func exclusionStructures(t testing.TB, threshold string) domain.PricingPlanStructures {
	t.Helper()
	structures, err := declaredSurcharges(t).WithExclusionRules(oversizeRefusal(t, threshold))
	if err != nil {
		t.Fatalf("exclusion structures: %v", err)
	}
	return structures
}

func oversizeRefusal(t testing.TB, threshold string) domain.ExclusionRule {
	t.Helper()
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, threshold, domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	rule, err := domain.NewExclusionRule("oversize-refusal", "over the last size tier the card carries", leafTrigger(t, condition))
	if err != nil {
		t.Fatalf("exclusion rule: %v", err)
	}
	return rule
}
