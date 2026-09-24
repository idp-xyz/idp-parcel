package domain

import (
	"errors"
	"fmt"
	"sort"
)

var (
	ErrInvalidLegalEntityIdentityLayer = errors.New("party commercial: invalid legal entity identity layer")
	// ErrIdentityLayerChangedWithoutCorrection 是一笔新修订改了身份层却没带身份更正依据。身份层不作变更
	// （ADR-0145 决定二）：号真的变了就是另一个法人，停用本法人、登记新法人；录错才走更正。
	ErrIdentityLayerChangedWithoutCorrection = errors.New("party commercial: identity layer changed without a correction basis")
	// ErrIdentityCorrectionWithoutChange 是身份更正依据出现在没改身份层的修订上——包括首笔登记与历史修订第一次
	// 补登身份层：前者之前没有登记过的号，后者是补上一直缺着的两格，都不是更正一个录错的号。
	ErrIdentityCorrectionWithoutChange = errors.New("party commercial: identity correction basis without an identity layer change")
)

// LifetimeRegistrationNumber 是责任法人身份上的一个终身注册号：按目录里哪一类登记，以及号本身。
// 号是否属该类型、格式是否合格，由写入用例按注册号类型目录判（RegistrationNumberTypeCatalogue.Check），
// 这里只保证两格都在。
type LifetimeRegistrationNumber struct {
	typeCode RegistrationNumberTypeCode
	number   RegistrationNumber
}

func NewLifetimeRegistrationNumber(
	typeCode RegistrationNumberTypeCode,
	number RegistrationNumber,
) (LifetimeRegistrationNumber, error) {
	if !typeCode.valid() || !number.valid() {
		return LifetimeRegistrationNumber{}, ErrInvalidLegalEntityIdentityLayer
	}
	return LifetimeRegistrationNumber{typeCode: typeCode, number: number}, nil
}

func (number LifetimeRegistrationNumber) TypeCode() RegistrationNumberTypeCode {
	return number.typeCode
}

func (number LifetimeRegistrationNumber) Number() RegistrationNumber {
	return number.number
}

// LegalEntityIdentityLayer 是责任法人身份层的两格（ADR-0145 决定一）：注册国家 / 地区与终身注册号。
// 它定义的是「这是哪一个法人」，随身份一起登记；注册地址、税务登记号、开票资料与联系人不在这里，
// 归法人资料。
//
// 一类只收一个号——终身注册号是一个法人一辈子一个的号，同一类给两个说明至少一个录错了。号按类型
// 代码升序存放：同一组号只有一种次序，登记册按内容比对重放时不随录入次序变答案。
type LegalEntityIdentityLayer struct {
	country RegistrationCountryCode
	numbers []LifetimeRegistrationNumber
}

func NewLegalEntityIdentityLayer(
	country RegistrationCountryCode,
	numbers []LifetimeRegistrationNumber,
) (LegalEntityIdentityLayer, error) {
	if !country.valid() {
		return LegalEntityIdentityLayer{}, fmt.Errorf("%w: registration country is required", ErrInvalidLegalEntityIdentityLayer)
	}
	if len(numbers) == 0 {
		return LegalEntityIdentityLayer{}, fmt.Errorf("%w: at least one lifetime registration number is required", ErrInvalidLegalEntityIdentityLayer)
	}
	ordered := make([]LifetimeRegistrationNumber, len(numbers))
	copy(ordered, numbers)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].typeCode.String() < ordered[j].typeCode.String()
	})
	for index, number := range ordered {
		if !number.typeCode.valid() || !number.number.valid() {
			return LegalEntityIdentityLayer{}, ErrInvalidLegalEntityIdentityLayer
		}
		if index > 0 && ordered[index-1].typeCode == number.typeCode {
			return LegalEntityIdentityLayer{}, fmt.Errorf(
				"%w: type %s appears twice", ErrInvalidLegalEntityIdentityLayer, number.typeCode)
		}
	}
	return LegalEntityIdentityLayer{country: country, numbers: ordered}, nil
}

func (layer LegalEntityIdentityLayer) Country() RegistrationCountryCode {
	return layer.country
}

// Numbers 交回一份拷贝，调用方改它不会改到身份层。
func (layer LegalEntityIdentityLayer) Numbers() []LifetimeRegistrationNumber {
	numbers := make([]LifetimeRegistrationNumber, len(layer.numbers))
	copy(numbers, layer.numbers)
	return numbers
}

// Equal 比国家 / 地区与整组号；号已按类型代码规整，逐项比即可。
func (layer LegalEntityIdentityLayer) Equal(other LegalEntityIdentityLayer) bool {
	if layer.country != other.country || len(layer.numbers) != len(other.numbers) {
		return false
	}
	for index := range layer.numbers {
		if layer.numbers[index] != other.numbers[index] {
			return false
		}
	}
	return true
}

func (layer LegalEntityIdentityLayer) valid() bool {
	return layer.country.valid() && len(layer.numbers) > 0
}

// CheckLegalEntityIdentitySuccession 判一笔新修订的身份层能不能接在册上最新修订之后（ADR-0145 决定二）。
//
//   - 最新修订带着身份层：新修订改了它，就必须带身份更正依据——更正声明的是「它一直就是这个号」；没改，
//     就不许带，一条不改任何东西的更正依据只会让读册的人去找一处并不存在的更正。
//   - 最新修订是本格落地之前的历史修订、没有身份层：新修订第一次补上两格，是补登不是更正，同样不许带。
//
// 新修订必须带身份层由写入用例在调它之前把门；停用修订由 LegalEntityRegistration.Deactivate 原样沿用
// 身份层、不经这里；首笔登记没有最新修订可比，也不经这里，它带不带更正依据由 WithIdentityLayer 判。
func CheckLegalEntityIdentitySuccession(latest, next LegalEntityRegistration) error {
	if !next.hasIdentity {
		return fmt.Errorf("%w: the successor revision carries no identity layer", ErrInvalidLegalEntityIdentityLayer)
	}
	if latest.hasIdentity && !latest.identity.Equal(next.identity) {
		if !next.hasCorrection {
			return ErrIdentityLayerChangedWithoutCorrection
		}
		return nil
	}
	if next.hasCorrection {
		return ErrIdentityCorrectionWithoutChange
	}
	return nil
}
