package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// PeriodicFeeRegister 写下一种周期费用形态及其数值。读口是 PeriodicFeeView。
type PeriodicFeeRegister interface {
	SavePeriodicFee(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.PeriodicFeeRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// PeriodicFeeView 按规则引用取周期费用形态。found=false 表示没有这一行：
// 不形成最低消费补差、保底量金额或返利，也不默认按零。
type PeriodicFeeView interface {
	LoadPeriodicFee(
		ctx context.Context,
		tenant domain.TenantID,
		rule domain.PeriodicFeeRuleReference,
	) (domain.PeriodicFeeTerms, bool, error)
}
