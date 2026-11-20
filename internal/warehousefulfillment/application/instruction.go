package application

import (
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// InstructionOutcome 是加工与换标指令的结果。账上不够整笔不入账，指令留在原状态。
type InstructionOutcome uint8

const (
	InstructionOutcomeInvalid InstructionOutcome = iota
	InstructionAccepted
	InstructionExisting
	InstructionConflict
	InstructionContentInvalid
	InstructionWarehouseNotFound
	InstructionWarehouseNotAccepting
	InstructionNotFound
	InstructionStatusMismatch
	InstructionWithdrawNotAllowed
	InstructionVoided
	InstructionPosted
	InstructionUnavailable
	InstructionNotConfigured
	InstructionCartonNotFound
)

func (outcome InstructionOutcome) String() string {
	switch outcome {
	case InstructionAccepted:
		return "ACCEPTED"
	case InstructionExisting:
		return "EXISTING_RESULT"
	case InstructionConflict:
		return "SOURCE_CONFLICT"
	case InstructionContentInvalid:
		return "CONTENT_INVALID"
	case InstructionWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case InstructionWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	case InstructionNotFound:
		return "INSTRUCTION_NOT_FOUND"
	case InstructionStatusMismatch:
		return "STATUS_MISMATCH"
	case InstructionWithdrawNotAllowed:
		return "WITHDRAW_NOT_ALLOWED"
	case InstructionVoided:
		return "VOIDED"
	case InstructionPosted:
		return "POSTED"
	case InstructionUnavailable:
		return "UNAVAILABLE"
	case InstructionNotConfigured:
		return "NOT_CONFIGURED"
	case InstructionCartonNotFound:
		return "CARTON_NOT_FOUND"
	default:
		return ""
	}
}

// InstructionResult 携带落点。指令在已受理、已有结果、已作废与已入账上有。
type InstructionResult struct {
	outcome     InstructionOutcome
	instruction domain.Instruction
	has         bool
	issues      []domain.InternalJobIssue
}

func (result InstructionResult) Outcome() InstructionOutcome { return result.outcome }
func (result InstructionResult) Instruction() (domain.Instruction, bool) {
	return result.instruction, result.has
}
func (result InstructionResult) Issues() []domain.InternalJobIssue {
	return append([]domain.InternalJobIssue(nil), result.issues...)
}

// AcceptInstructionCommand 是货主提交或运营发起一份指令。货主口的货主取自信封；运营发起的货主在载荷里。
type AcceptInstructionCommand struct {
	Tenant        domain.TenantID
	Owner         domain.OwnerAccountID
	Warehouse     domain.WarehouseCode
	Kind          domain.InstructionKind
	Origin        domain.InstructionOrigin
	Goods         domain.GoodsCode
	Unit          domain.GoodsUnitName
	Quantity      int64
	FromAlias     domain.GoodsAliasCode
	ToAlias       domain.GoodsAliasCode
	Batch         domain.BatchNumber
	Expiry        domain.ExpiryDate
	Reference     string
	RequestKey    domain.RequestKey
	DecisionID    domain.RequestKey
	Operator      domain.Registrant
	Reason        string
	Basis         string
	NamedRevision int
}

func (command AcceptInstructionCommand) spec(id domain.InstructionID, fixed int, components []domain.InstructionComponent, at time.Time) domain.InstructionSpec {
	return domain.InstructionSpec{
		Tenant: command.Tenant, ID: id, Owner: command.Owner, Warehouse: command.Warehouse, Kind: command.Kind,
		Origin: command.Origin, Goods: command.Goods, Unit: command.Unit, Quantity: command.Quantity,
		FromAlias: command.FromAlias, ToAlias: command.ToAlias, Batch: command.Batch, Expiry: command.Expiry,
		Reference: command.Reference, RequestKey: command.RequestKey, DecisionID: command.DecisionID,
		Operator: command.Operator, Reason: command.Reason, Basis: command.Basis, NamedRevision: command.NamedRevision,
		FixedRevision: fixed, Components: components, ReceivedAt: at,
	}
}

// InstructionExecutionLine 是执行事实的一行。加工的投入 Direction 取走，产出记入。换标只有取走，转入的别名取自指令。
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

// ExecuteInstructionCommand 是一次加工或换标执行。
type ExecuteInstructionCommand struct {
	Tenant         domain.TenantID
	FactID         domain.InternalJobFactID
	InstructionID  domain.InstructionID
	Device         domain.ReceiptDeviceID
	Operator       domain.ReceiptOperatorID
	OccurredAt     time.Time
	DeviceSequence int64
	Warehouse      domain.WarehouseCode
	DoneQuantity   int64
	Lines          []InstructionExecutionLine
}

// InstructionHandler 受理、撤回、作废并执行加工与换标。事务边界在调用方。
// 整笔成立才入账；账上不够不预支，事实不占身份，指令留在待加工或待换标。
type InstructionHandler struct {
	instructions ports.InstructionStore
	identities   ports.InstructionIdentities
	poster       *PostStockMovementHandler
	ledger       ports.StockLedger
	onHand       ports.StockOnHandRead
	warehouses   ports.WarehouseLookup
	locations    ports.StockLocationLookup
	zones        ports.ZoneLookup
	goods        ports.StockGoodsLookup
	revisions    ports.GoodsRevisionNumberRead
	owners       ports.OwnerAccountLookup
	holds        ports.StockHolds
	clock        ports.Clock
	anyWarehouse ports.TenantWarehouseLookup
	unboxes      ports.UnboxStore
	cartons      ports.CartonLedger
	cartonIDs    ports.CartonIdentities
	stockIDs     ports.StockIdentities
}

func NewInstructionHandler(
	instructions ports.InstructionStore,
	identities ports.InstructionIdentities,
	poster *PostStockMovementHandler,
	ledger ports.StockLedger,
	onHand ports.StockOnHandRead,
	warehouses ports.WarehouseLookup,
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
	goods ports.StockGoodsLookup,
	revisions ports.GoodsRevisionNumberRead,
	owners ports.OwnerAccountLookup,
	holds ports.StockHolds,
	clock ports.Clock,
	anyWarehouse ports.TenantWarehouseLookup,
	unboxes ports.UnboxStore,
	cartons ports.CartonLedger,
	cartonIDs ports.CartonIdentities,
	stockIDs ports.StockIdentities,
) *InstructionHandler {
	return &InstructionHandler{
		instructions: instructions, identities: identities, poster: poster, ledger: ledger, onHand: onHand,
		warehouses: warehouses, locations: locations, zones: zones, goods: goods, revisions: revisions,
		owners: owners, holds: holds, clock: clock, anyWarehouse: anyWarehouse,
		unboxes: unboxes, cartons: cartons, cartonIDs: cartonIDs, stockIDs: stockIDs,
	}
}
