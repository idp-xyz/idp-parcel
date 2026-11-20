package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// FreezeStore 保存冻结。决定标识先锁再认重放。没有环境事务时拒绝。
type FreezeStore interface {
	HoldDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) error
	LoadByDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.Freeze, bool, error)
	HoldFreeze(ctx context.Context, tenant domain.TenantID, id domain.FreezeID) error
	Load(ctx context.Context, tenant domain.TenantID, id domain.FreezeID) (domain.Freeze, bool, error)
	Save(ctx context.Context, freeze domain.Freeze) error
	MarkUnfrozen(ctx context.Context, freeze domain.Freeze) error
	CellFreeze(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, cell domain.StockCell) (int64, error)
	SummaryFreeze(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, goods domain.GoodsCode, unit domain.GoodsUnitName, state domain.StockState) (int64, error)
	CartonFrozen(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, carton domain.CartonNumber) (bool, error)
}

// FreezeIdentities 签发冻结标识。只在决定落册时取。
type FreezeIdentities interface {
	NextFreezeID() (domain.FreezeID, error)
}
