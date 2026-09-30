package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SellEvaluationTriggerOutcome 是 SELL 触发面的答案。未配置时不发起。
type SellEvaluationTriggerOutcome uint8

const (
	SellEvaluationTriggerOutcomeInvalid SellEvaluationTriggerOutcome = iota
	SellEvaluationTriggerUnconfigured
	SellEvaluationTriggerViewUnavailable
	SellEvaluationTriggerNotAccepted
	SellEvaluationTriggerFired
)

func (outcome SellEvaluationTriggerOutcome) String() string {
	switch outcome {
	case SellEvaluationTriggerUnconfigured:
		return "SELL_EVALUATION_TRIGGER_UNCONFIGURED"
	case SellEvaluationTriggerViewUnavailable:
		return "SELL_EVALUATION_TRIGGER_VIEW_UNAVAILABLE"
	case SellEvaluationTriggerNotAccepted:
		return "SELL_EVALUATION_TRIGGER_NOT_ACCEPTED"
	case SellEvaluationTriggerFired:
		return "SELL_EVALUATION_TRIGGER_FIRED"
	default:
		return ""
	}
}

type SellEvaluationTriggerResult struct {
	outcome SellEvaluationTriggerOutcome
}

func (result SellEvaluationTriggerResult) Outcome() SellEvaluationTriggerOutcome {
	return result.outcome
}

// TriggerSellEvaluationCommand 只带租户和发生项原因。不携带 BUY 评价请求的三件来源。
type TriggerSellEvaluationCommand struct {
	TenantID domain.TenantID
	Reason   domain.OccurrenceReasonReference
}

// TriggerSellEvaluationHandler 按已登记的发生项原因决定要不要发起 SELL 评价。
// 册上没有这一原因答未配置。不调用 BUY 评价请求编排。
type TriggerSellEvaluationHandler struct {
	triggers ports.SellEvaluationTriggerView
}

func NewTriggerSellEvaluationHandler(triggers ports.SellEvaluationTriggerView) (*TriggerSellEvaluationHandler, error) {
	if triggers == nil {
		return nil, fmt.Errorf("%w: sell evaluation trigger", ErrNilDependency)
	}
	return &TriggerSellEvaluationHandler{triggers: triggers}, nil
}

func (handler *TriggerSellEvaluationHandler) Trigger(
	ctx context.Context,
	command TriggerSellEvaluationCommand,
) (SellEvaluationTriggerResult, error) {
	if command.TenantID.String() == "" || command.Reason.String() == "" {
		return SellEvaluationTriggerResult{outcome: SellEvaluationTriggerNotAccepted}, nil
	}
	moment, found, err := handler.triggers.LoadSellEvaluationTrigger(ctx, command.TenantID, command.Reason)
	if err != nil {
		return SellEvaluationTriggerResult{outcome: SellEvaluationTriggerViewUnavailable}, nil
	}
	if !found || moment != domain.SellEvaluationTriggerOnOccurrenceFormed {
		return SellEvaluationTriggerResult{outcome: SellEvaluationTriggerUnconfigured}, nil
	}
	return SellEvaluationTriggerResult{outcome: SellEvaluationTriggerFired}, nil
}

// RegisterSellEvaluationTriggerHandler 登记一条发生项原因采用发生项形成这一时点。
type RegisterSellEvaluationTriggerHandler struct {
	triggers ports.SellEvaluationTriggerRegister
	clock    ports.Clock
}

func NewRegisterSellEvaluationTriggerHandler(
	triggers ports.SellEvaluationTriggerRegister,
	clock ports.Clock,
) (*RegisterSellEvaluationTriggerHandler, error) {
	if triggers == nil || clock == nil {
		return nil, fmt.Errorf("%w: sell evaluation trigger register", ErrNilDependency)
	}
	return &RegisterSellEvaluationTriggerHandler{triggers: triggers, clock: clock}, nil
}

func (handler *RegisterSellEvaluationTriggerHandler) RegisterSellEvaluationTrigger(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.SellEvaluationTriggerRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: tenant", domain.ErrBlankValue)
	}
	return handler.triggers.SaveSellEvaluationTrigger(ctx, tenant, registration, handler.clock.Now())
}
