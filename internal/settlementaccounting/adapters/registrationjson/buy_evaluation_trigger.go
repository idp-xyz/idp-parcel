package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// feeItem 可缺：缺了这一行照登，发生项已形成那一路对它答未配置（ADR-0208）。给了空串不是「没给」，按坏值拒。
type buyEvaluationTriggerDocument struct {
	TenantID         string  `json:"tenantId"`
	OccurrenceReason string  `json:"occurrenceReason"`
	Moment           string  `json:"moment"`
	FeeItem          *string `json:"feeItem"`
}

// BuyEvaluationTriggerFromJSON 译装一条触发选用。不预填发生项原因与费用项目，也不发起评价请求。
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
	if document.FeeItem != nil {
		feeItem, err := domain.NewFeeItemReference(*document.FeeItem)
		if err != nil {
			return domain.TenantID{}, domain.BuyEvaluationTriggerRegistration{}, fmt.Errorf("feeItem：%w", err)
		}
		if registration, err = registration.WithFeeItem(feeItem); err != nil {
			return domain.TenantID{}, domain.BuyEvaluationTriggerRegistration{}, err
		}
	}
	return tenant, registration, nil
}
