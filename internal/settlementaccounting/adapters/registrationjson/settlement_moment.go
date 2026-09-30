package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type settlementMomentDocument struct {
	TenantID string `json:"tenantId"`
	Moment   string `json:"moment"`
}

// SettlementMomentFromJSON 译装一条确认或截单的采用。不填钟点，不填账期。
func SettlementMomentFromJSON(raw []byte) (domain.TenantID, domain.SettlementMoment, error) {
	var document settlementMomentDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.SettlementMomentInvalid, fmt.Errorf("结算触发登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.SettlementMomentInvalid, fmt.Errorf("tenantId：%w", err)
	}
	moment, err := domain.SettlementMomentFromName(document.Moment)
	if err != nil {
		return domain.TenantID{}, domain.SettlementMomentInvalid, fmt.Errorf("moment：%w", err)
	}
	return tenant, moment, nil
}
