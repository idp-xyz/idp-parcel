package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ReevaluateSellOnMeasurementChangeOutcome 是一次实测变化对销售评价请求的落点。
type ReevaluateSellOnMeasurementChangeOutcome uint8

const (
	ReevaluateSellOnMeasurementChangeOutcomeInvalid ReevaluateSellOnMeasurementChangeOutcome = iota
	SellReevaluationNoRequest
	SellReevaluationOpened
	SellReevaluationAlreadyOpened
)

func (outcome ReevaluateSellOnMeasurementChangeOutcome) String() string {
	switch outcome {
	case SellReevaluationNoRequest:
		return "NO_SELL_REQUEST"
	case SellReevaluationOpened:
		return "GENERATION_OPENED"
	case SellReevaluationAlreadyOpened:
		return "ALREADY_OPENED"
	case ReevaluateSellOnMeasurementChangeOutcomeInvalid:
		return ""
	default:
		return ""
	}
}

// ReevaluateSellOnMeasurementChangeCommand 点名哪一次测量变化要另起销售评价的下一代。
// SourceID 是这次登记或作废自己的来源身份。
type ReevaluateSellOnMeasurementChangeCommand struct {
	Tenant   domain.TenantID
	Parcel   domain.ParcelReference
	SourceID string
}

// ReevaluateSellOnMeasurementChangeHandler 找到这件包裹当前的销售评价请求，凭这次变化另起下一代。
// 还没有请求时什么都不做：第一次评价会自己读到实测。同一来源沿代次链已经用过，不再开一代。
type ReevaluateSellOnMeasurementChangeHandler struct {
	requests   ports.SellEvaluationRequestView
	successors ports.SellEvaluationRequestSuccessorView
	registry   ports.SellEvaluationRequestRegistry
	identity   ports.SellEvaluationRequestIdentityFactory
	handoff    ports.SellEvaluationRequestHandoff
	clock      ports.Clock
}

func NewReevaluateSellOnMeasurementChangeHandler(
	requests ports.SellEvaluationRequestView,
	successors ports.SellEvaluationRequestSuccessorView,
	registry ports.SellEvaluationRequestRegistry,
	identity ports.SellEvaluationRequestIdentityFactory,
	handoff ports.SellEvaluationRequestHandoff,
	clock ports.Clock,
) (*ReevaluateSellOnMeasurementChangeHandler, error) {
	if requests == nil || successors == nil || registry == nil || identity == nil || handoff == nil || clock == nil {
		return nil, fmt.Errorf("settlement accounting: sell reevaluation on measurement change is missing a dependency")
	}
	return &ReevaluateSellOnMeasurementChangeHandler{
		requests: requests, successors: successors, registry: registry,
		identity: identity, handoff: handoff, clock: clock,
	}, nil
}

func (handler *ReevaluateSellOnMeasurementChangeHandler) Handle(
	ctx context.Context,
	command ReevaluateSellOnMeasurementChangeCommand,
) (ReevaluateSellOnMeasurementChangeOutcome, error) {
	if command.Tenant.String() == "" || command.Parcel.String() == "" || command.SourceID == "" {
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, fmt.Errorf("sell reevaluation: tenant, parcel and source are required")
	}
	basis, err := domain.NewMeasurementChangeResumeBasis(command.SourceID)
	if err != nil {
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, err
	}
	current, found, err := handler.requests.FindCurrentByParcel(ctx, command.Tenant, command.Parcel)
	if err != nil {
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, err
	}
	if !found {
		return SellReevaluationNoRequest, nil
	}
	chain := handler.chain(command.Tenant)
	head, err := chain.head(ctx, current)
	if err != nil {
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, err
	}
	used, err := chain.basisAlreadyUsed(ctx, head, basis)
	if err != nil {
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, err
	}
	if used {
		return SellReevaluationAlreadyOpened, nil
	}
	id, err := handler.identity.MintSellEvaluationRequestID(ctx)
	if err != nil {
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, err
	}
	next, err := head.Request.Resume(id, basis)
	if err != nil {
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, err
	}
	record := ports.SellEvaluationRequestRecord{
		Key:        ports.SellEvaluationRequestKey{TenantID: command.Tenant, Request: id},
		Request:    next,
		RecordedAt: handler.clock.Now(),
		State:      domain.SellRequestSubmitted,
	}
	saved, err := handler.registry.Save(ctx, record)
	if err != nil {
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, err
	}
	switch saved {
	case ports.EvaluationRequestSaved:
	case ports.EvaluationRequestAlreadyRequested:
		return SellReevaluationAlreadyOpened, nil
	default:
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, fmt.Errorf("sell reevaluation: unexpected save outcome %s", saved)
	}
	if err := handler.handoff.HandOffSellEvaluationRequest(ctx, ports.SellEvaluationRequestIntent{Record: record}); err != nil {
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, err
	}
	return SellReevaluationOpened, nil
}

// chain 把销售请求的读口接成代次链上的走法；沿前一代往回读的是同一租户的请求。
func (handler *ReevaluateSellOnMeasurementChangeHandler) chain(
	tenant domain.TenantID,
) resumeChain[ports.SellEvaluationRequestRecord, domain.SellEvaluationRequestID] {
	return resumeChain[ports.SellEvaluationRequestRecord, domain.SellEvaluationRequestID]{
		label: "sell reevaluation",
		successorOf: func(ctx context.Context, record ports.SellEvaluationRequestRecord) (ports.SellEvaluationRequestRecord, bool, error) {
			return handler.successors.FindSuccessor(ctx, record.Key)
		},
		basisOf: func(record ports.SellEvaluationRequestRecord) domain.ResumeBasis {
			return record.Request.ResumeBasis()
		},
		previousOf: func(record ports.SellEvaluationRequestRecord) (domain.SellEvaluationRequestID, bool) {
			return record.Request.Previous()
		},
		findByID: func(ctx context.Context, id domain.SellEvaluationRequestID) (ports.SellEvaluationRequestRecord, bool, error) {
			return handler.requests.FindByID(ctx, ports.SellEvaluationRequestKey{TenantID: tenant, Request: id})
		},
	}
}
