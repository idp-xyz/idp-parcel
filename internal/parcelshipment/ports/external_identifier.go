package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ParcelExternalIdentifierView 按（租户，号码）答此刻适用、被标识对象为包裹的外部标识关联所指的包裹（ADR-0184
// 决定五）。号码逐字比对；命中多件即交回多件、去重，不按形成时间、来源或分配方挑一件——消费方据候选数判身份。
// 只交回包裹，不交分配方、客户账户或委托（CONTEXT-MAP `parcel-shipment → node-operations`）。查无交空；依赖读不动
// 作为错误返回，空与「没查到」不是一回事。
type ParcelExternalIdentifierView interface {
	FindParcelsByExternalIdentifier(
		ctx context.Context,
		tenant domain.TenantID,
		number domain.ExternalIdentifierNumber,
	) ([]domain.DeclaredParcelID, error)
}
