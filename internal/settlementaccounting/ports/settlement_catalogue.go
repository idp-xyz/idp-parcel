package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// CatalogueRegistrationEffect 是四本结算读口登记册共用的落点。重放是同一键、同一内容再交一次；
// 冲突是同一键不同内容。两种都不改已落的行。
type CatalogueRegistrationEffect uint8

const (
	CatalogueRegistered CatalogueRegistrationEffect = iota + 1
	CatalogueReplay
	CatalogueConflict
)

// SupplierAuditAuthorityRegister 是审核授权册的写口。读口是 SupplierAuditAuthorityView。
type SupplierAuditAuthorityRegister interface {
	SaveSupplierAuditAuthority(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.SupplierAuditAuthorityRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// SupplierPayableAccountRegister 是供应商应付账户查问册的写口。读口是 SupplierPayableAccountView。
type SupplierPayableAccountRegister interface {
	SaveSupplierPayableAccount(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.SupplierPayableAccountRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// ClaimAmountRuleRegister 是索赔金额规则册的写口。读口是 ClaimAmountRuleView。
type ClaimAmountRuleRegister interface {
	SaveClaimAmountRule(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.ClaimAmountRuleRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// ChargeConfirmationFactRegister 是确认事实册的写口。读口是 ConfirmedChargeFactsView。
type ChargeConfirmationFactRegister interface {
	SaveChargeConfirmationFacts(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.ChargeConfirmationFactRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}
