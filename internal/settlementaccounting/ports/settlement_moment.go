package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// SettlementMomentRegister 写下租户采用了确认或截单这一触发。读口是 SettlementMomentView。
// 不写钟点，不写账期。
type SettlementMomentRegister interface {
	SaveSettlementMoment(
		ctx context.Context,
		tenant domain.TenantID,
		moment domain.SettlementMoment,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// SettlementMomentView 按触发时点问这一租户有没有采用。found=false 表示没登记。
type SettlementMomentView interface {
	LoadSettlementMoment(
		ctx context.Context,
		tenant domain.TenantID,
		moment domain.SettlementMoment,
	) (bool, error)
}
