package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: CONTEXT「它还声明计价所需的商业口径：……以及销售方向的体积系数」，与 parcel-pricing
// 那侧「系数是价卡或商业价格政策的内容，随其版本化，不得内置为常量」。
//
// 两句合起来把责任按方向劈开：采购方向的系数写在承运商价卡上，销售方向没有承运商卡，只能由
// 商业政策声明。**因此这不是「可选字段」而是按方向的必需与禁止**——采购方向再声明一份，同一个
// 包裹就有两个系数各自都合法，而计价选中哪一个取决于取数顺序，不取决于任何人的决定。
func TestVolumetricCaliberIsRequiredForSellAndForbiddenForPurchase(t *testing.T) {
	factor := commercialValue(t, domain.NewVolumetricFactorReference, "sell-divisor-5000-cm")

	t.Run("sell must declare a factor", func(t *testing.T) {
		if _, err := domain.NewVolumetricCaliber(
			domain.SellDirection, domain.VolumetricFactorReference{},
		); !errors.Is(err, domain.ErrInvalidVolumetricCaliber) {
			t.Fatalf("error = %v；销售方向没有体积系数也立住了", err)
		}
		caliber, err := domain.NewVolumetricCaliber(domain.SellDirection, factor)
		if err != nil {
			t.Fatalf("new volumetric caliber: %v", err)
		}
		if got, ok := caliber.Factor(); !ok || got != factor {
			t.Fatalf("factor = (%v, %v)", got, ok)
		}
	})

	t.Run("purchase must not declare one", func(t *testing.T) {
		if _, err := domain.NewVolumetricCaliber(
			domain.BuyDirection, factor,
		); !errors.Is(err, domain.ErrInvalidVolumetricCaliber) {
			t.Fatalf("error = %v；采购方向声明了第二份体积系数", err)
		}
		caliber, err := domain.NewVolumetricCaliber(domain.BuyDirection, domain.VolumetricFactorReference{})
		if err != nil {
			t.Fatalf("new volumetric caliber: %v", err)
		}
		if _, ok := caliber.Factor(); ok {
			t.Fatal("采购方向交回了一份体积系数")
		}
	})

	t.Run("internal follows purchase", func(t *testing.T) {
		// 法人间结算价按采购侧办：它引用的是已经存在的成本口径，不新造一份系数。
		if _, err := domain.NewVolumetricCaliber(
			domain.InternalDirection, factor,
		); !errors.Is(err, domain.ErrInvalidVolumetricCaliber) {
			t.Fatalf("error = %v；法人间方向声明了体积系数", err)
		}
	})
}
