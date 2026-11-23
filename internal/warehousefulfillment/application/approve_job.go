package application

import (
	"context"
	"fmt"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// JobDiscrepancyHandler 批准或驳回库内作业留下的差异。上架和移库从源头补入账。短少在所在格调减。
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
	return &JobDiscrepancyHandler{jobs: jobs, duties: duties, poster: poster, ledger: ledger, holds: holds, clock: clock, anyWarehouse: anyWarehouse}
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
	if item.Carton.String() != "" || item.CartonMissing || (item.Kind != "INTERNAL_MOVE" && item.Kind != "SHORTAGE") {
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
	if item.Kind == "SHORTAGE" {
		lines, reason, err := handler.decrease(ctx, item, rows, item.Location)
		return lines, reason, nil, err
	}
	return handler.replenish(ctx, item, rows)
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
