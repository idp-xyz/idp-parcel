package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterOperatingBasisAdoptionHandler 登记一户采用哪些内置经营口径。谁采用是租户取值，
// 本口不预选、不把缺席的布尔当成不采用——那一步在译装时就拒了。
type RegisterOperatingBasisAdoptionHandler struct {
	adoptions ports.OperatingBasisAdoptionStore
	clock     ports.Clock
}

func NewRegisterOperatingBasisAdoptionHandler(
	adoptions ports.OperatingBasisAdoptionStore,
	clock ports.Clock,
) (*RegisterOperatingBasisAdoptionHandler, error) {
	if adoptions == nil || clock == nil {
		return nil, fmt.Errorf("%w: operating basis adoption", ErrNilDependency)
	}
	return &RegisterOperatingBasisAdoptionHandler{adoptions: adoptions, clock: clock}, nil
}

func (handler *RegisterOperatingBasisAdoptionHandler) Register(
	ctx context.Context,
	tenant domain.TenantID,
	adoption domain.OperatingBasisAdoption,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" || (!adoption.Estimated() && !adoption.Confirmed()) {
		return 0, fmt.Errorf("%w: operating basis adoption", domain.ErrBlankValue)
	}
	effect, err := handler.adoptions.SaveOperatingBasisAdoption(ctx, tenant, adoption, handler.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("register operating basis adoption: %w", err)
	}
	return effect, nil
}
