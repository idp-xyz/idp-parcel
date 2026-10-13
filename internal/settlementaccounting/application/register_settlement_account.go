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

// RegisterSettlementAccountHandler 把一笔账户登记交给登记册，并在同一事务里交出「结算账户已登记」信封。
// 落库时刻由时钟给出，不是载荷的一格。
type RegisterSettlementAccountHandler struct {
	accounts ports.SettlementAccountRegister
	handoff  ports.SettlementAccountRegisteredHandoff
	clock    ports.Clock
}

// NewRegisterSettlementAccountHandler 三件都不许为 nil：交接口缺席时登记照样成功，停在作用域那一格的委托
// 却再也等不来重驱——那正是 ADR-0094 决定四说的「更安静的永久停滞」，装配疏漏不能长成它。
func NewRegisterSettlementAccountHandler(
	accounts ports.SettlementAccountRegister,
	handoff ports.SettlementAccountRegisteredHandoff,
	clock ports.Clock,
) (*RegisterSettlementAccountHandler, error) {
	if accounts == nil || handoff == nil || clock == nil {
		return nil, fmt.Errorf("%w: settlement account register", ErrNilDependency)
	}
	return &RegisterSettlementAccountHandler{accounts: accounts, handoff: handoff, clock: clock}, nil
}

// Register 判据是落点：`已登记`与`已存在`都交信封（重放重发同一封，ADR-0043，认领由适配器按同一个 ID
// 去重；本口落地之前登进册的账户也借重放补上它那一封），`冲突`不交——冲突那一行没写进册，下游重驱只会
// 再撞见同一个缺口。交接失败上抛，信封与登记行同生共死。
func (handler *RegisterSettlementAccountHandler) Register(
	ctx context.Context,
	command RegisterSettlementAccountCommand,
) (ports.SettlementAccountRegistrationEffect, error) {
	if command.Tenant.String() == "" || command.Account.ID().String() == "" {
		return 0, fmt.Errorf("%w: settlement account command", domain.ErrBlankValue)
	}
	registeredAt := handler.clock.Now()
	effect, err := handler.accounts.Save(ctx, command.Tenant, command.Account, registeredAt)
	if err != nil {
		return 0, fmt.Errorf("register settlement account: %w", err)
	}
	switch effect {
	case ports.SettlementAccountRegistered, ports.SettlementAccountReplay:
	case ports.SettlementAccountConflict:
		return effect, nil
	default:
		return 0, fmt.Errorf("register settlement account: unknown registration effect %d", effect)
	}
	if err := handler.handoff.HandOffSettlementAccountRegistered(ctx, ports.SettlementAccountRegisteredIntent{
		Tenant:       command.Tenant,
		Account:      command.Account.ID(),
		RegisteredAt: registeredAt,
	}); err != nil {
		return 0, fmt.Errorf("register settlement account: %w", err)
	}
	return effect, nil
}
