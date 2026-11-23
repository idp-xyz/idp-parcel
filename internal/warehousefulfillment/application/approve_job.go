package application

import (
	"context"
	"fmt"
	"math"
	"math/bits"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// jobFactLookup 取出形成这份差异的作业事实。差异行只记转入单位和状态，转出的要从原行读。
type jobFactLookup interface {
	Load(context.Context, domain.TenantID, domain.InternalJobFactID) (domain.RecordedInternalJob, bool, error)
}

// goodsFactorLookup 取商品当前修订上的包装折合。形态转换的补入账与登记作业用同一套折合。
type goodsFactorLookup interface {
	LoadLatestGoods(context.Context, domain.TenantID, domain.OwnerAccountID, domain.GoodsCode) (domain.GoodsRevision, bool, error)
}

// JobDiscrepancyHandler 批准或驳回库内作业留下的差异。上架和移库从源头补入账。短少在所在格调减。
// 形态转换与状态转换按原作业原因，从转出单位或状态补进差异所在的转入单位或状态。
// 整箱缺箱只在箱已在账上时从所在位移到报告库位，不新起入库批次。
type JobDiscrepancyHandler struct {
	jobs   ports.JobDiscrepancyStore
	duties ports.ApprovalDutyStore
	poster *PostStockMovementHandler
	ledger interface {
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
	}
	holds        ports.StockHolds
	clock        ports.Clock
	anyWarehouse ports.TenantWarehouseLookup
	picks        interface {
		OpenTasksOn(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.StockCell) ([]ports.PickTask, error)
		SaveOpenTask(context.Context, domain.TenantID, ports.PickTask) error
		InsertOpenTask(context.Context, domain.TenantID, ports.PickTask) error
	}
	pickIDs interface {
		NextPickTaskID() (domain.PickTaskID, error)
	}
	locations ports.StockLocationLookup
	zones     ports.ZoneLookup
	facts     jobFactLookup
	goods     goodsFactorLookup
	cartons   ports.CartonLedger
	cartonIDs ports.CartonIdentities
}

func NewJobDiscrepancyHandler(
	jobs ports.JobDiscrepancyStore,
	duties ports.ApprovalDutyStore,
	poster *PostStockMovementHandler,
	ledger interface {
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
	},
	holds ports.StockHolds,
	clock ports.Clock,
	anyWarehouse ports.TenantWarehouseLookup,
) *JobDiscrepancyHandler {
	handler := &JobDiscrepancyHandler{jobs: jobs, duties: duties, poster: poster, ledger: ledger, holds: holds, clock: clock, anyWarehouse: anyWarehouse}
	if lookup, ok := jobs.(jobFactLookup); ok {
		handler.facts = lookup
	}
	return handler
}

// WithCatalogues 接上商品折合与整箱账。形态转换按折合数补入账，整箱缺箱从整箱账的所在位移到报告库位。
func (handler *JobDiscrepancyHandler) WithCatalogues(
	goods goodsFactorLookup,
	cartons ports.CartonLedger,
	cartonIDs ports.CartonIdentities,
) *JobDiscrepancyHandler {
	handler.goods = goods
	handler.cartons = cartons
	handler.cartonIDs = cartonIDs
	return handler
}

// WithAllocationFollow 接上开放拣货与库位，补入账才能把分配改落到差异所在的格。
func (handler *JobDiscrepancyHandler) WithAllocationFollow(
	picks interface {
		OpenTasksOn(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.StockCell) ([]ports.PickTask, error)
		SaveOpenTask(context.Context, domain.TenantID, ports.PickTask) error
		InsertOpenTask(context.Context, domain.TenantID, ports.PickTask) error
	},
	ids interface {
		NextPickTaskID() (domain.PickTaskID, error)
	},
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
) *JobDiscrepancyHandler {
	handler.picks = picks
	handler.pickIDs = ids
	handler.locations = locations
	handler.zones = zones
	return handler
}

func (handler *JobDiscrepancyHandler) Approve(ctx context.Context, command ApproveCountCommand) (ApprovalResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	if !configured {
		return ApprovalResult{outcome: ApprovalNotConfigured}, nil
	}
	duty, found, err := handler.duties.LoadDuty(ctx, command.Tenant)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	if !found {
		return ApprovalResult{outcome: ApprovalNotConfigured}, nil
	}
	item, found, err := handler.jobs.LoadDiscrepancy(ctx, command.Tenant, command.ID)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	if !found {
		return ApprovalResult{outcome: ApprovalNotFound}, nil
	}
	switch item.Status {
	case "APPROVED":
		return ApprovalResult{outcome: ApprovalExisting, status: item.Status}, nil
	case "REJECTED":
		return ApprovalResult{outcome: ApprovalStatusMismatch, status: item.Status}, nil
	}
	if duty.Distinct && item.Operator == command.Operator.String() {
		return ApprovalResult{outcome: ApprovalDutyBlocked, status: item.Status}, nil
	}
	if item.CartonMissing {
		return handler.approveMissingCarton(ctx, command, item, duty)
	}
	// 箱号在、又不是缺箱，是占着的整箱。占着的整箱仍不批。
	if item.Carton.String() != "" || !approvableJobKind(item.Kind) {
		return ApprovalResult{outcome: ApprovalNotValid, status: item.Status}, nil
	}
	lines, reason, shifts, err := handler.lines(ctx, item)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	if len(lines) == 0 {
		return ApprovalResult{outcome: ApprovalNotValid, status: item.Status}, nil
	}
	source, err := domain.NewSourceFactReference(item.ID.String())
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{
		Tenant:  command.Tenant,
		Request: domain.NewStockMovementRequest(item.Owner, item.Warehouse, reason, source, handler.clock.Now(), lines),
	})
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	if posted.Outcome() != PostStockPosted && posted.Outcome() != PostStockAlreadyPosted {
		return ApprovalResult{outcome: ApprovalNotValid, status: item.Status}, nil
	}
	if err := handler.retargetAllocations(ctx, item, shifts); err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	if err := handler.jobs.MarkDiscrepancyApproved(ctx, command.Tenant, item.ID, command.Operator, handler.clock.Now(), command.Note, duty.Revision); err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	return ApprovalResult{outcome: ApprovalPosted, status: "APPROVED"}, nil
}

func (handler *JobDiscrepancyHandler) Reject(ctx context.Context, command RejectCountCommand) (ApprovalResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("reject job discrepancy: %w", err)
	}
	if !configured {
		return ApprovalResult{outcome: ApprovalNotConfigured}, nil
	}
	if command.Reason == "" {
		return ApprovalResult{}, fmt.Errorf("reject job discrepancy: 缺原因")
	}
	item, found, err := handler.jobs.LoadDiscrepancy(ctx, command.Tenant, command.ID)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("reject job discrepancy: %w", err)
	}
	if !found {
		return ApprovalResult{outcome: ApprovalNotFound}, nil
	}
	switch item.Status {
	case "REJECTED":
		return ApprovalResult{outcome: ApprovalExisting, status: item.Status}, nil
	case "APPROVED":
		return ApprovalResult{outcome: ApprovalStatusMismatch, status: item.Status}, nil
	}
	if err := handler.jobs.MarkDiscrepancyRejected(ctx, command.Tenant, item.ID, command.Operator, handler.clock.Now(), command.Reason); err != nil {
		return ApprovalResult{}, fmt.Errorf("reject job discrepancy: %w", err)
	}
	return ApprovalResult{outcome: ApprovalRejected, status: "REJECTED"}, nil
}

func (handler *JobDiscrepancyHandler) lines(ctx context.Context, item ports.StoredJobDiscrepancy) ([]domain.StockLineRequest, domain.StockMovementReason, []allocationShift, error) {
	rows, err := handler.ledger.ListOnHandCells(ctx, item.Tenant, item.Owner, item.Warehouse)
	if err != nil {
		return nil, 0, nil, err
	}
	switch item.Kind {
	case domain.InternalJobShortage.String():
		lines, reason, err := handler.decrease(ctx, item, rows, item.Location)
		return lines, reason, nil, err
	case domain.InternalJobForm.String(), domain.InternalJobCondition.String():
		return handler.replenishConverted(ctx, item, rows)
	default:
		return handler.replenish(ctx, item, rows)
	}
}

func approvableJobKind(kind string) bool {
	switch kind {
	case domain.InternalJobMove.String(), domain.InternalJobShortage.String(), domain.InternalJobForm.String(), domain.InternalJobCondition.String():
		return true
	default:
		return false
	}
}

type allocationShift struct {
	from domain.StockCell
	to   domain.StockCell
	qty  int64
}

func (handler *JobDiscrepancyHandler) replenish(ctx context.Context, item ports.StoredJobDiscrepancy, rows []ports.StockBalanceRow) ([]domain.StockLineRequest, domain.StockMovementReason, []allocationShift, error) {
	matched := matchingRows(rows, item, item.Source)
	for _, row := range matched {
		cell, err := cellFromBalance(row)
		if err != nil {
			return nil, 0, nil, err
		}
		locked, err := handler.holds.CellLock(ctx, item.Tenant, item.Owner, item.Warehouse, cell)
		if err != nil {
			return nil, 0, nil, err
		}
		if locked > 0 {
			return nil, 0, nil, nil
		}
	}
	need := item.Quantity
	var lines []domain.StockLineRequest
	var shifts []allocationShift
	for _, row := range matched {
		if need == 0 {
			break
		}
		cell, err := cellFromBalance(row)
		if err != nil {
			return nil, 0, nil, err
		}
		held, err := handler.holds.CellHold(ctx, item.Tenant, item.Owner, item.Warehouse, cell)
		if err != nil {
			return nil, 0, nil, err
		}
		allocated, err := handler.holds.Allocated(ctx, item.Tenant, item.Owner, item.Warehouse, cell)
		if err != nil {
			return nil, 0, nil, err
		}
		// 冻结仍记在源头那一格。冻着的数量留下；不够补全数时整笔不成立，避免冻结指着已经搬走的货。
		frozen := holdWithoutAllocation(held, allocated)
		free := row.Quantity - held
		if free < 0 {
			free = 0
		}
		movable := free + allocated
		if cap := row.Quantity - frozen; movable > cap {
			movable = cap
		}
		if movable <= 0 {
			continue
		}
		take := movable
		if take > need {
			take = need
		}
		fromFree := free
		if fromFree > take {
			fromFree = take
		}
		fromAllocated := take - fromFree
		dest, err := retarget(cell, item.Location, item.Unit, item.State)
		if err != nil {
			return nil, 0, nil, err
		}
		if fromAllocated > 0 {
			ok, err := handler.destinationAllocatable(ctx, item.Tenant, item.Warehouse, dest)
			if err != nil {
				return nil, 0, nil, err
			}
			if !ok {
				return nil, 0, nil, nil
			}
			shifts = append(shifts, allocationShift{from: cell, to: dest, qty: fromAllocated})
		}
		out, err := stockLine(domain.StockOut, take, cell)
		if err != nil {
			return nil, 0, nil, err
		}
		in, err := stockLine(domain.StockIn, take, dest)
		if err != nil {
			return nil, 0, nil, err
		}
		lines = append(lines, out, in)
		need -= take
	}
	if need != 0 {
		return nil, 0, nil, nil
	}
	return lines, domain.StockReasonInternalMove, shifts, nil
}

func (handler *JobDiscrepancyHandler) destinationAllocatable(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, cell domain.StockCell) (bool, error) {
	if handler.locations == nil || handler.zones == nil {
		return false, fmt.Errorf("去向能不能承接分配没有接上")
	}
	place, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, cell.Location())
	if err != nil || !found {
		return false, err
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, place.Content().ZoneCode())
	if err != nil || !found {
		return false, err
	}
	return domain.AllocatableGoods(cell.State(), zone.Content().Usage()), nil
}

func (handler *JobDiscrepancyHandler) retargetAllocations(ctx context.Context, item ports.StoredJobDiscrepancy, shifts []allocationShift) error {
	if len(shifts) == 0 {
		return nil
	}
	if handler.picks == nil || handler.pickIDs == nil {
		return fmt.Errorf("分配要跟着货走，拣货任务没有接上")
	}
	for _, shift := range shifts {
		tasks, err := handler.picks.OpenTasksOn(ctx, item.Tenant, item.Owner, item.Warehouse, shift.from)
		if err != nil {
			return err
		}
		left := shift.qty
		for _, task := range tasks {
			if left == 0 {
				break
			}
			if task.Goods != shift.to.Goods() || task.Unit != shift.to.Unit() || task.State != shift.to.State() || task.Alias != shift.to.Alias() || task.Inbound != shift.to.InboundBatch() {
				continue
			}
			open := task.Quantity - task.Picked
			if open <= 0 {
				continue
			}
			if open <= left {
				task.Location = shift.to.Location()
				if err := handler.picks.SaveOpenTask(ctx, item.Tenant, task); err != nil {
					return err
				}
				left -= open
				continue
			}
			id, err := handler.pickIDs.NextPickTaskID()
			if err != nil {
				return err
			}
			task.Quantity -= left
			if err := handler.picks.SaveOpenTask(ctx, item.Tenant, task); err != nil {
				return err
			}
			moved := task
			moved.ID = id
			moved.Location = shift.to.Location()
			moved.Quantity = left
			moved.Picked = 0
			moved.Status = "OPEN"
			moved.Fact = ""
			if err := handler.picks.InsertOpenTask(ctx, item.Tenant, moved); err != nil {
				return err
			}
			left = 0
		}
		if left != 0 {
			return fmt.Errorf("开放拣货少于要跟着走的分配")
		}
	}
	return nil
}

func (handler *JobDiscrepancyHandler) decrease(ctx context.Context, item ports.StoredJobDiscrepancy, rows []ports.StockBalanceRow, location domain.LocationCode) ([]domain.StockLineRequest, domain.StockMovementReason, error) {
	need := item.Quantity
	matched := matchingRows(rows, item, location)
	var lines []domain.StockLineRequest
	for _, row := range matched {
		if need == 0 {
			break
		}
		cell, err := cellFromBalance(row)
		if err != nil {
			return nil, 0, err
		}
		held, err := handler.holds.CellHold(ctx, item.Tenant, item.Owner, item.Warehouse, cell)
		if err != nil {
			return nil, 0, err
		}
		free := row.Quantity - held
		if free <= 0 {
			continue
		}
		take := free
		if take > need {
			take = need
		}
		line, err := stockLine(domain.StockOut, take, cell)
		if err != nil {
			return nil, 0, err
		}
		lines = append(lines, line)
		need -= take
	}
	if need != 0 {
		return nil, 0, nil
	}
	return lines, domain.StockReasonAdjustmentDecrease, nil
}

// replenishConverted 按原作业的转出单位或状态取货，记入差异上的转入单位或状态。
// 开放拣货仍指着转出的单位或状态，这里只取没被占用的数量，不改拣货任务。
func (handler *JobDiscrepancyHandler) replenishConverted(ctx context.Context, item ports.StoredJobDiscrepancy, rows []ports.StockBalanceRow) ([]domain.StockLineRequest, domain.StockMovementReason, []allocationShift, error) {
	if handler.facts == nil {
		return nil, 0, nil, fmt.Errorf("作业事实没有接上")
	}
	job, found, err := handler.facts.Load(ctx, item.Tenant, item.FactID)
	if err != nil {
		return nil, 0, nil, err
	}
	recorded := job.Lines()
	if !found || item.Line < 0 || item.Line >= len(recorded) {
		return nil, 0, nil, nil
	}
	input := recorded[item.Line].Input()
	probe := item
	probe.Unit = input.Unit()
	probe.State = input.State()
	if probe.Unit.String() == "" || probe.State.String() == "" {
		return nil, 0, nil, nil
	}
	fromFactor, toFactor := int64(1), int64(1)
	reason := domain.StockReasonStateChange
	if item.Kind == domain.InternalJobForm.String() {
		reason = domain.StockReasonUnitConversion
		if handler.goods == nil {
			return nil, 0, nil, fmt.Errorf("商品登记册没有接上")
		}
		revision, foundGoods, err := handler.goods.LoadLatestGoods(ctx, item.Tenant, item.Owner, item.Goods)
		if err != nil {
			return nil, 0, nil, err
		}
		if !foundGoods {
			return nil, 0, nil, nil
		}
		var ok bool
		fromFactor, ok = unitFactor(revision, input.Unit())
		if !ok {
			return nil, 0, nil, nil
		}
		toFactor, ok = unitFactor(revision, item.Unit)
		if !ok {
			return nil, 0, nil, nil
		}
	}
	matched := matchingRows(rows, probe, item.Source)
	for _, row := range matched {
		cell, err := cellFromBalance(row)
		if err != nil {
			return nil, 0, nil, err
		}
		locked, err := handler.holds.CellLock(ctx, item.Tenant, item.Owner, item.Warehouse, cell)
		if err != nil {
			return nil, 0, nil, err
		}
		if locked > 0 {
			return nil, 0, nil, nil
		}
	}
	need := item.Quantity
	var lines []domain.StockLineRequest
	for _, row := range matched {
		if need == 0 {
			break
		}
		cell, err := cellFromBalance(row)
		if err != nil {
			return nil, 0, nil, err
		}
		held, err := handler.holds.CellHold(ctx, item.Tenant, item.Owner, item.Warehouse, cell)
		if err != nil {
			return nil, 0, nil, err
		}
		free := row.Quantity - held
		if free < 0 {
			free = 0
		}
		_, canTo := wholeUnits(free, fromFactor, toFactor)
		if canTo <= 0 {
			continue
		}
		takeTo := canTo
		if takeTo > need {
			takeTo = need
		}
		takeFrom, ok := sourceTake(takeTo, fromFactor, toFactor, free)
		for !ok && takeTo > 1 {
			takeTo--
			takeFrom, ok = sourceTake(takeTo, fromFactor, toFactor, free)
		}
		if !ok {
			continue
		}
		dest, err := retarget(cell, item.Location, item.Unit, item.State)
		if err != nil {
			return nil, 0, nil, err
		}
		out, err := stockLine(domain.StockOut, takeFrom, cell)
		if err != nil {
			return nil, 0, nil, err
		}
		in, err := stockLine(domain.StockIn, takeTo, dest)
		if err != nil {
			return nil, 0, nil, err
		}
		lines = append(lines, out, in)
		need -= takeTo
	}
	if need != 0 {
		return nil, 0, nil, nil
	}
	return lines, reason, nil, nil
}

// sourceTake 把转入数量折回转出数量。折合与登记作业相同：转出 = 转入 × 转入折合 / 转出折合。
func sourceTake(takeTo, fromFactor, toFactor, free int64) (int64, bool) {
	if takeTo <= 0 || fromFactor <= 0 || toFactor <= 0 || free <= 0 {
		return 0, false
	}
	hi, lo := bits.Mul64(uint64(takeTo), uint64(toFactor))
	if hi != 0 || lo%uint64(fromFactor) != 0 {
		return 0, false
	}
	taken := lo / uint64(fromFactor)
	if taken == 0 || taken > uint64(free) || taken > uint64(math.MaxInt64) {
		return 0, false
	}
	return int64(taken), true
}

// approveMissingCarton 只在箱已经在账上时，从它所在的库位移到报告库位。不在账上批准不成立，不改账。
// 已经在报告库位就只改差异状态。占着的箱不搬。
func (handler *JobDiscrepancyHandler) approveMissingCarton(ctx context.Context, command ApproveCountCommand, item ports.StoredJobDiscrepancy, duty ports.ApprovalDuty) (ApprovalResult, error) {
	if handler.cartons == nil || handler.cartonIDs == nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: 整箱账没有接上")
	}
	if item.Location.String() == "" || item.Carton.String() == "" {
		return ApprovalResult{outcome: ApprovalNotValid, status: item.Status}, nil
	}
	place, err := handler.cartons.Locate(ctx, item.Tenant, item.Owner, item.Warehouse, item.Carton)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	if !place.Found {
		return ApprovalResult{outcome: ApprovalNotValid, status: item.Status}, nil
	}
	held, err := handler.holds.CartonHeld(ctx, item.Tenant, item.Owner, item.Warehouse, item.Carton)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	if held {
		return ApprovalResult{outcome: ApprovalNotValid, status: item.Status}, nil
	}
	if place.Location != item.Location {
		if err := handler.relocateMissingCarton(ctx, item, place); err != nil {
			again, locateErr := handler.cartons.Locate(ctx, item.Tenant, item.Owner, item.Warehouse, item.Carton)
			if locateErr != nil || !again.Found || again.Location != item.Location {
				return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
			}
		}
	}
	if err := handler.jobs.MarkDiscrepancyApproved(ctx, command.Tenant, item.ID, command.Operator, handler.clock.Now(), command.Note, duty.Revision); err != nil {
		return ApprovalResult{}, fmt.Errorf("approve job discrepancy: %w", err)
	}
	return ApprovalResult{outcome: ApprovalPosted, status: "APPROVED"}, nil
}

// relocateMissingCarton 沿用箱此刻的入库批次，记一笔相抵的库内移动。来源用差异标识，同一份批准重放时整箱册认得出已经记过。
func (handler *JobDiscrepancyHandler) relocateMissingCarton(ctx context.Context, item ports.StoredJobDiscrepancy, place ports.CartonPlace) error {
	movementID, err := handler.cartonIDs.NextCartonMovementID()
	if err != nil {
		return err
	}
	out, err := domain.NewCartonMovementDeltaLine(place.Location, item.Carton, place.Batch, -1)
	if err != nil {
		return err
	}
	in, err := domain.NewCartonMovementDeltaLine(item.Location, item.Carton, place.Batch, 1)
	if err != nil {
		return err
	}
	source, err := domain.NewSourceFactReference(item.ID.String())
	if err != nil {
		return err
	}
	now := handler.clock.Now()
	movement, err := domain.NewCartonMovement(
		item.Tenant, movementID, item.Owner, item.Warehouse, domain.CartonMovementInternalMove,
		source, now, now, []domain.CartonMovementLine{out, in},
	)
	if err != nil {
		return err
	}
	return handler.cartons.AppendRelocation(ctx, movement)
}

func matchingRows(rows []ports.StockBalanceRow, item ports.StoredJobDiscrepancy, location domain.LocationCode) []ports.StockBalanceRow {
	var preferred, rest []ports.StockBalanceRow
	for _, row := range rows {
		if row.LocationCode != location.String() || row.GoodsCode != item.Goods.String() || row.Unit != item.Unit.String() || row.State != item.State.String() {
			continue
		}
		if rowText(row.Alias) != item.Alias.String() || rowText(row.BatchNumber) != item.Batch.String() || rowText(row.Expiry) != item.Expiry.String() {
			continue
		}
		if item.Inbound.String() != "" && row.InboundBatchID == item.Inbound.String() {
			preferred = append(preferred, row)
			continue
		}
		rest = append(rest, row)
	}
	return append(preferred, rest...)
}

func rowText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
