package domain

import (
	"slices"
	"time"
)

// 建账导入是一份待批准的库存调整（UC-WF-006）。提交只固定货主、仓与各行，不改账。
// 批准成立之后才以建账导入流水入账。驳回不改账。内容相同只比载荷，不比提交人与时刻。

// StockImportID 是调用方给的导入标识，也是重放身份。
type StockImportID struct{ requiredValue }

func NewStockImportID(value string) (StockImportID, error) {
	required, err := newRequiredValue("stock import ID", value)
	return StockImportID{required}, err
}

// StockImportReason 是这份导入的原因，租户原词，逐字保存。
type StockImportReason struct{ requiredValue }

func NewStockImportReason(value string) (StockImportReason, error) {
	required, err := newRequiredValue("stock import reason", value)
	return StockImportReason{required}, err
}

// StockImportStatus 是建账导入的生命周期。
type StockImportStatus uint8

const (
	StockImportStatusInvalid StockImportStatus = iota
	StockImportPending
	StockImportPosted
	StockImportRejected
)

func (status StockImportStatus) String() string {
	switch status {
	case StockImportPending:
		return "PENDING"
	case StockImportPosted:
		return "POSTED"
	case StockImportRejected:
		return "REJECTED"
	default:
		return ""
	}
}

// StockImportGoodsLine 是一行商品库存。贴标别名、批号、效期与声明的入库时间都可以没有。
type StockImportGoodsLine struct {
	location          LocationCode
	goods             GoodsCode
	unit              GoodsUnitName
	state             StockState
	quantity          int64
	batch             BatchNumber
	expiry            ExpiryDate
	alias             GoodsAliasCode
	declaredInboundAt time.Time
}

func NewStockImportGoodsLine(
	location LocationCode,
	goods GoodsCode,
	unit GoodsUnitName,
	state StockState,
	quantity int64,
	batch BatchNumber,
	expiry ExpiryDate,
	alias GoodsAliasCode,
	declaredInboundAt time.Time,
) (StockImportGoodsLine, error) {
	line := StockImportGoodsLine{
		location: location, goods: goods, unit: unit, state: state, quantity: quantity,
		batch: batch, expiry: expiry, alias: alias, declaredInboundAt: normalizedInstant(declaredInboundAt),
	}
	if !location.valid() || !goods.valid() || !unit.valid() || state.String() == "" {
		return StockImportGoodsLine{}, ErrInvalidStockImport
	}
	return line, nil
}

func (line StockImportGoodsLine) Location() LocationCode       { return line.location }
func (line StockImportGoodsLine) Goods() GoodsCode             { return line.goods }
func (line StockImportGoodsLine) Unit() GoodsUnitName          { return line.unit }
func (line StockImportGoodsLine) State() StockState            { return line.state }
func (line StockImportGoodsLine) Quantity() int64              { return line.quantity }
func (line StockImportGoodsLine) BatchNumber() BatchNumber     { return line.batch }
func (line StockImportGoodsLine) Expiry() ExpiryDate           { return line.expiry }
func (line StockImportGoodsLine) Alias() GoodsAliasCode        { return line.alias }
func (line StockImportGoodsLine) DeclaredInboundAt() time.Time { return line.declaredInboundAt }

func (line StockImportGoodsLine) sameAs(other StockImportGoodsLine) bool {
	return line.location == other.location && line.goods == other.goods && line.unit == other.unit &&
		line.state == other.state && line.quantity == other.quantity && line.batch == other.batch &&
		line.expiry == other.expiry && line.alias == other.alias &&
		normalizedInstant(line.declaredInboundAt).Equal(normalizedInstant(other.declaredInboundAt))
}

// StockImportCartonContent 是整箱箱单的一行：商品与基本单位数量。
type StockImportCartonContent struct {
	goods    GoodsCode
	quantity int64
}

func NewStockImportCartonContent(goods GoodsCode, quantity int64) (StockImportCartonContent, error) {
	if !goods.valid() {
		return StockImportCartonContent{}, ErrInvalidStockImport
	}
	return StockImportCartonContent{goods: goods, quantity: quantity}, nil
}

func (content StockImportCartonContent) Goods() GoodsCode { return content.goods }
func (content StockImportCartonContent) Quantity() int64  { return content.quantity }

// StockImportCartonLine 是一行整箱。箱单至少一行。声明的入库时间可以没有。
type StockImportCartonLine struct {
	location          LocationCode
	number            CartonNumber
	contents          []StockImportCartonContent
	declaredInboundAt time.Time
}

func NewStockImportCartonLine(
	location LocationCode,
	number CartonNumber,
	contents []StockImportCartonContent,
	declaredInboundAt time.Time,
) (StockImportCartonLine, error) {
	if !location.valid() || !number.valid() {
		return StockImportCartonLine{}, ErrInvalidStockImport
	}
	return StockImportCartonLine{
		location: location, number: number, contents: slices.Clone(contents),
		declaredInboundAt: normalizedInstant(declaredInboundAt),
	}, nil
}

func (line StockImportCartonLine) Location() LocationCode { return line.location }
func (line StockImportCartonLine) Number() CartonNumber   { return line.number }
func (line StockImportCartonLine) Contents() []StockImportCartonContent {
	return slices.Clone(line.contents)
}
func (line StockImportCartonLine) DeclaredInboundAt() time.Time { return line.declaredInboundAt }

func (line StockImportCartonLine) sameAs(other StockImportCartonLine) bool {
	return line.location == other.location && line.number == other.number &&
		slices.Equal(line.contents, other.contents) &&
		normalizedInstant(line.declaredInboundAt).Equal(normalizedInstant(other.declaredInboundAt))
}

// ErrInvalidStockImport 是一份建账导入缺了身份、原因或行，或状态迁移对不上。
var ErrInvalidStockImport = errInvalidStockImport{}

type errInvalidStockImport struct{}

func (errInvalidStockImport) Error() string { return "warehouse fulfillment: invalid stock import" }

// StockImport 是一份已落册的建账导入。
type StockImport struct {
	tenant      TenantID
	id          StockImportID
	owner       OwnerAccountID
	warehouse   WarehouseCode
	reason      StockImportReason
	goods       []StockImportGoodsLine
	cartons     []StockImportCartonLine
	submitter   Registrant
	submittedAt time.Time
	status      StockImportStatus
	poster      Registrant
	postedAt    time.Time
	goodsMove   StockMovementID
	cartonMove  CartonMovementID
	rejector    Registrant
	rejectedAt  time.Time
	rejectNote  string
}

func NewPendingStockImport(
	tenant TenantID,
	id StockImportID,
	owner OwnerAccountID,
	warehouse WarehouseCode,
	reason StockImportReason,
	goods []StockImportGoodsLine,
	cartons []StockImportCartonLine,
	submitter Registrant,
	submittedAt time.Time,
) (StockImport, error) {
	item := StockImport{
		tenant: tenant, id: id, owner: owner, warehouse: warehouse, reason: reason,
		goods: slices.Clone(goods), cartons: slices.Clone(cartons),
		submitter: submitter, submittedAt: normalizedInstant(submittedAt), status: StockImportPending,
	}
	if err := item.validShape(); err != nil {
		return StockImport{}, err
	}
	return item, nil
}

func (item StockImport) validShape() error {
	if !item.tenant.valid() || !item.id.valid() || !item.owner.valid() || !item.warehouse.valid() ||
		!item.reason.valid() || !item.submitter.valid() || item.submittedAt.IsZero() || item.status.String() == "" {
		return ErrInvalidStockImport
	}
	if len(item.goods) == 0 && len(item.cartons) == 0 {
		return ErrInvalidStockImport
	}
	switch item.status {
	case StockImportPending:
		if item.poster.valid() || !item.postedAt.IsZero() || item.goodsMove.valid() || item.cartonMove.valid() ||
			item.rejector.valid() || !item.rejectedAt.IsZero() || item.rejectNote != "" {
			return ErrInvalidStockImport
		}
	case StockImportPosted:
		if !item.poster.valid() || item.postedAt.IsZero() || (!item.goodsMove.valid() && !item.cartonMove.valid()) ||
			item.rejector.valid() || !item.rejectedAt.IsZero() || item.rejectNote != "" {
			return ErrInvalidStockImport
		}
		if len(item.goods) > 0 && !item.goodsMove.valid() {
			return ErrInvalidStockImport
		}
		if len(item.cartons) > 0 && !item.cartonMove.valid() {
			return ErrInvalidStockImport
		}
	case StockImportRejected:
		if !item.rejector.valid() || item.rejectedAt.IsZero() || item.poster.valid() || !item.postedAt.IsZero() ||
			item.goodsMove.valid() || item.cartonMove.valid() {
			return ErrInvalidStockImport
		}
	default:
		return ErrInvalidStockImport
	}
	return nil
}

func (item StockImport) Tenant() TenantID          { return item.tenant }
func (item StockImport) ID() StockImportID         { return item.id }
func (item StockImport) Owner() OwnerAccountID     { return item.owner }
func (item StockImport) Warehouse() WarehouseCode  { return item.warehouse }
func (item StockImport) Reason() StockImportReason { return item.reason }
func (item StockImport) GoodsLines() []StockImportGoodsLine {
	return slices.Clone(item.goods)
}
func (item StockImport) CartonLines() []StockImportCartonLine {
	return slices.Clone(item.cartons)
}
func (item StockImport) Submitter() Registrant     { return item.submitter }
func (item StockImport) SubmittedAt() time.Time    { return item.submittedAt }
func (item StockImport) Status() StockImportStatus { return item.status }
func (item StockImport) Poster() Registrant        { return item.poster }
func (item StockImport) PostedAt() time.Time       { return item.postedAt }
func (item StockImport) GoodsMovementID() StockMovementID {
	return item.goodsMove
}
func (item StockImport) CartonMovementID() CartonMovementID { return item.cartonMove }
func (item StockImport) Rejector() Registrant               { return item.rejector }
func (item StockImport) RejectedAt() time.Time              { return item.rejectedAt }
func (item StockImport) RejectionNote() string              { return item.rejectNote }

// SameContent 只比载荷：货主、仓、原因与各行的先后。提交人、时刻与状态不参与。
func (item StockImport) SameContent(owner OwnerAccountID, warehouse WarehouseCode, reason StockImportReason, goods []StockImportGoodsLine, cartons []StockImportCartonLine) bool {
	if item.owner != owner || item.warehouse != warehouse || item.reason != reason ||
		len(item.goods) != len(goods) || len(item.cartons) != len(cartons) {
		return false
	}
	for index := range item.goods {
		if !item.goods[index].sameAs(goods[index]) {
			return false
		}
	}
	for index := range item.cartons {
		if !item.cartons[index].sameAs(cartons[index]) {
			return false
		}
	}
	return true
}

// Post 把待批准的导入收成已入账。有商品行必须带商品流水，有整箱行必须带整箱流水。
func (item StockImport) Post(poster Registrant, postedAt time.Time, goodsMove StockMovementID, cartonMove CartonMovementID) (StockImport, error) {
	if item.status != StockImportPending {
		return StockImport{}, ErrInvalidStockImport
	}
	next := item
	next.status = StockImportPosted
	next.poster = poster
	next.postedAt = normalizedInstant(postedAt)
	next.goodsMove = goodsMove
	next.cartonMove = cartonMove
	if err := next.validShape(); err != nil {
		return StockImport{}, err
	}
	return next, nil
}

// Reject 把待批准的导入收成已驳回。说明可以没有。
func (item StockImport) Reject(rejector Registrant, rejectedAt time.Time, note string) (StockImport, error) {
	if item.status != StockImportPending {
		return StockImport{}, ErrInvalidStockImport
	}
	next := item
	next.status = StockImportRejected
	next.rejector = rejector
	next.rejectedAt = normalizedInstant(rejectedAt)
	next.rejectNote = note
	if err := next.validShape(); err != nil {
		return StockImport{}, err
	}
	return next, nil
}
