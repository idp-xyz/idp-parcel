package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisterSettlementCatalogueHandler 把四本读口登记册的写入交给同一只时钟。落库时刻不是载荷的一格。
type RegisterSettlementCatalogueHandler struct {
	authorities ports.SupplierAuditAuthorityRegister
	accounts    ports.SupplierPayableAccountRegister
	rules       ports.ClaimAmountRuleRegister
	facts       ports.ChargeConfirmationFactRegister
	clock       ports.Clock
}

func NewRegisterSettlementCatalogueHandler(
	authorities ports.SupplierAuditAuthorityRegister,
	accounts ports.SupplierPayableAccountRegister,
	rules ports.ClaimAmountRuleRegister,
	facts ports.ChargeConfirmationFactRegister,
	clock ports.Clock,
) (*RegisterSettlementCatalogueHandler, error) {
	if authorities == nil || accounts == nil || rules == nil || facts == nil || clock == nil {
		return nil, fmt.Errorf("%w: settlement catalogue", ErrNilDependency)
	}
	return &RegisterSettlementCatalogueHandler{
		authorities: authorities,
		accounts:    accounts,
		rules:       rules,
		facts:       facts,
		clock:       clock,
	}, nil
}

func (handler *RegisterSettlementCatalogueHandler) RegisterSupplierAuditAuthority(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.SupplierAuditAuthorityRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: supplier audit authority", domain.ErrBlankValue)
	}
	effect, err := handler.authorities.SaveSupplierAuditAuthority(ctx, tenant, registration, handler.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("register supplier audit authority: %w", err)
	}
	return effect, nil
}

func (handler *RegisterSettlementCatalogueHandler) RegisterSupplierPayableAccount(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.SupplierPayableAccountRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: supplier payable account", domain.ErrBlankValue)
	}
	effect, err := handler.accounts.SaveSupplierPayableAccount(ctx, tenant, registration, handler.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("register supplier payable account: %w", err)
	}
	return effect, nil
}

func (handler *RegisterSettlementCatalogueHandler) RegisterClaimAmountRule(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.ClaimAmountRuleRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: claim amount rule", domain.ErrBlankValue)
	}
	effect, err := handler.rules.SaveClaimAmountRule(ctx, tenant, registration, handler.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("register claim amount rule: %w", err)
	}
	return effect, nil
}

func (handler *RegisterSettlementCatalogueHandler) RegisterChargeConfirmationFacts(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.ChargeConfirmationFactRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: charge confirmation facts", domain.ErrBlankValue)
	}
	effect, err := handler.facts.SaveChargeConfirmationFacts(ctx, tenant, registration, handler.clock.Now())
	if err != nil {
		return 0, fmt.Errorf("register charge confirmation facts: %w", err)
	}
	return effect, nil
}
