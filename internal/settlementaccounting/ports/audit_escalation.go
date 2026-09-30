package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// AuditEscalationCeilingRegister 写下越权升级的金额上限。读口是 AuditEscalationCeilingView。
// 审核授权册只交审核人，上限不写回去。
type AuditEscalationCeilingRegister interface {
	SaveAuditEscalationCeiling(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.AuditEscalationCeilingRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// AuditEscalationCeilingView 取供应商、责任法人、币种上的金额上限。found=false 表示
// 没有这一行：审核不形成应付，也不默认放行。
type AuditEscalationCeilingView interface {
	LoadAuditEscalationCeiling(
		ctx context.Context,
		tenant domain.TenantID,
		supplier domain.SupplierPartyReference,
		legalEntity domain.LegalEntityReference,
		currency domain.CurrencyCode,
	) (domain.AuditEscalationCeiling, bool, error)
}
