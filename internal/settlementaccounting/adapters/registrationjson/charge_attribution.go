package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type chargeAttributionDocument struct {
	TenantID     string `json:"tenantId"`
	FeeItem      string `json:"feeItem"`
	Form         string `json:"form"`
	TimeZone     string `json:"timeZone"`
	CutoffMinute *int   `json:"cutoffMinute"`
}

// ChargeAttributionFromJSON 译装一条归属日判定。不填默认时区或截单时刻，也不把某个日期判成归属日。
func ChargeAttributionFromJSON(raw []byte) (domain.TenantID, domain.ChargeAttributionRegistration, error) {
	var document chargeAttributionDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, fmt.Errorf("归属日判定登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	feeItem, err := domain.NewFeeItemReference(document.FeeItem)
	if err != nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, fmt.Errorf("feeItem：%w", err)
	}
	form, err := domain.ChargeAttributionFormFromName(document.Form)
	if err != nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, fmt.Errorf("form：%w", err)
	}
	if document.CutoffMinute == nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, fmt.Errorf("cutoffMinute：缺席")
	}
	registration, err := domain.NewChargeAttributionRegistration(feeItem, form, document.TimeZone, *document.CutoffMinute)
	if err != nil {
		return domain.TenantID{}, domain.ChargeAttributionRegistration{}, err
	}
	return tenant, registration, nil
}
