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

// ConfirmReceiptOutcome 是商品入库这一次收货的结果，平铺。没落册的那几格不占事实身份。
type ConfirmReceiptOutcome uint8

const (
	ConfirmReceiptOutcomeInvalid ConfirmReceiptOutcome = iota
	ConfirmReceiptConfirmed
	ConfirmReceiptExistingResult
	ConfirmReceiptContentConflict
	ConfirmReceiptContentInvalid
	ConfirmReceiptWarehouseNotFound
	ConfirmReceiptPreAdviceNotFound
	ConfirmReceiptDivertedToUnclaimed
)

func (outcome ConfirmReceiptOutcome) String() string {
	switch outcome {
	case ConfirmReceiptConfirmed:
		return "CONFIRMED"
	case ConfirmReceiptExistingResult:
		return "EXISTING_RESULT"
	case ConfirmReceiptContentConflict:
		return "CONTENT_CONFLICT"
	case ConfirmReceiptContentInvalid:
		return "CONTENT_INVALID"
	case ConfirmReceiptWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case ConfirmReceiptPreAdviceNotFound:
		return "PRE_ADVICE_NOT_FOUND"
	case ConfirmReceiptDivertedToUnclaimed:
		return "DIVERTED_TO_UNCLAIMED"
	default:
		return ""
	}
}

// ConfirmReceiptResult 携带落册的事实。只有 CONFIRMED、EXISTING_RESULT 与 DIVERTED_TO_UNCLAIMED 上回显。
type ConfirmReceiptResult struct {
	outcome ConfirmReceiptOutcome
	receipt domain.RecordedReceipt
	has     bool
}

func (result ConfirmReceiptResult) Outcome() ConfirmReceiptOutcome { return result.outcome }

func (result ConfirmReceiptResult) Receipt() (domain.RecordedReceipt, bool) {
	return result.receipt, result.has
}

func confirmReceiptEcho(outcome ConfirmReceiptOutcome, receipt domain.RecordedReceipt) ConfirmReceiptResult {
	return ConfirmReceiptResult{outcome: outcome, receipt: receipt, has: true}
}

func confirmReceiptBare(outcome ConfirmReceiptOutcome) ConfirmReceiptResult {
	return ConfirmReceiptResult{outcome: outcome}
}

// ConfirmReceiptCommand 是一次商品入库的收货。事实身份由设备签发。租户不取自载荷。
// 货主不在命令上：收的是预报上的那个货主，识别码不跨货主。
type ConfirmReceiptCommand struct {
	Tenant      domain.TenantID
	FactID      domain.ReceiptFactID
	Device      domain.ReceiptDeviceID
	Operator    domain.ReceiptOperatorID
	OccurredAt  time.Time
	Warehouse   domain.WarehouseCode
	PreAdvice   domain.PreAdviceID
	Shape       domain.InboundMode
	Lines       []domain.ReceiptLine
	CartonLines []domain.CartonReceiptLine
}

func (command ConfirmReceiptCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.FactID.String() == "":
		return errors.New("缺事实身份")
	case command.Device.String() == "":
		return errors.New("缺设备")
	case command.Operator.String() == "":
		return errors.New("缺操作者")
	case command.OccurredAt.IsZero():
		return errors.New("缺发生时刻")
	case command.Warehouse.String() == "":
		return errors.New("缺仓")
	case command.PreAdvice.String() == "":
		return errors.New("缺预报")
	case command.Shape.String() == "":
		return errors.New("缺行的形态")
	default:
		return nil
	}
}

func (command ConfirmReceiptCommand) submission() domain.ReceiptSubmission {
	return domain.ReceiptSubmission{
		Device: command.Device, Operator: command.Operator, OccurredAt: command.OccurredAt,
		Warehouse: command.Warehouse, PreAdvice: command.PreAdvice, Shape: command.Shape,
		Lines: command.Lines, CartonLines: command.CartonLines,
	}
}

// ConfirmReceiptHandler 把一条商品入库的收货事实落册，成立的行以收货流水入账，并形成这一次的差异。
// 事务边界在调用方。入账与推进预报状态必须在同一笔里：任一成立的行落不下，调用方回滚，预报不得留在收货中。
//
// preAdvices 与 notices 必须是同一只预报册。NoteReceiptStarted 拿的预报锁，和撤回是同一把。
type ConfirmReceiptHandler struct {
	receipts   ports.ReceiptStore
	preAdvices ports.PreAdviceStore
	notices    *AcceptPreAdviceHandler
	poster     *PostStockMovementHandler
	cartons    ports.CartonLedger
	cartonIDs  ports.CartonIdentities
	warehouses ports.WarehouseLookup
	locations  ports.StockLocationLookup
	zones      ports.ZoneLookup
	catalogue  ports.GoodsCurrentRead
	goods      ports.StockGoodsLookup
	identities ports.ReceiptDiscrepancyIdentities
	arrivals   ports.UnclaimedArrivalStore
	arrivalIDs ports.UnclaimedArrivalIdentities
	clock      ports.Clock
}

func NewConfirmReceiptHandler(
	receipts ports.ReceiptStore,
	preAdvices ports.PreAdviceStore,
	notices *AcceptPreAdviceHandler,
	poster *PostStockMovementHandler,
	cartons ports.CartonLedger,
	cartonIDs ports.CartonIdentities,
	warehouses ports.WarehouseLookup,
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
	catalogue ports.GoodsCurrentRead,
	goods ports.StockGoodsLookup,
	identities ports.ReceiptDiscrepancyIdentities,
	arrivals ports.UnclaimedArrivalStore,
	arrivalIDs ports.UnclaimedArrivalIdentities,
	clock ports.Clock,
) *ConfirmReceiptHandler {
	return &ConfirmReceiptHandler{
		receipts: receipts, preAdvices: preAdvices, notices: notices, poster: poster,
		cartons: cartons, cartonIDs: cartonIDs,
		warehouses: warehouses, locations: locations, zones: zones, catalogue: catalogue,
		goods: goods, identities: identities, arrivals: arrivals, arrivalIDs: arrivalIDs, clock: clock,
	}
}

// Confirm 判定次序：
//
//  1. 缺租户、事实身份、设备、操作者、发生时刻、仓、预报或行的形态：装配错，交 error。
//  2. 先拿事实身份这把锁，再按事实身份认重放。内容相同答 EXISTING_RESULT，不再入账；
//     内容不同答 CONTENT_CONFLICT，在册的与库存都不变。重放排在后面的校验之前。
//  3. 仓在本租户没登记：WAREHOUSE_NOT_FOUND，不落册。
//  4. 预报在本租户查不到：PRE_ADVICE_NOT_FOUND。预报的仓不是这一条事实的仓：CONTENT_INVALID，整条不落。
//  5. 行的形态与预报的入库方式不符：CONTENT_INVALID，整条不落，不占事实身份。
//     形态相符而且是整箱入库时，改走整箱收货：入整箱账，不进商品库存。
//  6. 整条先核，任一处不合就 CONTENT_INVALID，一行都不入账：库位在这个仓不存在，或所属库区不是收货暂存；
//     认得出的商品计量单位不是当前修订的包装层级；跟踪批号或效期的没带，不跟踪的却带了；
//     贴标别名不是该商品在事实发生时刻有效的平台商品码；数量不为正。库位已停用仍可入账。
//     别名按事实发生时刻判。解不出的码是认不出，不把整条判死，也不跨货主找。
//  7. 锁这份预报（与撤回同一把锁），再 NoteReceiptStarted。这一步必须先于入账：
//     入账会去拿货主锁与记数锁。若先拿货主锁再拿预报锁，会和改跟踪属性那一边成环——
//     商品修订先拿货主锁再问在库。已取消或已完结：原事实保全为一件待认领到货，
//     答 DIVERTED_TO_UNCLAIMED，不入商品库存，也不入整箱库存，预报状态不变。
//  8. 同一笔里入账。成立的行合成一笔原因是收货的流水，来源事实引用是这条事实的身份。
//     认不出的行不进流水。没有任何成立的行也答 CONFIRMED，预报仍进入收货中。
//     入账没有落成，交 error，调用方回滚，预报状态也不许单独推进。
//  9. 同一笔里算差异。多收按基本单位，超出的那一段记在这一次上。错货留下认得出的商品，或认不出的码与数量。
//     破损只记次品。待检不记差异。
func (handler *ConfirmReceiptHandler) Confirm(ctx context.Context, command ConfirmReceiptCommand) (ConfirmReceiptResult, error) {
	if err := command.assemblyError(); err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	if err := handler.receipts.HoldFact(ctx, command.Tenant, command.FactID); err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	existing, found, err := handler.receipts.Load(ctx, command.Tenant, command.FactID)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	if found {
		if existing.SameContent(command.submission()) {
			return confirmReceiptEcho(ConfirmReceiptExistingResult, existing), nil
		}
		return confirmReceiptBare(ConfirmReceiptContentConflict), nil
	}
	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	if !found {
		return confirmReceiptBare(ConfirmReceiptWarehouseNotFound), nil
	}
	advice, found, err := handler.preAdvices.Find(ctx, command.Tenant, command.PreAdvice)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	if !found {
		return confirmReceiptBare(ConfirmReceiptPreAdviceNotFound), nil
	}
	if advice.Warehouse() != command.Warehouse {
		return confirmReceiptBare(ConfirmReceiptContentInvalid), nil
	}
	if command.Shape != advice.Inbound() || receiptShapeMixed(command) {
		return confirmReceiptBare(ConfirmReceiptContentInvalid), nil
	}
	if advice.Inbound() == domain.InboundByCarton {
		return handler.confirmCarton(ctx, warehouse, advice, command)
	}
	prepared, invalid, err := handler.screen(ctx, advice, command)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	if invalid {
		return confirmReceiptBare(ConfirmReceiptContentInvalid), nil
	}
	if warehouse.Content().Kind() != domain.WarehouseKindSelfOperated {
		return ConfirmReceiptResult{}, errors.New("confirm receipt: 仓不是自营仓，收货入账无由落")
	}

	// 先拿预报锁，再让入账去拿货主锁与记数锁。次序反了会和改跟踪属性成环：那边先拿货主锁，再读库存账。
	noted, err := handler.notices.NoteReceiptStarted(ctx, NotePreAdviceCommand{Tenant: command.Tenant, ID: advice.ID()})
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	receivedAt := handler.clock.Now()
	switch noted.Outcome() {
	case PreAdviceReceiptNotReceivable:
		lines, err := markNotReceivable(command.Lines)
		if err != nil {
			return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
		}
		return handler.divert(ctx, advice, command, receivedAt, lines, nil)
	case PreAdviceReceiptStarted, PreAdviceReceiptAlreadyReceiving:
	default:
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: unexpected pre-advice receipt %s", noted.Outcome())
	}
	lines, postable, err := markConfirmed(prepared)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	if len(postable) > 0 {
		if err := handler.post(ctx, advice, command, postable); err != nil {
			return ConfirmReceiptResult{}, err
		}
	}
	differences, err := handler.differences(ctx, advice, command, lines)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	var discrepancyID domain.ReceiptDiscrepancyID
	if len(differences) > 0 {
		discrepancyID, err = handler.identities.NextReceiptDiscrepancyID()
		if err != nil {
			return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
		}
	}
	receipt, err := handler.record(ctx, command, receivedAt, domain.RecordedReceiptConfirmed, lines, nil, discrepancyID, differences)
	if err != nil {
		return ConfirmReceiptResult{}, err
	}
	return confirmReceiptEcho(ConfirmReceiptConfirmed, receipt), nil
}

type preparedLine struct {
	input    domain.ReceiptLine
	goods    domain.GoodsCode
	known    bool
	revision domain.GoodsRevision
}

func (handler *ConfirmReceiptHandler) screen(
	ctx context.Context,
	advice domain.PreAdvice,
	command ConfirmReceiptCommand,
) ([]preparedLine, bool, error) {
	if len(command.Lines) == 0 {
		return nil, true, nil
	}
	seenLocation := map[string]struct{}{}
	var prepared []preparedLine
	for _, line := range command.Lines {
		if line.Quantity() <= 0 {
			return nil, true, nil
		}
		if _, seen := seenLocation[line.Location().String()]; !seen {
			rejected, err := handler.locationRejected(ctx, command.Tenant, command.Warehouse, line.Location())
			if err != nil {
				return nil, false, err
			}
			if rejected {
				return nil, true, nil
			}
			seenLocation[line.Location().String()] = struct{}{}
		}
		known, err := handler.resolve(ctx, advice, command.OccurredAt, line)
		if err != nil {
			return nil, false, err
		}
		if known == nil {
			prepared = append(prepared, preparedLine{input: line})
			continue
		}
		if !lineAccepted(*known, line, command.OccurredAt) {
			return nil, true, nil
		}
		prepared = append(prepared, preparedLine{input: line, goods: known.Code(), known: true, revision: *known})
	}
	return prepared, false, nil
}

func (handler *ConfirmReceiptHandler) locationRejected(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
) (bool, error) {
	location, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, code)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, location.Content().ZoneCode())
	if err != nil {
		return false, err
	}
	if !found {
		return false, fmt.Errorf("库位 %s 已登记，它的库区 %s 读不到", code, location.Content().ZoneCode())
	}
	if zone.Content().Usage() != domain.ZoneUsageReceivingStaging {
		return true, nil
	}
	return false, nil
}

func (handler *ConfirmReceiptHandler) resolve(
	ctx context.Context,
	advice domain.PreAdvice,
	occurred time.Time,
	line domain.ReceiptLine,
) (*domain.GoodsRevision, error) {
	code, err := domain.NewIdentifierCode(line.Observed())
	if err != nil {
		return nil, nil
	}
	row, found, err := handler.catalogue.FindGoodsByCode(ctx, advice.Tenant(), advice.Owner(), code, occurred)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	goods, err := domain.NewGoodsCode(row.GoodsCode)
	if err != nil {
		return nil, err
	}
	revision, found, err := handler.goods.LoadLatestGoods(ctx, advice.Tenant(), advice.Owner(), goods)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("按码找到商品 %s，当前修订却读不到", goods)
	}
	return &revision, nil
}

func lineAccepted(revision domain.GoodsRevision, line domain.ReceiptLine, occurred time.Time) bool {
	if _, ok := unitFactor(revision, line.Unit()); !ok {
		return false
	}
	content := revision.Content()
	if content.TrackBatch() != line.Batch().Given() || content.TrackExpiry() != line.Expiry().Given() {
		return false
	}
	return aliasAccepted(revision, line.Alias(), occurred)
}

func unitFactor(revision domain.GoodsRevision, unit domain.GoodsUnitName) (int64, bool) {
	for _, level := range revision.Content().PackagingLevels() {
		if level.Name() == unit && level.Factor() > 0 {
			return level.Factor(), true
		}
	}
	return 0, false
}

func aliasAccepted(revision domain.GoodsRevision, alias domain.GoodsAliasCode, at time.Time) bool {
	if alias.String() == "" {
		return true
	}
	for _, item := range revision.Content().Aliases() {
		if item.Code() != alias {
			continue
		}
		return item.Kind() == domain.GoodsAliasPlatformCode && item.Validity().Contains(at)
	}
	return false
}

func markNotReceivable(lines []domain.ReceiptLine) ([]domain.ReceiptLine, error) {
	marked := make([]domain.ReceiptLine, len(lines))
	for index, line := range lines {
		next, err := line.AsNotReceivable()
		if err != nil {
			return nil, err
		}
		marked[index] = next
	}
	return marked, nil
}

func markConfirmed(prepared []preparedLine) ([]domain.ReceiptLine, []domain.ReceiptLine, error) {
	lines := make([]domain.ReceiptLine, len(prepared))
	var postable []domain.ReceiptLine
	for index, item := range prepared {
		var (
			line domain.ReceiptLine
			err  error
		)
		if item.known {
			line, err = item.input.AsPosted(item.goods)
		} else {
			line, err = item.input.AsUnknown()
		}
		if err != nil {
			return nil, nil, err
		}
		lines[index] = line
		if line.Result() == domain.ReceiptLinePosted {
			postable = append(postable, line)
		}
	}
	return lines, postable, nil
}

func (handler *ConfirmReceiptHandler) post(
	ctx context.Context,
	advice domain.PreAdvice,
	command ConfirmReceiptCommand,
	lines []domain.ReceiptLine,
) error {
	source, err := domain.NewSourceFactReference(command.FactID.String())
	if err != nil {
		return fmt.Errorf("confirm receipt: %w", err)
	}
	stockLines := make([]domain.StockLineRequest, len(lines))
	for index, line := range lines {
		cell, err := domain.NewStockCell(domain.StockCellSpec{
			Location: line.Location(), Goods: line.Goods(), Unit: line.Unit(), State: line.State(),
			Alias: line.Alias(), BatchNumber: line.Batch(), Expiry: line.Expiry(),
		})
		if err != nil {
			return fmt.Errorf("confirm receipt: %w", err)
		}
		stockLines[index] = domain.NewStockLineRequest(domain.StockIn, line.Quantity(), cell, time.Time{})
	}
	request := domain.NewStockMovementRequest(
		advice.Owner(), command.Warehouse, domain.StockReasonReceipt, source, command.OccurredAt, stockLines,
	)
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{Tenant: command.Tenant, Request: request})
	if err != nil {
		return fmt.Errorf("confirm receipt: %w", err)
	}
	if posted.Outcome() != PostStockPosted {
		return fmt.Errorf("confirm receipt: 收货入账没有落成 %s", posted.Outcome())
	}
	return nil
}

func (handler *ConfirmReceiptHandler) record(
	ctx context.Context,
	command ConfirmReceiptCommand,
	receivedAt time.Time,
	outcome domain.RecordedReceiptOutcome,
	lines []domain.ReceiptLine,
	cartonLines []domain.CartonReceiptLine,
	discrepancy domain.ReceiptDiscrepancyID,
	differences []domain.ReceiptDiscrepancyLine,
) (domain.RecordedReceipt, error) {
	receipt, err := domain.NewRecordedReceipt(domain.RecordedReceiptSpec{
		Tenant: command.Tenant, FactID: command.FactID, Device: command.Device, Operator: command.Operator,
		OccurredAt: command.OccurredAt, ReceivedAt: receivedAt, Warehouse: command.Warehouse, PreAdvice: command.PreAdvice,
		Shape: command.Shape, Outcome: outcome, Lines: lines, CartonLines: cartonLines,
		Discrepancy: discrepancy, Differences: differences,
	})
	if err != nil {
		return domain.RecordedReceipt{}, fmt.Errorf("confirm receipt: %w", err)
	}
	inserted, err := handler.receipts.Save(ctx, receipt)
	if err != nil {
		return domain.RecordedReceipt{}, fmt.Errorf("confirm receipt: %w", err)
	}
	if !inserted {
		return domain.RecordedReceipt{}, errors.New("confirm receipt: 事实身份在锁内仍被占")
	}
	return receipt, nil
}

// divert 把已经不能收货的预报上迟到的事实保全成一件待认领到货。
// 原事实原样留下，不入商品库存，也不入整箱库存。预报状态已由 NoteReceiptStarted 保持不动。
// 同一事实再投走前面的重放：内容相同不另建，内容不同答冲突、在册的不变。
func (handler *ConfirmReceiptHandler) divert(
	ctx context.Context,
	advice domain.PreAdvice,
	command ConfirmReceiptCommand,
	receivedAt time.Time,
	lines []domain.ReceiptLine,
	cartonLines []domain.CartonReceiptLine,
) (ConfirmReceiptResult, error) {
	if handler.arrivals == nil || handler.arrivalIDs == nil {
		return ConfirmReceiptResult{}, errors.New("confirm receipt: 待认领到货没有接上")
	}
	receipt, err := handler.record(ctx, command, receivedAt, domain.RecordedReceiptDivertedToUnclaimed, lines, cartonLines, domain.ReceiptDiscrepancyID{}, nil)
	if err != nil {
		return ConfirmReceiptResult{}, err
	}
	arrivalID, err := handler.arrivalIDs.NextUnclaimedArrivalID()
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	arrival, err := domain.NewUnclaimedArrival(arrivalID, advice.Owner(), receipt)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	inserted, err := handler.arrivals.Save(ctx, arrival)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	if !inserted {
		return ConfirmReceiptResult{}, errors.New("confirm receipt: 待认领到货在锁内仍被占")
	}
	return confirmReceiptEcho(ConfirmReceiptDivertedToUnclaimed, receipt), nil
}

func (handler *ConfirmReceiptHandler) differences(
	ctx context.Context,
	advice domain.PreAdvice,
	command ConfirmReceiptCommand,
	lines []domain.ReceiptLine,
) ([]domain.ReceiptDiscrepancyLine, error) {
	cache := map[string]domain.GoodsRevision{}
	factor := func(goods domain.GoodsCode, unit domain.GoodsUnitName) (int64, error) {
		revision, ok := cache[goods.String()]
		if !ok {
			loaded, found, err := handler.goods.LoadLatestGoods(ctx, advice.Tenant(), advice.Owner(), goods)
			if err != nil {
				return 0, err
			}
			if !found {
				return 0, fmt.Errorf("商品 %s 没有当前修订", goods)
			}
			revision = loaded
			cache[goods.String()] = revision
		}
		value, ok := unitFactor(revision, unit)
		if !ok {
			return 0, fmt.Errorf("商品 %s 的计量单位 %s 没有折合数", goods, unit)
		}
		return value, nil
	}
	advised, err := baseByGoods(advice.GoodsLines(), func(goods domain.GoodsCode, unit domain.GoodsUnitName, quantity int64) (int64, error) {
		value, err := factor(goods, unit)
		if err != nil {
			return 0, err
		}
		base, ok := domain.BaseQuantity(quantity, value)
		if !ok {
			return 0, errors.New("预报数量折成基本单位放不下")
		}
		return base, nil
	})
	if err != nil {
		return nil, err
	}
	earlier, err := handler.receipts.ListByPreAdvices(ctx, advice.Tenant(), []domain.PreAdviceID{advice.ID()})
	if err != nil {
		return nil, err
	}
	prior, err := postedBase(earlier, factor)
	if err != nil {
		return nil, err
	}
	current, err := postedLineBase(lines, factor)
	if err != nil {
		return nil, err
	}
	damaged, err := damageBase(lines, factor)
	if err != nil {
		return nil, err
	}
	return differenceLines(advised, prior, current, damaged, lines)
}

func baseByGoods(
	lines []domain.GoodsInboundLine,
	convert func(domain.GoodsCode, domain.GoodsUnitName, int64) (int64, error),
) (map[string]int64, error) {
	totals := map[string]int64{}
	for _, line := range lines {
		base, err := convert(line.Goods(), line.Unit(), line.Quantity())
		if err != nil {
			return nil, err
		}
		next, ok := domain.AddQuantity(totals[line.Goods().String()], base)
		if !ok {
			return nil, errors.New("预报数量相加放不下")
		}
		totals[line.Goods().String()] = next
	}
	return totals, nil
}

func postedBase(receipts []domain.RecordedReceipt, factor func(domain.GoodsCode, domain.GoodsUnitName) (int64, error)) (map[string]int64, error) {
	var lines []domain.ReceiptLine
	for _, receipt := range receipts {
		lines = append(lines, receipt.Lines()...)
	}
	return postedLineBase(lines, factor)
}

func postedLineBase(lines []domain.ReceiptLine, factor func(domain.GoodsCode, domain.GoodsUnitName) (int64, error)) (map[string]int64, error) {
	totals := map[string]int64{}
	for _, line := range lines {
		if line.Result() != domain.ReceiptLinePosted {
			continue
		}
		base, err := lineBase(line, factor)
		if err != nil {
			return nil, err
		}
		next, ok := domain.AddQuantity(totals[line.Goods().String()], base)
		if !ok {
			return nil, errors.New("实收数量相加放不下")
		}
		totals[line.Goods().String()] = next
	}
	return totals, nil
}

func lineBase(line domain.ReceiptLine, factor func(domain.GoodsCode, domain.GoodsUnitName) (int64, error)) (int64, error) {
	value, err := factor(line.Goods(), line.Unit())
	if err != nil {
		return 0, err
	}
	base, ok := domain.BaseQuantity(line.Quantity(), value)
	if !ok {
		return 0, errors.New("实收数量折成基本单位放不下")
	}
	return base, nil
}

func differenceLines(advised, prior, current, damaged map[string]int64, lines []domain.ReceiptLine) ([]domain.ReceiptDiscrepancyLine, error) {
	var differences []domain.ReceiptDiscrepancyLine
	goods := make([]string, 0, len(advised))
	for code := range advised {
		goods = append(goods, code)
	}
	slices.Sort(goods)
	for _, code := range goods {
		over, ok := domain.OverageQuantity(advised[code], prior[code], current[code])
		if !ok {
			return nil, errors.New("多收算不出来")
		}
		if over == 0 {
			continue
		}
		item, err := domain.NewReceiptDiscrepancyLine(domain.ReceiptDiscrepancyOverage, over, mustGoods(code), "")
		if err != nil {
			return nil, err
		}
		differences = append(differences, item)
	}
	wrong := map[string]int64{}
	for code, quantity := range current {
		if _, onAdvice := advised[code]; onAdvice || quantity == 0 {
			continue
		}
		wrong[code] = quantity
	}
	wrongCodes := make([]string, 0, len(wrong))
	for code := range wrong {
		wrongCodes = append(wrongCodes, code)
	}
	slices.Sort(wrongCodes)
	for _, code := range wrongCodes {
		item, err := domain.NewReceiptDiscrepancyLine(domain.ReceiptDiscrepancyWrongGoods, wrong[code], mustGoods(code), "")
		if err != nil {
			return nil, err
		}
		differences = append(differences, item)
	}
	for _, line := range lines {
		if line.Result() != domain.ReceiptLineUnknownGoods {
			continue
		}
		item, err := domain.NewReceiptDiscrepancyLine(domain.ReceiptDiscrepancyWrongGoods, line.Quantity(), domain.GoodsCode{}, line.Observed())
		if err != nil {
			return nil, err
		}
		differences = append(differences, item)
	}
	for _, code := range sortedQuantities(damaged) {
		item, err := domain.NewReceiptDiscrepancyLine(domain.ReceiptDiscrepancyDamage, damaged[code], mustGoods(code), "")
		if err != nil {
			return nil, err
		}
		differences = append(differences, item)
	}
	return differences, nil
}

// damageBase 把记成次品的已入账数量折成基本单位。待检不进破损。
func damageBase(lines []domain.ReceiptLine, factor func(domain.GoodsCode, domain.GoodsUnitName) (int64, error)) (map[string]int64, error) {
	var defective []domain.ReceiptLine
	for _, line := range lines {
		if line.Result() == domain.ReceiptLinePosted && line.State() == domain.StockStateDefective {
			defective = append(defective, line)
		}
	}
	return postedLineBase(defective, factor)
}

func sortedQuantities(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for key, quantity := range values {
		if quantity != 0 {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func mustGoods(code string) domain.GoodsCode {
	goods, err := domain.NewGoodsCode(code)
	if err != nil {
		return domain.GoodsCode{}
	}
	return goods
}
