package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// AdjustmentKind 是库存调整的一行：调增、调减或销毁。
type AdjustmentKind string

const (
	AdjustmentIncrease   AdjustmentKind = "INCREASE"
	AdjustmentDecrease   AdjustmentKind = "DECREASE"
	AdjustmentDestruction AdjustmentKind = "DESTRUCTION"
)

// AdjustmentLine 是库存调整的一行。销毁带货主授权的依据引用。
type AdjustmentLine struct {
	Kind     AdjustmentKind
	Location domain.LocationCode
	Goods    domain.GoodsCode
	Unit     domain.GoodsUnitName
	State    domain.StockState
	Alias    domain.GoodsAliasCode
	Batch    domain.BatchNumber
	Expiry   domain.ExpiryDate
	Inbound  domain.InboundBatchID
	Quantity int64
	Basis    string
}

// StockAdjustment 是一份已提交的库存调整。
type StockAdjustment struct {
	ID        domain.RequestKey
	Owner     domain.OwnerAccountID
	Warehouse domain.WarehouseCode
	Reason    string
	Operator  domain.Registrant
	Submitted time.Time
	Status    string
	Lines     []AdjustmentLine
}

// AdjustmentStore 保存库存调整。提交不改库存账。
type AdjustmentStore interface {
	Hold(ctx context.Context, tenant domain.TenantID, id domain.RequestKey) error
	Load(ctx context.Context, tenant domain.TenantID, id domain.RequestKey) (StockAdjustment, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, adjustment StockAdjustment) error
	MarkPosted(ctx context.Context, tenant domain.TenantID, id domain.RequestKey, operator domain.Registrant, at time.Time, note string, dutyRevision int) error
	MarkRejected(ctx context.Context, tenant domain.TenantID, id domain.RequestKey, operator domain.Registrant, at time.Time, reason string) error
}
