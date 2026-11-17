package main

import (
	"os"
	"path/filepath"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 专属卡只写谈下来的 Z1，分区目录仍是能解开 Z1 与 Z2 的那一本。摘要钉住：价表少一格而摘要没变，就是文件没改对。
func TestTheExclusiveDemoCardPricesOnlyTheNegotiatedZone(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing", "price-card-cn-sg-exclusive.json"))
	if err != nil {
		t.Fatalf("读种子：%v", err)
	}
	card, err := domain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatalf("重建专属卡：%v", err)
	}
	plan := card.Plan()
	if plan.ContentDigest() != "2f12fa95f6be62fdd5da512a1accc84c8ce9e8e7f6caafcd84c9dcb1b6f7af7b" {
		t.Fatalf("内容摘要 %s", plan.ContentDigest())
	}
	if plan.Scope().String() != "SYN-SCOPE-EXCL-01" {
		t.Fatalf("范围 %s", plan.Scope())
	}
	zones := map[string]bool{}
	for _, entry := range plan.RateTable().FirstContinueRates() {
		zones[entry.Zone()] = true
	}
	if !zones["Z1"] || zones["Z2"] || len(zones) != 1 {
		t.Fatalf("运费表分区 %v，应只有谈下来的 Z1", zones)
	}
	catalogues := plan.Structures().ReferenceCatalogues()
	if len(catalogues) != 1 || catalogues[0].CatalogueID() != "SYN-CAT-ZONE-CN-SG" {
		t.Fatalf("目录 %v，应仍绑 SYN-CAT-ZONE-CN-SG", catalogues)
	}
}
