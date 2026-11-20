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
	lines, reason, err := handler.lines(ctx, item)
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

func (handler *JobDiscrepancyHandler) lines(ctx context.Context, item ports.StoredJobDiscrepancy) ([]domain.StockLineRequest, domain.StockMovementReason, error) {
	rows, err := handler.ledger.ListOnHandCells(ctx, item.Tenant, item.Owner, item.Warehouse)
	if err != nil {
		return nil, 0, err
	}
	if item.Kind == "SHORTAGE" {
		return handler.decrease(ctx, item, rows, item.Location)
	}
	return handler.replenish(ctx, item, rows)
}

func (handler *JobDiscrepancyHandler) replenish(ctx context.Context, item ports.StoredJobDiscrepancy, rows []ports.StockBalanceRow) ([]domain.StockLineRequest, domain.StockMovementReason, error) {
	need := item.Quantity
	matched := matchingRows(rows, item, item.Source)
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
		out, err := stockLine(domain.StockOut, take, cell)
		if err != nil {
			return nil, 0, err
		}
		dest, err := retarget(cell, item.Location, item.Unit, item.State)
		if err != nil {
			return nil, 0, err
		}
		in, err := stockLine(domain.StockIn, take, dest)
		if err != nil {
			return nil, 0, err
		}
		lines = append(lines, out, in)
		need -= take
	}
	if need != 0 {
		return nil, 0, nil
	}
	return lines, domain.StockReasonInternalMove, nil
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
