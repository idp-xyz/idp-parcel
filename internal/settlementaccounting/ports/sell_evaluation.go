package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// SellEvaluationTriggerRegister 写下哪些发生项原因会发起 SELL 评价。读口是 SellEvaluationTriggerView。
// 不写进 BUY 评价请求。
type SellEvaluationTriggerRegister interface {
	SaveSellEvaluationTrigger(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.SellEvaluationTriggerRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// SellEvaluationTriggerView 按发生项原因取 SELL 触发时点。found=false 表示没有这一行：
// 不发起 SELL 评价。
type SellEvaluationTriggerView interface {
	LoadSellEvaluationTrigger(
		ctx context.Context,
		tenant domain.TenantID,
		reason domain.OccurrenceReasonReference,
	) (domain.SellEvaluationTriggerMoment, bool, error)
}

// SellEvaluationOutcome 是一份 SELL 评价在 SA 眼里的封闭结果。与 BUY 的五格同形，但是另一条读口。
type SellEvaluationOutcome uint8

const (
	SellEvaluationOutcomeInvalid SellEvaluationOutcome = iota
	SellEvaluationCompleted
	SellEvaluationPending
	SellEvaluationUnratable
	SellEvaluationConflict
	SellEvaluationNotFormed
)

func (outcome SellEvaluationOutcome) String() string {
	switch outcome {
	case SellEvaluationCompleted:
		return "COMPLETED"
	case SellEvaluationPending:
		return "PENDING"
	case SellEvaluationUnratable:
		return "UNRATABLE"
	case SellEvaluationConflict:
		return "CONFLICT"
	case SellEvaluationNotFormed:
		return "NOT_FORMED"
	default:
		return ""
	}
}

// SellEvaluationAdoption 是从一份 SELL·CUSTOMER_CHARGE 评价采用的金额。命令不得再带一套金额。
type SellEvaluationAdoption struct {
	Evaluation         domain.SellEvaluationReference
	Outcome            SellEvaluationOutcome
	OriginalCurrency   domain.CurrencyCode
	OriginalMinor      int64
	SettlementCurrency domain.CurrencyCode
	SettlementMinor    int64
	Conversion         domain.ConversionStepReference
}

// SellEvaluationView 按评价引用取 SELL 采用快照。方向不是 SELL·CUSTOMER_CHARGE 时返回错误，不把 BUY 装进来。
type SellEvaluationView interface {
	LoadSellEvaluation(
		ctx context.Context,
		tenant domain.TenantID,
		evaluation domain.SellEvaluationReference,
	) (SellEvaluationAdoption, bool, error)
}

// EstimatedCustomerChargeStore 保存一笔尚未确认的客户费用。确认仍走 CustomerChargeStore.SaveConfirmed。
type EstimatedCustomerChargeStore interface {
	SaveEstimated(
		ctx context.Context,
		tenant domain.TenantID,
		charge domain.CustomerCharge,
	) (ChargeSaveOutcome, error)
}
