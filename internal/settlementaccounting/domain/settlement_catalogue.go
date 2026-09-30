package domain

import (
	"errors"
	"fmt"
)

// ErrCatalogueTargetMissing 表示登记指向的结算账户还不在账户册上。本册不代为建账户。
var ErrCatalogueTargetMissing = errors.New("settlement accounting: catalogue target is not registered")

// SupplierAuditAuthorityRegistration 是一组供应商/责任法人的审核授权。册上没有这一组，审核停在未决。
type SupplierAuditAuthorityRegistration struct {
	supplier    SupplierPartyReference
	legalEntity LegalEntityReference
	auditor     AuditorReference
}

func NewSupplierAuditAuthorityRegistration(
	supplier SupplierPartyReference,
	legalEntity LegalEntityReference,
	auditor AuditorReference,
) (SupplierAuditAuthorityRegistration, error) {
	if !supplier.valid() || !legalEntity.valid() || !auditor.valid() {
		return SupplierAuditAuthorityRegistration{}, fmt.Errorf("%w: supplier audit authority", ErrBlankValue)
	}
	return SupplierAuditAuthorityRegistration{supplier: supplier, legalEntity: legalEntity, auditor: auditor}, nil
}

func (registration SupplierAuditAuthorityRegistration) Supplier() SupplierPartyReference {
	return registration.supplier
}
func (registration SupplierAuditAuthorityRegistration) LegalEntity() LegalEntityReference {
	return registration.legalEntity
}
func (registration SupplierAuditAuthorityRegistration) Auditor() AuditorReference {
	return registration.auditor
}

func (registration SupplierAuditAuthorityRegistration) SameRegistration(other SupplierAuditAuthorityRegistration) bool {
	return registration.supplier.String() == other.supplier.String() &&
		registration.legalEntity.String() == other.legalEntity.String() &&
		registration.auditor.String() == other.auditor.String()
}

// SupplierPayableAccountRegistration 是「供应商/责任法人/币种 → 已有结算账户」的一问。
// 账户五格在结算账户登记册上，这里只保存查问与账户标识。
type SupplierPayableAccountRegistration struct {
	supplier    SupplierPartyReference
	legalEntity LegalEntityReference
	currency    CurrencyCode
	account     SettlementAccountID
}

func NewSupplierPayableAccountRegistration(
	supplier SupplierPartyReference,
	legalEntity LegalEntityReference,
	currency CurrencyCode,
	account SettlementAccountID,
) (SupplierPayableAccountRegistration, error) {
	if !supplier.valid() || !legalEntity.valid() || !currency.valid() || !account.valid() {
		return SupplierPayableAccountRegistration{}, fmt.Errorf("%w: supplier payable account", ErrBlankValue)
	}
	return SupplierPayableAccountRegistration{
		supplier: supplier, legalEntity: legalEntity, currency: currency, account: account,
	}, nil
}

func (registration SupplierPayableAccountRegistration) Supplier() SupplierPartyReference {
	return registration.supplier
}
func (registration SupplierPayableAccountRegistration) LegalEntity() LegalEntityReference {
	return registration.legalEntity
}
func (registration SupplierPayableAccountRegistration) Currency() CurrencyCode {
	return registration.currency
}
func (registration SupplierPayableAccountRegistration) Account() SettlementAccountID {
	return registration.account
}

func (registration SupplierPayableAccountRegistration) SameRegistration(other SupplierPayableAccountRegistration) bool {
	return registration.supplier.String() == other.supplier.String() &&
		registration.legalEntity.String() == other.legalEntity.String() &&
		registration.currency.String() == other.currency.String() &&
		registration.account.String() == other.account.String()
}

// ClaimAmountRuleRegistration 把一条责任结论指到金额规则版本。怎样用该版本算出金额不在这一行。
type ClaimAmountRuleRegistration struct {
	responsibility ResponsibilityConclusionReference
	rule           AmountRuleVersionReference
}

func NewClaimAmountRuleRegistration(
	responsibility ResponsibilityConclusionReference,
	rule AmountRuleVersionReference,
) (ClaimAmountRuleRegistration, error) {
	if !responsibility.valid() || !rule.valid() {
		return ClaimAmountRuleRegistration{}, fmt.Errorf("%w: claim amount rule", ErrBlankValue)
	}
	return ClaimAmountRuleRegistration{responsibility: responsibility, rule: rule}, nil
}

func (registration ClaimAmountRuleRegistration) Responsibility() ResponsibilityConclusionReference {
	return registration.responsibility
}
func (registration ClaimAmountRuleRegistration) Rule() AmountRuleVersionReference {
	return registration.rule
}

func (registration ClaimAmountRuleRegistration) SameRegistration(other ClaimAmountRuleRegistration) bool {
	return registration.responsibility.String() == other.responsibility.String() &&
		registration.rule.String() == other.rule.String()
}

// ChargeConfirmationFactRegistration 是确认一笔费用之前要读到的七项。确认结果写在费用行上，不写回这一行。
type ChargeConfirmationFactRegistration struct {
	charge CustomerChargeID
	facts  ConfirmedChargeFacts
}

func NewChargeConfirmationFactRegistration(
	charge CustomerChargeID,
	facts ConfirmedChargeFacts,
) (ChargeConfirmationFactRegistration, error) {
	if !charge.valid() || !facts.complete() {
		return ChargeConfirmationFactRegistration{}, fmt.Errorf("%w: charge confirmation facts", ErrBlankValue)
	}
	return ChargeConfirmationFactRegistration{charge: charge, facts: facts}, nil
}

func (registration ChargeConfirmationFactRegistration) Charge() CustomerChargeID {
	return registration.charge
}
func (registration ChargeConfirmationFactRegistration) Facts() ConfirmedChargeFacts {
	return registration.facts
}

func (registration ChargeConfirmationFactRegistration) SameRegistration(other ChargeConfirmationFactRegistration) bool {
	return registration.charge.String() == other.charge.String() && sameConfirmedFacts(registration.facts, other.facts)
}

func sameConfirmedFacts(left, right ConfirmedChargeFacts) bool {
	return left.ResponsibleEntity.String() == right.ResponsibleEntity.String() &&
		left.Counterparty.String() == right.Counterparty.String() &&
		left.Direction == right.Direction &&
		left.SettlementAccount.String() == right.SettlementAccount.String() &&
		left.ContractBasis.String() == right.ContractBasis.String() &&
		left.PrimaryChargingScope.String() == right.PrimaryChargingScope.String() &&
		left.SourceFact.String() == right.SourceFact.String()
}
