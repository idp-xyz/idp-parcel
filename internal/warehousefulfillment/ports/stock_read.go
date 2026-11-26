package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// StockCellBalance 是一格在库余额，带入库时间。余额由流水现算。
type StockCellBalance struct {
	Warehouse    string
	Owner        string
	Location     string
	Goods        string
	Unit         string
	State        string
	Alias        *string
	InboundBatch string
	BatchNumber  *string
	Expiry       *string
	InboundAt    time.Time
	Quantity     int64
}

// StockCartonBalance 是一箱在库的位置与入库时间。
type StockCartonBalance struct {
	Warehouse    string
	Location     string
	Carton       string
	InboundBatch string
	InboundAt    time.Time
}

// StockFreezeRow 是一份还冻着的冻结。原因与操作者只给运营面。
type StockFreezeRow struct {
	Warehouse    string
	Target       string
	Location     string
	Goods        string
	Unit         string
	State        string
	Carton       string
	InboundBatch string
	Quantity     int64
	Reason       string
	Operator     string
	FrozenAt     time.Time
}

// StockMovementRow 是一笔已入账的流水头。ID 只用来给翻页定位：它随游标交给运营面，不进答复里的行。
type StockMovementRow struct {
	ID        string
	Warehouse string
	Reason    string
	Source    string
	Occurred  time.Time
	Posted    time.Time
	Lines     []StockMovementLineRow
}

// StockMovementLineRow 是流水的一行增减。
type StockMovementLineRow struct {
	Location string
	Goods    string
	Unit     string
	State    string
	Delta    int64
}

// StockReadStore 读库存账、整箱账、冻结与流水。不写。
type StockReadStore interface {
	ListCellBalances(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]StockCellBalance, error)
	ListCartonBalances(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]StockCartonBalance, error)
	ListActiveFreezes(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]StockFreezeRow, error)
	// ListMovements 按 StockMovementPages 解出的查询取一页流水，带下一页的游标与同一组条件下的总数。
	ListMovements(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, limit int, query cataloguepage.Query) (cataloguepage.Result[StockMovementRow], error)
	CartonBill(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, carton domain.CartonNumber) ([]domain.CartonContentLine, error)
}
