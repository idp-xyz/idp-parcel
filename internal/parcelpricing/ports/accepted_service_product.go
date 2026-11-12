package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// AcceptedServiceProductView 是正式 SELL·客户费用评价读委托申报产品的消费侧口（ADR-0382 决定三）：按（租户、包裹）答覆盖这件包裹的
// 已接受委托申报的服务产品。零值即没有——没有已接受委托覆盖它，或委托没申报；两种在选卡上是同一个答案，只看不声明产品的卡。error
// 只表示读面坏了。包裹是 parcel-shipment 的声明包裹标识原样，与评价请求上点名的那一件同一个。
//
// 产品取委托的申报，不取商业解析（ADR-0260 决定一）：它是事实，进计价输入快照的那一格；商业依据不进。
type AcceptedServiceProductView interface {
	LoadAcceptedServiceProduct(ctx context.Context, tenant domain.TenantID, parcel string) (domain.ServiceProductReference, error)
}
