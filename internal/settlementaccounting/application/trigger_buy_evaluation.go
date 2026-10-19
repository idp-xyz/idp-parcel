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
	// BuyEvaluationTriggerUndecided 只出自发生项已形成那一路：某个依赖答不出，停在哪一口见 UndecidedReason。
	// 调用方整笔回滚重来。ADR-0164 那条触发面仍按 ViewUnavailable 答册读不动，两路不合一格。
	BuyEvaluationTriggerUndecided
	// BuyEvaluationTriggerOccurrenceRevised 只出自发生项已形成那一路：这一版是有效性修订（失效或替代），本路不为
	// 它发起请求。修订怎样驱动纠错归票 demo-chain-gaps/15；失效的那一版若也发起，会给一个已证无效的范围估成本。
	BuyEvaluationTriggerOccurrenceRevised
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
	case BuyEvaluationTriggerUndecided:
		return "BUY_EVALUATION_TRIGGER_UNDECIDED"
	case BuyEvaluationTriggerOccurrenceRevised:
		return "OCCURRENCE_REVISION_NOT_TRIGGERED"
	default:
		return ""
	}
}

// BuyEvaluationTriggerUnconfiguredReason 指名`未配置`缺的是哪一格。两格都是租户取值：没登这个原因，或登了却没登费用
// 项目。ADR-0164 那条触发面只会落在前一格。
type BuyEvaluationTriggerUnconfiguredReason uint8

const (
	BuyEvaluationTriggerUnconfiguredReasonNone BuyEvaluationTriggerUnconfiguredReason = iota
	BuyEvaluationTriggerReasonNotRegistered
	BuyEvaluationTriggerFeeItemNotRegistered
)

func (reason BuyEvaluationTriggerUnconfiguredReason) String() string {
	switch reason {
	case BuyEvaluationTriggerReasonNotRegistered:
		return "OCCURRENCE_REASON_NOT_REGISTERED"
	case BuyEvaluationTriggerFeeItemNotRegistered:
		return "FEE_ITEM_NOT_REGISTERED"
	default:
		return ""
	}
}

// BuyEvaluationTriggerUndecidedReason 指名发生项已形成那一路停在哪一口。
type BuyEvaluationTriggerUndecidedReason uint8

const (
	BuyEvaluationTriggerUndecidedReasonNone BuyEvaluationTriggerUndecidedReason = iota
	BuyEvaluationTriggerRegisterUnavailable
	BuyEvaluationTriggerAgreementScopeUnavailable
	BuyEvaluationTriggerRequestUndecided
	BuyEvaluationTriggerRequestHandoffUnavailable
)

func (reason BuyEvaluationTriggerUndecidedReason) String() string {
	switch reason {
	case BuyEvaluationTriggerRegisterUnavailable:
		return "BUY_EVALUATION_TRIGGER_REGISTER_UNAVAILABLE"
	case BuyEvaluationTriggerAgreementScopeUnavailable:
		return "SUPPLIER_AGREEMENT_SCOPE_UNAVAILABLE"
	case BuyEvaluationTriggerRequestUndecided:
		return "EVALUATION_REQUEST_UNDECIDED"
	case BuyEvaluationTriggerRequestHandoffUnavailable:
		return "EVALUATION_REQUEST_HANDOFF_UNAVAILABLE"
	default:
		return ""
	}
}

// BuyEvaluationTriggerResult 在发起时带上请求编排的答案。没登记时请求结果是空的。
type BuyEvaluationTriggerResult struct {
	outcome      BuyEvaluationTriggerOutcome
	unconfigured BuyEvaluationTriggerUnconfiguredReason
	undecided    BuyEvaluationTriggerUndecidedReason
	request      RequestBuyEvaluationResult
	has          bool
}

func (result BuyEvaluationTriggerResult) Outcome() BuyEvaluationTriggerOutcome {
	return result.outcome
}

// UnconfiguredReason 只在发生项已形成那一路答`未配置`时非零。
func (result BuyEvaluationTriggerResult) UnconfiguredReason() BuyEvaluationTriggerUnconfiguredReason {
	return result.unconfigured
}

// UndecidedReason 只在`未决`时非零。
func (result BuyEvaluationTriggerResult) UndecidedReason() BuyEvaluationTriggerUndecidedReason {
	return result.undecided
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
	registration, found, err := handler.triggers.LoadBuyEvaluationTrigger(ctx, command.TenantID, reason)
	if err != nil {
		return BuyEvaluationTriggerResult{outcome: BuyEvaluationTriggerViewUnavailable}, nil
	}
	if !found || registration.Moment() != domain.BuyEvaluationTriggerOnOccurrenceFormed {
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
