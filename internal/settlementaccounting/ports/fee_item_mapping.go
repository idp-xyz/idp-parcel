package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// FeeItemMappingView 是费用项目映射册的只读口：按（租户、计价费用代码）交回那一行；没登记答 false。SELL 消费门据此
// 答待判断（AT-SA-174），不拿价格代码当费用项目、不建占位费用（ADR-0190 决定五）。
type FeeItemMappingView interface {
	FindFeeItemMapping(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.PricingChargeCode,
	) (domain.FeeItemMapping, bool, error)
}

// FeeItemMappingRegister 是映射册的写口，只增不改：同一代码同一份答重放，内容不同答冲突、不覆盖。
type FeeItemMappingRegister interface {
	SaveFeeItemMapping(
		ctx context.Context,
		tenant domain.TenantID,
		mapping domain.FeeItemMapping,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}
