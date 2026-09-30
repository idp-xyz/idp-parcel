package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ChargeAttributionRegister 写下一条费用项目的归属日判定。读口是 ChargeAttributionView。
// 时区与截单时刻写在这一行上，不预填。
type ChargeAttributionRegister interface {
	SaveChargeAttribution(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.ChargeAttributionRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// ChargeAttributionView 按费用项目取归属日判定。found=false 表示没有这一行：不形成归属日。
type ChargeAttributionView interface {
	LoadChargeAttribution(
		ctx context.Context,
		tenant domain.TenantID,
		feeItem domain.FeeItemReference,
	) (domain.ChargeAttributionRegistration, bool, error)
}
