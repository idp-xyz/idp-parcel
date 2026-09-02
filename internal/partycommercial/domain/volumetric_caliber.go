package domain

import "errors"

// ErrInvalidVolumetricCaliber 拒绝立不住的体积口径。
var ErrInvalidVolumetricCaliber = errors.New("party commercial: invalid volumetric caliber")

// VolumetricFactorReference 指向一份版本化声明的体积系数。本上下文持引用而不持数值本身：
// 系数带着单位与商精度，那是计价那侧的表达（`parcel-pricing` 的 `VolumetricFactor` 要求系数
// 声明单位一致与商落在哪个精度上），此处抄一份数会多出一处能与它分岔的副本。
type VolumetricFactorReference struct{ requiredValue }

func NewVolumetricFactorReference(value string) (VolumetricFactorReference, error) {
	required, err := newRequiredValue("volumetric factor reference", value)
	return VolumetricFactorReference{required}, err
}

// VolumetricCaliber 是按价格方向劈开的体积系数声明。
//
// **销售方向必须声明，采购与法人间方向不得声明。** 这不是「可选字段」：采购方向的系数写在承运商
// 价卡上（`parcel-pricing` 的 `VolumetricFactor` 注释举的例子就是卡上那句「体积磅 = 长×宽×高
// / 250」），销售方向没有承运商卡，只能由商业政策声明。采购方向再声明一份，同一个包裹就有两个
// 系数**各自都合法**，而计价选中哪一个取决于取数顺序、不取决于任何人的决定——那种冲突不会报错，
// 只会让计价重量悄悄变一档，而系数差异足以让单价更低的渠道总价更高（`PAR-NET-16`）。
//
// 法人间方向随采购办：它引用的是已经存在的成本口径，不新造一份系数。
type VolumetricCaliber struct {
	direction PriceDirection
	factor    VolumetricFactorReference
}

func NewVolumetricCaliber(
	direction PriceDirection,
	factor VolumetricFactorReference,
) (VolumetricCaliber, error) {
	switch direction {
	case SellDirection:
		if !factor.valid() {
			return VolumetricCaliber{}, ErrInvalidVolumetricCaliber
		}
	case BuyDirection, InternalDirection:
		if factor.valid() {
			return VolumetricCaliber{}, ErrInvalidVolumetricCaliber
		}
	default:
		return VolumetricCaliber{}, ErrInvalidVolumetricCaliber
	}
	return VolumetricCaliber{direction: direction, factor: factor}, nil
}

func (caliber VolumetricCaliber) Direction() PriceDirection {
	return caliber.direction
}

// Factor 的第二个返回值把「本方向不该有系数」与「本该有却缺了」分开。后者造不出来（构造门
// 已拒），所以为假只意味着前者；不带这个布尔的话，调用方拿到零值引用时分不出自己该去补一份
// 声明，还是该去别处（承运商价卡）取。
func (caliber VolumetricCaliber) Factor() (VolumetricFactorReference, bool) {
	return caliber.factor, caliber.factor.valid()
}
