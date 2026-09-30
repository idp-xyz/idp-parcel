package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type supplierAuditAuthorityDocument struct {
	TenantID      string `json:"tenantId"`
	SupplierID    string `json:"supplierId"`
	LegalEntityID string `json:"legalEntityId"`
	AuditorID     string `json:"auditorId"`
}

type supplierPayableAccountDocument struct {
	TenantID      string `json:"tenantId"`
	SupplierID    string `json:"supplierId"`
	LegalEntityID string `json:"legalEntityId"`
	Currency      string `json:"currency"`
	AccountID     string `json:"accountId"`
}

type claimAmountRuleDocument struct {
	TenantID                   string `json:"tenantId"`
	ResponsibilityConclusionID string `json:"responsibilityConclusionId"`
	RuleVersion                string `json:"ruleVersion"`
}

type chargeConfirmationFactDocument struct {
	TenantID             string `json:"tenantId"`
	ChargeID             string `json:"chargeId"`
	ResponsibleEntity    string `json:"responsibleEntity"`
	CounterpartyID       string `json:"counterpartyId"`
	Direction            string `json:"direction"`
	SettlementAccountID  string `json:"settlementAccountId"`
	ContractBasis        string `json:"contractBasis"`
	PrimaryChargingScope string `json:"primaryChargingScope"`
	SourceFactRef        string `json:"sourceFactRef"`
}

// SupplierAuditAuthorityFromJSON 译装一条审核授权。册上没有这一组时读口答未配置，这里不填默认授权人。
func SupplierAuditAuthorityFromJSON(raw []byte) (domain.TenantID, domain.SupplierAuditAuthorityRegistration, error) {
	var document supplierAuditAuthorityDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.SupplierAuditAuthorityRegistration{}, fmt.Errorf("审核授权登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.SupplierAuditAuthorityRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	registration, err := domain.NewSupplierAuditAuthorityRegistration(
		catalogueRef(domain.NewSupplierPartyReference, document.SupplierID),
		catalogueRef(domain.NewLegalEntityReference, document.LegalEntityID),
		catalogueRef(domain.NewAuditorReference, document.AuditorID),
	)
	if err != nil {
		return domain.TenantID{}, domain.SupplierAuditAuthorityRegistration{}, err
	}
	return tenant, registration, nil
}

// SupplierPayableAccountFromJSON 译装一条供应商应付账户查问。账户标识必须是已登记的结算账户，本函数不建账户。
func SupplierPayableAccountFromJSON(raw []byte) (domain.TenantID, domain.SupplierPayableAccountRegistration, error) {
	var document supplierPayableAccountDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.SupplierPayableAccountRegistration{}, fmt.Errorf("供应商应付账户登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.SupplierPayableAccountRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	registration, err := domain.NewSupplierPayableAccountRegistration(
		catalogueRef(domain.NewSupplierPartyReference, document.SupplierID),
		catalogueRef(domain.NewLegalEntityReference, document.LegalEntityID),
		catalogueRef(domain.NewCurrencyCode, document.Currency),
		catalogueRef(domain.NewSettlementAccountID, document.AccountID),
	)
	if err != nil {
		return domain.TenantID{}, domain.SupplierPayableAccountRegistration{}, err
	}
	return tenant, registration, nil
}

// ClaimAmountRuleFromJSON 译装一条金额规则版本指向。不计算限额、比例或免赔。
func ClaimAmountRuleFromJSON(raw []byte) (domain.TenantID, domain.ClaimAmountRuleRegistration, error) {
	var document claimAmountRuleDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.ClaimAmountRuleRegistration{}, fmt.Errorf("金额规则登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.ClaimAmountRuleRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	registration, err := domain.NewClaimAmountRuleRegistration(
		catalogueRef(domain.NewResponsibilityConclusionReference, document.ResponsibilityConclusionID),
		catalogueRef(domain.NewAmountRuleVersionReference, document.RuleVersion),
	)
	if err != nil {
		return domain.TenantID{}, domain.ClaimAmountRuleRegistration{}, err
	}
	return tenant, registration, nil
}

// ChargeConfirmationFactsFromJSON 译装确认前要读到的七项。确认结果仍写在费用行上。
func ChargeConfirmationFactsFromJSON(raw []byte) (domain.TenantID, domain.ChargeConfirmationFactRegistration, error) {
	var document chargeConfirmationFactDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.ChargeConfirmationFactRegistration{}, fmt.Errorf("确认事实登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.ChargeConfirmationFactRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	direction, err := domain.ChargeDirectionFromName(document.Direction)
	if err != nil {
		return domain.TenantID{}, domain.ChargeConfirmationFactRegistration{}, fmt.Errorf("direction：%w", err)
	}
	facts := domain.ConfirmedChargeFacts{
		ResponsibleEntity:    catalogueRef(domain.NewLegalEntityReference, document.ResponsibleEntity),
		Counterparty:         catalogueRef(domain.NewSettlementCounterpartyReference, document.CounterpartyID),
		Direction:            direction,
		SettlementAccount:    catalogueRef(domain.NewSettlementAccountID, document.SettlementAccountID),
		ContractBasis:        catalogueRef(domain.NewContractBasisReference, document.ContractBasis),
		PrimaryChargingScope: catalogueRef(domain.NewChargingScopeReference, document.PrimaryChargingScope),
		SourceFact:           catalogueRef(domain.NewSourceFactReference, document.SourceFactRef),
	}
	registration, err := domain.NewChargeConfirmationFactRegistration(
		catalogueRef(domain.NewCustomerChargeID, document.ChargeID), facts)
	if err != nil {
		return domain.TenantID{}, domain.ChargeConfirmationFactRegistration{}, err
	}
	return tenant, registration, nil
}

func catalogueRef[T any](parse func(string) (T, error), value string) T {
	parsed, _ := parse(value)
	return parsed
}
