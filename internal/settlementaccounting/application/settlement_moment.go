package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SettlementMomentOutcome 是确认或截单触发面的答案。未配置时不调用对应编排。
type SettlementMomentOutcome uint8

const (
	SettlementMomentOutcomeInvalid SettlementMomentOutcome = iota
	SettlementMomentUnconfigured
	SettlementMomentViewUnavailable
	SettlementMomentNotAccepted
	SettlementMomentAdmitted
)

func (outcome SettlementMomentOutcome) String() string {
	switch outcome {
	case SettlementMomentUnconfigured:
		return "SETTLEMENT_MOMENT_UNCONFIGURED"
	case SettlementMomentViewUnavailable:
		return "SETTLEMENT_MOMENT_VIEW_UNAVAILABLE"
	case SettlementMomentNotAccepted:
		return "SETTLEMENT_MOMENT_NOT_ACCEPTED"
	case SettlementMomentAdmitted:
		return "SETTLEMENT_MOMENT_ADMITTED"
	default:
		return ""
	}
}

// AdmitSettlementMomentHandler 问这一租户有没有采用确认或截单。册上没有就答未配置。
type AdmitSettlementMomentHandler struct {
	moments ports.SettlementMomentView
}

func NewAdmitSettlementMomentHandler(moments ports.SettlementMomentView) (*AdmitSettlementMomentHandler, error) {
	if moments == nil {
		return nil, fmt.Errorf("%w: settlement moment", ErrNilDependency)
	}
	return &AdmitSettlementMomentHandler{moments: moments}, nil
}

func (handler *AdmitSettlementMomentHandler) Admit(
	ctx context.Context,
	tenant domain.TenantID,
	moment domain.SettlementMoment,
) (SettlementMomentOutcome, error) {
	if tenant.String() == "" || (moment != domain.SettlementMomentConfirm && moment != domain.SettlementMomentCutOff) {
		return SettlementMomentNotAccepted, nil
	}
	found, err := handler.moments.LoadSettlementMoment(ctx, tenant, moment)
	if err != nil {
		return SettlementMomentViewUnavailable, nil
	}
	if !found {
		return SettlementMomentUnconfigured, nil
	}
	return SettlementMomentAdmitted, nil
}

// RegisterSettlementMomentHandler 登记租户采用确认或截单。落库时刻不是载荷的一格。
type RegisterSettlementMomentHandler struct {
	moments ports.SettlementMomentRegister
	clock   ports.Clock
}

func NewRegisterSettlementMomentHandler(
	moments ports.SettlementMomentRegister,
	clock ports.Clock,
) (*RegisterSettlementMomentHandler, error) {
	if moments == nil || clock == nil {
		return nil, fmt.Errorf("%w: settlement moment register", ErrNilDependency)
	}
	return &RegisterSettlementMomentHandler{moments: moments, clock: clock}, nil
}

func (handler *RegisterSettlementMomentHandler) RegisterSettlementMoment(
	ctx context.Context,
	tenant domain.TenantID,
	moment domain.SettlementMoment,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" || (moment != domain.SettlementMomentConfirm && moment != domain.SettlementMomentCutOff) {
		return 0, fmt.Errorf("%w: settlement moment", domain.ErrBlankValue)
	}
	return handler.moments.SaveSettlementMoment(ctx, tenant, moment, handler.clock.Now())
}
