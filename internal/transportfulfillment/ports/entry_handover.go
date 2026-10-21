package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// EntryHandoverView 按（租户、对象、交接版本）找回`已交接`的那几版，供门禁读数找对象进段的那次交接：交付读数取它声明的交出节点
// 作 at（ADR-0213 决定五），出发读数核它声明的交出、去向节点就是班次的起讫（ADR-0237 决定五）。
//
// ADR-0221 之前的存量参与，入场依据只带版本、不带交接范围，所以这一问不按幂等键问：同一对象在不同范围用了同一个版本号时如实全
// 交回，哪一版是进段那一次由领域（FulfillmentParticipation.EnteredByHandover）认，本口不挑；按交接键铸的依据在那一处只认得出
// 范围相同的一版。拒收与待确认不交回——它们立不起参与。
type EntryHandoverView interface {
	FindHandedOverByVersion(
		ctx context.Context,
		tenant domain.TenantID,
		object domain.CarriedObjectReference,
		version domain.HandoverResultVersion,
	) ([]domain.TransportHandover, error)
}
