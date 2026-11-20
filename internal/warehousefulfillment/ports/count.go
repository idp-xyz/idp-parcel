package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// CountEntryLine 是清点结果的一行。认不出的货只填 Observed，不成差异。
type CountEntryLine struct {
	Owner    domain.OwnerAccountID
	Goods    domain.GoodsCode
	Unit     domain.GoodsUnitName
	State    domain.StockState
	Alias    domain.GoodsAliasCode
	Batch    domain.BatchNumber
	Expiry   domain.ExpiryDate
	Quantity int64
	Observed string
}

// CountEntry 是一个库位的一份清点结果。
type CountEntry struct {
	Tenant     domain.TenantID
	FactID     domain.InternalJobFactID
	CountID    domain.CountID
	Location   domain.LocationCode
	OccurredAt time.Time
	Sequence   int64
	Device     domain.ReceiptDeviceID
	Operator   domain.ReceiptOperatorID
	ReceivedAt time.Time
	Lines      []CountEntryLine
}

// CountDifference 是完成时一格实点与账面的差额。不比入库批次。账面与实点都按基本比较键合计。
type CountDifference struct {
	ID       domain.StockDiscrepancyID
	Owner    domain.OwnerAccountID
	Location domain.LocationCode
	Goods    domain.GoodsCode
	Unit     domain.GoodsUnitName
	State    domain.StockState
	Alias    domain.GoodsAliasCode
	Batch    domain.BatchNumber
	Expiry   domain.ExpiryDate
	Book     int64
	Counted  int64
}

// CountStore 保存盘点、作业锁定、清点结果与差额。没有环境事务时拒绝写入。
type CountStore interface {
	HoldDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) error
	LoadByDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.Count, bool, error)
	HoldCount(ctx context.Context, tenant domain.TenantID, id domain.CountID) error
	Load(ctx context.Context, tenant domain.TenantID, id domain.CountID) (domain.Count, bool, error)
	Save(ctx context.Context, count domain.Count) error
	SaveStart(ctx context.Context, count domain.Count, locks []domain.CountLock) error
	SaveEntry(ctx context.Context, entry CountEntry) error
	LoadEntries(ctx context.Context, tenant domain.TenantID, id domain.CountID) ([]CountEntry, error)
	SaveFinish(ctx context.Context, count domain.Count, differences []CountDifference) error
	SaveVoid(ctx context.Context, count domain.Count) error
	LoadDifference(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID) (StoredCountDifference, bool, error)
	MarkApproved(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID, operator domain.Registrant, at time.Time, note string, dutyRevision int) error
	MarkRejected(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID, operator domain.Registrant, at time.Time, reason string) error
	CellLock(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, cell domain.StockCell) (int64, error)
	CountingOverlap(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, location domain.LocationCode, owner domain.OwnerAccountID, goods domain.GoodsCode) (bool, error)
	LocationBalances(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, locations []domain.LocationCode) ([]StockCellBalance, error)
}

// StoredCountDifference 是一份已落册的盘点差额，带状态。
type StoredCountDifference struct {
	CountDifference
	Tenant    domain.TenantID
	Warehouse domain.WarehouseCode
	CountID   domain.CountID
	Status    string
}

// ApprovalDuty 是一个已登记的批准职责规则修订。
type ApprovalDuty struct {
	Revision     int
	Distinct     bool
	Operator     domain.Registrant
	RegisteredAt time.Time
}

// ApprovalDutyStore 保存批准职责规则。没登记时 Load 的第二个返回值是假。
type ApprovalDutyStore interface {
	SaveDuty(ctx context.Context, tenant domain.TenantID, duty ApprovalDuty) error
	LoadDuty(ctx context.Context, tenant domain.TenantID) (ApprovalDuty, bool, error)
}

// CountIdentities 签发盘点标识。差额沿用库存差异的签发器。
type CountIdentities interface {
	NextCountID() (domain.CountID, error)
}
