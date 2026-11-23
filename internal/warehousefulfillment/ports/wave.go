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
	Staging  domain.LocationCode
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
	LoadConfirmation(ctx context.Context, tenant domain.TenantID, factID string) (OutboundConfirmation, bool, error)
	SaveConfirmation(ctx context.Context, tenant domain.TenantID, confirmation OutboundConfirmation) error
	ParcelConfirmed(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (bool, error)
	LoadOwnerLabel(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (string, bool, error)
	SaveOwnerLabel(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, externalNumber string) error
	OwnerLabelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (map[int]string, error)
	LoadCorrection(ctx context.Context, tenant domain.TenantID, factID string) (ConfirmationCorrection, bool, error)
	SaveCorrection(ctx context.Context, tenant domain.TenantID, correction ConfirmationCorrection) error
	PostReplenishment(ctx context.Context, tenant domain.TenantID, factID string, approver domain.Registrant, dutyRevision int, at time.Time) error
	LoadCutoff(ctx context.Context, tenant domain.TenantID, key string) (requestID string, sequence int, found bool, err error)
	SaveCutoff(ctx context.Context, tenant domain.TenantID, key, requestID string, sequence int, basis, outcome string, at time.Time) error
	ReleaseReservations(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) error
	CancelOpenTasks(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) error
	SaveReturnTask(ctx context.Context, tenant domain.TenantID, task ReturnTask) error
	SetRequestStatus(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, status domain.OutboundStatus) error
	MarkParcelCutoff(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (domain.OutboundStatus, error)
	SaveLatePick(ctx context.Context, tenant domain.TenantID, task PickTask, factID string) error
	LoadReturnTask(ctx context.Context, tenant domain.TenantID, id string) (ReturnTask, bool, error)
	CompleteReturn(ctx context.Context, tenant domain.TenantID, id, factID string) error
	OpenReturns(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (int, error)
}

// ReturnTask 是截单后把已拣出的货移回可分配库区的任务。货还在待出库时它只是任务。
type ReturnTask struct {
	ID       string
	Request  domain.OutboundRequestID
	Sequence int
	Location domain.LocationCode
	Goods    domain.GoodsCode
	Unit     domain.GoodsUnitName
	State    domain.StockState
	Inbound  domain.InboundBatchID
	Quantity int64
	Status   string
	Fact     string
}

// ConfirmationCorrection 是对一版出库确认的一次更正。一版只有这一次。
type ConfirmationCorrection struct {
	FactID       string
	Kind         string
	OccurredAt   time.Time
	Operator     string
	Status       string
	DutyRevision int
	Approver     string
}

// OutboundConfirmation 是一次已经记下的出库确认或自提交付。
type OutboundConfirmation struct {
	FactID           string
	Request          domain.OutboundRequestID
	Sequence         int
	Kind             string
	Offline          bool
	Missing          string
	ExternalNumber   string
	PickupParty      string
	PickupCredential string
	OccurredAt       time.Time
	State            string
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
