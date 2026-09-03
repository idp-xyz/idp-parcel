package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func pricePolicyVersion(t *testing.T, objectID string) domain.CommercialVersion {
	t.Helper()
	live, err := registerable(t, domain.PriceRuleObject, objectID, "v1", "sha256:"+objectID).
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

func taxInclusive(t *testing.T) domain.TaxCaliber {
	t.Helper()
	caliber, err := domain.NewTaxCaliber(domain.TaxInclusive,
		commercialValue(t, domain.NewTaxClassificationReference, "vat-standard"))
	if err != nil {
		t.Fatalf("new tax caliber: %v", err)
	}
	return caliber
}

func volumetricFor(t *testing.T, direction domain.PriceDirection) domain.VolumetricCaliber {
	t.Helper()
	factor := domain.VolumetricFactorReference{}
	if direction == domain.SellDirection {
		factor = commercialValue(t, domain.NewVolumetricFactorReference, "sell-divisor-5000-cm")
	}
	caliber, err := domain.NewVolumetricCaliber(direction, factor)
	if err != nil {
		t.Fatalf("new volumetric caliber: %v", err)
	}
	return caliber
}

func fxCaliber(t *testing.T) domain.FxCaliber {
	t.Helper()
	caliber, err := domain.NewFxCaliber(
		commercialValue(t, domain.NewFxQuoteTypeReference, "boc-cash-selling"),
		commercialValue(t, domain.NewAsOfSemanticsReference, "AT_ORDER_DATE"),
		commercialValue(t, domain.NewAsOfPolicyVersion, "asof-policy/v3"),
	)
	if err != nil {
		t.Fatalf("new fx caliber: %v", err)
	}
	return caliber
}

// Covers: CONTEXT「商业价格政策版本……声明计价所需的商业口径」与「商业价格规则必须声明含税、
// 未税或税务不适用」——税务与体积两格必需，汇率一格可缺（不涉及外币的政策没有汇率口径，缺席
// 是合法声明，与忘了填分开）。
func TestPricePolicyCaliberRequiresTaxAndVolumetricAndMayOmitFx(t *testing.T) {
	version := pricePolicyVersion(t, "price-1")

	t.Run("without fx", func(t *testing.T) {
		caliber, err := domain.NewPricePolicyCaliber(version, taxInclusive(t), volumetricFor(t, domain.SellDirection))
		if err != nil {
			t.Fatalf("new caliber: %v", err)
		}
		if _, declared := caliber.Fx(); declared {
			t.Fatal("没声明汇率口径却读出了一份")
		}
		if caliber.Tax().Disposition() != domain.TaxInclusive || caliber.Volumetric().Direction() != domain.SellDirection {
			t.Fatalf("两格必需口径被改动：%#v", caliber)
		}
		if !caliber.Version().SameVersionAs(version) {
			t.Fatal("口径挂到了另一个版本")
		}
	})

	t.Run("with fx", func(t *testing.T) {
		caliber, err := domain.NewPricePolicyCaliberWithFx(version, taxInclusive(t), volumetricFor(t, domain.BuyDirection), fxCaliber(t))
		if err != nil {
			t.Fatalf("new caliber with fx: %v", err)
		}
		fx, declared := caliber.Fx()
		if !declared || fx.QuoteType().String() != "boc-cash-selling" {
			t.Fatalf("汇率口径 = (%v, %v)", fx, declared)
		}
	})

	t.Run("zero-value tax or volumetric is refused", func(t *testing.T) {
		if _, err := domain.NewPricePolicyCaliber(version, domain.TaxCaliber{}, volumetricFor(t, domain.SellDirection)); !errors.Is(err, domain.ErrInvalidPricePolicyCaliber) {
			t.Fatalf("error = %v；没有税务口径也立住了", err)
		}
		if _, err := domain.NewPricePolicyCaliber(version, taxInclusive(t), domain.VolumetricCaliber{}); !errors.Is(err, domain.ErrInvalidPricePolicyCaliber) {
			t.Fatalf("error = %v；没有体积口径也立住了", err)
		}
		if _, err := domain.NewPricePolicyCaliberWithFx(version, taxInclusive(t), volumetricFor(t, domain.SellDirection), domain.FxCaliber{}); !errors.Is(err, domain.ErrInvalidPricePolicyCaliber) {
			t.Fatalf("error = %v；零值汇率口径经 WithFx 立住了——要缺席该走不带 Fx 的构造", err)
		}
	})

	t.Run("another object kind is refused", func(t *testing.T) {
		if _, err := domain.NewPricePolicyCaliber(contractVersion(t, "contract-9"), taxInclusive(t), volumetricFor(t, domain.SellDirection)); !errors.Is(err, domain.ErrInvalidPricePolicyCaliber) {
			t.Fatalf("error = %v, want ErrInvalidPricePolicyCaliber", err)
		}
	})
}

// Covers: 口径里的体积方向必须与政策自己的方向一致——采购政策挂一份销售方向的体积口径，等于
// 给采购方向声明了承运商价卡之外的第二份系数，正是 VolumetricCaliber 那条禁令要挡的。
func TestPricePolicyCaliberMustAgreeWithThePolicyDirection(t *testing.T) {
	version := pricePolicyVersion(t, "price-1")
	caliber, err := domain.NewPricePolicyCaliber(version, taxInclusive(t), volumetricFor(t, domain.SellDirection))
	if err != nil {
		t.Fatalf("new caliber: %v", err)
	}
	if err := caliber.ConsistentWithDirection(domain.SellDirection); err != nil {
		t.Fatalf("同向却报不一致：%v", err)
	}
	if err := caliber.ConsistentWithDirection(domain.BuyDirection); !errors.Is(err, domain.ErrPricePolicyCaliberDirectionMismatch) {
		t.Fatalf("error = %v；销售方向的体积口径挂上了采购政策", err)
	}
}
