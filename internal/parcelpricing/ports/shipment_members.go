package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// ShipmentMemberPicture 是一件包裹此刻所属的已接受委托，以及这份委托当前仍然有效的成员。
// 取消了的成员不在 Members 里。顺序是委托的当前声明顺序。
type ShipmentMemberPicture struct {
	Shipment string
	Members  []string
}

// ShipmentMemberView 按包裹读它所属的委托和当前有效成员。正式销售评价在有序列表上、包裹级请求撞上逐委托卡时，
// 用这份清单探整票的基础运费表，不另编一件清单。读不到委托时第二个返回值为假。error 只表示读面坏了。
type ShipmentMemberView interface {
	LoadShipmentMembers(
		ctx context.Context,
		tenant domain.TenantID,
		parcel string,
	) (ShipmentMemberPicture, bool, error)
}
