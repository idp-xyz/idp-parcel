package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterSettlementAccountCommand 是一笔结算账户登记。租户在命令上，不从上下文里补。
type RegisterSettlementAccountCommand struct {
	Tenant  domain.TenantID
	Account domain.SettlementAccount
}

// RegisterSettlementAccountHandler 把一笔账户登记交给登记册。落库时刻由时钟给出，不是载荷的一格。
type RegisterSettlementAccountHandler struct {
	accounts ports.SettlementAccountRegister
	clock    ports.Clock
}

func NewRegisterSettlementAccountHandler(
	accounts ports.SettlementAccountRegister,
	clock ports.Clock,
) (*RegisterSettlementAccountHandler, error) {
	if accounts == nil || clock == nil {
		return nil, fmt.Errorf("%w: settlement account register", ErrNilDependency)
	}
	return &RegisterSettlementAccountHandler{accounts: accounts, clock: clock}, nil
}

func (handler *RegisterSettlementAccountHandler) Register(
	ctx context.Context,
	command RegisterSettlementAccountCommand,
) (ports.SettlementAccountRegistrationEffect, error) {
	if command.Tenant.String() == "" || command.Account.ID().String() == "" {
		return 0, fmt.Errorf("%w: settlement account command", domain.ErrBlankValue)
	}
	effect, err := handler.accounts.Save(ctx, command.Tenant, command.Account, handler.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("register settlement account: %w", err)
	}
	return effect, nil
}
