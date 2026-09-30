package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// BuyEvaluationTriggerOutcome 是触发面的答案。未配置时不发起请求。
type BuyEvaluationTriggerOutcome uint8

const (
	BuyEvaluationTriggerOutcomeInvalid BuyEvaluationTriggerOutcome = iota
	BuyEvaluationTriggerUnconfigured
	BuyEvaluationTriggerViewUnavailable
	BuyEvaluationTriggerNotAccepted
	BuyEvaluationTriggerFired
)

func (outcome BuyEvaluationTriggerOutcome) String() string {
	switch outcome {
	case BuyEvaluationTriggerUnconfigured:
		return "BUY_EVALUATION_TRIGGER_UNCONFIGURED"
	case BuyEvaluationTriggerViewUnavailable:
		return "BUY_EVALUATION_TRIGGER_VIEW_UNAVAILABLE"
	case BuyEvaluationTriggerNotAccepted:
		return "BUY_EVALUATION_TRIGGER_NOT_ACCEPTED"
	case BuyEvaluationTriggerFired:
		return "BUY_EVALUATION_TRIGGER_FIRED"
	default:
		return ""
	}
}

// BuyEvaluationTriggerResult 在发起时带上请求编排的答案。没登记时请求结果是空的。
type BuyEvaluationTriggerResult struct {
	outcome BuyEvaluationTriggerOutcome
	request RequestBuyEvaluationResult
	has     bool
}

func (result BuyEvaluationTriggerResult) Outcome() BuyEvaluationTriggerOutcome {
	return result.outcome
}

func (result BuyEvaluationTriggerResult) Request() (RequestBuyEvaluationResult, bool) {
	return result.request, result.has
}

// TriggerBuyEvaluationHandler 按已登记的发生项原因决定要不要发起 BUY 评价请求。
// 册上没有这一原因答未配置，不调用请求编排，也不预列订舱、取消或履约。
type TriggerBuyEvaluationHandler struct {
	triggers ports.BuyEvaluationTriggerView
	requests *RequestBuyEvaluationHandler
}

func NewTriggerBuyEvaluationHandler(
	triggers ports.BuyEvaluationTriggerView,
	requests *RequestBuyEvaluationHandler,
) (*TriggerBuyEvaluationHandler, error) {
	if triggers == nil || requests == nil {
		return nil, fmt.Errorf("%w: buy evaluation trigger", ErrNilDependency)
	}
	return &TriggerBuyEvaluationHandler{triggers: triggers, requests: requests}, nil
}

func (handler *TriggerBuyEvaluationHandler) Trigger(
	ctx context.Context,
	command RequestBuyEvaluationCommand,
) (BuyEvaluationTriggerResult, error) {
	reason := command.Occurrence.Reason()
	if command.TenantID.String() == "" || reason.String() == "" {
		return BuyEvaluationTriggerResult{outcome: BuyEvaluationTriggerNotAccepted}, nil
	}
	moment, found, err := handler.triggers.LoadBuyEvaluationTrigger(ctx, command.TenantID, reason)
	if err != nil {
		return BuyEvaluationTriggerResult{outcome: BuyEvaluationTriggerViewUnavailable}, nil
	}
	if !found || moment != domain.BuyEvaluationTriggerOnOccurrenceFormed {
		return BuyEvaluationTriggerResult{outcome: BuyEvaluationTriggerUnconfigured}, nil
	}
	requested, err := handler.requests.Handle(ctx, command)
	if err != nil {
		return BuyEvaluationTriggerResult{}, err
	}
	return BuyEvaluationTriggerResult{
		outcome: BuyEvaluationTriggerFired,
		request: requested,
		has:     true,
	}, nil
}

// RegisterBuyEvaluationTriggerHandler 登记一条发生项原因采用发生项形成这一时点。落库时刻不是载荷的一格。
type RegisterBuyEvaluationTriggerHandler struct {
	triggers ports.BuyEvaluationTriggerRegister
	clock    ports.Clock
}

func NewRegisterBuyEvaluationTriggerHandler(
	triggers ports.BuyEvaluationTriggerRegister,
	clock ports.Clock,
) (*RegisterBuyEvaluationTriggerHandler, error) {
	if triggers == nil || clock == nil {
		return nil, fmt.Errorf("%w: buy evaluation trigger register", ErrNilDependency)
	}
	return &RegisterBuyEvaluationTriggerHandler{triggers: triggers, clock: clock}, nil
}

func (handler *RegisterBuyEvaluationTriggerHandler) RegisterBuyEvaluationTrigger(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.BuyEvaluationTriggerRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: tenant", domain.ErrBlankValue)
	}
	return handler.triggers.SaveBuyEvaluationTrigger(ctx, tenant, registration, handler.clock.Now())
}
