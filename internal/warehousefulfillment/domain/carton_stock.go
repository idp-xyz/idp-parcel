package domain

import (
	"slices"
	"time"
)

// 整箱库存与商品库存分开记、分开守恒（CONTEXT「整箱库存」「库存账与守恒」）。
// 一箱是一件，余额只由流水的增减现算，不另存可改写的余额。已入账的流水不改不删。
// 箱号在（租户，货主，仓）内只在余额为正的期间唯一。整箱不分库存状态。

// CartonMovementID 是整箱流水的标识，由整箱账签发。不与商品库存流水共用一个类型。
type CartonMovementID struct{ requiredValue }

func NewCartonMovementID(value string) (CartonMovementID, error) {
	required, err := newRequiredValue("carton movement ID", value)
	return CartonMovementID{required}, err
}

// CartonInboundBatchID 是整箱入库批次的标识，由整箱账签发。不与商品入库批次共用一个类型。
type CartonInboundBatchID struct{ requiredValue }

func NewCartonInboundBatchID(value string) (CartonInboundBatchID, error) {
	required, err := newRequiredValue("carton inbound batch ID", value)
	return CartonInboundBatchID{required}, err
}

// CartonMovementReason 是整箱流水的原因。收货写 +1；库内移动同一箱写 -1 与 +1，不新起入库批次。
type CartonMovementReason uint8

const (
	CartonMovementReasonInvalid CartonMovementReason = iota
	CartonMovementReceipt
	CartonMovementInternalMove
)

func (reason CartonMovementReason) String() string {
	switch reason {
	case CartonMovementReceipt:
		return "RECEIPT"
	case CartonMovementInternalMove:
		return "INTERNAL_MOVE"
	default:
		return ""
	}
}

// CartonStockZoneAllowed 回答整箱库存能不能停在这种库区。
// 只许收货暂存、存储与待出库。收货这一段只往收货暂存入账，调用方再收窄。
func CartonStockZoneAllowed(usage ZoneUsage) bool {
	switch usage {
	case ZoneUsageReceivingStaging, ZoneUsageStorage, ZoneUsageOutboundStaging:
		return true
	default:
		return false
	}
}

// CartonMovementLine 是整箱流水的一行。Carton 是入账的箱号。
// OwnerCarton 只在仓另编了箱号时有值，记货主原来的箱号；没另编就是零值。
type CartonMovementLine struct {
	location    LocationCode
	carton      CartonNumber
	ownerCarton CartonNumber
	batch       CartonInboundBatchID
	delta       int64
}

func NewCartonMovementLine(
	location LocationCode,
	carton CartonNumber,
	ownerCarton CartonNumber,
	batch CartonInboundBatchID,
) (CartonMovementLine, error) {
	return newCartonMovementLine(location, carton, ownerCarton, batch, 1)
}

// NewCartonMovementDeltaLine 是整箱移动的一行。移出记 -1，移入记 +1，入库批次沿用账上的那一个。
func NewCartonMovementDeltaLine(
	location LocationCode,
	carton CartonNumber,
	batch CartonInboundBatchID,
	delta int64,
) (CartonMovementLine, error) {
	return newCartonMovementLine(location, carton, CartonNumber{}, batch, delta)
}

func newCartonMovementLine(
	location LocationCode,
	carton CartonNumber,
	ownerCarton CartonNumber,
	batch CartonInboundBatchID,
	delta int64,
) (CartonMovementLine, error) {
	if !location.valid() || !carton.valid() || !batch.valid() || (delta != 1 && delta != -1) {
		return CartonMovementLine{}, ErrInvalidCartonStock
	}
	if ownerCarton.valid() && ownerCarton == carton {
		return CartonMovementLine{}, ErrInvalidCartonStock
	}
	return CartonMovementLine{location: location, carton: carton, ownerCarton: ownerCarton, batch: batch, delta: delta}, nil
}

func (line CartonMovementLine) Delta() int64                { return line.delta }
func (line CartonMovementLine) Location() LocationCode      { return line.location }
func (line CartonMovementLine) Carton() CartonNumber        { return line.carton }
func (line CartonMovementLine) OwnerCarton() CartonNumber   { return line.ownerCarton }
func (line CartonMovementLine) Batch() CartonInboundBatchID { return line.batch }

// CartonInboundBatch 是一次收货为一箱形成的入库批次。同一事实的多箱各一个，按入账的箱号区分。
type CartonInboundBatch struct {
	tenant     TenantID
	id         CartonInboundBatchID
	owner      OwnerAccountID
	warehouse  WarehouseCode
	carton     CartonNumber
	inboundAt  time.Time
	movementID CartonMovementID
}

func NewCartonInboundBatch(
	tenant TenantID,
	id CartonInboundBatchID,
	owner OwnerAccountID,
	warehouse WarehouseCode,
	carton CartonNumber,
	inboundAt time.Time,
	movementID CartonMovementID,
) (CartonInboundBatch, error) {
	if !tenant.valid() || !id.valid() || !owner.valid() || !warehouse.valid() || !carton.valid() || !movementID.valid() || inboundAt.IsZero() {
		return CartonInboundBatch{}, ErrInvalidCartonStock
	}
	return CartonInboundBatch{
		tenant: tenant, id: id, owner: owner, warehouse: warehouse, carton: carton,
		inboundAt: normalizedInstant(inboundAt), movementID: movementID,
	}, nil
}

func (batch CartonInboundBatch) Tenant() TenantID             { return batch.tenant }
func (batch CartonInboundBatch) ID() CartonInboundBatchID     { return batch.id }
func (batch CartonInboundBatch) Owner() OwnerAccountID        { return batch.owner }
func (batch CartonInboundBatch) Warehouse() WarehouseCode     { return batch.warehouse }
func (batch CartonInboundBatch) Carton() CartonNumber         { return batch.carton }
func (batch CartonInboundBatch) InboundAt() time.Time         { return batch.inboundAt }
func (batch CartonInboundBatch) MovementID() CartonMovementID { return batch.movementID }

// CartonMovement 是已经入账的一笔整箱流水。一行一箱，收货的数量是一。
type CartonMovement struct {
	tenant     TenantID
	id         CartonMovementID
	owner      OwnerAccountID
	warehouse  WarehouseCode
	reason     CartonMovementReason
	source     SourceFactReference
	occurredAt time.Time
	postedAt   time.Time
	lines      []CartonMovementLine
}

func NewCartonMovement(
	tenant TenantID,
	id CartonMovementID,
	owner OwnerAccountID,
	warehouse WarehouseCode,
	reason CartonMovementReason,
	source SourceFactReference,
	occurredAt time.Time,
	postedAt time.Time,
	lines []CartonMovementLine,
) (CartonMovement, error) {
	if !tenant.valid() || !id.valid() || !owner.valid() || !warehouse.valid() || reason.String() == "" || !source.valid() || occurredAt.IsZero() || postedAt.IsZero() || len(lines) == 0 {
		return CartonMovement{}, ErrInvalidCartonStock
	}
	seen := map[string]int64{}
	stored := make([]CartonMovementLine, len(lines))
	for index, line := range lines {
		if !line.location.valid() || !line.carton.valid() || !line.batch.valid() || (line.delta != 1 && line.delta != -1) {
			return CartonMovement{}, ErrInvalidCartonStock
		}
		seen[line.carton.String()] += line.delta
		stored[index] = line
	}
	for _, net := range seen {
		if net != 1 && net != 0 {
			return CartonMovement{}, ErrInvalidCartonStock
		}
	}
	return CartonMovement{
		tenant: tenant, id: id, owner: owner, warehouse: warehouse, reason: reason, source: source,
		occurredAt: normalizedInstant(occurredAt), postedAt: normalizedInstant(postedAt), lines: stored,
	}, nil
}

func (movement CartonMovement) Tenant() TenantID             { return movement.tenant }
func (movement CartonMovement) ID() CartonMovementID         { return movement.id }
func (movement CartonMovement) Owner() OwnerAccountID        { return movement.owner }
func (movement CartonMovement) Warehouse() WarehouseCode     { return movement.warehouse }
func (movement CartonMovement) Reason() CartonMovementReason { return movement.reason }
func (movement CartonMovement) Source() SourceFactReference  { return movement.source }
func (movement CartonMovement) OccurredAt() time.Time        { return movement.occurredAt }
func (movement CartonMovement) PostedAt() time.Time          { return movement.postedAt }
func (movement CartonMovement) Lines() []CartonMovementLine {
	return slices.Clone(movement.lines)
}

// ErrInvalidCartonStock 是一笔整箱流水或它的入库批次凑不成自洽的一份。
var ErrInvalidCartonStock = errInvalidCartonStock{}

type errInvalidCartonStock struct{}

func (errInvalidCartonStock) Error() string { return "warehouse fulfillment: invalid carton stock" }
