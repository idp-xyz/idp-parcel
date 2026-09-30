package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type buyEvaluationTriggerDocument struct {
	TenantID         string `json:"tenantId"`
	OccurrenceReason string `json:"occurrenceReason"`
	Moment           string `json:"moment"`
}

// BuyEvaluationTriggerFromJSON 译装一条触发选用。不预填发生项原因，也不发起评价请求。
func BuyEvaluationTriggerFromJSON(raw []byte) (domain.TenantID, domain.BuyEvaluationTriggerRegistration, error) {
	var document buyEvaluationTriggerDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.BuyEvaluationTriggerRegistration{}, fmt.Errorf("评价请求触发登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.BuyEvaluationTriggerRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	reason, err := domain.NewOccurrenceReasonReference(document.OccurrenceReason)
	if err != nil {
		return domain.TenantID{}, domain.BuyEvaluationTriggerRegistration{}, fmt.Errorf("occurrenceReason：%w", err)
	}
	moment, err := domain.BuyEvaluationTriggerMomentFromName(document.Moment)
	if err != nil {
		return domain.TenantID{}, domain.BuyEvaluationTriggerRegistration{}, fmt.Errorf("moment：%w", err)
	}
	registration, err := domain.NewBuyEvaluationTriggerRegistration(reason, moment)
	if err != nil {
		return domain.TenantID{}, domain.BuyEvaluationTriggerRegistration{}, err
	}
	return tenant, registration, nil
}
