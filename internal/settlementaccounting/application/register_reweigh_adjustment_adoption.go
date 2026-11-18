package application

import (
	"context"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterReweighAdjustmentAdoptionHandler 登记一户采用「实测差异自动调整」，并交「评价参数已登记」。
// 停在没采用上的销售请求由这一封续办。谁采用是租户取值，本口不预选。
type RegisterReweighAdjustmentAdoptionHandler struct {
	adoptions ports.ReweighAdjustmentAdoptionRegister
	handoff   ports.EvaluationParameterRegisteredHandoff
	clock     ports.Clock
}

func NewRegisterReweighAdjustmentAdoptionHandler(
	adoptions ports.ReweighAdjustmentAdoptionRegister,
	handoff ports.EvaluationParameterRegisteredHandoff,
	clock ports.Clock,
) (*RegisterReweighAdjustmentAdoptionHandler, error) {
	if adoptions == nil || handoff == nil || clock == nil {
		return nil, fmt.Errorf("%w: reweigh adjustment adoption", ErrNilDependency)
	}
	return &RegisterReweighAdjustmentAdoptionHandler{adoptions: adoptions, handoff: handoff, clock: clock}, nil
}

func (handler *RegisterReweighAdjustmentAdoptionHandler) RegisterReweighAdjustmentAdoption(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
	basis string,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" || account.String() == "" || strings.TrimSpace(basis) == "" {
		return 0, fmt.Errorf("%w: reweigh adjustment adoption", domain.ErrBlankValue)
	}
	registeredAt := handler.clock.Now()
	effect, err := handler.adoptions.SaveReweighAdjustmentAdoption(ctx, tenant, account, strings.TrimSpace(basis), registeredAt)
	if err != nil {
		return 0, fmt.Errorf("register reweigh adjustment adoption: %w", err)
	}
	if err := announceEvaluationParameter(ctx, handler.handoff, effect, ports.EvaluationParameterRegisteredIntent{
		Tenant:       tenant,
		Kind:         domain.ParameterReweighAdjustmentAdoption,
		Subject:      account.String(),
		Account:      account,
		RegisteredAt: registeredAt,
	}); err != nil {
		return 0, fmt.Errorf("register reweigh adjustment adoption: %w", err)
	}
	return effect, nil
}
