package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ChargeAttributionOutcome 是一次归属日判定的答案。未配置时没有日期。
type ChargeAttributionOutcome uint8

const (
	ChargeAttributionOutcomeInvalid ChargeAttributionOutcome = iota
	ChargeAttributionUnconfigured
	ChargeAttributionViewUnavailable
	ChargeAttributionNotAccepted
	ChargeAttributionJudged
)

func (outcome ChargeAttributionOutcome) String() string {
	switch outcome {
	case ChargeAttributionUnconfigured:
		return "CHARGE_ATTRIBUTION_UNCONFIGURED"
	case ChargeAttributionViewUnavailable:
		return "CHARGE_ATTRIBUTION_VIEW_UNAVAILABLE"
	case ChargeAttributionNotAccepted:
		return "CHARGE_ATTRIBUTION_NOT_ACCEPTED"
	case ChargeAttributionJudged:
		return "CHARGE_ATTRIBUTION_JUDGED"
	default:
		return ""
	}
}

// JudgeChargeAttributionCommand 携带一次判定。只有来源发生与费用确认两个时点。
// 没有包裹创建、收寄或签收。
type JudgeChargeAttributionCommand struct {
	TenantID        domain.TenantID
	FeeItem         string
	SourceOccurred  time.Time
	ChargeConfirmed time.Time
}

// ChargeAttributionResult 在判定成功时交回归属日。没登记时日期是空的。
type ChargeAttributionResult struct {
	outcome ChargeAttributionOutcome
	date    domain.AttributionDate
	has     bool
}

func (result ChargeAttributionResult) Outcome() ChargeAttributionOutcome { return result.outcome }

func (result ChargeAttributionResult) Date() (domain.AttributionDate, bool) {
	return result.date, result.has
}

// JudgeChargeAttributionHandler 按已登记的形态判定归属日。册上没有这一费用项目答未配置，
// 不用签收日或系统当天顶上。
type JudgeChargeAttributionHandler struct {
	attributions ports.ChargeAttributionView
}

func NewJudgeChargeAttributionHandler(attributions ports.ChargeAttributionView) (*JudgeChargeAttributionHandler, error) {
	if attributions == nil {
		return nil, fmt.Errorf("%w: charge attribution", ErrNilDependency)
	}
	return &JudgeChargeAttributionHandler{attributions: attributions}, nil
}

func (handler *JudgeChargeAttributionHandler) Judge(
	ctx context.Context,
	command JudgeChargeAttributionCommand,
) (ChargeAttributionResult, error) {
	feeItem, err := domain.NewFeeItemReference(command.FeeItem)
	if err != nil || command.TenantID.String() == "" {
		return ChargeAttributionResult{outcome: ChargeAttributionNotAccepted}, nil
	}
	registration, found, err := handler.attributions.LoadChargeAttribution(ctx, command.TenantID, feeItem)
	if err != nil {
		return ChargeAttributionResult{outcome: ChargeAttributionViewUnavailable}, nil
	}
	if !found {
		return ChargeAttributionResult{outcome: ChargeAttributionUnconfigured}, nil
	}
	date, err := registration.Judge(command.SourceOccurred, command.ChargeConfirmed)
	if err != nil {
		return ChargeAttributionResult{outcome: ChargeAttributionNotAccepted}, nil
	}
	return ChargeAttributionResult{outcome: ChargeAttributionJudged, date: date, has: true}, nil
}

// RegisterChargeAttributionHandler 登记一条费用项目的归属日判定。落库时刻不是载荷的一格。
type RegisterChargeAttributionHandler struct {
	attributions ports.ChargeAttributionRegister
	clock        ports.Clock
}

func NewRegisterChargeAttributionHandler(
	attributions ports.ChargeAttributionRegister,
	clock ports.Clock,
) (*RegisterChargeAttributionHandler, error) {
	if attributions == nil || clock == nil {
		return nil, fmt.Errorf("%w: charge attribution register", ErrNilDependency)
	}
	return &RegisterChargeAttributionHandler{attributions: attributions, clock: clock}, nil
}

func (handler *RegisterChargeAttributionHandler) RegisterChargeAttribution(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.ChargeAttributionRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: tenant", domain.ErrBlankValue)
	}
	return handler.attributions.SaveChargeAttribution(ctx, tenant, registration, handler.clock.Now())
}
