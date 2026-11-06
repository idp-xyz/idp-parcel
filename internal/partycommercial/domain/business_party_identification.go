package domain

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	ErrInvalidExternalReference = errors.New("party commercial: invalid external system reference")
	// ErrBusinessPartyIdentityChangedWithoutCorrection 是一笔新修订改了或去掉了已登的识别号却没带身份更正依据。
	// 往来单位的号与法人身份层同一条变更语义（ADR-0367 决定一沿 ADR-0145 决定二）：号真的变了就是另一个法律
	// 主体，停用本单位、登记新单位；当初录错了才走更正。
	ErrBusinessPartyIdentityChangedWithoutCorrection = errors.New("party commercial: business party identification changed without a correction basis")
)

// externalSystemCodeShape 收租户自定的系统标识：字母或数字开头，其后字母、数字、点、下划线、连字符，至多四十个字符。
// 不收空白——系统标识是拿来与对方系统对账的键，「KD ERP」与「KD  ERP」在屏幕上分不出，在库里却是两个系统。
var externalSystemCodeShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)

// ExternalSystemCode 是租户给某个外部系统起的标识（ADR-0367 决定三）。哪些系统由租户定，产品不预置。
type ExternalSystemCode struct{ value string }

func NewExternalSystemCode(value string) (ExternalSystemCode, error) {
	if !externalSystemCodeShape.MatchString(value) {
		return ExternalSystemCode{}, fmt.Errorf("%w: system %q", ErrInvalidExternalReference, value)
	}
	return ExternalSystemCode{value: value}, nil
}

func (code ExternalSystemCode) String() string {
	return code.value
}

// ExternalReferencePurpose 说这个编码在对方系统里是哪一本主档的码。封闭三值：对方系统常把客户主档与供应商主档
// 分成两本，同一家公司在那边两个码；只有一本往来主档的系统用 PARTY。
type ExternalReferencePurpose uint8

const (
	ExternalReferencePurposeInvalid ExternalReferencePurpose = iota
	ExternalCustomerCode
	ExternalSupplierCode
	ExternalPartyCode
)

func ParseExternalReferencePurpose(value string) (ExternalReferencePurpose, error) {
	switch value {
	case "CUSTOMER":
		return ExternalCustomerCode, nil
	case "SUPPLIER":
		return ExternalSupplierCode, nil
	case "PARTY":
		return ExternalPartyCode, nil
	default:
		return ExternalReferencePurposeInvalid, fmt.Errorf("%w: purpose %q", ErrInvalidExternalReference, value)
	}
}

func (purpose ExternalReferencePurpose) String() string {
	switch purpose {
	case ExternalCustomerCode:
		return "CUSTOMER"
	case ExternalSupplierCode:
		return "SUPPLIER"
	case ExternalPartyCode:
		return "PARTY"
	default:
		return ""
	}
}

// ExternalReference 是往来单位在一个外部系统里的一个编码：哪个系统、哪一本主档、码是什么。
type ExternalReference struct {
	system  ExternalSystemCode
	purpose ExternalReferencePurpose
	code    string
}

// NewExternalReference 只收首尾没有空白的码：码与对方系统逐字比对，首尾空白只会让两边各说各的。
func NewExternalReference(system ExternalSystemCode, purpose ExternalReferencePurpose, code string) (ExternalReference, error) {
	if system.value == "" || purpose == ExternalReferencePurposeInvalid {
		return ExternalReference{}, ErrInvalidExternalReference
	}
	if strings.TrimSpace(code) == "" || strings.TrimSpace(code) != code || len(code) > 64 {
		return ExternalReference{}, fmt.Errorf("%w: code %q", ErrInvalidExternalReference, code)
	}
	return ExternalReference{system: system, purpose: purpose, code: code}, nil
}

// ParseExternalReference 从三个原文串立起一个外部编码，供载荷与快照两边的翻译共用。
func ParseExternalReference(system, purpose, code string) (ExternalReference, error) {
	systemCode, err := NewExternalSystemCode(system)
	if err != nil {
		return ExternalReference{}, err
	}
	parsed, err := ParseExternalReferencePurpose(purpose)
	if err != nil {
		return ExternalReference{}, err
	}
	return NewExternalReference(systemCode, parsed, code)
}

func (reference ExternalReference) System() ExternalSystemCode {
	return reference.system
}

func (reference ExternalReference) Purpose() ExternalReferencePurpose {
	return reference.purpose
}

func (reference ExternalReference) Code() string {
	return reference.code
}

// canonicalExternalReferences 按（系统，用途）排序并拒同一（系统，用途）给两个码：同一组编码只有一种次序，登记册
// 按内容摘要判重放时不随录入次序变答案；一家单位在同一本主档里只有一个码，给两个说明至少一个录错了。
func canonicalExternalReferences(references []ExternalReference) ([]ExternalReference, error) {
	if len(references) == 0 {
		return nil, nil
	}
	ordered := make([]ExternalReference, len(references))
	copy(ordered, references)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].system.value != ordered[j].system.value {
			return ordered[i].system.value < ordered[j].system.value
		}
		return ordered[i].purpose < ordered[j].purpose
	})
	for index, reference := range ordered {
		if reference.system.value == "" || reference.purpose == ExternalReferencePurposeInvalid || reference.code == "" {
			return nil, ErrInvalidExternalReference
		}
		if index > 0 && ordered[index-1].system == reference.system && ordered[index-1].purpose == reference.purpose {
			return nil, fmt.Errorf("%w: system %s purpose %s appears twice", ErrInvalidExternalReference, reference.system, reference.purpose)
		}
	}
	return ordered, nil
}

// WithIdentification 交回带着识别信息的同一笔修订（ADR-0367 决定一、三）。identity 为空即这笔修订不带号；
// correction 非空即这笔修订是身份更正。更正依据可以脱离号出现——去掉一个录错的号也是更正——但不能落在修订 1
// 上：首笔登记之前没有登过的号可更正。外部编码按（系统，用途）规整。
func (registration BusinessPartyRegistration) WithIdentification(
	identity *LegalEntityIdentityLayer,
	correction *IdentityBasisReference,
	references []ExternalReference,
) (BusinessPartyRegistration, error) {
	registration.identity = LegalEntityIdentityLayer{}
	registration.hasIdentity = false
	if identity != nil {
		if !identity.valid() {
			return BusinessPartyRegistration{}, ErrInvalidLegalEntityIdentityLayer
		}
		registration.identity = *identity
		registration.hasIdentity = true
	}
	registration.correction = IdentityBasisReference{}
	registration.hasCorrection = false
	if correction != nil {
		if !correction.valid() {
			return BusinessPartyRegistration{}, ErrInvalidIdentityRegistration
		}
		if registration.revision == 1 {
			return BusinessPartyRegistration{}, ErrIdentityCorrectionWithoutChange
		}
		registration.correction = *correction
		registration.hasCorrection = true
	}
	ordered, err := canonicalExternalReferences(references)
	if err != nil {
		return BusinessPartyRegistration{}, err
	}
	registration.references = ordered
	return registration, nil
}

// IdentityLayer 交回这笔修订登记的注册国家与终身注册号；没带答 false。
func (registration BusinessPartyRegistration) IdentityLayer() (LegalEntityIdentityLayer, bool) {
	return registration.identity, registration.hasIdentity
}

// IdentityCorrectionBasis 交回身份更正依据；不是身份更正的修订答 false。
func (registration BusinessPartyRegistration) IdentityCorrectionBasis() (IdentityBasisReference, bool) {
	return registration.correction, registration.hasCorrection
}

// ExternalReferences 交回一份按（系统，用途）排好的拷贝。
func (registration BusinessPartyRegistration) ExternalReferences() []ExternalReference {
	if len(registration.references) == 0 {
		return nil
	}
	references := make([]ExternalReference, len(registration.references))
	copy(references, registration.references)
	return references
}

// CheckBusinessPartyIdentitySuccession 判一笔新修订的识别号能不能接在册上最新修订之后。和法人的接续判法
// （CheckLegalEntityIdentitySuccession）只差一处：往来单位的号可缺，所以「第一次补登」与「去掉已登的号」都是
// 合法的形状——前者不是更正、不许带更正依据，后者改了已登的号、必须带。
func CheckBusinessPartyIdentitySuccession(latest, next BusinessPartyRegistration) error {
	changed := latest.hasIdentity && (!next.hasIdentity || !latest.identity.Equal(next.identity))
	if changed {
		if !next.hasCorrection {
			return ErrBusinessPartyIdentityChangedWithoutCorrection
		}
		return nil
	}
	if next.hasCorrection {
		return ErrIdentityCorrectionWithoutChange
	}
	return nil
}
