package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type sellBillingMomentAdoptionDocument struct {
	TenantID       string `json:"tenantId"`
	AccountID      string `json:"accountId"`
	BillingMoment  string `json:"billingMoment"`
	PricingScopeID string `json:"pricingScopeId"`
}

// SellBillingMomentAdoptionFromJSON 译装一行 SELL 计费时点采用（ADR-0190 决定三）。各格都没有缺席这一态；时点不在封闭
// 词表内在这里就拒并指名字段——不默认成任何一种，没写就是没写。
func SellBillingMomentAdoptionFromJSON(raw []byte) (application.RegisterSellBillingMomentAdoptionCommand, error) {
	none := application.RegisterSellBillingMomentAdoptionCommand{}
	var document sellBillingMomentAdoptionDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("SELL 计费时点采用登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, fmt.Errorf("tenantId：%w", err)
	}
	account, err := domain.NewSettlementAccountID(document.AccountID)
	if err != nil {
		return none, fmt.Errorf("accountId：%w", err)
	}
	moment, err := domain.ParcelBillingMomentFromName(document.BillingMoment)
	if err != nil {
		return none, fmt.Errorf("billingMoment：%w", err)
	}
	scope, err := domain.NewEstimatePricingScope(document.PricingScopeID)
	if err != nil {
		return none, fmt.Errorf("pricingScopeId：%w", err)
	}
	adoption, err := domain.NewSellBillingMomentAdoption(account, moment, scope)
	if err != nil {
		return none, err
	}
	return application.RegisterSellBillingMomentAdoptionCommand{Tenant: tenant, Adoption: adoption}, nil
}

type feeItemMappingDocument struct {
	TenantID          string `json:"tenantId"`
	PricingChargeCode string `json:"pricingChargeCode"`
	FeeItem           string `json:"feeItem"`
	BasisRef          string `json:"basisRef"`
}

// FeeItemMappingFromJSON 译装一行费用项目映射（ADR-0190 决定五）。代码键名带 pricing 前缀：本上下文另有自己的费用
// （客户费用）标识，裸写 chargeCode 会让登记人把两种代码填混。
func FeeItemMappingFromJSON(raw []byte) (domain.TenantID, domain.FeeItemMapping, error) {
	var document feeItemMappingDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.FeeItemMapping{}, fmt.Errorf("费用项目映射登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.FeeItemMapping{}, fmt.Errorf("tenantId：%w", err)
	}
	code, err := domain.NewPricingChargeCode(document.PricingChargeCode)
	if err != nil {
		return domain.TenantID{}, domain.FeeItemMapping{}, fmt.Errorf("pricingChargeCode：%w", err)
	}
	feeItem, err := domain.NewFeeItemReference(document.FeeItem)
	if err != nil {
		return domain.TenantID{}, domain.FeeItemMapping{}, fmt.Errorf("feeItem：%w", err)
	}
	basis, err := domain.NewFeeItemMappingBasisReference(document.BasisRef)
	if err != nil {
		return domain.TenantID{}, domain.FeeItemMapping{}, fmt.Errorf("basisRef：%w", err)
	}
	mapping, err := domain.NewFeeItemMapping(code, feeItem, basis)
	if err != nil {
		return domain.TenantID{}, domain.FeeItemMapping{}, err
	}
	return tenant, mapping, nil
}
