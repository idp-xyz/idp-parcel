package main

import (
	"os"
	"path/filepath"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 演示租户的逐委托售价卡只挂在 SYN-SCOPE-SHIP-23 上。公共卡、专属卡仍是逐包裹。种子被改成逐包裹或换了范围，这里先红。
func TestTheShipmentDemoCardIsPerShipmentOnItsOwnScope(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing", "price-card-cn-sg-shipment.json"))
	if err != nil {
		t.Fatalf("读种子：%v", err)
	}
	card, err := domain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatalf("重建逐委托卡：%v", err)
	}
	plan := card.Plan()
	const wantDigest = "bf9d05239fc6d225bedd89d597ec12c89fb47e77078c569af1d7442ca67d7d07"
	if plan.ContentDigest() != wantDigest {
		t.Fatalf("内容摘要 %s，钉住的是 %s", plan.ContentDigest(), wantDigest)
	}
	if plan.Aggregation() != domain.AggregationPerShipment || plan.Scope().String() != "SYN-SCOPE-SHIP-23" ||
		plan.Direction() != domain.PricingDirectionSell || plan.Purpose() != domain.PricingPurposeCustomerCharge ||
		plan.CanonicalizationVersion() != "PPC-5" || plan.WeightPolicy().Method() != domain.PricingWeightActualOnly {
		t.Fatalf("聚合 %s 范围 %s 方向 %s 目的 %s 版本 %s 计费重 %s，应是逐委托、SYN-SCOPE-SHIP-23、SELL、客户费用、PPC-5、实重",
			plan.Aggregation(), plan.Scope(), plan.Direction(), plan.Purpose(), plan.CanonicalizationVersion(), plan.WeightPolicy().Method())
	}
}
