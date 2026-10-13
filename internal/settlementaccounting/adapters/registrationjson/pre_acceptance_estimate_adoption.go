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

// PreAcceptanceEstimateAdoptionFromJSON 译装一行受理前估价采用（ADR-0179 决定五）。四格都没有缺席这一态；形态不在封闭词表内
// 由领域门拒——不默认成唯一那一种，没写就是没写。
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
	scope, err := domain.NewEstimatePricingScope(document.PricingScopeID)
	if err != nil {
		return none, fmt.Errorf("pricingScopeId：%w", err)
	}
	adoption, err := domain.NewPreAcceptanceEstimateAdoption(account, form, scope)
	if err != nil {
		return none, err
	}
	return application.RegisterPreAcceptanceEstimateAdoptionCommand{Tenant: tenant, Adoption: adoption}, nil
}
