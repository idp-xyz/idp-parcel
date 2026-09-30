package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterPeriodicFeeHandler 登记一种周期费用形态。落库时刻不是载荷的一格。
type RegisterPeriodicFeeHandler struct {
	fees  ports.PeriodicFeeRegister
	clock ports.Clock
}

func NewRegisterPeriodicFeeHandler(
	fees ports.PeriodicFeeRegister,
	clock ports.Clock,
) (*RegisterPeriodicFeeHandler, error) {
	if fees == nil || clock == nil {
		return nil, fmt.Errorf("%w: periodic fee", ErrNilDependency)
	}
	return &RegisterPeriodicFeeHandler{fees: fees, clock: clock}, nil
}

func (handler *RegisterPeriodicFeeHandler) RegisterPeriodicFee(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.PeriodicFeeRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: tenant", domain.ErrBlankValue)
	}
	return handler.fees.SavePeriodicFee(ctx, tenant, registration, handler.clock.Now())
}
