package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func scope(t *testing.T, raw string) domain.EstimatePricingScope {
	t.Helper()
	built, err := domain.NewEstimatePricingScope(raw)
	if err != nil {
		t.Fatalf("计价范围 %q：%v", raw, err)
	}
	return built
}

// Covers: 有序来源原样保留声明的顺序、不排序，交出去的是副本——调用方事后改自己的切片不改答复。
func TestOrderedSellPriceSourcesKeepTheDeclaredOrder(t *testing.T) {
	sources := []domain.EstimatePricingScope{scope(t, "SYN-SCOPE-M"), scope(t, "SYN-SCOPE-Z"), scope(t, "SYN-SCOPE-A")}
	arrangement, err := domain.NewOrderedSellPriceSources(sources)
	if err != nil {
		t.Fatalf("有序来源：%v", err)
	}
	sources[0] = scope(t, "SYN-SCOPE-MUTATED")
	got, ok := arrangement.Sources()
	if !ok || arrangement.NotDeclared() {
		t.Fatalf("有序来源被读成别的答复：ok=%v notDeclared=%v", ok, arrangement.NotDeclared())
	}
	if _, priced := arrangement.NoSellPriceBasis(); priced {
		t.Fatal("有序来源同时带了不计销售价依据")
	}
	joined := got[0].String() + "," + got[1].String() + "," + got[2].String()
	if joined != "SYN-SCOPE-M,SYN-SCOPE-Z,SYN-SCOPE-A" {
		t.Fatalf("来源 = %s，want 声明顺序", joined)
	}
}

// Covers: 票面「三种不合并，也不拿空列表顶」——空列表造不出有序来源那一种；同一范围出现两次同拒。
func TestAnEmptySourceListIsNotASellPriceAnswer(t *testing.T) {
	if _, err := domain.NewOrderedSellPriceSources(nil); !errors.Is(err, domain.ErrInvalidAdoptedSellPriceArrangement) {
		t.Fatalf("空列表 err = %v", err)
	}
	repeated := scope(t, "SYN-SCOPE-M")
	if _, err := domain.NewOrderedSellPriceSources([]domain.EstimatePricingScope{repeated, repeated}); !errors.Is(err, domain.ErrInvalidAdoptedSellPriceArrangement) {
		t.Fatalf("同一范围两次 err = %v", err)
	}
}

// Covers: 显式不计销售价必带依据，且不被读成来源或没有约定。
func TestAnExplicitNoSellPriceKeepsItsBasis(t *testing.T) {
	basis, err := domain.NewNoSellPriceBasis("SYN-NO-SELL-PRICE-01")
	if err != nil {
		t.Fatalf("依据：%v", err)
	}
	arrangement, err := domain.NewExplicitlyNotPricedSellPrice(basis)
	if err != nil {
		t.Fatalf("不计销售价：%v", err)
	}
	got, ok := arrangement.NoSellPriceBasis()
	if !ok || got.String() != "SYN-NO-SELL-PRICE-01" {
		t.Fatalf("依据 = %q ok=%v", got.String(), ok)
	}
	if _, ordered := arrangement.Sources(); ordered || arrangement.NotDeclared() {
		t.Fatal("不计销售价被读成来源或没有约定")
	}
	if _, err := domain.NewExplicitlyNotPricedSellPrice(domain.NoSellPriceBasis{}); !errors.Is(err, domain.ErrInvalidAdoptedSellPriceArrangement) {
		t.Fatalf("空依据 err = %v", err)
	}
}

// Covers: 「这一版合同没有销售价格约定」是单独的一种；没构造过的零值三口都不是，不会被当成旧合同。
func TestAContractWithoutASellPriceArrangementIsAThirdAnswer(t *testing.T) {
	arrangement := domain.NewNotDeclaredSellPriceArrangement()
	if !arrangement.NotDeclared() {
		t.Fatal("没有约定没有被标成第三种")
	}
	if _, ordered := arrangement.Sources(); ordered {
		t.Fatal("没有约定被读成空列表在场")
	}
	if _, priced := arrangement.NoSellPriceBasis(); priced {
		t.Fatal("没有约定被读成不计销售价")
	}
	var zero domain.AdoptedSellPriceArrangement
	if zero.NotDeclared() {
		t.Fatal("零值被当成这一版合同没有销售价格约定")
	}
}
