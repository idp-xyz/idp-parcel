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

// RegisterChargeConfirmationConditionHandler 登记确认条件目录的一行：哪个费用项目要哪种确认依据（ADR-0336 决定二）。
// 用哪种依据确认哪类费用是租户取值，本口不预选；落库时刻不是载荷的一格。
type RegisterChargeConfirmationConditionHandler struct {
	conditions ports.ChargeConfirmationConditionRegister
	clock      ports.Clock
}

func NewRegisterChargeConfirmationConditionHandler(
	conditions ports.ChargeConfirmationConditionRegister,
	clock ports.Clock,
) (*RegisterChargeConfirmationConditionHandler, error) {
	if conditions == nil || clock == nil {
		return nil, fmt.Errorf("%w: charge confirmation condition register", ErrNilDependency)
	}
	return &RegisterChargeConfirmationConditionHandler{conditions: conditions, clock: clock}, nil
}

func (handler *RegisterChargeConfirmationConditionHandler) RegisterChargeConfirmationCondition(
	ctx context.Context,
	tenant domain.TenantID,
	feeItem domain.FeeItemReference,
	kind domain.ConfirmationBasisKind,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" || feeItem.String() == "" || kind.String() == "" {
		return 0, fmt.Errorf("%w: charge confirmation condition", domain.ErrBlankValue)
	}
	return handler.conditions.SaveConfirmationCondition(ctx, tenant, feeItem, kind, handler.clock.Now())
}

// RegisterStatementCycleHandler 登一户的对账周期（ADR-0339 决定三）。切法与截止日是租户取值，本口不预选；落库时刻不是载荷的一格。
type RegisterStatementCycleHandler struct {
	cycles ports.StatementCycleRegister
	clock  ports.Clock
}

func NewRegisterStatementCycleHandler(cycles ports.StatementCycleRegister, clock ports.Clock) (*RegisterStatementCycleHandler, error) {
	if cycles == nil || clock == nil {
		return nil, fmt.Errorf("%w: statement cycle register", ErrNilDependency)
	}
	return &RegisterStatementCycleHandler{cycles: cycles, clock: clock}, nil
}

func (handler *RegisterStatementCycleHandler) RegisterStatementCycle(
	ctx context.Context,
	tenant domain.TenantID,
	cycle domain.StatementCycle,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" || cycle.Account().String() == "" {
		return 0, fmt.Errorf("%w: statement cycle", domain.ErrBlankValue)
	}
	return handler.cycles.SaveStatementCycle(ctx, tenant, cycle, handler.clock.Now())
}
