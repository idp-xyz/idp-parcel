package domain

import (
	"fmt"
	"slices"
	"strings"
)

// 联系用途只描述资料维护职责，不授予系统权限或签约权。
type ContactPurpose string

const (
	ContactGeneral    ContactPurpose = "GENERAL"
	ContactBusiness   ContactPurpose = "BUSINESS"
	ContactOperations ContactPurpose = "OPERATIONS"
	ContactFinance    ContactPurpose = "FINANCE"
	ContactLegal      ContactPurpose = "LEGAL"
)

func contactPurposes(values []ContactPurpose) ([]ContactPurpose, error) {
	result := slices.Clone(values)
	slices.Sort(result)
	for i, value := range result {
		switch value {
		case ContactGeneral, ContactBusiness, ContactOperations, ContactFinance, ContactLegal:
		default:
			return nil, fmt.Errorf("%w: unknown contact purpose %q", ErrInvalidLegalEntityProfile, value)
		}
		if i > 0 && result[i-1] == value {
			return nil, fmt.Errorf("%w: duplicate contact purpose %q", ErrInvalidLegalEntityProfile, value)
		}
	}
	return result, nil
}

type LegalEntityContactDetails struct {
	Department     string
	Position       string
	Purposes       []ContactPurpose
	PrimaryFor     []ContactPurpose
	SocialAccounts []ContactSocialAccount
}

type ContactSocialAccount struct {
	Platform string `json:"platform"`
	Account  string `json:"account"`
}

func contactSocialAccounts(values []ContactSocialAccount) ([]ContactSocialAccount, error) {
	result := slices.Clone(values)
	for i := range result {
		result[i].Platform = strings.TrimSpace(result[i].Platform)
		result[i].Account = strings.TrimSpace(result[i].Account)
		if result[i].Platform == "" || result[i].Account == "" {
			return nil, fmt.Errorf("%w: social account needs a platform and an account", ErrInvalidLegalEntityProfile)
		}
		for _, previous := range result[:i] {
			if strings.EqualFold(previous.Platform, result[i].Platform) && previous.Account == result[i].Account {
				return nil, fmt.Errorf("%w: duplicate social account", ErrInvalidLegalEntityProfile)
			}
		}
	}
	return result, nil
}

func NewLegalEntityContactWithDetails(name, email, phone string, details LegalEntityContactDetails) (LegalEntityContact, error) {
	contact, err := NewLegalEntityContact(name, email, phone)
	if err != nil {
		return LegalEntityContact{}, err
	}
	details.Purposes, err = contactPurposes(details.Purposes)
	if err != nil {
		return LegalEntityContact{}, err
	}
	details.PrimaryFor, err = contactPurposes(details.PrimaryFor)
	if err != nil {
		return LegalEntityContact{}, err
	}
	for _, purpose := range details.PrimaryFor {
		if !slices.Contains(details.Purposes, purpose) {
			return LegalEntityContact{}, fmt.Errorf("%w: primary purpose must be one of the contact purposes", ErrInvalidLegalEntityProfile)
		}
	}
	details.SocialAccounts, err = contactSocialAccounts(details.SocialAccounts)
	if err != nil {
		return LegalEntityContact{}, err
	}
	contact.details = details
	return contact, nil
}

func (contact LegalEntityContact) Details() LegalEntityContactDetails {
	details := contact.details
	details.Purposes = slices.Clone(details.Purposes)
	details.PrimaryFor = slices.Clone(details.PrimaryFor)
	details.SocialAccounts = slices.Clone(details.SocialAccounts)
	return details
}

type LegalEntityRepresentative struct{ kind, name string }

func NewLegalEntityRepresentative(kind, name string) (LegalEntityRepresentative, error) {
	if (kind != "LEGAL_REPRESENTATIVE" && kind != "RESPONSIBLE_PERSON") || strings.TrimSpace(name) == "" {
		return LegalEntityRepresentative{}, fmt.Errorf("%w: representative needs a supported kind and a name", ErrInvalidLegalEntityProfile)
	}
	return LegalEntityRepresentative{kind: kind, name: name}, nil
}

func (representative LegalEntityRepresentative) Kind() string { return representative.kind }
func (representative LegalEntityRepresentative) Name() string { return representative.name }

// 缺席表示未配置；办公和通信地址不会借注册地址补齐。收款账户（ADR-0359 决定三）也在这里：它与代表人、分类地址一样
// 是随完整修订保存的可选明细，缺席即这一版不带收款账户，旧修订不回填。
type LegalEntityProfileSupplement struct {
	Representative        *LegalEntityRepresentative
	OfficeAddress         *RegisteredAddress
	CorrespondenceAddress *RegisteredAddress
	BankAccounts          []LegalEntityBankAccount
}

func (supplement LegalEntityProfileSupplement) clone() LegalEntityProfileSupplement {
	if supplement.Representative != nil {
		value := *supplement.Representative
		supplement.Representative = &value
	}
	if supplement.OfficeAddress != nil {
		value := *supplement.OfficeAddress
		supplement.OfficeAddress = &value
	}
	if supplement.CorrespondenceAddress != nil {
		value := *supplement.CorrespondenceAddress
		supplement.CorrespondenceAddress = &value
	}
	supplement.BankAccounts = slices.Clone(supplement.BankAccounts)
	return supplement
}

func (content LegalEntityProfileContent) Supplement() LegalEntityProfileSupplement {
	return content.supplement.clone()
}

func NewLegalEntityProfileContentWithSupplement(address RegisteredAddress, taxes []TaxRegistrationNumber, invoicing *InvoicingDetails, contacts []LegalEntityContact, supplement LegalEntityProfileSupplement) (LegalEntityProfileContent, error) {
	content, err := NewLegalEntityProfileContent(address, taxes, invoicing, contacts)
	if err != nil {
		return LegalEntityProfileContent{}, err
	}
	if representative := supplement.Representative; representative != nil {
		if _, err := NewLegalEntityRepresentative(representative.Kind(), representative.Name()); err != nil {
			return LegalEntityProfileContent{}, err
		}
	}
	for _, address := range []*RegisteredAddress{supplement.OfficeAddress, supplement.CorrespondenceAddress} {
		if address != nil && !address.valid() {
			return LegalEntityProfileContent{}, fmt.Errorf("%w: supplementary address is invalid", ErrInvalidLegalEntityProfile)
		}
	}
	accounts, err := NewLegalEntityBankAccounts(supplement.BankAccounts)
	if err != nil {
		return LegalEntityProfileContent{}, err
	}
	content.supplement = supplement.clone()
	content.supplement.BankAccounts = accounts
	return content, nil
}
