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

// Covers: 票 demo-chain-gaps/30 完成判据「走 ADR-0200 路径时不再落金额未声明格」与票 43 完成判据「计价评价不再是
// ZONE_UNRESOLVED」。演示成本卡照种子原样重建，对一件 3 kg、邮编路线 200000 → 018000 的包裹真算一份 BUY 评价：卡绑了分区
// 目录，分区从演示目录按路线读出 Z1，读数照正式评价补齐读数的形状冻结进输入。经本适配器译成采用快照：交回已完成、带最小币
// 单位金额。卡上取整一格若被 seedgen 漏掉，落金额未声明；分区目录绑定若被漏掉，输入不带分区，落待判断。证据只记 S。
func TestTheDemoCostCardYieldsAnAdoptableBuyEvaluation(t *testing.T) {
	seeds := filepath.Join("..", "..", "..", "..", "scripts", "demo-seeds", "data", "pricing")
	raw, err := os.ReadFile(filepath.Join(seeds, "price-card-cn-sg-cost.json"))
	if err != nil {
		t.Fatalf("读演示成本卡：%v", err)
	}
	registration, err := ppdomain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatalf("重建演示成本卡：%v", err)
	}
	rawCatalogue, err := os.ReadFile(filepath.Join(seeds, "reference-catalogue-zone-cn-sg.json"))
	if err != nil {
		t.Fatalf("读演示分区目录：%v", err)
	}
	catalogue, err := ppdomain.RehydrateReferenceCatalogueRegistration(rawCatalogue)
	if err != nil {
		t.Fatalf("重建演示分区目录：%v", err)
	}
	businessAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	route, err := ppdomain.NewPostalRoute("200000", "018000")
	if err != nil {
		t.Fatalf("邮编路线：%v", err)
	}
	reading, applicable := catalogue.ResolveAt(businessAt, route)
	if !applicable {
		t.Fatal("演示分区目录在计价时点不生效")
	}
	subject, err := ppdomain.NewAcceptedPackageSubject(pptest.Value(t, ppdomain.NewPackageID, "SYN-PARCEL-01"))
	if err != nil {
		t.Fatalf("评价主体：%v", err)
	}
	weight, err := ppdomain.NewWeightFromString("3", ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("重量：%v", err)
	}
	input, err := ppdomain.NewPostalPricingInputSnapshot(pptest.Value(t, ppdomain.NewTenantID, "SYN-TENANT-01"),
		pptest.Value(t, ppdomain.NewPricingScopeID, "SYN-SCOPE-01"), subject, route, weight, nil, businessAt)
	if err == nil {
		input, err = input.WithReferenceCatalogues(reading)
	}
	if err != nil {
		t.Fatalf("计价输入：%v", err)
	}
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
