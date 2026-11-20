package application

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 库存账的判重位置是（租户，原因，来源事实引用），不含货主。一条事实可以带几个货主，
// 来源引用带上货主，后一个货主的流水才不会被判成前一个货主的重投。
func internalJobSource(fact domain.InternalJobFactID, owner domain.OwnerAccountID) (domain.SourceFactReference, error) {
	return domain.NewSourceFactReference(fact.String() + "#" + owner.String())
}

func basesEqual(leftQty, leftFactor, rightQty, rightFactor int64) bool {
	if leftQty < 0 || rightQty < 0 || leftFactor <= 0 || rightFactor <= 0 {
		return false
	}
	leftHi, leftLo := bits.Mul64(uint64(leftQty), uint64(leftFactor))
	rightHi, rightLo := bits.Mul64(uint64(rightQty), uint64(rightFactor))
	return leftHi == rightHi && leftLo == rightLo
}

type poolCell struct {
	cell domain.StockCell
	at   time.Time
	qty  int64
	hold int64
}

func (cell poolCell) free() int64 {
	if cell.qty <= cell.hold {
		return 0
	}
	return cell.qty - cell.hold
}

type stockBook struct {
	cells []poolCell
}

func (book *stockBook) add(cell domain.StockCell, at time.Time, qty int64) {
	for index := range book.cells {
		if book.cells[index].cell == cell {
			book.cells[index].qty += qty
			return
		}
	}
	book.cells = append(book.cells, poolCell{cell: cell, at: at, qty: qty})
}

type takenChunk struct {
	cell domain.StockCell
	at   time.Time
	qty  int64
}

type takePlan struct {
	from    int64
	to      int64
	sources []takenChunk
	dests   []takenChunk
	onBook  int64
	freeSum int64
	holdSum int64
}

func (handler *RegisterInternalJobHandler) post(
	ctx context.Context,
	command RegisterInternalJobCommand,
	places map[string]placeInfo,
	revisions map[string]domain.GoodsRevision,
) ([]domain.RecordedInternalJobLine, []domain.StockDiscrepancy, []domain.InternalJobIssue, error) {
	if err := handler.lockOwners(ctx, command); err != nil {
		return nil, nil, nil, err
	}
	cartonAt := map[int]ports.CartonPlace{}
	var outbound []domain.InternalJobIssue
	for index, line := range command.Lines {
		if !line.CartonLine() || command.Kind != domain.InternalJobMove {
			continue
		}
		place, err := handler.cartons.Locate(ctx, command.Tenant, line.Owner(), command.Warehouse, line.Carton())
		if err != nil {
			return nil, nil, nil, err
		}
		cartonAt[index] = place
		if !place.Found {
			continue
		}
		usage, found, err := handler.place(ctx, command.Tenant, command.Warehouse, place.Location, places)
		if err != nil {
			return nil, nil, nil, err
		}
		if found && usage == domain.ZoneUsageOutboundStaging {
			outbound = append(outbound, domain.NewInternalJobIssue(index, domain.InternalJobIssueOutboundStaging))
		}
	}
	if len(outbound) > 0 {
		return nil, nil, outbound, nil
	}
	books := map[string]*stockBook{}
	for _, owner := range ownersOf(command.Lines) {
		if !ownerHasGoods(command.Lines, owner) || command.Kind == domain.InternalJobShortage {
			continue
		}
		book, err := handler.loadBook(ctx, command.Tenant, owner, command.Warehouse)
		if err != nil {
			return nil, nil, nil, err
		}
		books[owner.String()] = book
	}
	recorded := make([]domain.RecordedInternalJobLine, len(command.Lines))
	var discrepancies []domain.StockDiscrepancy
	pending := map[string]*pendingStock{}
	for index, line := range command.Lines {
		var (
			result domain.InternalJobLineResult
			item   *domain.StockDiscrepancy
			err    error
		)
		switch {
		case command.Kind == domain.InternalJobShortage:
			result, item, err = handler.shortageLine(command, index, line)
		case line.CartonLine():
			result, item, err = handler.cartonLine(ctx, command, index, line, cartonAt[index])
		default:
			result, item, err = handler.goodsLine(ctx, command, index, line, books[line.Owner().String()], places, revisions, pending)
		}
		if err != nil {
			return nil, nil, nil, err
		}
		recorded[index] = domain.NewRecordedInternalJobLine(line, result)
		if item != nil {
			discrepancies = append(discrepancies, *item)
		}
	}
	if err := handler.flushStock(ctx, command, pending, recorded); err != nil {
		return nil, nil, nil, err
	}
	return recorded, discrepancies, nil, nil
}

func (handler *RegisterInternalJobHandler) lockOwners(ctx context.Context, command RegisterInternalJobCommand) error {
	for _, owner := range ownersOf(command.Lines) {
		if ownerHasGoods(command.Lines, owner) && command.Kind != domain.InternalJobShortage {
			if err := handler.ledger.SerializePosting(ctx, command.Tenant, owner, command.Warehouse); err != nil {
				return err
			}
		}
		if ownerHasCarton(command.Lines, owner) && command.Kind == domain.InternalJobMove {
			if err := handler.cartons.Serialize(ctx, command.Tenant, owner, command.Warehouse); err != nil {
				return err
			}
		}
	}
	return nil
}

func (handler *RegisterInternalJobHandler) loadBook(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) (*stockBook, error) {
	rows, err := handler.onHand.ListOnHandCells(ctx, tenant, owner, warehouse)
	if err != nil {
		return nil, err
	}
	book := &stockBook{}
	for _, row := range rows {
		cell, err := cellFromBalance(row)
		if err != nil {
			return nil, err
		}
		batch, found, err := handler.ledger.LoadInboundBatch(ctx, tenant, cell.InboundBatch())
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("在库的格 %s 没有入库批次 %s", cell.Location(), cell.InboundBatch())
		}
		hold, err := handler.holds.CellHold(ctx, tenant, owner, warehouse, cell)
		if err != nil {
			return nil, err
		}
		if hold < 0 {
			return nil, fmt.Errorf("占用为负")
		}
		book.cells = append(book.cells, poolCell{cell: cell, at: batch.InboundAt(), qty: row.Quantity, hold: hold})
	}
	slices.SortFunc(book.cells, func(left, right poolCell) int {
		if left.at.Before(right.at) {
			return -1
		}
		if right.at.Before(left.at) {
			return 1
		}
		if left.cell.InboundBatch().String() < right.cell.InboundBatch().String() {
			return -1
		}
		if left.cell.InboundBatch().String() > right.cell.InboundBatch().String() {
			return 1
		}
		return 0
	})
	return book, nil
}

func (handler *RegisterInternalJobHandler) goodsLine(
	ctx context.Context,
	command RegisterInternalJobCommand,
	index int,
	line domain.InternalJobLine,
	book *stockBook,
	places map[string]placeInfo,
	revisions map[string]domain.GoodsRevision,
	pending map[string]*pendingStock,
) (domain.InternalJobLineResult, *domain.StockDiscrepancy, error) {
	destLocation := line.Target()
	destUnit := line.Unit()
	destState := line.State()
	wantTo := line.Quantity()
	fromFactor, toFactor := int64(1), int64(1)
	reason := domain.StockReasonInternalMove
	switch command.Kind {
	case domain.InternalJobForm:
		destLocation = line.Source()
		destUnit = line.ToUnit()
		wantTo = line.ToQuantity()
		reason = domain.StockReasonUnitConversion
	case domain.InternalJobCondition:
		destLocation = line.Source()
		destState = line.ToState()
		reason = domain.StockReasonStateChange
	}
	if line.ToUnit().String() != "" && command.Kind == domain.InternalJobMove {
		destUnit = line.ToUnit()
		wantTo = line.ToQuantity()
		reason = domain.StockReasonUnitConversion
	}
	if reason == domain.StockReasonUnitConversion {
		revision := revisions[line.Owner().String()+"\x1f"+line.Goods().String()]
		var ok bool
		fromFactor, ok = unitFactor(revision, line.Unit())
		if !ok {
			return domain.InternalJobLineResult{}, nil, fmt.Errorf("转出单位没有折合数")
		}
		toFactor, ok = unitFactor(revision, destUnit)
		if !ok {
			return domain.InternalJobLineResult{}, nil, fmt.Errorf("转入单位没有折合数")
		}
	}
	if command.Kind == domain.InternalJobMove {
		if info, found := places[destLocation.String()]; found && info.usage == domain.ZoneUsageDefective {
			destState = domain.StockStateDefective
		}
	}
	destUsage := places[destLocation.String()].usage
	leave := !(destState == domain.StockStateGood && (destUsage == domain.ZoneUsageStorage || destUsage == domain.ZoneUsagePicking))
	reservation, err := handler.holds.UnassignedReservation(ctx, command.Tenant, line.Owner(), command.Warehouse, line.Goods(), line.State(), line.Alias(), line.Batch(), line.Expiry())
	if err != nil {
		return domain.InternalJobLineResult{}, nil, err
	}
	if reservation < 0 {
		return domain.InternalJobLineResult{}, nil, fmt.Errorf("预占为负")
	}
	match := matchLine(line)
	allocatable := func(cell domain.StockCell) bool {
		info, found := places[cell.Location().String()]
		return found && cell.State() == domain.StockStateGood && (info.usage == domain.ZoneUsageStorage || info.usage == domain.ZoneUsagePicking)
	}
	natural := book.plan(match, fromFactor, toFactor, line.Quantity(), wantTo, math.MaxInt64, false, allocatable, destLocation, destUnit, destState)
	var allocFree int64
	for _, cell := range book.cells {
		if match(cell.cell) && allocatable(cell.cell) {
			allocFree += cell.free()
		}
	}
	headroom := allocFree - reservation
	if !leave || headroom < 0 {
		if !leave {
			headroom = math.MaxInt64
		} else if headroom < 0 {
			headroom = 0
		}
	}
	actual := book.plan(match, fromFactor, toFactor, line.Quantity(), wantTo, headroom, true, allocatable, destLocation, destUnit, destState)
	heldFromCells := min64(line.Quantity(), actual.onBook) - actual.freeSum
	if heldFromCells < 0 {
		heldFromCells = 0
	}
	short := line.Quantity() - actual.onBook
	if short < 0 {
		short = 0
	}
	held := heldFromCells + (natural.from - actual.from)
	result := domain.InternalJobLineResult{Posted: actual.from, PostedInbound: actual.to, ShortOnBook: short, Held: held}
	for _, chunk := range actual.sources {
		result.Batches = append(result.Batches, chunk.cell.InboundBatch())
		out, err := stockLine(domain.StockOut, chunk.qty, chunk.cell)
		if err != nil {
			return domain.InternalJobLineResult{}, nil, err
		}
		inCell, err := retarget(chunk.cell, destLocation, destUnit, destState)
		if err != nil {
			return domain.InternalJobLineResult{}, nil, err
		}
		inQty := chunk.qty
		if reason == domain.StockReasonUnitConversion {
			inQty = chunk.qty * fromFactor / toFactor
		}
		in, err := stockLine(domain.StockIn, inQty, inCell)
		if err != nil {
			return domain.InternalJobLineResult{}, nil, err
		}
		key := line.Owner().String() + "\x1f" + reason.String()
		group := pending[key]
		if group == nil {
			group = &pendingStock{owner: line.Owner(), reason: reason}
			pending[key] = group
		}
		group.lines = append(group.lines, out, in)
		group.linesTouched = append(group.linesTouched, index)
	}
	unposted := wantTo - actual.to
	if unposted <= 0 {
		return result, nil, nil
	}
	inbound := line.NamedBatch()
	if inbound.String() == "" && len(actual.sources) == 1 {
		inbound = actual.sources[0].cell.InboundBatch()
	}
	item, err := handler.discrepancy(command, index, line, destLocation, destUnit, destState, inbound, line.Source(), domain.CartonNumber{}, line.Note(), unposted, short, held, false)
	if err != nil {
		return domain.InternalJobLineResult{}, nil, err
	}
	result.Discrepancy = item.ID()
	return result, &item, nil
}

func (book *stockBook) plan(
	match func(domain.StockCell) bool,
	fromFactor, toFactor, wantFrom, wantTo, headroom int64,
	mutate bool,
	allocatable func(domain.StockCell) bool,
	destLocation domain.LocationCode,
	destUnit domain.GoodsUnitName,
	destState domain.StockState,
) takePlan {
	var plan takePlan
	for _, cell := range book.cells {
		if !match(cell.cell) {
			continue
		}
		plan.onBook += cell.qty
		plan.freeSum += cell.free()
		capped := cell.hold
		if capped > cell.qty {
			capped = cell.qty
		}
		plan.holdSum += capped
	}
	remainingTo := wantTo
	room := headroom
	for index := range book.cells {
		current := book.cells[index]
		if !match(current.cell) || remainingTo == 0 {
			continue
		}
		canFrom, canTo := wholeUnits(current.free(), fromFactor, toFactor)
		canTo = min64(canTo, remainingTo)
		for canTo > 0 && (canTo*toFactor)%fromFactor != 0 {
			canTo--
		}
		if canTo == 0 {
			continue
		}
		canFrom = canTo * toFactor / fromFactor
		if canFrom > current.free() {
			continue
		}
		if allocatable(current.cell) && room != math.MaxInt64 {
			for canTo > 0 && canFrom > room {
				canTo--
				for canTo > 0 && (canTo*toFactor)%fromFactor != 0 {
					canTo--
				}
				if canTo == 0 {
					break
				}
				canFrom = canTo * toFactor / fromFactor
			}
			if canTo == 0 || canFrom > room {
				continue
			}
		}
		if mutate {
			book.cells[index].qty -= canFrom
			if allocatable(current.cell) && room != math.MaxInt64 {
				room -= canFrom
			}
			dest, err := retarget(current.cell, destLocation, destUnit, destState)
			if err != nil {
				continue
			}
			book.add(dest, current.at, canTo)
		}
		plan.from += canFrom
		plan.to += canTo
		remainingTo -= canTo
		plan.sources = append(plan.sources, takenChunk{cell: current.cell, at: current.at, qty: canFrom})
		plan.dests = append(plan.dests, takenChunk{qty: canTo})
	}
	_ = wantFrom
	return plan
}

func wholeUnits(free, fromFactor, toFactor int64) (int64, int64) {
	if free <= 0 || fromFactor <= 0 || toFactor <= 0 {
		return 0, 0
	}
	hi, lo := bits.Mul64(uint64(free), uint64(fromFactor))
	if hi != 0 {
		return 0, 0
	}
	made := lo / uint64(toFactor)
	for made > 0 {
		used := made * uint64(toFactor)
		if used%uint64(fromFactor) == 0 {
			return int64(used / uint64(fromFactor)), int64(made)
		}
		made--
	}
	return 0, 0
}

func (handler *RegisterInternalJobHandler) flushStock(
	ctx context.Context,
	command RegisterInternalJobCommand,
	pending map[string]*pendingStock,
	recorded []domain.RecordedInternalJobLine,
) error {
	for _, group := range pending {
		if len(group.lines) == 0 {
			continue
		}
		source, err := internalJobSource(command.FactID, group.owner)
		if err != nil {
			return err
		}
		request := domain.NewStockMovementRequest(group.owner, command.Warehouse, group.reason, source, command.OccurredAt, group.lines)
		posted, err := handler.poster.Post(ctx, PostStockMovementCommand{Tenant: command.Tenant, Request: request})
		if err != nil {
			return err
		}
		if posted.Outcome() != PostStockPosted {
			return fmt.Errorf("库内流水没有入账：%s", posted.Outcome())
		}
		movement, ok := posted.Movement()
		if !ok {
			return fmt.Errorf("库内流水没有交回")
		}
		seen := map[int]struct{}{}
		for _, index := range group.linesTouched {
			if _, done := seen[index]; done {
				continue
			}
			seen[index] = struct{}{}
			result := recorded[index].Result()
			result.Movement = movement.ID().String()
			recorded[index] = domain.NewRecordedInternalJobLine(recorded[index].Input(), result)
		}
	}
	return nil
}

type pendingStock struct {
	owner        domain.OwnerAccountID
	reason       domain.StockMovementReason
	lines        []domain.StockLineRequest
	linesTouched []int
}

func (handler *RegisterInternalJobHandler) cartonLine(
	ctx context.Context,
	command RegisterInternalJobCommand,
	index int,
	line domain.InternalJobLine,
	place ports.CartonPlace,
) (domain.InternalJobLineResult, *domain.StockDiscrepancy, error) {
	if !place.Found {
		item, err := handler.discrepancy(command, index, line, line.Target(), domain.GoodsUnitName{}, domain.StockStateInvalid, domain.InboundBatchID{}, domain.LocationCode{}, line.Carton(), "", 1, 0, 0, true)
		if err != nil {
			return domain.InternalJobLineResult{}, nil, err
		}
		return domain.InternalJobLineResult{CartonMissing: true, Discrepancy: item.ID()}, &item, nil
	}
	held, err := handler.holds.CartonHeld(ctx, command.Tenant, line.Owner(), command.Warehouse, line.Carton())
	if err != nil {
		return domain.InternalJobLineResult{}, nil, err
	}
	if held {
		item, err := handler.discrepancy(command, index, line, line.Target(), domain.GoodsUnitName{}, domain.StockStateInvalid, domain.InboundBatchID{}, place.Location, line.Carton(), "", 1, 0, 1, false)
		if err != nil {
			return domain.InternalJobLineResult{}, nil, err
		}
		return domain.InternalJobLineResult{Held: 1, Discrepancy: item.ID()}, &item, nil
	}
	movementID, err := handler.cartonIDs.NextCartonMovementID()
	if err != nil {
		return domain.InternalJobLineResult{}, nil, err
	}
	out, err := domain.NewCartonMovementDeltaLine(place.Location, line.Carton(), place.Batch, -1)
	if err != nil {
		return domain.InternalJobLineResult{}, nil, err
	}
	in, err := domain.NewCartonMovementDeltaLine(line.Target(), line.Carton(), place.Batch, 1)
	if err != nil {
		return domain.InternalJobLineResult{}, nil, err
	}
	source, err := internalJobSource(command.FactID, line.Owner())
	if err != nil {
		return domain.InternalJobLineResult{}, nil, err
	}
	movement, err := domain.NewCartonMovement(
		command.Tenant, movementID, line.Owner(), command.Warehouse, domain.CartonMovementInternalMove,
		source, command.OccurredAt, handler.clock.Now(), []domain.CartonMovementLine{out, in},
	)
	if err != nil {
		return domain.InternalJobLineResult{}, nil, err
	}
	if err := handler.cartons.AppendRelocation(ctx, movement); err != nil {
		return domain.InternalJobLineResult{}, nil, err
	}
	return domain.InternalJobLineResult{Posted: 1, PostedInbound: 1, Movement: movementID.String()}, nil, nil
}

func (handler *RegisterInternalJobHandler) shortageLine(
	command RegisterInternalJobCommand,
	index int,
	line domain.InternalJobLine,
) (domain.InternalJobLineResult, *domain.StockDiscrepancy, error) {
	quantity := line.Quantity()
	location := line.Source()
	if line.CartonLine() {
		quantity = 1
		location = domain.LocationCode{}
	}
	item, err := handler.discrepancy(command, index, line, location, line.Unit(), line.State(), line.NamedBatch(), line.Source(), line.Carton(), line.Note(), quantity, 0, 0, false)
	if err != nil {
		return domain.InternalJobLineResult{}, nil, err
	}
	return domain.InternalJobLineResult{Discrepancy: item.ID()}, &item, nil
}

func (handler *RegisterInternalJobHandler) discrepancy(
	command RegisterInternalJobCommand,
	index int,
	line domain.InternalJobLine,
	location domain.LocationCode,
	unit domain.GoodsUnitName,
	state domain.StockState,
	inbound domain.InboundBatchID,
	source domain.LocationCode,
	carton domain.CartonNumber,
	note string,
	quantity, short, held int64,
	missing bool,
) (domain.StockDiscrepancy, error) {
	id, err := handler.discrepancies.NextStockDiscrepancyID()
	if err != nil {
		return domain.StockDiscrepancy{}, err
	}
	return domain.NewStockDiscrepancy(domain.StockDiscrepancySpec{
		Tenant: command.Tenant, ID: id, FactID: command.FactID, Line: index, Owner: line.Owner(), Warehouse: command.Warehouse,
		Location: location, Goods: line.Goods(), Unit: unit, State: state, Alias: line.Alias(), Batch: line.Batch(), Expiry: line.Expiry(),
		InboundBatch: inbound, Source: source, Carton: carton, Note: note, Quantity: quantity, ShortOnBook: short, Held: held, CartonMissing: missing,
	})
}

func matchLine(line domain.InternalJobLine) func(domain.StockCell) bool {
	return func(cell domain.StockCell) bool {
		if cell.Location() != line.Source() || cell.Goods() != line.Goods() || cell.Unit() != line.Unit() || cell.State() != line.State() {
			return false
		}
		if cell.Alias() != line.Alias() || cell.BatchNumber() != line.Batch() || cell.Expiry() != line.Expiry() {
			return false
		}
		return line.NamedBatch().String() == "" || cell.InboundBatch() == line.NamedBatch()
	}
}

func stockLine(direction domain.StockDirection, quantity int64, cell domain.StockCell) (domain.StockLineRequest, error) {
	if quantity <= 0 {
		return domain.StockLineRequest{}, fmt.Errorf("流水数量不为正")
	}
	return domain.NewStockLineRequest(direction, quantity, cell, time.Time{}), nil
}

func retarget(cell domain.StockCell, location domain.LocationCode, unit domain.GoodsUnitName, state domain.StockState) (domain.StockCell, error) {
	return domain.NewStockCell(domain.StockCellSpec{
		Location: location, Goods: cell.Goods(), Unit: unit, State: state, Alias: cell.Alias(),
		InboundBatch: cell.InboundBatch(), BatchNumber: cell.BatchNumber(), Expiry: cell.Expiry(),
	})
}

func cellFromBalance(row ports.StockBalanceRow) (domain.StockCell, error) {
	location, err := domain.NewLocationCode(row.LocationCode)
	if err != nil {
		return domain.StockCell{}, err
	}
	goods, err := domain.NewGoodsCode(row.GoodsCode)
	if err != nil {
		return domain.StockCell{}, err
	}
	unit, err := domain.NewGoodsUnitName(row.Unit)
	if err != nil {
		return domain.StockCell{}, err
	}
	state, err := domain.NewStockState(row.State)
	if err != nil {
		return domain.StockCell{}, err
	}
	inbound, err := domain.NewInboundBatchID(row.InboundBatchID)
	if err != nil {
		return domain.StockCell{}, err
	}
	var alias domain.GoodsAliasCode
	if row.Alias != nil {
		alias, err = domain.NewGoodsAliasCode(*row.Alias)
		if err != nil {
			return domain.StockCell{}, err
		}
	}
	var batch domain.BatchNumber
	if row.BatchNumber != nil {
		batch, err = domain.NewBatchNumber(*row.BatchNumber)
		if err != nil {
			return domain.StockCell{}, err
		}
	}
	var expiry domain.ExpiryDate
	if row.Expiry != nil {
		expiry, err = domain.NewExpiryDate(*row.Expiry)
		if err != nil {
			return domain.StockCell{}, err
		}
	}
	return domain.NewStockCell(domain.StockCellSpec{
		Location: location, Goods: goods, Unit: unit, State: state, Alias: alias,
		InboundBatch: inbound, BatchNumber: batch, Expiry: expiry,
	})
}

func ownersOf(lines []domain.InternalJobLine) []domain.OwnerAccountID {
	seen := map[string]struct{}{}
	var owners []domain.OwnerAccountID
	for _, line := range lines {
		if _, ok := seen[line.Owner().String()]; ok {
			continue
		}
		seen[line.Owner().String()] = struct{}{}
		owners = append(owners, line.Owner())
	}
	slices.SortFunc(owners, func(left, right domain.OwnerAccountID) int {
		switch {
		case left.String() < right.String():
			return -1
		case left.String() > right.String():
			return 1
		default:
			return 0
		}
	})
	return owners
}

func ownerHasGoods(lines []domain.InternalJobLine, owner domain.OwnerAccountID) bool {
	for _, line := range lines {
		if line.Owner() == owner && !line.CartonLine() {
			return true
		}
	}
	return false
}

func ownerHasCarton(lines []domain.InternalJobLine, owner domain.OwnerAccountID) bool {
	for _, line := range lines {
		if line.Owner() == owner && line.CartonLine() {
			return true
		}
	}
	return false
}

func min64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
