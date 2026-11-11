package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// AcceptedServiceProductView 按（租户，声明包裹身份）答覆盖它的已接受委托申报的服务产品（ADR-0378 决定三）。消费方是 `parcel-pricing`
// 正式 SELL·客户费用评价的消费侧适配器：评价点名包裹，选售价卡要知道委托申报的是哪个产品。它是事实读口，不是商业依据——产品取
// 委托的申报，不取商业解析（ADR-0260 决定一「计价输入快照不带商业依据」）。
//
// 另开一口而不拓宽 CommercialResolutionReferenceView：那一口答的是商业解析回指，合成一个宽口就是预设消费方同时要两样（ADR-0130、
// ADR-0133 读口一口一问）。内部与其余按（租户，包裹）答问的读口走同一条取行 + 重建的路。
//
// 两格：产品（Declared 为真）/ 没有产品（零值、error 为空——包裹不属任何已接受委托的成员集合，或那一版没申报；按统一不可见结果，
// 不区分不存在、他租户与未授权）。error 只表示读面坏了或两份已接受委托同时声明这件包裹（歧义）。
type AcceptedServiceProductView interface {
	LoadAcceptedServiceProduct(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.DeclaredServiceProduct, error)
}
