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
	head, err := handler.head(ctx, current)
	if err != nil {
		return ReevaluateSellOnMeasurementChangeOutcomeInvalid, err
	}
	used, err := handler.basisAlreadyUsed(ctx, command.Tenant, head, basis)
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

func (handler *ReevaluateSellOnMeasurementChangeHandler) head(
	ctx context.Context,
	record ports.SellEvaluationRequestRecord,
) (ports.SellEvaluationRequestRecord, error) {
	for {
		successor, found, err := handler.successors.FindSuccessor(ctx, record.Key)
		if err != nil {
			return ports.SellEvaluationRequestRecord{}, err
		}
		if !found {
			return record, nil
		}
		record = successor
	}
}

func (handler *ReevaluateSellOnMeasurementChangeHandler) basisAlreadyUsed(
	ctx context.Context,
	tenant domain.TenantID,
	record ports.SellEvaluationRequestRecord,
	basis domain.ResumeBasis,
) (bool, error) {
	for {
		if record.Request.ResumeBasis() == basis {
			return true, nil
		}
		previous, has := record.Request.Previous()
		if !has {
			return false, nil
		}
		earlier, found, err := handler.requests.FindByID(ctx, ports.SellEvaluationRequestKey{TenantID: tenant, Request: previous})
		if err != nil {
			return false, err
		}
		if !found {
			return false, fmt.Errorf("sell reevaluation: previous request %s is missing", previous)
		}
		record = earlier
	}
}
