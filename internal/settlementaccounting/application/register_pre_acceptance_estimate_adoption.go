package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterPreAcceptanceEstimateAdoptionCommand 是一行受理前估价采用的登记（ADR-0179 决定五）。租户在命令上，不从上下文里补。
type RegisterPreAcceptanceEstimateAdoptionCommand struct {
	Tenant   domain.TenantID
	Adoption domain.PreAcceptanceEstimateAdoption
}

// RegisterPreAcceptanceEstimateAdoptionHandler 把一行采用交给采用册，并在同一事务里交出「受理前估价采用已登记」信封。落库时刻由
// 时钟给出，不是载荷的一格。
type RegisterPreAcceptanceEstimateAdoptionHandler struct {
	adoptions ports.PreAcceptanceEstimateAdoptionRegister
	handoff   ports.PreAcceptanceEstimateAdoptionRegisteredHandoff
	clock     ports.Clock
}

// NewRegisterPreAcceptanceEstimateAdoptionHandler 三件都不许为 nil：交接口缺席时登记照样成功，停在金额那一格的委托却再也等不来
// 重驱——理由同 NewRegisterSettlementAccountHandler。
func NewRegisterPreAcceptanceEstimateAdoptionHandler(
	adoptions ports.PreAcceptanceEstimateAdoptionRegister,
	handoff ports.PreAcceptanceEstimateAdoptionRegisteredHandoff,
	clock ports.Clock,
) (*RegisterPreAcceptanceEstimateAdoptionHandler, error) {
	if adoptions == nil || handoff == nil || clock == nil {
		return nil, fmt.Errorf("%w: pre-acceptance estimate adoption register", ErrNilDependency)
	}
	return &RegisterPreAcceptanceEstimateAdoptionHandler{adoptions: adoptions, handoff: handoff, clock: clock}, nil
}

// Register 的交信封判据与 RegisterSettlementAccountHandler.Register 同一条：`已登记`与`已存在`都交（重放重发同一封，认领按同一个
// ID 去重），`冲突`不交——冲突那一行没写进册，下游重驱只会再撞见同一个缺口。交接失败上抛，信封与采用行同生共死。
func (handler *RegisterPreAcceptanceEstimateAdoptionHandler) Register(
	ctx context.Context,
	command RegisterPreAcceptanceEstimateAdoptionCommand,
) (ports.PreAcceptanceEstimateAdoptionEffect, error) {
	if command.Tenant.String() == "" || command.Adoption.Account().String() == "" {
		return 0, fmt.Errorf("%w: pre-acceptance estimate adoption command", domain.ErrBlankValue)
	}
	registeredAt := handler.clock.Now()
	effect, err := handler.adoptions.SaveAdoption(ctx, command.Tenant, command.Adoption, registeredAt)
	if err != nil {
		return 0, fmt.Errorf("register pre-acceptance estimate adoption: %w", err)
	}
	switch effect {
	case ports.PreAcceptanceEstimateAdoptionRegistered, ports.PreAcceptanceEstimateAdoptionReplay:
	case ports.PreAcceptanceEstimateAdoptionConflict:
		return effect, nil
	default:
		return 0, fmt.Errorf("register pre-acceptance estimate adoption: unknown registration effect %d", effect)
	}
	if err := handler.handoff.HandOffPreAcceptanceEstimateAdoptionRegistered(ctx, ports.PreAcceptanceEstimateAdoptionRegisteredIntent{
		Tenant:       command.Tenant,
		Account:      command.Adoption.Account(),
		RegisteredAt: registeredAt,
	}); err != nil {
		return 0, fmt.Errorf("register pre-acceptance estimate adoption: %w", err)
	}
	return effect, nil
}
