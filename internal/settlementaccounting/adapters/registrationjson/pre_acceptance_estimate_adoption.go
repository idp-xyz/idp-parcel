package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type preAcceptanceEstimateAdoptionDocument struct {
	TenantID       string `json:"tenantId"`
	AccountID      string `json:"accountId"`
	Form           string `json:"form"`
	PricingScopeID string `json:"pricingScopeId"`
}

// PreAcceptanceEstimateAdoptionFromJSON 译装一行受理前估价采用（ADR-0179 决定五、ADR-0380 决定七）。形态必填，没写就是没写，
// 不默认成唯一那一种。计价范围可缺：空串是不带范围的行，空白仍拒。
func PreAcceptanceEstimateAdoptionFromJSON(raw []byte) (application.RegisterPreAcceptanceEstimateAdoptionCommand, error) {
	none := application.RegisterPreAcceptanceEstimateAdoptionCommand{}
	var document preAcceptanceEstimateAdoptionDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("受理前估价采用登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, fmt.Errorf("tenantId：%w", err)
	}
	account, err := domain.NewSettlementAccountID(document.AccountID)
	if err != nil {
		return none, fmt.Errorf("accountId：%w", err)
	}
	form, err := domain.PreAcceptanceEstimateFormFromName(document.Form)
	if err != nil {
		return none, fmt.Errorf("form：%w", err)
	}
	scope, err := optionalPricingScope(document.PricingScopeID)
	if err != nil {
		return none, fmt.Errorf("pricingScopeId：%w", err)
	}
	adoption, err := domain.NewPreAcceptanceEstimateAdoption(account, form, scope)
	if err != nil {
		return none, err
	}
	return application.RegisterPreAcceptanceEstimateAdoptionCommand{Tenant: tenant, Adoption: adoption}, nil
}
