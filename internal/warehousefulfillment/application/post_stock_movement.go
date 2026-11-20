package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// PostStockMovementOutcome 是记数的结果，四格平铺。输入被拒是已形成的答案，不是技术失败；只有账或主数据读不动才另走 error。
type PostStockMovementOutcome uint8

const (
	PostStockMovementOutcomeInvalid PostStockMovementOutcome = iota
	// PostStockPosted 是这一笔新落册。
	PostStockPosted
	// PostStockAlreadyPosted 是同一判重位置、内容相同的重投：回显在册的那一笔。
	PostStockAlreadyPosted
	// PostStockSourceConflict 是同一判重位置、内容不同：在册的不变。
	PostStockSourceConflict
	// PostStockRefused 是有不合之处，或并发下取走之后余额会成负的：整笔不落。
	PostStockRefused
)

func (outcome PostStockMovementOutcome) String() string {
	switch outcome {
	case PostStockPosted:
		return "POSTED"
	case PostStockAlreadyPosted:
		return "ALREADY_POSTED"
	case PostStockSourceConflict:
		return "SOURCE_CONFLICT"
	case PostStockRefused:
		return "REFUSED"
	default:
		return ""
	}
}

// StockViolationKind 是一处不合的种类。字面量与名字一致，封闭集之外没有别的答法。
type StockViolationKind uint8

const (
	StockViolationKindInvalid StockViolationKind = iota
	StockViolationReasonNotSupported
	StockViolationWarehouseNotRegistered
	StockViolationWarehouseNotSelfOperated
	StockViolationShapeNotAllowed
	StockViolationLocationNotRegistered
	StockViolationZoneIncompatible
	StockViolationGoodsNotRegistered
	StockViolationUnitNotDeclared
	StockViolationBatchNumberRequired
	StockViolationBatchNumberNotTracked
	StockViolationExpiryRequired
	StockViolationExpiryNotTracked
	StockViolationLabelAliasNotValid
	StockViolationInboundBatchRequired
	StockViolationInboundBatchNotAllowed
	StockViolationInboundBatchNotFound
	StockViolationInboundBatchMismatch
	StockViolationDeclaredInboundTimeInvalid
	StockViolationInsufficientBalance
)

func (kind StockViolationKind) String() string {
	switch kind {
	case StockViolationReasonNotSupported:
		return "REASON_NOT_SUPPORTED"
	case StockViolationWarehouseNotRegistered:
		return "WAREHOUSE_NOT_REGISTERED"
	case StockViolationWarehouseNotSelfOperated:
		return "WAREHOUSE_NOT_SELF_OPERATED"
	case StockViolationShapeNotAllowed:
		return "SHAPE_NOT_ALLOWED_FOR_REASON"
	case StockViolationLocationNotRegistered:
		return "LOCATION_NOT_REGISTERED"
	case StockViolationZoneIncompatible:
		return "ZONE_INCOMPATIBLE"
	case StockViolationGoodsNotRegistered:
		return "GOODS_NOT_REGISTERED"
	case StockViolationUnitNotDeclared:
		return "UNIT_NOT_DECLARED"
	case StockViolationBatchNumberRequired:
		return "BATCH_NUMBER_REQUIRED"
	case StockViolationBatchNumberNotTracked:
		return "BATCH_NUMBER_NOT_TRACKED"
	case StockViolationExpiryRequired:
		return "EXPIRY_REQUIRED"
	case StockViolationExpiryNotTracked:
		return "EXPIRY_NOT_TRACKED"
	case StockViolationLabelAliasNotValid:
		return "LABEL_ALIAS_NOT_VALID"
	case StockViolationInboundBatchRequired:
		return "INBOUND_BATCH_REQUIRED"
	case StockViolationInboundBatchNotAllowed:
		return "INBOUND_BATCH_NOT_ALLOWED"
	case StockViolationInboundBatchNotFound:
		return "INBOUND_BATCH_NOT_FOUND"
	case StockViolationInboundBatchMismatch:
		return "INBOUND_BATCH_MISMATCH"
	case StockViolationDeclaredInboundTimeInvalid:
		return "DECLARED_INBOUND_TIME_INVALID"
	case StockViolationInsufficientBalance:
		return "INSUFFICIENT_BALANCE"
	default:
		return ""
	}
}

// StockViolation 是一处不合。Line 是请求里行的下标；整笔层面的用 -1。
type StockViolation struct {
	Line int
	Kind StockViolationKind
}

func (violation StockViolation) String() string {
	return fmt.Sprintf("%d:%s", violation.Line, violation.Kind)
}

// PostStockMovementResult 携带落点。流水只在 POSTED 与 ALREADY_POSTED 上有；不合之处只在 REFUSED 上有。
type PostStockMovementResult struct {
	outcome     PostStockMovementOutcome
	movement    domain.StockMovement
	hasMovement bool
	violations  []StockViolation
}

func (result PostStockMovementResult) Outcome() PostStockMovementOutcome {
	return result.outcome
}

// Movement 是落册的流水，或重放时在册的那一笔。
func (result PostStockMovementResult) Movement() (domain.StockMovement, bool) {
	return result.movement, result.hasMovement
}

func (result PostStockMovementResult) Violations() []StockViolation {
	return slices.Clone(result.violations)
}

func postedStock(movement domain.StockMovement) PostStockMovementResult {
	return PostStockMovementResult{outcome: PostStockPosted, movement: movement, hasMovement: true}
}

func alreadyPostedStock(movement domain.StockMovement) PostStockMovementResult {
	return PostStockMovementResult{outcome: PostStockAlreadyPosted, movement: movement, hasMovement: true}
}

func conflictingStock() PostStockMovementResult {
	return PostStockMovementResult{outcome: PostStockSourceConflict}
}

func refusedStock(violations []StockViolation) PostStockMovementResult {
	return PostStockMovementResult{outcome: PostStockRefused, violations: violations}
}

func wholeStockViolation(kind StockViolationKind) []StockViolation {
	return []StockViolation{{Line: -1, Kind: kind}}
}

// errStockQuantityOverflow 是记入与取走合并、或余额加净额时超出 int64。这是技术失败，不是「不够」。
var errStockQuantityOverflow = errors.New("数量相加回绕")

// PostStockMovementCommand 是往库存账记的一笔。Tenant 取自认证信封，不取自载荷。
type PostStockMovementCommand struct {
	Tenant  domain.TenantID
	Request domain.StockMovementRequest
}

func (command PostStockMovementCommand) assemblyError() error {
	request := command.Request
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case request.Owner().String() == "":
		return errors.New("缺货主")
	case request.Warehouse().String() == "":
		return errors.New("缺仓")
	case request.Reason().String() == "":
		return errors.New("缺原因")
	case request.Source().String() == "":
		return errors.New("缺来源事实引用")
	case request.OccurredAt().IsZero():
		return errors.New("缺事实发生时刻")
	default:
		return nil
	}
}

// PostStockMovementHandler 把一笔记进库存账。事务边界在调用方：每笔记数各起一个环境事务，账的落册在没有环境事务时拒绝
// （同 RegisterGoodsHandler）。收货确认在调用方的同一笔事务里调用它。生产装配在仓配开关打开时从收货确认口进来。
type PostStockMovementHandler struct {
	ledger     ports.StockLedger
	warehouses ports.WarehouseLookup
	locations  ports.StockLocationLookup
	zones      ports.ZoneLookup
	goods      ports.StockGoodsLookup
	identities ports.StockIdentities
	clock      ports.Clock
}

func NewPostStockMovementHandler(
	ledger ports.StockLedger,
	warehouses ports.WarehouseLookup,
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
	goods ports.StockGoodsLookup,
	identities ports.StockIdentities,
	clock ports.Clock,
) *PostStockMovementHandler {
	return &PostStockMovementHandler{
		ledger:     ledger,
		warehouses: warehouses,
		locations:  locations,
		zones:      zones,
		goods:      goods,
		identities: identities,
		clock:      clock,
	}
}

// Post 把一笔流水记进库存账。判定次序：
//
//  1. 命令缺租户、货主、仓、原因、来源事实引用或事实发生时刻：装配错，交 error，不是业务答复。
//  2. SerializePosting。必须排在任何读之前：读到的商品修订与余额要撑到落册。
//  3. 按判重位置取在册的那一笔：内容相同答 ALREADY_POSTED，不同答 SOURCE_CONFLICT。重放排在一切校验之前——
//     重放的是已经形成的答案，不因为此后主数据变了就改口。
//  4. 原因是开箱：REFUSED，REASON_NOT_SUPPORTED。开箱要整箱库存的行，那一段还没做。
//  5. 仓：本租户没登记答 WAREHOUSE_NOT_REGISTERED；不是自营仓答 WAREHOUSE_NOT_SELF_OPERATED。停用的仓不拒。这两格直接返回，不再看行。
//  6. 行的形状与原因的类不符：SHAPE_NOT_ALLOWED_FOR_REASON，直接返回。
//  7. 逐行收齐全部不合之处，不只指第一处。商品每一行都查，没登记则这一行其余依赖商品的格不再判。
//     库位与库区只查记入行，停用的库位、库区不拒；库位登记了而库区读不到是册不自洽，交 error。
//     计量单位、批号、效期、贴标别名只在该看的记入行上判。声明的入库时间只有建账导入的记入行可以带，且不得晚于入账那一刻。
//     形成批次的行不许点名入库批次；其余行必须点名，并且批次的货主、仓、商品、批号、效期与行一致。
//  8. 上一步没有任何不合时，不改汇总的四种组内不相抵：SHAPE_NOT_ALLOWED_FOR_REASON。折合数取各商品当前修订的包装层级。
//  9. 到这里有不合之处：答 REFUSED，不落册。
//  10. 余额：同一格的记入与取走合并，净取走的格余额加净额为负，则该格的每个取走行各记一条 INSUFFICIENT_BALANCE，答 REFUSED。
//  11. 入账时刻取时钟，整个 Post 只取一次，第 7 步也用这一个。形成批次的行按（商品，批号，效期，入库时间）分组，
//     每组一个入库批次；入库时间取声明的那个，没声明取入账时刻。行的先后保持请求里的先后，批次按首次出现的先后签发。
//  12. 落册：Appended 答 POSTED；AlreadyPosted 重新取在册的那一笔，按第 3 步的规则答，取不回来交 error；
//     InsufficientBalance 答 REFUSED，一条整笔的 INSUFFICIENT_BALANCE。
//
// 同一个商品、库位、库区、入库批次在一笔里只读一次。
func (handler *PostStockMovementHandler) Post(ctx context.Context, command PostStockMovementCommand) (PostStockMovementResult, error) {
	if err := command.assemblyError(); err != nil {
		return PostStockMovementResult{}, fmt.Errorf("post stock movement: %w", err)
	}
	request := command.Request
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, request.Owner(), request.Warehouse()); err != nil {
		return PostStockMovementResult{}, fmt.Errorf("post stock movement: %w", err)
	}
	if result, done, err := handler.replay(ctx, command.Tenant, request); err != nil || done {
		return result, err
	}
	if request.Reason() == domain.StockReasonUnboxing {
		return refusedStock(wholeStockViolation(StockViolationReasonNotSupported)), nil
	}
	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, request.Warehouse())
	if err != nil {
		return PostStockMovementResult{}, fmt.Errorf("post stock movement: %w", err)
	}
	if !found {
		return refusedStock(wholeStockViolation(StockViolationWarehouseNotRegistered)), nil
	}
	if warehouse.Content().Kind() != domain.WarehouseKindSelfOperated {
		return refusedStock(wholeStockViolation(StockViolationWarehouseNotSelfOperated)), nil
	}
	if request.ShapeDefect() {
		return refusedStock(wholeStockViolation(StockViolationShapeNotAllowed)), nil
	}

	// 入账时刻只取这一次。第 7 步判声明的入库时间，第 11 步给没声明的批次，用的是同一个瞬间。
	postedAt := handler.clock.Now()
	screen, err := handler.screenLines(ctx, command.Tenant, request, postedAt)
	if err != nil {
		return PostStockMovementResult{}, fmt.Errorf("post stock movement: %w", err)
	}
	if len(screen.violations) == 0 && request.ConservationDefect(screen.factor) {
		screen.violations = append(screen.violations, StockViolation{Line: -1, Kind: StockViolationShapeNotAllowed})
	}
	if len(screen.violations) > 0 {
		return refusedStock(screen.violations), nil
	}
	short, err := handler.shortBalances(ctx, command.Tenant, request)
	if err != nil {
		return PostStockMovementResult{}, fmt.Errorf("post stock movement: %w", err)
	}
	if len(short) > 0 {
		return refusedStock(short), nil
	}
	movement, formed, err := handler.assemble(command.Tenant, request, postedAt)
	if err != nil {
		return PostStockMovementResult{}, fmt.Errorf("post stock movement: %w", err)
	}
	outcome, err := handler.ledger.AppendMovement(ctx, movement, formed)
	if err != nil {
		return PostStockMovementResult{}, fmt.Errorf("post stock movement: %w", err)
	}
	switch outcome {
	case ports.StockAppended:
		return postedStock(movement), nil
	case ports.StockAlreadyPosted:
		return handler.replayAfterAppend(ctx, command.Tenant, request)
	case ports.StockInsufficientBalance:
		return refusedStock(wholeStockViolation(StockViolationInsufficientBalance)), nil
	default:
		return PostStockMovementResult{}, fmt.Errorf("post stock movement: 不认识的落册结果 %q", outcome)
	}
}

func (handler *PostStockMovementHandler) replay(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.StockMovementRequest,
) (PostStockMovementResult, bool, error) {
	existing, found, err := handler.ledger.LoadMovementBySource(ctx, tenant, request.Reason(), request.Source())
	if err != nil {
		return PostStockMovementResult{}, false, fmt.Errorf("post stock movement: %w", err)
	}
	if !found {
		return PostStockMovementResult{}, false, nil
	}
	return comparePosted(existing, request), true, nil
}

func comparePosted(existing domain.StockMovement, request domain.StockMovementRequest) PostStockMovementResult {
	if existing.Request().SameAs(request) {
		return alreadyPostedStock(existing)
	}
	return conflictingStock()
}

func (handler *PostStockMovementHandler) replayAfterAppend(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.StockMovementRequest,
) (PostStockMovementResult, error) {
	existing, found, err := handler.ledger.LoadMovementBySource(ctx, tenant, request.Reason(), request.Source())
	if err != nil {
		return PostStockMovementResult{}, fmt.Errorf("post stock movement: %w", err)
	}
	if !found {
		return PostStockMovementResult{}, errors.New("post stock movement: 落册答已在册，按判重位置却取不回")
	}
	return comparePosted(existing, request), nil
}

type lineScreen struct {
	violations []StockViolation
	factor     func(domain.GoodsCode, domain.GoodsUnitName) (int64, bool)
}

func (handler *PostStockMovementHandler) screenLines(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.StockMovementRequest,
	postedAt time.Time,
) (lineScreen, error) {
	goods := &revisionCache[domain.GoodsCode, domain.GoodsRevision]{}
	goodsAt := &revisionCache[domain.GoodsCode, domain.GoodsRevision]{}
	locations := &revisionCache[domain.LocationCode, domain.LocationRevision]{}
	zones := &revisionCache[domain.ZoneCode, domain.ZoneRevision]{}
	batches := &revisionCache[domain.InboundBatchID, domain.InboundBatch]{}
	var violations []StockViolation
	for index, line := range request.Lines() {
		hit, err := goods.load(line.Cell().Goods(), func() (domain.GoodsRevision, bool, error) {
			return handler.goods.LoadLatestGoods(ctx, tenant, request.Owner(), line.Cell().Goods())
		})
		if err != nil {
			return lineScreen{}, err
		}
		if !hit.found {
			violations = append(violations, StockViolation{Line: index, Kind: StockViolationGoodsNotRegistered})
		}
		if line.Direction() == domain.StockIn {
			more, err := handler.screenInbound(ctx, tenant, request, line, index, hit, goodsAt, locations, zones)
			if err != nil {
				return lineScreen{}, err
			}
			violations = append(violations, more...)
		}
		if declaredInboundTimeInvalid(request, line, postedAt) {
			violations = append(violations, StockViolation{Line: index, Kind: StockViolationDeclaredInboundTimeInvalid})
		}
		batchViolation, err := handler.screenBatch(ctx, tenant, request, line, batches)
		if err != nil {
			return lineScreen{}, err
		}
		if batchViolation != StockViolationKindInvalid {
			violations = append(violations, StockViolation{Line: index, Kind: batchViolation})
		}
	}
	return lineScreen{violations: violations, factor: factorFrom(goods.found)}, nil
}

func (handler *PostStockMovementHandler) screenInbound(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.StockMovementRequest,
	line domain.StockLineRequest,
	index int,
	goods goodsHitOf[domain.GoodsRevision],
	goodsAt *revisionCache[domain.GoodsCode, domain.GoodsRevision],
	locations *revisionCache[domain.LocationCode, domain.LocationRevision],
	zones *revisionCache[domain.ZoneCode, domain.ZoneRevision],
) ([]StockViolation, error) {
	var violations []StockViolation
	location, err := locations.load(line.Cell().Location(), func() (domain.LocationRevision, bool, error) {
		return handler.locations.LoadLatestLocation(ctx, tenant, request.Warehouse(), line.Cell().Location())
	})
	if err != nil {
		return nil, err
	}
	if !location.found {
		violations = append(violations, StockViolation{Line: index, Kind: StockViolationLocationNotRegistered})
	} else {
		zoneCode := location.value.Content().ZoneCode()
		zone, err := zones.load(zoneCode, func() (domain.ZoneRevision, bool, error) {
			return handler.zones.LoadLatestZone(ctx, tenant, request.Warehouse(), zoneCode)
		})
		if err != nil {
			return nil, err
		}
		if !zone.found {
			// 库位登记了而库区读不到，册不自洽，不是业务拒绝。
			return nil, fmt.Errorf("库位 %s 已登记，它的库区 %s 读不到", line.Cell().Location(), zoneCode)
		}
		if !line.Cell().State().AllowedIn(zone.value.Content().Usage()) {
			violations = append(violations, StockViolation{Line: index, Kind: StockViolationZoneIncompatible})
		}
	}
	if !goods.found {
		return violations, nil
	}
	if !unitDeclared(goods.value, line.Cell().Unit()) {
		violations = append(violations, StockViolation{Line: index, Kind: StockViolationUnitNotDeclared})
	}
	if tracksNewGoods(request.Reason()) {
		violations = append(violations, trackingViolations(index, goods.value, line)...)
	}
	// 库内移动、状态转换与形态转换移的是已经在格上的货，别名过期之后仍要能移动。
	// 有效区间只约束这一行新贴上的别名：增项、相抵，以及商品换标换上去的那一格。
	// 贴标别名按事实发生时刻生效的那一笔修订判，包装层级与跟踪属性仍按入账时的当前修订判。
	if checksLabel(request.Reason()) && line.Cell().Alias().String() != "" {
		atHit, err := goodsAt.load(line.Cell().Goods(), func() (domain.GoodsRevision, bool, error) {
			return handler.goods.LoadGoodsAt(ctx, tenant, request.Owner(), line.Cell().Goods(), request.OccurredAt())
		})
		if err != nil {
			return nil, err
		}
		if !atHit.found || !labelValid(atHit.value, line.Cell().Alias(), request.OccurredAt()) {
			violations = append(violations, StockViolation{Line: index, Kind: StockViolationLabelAliasNotValid})
		}
	}
	return violations, nil
}

func trackingViolations(index int, revision domain.GoodsRevision, line domain.StockLineRequest) []StockViolation {
	var violations []StockViolation
	content := revision.Content()
	switch {
	case content.TrackBatch() && !line.Cell().BatchNumber().Given():
		violations = append(violations, StockViolation{Line: index, Kind: StockViolationBatchNumberRequired})
	case !content.TrackBatch() && line.Cell().BatchNumber().Given():
		violations = append(violations, StockViolation{Line: index, Kind: StockViolationBatchNumberNotTracked})
	}
	switch {
	case content.TrackExpiry() && !line.Cell().Expiry().Given():
		violations = append(violations, StockViolation{Line: index, Kind: StockViolationExpiryRequired})
	case !content.TrackExpiry() && line.Cell().Expiry().Given():
		violations = append(violations, StockViolation{Line: index, Kind: StockViolationExpiryNotTracked})
	}
	return violations
}

func declaredInboundTimeInvalid(request domain.StockMovementRequest, line domain.StockLineRequest, postedAt time.Time) bool {
	if line.DeclaredInboundAt().IsZero() {
		return false
	}
	if request.Reason() != domain.StockReasonTakeOnImport || line.Direction() != domain.StockIn {
		return true
	}
	return line.DeclaredInboundAfterPosting(postedAt)
}

func (handler *PostStockMovementHandler) screenBatch(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.StockMovementRequest,
	line domain.StockLineRequest,
	batches *revisionCache[domain.InboundBatchID, domain.InboundBatch],
) (StockViolationKind, error) {
	forms := request.Reason().FormsInboundBatch() && line.Direction() == domain.StockIn
	named := line.Cell().InboundBatch().String() != ""
	if forms {
		if named {
			return StockViolationInboundBatchNotAllowed, nil
		}
		return StockViolationKindInvalid, nil
	}
	if !named {
		return StockViolationInboundBatchRequired, nil
	}
	batch, err := batches.load(line.Cell().InboundBatch(), func() (domain.InboundBatch, bool, error) {
		return handler.ledger.LoadInboundBatch(ctx, tenant, line.Cell().InboundBatch())
	})
	if err != nil {
		return StockViolationKindInvalid, err
	}
	if !batch.found || batch.value.Tenant() != tenant || batch.value.Owner() != request.Owner() || batch.value.Warehouse() != request.Warehouse() || batch.value.Goods() != line.Cell().Goods() {
		return StockViolationInboundBatchNotFound, nil
	}
	if batch.value.BatchNumber() != line.Cell().BatchNumber() || batch.value.Expiry() != line.Cell().Expiry() {
		return StockViolationInboundBatchMismatch, nil
	}
	return StockViolationKindInvalid, nil
}

func tracksNewGoods(reason domain.StockMovementReason) bool {
	class := reason.Class()
	return class == domain.StockReasonIncrease || class == domain.StockReasonOffsetting
}

func checksLabel(reason domain.StockMovementReason) bool {
	class := reason.Class()
	return class == domain.StockReasonIncrease || class == domain.StockReasonOffsetting || reason == domain.StockReasonRelabel
}

func unitDeclared(revision domain.GoodsRevision, unit domain.GoodsUnitName) bool {
	for _, level := range revision.Content().PackagingLevels() {
		if level.Name() == unit {
			return true
		}
	}
	return false
}

func labelValid(revision domain.GoodsRevision, alias domain.GoodsAliasCode, at time.Time) bool {
	for _, item := range revision.Content().Aliases() {
		if item.Code() != alias {
			continue
		}
		return item.Kind() == domain.GoodsAliasPlatformCode && item.Validity().Contains(at)
	}
	return false
}

func factorFrom(found map[domain.GoodsCode]domain.GoodsRevision) func(domain.GoodsCode, domain.GoodsUnitName) (int64, bool) {
	return func(code domain.GoodsCode, unit domain.GoodsUnitName) (int64, bool) {
		revision, ok := found[code]
		if !ok {
			return 0, false
		}
		for _, level := range revision.Content().PackagingLevels() {
			if level.Name() == unit {
				return level.Factor(), true
			}
		}
		return 0, false
	}
}

type cellNet struct {
	net  int64
	outs []int
}

func (handler *PostStockMovementHandler) shortBalances(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.StockMovementRequest,
) ([]StockViolation, error) {
	nets := map[domain.StockCell]*cellNet{}
	var order []domain.StockCell
	for index, line := range request.Lines() {
		cell := line.Cell()
		entry := nets[cell]
		if entry == nil {
			entry = &cellNet{}
			nets[cell] = entry
			order = append(order, cell)
		}
		delta := line.Quantity()
		if line.Direction() == domain.StockOut {
			delta = -line.Quantity()
			entry.outs = append(entry.outs, index)
		}
		// 回绕交 error，不形成「够」或「不够」的答复：回绕会把不够算成够，而这次答复若被提交，账就短了。
		next, ok := domain.AddQuantity(entry.net, delta)
		if !ok {
			return nil, errStockQuantityOverflow
		}
		entry.net = next
	}
	var violations []StockViolation
	for _, cell := range order {
		entry := nets[cell]
		if entry.net >= 0 {
			continue
		}
		balance, err := handler.ledger.CellBalance(ctx, tenant, request.Owner(), request.Warehouse(), cell)
		if err != nil {
			return nil, err
		}
		total, ok := domain.AddQuantity(balance, entry.net)
		if !ok {
			return nil, errStockQuantityOverflow
		}
		if total < 0 {
			for _, index := range entry.outs {
				violations = append(violations, StockViolation{Line: index, Kind: StockViolationInsufficientBalance})
			}
		}
	}
	return violations, nil
}

type batchGroup struct {
	goods   domain.GoodsCode
	number  domain.BatchNumber
	expiry  domain.ExpiryDate
	inbound time.Time
}

func (handler *PostStockMovementHandler) assemble(
	tenant domain.TenantID,
	request domain.StockMovementRequest,
	postedAt time.Time,
) (domain.StockMovement, []domain.InboundBatch, error) {
	movementID, err := handler.identities.NextStockMovementID()
	if err != nil {
		return domain.StockMovement{}, nil, err
	}
	// 按行的先后签发，不靠 map 的遍历次序：同一份请求必须得到同一串入库批次标识。
	ids := map[batchGroup]domain.InboundBatchID{}
	var formed []domain.InboundBatch
	lines := request.Lines()
	for _, line := range lines {
		if !formsBatch(request, line) {
			continue
		}
		key := batchGroup{
			goods:   line.Cell().Goods(),
			number:  line.Cell().BatchNumber(),
			expiry:  line.Cell().Expiry(),
			inbound: line.InboundTime(postedAt),
		}
		if _, seen := ids[key]; seen {
			continue
		}
		id, err := handler.identities.NextInboundBatchID()
		if err != nil {
			return domain.StockMovement{}, nil, err
		}
		batch, err := domain.NewInboundBatch(
			tenant, id, request.Owner(), request.Warehouse(), key.goods, key.number, key.expiry, key.inbound, movementID,
		)
		if err != nil {
			return domain.StockMovement{}, nil, err
		}
		ids[key] = id
		formed = append(formed, batch)
	}
	stored := make([]domain.StockMovementLine, len(lines))
	for index, line := range lines {
		cell := line.Cell()
		formedLine := false
		if formsBatch(request, line) {
			key := batchGroup{
				goods:   cell.Goods(),
				number:  cell.BatchNumber(),
				expiry:  cell.Expiry(),
				inbound: line.InboundTime(postedAt),
			}
			cell = cell.WithInboundBatch(ids[key])
			formedLine = true
		}
		stored[index] = domain.NewStockMovementLine(line.Direction(), line.Quantity(), cell, line.DeclaredInboundAt(), formedLine)
	}
	movement, err := domain.NewStockMovement(
		tenant, movementID, request.Owner(), request.Warehouse(), request.Reason(), request.Source(), request.OccurredAt(), postedAt, stored,
	)
	if err != nil {
		return domain.StockMovement{}, nil, err
	}
	return movement, formed, nil
}

func formsBatch(request domain.StockMovementRequest, line domain.StockLineRequest) bool {
	return request.Reason().FormsInboundBatch() && line.Direction() == domain.StockIn
}

// revisionCache 让同一笔里同一个键只读一次。读到没有也记住，免得下一行再打一次已经知道没有的册。
type revisionCache[K comparable, V any] struct {
	hits  map[K]goodsHitOf[V]
	found map[K]V
}

type goodsHitOf[V any] struct {
	value V
	found bool
}

func (cache *revisionCache[K, V]) load(key K, read func() (V, bool, error)) (goodsHitOf[V], error) {
	if cache.hits == nil {
		cache.hits = map[K]goodsHitOf[V]{}
		cache.found = map[K]V{}
	}
	if hit, ok := cache.hits[key]; ok {
		return hit, nil
	}
	value, found, err := read()
	if err != nil {
		return goodsHitOf[V]{}, err
	}
	hit := goodsHitOf[V]{value: value, found: found}
	cache.hits[key] = hit
	if found {
		cache.found[key] = value
	}
	return hit, nil
}
