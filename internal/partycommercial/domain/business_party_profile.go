package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var ErrInvalidBusinessPartyProfile = errors.New("party commercial: invalid business party profile revision")

// BusinessPartyProfileBasisReference 指向一笔参与方资料修订所依据的东西。判据同 LegalEntityProfileBasisReference，但两本
// 资料册各引各的主体，引用不共用一个类型。
type BusinessPartyProfileBasisReference struct{ requiredValue }

func NewBusinessPartyProfileBasisReference(value string) (BusinessPartyProfileBasisReference, error) {
	required, err := newRequiredValue("business party profile basis reference", value)
	return BusinessPartyProfileBasisReference{required}, err
}

// LocalizedPartyName 是往来单位的一个其他语言名称（ADR-0369 决定一）：Language 是 BCP 47 语言标签，Name 是该语言的名称。
// 身份上的名称照旧是那一个；这里登的是英文名等对外单据与海外往来要用的写法。字段导出带 JSON 标签，照 LocalizedInvoiceTitle：
// 在线口与快照原样搬它。
type LocalizedPartyName struct {
	Language string `json:"language"`
	Name     string `json:"name"`
}

// canonicalLocalizedNames 复核其他语言名称：语言标签按 BCP 47 的大体形状判、名称不得空白，同一修订内语言不重复（不分大小写
// 比对，原样保存），照录入次序保存。
func canonicalLocalizedNames(names []LocalizedPartyName) ([]LocalizedPartyName, error) {
	if len(names) == 0 {
		return nil, nil
	}
	result := make([]LocalizedPartyName, 0, len(names))
	for _, name := range names {
		tag, err := newLanguageTag(name.Language)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(name.Name) == "" {
			return nil, fmt.Errorf("%w: name for %s is blank", ErrInvalidBusinessPartyProfile, tag)
		}
		for _, previous := range result {
			if strings.EqualFold(previous.Language, tag) {
				return nil, fmt.Errorf("%w: duplicate name language %s", ErrInvalidBusinessPartyProfile, tag)
			}
		}
		result = append(result, LocalizedPartyName{Language: tag, Name: name.Name})
	}
	return result, nil
}

// CheckLocalizedPartyNames 是其他语言名称的形状门，给接入口在翻译时用：形状不对是改载荷才会好的错，接入口答形状错，不等用例。
func CheckLocalizedPartyNames(names []LocalizedPartyName) error {
	_, err := canonicalLocalizedNames(names)
	return err
}

// BusinessPartyProfileParts 是一笔参与方资料修订正文的各项，交给 NewBusinessPartyProfileContent 复核。指针为空、切片为空即
// 这一版不带那一项。
type BusinessPartyProfileParts struct {
	Names                 []LocalizedPartyName
	RegisteredAddress     *RegisteredAddress
	OfficeAddress         *RegisteredAddress
	CorrespondenceAddress *RegisteredAddress
	TaxNumbers            []TaxRegistrationNumber
	Invoicing             *InvoicingDetails
	Contacts              []LegalEntityContact
	BankAccounts          []LegalEntityBankAccount
}

// BusinessPartyProfileContent 是一笔参与方资料修订的正文（ADR-0369 决定一）。各项都可缺，一项都不带也成立——那表示这一版
// 不再登这些资料。各项的形状与校验同法人资料，复核走与法人资料共用的函数；不登法定代表人。
type BusinessPartyProfileContent struct {
	parts BusinessPartyProfileParts
}

// NewBusinessPartyProfileContent 复核各项。共用的校验交回的错误带法人资料的哨兵，这里另包一层参与方资料的哨兵：两者都认得出，
// 而原因读起来先说的是往来单位资料。
func NewBusinessPartyProfileContent(parts BusinessPartyProfileParts) (BusinessPartyProfileContent, error) {
	content, err := newBusinessPartyProfileContent(parts)
	if err != nil && !errors.Is(err, ErrInvalidBusinessPartyProfile) {
		return BusinessPartyProfileContent{}, fmt.Errorf("%w: %w", ErrInvalidBusinessPartyProfile, err)
	}
	return content, err
}

func newBusinessPartyProfileContent(parts BusinessPartyProfileParts) (BusinessPartyProfileContent, error) {
	var content BusinessPartyProfileContent
	var err error
	if content.parts.Names, err = canonicalLocalizedNames(parts.Names); err != nil {
		return BusinessPartyProfileContent{}, err
	}
	for _, address := range []struct {
		value *RegisteredAddress
		slot  **RegisteredAddress
	}{
		{parts.RegisteredAddress, &content.parts.RegisteredAddress},
		{parts.OfficeAddress, &content.parts.OfficeAddress},
		{parts.CorrespondenceAddress, &content.parts.CorrespondenceAddress},
	} {
		if address.value == nil {
			continue
		}
		if !address.value.valid() {
			return BusinessPartyProfileContent{}, fmt.Errorf("%w: address needs a country or region and at least one line", ErrInvalidLegalEntityProfile)
		}
		copied := *address.value
		*address.slot = &copied
	}
	if content.parts.TaxNumbers, err = canonicalTaxRegistrationNumbers(parts.TaxNumbers); err != nil {
		return BusinessPartyProfileContent{}, err
	}
	if len(content.parts.TaxNumbers) == 0 {
		content.parts.TaxNumbers = nil
	}
	if parts.Invoicing != nil {
		checked, err := checkedInvoicingDetails(*parts.Invoicing)
		if err != nil {
			return BusinessPartyProfileContent{}, err
		}
		content.parts.Invoicing = &checked
	}
	if content.parts.Contacts, err = checkedProfileContacts(parts.Contacts); err != nil {
		return BusinessPartyProfileContent{}, err
	}
	if len(content.parts.Contacts) == 0 {
		content.parts.Contacts = nil
	}
	if content.parts.BankAccounts, err = NewLegalEntityBankAccounts(parts.BankAccounts); err != nil {
		return BusinessPartyProfileContent{}, err
	}
	return content, nil
}

// Empty 回答这一版是不是一项都没带。我方公司的往来单位不留参与方资料（ADR-0369 决定二）：要把一家已登资料的往来单位
// 钉成公司主体，先登一版空资料把它撤下。
func (content BusinessPartyProfileContent) Empty() bool {
	parts := content.parts
	return len(parts.Names) == 0 && parts.RegisteredAddress == nil && parts.OfficeAddress == nil &&
		parts.CorrespondenceAddress == nil && len(parts.TaxNumbers) == 0 && parts.Invoicing == nil &&
		len(parts.Contacts) == 0 && len(parts.BankAccounts) == 0
}

// Parts 交回各项的一份拷贝，调用方改它不会改到正文。
func (content BusinessPartyProfileContent) Parts() BusinessPartyProfileParts {
	parts := content.parts
	parts.Names = slices.Clone(parts.Names)
	for _, slot := range []**RegisteredAddress{&parts.RegisteredAddress, &parts.OfficeAddress, &parts.CorrespondenceAddress} {
		if *slot != nil {
			copied := **slot
			copied.lines = slices.Clone(copied.lines)
			*slot = &copied
		}
	}
	parts.TaxNumbers = slices.Clone(parts.TaxNumbers)
	if parts.Invoicing != nil {
		copied := *parts.Invoicing
		copied.localizedTitles = slices.Clone(copied.localizedTitles)
		parts.Invoicing = &copied
	}
	parts.Contacts = slices.Clone(parts.Contacts)
	parts.BankAccounts = slices.Clone(parts.BankAccounts)
	return parts
}

// BusinessPartyProfileRevision 是参与方资料修订链上的一笔：租户 + 往来单位 + 修订号，结构判据同 LegalEntityProfileRevision。
type BusinessPartyProfileRevision struct {
	tenant        TenantID
	party         PartyID
	revision      int
	basis         BusinessPartyProfileBasisReference
	effectiveFrom time.Time
	content       BusinessPartyProfileContent
}

func NewBusinessPartyProfileRevision(
	tenant TenantID,
	party PartyID,
	revision int,
	basis BusinessPartyProfileBasisReference,
	effectiveFrom time.Time,
	content BusinessPartyProfileContent,
) (BusinessPartyProfileRevision, error) {
	switch {
	case !tenant.valid() || !party.valid():
		return BusinessPartyProfileRevision{}, fmt.Errorf("%w: tenant and business party are required", ErrInvalidBusinessPartyProfile)
	case revision < 1:
		return BusinessPartyProfileRevision{}, fmt.Errorf("%w: revision starts at 1, got %d", ErrInvalidBusinessPartyProfile, revision)
	case !basis.valid():
		return BusinessPartyProfileRevision{}, fmt.Errorf("%w: basis reference is required", ErrInvalidBusinessPartyProfile)
	case effectiveFrom.IsZero():
		return BusinessPartyProfileRevision{}, fmt.Errorf("%w: effective-from is required", ErrInvalidBusinessPartyProfile)
	}
	return BusinessPartyProfileRevision{
		tenant:        tenant,
		party:         party,
		revision:      revision,
		basis:         basis,
		effectiveFrom: effectiveFrom.UTC(),
		content:       content,
	}, nil
}

func (revision BusinessPartyProfileRevision) Tenant() TenantID { return revision.tenant }

func (revision BusinessPartyProfileRevision) Party() PartyID { return revision.party }

func (revision BusinessPartyProfileRevision) Revision() int { return revision.revision }

func (revision BusinessPartyProfileRevision) Basis() BusinessPartyProfileBasisReference {
	return revision.basis
}

func (revision BusinessPartyProfileRevision) EffectiveFrom() time.Time { return revision.effectiveFrom }

func (revision BusinessPartyProfileRevision) Content() BusinessPartyProfileContent {
	return revision.content
}

// EffectiveBusinessPartyProfileRevision 回答某一时点有效的是哪一笔（ADR-0369 决定一）：生效时点不晚于它的修订里修订号最大的
// 那一笔，判据与 ResolveLegalEntityProfile 选修订的那一段一致。没有一笔生效答 false。同一修订号出现两次是读侧没按主键取数，
// 答错误而不是随取数次序给一个答案。
func EffectiveBusinessPartyProfileRevision(chain []BusinessPartyProfileRevision, at time.Time) (int, bool, error) {
	if at.IsZero() {
		return 0, false, fmt.Errorf("%w: resolution time is required", ErrInvalidBusinessPartyProfile)
	}
	seen := make(map[int]bool, len(chain))
	effective, found := 0, false
	for _, revision := range chain {
		if seen[revision.revision] {
			return 0, false, fmt.Errorf("%w: revision %d appears twice", ErrInvalidBusinessPartyProfile, revision.revision)
		}
		seen[revision.revision] = true
		if revision.effectiveFrom.After(at) {
			continue
		}
		if !found || revision.revision > effective {
			effective, found = revision.revision, true
		}
	}
	return effective, found, nil
}
