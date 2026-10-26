package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUnexpectedSellChargeSave 说明预估客户费用库交回了封闭集合以外的写入结果。
var ErrUnexpectedSellChargeSave = errors.New("settlement accounting: unexpected sell charge save outcome")

// SellChargeOutcome 是由 SELL 评价形成客户费用的结果。金额只来自评价。
type SellChargeOutcome uint8

const (
	SellChargeOutcomeInvalid SellChargeOutcome = iota
	SellChargeFormed
	SellChargeAlreadyFormed
	SellChargeUnratable
	SellChargeConflict
	SellChargeNotAccepted
	SellChargeUndecided
	// SellChargeZeroTotal：评价已完成而合计为零，形成门不成行（ADR-0306 决定一）。是定局，不是未受理：评价不可变，重投
	// 还是零；包裹计费时点那一路据此把请求记成`负向结束（零额）`。
	SellChargeZeroTotal
)

func (outcome SellChargeOutcome) String() string {
	switch outcome {
	case SellChargeFormed:
		return "SELL_CHARGE_FORMED"
	case SellChargeAlreadyFormed:
		return "SELL_CHARGE_ALREADY_FORMED"
	case SellChargeUnratable:
		return "PRICING_EXCLUDED"
	case SellChargeConflict:
		return "EVALUATION_CONFLICT"
	case SellChargeNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case SellChargeUndecided:
		return "SELL_CHARGE_UNDECIDED"
	case SellChargeZeroTotal:
		return "EVALUATION_ZERO_TOTAL"
	default:
		return ""
	}
}

type SellChargeUndecidedReason uint8

const (
	SellChargeUndecidedReasonNone SellChargeUndecidedReason = iota
	SellEvaluationViewUnavailable
	SellEvaluationPending
	SellEvaluationNotFormed
	SellChargeStoreUnavailable
)

func (reason SellChargeUndecidedReason) String() string {
	switch reason {
	case SellEvaluationViewUnavailable:
		return "SELL_EVALUATION_VIEW_UNAVAILABLE"
	case SellEvaluationPending:
		return "SELL_EVALUATION_PENDING"
	case SellEvaluationNotFormed:
		return "SELL_EVALUATION_NOT_FORMED"
	case SellChargeStoreUnavailable:
		return "SELL_CHARGE_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// FormSellCustomerChargeCommand 指名费用身份和费用项目。金额不在命令上。SourceOccurredAt 是来源事实的发生时点，
// 归属日形态为来源发生时取它（ADR-0269 决定三）；包裹计费时点那一路是 SELL 请求上的计价基准时点。
type FormSellCustomerChargeCommand struct {
	TenantID         domain.TenantID
	Charge           string
	FeeItem          string
	Evaluation       string
	FormedAt         time.Time
	SourceOccurredAt time.Time
}

type SellChargeResult struct {
	outcome SellChargeOutcome
	reason  SellChargeUndecidedReason
	charge  domain.CustomerCharge
	has     bool
}

func (result SellChargeResult) Outcome() SellChargeOutcome { return result.outcome }

func (result SellChargeResult) UndecidedReason() SellChargeUndecidedReason { return result.reason }

func (result SellChargeResult) Charge() (domain.CustomerCharge, bool) {
	return result.charge, result.has
}

// FormSellCustomerChargeHandler 把一份已完成的 SELL 评价收成预估客户费用。
type FormSellCustomerChargeHandler struct {
	evaluations ports.SellEvaluationView
	charges     ports.EstimatedCustomerChargeStore
}

func NewFormSellCustomerChargeHandler(
	evaluations ports.SellEvaluationView,
	charges ports.EstimatedCustomerChargeStore,
) (*FormSellCustomerChargeHandler, error) {
	if evaluations == nil || charges == nil {
		return nil, fmt.Errorf("%w: sell customer charge", ErrNilDependency)
	}
	return &FormSellCustomerChargeHandler{evaluations: evaluations, charges: charges}, nil
}

func (handler *FormSellCustomerChargeHandler) Handle(
	ctx context.Context,
	command FormSellCustomerChargeCommand,
) (SellChargeResult, error) {
	chargeID, err := domain.NewCustomerChargeID(command.Charge)
	if err != nil {
		return SellChargeResult{outcome: SellChargeNotAccepted}, nil
	}
	feeItem, err := domain.NewFeeItemReference(command.FeeItem)
	if err != nil {
		return SellChargeResult{outcome: SellChargeNotAccepted}, nil
	}
	evaluation, err := domain.NewSellEvaluationReference(command.Evaluation)
	// 来源发生时点缺席不当未知收下：形成之后再没有一处能把它补上，形态为来源发生的归属日就永远判不出。
	if err != nil || command.FormedAt.IsZero() || command.SourceOccurredAt.IsZero() {
		return SellChargeResult{outcome: SellChargeNotAccepted}, nil
	}
	adoption, found, err := handler.evaluations.LoadSellEvaluation(ctx, command.TenantID, evaluation)
	if err != nil {
		return SellChargeResult{outcome: SellChargeUndecided, reason: SellEvaluationViewUnavailable}, nil
	}
	if !found {
		return SellChargeResult{outcome: SellChargeNotAccepted}, nil
	}
	switch adoption.Outcome {
	case ports.SellEvaluationPending:
		return SellChargeResult{outcome: SellChargeUndecided, reason: SellEvaluationPending}, nil
	case ports.SellEvaluationNotFormed:
		return SellChargeResult{outcome: SellChargeUndecided, reason: SellEvaluationNotFormed}, nil
	case ports.SellEvaluationUnratable:
		return SellChargeResult{outcome: SellChargeUnratable}, nil
	case ports.SellEvaluationConflict:
		return SellChargeResult{outcome: SellChargeConflict}, nil
	case ports.SellEvaluationAmountUndeclared:
		// 卡没声明金额取整：金额换写不成最小币单位，按现卡这份评价永远形成不了费用，重投无益，所以不作未决。包裹计费
		// 时点那一路由外层 FormEstimatedCustomerChargeHandler 在请求册记`待判断`，不会带着这一格调到这里。
		return SellChargeResult{outcome: SellChargeNotAccepted}, nil
	case ports.SellEvaluationCompleted:
	default:
		return SellChargeResult{outcome: SellChargeNotAccepted}, nil
	}
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 chargeID,
		FeeItem:            feeItem,
		Evaluation:         adoption.Evaluation,
		OriginalCurrency:   adoption.OriginalCurrency,
		OriginalMinor:      adoption.OriginalMinor,
		SettlementCurrency: adoption.SettlementCurrency,
		SettlementMinor:    adoption.SettlementMinor,
		Conversion:         adoption.Conversion,
		Stage:              domain.ChargeEstimated,
		FormedAt:           command.FormedAt,
		SourceOccurredAt:   command.SourceOccurredAt,
	})
	if errors.Is(err, domain.ErrZeroEvaluationTotal) {
		return SellChargeResult{outcome: SellChargeZeroTotal}, nil
	}
	if err != nil {
		return SellChargeResult{outcome: SellChargeNotAccepted}, nil
	}
	saved, err := handler.charges.SaveEstimated(ctx, command.TenantID, charge)
	if err != nil {
		return SellChargeResult{outcome: SellChargeUndecided, reason: SellChargeStoreUnavailable}, nil
	}
	switch saved {
	case ports.ChargeSaved:
		return SellChargeResult{outcome: SellChargeFormed, charge: charge, has: true}, nil
	case ports.ChargeAlreadyConfirmed:
		return SellChargeResult{outcome: SellChargeAlreadyFormed, charge: charge, has: true}, nil
	default:
		return SellChargeResult{}, fmt.Errorf("%w: %d", ErrUnexpectedSellChargeSave, saved)
	}
}
