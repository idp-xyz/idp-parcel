package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// BuyEvaluationTriggerRegister 写下一条发生项原因采用的触发时点。读口是 BuyEvaluationTriggerView。
// 不预列发生项原因。
type BuyEvaluationTriggerRegister interface {
	SaveBuyEvaluationTrigger(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.BuyEvaluationTriggerRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// BuyEvaluationTriggerView 按发生项原因取已采用的触发时点。found=false 表示没有这一行：
// 不发起评价请求。
type BuyEvaluationTriggerView interface {
	LoadBuyEvaluationTrigger(
		ctx context.Context,
		tenant domain.TenantID,
		reason domain.OccurrenceReasonReference,
	) (domain.BuyEvaluationTriggerMoment, bool, error)
}
