package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidAddressNature 表示地址性质不在封闭词表内。
var ErrInvalidAddressNature = errors.New("parcel pricing: invalid address nature")

// ErrAddressNatureRequired 表示卡上有规则按地址性质判定、而输入没有声明地址性质。缺的是发起方补得上的事实，
// 所以评价答待判断（ADDRESS_NATURE_REQUIRED），不套默认住宅或默认商业；它仍是 ErrFeatureUnavailable 的一种，
// 直接调判定条件的人照旧按「特征不可用」识别它，只有评价这一层把它译成待判断。
var ErrAddressNatureRequired = fmt.Errorf("%w: a rule of the price card reads the address nature and the input declares none", ErrFeatureUnavailable)

// AddressNature 是收件地址的性质：商业或住宅。它与偏远档位是两个独立维度（CONTEXT「地址分类」）——档位由邮编
// 分类事实解出，性质是发起方或来源上下文声明的一格事实，计价自己既不推断也不拥有。词表封闭为这两个词；
// 日后要加第三个词（如邮政信箱）属于特征取值集合的扩充，进版本内容变化，不在这里顺手放行。
type AddressNature string

const (
	AddressNatureCommercial  AddressNature = "COMMERCIAL"
	AddressNatureResidential AddressNature = "RESIDENTIAL"
)

// NewAddressNature 只认封闭词表里的原词，大小写与空白都不宽容：它进评价输入快照与卡上的期望取值，
// 宽容一格就多出一种「看着相同的取值」。
func NewAddressNature(text string) (AddressNature, error) {
	nature := AddressNature(text)
	if !nature.valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidAddressNature, text)
	}
	return nature, nil
}

func (nature AddressNature) String() string { return string(nature) }

func (nature AddressNature) valid() bool {
	return nature == AddressNatureCommercial || nature == AddressNatureResidential
}
