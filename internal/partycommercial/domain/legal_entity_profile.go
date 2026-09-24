package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var ErrInvalidLegalEntityProfile = errors.New("party commercial: invalid legal entity profile revision")

// LegalEntityProfileBasisReference 指向一笔法人资料修订所依据的东西（CONTEXT Lifecycles「法人资料」：修订必须
// 携带登记依据）。判据同 IdentityBasisReference，但资料不是身份，两个引用不共用一个类型。
type LegalEntityProfileBasisReference struct{ requiredValue }

func NewLegalEntityProfileBasisReference(value string) (LegalEntityProfileBasisReference, error) {
	required, err := newRequiredValue("legal entity profile basis reference", value)
	return LegalEntityProfileBasisReference{required}, err
}

// RegisteredAddress 是法人资料上的注册地址：国家 / 地区与地址行。国家 / 地区单列一格而不埋进地址行，是因为
// 写入用例要拿它对身份上的注册国家 / 地区（ADR-0145 决定三），埋进行文就只能靠猜。地址行按录入次序保存，
// 次序即内容。
type RegisteredAddress struct {
	country RegistrationCountryCode
	lines   []string
}

func NewRegisteredAddress(country RegistrationCountryCode, lines []string) (RegisteredAddress, error) {
	if !country.valid() {
		return RegisteredAddress{}, fmt.Errorf("%w: registered address needs a country or region", ErrInvalidLegalEntityProfile)
	}
	if len(lines) == 0 {
		return RegisteredAddress{}, fmt.Errorf("%w: registered address needs at least one line", ErrInvalidLegalEntityProfile)
	}
	kept := make([]string, len(lines))
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			return RegisteredAddress{}, fmt.Errorf("%w: registered address line %d is blank", ErrInvalidLegalEntityProfile, index+1)
		}
		kept[index] = line
	}
	return RegisteredAddress{country: country, lines: kept}, nil
}

func (address RegisteredAddress) Country() RegistrationCountryCode {
	return address.country
}

// Lines 交回一份拷贝，调用方改它不会改到地址。
func (address RegisteredAddress) Lines() []string {
	lines := make([]string, len(address.lines))
	copy(lines, address.lines)
	return lines
}

func (address RegisteredAddress) valid() bool {
	return address.country.valid() && len(address.lines) > 0
}

// TaxRegistrationNumber 是法人资料上的一个税务登记号：按目录里哪一类登记，以及号本身。它属资料层——可后办、
// 可变的那类（ADR-0145 决定一、三）；号是否属该类型、格式是否合格，由写入用例按注册号类型目录判。
type TaxRegistrationNumber struct {
	typeCode RegistrationNumberTypeCode
	number   RegistrationNumber
}

func NewTaxRegistrationNumber(
	typeCode RegistrationNumberTypeCode,
	number RegistrationNumber,
) (TaxRegistrationNumber, error) {
	if !typeCode.valid() || !number.valid() {
		return TaxRegistrationNumber{}, fmt.Errorf("%w: tax registration number needs a type and a number", ErrInvalidLegalEntityProfile)
	}
	return TaxRegistrationNumber{typeCode: typeCode, number: number}, nil
}

func (number TaxRegistrationNumber) TypeCode() RegistrationNumberTypeCode {
	return number.typeCode
}

func (number TaxRegistrationNumber) Number() RegistrationNumber {
	return number.number
}

// InvoiceTitle 是开票抬头：开票用的法人名称，可以是与参与方名称不同的语言版本（ADR-0145 决定三）。
type InvoiceTitle struct{ requiredValue }

func NewInvoiceTitle(value string) (InvoiceTitle, error) {
	required, err := newRequiredValue("invoice title", value)
	return InvoiceTitle{required}, err
}

// InvoicingDetails 是开票资料。首版只含开票抬头；其余开票项随开立方的实施票按需增列，不预设（ADR-0145
// 决定三）——所以单立一个类型，增列时不必改动法人资料的形状。
type InvoicingDetails struct {
	title InvoiceTitle
}

func NewInvoicingDetails(title InvoiceTitle) (InvoicingDetails, error) {
	if !title.valid() {
		return InvoicingDetails{}, fmt.Errorf("%w: invoicing details need an invoice title", ErrInvalidLegalEntityProfile)
	}
	return InvoicingDetails{title: title}, nil
}

func (details InvoicingDetails) Title() InvoiceTitle {
	return details.title
}

// LegalEntityContact 是法人资料上的一位联系人：姓名必填，邮箱与电话可空，取值照录入原文，不在这里判格式。
type LegalEntityContact struct {
	name  string
	email string
	phone string
}

func NewLegalEntityContact(name, email, phone string) (LegalEntityContact, error) {
	if strings.TrimSpace(name) == "" {
		return LegalEntityContact{}, fmt.Errorf("%w: contact needs a name", ErrInvalidLegalEntityProfile)
	}
	contact := LegalEntityContact{name: name}
	if strings.TrimSpace(email) != "" {
		contact.email = email
	}
	if strings.TrimSpace(phone) != "" {
		contact.phone = phone
	}
	return contact, nil
}

func (contact LegalEntityContact) Name() string {
	return contact.name
}

func (contact LegalEntityContact) Email() string {
	return contact.email
}

func (contact LegalEntityContact) Phone() string {
	return contact.phone
}

// LegalEntityProfileContent 是一笔法人资料修订的正文（ADR-0145 决定三）。注册地址必填；税务登记号可后办、
// 可以为空；开票资料可以缺——缺的后果在解析时才出现（决定六：登记时不拦，开立时答`资料不全`）；联系人可空。
//
// 税务登记号按类型代码、再按号升序存放：同一组号只有一种次序，登记册按内容比对重放时不随录入次序变答案。
// 同一类型可以有多个号，同一类型下同一个号只收一次。联系人照录入次序保存，次序即内容。币种不在这里
// （决定四）。
type LegalEntityProfileContent struct {
	address      RegisteredAddress
	taxNumbers   []TaxRegistrationNumber
	invoicing    InvoicingDetails
	hasInvoicing bool
	contacts     []LegalEntityContact
}

func NewLegalEntityProfileContent(
	address RegisteredAddress,
	taxNumbers []TaxRegistrationNumber,
	invoicing *InvoicingDetails,
	contacts []LegalEntityContact,
) (LegalEntityProfileContent, error) {
	if !address.valid() {
		return LegalEntityProfileContent{}, fmt.Errorf("%w: registered address is required", ErrInvalidLegalEntityProfile)
	}
	ordered := make([]TaxRegistrationNumber, len(taxNumbers))
	copy(ordered, taxNumbers)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].typeCode != ordered[j].typeCode {
			return ordered[i].typeCode.String() < ordered[j].typeCode.String()
		}
		return ordered[i].number.String() < ordered[j].number.String()
	})
	for index, number := range ordered {
		if !number.typeCode.valid() || !number.number.valid() {
			return LegalEntityProfileContent{}, fmt.Errorf("%w: tax registration number needs a type and a number", ErrInvalidLegalEntityProfile)
		}
		if index > 0 && ordered[index-1] == number {
			return LegalEntityProfileContent{}, fmt.Errorf(
				"%w: tax registration number %s %s appears twice", ErrInvalidLegalEntityProfile, number.typeCode, number.number)
		}
	}
	content := LegalEntityProfileContent{address: address, taxNumbers: ordered}
	if invoicing != nil {
		if !invoicing.title.valid() {
			return LegalEntityProfileContent{}, fmt.Errorf("%w: invoicing details need an invoice title", ErrInvalidLegalEntityProfile)
		}
		content.invoicing = *invoicing
		content.hasInvoicing = true
	}
	content.contacts = make([]LegalEntityContact, len(contacts))
	for index, contact := range contacts {
		if strings.TrimSpace(contact.name) == "" {
			return LegalEntityProfileContent{}, fmt.Errorf("%w: contact needs a name", ErrInvalidLegalEntityProfile)
		}
		content.contacts[index] = contact
	}
	return content, nil
}

func (content LegalEntityProfileContent) Address() RegisteredAddress {
	return content.address
}

// TaxNumbers 交回一份拷贝。
func (content LegalEntityProfileContent) TaxNumbers() []TaxRegistrationNumber {
	numbers := make([]TaxRegistrationNumber, len(content.taxNumbers))
	copy(numbers, content.taxNumbers)
	return numbers
}

// Invoicing 交回开票资料；第二个返回值为假即这笔修订没带开票资料。
func (content LegalEntityProfileContent) Invoicing() (InvoicingDetails, bool) {
	return content.invoicing, content.hasInvoicing
}

// Contacts 交回一份拷贝。
func (content LegalEntityProfileContent) Contacts() []LegalEntityContact {
	contacts := make([]LegalEntityContact, len(content.contacts))
	copy(contacts, content.contacts)
	return contacts
}

func (content LegalEntityProfileContent) valid() bool {
	return content.address.valid()
}

// LegalEntityProfileReference 是一笔法人资料修订的引用：租户 + 责任法人 + 修订号。开立方把它固定在单据上
// （ADR-0145 决定五）——此后资料再改，包括追溯生效的修订，单据仍指向这一笔。
type LegalEntityProfileReference struct {
	tenant   TenantID
	entity   LegalEntityReference
	revision int
}

func (reference LegalEntityProfileReference) Tenant() TenantID {
	return reference.tenant
}

func (reference LegalEntityProfileReference) LegalEntity() LegalEntityReference {
	return reference.entity
}

func (reference LegalEntityProfileReference) Revision() int {
	return reference.revision
}

// LegalEntityProfileRevision 是法人资料修订链上的一笔（ADR-0145 决定三；CONTEXT Lifecycles「法人资料」）：
// 租户 + 责任法人 + 修订号。修订从 1 起连续递增、不可覆盖；每笔带登记依据与生效时点，可以登记未来生效的
// 修订，也可以登记追溯生效的修订。哪一笔在某个时点有效由 ResolveLegalEntityProfile 回答，不存状态格。
type LegalEntityProfileRevision struct {
	tenant        TenantID
	entity        LegalEntityReference
	revision      int
	basis         LegalEntityProfileBasisReference
	effectiveFrom time.Time
	content       LegalEntityProfileContent
}

func NewLegalEntityProfileRevision(
	tenant TenantID,
	entity LegalEntityReference,
	revision int,
	basis LegalEntityProfileBasisReference,
	effectiveFrom time.Time,
	content LegalEntityProfileContent,
) (LegalEntityProfileRevision, error) {
	switch {
	case !tenant.valid() || !entity.valid():
		return LegalEntityProfileRevision{}, fmt.Errorf("%w: tenant and legal entity are required", ErrInvalidLegalEntityProfile)
	case revision < 1:
		return LegalEntityProfileRevision{}, fmt.Errorf("%w: revision starts at 1, got %d", ErrInvalidLegalEntityProfile, revision)
	case !basis.valid():
		return LegalEntityProfileRevision{}, fmt.Errorf("%w: basis reference is required", ErrInvalidLegalEntityProfile)
	case effectiveFrom.IsZero():
		return LegalEntityProfileRevision{}, fmt.Errorf("%w: effective-from is required", ErrInvalidLegalEntityProfile)
	case !content.valid():
		return LegalEntityProfileRevision{}, fmt.Errorf("%w: content is incomplete", ErrInvalidLegalEntityProfile)
	}
	return LegalEntityProfileRevision{
		tenant:        tenant,
		entity:        entity,
		revision:      revision,
		basis:         basis,
		effectiveFrom: effectiveFrom.UTC(),
		content:       content,
	}, nil
}

func (revision LegalEntityProfileRevision) Tenant() TenantID {
	return revision.tenant
}

func (revision LegalEntityProfileRevision) LegalEntity() LegalEntityReference {
	return revision.entity
}

func (revision LegalEntityProfileRevision) Revision() int {
	return revision.revision
}

func (revision LegalEntityProfileRevision) Basis() LegalEntityProfileBasisReference {
	return revision.basis
}

func (revision LegalEntityProfileRevision) EffectiveFrom() time.Time {
	return revision.effectiveFrom
}

func (revision LegalEntityProfileRevision) Content() LegalEntityProfileContent {
	return revision.content
}

func (revision LegalEntityProfileRevision) Reference() LegalEntityProfileReference {
	return LegalEntityProfileReference{tenant: revision.tenant, entity: revision.entity, revision: revision.revision}
}
