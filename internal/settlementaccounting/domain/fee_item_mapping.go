package domain

import "fmt"

// PricingChargeCode 是 parcel-pricing 评价费用行上的计价费用代码。代码的文法归计价，本上下文只按字面比对，不在这里
// 再立一份格式门——两边的门一旦漂开，映射册上就会出现计价永远交不出的代码，或拒掉计价真会交出的代码。
type PricingChargeCode struct{ requiredValue }

func NewPricingChargeCode(value string) (PricingChargeCode, error) {
	required, err := newRequiredValue("pricing charge code", value)
	return PricingChargeCode{required}, err
}

// FeeItemMappingBasisReference 指名一行费用项目映射成立的依据，例如合同条款或授权的运营决定。必填：价格代码映到
// 哪个费用项目是租户取值（ADR-0190 决定五），册上的每一行都要说得出凭什么。
type FeeItemMappingBasisReference struct{ requiredValue }

func NewFeeItemMappingBasisReference(value string) (FeeItemMappingBasisReference, error) {
	required, err := newRequiredValue("fee item mapping basis reference", value)
	return FeeItemMappingBasisReference{required}, err
}

// FeeItemMapping 是费用项目映射册的一行：键是（租户、计价费用代码），值是（费用项目、登记依据）。只增不改：同一个
// 代码再登同一份是重放，内容不同是冲突、不覆盖——改映射会让已形成的费用与册上说法对不上。租户在册的键上。
type FeeItemMapping struct {
	code    PricingChargeCode
	feeItem FeeItemReference
	basis   FeeItemMappingBasisReference
}

func NewFeeItemMapping(
	code PricingChargeCode,
	feeItem FeeItemReference,
	basis FeeItemMappingBasisReference,
) (FeeItemMapping, error) {
	if !code.valid() || !feeItem.valid() || !basis.valid() {
		return FeeItemMapping{}, fmt.Errorf("%w: fee item mapping", ErrBlankValue)
	}
	return FeeItemMapping{code: code, feeItem: feeItem, basis: basis}, nil
}

func (mapping FeeItemMapping) ChargeCode() PricingChargeCode       { return mapping.code }
func (mapping FeeItemMapping) FeeItem() FeeItemReference           { return mapping.feeItem }
func (mapping FeeItemMapping) Basis() FeeItemMappingBasisReference { return mapping.basis }

// SameRegistration 比的是登记内容，不含落库时刻。
func (mapping FeeItemMapping) SameRegistration(other FeeItemMapping) bool {
	return mapping.code.String() == other.code.String() &&
		mapping.feeItem.String() == other.feeItem.String() &&
		mapping.basis.String() == other.basis.String()
}
