package domain_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证售价卡声明适用服务产品的领域一半（ADR-0382 决定一、四）：只有 SELL·客户费用能声明，去重排序后保存，带产品的卡写
// PPC-7、进内容摘要，不带的卡摘要与改动前逐字节相同。

// unnamedSellPinnedDigest 取证于改动前提交 37c172945：同一张不声明产品的 SELL·客户费用卡（sellUnnamedPlan）那时算出的内容
// 摘要。适用产品是可缺键，不声明的卡必须仍落在这一串上（ADR-0382 决定四「不带的……字节与摘要不动」）。
const unnamedSellPinnedDigest = "881e0a73099cabf819a2fd54a3f8e2a2c86b02d6c7560ee7860dbf34271608a5"

func sellUnnamedPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	return syntheticPlan(t, "sell-unnamed", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "12.00",
		domain.PricingWeightActualOnly, []domain.FixedChargeRule{fixedRule(t, "handling", domain.ChargeEffectAdd, "1.50", 1)})
}

func product(t testing.TB, value string) domain.ServiceProductReference {
	t.Helper()
	return mustValue(t, domain.NewServiceProductReference, value)
}

func TestAPlanThatNamesNoServiceProductKeepsItsDigestFromBeforeTheKeyExisted(t *testing.T) {
	plan := sellUnnamedPlan(t)
	if plan.ContentDigest() != unnamedSellPinnedDigest || plan.CanonicalizationVersion() != "PPC-5" {
		t.Fatalf("不声明产品的卡摘要变了：%s（%s），取证于改动前为 %s（PPC-5）", plan.ContentDigest(), plan.CanonicalizationVersion(), unnamedSellPinnedDigest)
	}
	if len(plan.ServiceProducts()) != 0 {
		t.Fatalf("没声明却带了适用产品：%v", plan.ServiceProducts())
	}
	raw, rebuilt := planSnapshotRoundTrip(t, plan)
	if strings.Contains(string(raw), "serviceProducts") {
		t.Fatalf("不声明的卡的快照带了 serviceProducts 键：%s", raw)
	}
	if rebuilt.ContentDigest() != unnamedSellPinnedDigest {
		t.Fatalf("快照往返后摘要变了：%s", rebuilt.ContentDigest())
	}
}

func TestOnlyASellCustomerChargePlanMayNameServiceProducts(t *testing.T) {
	for name, plan := range map[string]domain.PricingPlanVersion{
		"BUY":      syntheticPlan(t, "buy", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, "8", domain.PricingWeightActualOnly, nil),
		"INTERNAL": syntheticPlan(t, "internal", domain.PricingDirectionInternal, domain.PricingPurposeInternalPrice, "8", domain.PricingWeightActualOnly, nil),
	} {
		if _, err := plan.WithServiceProducts(product(t, "SYN-PROD-FBA")); !errors.Is(err, domain.ErrServiceProductsOnlyOnSellCustomerCharge) ||
			!errors.Is(err, domain.ErrInvalidPricingPlan) {
			t.Fatalf("%s 卡声明适用产品：%v，应答构造错误", name, err)
		}
	}
	if _, err := sellUnnamedPlan(t).WithServiceProducts(); !errors.Is(err, domain.ErrInvalidPricingPlan) {
		t.Fatalf("声明零个产品：%v，应答构造错误——不声明就别调", err)
	}
	if _, err := domain.NewServiceProductReference(" SYN-PROD-FBA"); err == nil {
		t.Fatal("带首尾空白的产品标识立住了")
	}
	if _, err := domain.NewServiceProductReference("SYN-PROD-A;SYN-PROD-B"); !errors.Is(err, domain.ErrInvalidIdentifier) {
		t.Fatalf("带分号的产品标识：%v，分号是模板列表的分隔符，标识里不许有", err)
	}
	if (domain.ServiceProductReference{}).Declared() {
		t.Fatal("零值报成有产品")
	}
}

func TestServiceProductsAreDeduplicatedAndSortedAndWrittenAsPPC7(t *testing.T) {
	plain := sellUnnamedPlan(t)
	named, err := plain.WithServiceProducts(product(t, "SYN-PROD-FBA"), product(t, "SYN-PROD-EXPRESS"), product(t, "SYN-PROD-FBA"))
	if err != nil {
		t.Fatalf("声明适用产品：%v", err)
	}
	reordered, err := plain.WithServiceProducts(product(t, "SYN-PROD-EXPRESS"), product(t, "SYN-PROD-FBA"))
	if err != nil {
		t.Fatalf("声明适用产品：%v", err)
	}
	got := named.ServiceProducts()
	if len(got) != 2 || got[0].String() != "SYN-PROD-EXPRESS" || got[1].String() != "SYN-PROD-FBA" {
		t.Fatalf("适用产品没去重排序：%v", got)
	}
	if named.CanonicalizationVersion() != "PPC-7" || named.ContentDigest() != reordered.ContentDigest() {
		t.Fatalf("版本 %s，摘要 %s 对 %s：同一组产品换个写法应同摘要、写 PPC-7", named.CanonicalizationVersion(), named.ContentDigest(), reordered.ContentDigest())
	}
	if named.ContentDigest() == plain.ContentDigest() {
		t.Fatal("声明了产品的卡与不声明的卡摘要相同：适用产品没进内容摘要")
	}
	onlyFBA, err := plain.WithServiceProducts(product(t, "SYN-PROD-FBA"))
	if err != nil {
		t.Fatalf("声明适用产品：%v", err)
	}
	if onlyFBA.ContentDigest() == named.ContentDigest() {
		t.Fatal("两张只差适用产品的卡摘要相同")
	}
	if plain.CanonicalizationVersion() != "PPC-5" || plain.ContentDigest() != unnamedSellPinnedDigest {
		t.Fatal("声明产品改动了原卡：值对象应交回副本")
	}

	bounded, err := boundedPlan(t, "25", "").WithServiceProducts(product(t, "SYN-PROD-FBA"))
	if err != nil {
		t.Fatalf("带总额下限的卡声明适用产品：%v", err)
	}
	if bounded.CanonicalizationVersion() != "PPC-7" {
		t.Fatalf("带总额下限又声明产品的卡写 %s，应写 PPC-7（PPC-7 是 PPC-6 的超集）", bounded.CanonicalizationVersion())
	}
}

func TestTheRegistrationSnapshotCarriesServiceProductsAndTheReadbackGateHoldsThem(t *testing.T) {
	named, err := sellUnnamedPlan(t).WithServiceProducts(product(t, "SYN-PROD-FBA"), product(t, "SYN-PROD-EXPRESS"))
	if err != nil {
		t.Fatalf("声明适用产品：%v", err)
	}
	raw, rebuilt := planSnapshotRoundTrip(t, named)
	if !strings.Contains(string(raw), `"serviceProducts":["SYN-PROD-EXPRESS","SYN-PROD-FBA"]`) {
		t.Fatalf("快照没带排序后的适用产品：%s", raw)
	}
	if rebuilt.ContentDigest() != named.ContentDigest() || rebuilt.CanonicalizationVersion() != "PPC-7" || len(rebuilt.ServiceProducts()) != 2 {
		t.Fatalf("快照往返后变了：%s %s %v", rebuilt.CanonicalizationVersion(), rebuilt.ContentDigest(), rebuilt.ServiceProducts())
	}

	for name, mutate := range map[string]func(map[string]any){
		"次序不对": func(plan map[string]any) { plan["serviceProducts"] = []any{"SYN-PROD-FBA", "SYN-PROD-EXPRESS"} },
		"同一个写两遍": func(plan map[string]any) {
			plan["serviceProducts"] = []any{"SYN-PROD-EXPRESS", "SYN-PROD-EXPRESS", "SYN-PROD-FBA"}
		},
		"改了产品而摘要没变":      func(plan map[string]any) { plan["serviceProducts"] = []any{"SYN-PROD-EXPRESS", "SYN-PROD-OTHER"} },
		"带产品却记着 PPC-6":   func(plan map[string]any) { plan["canonicalization"] = "PPC-6" },
		"去掉产品却仍记着 PPC-7": func(plan map[string]any) { delete(plan, "serviceProducts") },
		"写了键却是空列表":       func(plan map[string]any) { plan["serviceProducts"] = []any{} },
		"标识带分号":          func(plan map[string]any) { plan["serviceProducts"] = []any{"SYN-PROD-EXPRESS;SYN-PROD-FBA"} },
		"空标识":            func(plan map[string]any) { plan["serviceProducts"] = []any{"", "SYN-PROD-FBA"} },
	} {
		if _, err := domain.RehydratePriceCardRegistration(tamperedDocument(t, raw, func(document map[string]any) {
			mutate(document["plan"].(map[string]any))
		})); err == nil {
			t.Fatalf("%s：读回门放行了", name)
		}
	}

	// BUY 卡带着适用产品的快照：构造门拒的，读回门同样拒。
	buy := syntheticPlan(t, "buy", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, "8", domain.PricingWeightActualOnly, nil)
	buyRaw, _ := planSnapshotRoundTrip(t, buy)
	if _, err := domain.RehydratePriceCardRegistration(tamperedDocument(t, buyRaw, func(document map[string]any) {
		plan := document["plan"].(map[string]any)
		plan["serviceProducts"] = []any{"SYN-PROD-FBA"}
		plan["canonicalization"] = "PPC-7"
	})); err == nil {
		t.Fatal("BUY 卡带适用产品的快照被读回门放行了")
	}
}

func TestAnEvaluationOfAPPC7PlanIsWrittenUnderPPC7AndReplaysToTheSameDigest(t *testing.T) {
	named, err := sellUnnamedPlan(t).WithServiceProducts(product(t, "SYN-PROD-FBA"))
	if err != nil {
		t.Fatalf("声明适用产品：%v", err)
	}
	original := evaluate(t, "eval-ppc7-source", named, syntheticInput(t, "3", "Z1"))
	if original.Status() != domain.EvaluationCompleted || original.PlanCanonicalizationVersion() != "PPC-7" {
		t.Fatalf("状态 %s 版本 %s", original.Status(), original.PlanCanonicalizationVersion())
	}
	raw, err := domain.MarshalEvaluationSnapshot(original)
	if err != nil {
		t.Fatalf("折装评价快照：%v", err)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("重建评价：%v", err)
	}
	replayed, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-ppc7-replay"), restored, named, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if replayed.Status() != domain.EvaluationCompleted || replayed.SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("重放：%s，摘要 %s 对 %s", replayed.Status(), replayed.SemanticDigest(), original.SemanticDigest())
	}
}

// 计价输入快照记选卡用的服务产品（ADR-0382 决定三）：可缺的一格，进评价快照与解释、不进语义摘要——同一张卡、同一份输入算出的
// 价不随它变。
func TestTheInputSnapshotRecordsTheServiceProductOutsideTheSemanticDigest(t *testing.T) {
	plan := sellUnnamedPlan(t)
	plain := syntheticInput(t, "3", "Z1")
	if _, declared := plain.ServiceProduct(); declared {
		t.Fatal("没记产品的输入报成记了")
	}
	if _, err := plain.WithServiceProduct(domain.ServiceProductReference{}); !errors.Is(err, domain.ErrPricingInputInvalid) {
		t.Fatalf("零值产品：%v，没按产品选就别调", err)
	}
	withProduct, err := plain.WithServiceProduct(product(t, "SYN-PROD-FBA"))
	if err != nil {
		t.Fatalf("记产品：%v", err)
	}

	without := evaluate(t, "eval-product", plan, plain)
	with := evaluate(t, "eval-product", plan, withProduct)
	if with.SemanticDigest() != without.SemanticDigest() {
		t.Fatal("服务产品进了语义摘要")
	}
	if explanation := with.Explanation(); len(explanation) == 0 || !strings.Contains(explanation[0], "SYN-PROD-FBA") {
		t.Fatalf("解释没点出按哪个产品选的卡：%v", explanation)
	}
	for _, line := range without.Explanation() {
		if strings.Contains(line, "service product") {
			t.Fatalf("没按产品选的评价解释里出现了产品：%v", without.Explanation())
		}
	}

	raw, err := domain.MarshalEvaluationSnapshot(with)
	if err != nil {
		t.Fatalf("折装评价快照：%v", err)
	}
	if !strings.Contains(string(raw), `"serviceProduct":"SYN-PROD-FBA"`) {
		t.Fatalf("评价快照没记产品：%s", raw)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("重建评价：%v", err)
	}
	if got, declared := restored.Input().ServiceProduct(); !declared || got.String() != "SYN-PROD-FBA" {
		t.Fatalf("重建后的产品 = %q %v", got.String(), declared)
	}
	plainRaw, err := domain.MarshalEvaluationSnapshot(without)
	if err != nil {
		t.Fatalf("折装评价快照：%v", err)
	}
	if strings.Contains(string(plainRaw), "serviceProduct") {
		t.Fatalf("没按产品选的评价快照带了 serviceProduct 键：%s", plainRaw)
	}
}
