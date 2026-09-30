package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterAuditEscalationCeilingHandler 登记越权升级的金额上限。落库时刻不是载荷的一格。
type RegisterAuditEscalationCeilingHandler struct {
	ceilings ports.AuditEscalationCeilingRegister
	clock    ports.Clock
}

func NewRegisterAuditEscalationCeilingHandler(
	ceilings ports.AuditEscalationCeilingRegister,
	clock ports.Clock,
) (*RegisterAuditEscalationCeilingHandler, error) {
	if ceilings == nil || clock == nil {
		return nil, fmt.Errorf("%w: audit escalation ceiling", ErrNilDependency)
	}
	return &RegisterAuditEscalationCeilingHandler{ceilings: ceilings, clock: clock}, nil
}

func (handler *RegisterAuditEscalationCeilingHandler) RegisterAuditEscalationCeiling(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.AuditEscalationCeilingRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: tenant", domain.ErrBlankValue)
	}
	return handler.ceilings.SaveAuditEscalationCeiling(ctx, tenant, registration, handler.clock.Now())
}
