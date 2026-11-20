package application

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// CountOutcome 是盘点各口的结果。盘点不改库存账。
type CountOutcome uint8

const (
	CountOutcomeInvalid CountOutcome = iota
	CountCreated
	CountExisting
	CountConflict
	CountContentInvalid
	CountWarehouseNotFound
	CountWarehouseNotAccepting
	CountNotConfigured
	CountStarted
	CountCannotStart
	CountNotFound
	CountStatusMismatch
	CountRecorded
	CountContentConflict
	CountCounted
	CountIncomplete
	CountVoided
)

func (outcome CountOutcome) String() string {
	switch outcome {
	case CountCreated:
		return "CREATED"
	case CountExisting:
		return "EXISTING_RESULT"
	case CountConflict:
		return "SOURCE_CONFLICT"
	case CountContentInvalid:
		return "CONTENT_INVALID"
	case CountWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case CountWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	case CountNotConfigured:
		return "NOT_CONFIGURED"
	case CountStarted:
		return "STARTED"
	case CountCannotStart:
		return "CANNOT_START"
	case CountNotFound:
		return "COUNT_NOT_FOUND"
	case CountStatusMismatch:
		return "STATUS_MISMATCH"
	case CountRecorded:
		return "RECORDED"
	case CountContentConflict:
		return "CONTENT_CONFLICT"
	case CountCounted:
		return "COUNTED"
	case CountIncomplete:
		return "INCOMPLETE"
	case CountVoided:
		return "VOIDED"
	default:
		return ""
	}
}

// CountBlock 是盘点不能开始的哪一种。
type CountBlock uint8

const (
	CountBlockInvalid CountBlock = iota
	CountBlockedByHold
	CountBlockedByOther
)

func (block CountBlock) String() string {
	switch block {
	case CountBlockedByHold:
		return "HOLD_CONFLICT"
	case CountBlockedByOther:
		return "ALREADY_COUNTING"
	default:
		return ""
	}
}

// CountResult 携带落点。不能开始时带库位。清点未齐时带还缺的库位。
type CountResult struct {
	outcome  CountOutcome
	count    domain.Count
	has      bool
	issues   []domain.FreezeIssue
	block    CountBlock
	location string
	missing  []string
	status   string
}

func (result CountResult) Outcome() CountOutcome       { return result.outcome }
func (result CountResult) Count() (domain.Count, bool) { return result.count, result.has }
func (result CountResult) Issues() []domain.FreezeIssue {
	return append([]domain.FreezeIssue(nil), result.issues...)
}
func (result CountResult) Block() CountBlock { return result.block }
func (result CountResult) Location() string  { return result.location }
func (result CountResult) Missing() []string { return append([]string(nil), result.missing...) }
func (result CountResult) Status() string    { return result.status }

// CreateCountCommand 是创建一份盘点。库位至少一个。货主与商品可以不限定。
type CreateCountCommand struct {
	Tenant     domain.TenantID
	DecisionID domain.RequestKey
	Warehouse  domain.WarehouseCode
	Locations  []domain.LocationCode
	Owner      domain.OwnerAccountID
	Goods      []domain.GoodsCode
	Note       string
	Operator   domain.Registrant
}

// CountCommand 是开始、完成或作废。作废可以带原因。
type CountCommand struct {
	Tenant   domain.TenantID
	ID       domain.CountID
	Operator domain.Registrant
	Reason   string
}

// RecordCountCommand 是一个库位的清点结果。
type RecordCountCommand struct {
	Tenant     domain.TenantID
	FactID     domain.InternalJobFactID
	CountID    domain.CountID
	Warehouse  domain.WarehouseCode
	Location   domain.LocationCode
	OccurredAt time.Time
	Sequence   int64
	Device     domain.ReceiptDeviceID
	Operator   domain.ReceiptOperatorID
	Lines      []ports.CountEntryLine
}

// CountHandler 创建、开始、清点、完成与作废盘点。
type CountHandler struct {
	counts      ports.CountStore
	identities  ports.CountIdentities
	differences ports.StockDiscrepancyIdentities
	warehouses  ports.WarehouseLookup
	locations   ports.StockLocationLookup
	zones       ports.ZoneLookup
	goods       ports.StockGoodsLookup
	holds       ports.StockHolds
	posting     interface {
		SerializePosting(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) error
	}
	clock        ports.Clock
	anyWarehouse ports.TenantWarehouseLookup
}

func NewCountHandler(
	counts ports.CountStore,
	identities ports.CountIdentities,
	differences ports.StockDiscrepancyIdentities,
	warehouses ports.WarehouseLookup,
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
	goods ports.StockGoodsLookup,
	holds ports.StockHolds,
	posting interface {
		SerializePosting(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) error
	},
	clock ports.Clock,
	anyWarehouse ports.TenantWarehouseLookup,
) *CountHandler {
	return &CountHandler{
		counts: counts, identities: identities, differences: differences, warehouses: warehouses,
		locations: locations, zones: zones, goods: goods, holds: holds, posting: posting,
		clock: clock, anyWarehouse: anyWarehouse,
	}
}
