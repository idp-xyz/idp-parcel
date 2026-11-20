package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// InstructionStore 是加工与换标指令的册。加锁与写入在没有环境事务时拒绝。
// 作业内容不合与库存不可用不落执行事实，不占事实身份。
type InstructionStore interface {
	HoldSubmission(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) error
	LoadBySubmission(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.Instruction, bool, error)
	HoldDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) error
	LoadByDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.Instruction, bool, error)
	LoadByWithdrawal(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.Instruction, bool, error)
	HoldInstruction(ctx context.Context, tenant domain.TenantID, id domain.InstructionID) error
	Load(ctx context.Context, tenant domain.TenantID, id domain.InstructionID) (domain.Instruction, bool, error)
	Save(ctx context.Context, instruction domain.Instruction) error
	MarkVoid(ctx context.Context, instruction domain.Instruction) error
	HoldExecution(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) error
	LoadExecution(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) (InstructionExecution, bool, error)
	SaveExecution(ctx context.Context, execution InstructionExecution) error
}

// InstructionExecution 是一次已经入账的执行，挂在指令上。
type InstructionExecution struct {
	Tenant         domain.TenantID
	FactID         domain.InternalJobFactID
	InstructionID  domain.InstructionID
	Device         domain.ReceiptDeviceID
	Operator       domain.ReceiptOperatorID
	OccurredAt     time.Time
	DeviceSequence int64
	ReceivedAt     time.Time
	DoneQuantity   int64
	MovementID     domain.StockMovementID
	Lines          []InstructionExecutionLine
}

// InstructionExecutionLine 是执行事实落册的一行。
type InstructionExecutionLine struct {
	Direction  domain.StockDirection
	Location   domain.LocationCode
	Goods      domain.GoodsCode
	Unit       domain.GoodsUnitName
	State      domain.StockState
	Alias      domain.GoodsAliasCode
	Batch      domain.BatchNumber
	Expiry     domain.ExpiryDate
	NamedBatch domain.InboundBatchID
	Quantity   int64
}

// InstructionIdentities 签发指令标识。
type InstructionIdentities interface {
	NextInstructionID() (domain.InstructionID, error)
}

// UnboxStore 是开箱事实的册。箱不在库、被占着与作业内容不合都不落册。
type UnboxStore interface {
	HoldUnbox(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) error
	LoadUnbox(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) (UnboxFact, bool, error)
	SaveUnbox(ctx context.Context, fact UnboxFact) error
	CartonBill(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, carton domain.CartonNumber) ([]domain.CartonContentLine, error)
}

// UnboxFact 是已经入账的一次开箱。
type UnboxFact struct {
	Tenant         domain.TenantID
	FactID         domain.InternalJobFactID
	Owner          domain.OwnerAccountID
	Warehouse      domain.WarehouseCode
	Carton         domain.CartonNumber
	Device         domain.ReceiptDeviceID
	Operator       domain.ReceiptOperatorID
	OccurredAt     time.Time
	DeviceSequence int64
	ReceivedAt     time.Time
	MovementID     domain.StockMovementID
	Lines          []UnboxLine
	Differences    []UnboxDifference
}

// UnboxLine 是开箱实点的一行。认不出的货只有 Observed。
type UnboxLine struct {
	Observed string
	Goods    domain.GoodsCode
	Unit     domain.GoodsUnitName
	State    domain.StockState
	Alias    domain.GoodsAliasCode
	Batch    domain.BatchNumber
	Expiry   domain.ExpiryDate
	Location domain.LocationCode
	Quantity int64
}

// UnboxDifference 是实点与箱单的一笔差额。
type UnboxDifference struct {
	Kind     domain.UnboxDifferenceKind
	Quantity int64
	Goods    domain.GoodsCode
	Observed string
}
