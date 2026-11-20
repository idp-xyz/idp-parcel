package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// AllocationStrategy 是租户采用的那一版参考分配策略。没有这一行就是没采用。
type AllocationStrategy struct {
	Revision     int
	Operator     string
	RegisteredAt time.Time
}

// PickTask 是一条已经落到库位的拣货任务。数量按基本单位。
type PickTask struct {
	ID       domain.PickTaskID
	WaveID   domain.WaveID
	Request  domain.OutboundRequestID
	Location domain.LocationCode
	Goods    domain.GoodsCode
	Unit     domain.GoodsUnitName
	State    domain.StockState
	Inbound  domain.InboundBatchID
	Quantity int64
	Picked   int64
	Status   string
	Fact     string
}

// WaveStore 读写波次、分配策略与拣货任务。写入在没有环境事务时拒绝。
type WaveStore interface {
	LoadStrategy(ctx context.Context, tenant domain.TenantID) (AllocationStrategy, bool, error)
	SaveStrategy(ctx context.Context, tenant domain.TenantID, strategy AllocationStrategy) error
	LoadWave(ctx context.Context, tenant domain.TenantID, id domain.WaveID) ([]domain.OutboundRequestID, bool, error)
	SaveWave(ctx context.Context, tenant domain.TenantID, id domain.WaveID, warehouse domain.WarehouseCode, operator domain.Registrant, prefetch bool, at time.Time, requests []domain.OutboundRequestID, tasks []PickTask) error
	LoadTask(ctx context.Context, tenant domain.TenantID, id domain.PickTaskID) (PickTask, bool, error)
	SavePick(ctx context.Context, tenant domain.TenantID, task PickTask, requestStatus domain.OutboundStatus, differenceID, factID string, short int64) error
	TasksOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]PickTask, error)
	SaveCheck(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID, request domain.OutboundRequestID, observed int64, matched bool, at time.Time, next domain.OutboundStatus) error
	LoadCheck(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID) (observed int64, matched bool, found bool, err error)
	SavePack(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID, request domain.OutboundRequestID, parcels []PackParcel, at time.Time) error
	LoadPack(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID) (bool, error)
	ParcelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]PackParcel, error)
	MarkShipment(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, shipmentRequestID, declaredParcelID string) error
}

// PackParcel 是一个包裹的装箱记录。重量和尺寸按货主交来的字面留下。
type PackParcel struct {
	Sequence          int
	Packaging         string
	Weight            string
	WeightUnit        string
	Length            string
	Width             string
	Height            string
	Dimension         string
	Goods             domain.GoodsCode
	Unit              domain.GoodsUnitName
	Quantity          int64
	ShipmentRequestID string
	DeclaredParcelID  string
	CommissionState   string
	CorrectionState   string
}
