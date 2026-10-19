package parcelpricing_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// Covers: 票 demo-chain-gaps/30 完成判据「走 ADR-0200 路径时不再落金额未声明格」。演示成本卡照种子原样重建，对一件
// 3 kg / Z1 的包裹真算一份 BUY 评价，经本适配器译成采用快照：交回已完成、带最小币单位金额，而不是金额未声明。
// 卡上取整一格若被 seedgen 漏掉，评价带 AMOUNT_PRECISION_UNDECLARED，这里落到金额未声明那一格。证据只记 S。
func TestTheDemoCostCardYieldsAnAdoptableBuyEvaluation(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "scripts", "demo-seeds", "data", "pricing", "price-card-cn-sg-cost.json"))
	if err != nil {
		t.Fatalf("读演示成本卡：%v", err)
	}
	registration, err := ppdomain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatalf("重建演示成本卡：%v", err)
	}
	input := pptest.Input(t, pptest.InputSpec{
		Tenant:     "SYN-TENANT-01",
		Scope:      "SYN-SCOPE-01",
		PackageID:  "SYN-PARCEL-01",
		Zone:       "Z1",
		Kilograms:  "3",
		BusinessAt: time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	})
	evaluation := pptest.Evaluate(t, "SYN-EVAL-DEMO-COST", registration.Plan(), input)

	view := must[*adapter.BuyEvaluationAdapter](t)(adapter.NewBuyEvaluationAdapter(&evaluationSourceDouble{
		byID: map[string]ppdomain.PricingEvaluation{"SYN-EVAL-DEMO-COST": evaluation},
	}))
	adoption, found, err := view.LoadBuyEvaluation(t.Context(),
		must[sadomain.TenantID](t)(sadomain.NewTenantID("SYN-TENANT-01")),
		must[sadomain.BuyEvaluationReference](t)(sadomain.NewBuyEvaluationReference("SYN-EVAL-DEMO-COST")))
	if err != nil || !found {
		t.Fatalf("读采用快照：found=%v err=%v", found, err)
	}
	if adoption.Outcome != saports.BuyEvaluationCompleted {
		t.Fatalf("演示成本卡的评价 = %v，want 已完成；评价状态 %s，问题项 %v", adoption.Outcome, evaluation.Status(), evaluation.Issues())
	}
	// 3 kg 落在 1–5 kg 那一段，85 CNY；合计按卡上 0.01 的进位单位换写成 8500 分。
	if adoption.SettlementCurrency.String() != "CNY" || adoption.SettlementMinor != 8500 {
		t.Fatalf("金额 = %s %d，want CNY 8500", adoption.SettlementCurrency, adoption.SettlementMinor)
	}
}
