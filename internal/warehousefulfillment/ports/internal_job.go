package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// InternalJobPages 是运营查阅库内作业事实的查询声明（UC-WF-007）。
// 筛选维覆盖仓与作业种类。
var InternalJobPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-internal-jobs",
	Sorts: []cataloguepage.SortDimension{
		{Name: "receivedAt", Kind: cataloguepage.Instant},
		{Name: "factId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "receivedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "warehouseCode"},
		{Name: "kind", Vocabulary: []string{"INTERNAL_MOVE", "FORM_CONVERSION", "CONDITION_CONVERSION", "SHORTAGE"}},
	},
})

// InternalJobStore 是库内作业事实的册。HoldFact 串行同一事实身份；没有环境事务时拒绝。
// 作业内容不合与仓不受理不落册，不占事实身份。
type InternalJobStore interface {
	HoldFact(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) error
	Load(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) (domain.RecordedInternalJob, bool, error)
	Save(ctx context.Context, job domain.RecordedInternalJob, discrepancies []domain.StockDiscrepancy) error
	List(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[domain.RecordedInternalJob], error)
}

// StoredJobDiscrepancy 是一份库内作业留下的待处理差异，带形成它的作业种类和操作者。
type StoredJobDiscrepancy struct {
	Tenant        domain.TenantID
	ID            domain.StockDiscrepancyID
	FactID        domain.InternalJobFactID
	Kind          string
	Operator      string
	Owner         domain.OwnerAccountID
	Warehouse     domain.WarehouseCode
	Location      domain.LocationCode
	Source        domain.LocationCode
	Goods         domain.GoodsCode
	Unit          domain.GoodsUnitName
	State         domain.StockState
	Alias         domain.GoodsAliasCode
	Batch         domain.BatchNumber
	Expiry        domain.ExpiryDate
	Inbound       domain.InboundBatchID
	Quantity      int64
	Carton        domain.CartonNumber
	CartonMissing bool
	Status        string
}

// JobDiscrepancyStore 读写库内作业形成的库存差异。
type JobDiscrepancyStore interface {
	LoadDiscrepancy(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID) (StoredJobDiscrepancy, bool, error)
	MarkDiscrepancyApproved(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID, operator domain.Registrant, at time.Time, note string, dutyRevision int) error
	MarkDiscrepancyRejected(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID, operator domain.Registrant, at time.Time, reason string) error
}

// ConditionRestoreStore 是次品转良品的册。不成立的决定不落册，不占决定标识。
type ConditionRestoreStore interface {
	HoldDecision(ctx context.Context, tenant domain.TenantID, id domain.ConditionRestoreID) error
	LoadDecision(ctx context.Context, tenant domain.TenantID, id domain.ConditionRestoreID) (domain.ConditionRestore, bool, error)
	SaveDecision(ctx context.Context, restore domain.ConditionRestore) error
}

// StockDiscrepancyIdentities 签发库存差异的标识。只在决定落册时取。
type StockDiscrepancyIdentities interface {
	NextStockDiscrepancyID() (domain.StockDiscrepancyID, error)
}

// CartonPlace 是一箱此刻所在的库位与它沿用的入库批次。Found 为假是账上没有这一箱。
type CartonPlace struct {
	Location domain.LocationCode
	Batch    domain.CartonInboundBatchID
	Found    bool
}

// StockHolds 回答库内入账取不走的数量。冻结与盘点锁定按格，未分配的预占按出库请求的条件。
// 分配还没有账。取不走的减法已经写在入账里。
type StockHolds interface {
	CellHold(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		warehouse domain.WarehouseCode,
		cell domain.StockCell,
	) (int64, error)
	UnassignedReservation(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		warehouse domain.WarehouseCode,
		goods domain.GoodsCode,
		state domain.StockState,
		alias domain.GoodsAliasCode,
		batch domain.BatchNumber,
		expiry domain.ExpiryDate,
	) (int64, error)
	CartonHeld(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		warehouse domain.WarehouseCode,
		carton domain.CartonNumber,
	) (bool, error)
}
