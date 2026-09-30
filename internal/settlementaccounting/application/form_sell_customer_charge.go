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

// FormSellCustomerChargeCommand 指名费用身份和费用项目。金额不在命令上。
type FormSellCustomerChargeCommand struct {
	TenantID   domain.TenantID
	Charge     string
	FeeItem    string
	Evaluation string
	FormedAt   time.Time
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
	if err != nil || command.FormedAt.IsZero() {
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
	})
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
