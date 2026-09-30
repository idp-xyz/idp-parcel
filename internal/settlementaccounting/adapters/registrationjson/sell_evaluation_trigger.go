package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type sellEvaluationTriggerDocument struct {
	TenantID         string `json:"tenantId"`
	OccurrenceReason string `json:"occurrenceReason"`
	Moment           string `json:"moment"`
}

// SellEvaluationTriggerFromJSON 译装一条 SELL 触发选用。不发起 BUY 评价请求。
func SellEvaluationTriggerFromJSON(raw []byte) (domain.TenantID, domain.SellEvaluationTriggerRegistration, error) {
	var document sellEvaluationTriggerDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.SellEvaluationTriggerRegistration{}, fmt.Errorf("SELL 评价触发登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.SellEvaluationTriggerRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	reason, err := domain.NewOccurrenceReasonReference(document.OccurrenceReason)
	if err != nil {
		return domain.TenantID{}, domain.SellEvaluationTriggerRegistration{}, fmt.Errorf("occurrenceReason：%w", err)
	}
	moment, err := domain.SellEvaluationTriggerMomentFromName(document.Moment)
	if err != nil {
		return domain.TenantID{}, domain.SellEvaluationTriggerRegistration{}, fmt.Errorf("moment：%w", err)
	}
	registration, err := domain.NewSellEvaluationTriggerRegistration(reason, moment)
	if err != nil {
		return domain.TenantID{}, domain.SellEvaluationTriggerRegistration{}, err
	}
	return tenant, registration, nil
}
