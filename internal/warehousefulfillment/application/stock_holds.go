package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ZeroStockHolds 在冻结、分配、作业锁定与预占还没有账时答零。
// 入账仍按「只取没被占着的部分」减这一口交回的数；接上占用账之后不用改入账。
type ZeroStockHolds struct{}

func (ZeroStockHolds) CellHold(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.StockCell) (int64, error) {
	return 0, nil
}

func (ZeroStockHolds) UnassignedReservation(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.GoodsCode, domain.StockState, domain.GoodsAliasCode, domain.BatchNumber, domain.ExpiryDate) (int64, error) {
	return 0, nil
}

func (ZeroStockHolds) CartonHeld(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.CartonNumber) (bool, error) {
	return false, nil
}

var _ ports.StockHolds = ZeroStockHolds{}
