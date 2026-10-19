package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterSellBillingMomentAdoptionCommand 是一行 SELL 计费时点采用的登记（ADR-0190 决定三）。租户在命令上，不从上下文里补。
type RegisterSellBillingMomentAdoptionCommand struct {
	Tenant   domain.TenantID
	Adoption domain.SellBillingMomentAdoption
}

// RegisterSellBillingMomentAdoptionHandler 把一行采用交给采用册。与受理前估价采用不同，这里不交「已登记」信封：没登记时
// 时点到来的那一拍已经入账不重投（ADR-0190 决定三），补登之后没有停着等它的东西可重驱。落库时刻由时钟给出。
type RegisterSellBillingMomentAdoptionHandler struct {
	adoptions ports.SellBillingMomentAdoptionRegister
	clock     ports.Clock
}

func NewRegisterSellBillingMomentAdoptionHandler(
	adoptions ports.SellBillingMomentAdoptionRegister,
	clock ports.Clock,
) (*RegisterSellBillingMomentAdoptionHandler, error) {
	if adoptions == nil || clock == nil {
		return nil, fmt.Errorf("%w: sell billing moment adoption register", ErrNilDependency)
	}
	return &RegisterSellBillingMomentAdoptionHandler{adoptions: adoptions, clock: clock}, nil
}

func (handler *RegisterSellBillingMomentAdoptionHandler) Register(
	ctx context.Context,
	command RegisterSellBillingMomentAdoptionCommand,
) (ports.CatalogueRegistrationEffect, error) {
	if command.Tenant.String() == "" || command.Adoption.Account().String() == "" {
		return 0, fmt.Errorf("%w: sell billing moment adoption command", domain.ErrBlankValue)
	}
	effect, err := handler.adoptions.SaveSellBillingMomentAdoption(ctx, command.Tenant, command.Adoption, handler.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("register sell billing moment adoption: %w", err)
	}
	return effect, nil
}

// RegisterFeeItemMappingHandler 登记一行费用项目映射（ADR-0190 决定五）。只增不改的判定在册上，这里不重判。
type RegisterFeeItemMappingHandler struct {
	mappings ports.FeeItemMappingRegister
	clock    ports.Clock
}

func NewRegisterFeeItemMappingHandler(
	mappings ports.FeeItemMappingRegister,
	clock ports.Clock,
) (*RegisterFeeItemMappingHandler, error) {
	if mappings == nil || clock == nil {
		return nil, fmt.Errorf("%w: fee item mapping register", ErrNilDependency)
	}
	return &RegisterFeeItemMappingHandler{mappings: mappings, clock: clock}, nil
}

func (handler *RegisterFeeItemMappingHandler) RegisterFeeItemMapping(
	ctx context.Context,
	tenant domain.TenantID,
	mapping domain.FeeItemMapping,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" || mapping.ChargeCode().String() == "" {
		return 0, fmt.Errorf("%w: fee item mapping command", domain.ErrBlankValue)
	}
	effect, err := handler.mappings.SaveFeeItemMapping(ctx, tenant, mapping, handler.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("register fee item mapping: %w", err)
	}
	return effect, nil
}
