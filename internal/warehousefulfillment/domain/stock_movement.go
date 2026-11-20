package domain

import (
	"errors"
	"math"
	"math/bits"
	"time"
)

var (
	// ErrInvalidStockCell 是一格缺了库位、商品、计量单位或库存状态。入库批次在请求里可以没有，不在这里拒。
	ErrInvalidStockCell = errors.New("warehouse fulfillment: invalid stock cell")
	// ErrInvalidInboundBatch 是一个入库批次缺了身份、商品或入库时间。
	ErrInvalidInboundBatch = errors.New("warehouse fulfillment: invalid inbound batch")
	// ErrInvalidStockMovement 是一笔已入账的流水缺了必填、没有行、某行没有入库批次，或行的形状与原因的类不符。
	ErrInvalidStockMovement = errors.New("warehouse fulfillment: invalid stock movement")
)

// StockDirection 是一行对一格的方向：记入或取走。零值不是其中任何一个。
type StockDirection uint8

const (
	StockDirectionInvalid StockDirection = iota
	StockIn
	StockOut
)

func (direction StockDirection) valid() bool {
	return direction == StockIn || direction == StockOut
}

// StockCellSpec 是一格在货主、仓之内的那几维。贴标别名、入库批次、批号、效期都可以没有。
type StockCellSpec struct {
	Location     LocationCode
	Goods        GoodsCode
	Unit         GoodsUnitName
	State        StockState
	Alias        GoodsAliasCode
	InboundBatch InboundBatchID
	BatchNumber  BatchNumber
	Expiry       ExpiryDate
}

// StockCell 是商品库存的一格里、随流水行走的那几维（CONTEXT「库存账」）。租户、货主与仓在流水上，不在格上。
// 每一维都可比较，格可以做余额合并的键：往里放切片或时刻会让同一格对不上，也会把单调钟带进键。
type StockCell struct {
	location     LocationCode
	goods        GoodsCode
	unit         GoodsUnitName
	state        StockState
	alias        GoodsAliasCode
	inboundBatch InboundBatchID
	batchNumber  BatchNumber
	expiry       ExpiryDate
}

// NewStockCell 要求库位、商品、计量单位与库存状态都给齐。入库批次在请求里可以为空，落册时才补上。
func NewStockCell(spec StockCellSpec) (StockCell, error) {
	cell := StockCell{
		location:     spec.Location,
		goods:        spec.Goods,
		unit:         spec.Unit,
		state:        spec.State,
		alias:        spec.Alias,
		inboundBatch: spec.InboundBatch,
		batchNumber:  spec.BatchNumber,
		expiry:       spec.Expiry,
	}
	if !cell.location.valid() || !cell.goods.valid() || !cell.unit.valid() || cell.state.String() == "" {
		return StockCell{}, ErrInvalidStockCell
	}
	return cell, nil
}

func (cell StockCell) Location() LocationCode { return cell.location }

func (cell StockCell) Goods() GoodsCode { return cell.goods }

func (cell StockCell) Unit() GoodsUnitName { return cell.unit }

func (cell StockCell) State() StockState { return cell.state }

func (cell StockCell) Alias() GoodsAliasCode { return cell.alias }

func (cell StockCell) InboundBatch() InboundBatchID { return cell.inboundBatch }

func (cell StockCell) BatchNumber() BatchNumber { return cell.batchNumber }

func (cell StockCell) Expiry() ExpiryDate { return cell.expiry }

// WithInboundBatch 交回换了入库批次的副本，不动原来那一格。形成批次的行落册前没有标识，落册时才换上。
func (cell StockCell) WithInboundBatch(id InboundBatchID) StockCell {
	cell.inboundBatch = id
	return cell
}

// StockLineRequest 是请求里的一行：方向、按行上的计量单位计的数量、一格，以及可缺的声明入库时间。
// 数量须为正由形状来判，不拦在构造器里——零行、取走记成记入这类形状要能被指出来。
type StockLineRequest struct {
	direction         StockDirection
	quantity          int64
	cell              StockCell
	declaredInboundAt time.Time
}

func NewStockLineRequest(direction StockDirection, quantity int64, cell StockCell, declaredInboundAt time.Time) StockLineRequest {
	return StockLineRequest{direction: direction, quantity: quantity, cell: cell, declaredInboundAt: declaredInboundAt}
}

func (line StockLineRequest) Direction() StockDirection { return line.direction }

func (line StockLineRequest) Quantity() int64 { return line.quantity }

func (line StockLineRequest) Cell() StockCell { return line.cell }

func (line StockLineRequest) DeclaredInboundAt() time.Time { return line.declaredInboundAt }

// DeclaredInboundAfterPosting 回答声明的入库时间是否晚于入账那一刻。两边都先收到库里存得下的精度：
// 同一微秒里的纳秒差存不下来，不能拿它把建账导入拒掉。没声明的答否。
func (line StockLineRequest) DeclaredInboundAfterPosting(postedAt time.Time) bool {
	if line.declaredInboundAt.IsZero() {
		return false
	}
	return normalizedInstant(line.declaredInboundAt).After(normalizedInstant(postedAt))
}

// InboundTime 是这一行形成入库批次时用的入库时间：带了声明的用声明的，没带的用入账那一刻。
// 两处都收到同一精度，分组键才不会被纳秒或时区写法拆成两个批次。
func (line StockLineRequest) InboundTime(postedAt time.Time) time.Time {
	if line.declaredInboundAt.IsZero() {
		return normalizedInstant(postedAt)
	}
	return normalizedInstant(line.declaredInboundAt)
}

func (line StockLineRequest) sameAs(other StockLineRequest) bool {
	return line.direction == other.direction &&
		line.quantity == other.quantity &&
		line.cell == other.cell &&
		normalizedInstant(line.declaredInboundAt).Equal(normalizedInstant(other.declaredInboundAt))
}

// StockMovementRequest 是记数前的一笔：一个货主、一个仓、原因、来源事实引用、事实发生时刻与若干行。
// 仓间调拨不进这一笔（CONTEXT「库存账与守恒」）。
type StockMovementRequest struct {
	owner      OwnerAccountID
	warehouse  WarehouseCode
	reason     StockMovementReason
	source     SourceFactReference
	occurredAt time.Time
	lines      []StockLineRequest
}

func NewStockMovementRequest(
	owner OwnerAccountID,
	warehouse WarehouseCode,
	reason StockMovementReason,
	source SourceFactReference,
	occurredAt time.Time,
	lines []StockLineRequest,
) StockMovementRequest {
	return StockMovementRequest{
		owner:      owner,
		warehouse:  warehouse,
		reason:     reason,
		source:     source,
		occurredAt: occurredAt,
		lines:      append([]StockLineRequest(nil), lines...),
	}
}

func (request StockMovementRequest) Owner() OwnerAccountID { return request.owner }

func (request StockMovementRequest) Warehouse() WarehouseCode { return request.warehouse }

func (request StockMovementRequest) Reason() StockMovementReason { return request.reason }

func (request StockMovementRequest) Source() SourceFactReference { return request.source }

func (request StockMovementRequest) OccurredAt() time.Time { return request.occurredAt }

func (request StockMovementRequest) Lines() []StockLineRequest {
	return append([]StockLineRequest(nil), request.lines...)
}

// SameAs 回答两份请求是不是同一份内容：逐格相同，行的先后也相同。时刻按同一瞬间比，不用 ==：
// 不同时区写法与同一微秒里的纳秒差都要算同一时刻，否则原样重投会被判成内容不同。
func (request StockMovementRequest) SameAs(other StockMovementRequest) bool {
	if request.owner != other.owner || request.warehouse != other.warehouse || request.reason != other.reason || request.source != other.source {
		return false
	}
	if !normalizedInstant(request.occurredAt).Equal(normalizedInstant(other.occurredAt)) {
		return false
	}
	if len(request.lines) != len(other.lines) {
		return false
	}
	for index := range request.lines {
		if !request.lines[index].sameAs(other.lines[index]) {
			return false
		}
	}
	return true
}

// ShapeDefect 回答行的形状与原因的类是否不符。至少一行；增项只有记入，减项只有取走；
// 不改汇总与相抵两类记入与取走都至少一行；数量为正；方向得是记入或取走。类不认识的也算不符。
func (request StockMovementRequest) ShapeDefect() bool {
	if len(request.lines) == 0 {
		return true
	}
	var inbound, outbound int
	for _, line := range request.lines {
		if line.quantity <= 0 || !line.direction.valid() {
			return true
		}
		switch line.direction {
		case StockIn:
			inbound++
		case StockOut:
			outbound++
		}
	}
	switch request.reason.Class() {
	case StockReasonIncrease:
		return outbound > 0
	case StockReasonDecrease:
		return inbound > 0
	case StockReasonConserving, StockReasonOffsetting:
		// 开箱的取走记在整箱账。商品账这一笔只有记入，沿用该箱的入库时间形成商品入库批次。
		if request.reason == StockReasonUnboxing {
			return inbound == 0 || outbound > 0
		}
		return inbound == 0 || outbound == 0
	default:
		return true
	}
}

type conservationKey struct {
	goods  GoodsCode
	batch  InboundBatchID
	number BatchNumber
	expiry ExpiryDate
	unit   GoodsUnitName
	alias  GoodsAliasCode
}

func conservationKeyFor(reason StockMovementReason, cell StockCell) conservationKey {
	key := conservationKey{
		goods:  cell.goods,
		batch:  cell.inboundBatch,
		number: cell.batchNumber,
		expiry: cell.expiry,
	}
	switch reason {
	case StockReasonInternalMove, StockReasonStateChange:
		key.unit = cell.unit
		key.alias = cell.alias
	case StockReasonRelabel:
		key.unit = cell.unit
	case StockReasonUnitConversion:
		key.alias = cell.alias
	}
	return key
}

// ConservationDefect 只对不改汇总的四种判组内相抵，其余原因恒假（加工与拆解不要求按商品相抵）。
// factor 把一行的数量折成基本单位；答不出折合数，或乘法、累加会回绕，都算有缺陷——回绕会把不相抵算成相抵。
func (request StockMovementRequest) ConservationDefect(factor func(GoodsCode, GoodsUnitName) (int64, bool)) bool {
	switch request.reason {
	case StockReasonInternalMove, StockReasonStateChange, StockReasonRelabel, StockReasonUnitConversion:
	default:
		return false
	}
	sums := map[conservationKey]int64{}
	converting := request.reason == StockReasonUnitConversion
	for _, line := range request.lines {
		if line.quantity <= 0 || !line.direction.valid() {
			return true
		}
		key := conservationKeyFor(request.reason, line.cell)
		delta, ok := signedQuantity(line, converting, factor)
		if !ok {
			return true
		}
		next, ok := AddQuantity(sums[key], delta)
		if !ok {
			return true
		}
		sums[key] = next
	}
	for _, sum := range sums {
		if sum != 0 {
			return true
		}
	}
	return false
}

func signedQuantity(line StockLineRequest, converting bool, factor func(GoodsCode, GoodsUnitName) (int64, bool)) (int64, bool) {
	if !converting {
		if line.direction == StockOut {
			return -line.quantity, true
		}
		return line.quantity, true
	}
	if factor == nil {
		return 0, false
	}
	factorValue, found := factor(line.cell.goods, line.cell.unit)
	if !found {
		return 0, false
	}
	return scaleQuantity(line.quantity, factorValue, line.direction == StockOut)
}

// scaleQuantity 把数量乘折合数再按方向取符号。任一端是 int64 的下界时取负本身就会回绕，直接算有缺陷。
func scaleQuantity(quantity, factor int64, outbound bool) (int64, bool) {
	if quantity == math.MinInt64 || factor == math.MinInt64 {
		return 0, false
	}
	magnitude, factorMagnitude := quantity, factor
	negative := outbound
	if magnitude < 0 {
		negative = !negative
		magnitude = -magnitude
	}
	if factorMagnitude < 0 {
		negative = !negative
		factorMagnitude = -factorMagnitude
	}
	hi, lo := bits.Mul64(uint64(magnitude), uint64(factorMagnitude))
	if hi != 0 || lo > uint64(math.MaxInt64) {
		return 0, false
	}
	product := int64(lo)
	if negative {
		product = -product
	}
	return product, true
}

// AddQuantity 把两笔带符号的数量相加。回绕时答不出和：回绕会把不够算成够，也会把不相抵算成相抵。
func AddQuantity(sum, delta int64) (int64, bool) {
	next := sum + delta
	if (delta > 0 && next < sum) || (delta < 0 && next > sum) {
		return 0, false
	}
	return next, true
}

// InboundBatch 是一次入账为一个商品形成的批次（CONTEXT「入库批次」）。批号与效期可以没有。
type InboundBatch struct {
	tenant      TenantID
	id          InboundBatchID
	owner       OwnerAccountID
	warehouse   WarehouseCode
	goods       GoodsCode
	batchNumber BatchNumber
	expiry      ExpiryDate
	inboundAt   time.Time
	movementID  StockMovementID
}

// NewInboundBatch 要求身份、商品、形成它的流水与入库时间都齐。入库时间统一到 UTC 并收到微秒，与商品资料里的时刻同一口径。
func NewInboundBatch(
	tenant TenantID,
	id InboundBatchID,
	owner OwnerAccountID,
	warehouse WarehouseCode,
	goods GoodsCode,
	batchNumber BatchNumber,
	expiry ExpiryDate,
	inboundAt time.Time,
	movementID StockMovementID,
) (InboundBatch, error) {
	if !tenant.valid() || !id.valid() || !owner.valid() || !warehouse.valid() || !goods.valid() || !movementID.valid() || inboundAt.IsZero() {
		return InboundBatch{}, ErrInvalidInboundBatch
	}
	return InboundBatch{
		tenant:      tenant,
		id:          id,
		owner:       owner,
		warehouse:   warehouse,
		goods:       goods,
		batchNumber: batchNumber,
		expiry:      expiry,
		inboundAt:   normalizedInstant(inboundAt),
		movementID:  movementID,
	}, nil
}

func (batch InboundBatch) Tenant() TenantID { return batch.tenant }

func (batch InboundBatch) ID() InboundBatchID { return batch.id }

func (batch InboundBatch) Owner() OwnerAccountID { return batch.owner }

func (batch InboundBatch) Warehouse() WarehouseCode { return batch.warehouse }

func (batch InboundBatch) Goods() GoodsCode { return batch.goods }

func (batch InboundBatch) BatchNumber() BatchNumber { return batch.batchNumber }

func (batch InboundBatch) Expiry() ExpiryDate { return batch.expiry }

func (batch InboundBatch) InboundAt() time.Time { return batch.inboundAt }

func (batch InboundBatch) MovementID() StockMovementID { return batch.movementID }

// StockMovementLine 是已入账的一行。这一行的格上入库批次必有。
// FormedInboundBatch 记下这一行的入库批次是不是这笔流水形成的，还原请求时才知道哪一格当时是空的。
type StockMovementLine struct {
	direction          StockDirection
	quantity           int64
	cell               StockCell
	declaredInboundAt  time.Time
	formedInboundBatch bool
}

func NewStockMovementLine(
	direction StockDirection,
	quantity int64,
	cell StockCell,
	declaredInboundAt time.Time,
	formedInboundBatch bool,
) StockMovementLine {
	return StockMovementLine{
		direction:          direction,
		quantity:           quantity,
		cell:               cell,
		declaredInboundAt:  declaredInboundAt,
		formedInboundBatch: formedInboundBatch,
	}
}

func (line StockMovementLine) Direction() StockDirection { return line.direction }

func (line StockMovementLine) Quantity() int64 { return line.quantity }

func (line StockMovementLine) Cell() StockCell { return line.cell }

func (line StockMovementLine) DeclaredInboundAt() time.Time { return line.declaredInboundAt }

func (line StockMovementLine) FormedInboundBatch() bool { return line.formedInboundBatch }

// StockMovement 是已经入账的一笔流水。已入账的不改不删（CONTEXT「库存账与守恒」）。
type StockMovement struct {
	tenant     TenantID
	id         StockMovementID
	owner      OwnerAccountID
	warehouse  WarehouseCode
	reason     StockMovementReason
	source     SourceFactReference
	occurredAt time.Time
	postedAt   time.Time
	lines      []StockMovementLine
}

// NewStockMovement 要求必填齐、至少一行、每行的入库批次都有，且行的形状与原因的类相符。
// 时刻统一到 UTC 并收到微秒：不收的话，回放时纳秒上的差会把原样重投看成另一份内容。
func NewStockMovement(
	tenant TenantID,
	id StockMovementID,
	owner OwnerAccountID,
	warehouse WarehouseCode,
	reason StockMovementReason,
	source SourceFactReference,
	occurredAt time.Time,
	postedAt time.Time,
	lines []StockMovementLine,
) (StockMovement, error) {
	if !tenant.valid() || !id.valid() || !owner.valid() || !warehouse.valid() || reason.String() == "" || !source.valid() || occurredAt.IsZero() || postedAt.IsZero() || len(lines) == 0 {
		return StockMovement{}, ErrInvalidStockMovement
	}
	stored := make([]StockMovementLine, len(lines))
	requested := make([]StockLineRequest, len(lines))
	for index, line := range lines {
		if line.cell.inboundBatch.String() == "" || line.quantity <= 0 || !line.direction.valid() {
			return StockMovement{}, ErrInvalidStockMovement
		}
		stored[index] = StockMovementLine{
			direction:          line.direction,
			quantity:           line.quantity,
			cell:               line.cell,
			declaredInboundAt:  normalizedInstant(line.declaredInboundAt),
			formedInboundBatch: line.formedInboundBatch,
		}
		requested[index] = NewStockLineRequest(line.direction, line.quantity, line.cell, line.declaredInboundAt)
	}
	if NewStockMovementRequest(owner, warehouse, reason, source, occurredAt, requested).ShapeDefect() {
		return StockMovement{}, ErrInvalidStockMovement
	}
	return StockMovement{
		tenant:     tenant,
		id:         id,
		owner:      owner,
		warehouse:  warehouse,
		reason:     reason,
		source:     source,
		occurredAt: normalizedInstant(occurredAt),
		postedAt:   normalizedInstant(postedAt),
		lines:      stored,
	}, nil
}

func (movement StockMovement) Tenant() TenantID { return movement.tenant }

func (movement StockMovement) ID() StockMovementID { return movement.id }

func (movement StockMovement) Owner() OwnerAccountID { return movement.owner }

func (movement StockMovement) Warehouse() WarehouseCode { return movement.warehouse }

func (movement StockMovement) Reason() StockMovementReason { return movement.reason }

func (movement StockMovement) Source() SourceFactReference { return movement.source }

func (movement StockMovement) OccurredAt() time.Time { return movement.occurredAt }

func (movement StockMovement) PostedAt() time.Time { return movement.postedAt }

func (movement StockMovement) Lines() []StockMovementLine {
	return append([]StockMovementLine(nil), movement.lines...)
}

// Request 还原当初的请求，供重放时比内容。形成批次的行当时没有入库批次，标识是落册时签发的，还原时清空；
// 其余行的批次是请求里点名的，原样留下。事实发生时刻与声明的入库时间用落册时收过精度的值。
func (movement StockMovement) Request() StockMovementRequest {
	lines := make([]StockLineRequest, len(movement.lines))
	for index, line := range movement.lines {
		cell := line.cell
		if line.formedInboundBatch {
			cell = cell.WithInboundBatch(InboundBatchID{})
		}
		lines[index] = NewStockLineRequest(line.direction, line.quantity, cell, line.declaredInboundAt)
	}
	return NewStockMovementRequest(movement.owner, movement.warehouse, movement.reason, movement.source, movement.occurredAt, lines)
}
