package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ReevaluateBuyOnMeasurementChangeOutcome 是一次实测变化对采购评价请求的落点。
type ReevaluateBuyOnMeasurementChangeOutcome uint8

const (
	ReevaluateBuyOnMeasurementChangeOutcomeInvalid ReevaluateBuyOnMeasurementChangeOutcome = iota
	BuyReevaluationNoRequest
	BuyReevaluationOpened
	BuyReevaluationAlreadyOpened
)

func (outcome ReevaluateBuyOnMeasurementChangeOutcome) String() string {
	switch outcome {
	case BuyReevaluationNoRequest:
		return "NO_BUY_REQUEST"
	case BuyReevaluationOpened:
		return "GENERATION_OPENED"
	case BuyReevaluationAlreadyOpened:
		return "ALREADY_OPENED"
	case ReevaluateBuyOnMeasurementChangeOutcomeInvalid:
		return ""
	default:
		return ""
	}
}

// ReevaluateBuyOnMeasurementChangeCommand 点名哪一版发生项要为这次测量变化另起采购评价的下一代。
type ReevaluateBuyOnMeasurementChangeCommand struct {
	Tenant            domain.TenantID
	OccurrenceID      string
	OccurrenceVersion string
	SourceID          string
}

// ReevaluateBuyOnMeasurementChangeHandler 找到这一版发生项上还没有下一代的采购请求，凭这次变化各另起一代。
// 还没有请求时什么都不做。同一来源沿代次链已经用过，不再开一代。发生项版本原样留在请求上。
type ReevaluateBuyOnMeasurementChangeHandler struct {
	current    ports.EvaluationRequestsByOccurrence
	requests   ports.EvaluationRequestView
	successors ports.EvaluationRequestSuccessorView
	registry   ports.EvaluationRequestRegistry
	identity   ports.EvaluationRequestIdentityFactory
	handoff    ports.EvaluationRequestHandoff
	clock      ports.Clock
}

func NewReevaluateBuyOnMeasurementChangeHandler(
	current ports.EvaluationRequestsByOccurrence,
	requests ports.EvaluationRequestView,
	successors ports.EvaluationRequestSuccessorView,
	registry ports.EvaluationRequestRegistry,
	identity ports.EvaluationRequestIdentityFactory,
	handoff ports.EvaluationRequestHandoff,
	clock ports.Clock,
) (*ReevaluateBuyOnMeasurementChangeHandler, error) {
	if current == nil || requests == nil || successors == nil || registry == nil || identity == nil || handoff == nil || clock == nil {
		return nil, fmt.Errorf("settlement accounting: buy reevaluation on measurement change is missing a dependency")
	}
	return &ReevaluateBuyOnMeasurementChangeHandler{
		current: current, requests: requests, successors: successors, registry: registry,
		identity: identity, handoff: handoff, clock: clock,
	}, nil
}

func (handler *ReevaluateBuyOnMeasurementChangeHandler) Handle(
	ctx context.Context,
	command ReevaluateBuyOnMeasurementChangeCommand,
) (ReevaluateBuyOnMeasurementChangeOutcome, error) {
	if command.Tenant.String() == "" || command.OccurrenceID == "" || command.OccurrenceVersion == "" || command.SourceID == "" {
		return ReevaluateBuyOnMeasurementChangeOutcomeInvalid, fmt.Errorf("buy reevaluation: tenant, occurrence and source are required")
	}
	basis, err := domain.NewMeasurementChangeResumeBasis(command.SourceID)
	if err != nil {
		return ReevaluateBuyOnMeasurementChangeOutcomeInvalid, err
	}
	occurrence, err := domain.NewChargeOccurrenceID(command.OccurrenceID)
	if err != nil {
		return ReevaluateBuyOnMeasurementChangeOutcomeInvalid, err
	}
	version, err := domain.NewOccurrenceVersion(command.OccurrenceVersion)
	if err != nil {
		return ReevaluateBuyOnMeasurementChangeOutcomeInvalid, err
	}
	heads, err := handler.current.ListCurrentByOccurrence(ctx, command.Tenant, occurrence, version)
	if err != nil {
		return ReevaluateBuyOnMeasurementChangeOutcomeInvalid, err
	}
	if len(heads) == 0 {
		return BuyReevaluationNoRequest, nil
	}
	opened := false
	for _, current := range heads {
		did, err := handler.open(ctx, command.Tenant, current, basis)
		if err != nil {
			return ReevaluateBuyOnMeasurementChangeOutcomeInvalid, err
		}
		opened = opened || did
	}
	if opened {
		return BuyReevaluationOpened, nil
	}
	return BuyReevaluationAlreadyOpened, nil
}

func (handler *ReevaluateBuyOnMeasurementChangeHandler) open(
	ctx context.Context,
	tenant domain.TenantID,
	record ports.EvaluationRequestRecord,
	basis domain.ResumeBasis,
) (bool, error) {
	head, err := handler.head(ctx, record)
	if err != nil {
		return false, err
	}
	used, err := handler.basisAlreadyUsed(ctx, tenant, head, basis)
	if err != nil {
		return false, err
	}
	if used {
		return false, nil
	}
	id, err := handler.identity.MintEvaluationRequestID(ctx)
	if err != nil {
		return false, err
	}
	now := handler.clock.Now()
	requester, err := domain.NewRequesterReference(resumptionRequester)
	if err != nil {
		return false, err
	}
	next, err := head.Request.Resume(id, basis, now, requester)
	if err != nil {
		return false, err
	}
	savedRecord := ports.EvaluationRequestRecord{
		Key:        ports.EvaluationRequestKey{TenantID: tenant, Request: id},
		Request:    next,
		RecordedAt: now,
	}
	saved, err := handler.registry.Save(ctx, savedRecord)
	if err != nil {
		return false, err
	}
	switch saved {
	case ports.EvaluationRequestSaved:
	case ports.EvaluationRequestAlreadyRequested:
		return false, nil
	default:
		return false, fmt.Errorf("buy reevaluation: unexpected save outcome %s", saved)
	}
	if err := handler.handoff.HandOffEvaluationRequest(ctx, ports.EvaluationRequestIntent{Record: savedRecord}); err != nil {
		return false, err
	}
	return true, nil
}

func (handler *ReevaluateBuyOnMeasurementChangeHandler) head(
	ctx context.Context,
	record ports.EvaluationRequestRecord,
) (ports.EvaluationRequestRecord, error) {
	for {
		successor, found, err := handler.successors.FindSuccessor(ctx, record.Key)
		if err != nil {
			return ports.EvaluationRequestRecord{}, err
		}
		if !found {
			return record, nil
		}
		record = successor
	}
}

func (handler *ReevaluateBuyOnMeasurementChangeHandler) basisAlreadyUsed(
	ctx context.Context,
	tenant domain.TenantID,
	record ports.EvaluationRequestRecord,
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
		earlier, found, err := handler.requests.FindByID(ctx, ports.EvaluationRequestKey{TenantID: tenant, Request: previous})
		if err != nil {
			return false, err
		}
		if !found {
			return false, fmt.Errorf("buy reevaluation: previous request %s is missing", previous)
		}
		record = earlier
	}
}
