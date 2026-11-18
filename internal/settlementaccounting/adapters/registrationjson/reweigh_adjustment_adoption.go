package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type reweighAdjustmentAdoptionDocument struct {
	TenantID  string `json:"tenantId"`
	AccountID string `json:"accountId"`
	Basis     string `json:"basis"`
}

// ReweighAdjustmentAdoptionFromJSON 译装一户对「实测差异自动调整」的采用。
func ReweighAdjustmentAdoptionFromJSON(raw []byte) (domain.TenantID, domain.SettlementAccountID, string, error) {
	var document reweighAdjustmentAdoptionDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.SettlementAccountID{}, "", fmt.Errorf("实测差异自动调整的采用输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.SettlementAccountID{}, "", fmt.Errorf("tenantId：%w", err)
	}
	account, err := domain.NewSettlementAccountID(document.AccountID)
	if err != nil {
		return domain.TenantID{}, domain.SettlementAccountID{}, "", fmt.Errorf("accountId：%w", err)
	}
	if document.Basis == "" {
		return domain.TenantID{}, domain.SettlementAccountID{}, "", fmt.Errorf("basis：缺依据")
	}
	return tenant, account, document.Basis, nil
}
