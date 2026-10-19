package ports

import (
	"context"
	"errors"
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

// SellEvaluationOutcome 是一份 SELL 评价在 SA 眼里的封闭结果。各格与 BUY 同形，但是另一条读口。
//
// SellEvaluationAmountUndeclared：评价已完成，可价卡没声明金额取整（提供方问题项
// AMOUNT_PRECISION_UNDECLARED），合计是精确十进制，本上下文没有依据把它换写成最小币单位（ADR-0107 决定五）。
// 它不是读口故障——评价不可变，重读一百次还是这一格——所以作结果交回、不作错误：SELL 形成编排要凭同一份采用
// 快照里的回指在请求册上记`待判断`（ADR-0190 决定五），报错就连回指一起丢了。这一格不带金额。
type SellEvaluationOutcome uint8

const (
	SellEvaluationOutcomeInvalid SellEvaluationOutcome = iota
	SellEvaluationCompleted
	SellEvaluationPending
	SellEvaluationUnratable
	SellEvaluationConflict
	SellEvaluationNotFormed
	SellEvaluationAmountUndeclared
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
	case SellEvaluationAmountUndeclared:
		return "AMOUNT_PRECISION_UNDECLARED"
	default:
		return ""
	}
}

// SellEvaluationAdoption 是从一份 SELL·CUSTOMER_CHARGE 评价采用的金额。命令不得再带一套金额。
//
// Request 是评价回指的请求标识，照 SELL 请求身份读；评价没有回指时是零值。它是不是真的一份 SELL 请求，由 SELL 请求册
// 按标识回答——计价对 BUY、SELL 两种请求共用一列回指（票 sell-customer-charge/03），读口这一层不认前缀、不猜。
//
// ChargeCodes 是评价全部费用行的计价费用代码，按行序、不去重；只有已完成的评价（含金额未声明那一格）才有费用行。
// 映到哪个费用项目归费用项目映射册（ADR-0190 决定五），读口只照抄代码。
type SellEvaluationAdoption struct {
	Evaluation         domain.SellEvaluationReference
	Outcome            SellEvaluationOutcome
	Request            domain.SellEvaluationRequestID
	ChargeCodes        []domain.PricingChargeCode
	OriginalCurrency   domain.CurrencyCode
	OriginalMinor      int64
	SettlementCurrency domain.CurrencyCode
	SettlementMinor    int64
	Conversion         domain.ConversionStepReference
}

// ErrNotASellEvaluation 表示指名的评价不是 SELL·CUSTOMER_CHARGE。提供方对 BUY、SELL 发同一种评价已记录信封，SELL 消费门
// 凭它答「不是本门的评价」、入账不重投。
var ErrNotASellEvaluation = errors.New("settlement accounting: evaluation is not SELL / CUSTOMER_CHARGE")

// SellEvaluationView 按评价引用取 SELL 采用快照。方向不是 SELL·CUSTOMER_CHARGE 时返回 errors.Is ErrNotASellEvaluation 的
// 错误，不把 BUY 装进来。
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
