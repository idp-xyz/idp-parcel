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

// Covers: CONTEXT「商业价格政策版本……可以声明汇率口径」与「商业价格规则必须声明含税、未税或税务
// 不适用」——税务一格必需，汇率一格可缺（不涉及外币的政策没有汇率口径，缺席是合法声明，与忘了填
// 分开）；方向是拥有口径的政策的方向，三个方向同一形状。AT-PC-062 的领域半边：口径上没有体积系数
// 可以带——那是价卡内容，任何方向都不在政策上声明（ADR-0260 决定二），只有税务的销售口径立得住。
func TestPricePolicyCaliberRequiresTaxAndMayOmitFx(t *testing.T) {
	version := pricePolicyVersion(t, "price-1")

	for _, direction := range []domain.PriceDirection{domain.SellDirection, domain.BuyDirection, domain.InternalDirection} {
		t.Run(direction.String()+" without fx", func(t *testing.T) {
			caliber, err := domain.NewPricePolicyCaliber(version, direction, taxInclusive(t))
			if err != nil {
				t.Fatalf("new caliber: %v", err)
			}
			if _, declared := caliber.Fx(); declared {
				t.Fatal("没声明汇率口径却读出了一份")
			}
			if caliber.Tax().Disposition() != domain.TaxInclusive || caliber.Direction() != direction {
				t.Fatalf("必需两格被改动：%#v", caliber)
			}
			if !caliber.Version().SameVersionAs(version) {
				t.Fatal("口径挂到了另一个版本")
			}
		})
	}

	t.Run("with fx", func(t *testing.T) {
		caliber, err := domain.NewPricePolicyCaliberWithFx(version, domain.BuyDirection, taxInclusive(t), fxCaliber(t))
		if err != nil {
			t.Fatalf("new caliber with fx: %v", err)
		}
		fx, declared := caliber.Fx()
		if !declared || fx.QuoteType().String() != "boc-cash-selling" {
			t.Fatalf("汇率口径 = (%v, %v)", fx, declared)
		}
	})

	t.Run("zero-value tax or direction is refused", func(t *testing.T) {
		if _, err := domain.NewPricePolicyCaliber(version, domain.SellDirection, domain.TaxCaliber{}); !errors.Is(err, domain.ErrInvalidPricePolicyCaliber) {
			t.Fatalf("error = %v；没有税务口径也立住了", err)
		}
		if _, err := domain.NewPricePolicyCaliber(version, domain.PriceDirectionInvalid, taxInclusive(t)); !errors.Is(err, domain.ErrInvalidPricePolicyCaliber) {
			t.Fatalf("error = %v；没有方向也立住了", err)
		}
		if _, err := domain.NewPricePolicyCaliberWithFx(version, domain.SellDirection, taxInclusive(t), domain.FxCaliber{}); !errors.Is(err, domain.ErrInvalidPricePolicyCaliber) {
			t.Fatalf("error = %v；零值汇率口径经 WithFx 立住了——要缺席该走不带 Fx 的构造", err)
		}
	})

	t.Run("another object kind is refused", func(t *testing.T) {
		if _, err := domain.NewPricePolicyCaliber(contractVersion(t, "contract-9"), domain.SellDirection, taxInclusive(t)); !errors.Is(err, domain.ErrInvalidPricePolicyCaliber) {
			t.Fatalf("error = %v, want ErrInvalidPricePolicyCaliber", err)
		}
	})
}
