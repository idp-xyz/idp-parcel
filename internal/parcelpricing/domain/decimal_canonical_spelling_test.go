package domain_test

import (
	"encoding/json"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: ADR-0123 Decision 二「产出 Decimal 的算术一律经 decimalFromBig 或 ParseDecimal，不按字段构造」——
// 按基数百分比那条费用行曾是生产路径上唯一按字段移位的产出者（票 wiring-baseline-remainder/07 第四格）。
// 基数是 10 基础运费 + 5 处理费 = 15，10% 是 1.5；这条行的写法不得随卡有没有声明逐行取整而变，也不得
// 与固定费用行上同一个数的写法不同——语义摘要按 String() 取值，两种写法就是两个串。
func TestPercentOfBasisChargeLineIsSpelledCanonically(t *testing.T) {
	evaluation := evaluateWithSides(t, percentBasisPlan(t), "eval-percent-spelling", "50")
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	fuel := chargeLineNamed(t, evaluation, "FUEL")
	if got := fuel.Amount().Amount().String(); got != "1.5" {
		t.Fatalf("FUEL 行金额写法 = %q，规范写法应为 1.5（基数 15 的 10%%）", got)
	}

	// 快照里的字段写法也只有一种：系数 15、标度 1——不是系数 150、标度 2。
	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("折装评价快照：%v", err)
	}
	amount := fuelAmountDocument(t, raw)
	if amount["coefficient"] != "15" || amount["scale"] != float64(1) {
		t.Fatalf("快照里 FUEL 行金额字段 = %v，想要 coefficient=15 scale=1", amount)
	}
}

// Covers: ADR-0123 Decision 三「重建门拒绝非规范写法，不规范化」——同一个数换一种字段写法写进快照，重建门
// 要拒（与内容被改的快照同格），而不是原样收回、让语义摘要对同一个数出两个串。正向那一半钉住原样重建仍
// 得回同一份评价、同一个摘要；反向那一半把 FUEL 行金额改写成系数 150、标度 2——数没变，仍是 1.5，只有
// 写法变了。
func TestSnapshotSpellingTheSameNumberNonCanonicallyIsRejectedAtRebuild(t *testing.T) {
	evaluation := evaluateWithSides(t, percentBasisPlan(t), "eval-percent-rebuild", "50")
	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("折装评价快照：%v", err)
	}

	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("原样重建：%v", err)
	}
	if restored.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatal("原样重建改变了语义摘要")
	}
	if got := chargeLineNamed(t, restored, "FUEL").Amount().Amount().String(); got != "1.5" {
		t.Fatalf("原样重建后 FUEL 行金额写法 = %q，想要 1.5", got)
	}

	respelled := withFuelAmountSpelledAs(t, raw, "150", 2)
	if _, err := domain.RehydrateEvaluationSnapshot(respelled); !errors.Is(err, domain.ErrEvaluationSnapshotInvalid) {
		t.Fatalf("同一个数的非规范写法进了重建门：err = %v，想要 ErrEvaluationSnapshotInvalid", err)
	}
}

// percentBasisPlan 是 TestPercentSurchargeChargesItsDeclaredShareOfTheBasis 用的那张卡：10 基础运费、
// 5 处理费、3 复核费，燃油按「全部其他费用扣除复核费」的 10% 计——基数 15，燃油 1.5。
func percentBasisPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	return dependencyPlan(t,
		[]domain.FixedChargeRule{
			fixedRule(t, "handling", domain.ChargeEffectAdd, "5", 1),
			fixedRule(t, "audit", domain.ChargeEffectAdd, "3", 2),
		},
		allChargesBasis(t, "fuel", "FUEL", "RULE_AUDIT"),
		percentRule(t, "fuel", "FUEL", "48", "10", "fuel"),
	)
}

func chargeLineNamed(t testing.TB, evaluation domain.PricingEvaluation, code string) domain.ChargeLine {
	t.Helper()
	for _, line := range evaluation.ChargeLines() {
		if line.Code().String() == code {
			return line
		}
	}
	t.Fatalf("评价里没有费用代码 %s 的行：%#v", code, evaluation.ChargeLines())
	return domain.ChargeLine{}
}

// fuelAmountDocument 从评价快照的 JSON 里取出 FUEL 行金额的 decimal 文档（coefficient / scale 两格）。
func fuelAmountDocument(t testing.TB, raw []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("解开快照：%v", err)
	}
	lines, ok := document["chargeLines"].([]any)
	if !ok {
		t.Fatalf("快照缺费用行：%v", document["chargeLines"])
	}
	for _, entry := range lines {
		line := entry.(map[string]any)
		if line["code"] != "FUEL" {
			continue
		}
		money := line["amount"].(map[string]any)
		return money["amount"].(map[string]any)
	}
	t.Fatal("快照里没有 FUEL 行")
	return nil
}

// withFuelAmountSpelledAs 把快照里 FUEL 行金额的两个字段改写成给定写法，其余一字不动。
func withFuelAmountSpelledAs(t testing.TB, raw []byte, coefficient string, scale uint32) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("解开快照：%v", err)
	}
	for _, entry := range document["chargeLines"].([]any) {
		line := entry.(map[string]any)
		if line["code"] != "FUEL" {
			continue
		}
		amount := line["amount"].(map[string]any)["amount"].(map[string]any)
		amount["coefficient"] = coefficient
		amount["scale"] = scale
	}
	respelled, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("重封快照：%v", err)
	}
	return respelled
}
