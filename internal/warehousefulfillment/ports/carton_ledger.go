package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// CartonLedger 是整箱账。余额不存，由流水现算。写入只追加。
//
// Serialize 锁（租户，货主，仓）的整箱账。这把锁与商品库存的货主锁不是同一把，键里写明是整箱。
// 没有环境事务时拒绝：锁在语句结束就放掉，等于没锁。
// OnHandCartons 交回余额为正的箱号。Append 在锁里再看这些箱号还在不在库，然后写入流水与入库批次。
type CartonLedger interface {
	Serialize(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) error
	OnHandCartons(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]domain.CartonNumber, error)
	Append(ctx context.Context, movement domain.CartonMovement, formed []domain.CartonInboundBatch) error
}

// CartonIdentities 签发整箱流水与整箱入库批次的标识。两类各一个签发器，不并进商品库存的签发器。
type CartonIdentities interface {
	NextCartonMovementID() (domain.CartonMovementID, error)
	NextCartonInboundBatchID() (domain.CartonInboundBatchID, error)
}
