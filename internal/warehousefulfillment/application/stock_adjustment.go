package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// SubmitAdjustmentCommand 是提交一份库存调整。调整标识由调用方给。
type SubmitAdjustmentCommand struct {
	Tenant    domain.TenantID
	ID        domain.RequestKey
	Owner     domain.OwnerAccountID
	Warehouse domain.WarehouseCode
	Reason    string
	Operator  domain.Registrant
	Lines     []ports.AdjustmentLine
}

// AdjustmentResult 是提交、批准或驳回的结果。
type AdjustmentResult struct {
	outcome ApprovalOutcome
	status  string
	issues  []domain.FreezeIssue
}

func (result AdjustmentResult) Outcome() ApprovalOutcome { return result.outcome }
func (result AdjustmentResult) Status() string           { return result.status }
func (result AdjustmentResult) Issues() []domain.FreezeIssue {
	return append([]domain.FreezeIssue(nil), result.issues...)
}

// AdjustmentHandler 提交库存调整，并在批准时入账。没登记批准职责规则时批准答未配置。
type AdjustmentHandler struct {
	adjustments ports.AdjustmentStore
	duties      ports.ApprovalDutyStore
	poster      *PostStockMovementHandler
	ledger      interface {
		CellBalance(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.StockCell) (int64, error)
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
		LoadInboundBatch(context.Context, domain.TenantID, domain.InboundBatchID) (domain.InboundBatch, bool, error)
	}
	warehouses   ports.WarehouseLookup
	locations    ports.StockLocationLookup
	zones        ports.ZoneLookup
	goods        ports.StockGoodsLookup
	owners       ports.OwnerAccountLookup
	holds        ports.StockHolds
	clock        ports.Clock
	anyWarehouse ports.TenantWarehouseLookup
}

func NewAdjustmentHandler(
	adjustments ports.AdjustmentStore,
	duties ports.ApprovalDutyStore,
	poster *PostStockMovementHandler,
	ledger interface {
		CellBalance(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.StockCell) (int64, error)
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
		LoadInboundBatch(context.Context, domain.TenantID, domain.InboundBatchID) (domain.InboundBatch, bool, error)
	},
	warehouses ports.WarehouseLookup,
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
	goods ports.StockGoodsLookup,
	owners ports.OwnerAccountLookup,
	holds ports.StockHolds,
	clock ports.Clock,
	anyWarehouse ports.TenantWarehouseLookup,
) *AdjustmentHandler {
	return &AdjustmentHandler{
		adjustments: adjustments, duties: duties, poster: poster, ledger: ledger, warehouses: warehouses,
		locations: locations, zones: zones, goods: goods, owners: owners, holds: holds, clock: clock, anyWarehouse: anyWarehouse,
	}
}

func (handler *AdjustmentHandler) Submit(ctx context.Context, command SubmitAdjustmentCommand) (AdjustmentResult, error) {
	if command.Tenant.String() == "" || command.ID.String() == "" || command.Owner.String() == "" || command.Warehouse.String() == "" || command.Reason == "" || command.Operator.String() == "" || len(command.Lines) == 0 {
		return AdjustmentResult{}, fmt.Errorf("submit adjustment: 命令不完整")
	}
	if handler.owners == nil {
		return AdjustmentResult{}, fmt.Errorf("submit adjustment: %w", errOwnerLookupMissing)
	}
	registered, err := handler.owners.OwnerAccountRegistered(ctx, command.Tenant, command.Owner)
	if err != nil {
		return AdjustmentResult{}, fmt.Errorf("submit adjustment: %w", err)
	}
	if !registered {
		return AdjustmentResult{}, ErrOwnerAccountNotInTenant
	}
	if err := handler.adjustments.Hold(ctx, command.Tenant, command.ID); err != nil {
		return AdjustmentResult{}, fmt.Errorf("submit adjustment: %w", err)
	}
	existing, found, err := handler.adjustments.Load(ctx, command.Tenant, command.ID)
	if err != nil {
		return AdjustmentResult{}, fmt.Errorf("submit adjustment: %w", err)
	}
	if found {
		if sameAdjustment(existing, command) {
			return AdjustmentResult{outcome: ApprovalExisting, status: existing.Status}, nil
		}
		return AdjustmentResult{outcome: ApprovalConflict}, nil
	}
	warehouse, foundWarehouse, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return AdjustmentResult{}, fmt.Errorf("submit adjustment: %w", err)
	}
	if !foundWarehouse {
		return AdjustmentResult{outcome: ApprovalWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return AdjustmentResult{outcome: ApprovalWarehouseNotAccepting}, nil
	}
	lines, issues, err := handler.screen(ctx, command, false)
	if err != nil {
		return AdjustmentResult{}, fmt.Errorf("submit adjustment: %w", err)
	}
	if len(issues) > 0 {
		return AdjustmentResult{outcome: ApprovalNotValid, issues: issues}, nil
	}
	adjustment := ports.StockAdjustment{
		ID: command.ID, Owner: command.Owner, Warehouse: command.Warehouse, Reason: command.Reason,
		Operator: command.Operator, Submitted: handler.clock.Now(), Status: "PENDING", Lines: lines,
	}
	if err := handler.adjustments.Save(ctx, command.Tenant, adjustment); err != nil {
		return AdjustmentResult{}, fmt.Errorf("submit adjustment: %w", err)
	}
	return AdjustmentResult{outcome: ApprovalSubmitted, status: "PENDING"}, nil
}

func (handler *AdjustmentHandler) Approve(ctx context.Context, command ApproveAdjustmentCommand) (AdjustmentResult, error) {
	configured, duty, result, err := handler.ready(ctx, command.Tenant, "approve adjustment")
	if err != nil || !configured {
		return result, err
	}
	adjustment, found, err := handler.adjustments.Load(ctx, command.Tenant, command.ID)
	if err != nil {
		return AdjustmentResult{}, fmt.Errorf("approve adjustment: %w", err)
	}
	if !found {
		return AdjustmentResult{outcome: ApprovalAdjustmentNotFound}, nil
	}
	switch adjustment.Status {
	case "POSTED":
		return AdjustmentResult{outcome: ApprovalExisting, status: adjustment.Status}, nil
	case "REJECTED":
		return AdjustmentResult{outcome: ApprovalStatusMismatch, status: adjustment.Status}, nil
	}
	if duty.Distinct && adjustment.Operator.String() == command.Operator.String() {
		return AdjustmentResult{outcome: ApprovalDutyBlocked, status: adjustment.Status}, nil
	}
	screened := SubmitAdjustmentCommand{Tenant: command.Tenant, Owner: adjustment.Owner, Warehouse: adjustment.Warehouse, Lines: adjustment.Lines}
	_, issues, err := handler.screen(ctx, screened, true)
	if err != nil {
		return AdjustmentResult{}, fmt.Errorf("approve adjustment: %w", err)
	}
	if len(issues) > 0 {
		return AdjustmentResult{outcome: ApprovalNotValid, status: adjustment.Status, issues: issues}, nil
	}
	for index, line := range adjustment.Lines {
		if err := handler.postLine(ctx, command.Tenant, adjustment, index, line); err != nil {
			return AdjustmentResult{}, err
		}
	}
	if err := handler.adjustments.MarkPosted(ctx, command.Tenant, adjustment.ID, command.Operator, handler.clock.Now(), command.Note, duty.Revision); err != nil {
		return AdjustmentResult{}, fmt.Errorf("approve adjustment: %w", err)
	}
	return AdjustmentResult{outcome: ApprovalPosted, status: "POSTED"}, nil
}

func (handler *AdjustmentHandler) Reject(ctx context.Context, command RejectAdjustmentCommand) (AdjustmentResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return AdjustmentResult{}, fmt.Errorf("reject adjustment: %w", err)
	}
	if !configured {
		return AdjustmentResult{outcome: ApprovalNotConfigured}, nil
	}
	if command.Reason == "" {
		return AdjustmentResult{}, fmt.Errorf("reject adjustment: 缺原因")
	}
	adjustment, found, err := handler.adjustments.Load(ctx, command.Tenant, command.ID)
	if err != nil {
		return AdjustmentResult{}, fmt.Errorf("reject adjustment: %w", err)
	}
	if !found {
		return AdjustmentResult{outcome: ApprovalAdjustmentNotFound}, nil
	}
	switch adjustment.Status {
	case "REJECTED":
		return AdjustmentResult{outcome: ApprovalExisting, status: adjustment.Status}, nil
	case "POSTED":
		return AdjustmentResult{outcome: ApprovalStatusMismatch, status: adjustment.Status}, nil
	}
	if err := handler.adjustments.MarkRejected(ctx, command.Tenant, adjustment.ID, command.Operator, handler.clock.Now(), command.Reason); err != nil {
		return AdjustmentResult{}, fmt.Errorf("reject adjustment: %w", err)
	}
	return AdjustmentResult{outcome: ApprovalRejected, status: "REJECTED"}, nil
}

// ApproveAdjustmentCommand 批准一份库存调整。
type ApproveAdjustmentCommand struct {
	Tenant   domain.TenantID
	ID       domain.RequestKey
	Operator domain.Registrant
	Note     string
}

// RejectAdjustmentCommand 驳回一份库存调整。原因必带。
type RejectAdjustmentCommand struct {
	Tenant   domain.TenantID
	ID       domain.RequestKey
	Operator domain.Registrant
	Reason   string
}

func (handler *AdjustmentHandler) ready(ctx context.Context, tenant domain.TenantID, operation string) (bool, ports.ApprovalDuty, AdjustmentResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, tenant)
	if err != nil {
		return false, ports.ApprovalDuty{}, AdjustmentResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !configured {
		return false, ports.ApprovalDuty{}, AdjustmentResult{outcome: ApprovalNotConfigured}, nil
	}
	duty, found, err := handler.duties.LoadDuty(ctx, tenant)
	if err != nil {
		return false, ports.ApprovalDuty{}, AdjustmentResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return false, ports.ApprovalDuty{}, AdjustmentResult{outcome: ApprovalNotConfigured}, nil
	}
	return true, duty, AdjustmentResult{}, nil
}

func (handler *AdjustmentHandler) postLine(ctx context.Context, tenant domain.TenantID, adjustment ports.StockAdjustment, index int, line ports.AdjustmentLine) error {
	cell, err := domain.NewStockCell(domain.StockCellSpec{
		Location: line.Location, Goods: line.Goods, Unit: line.Unit, State: line.State, Alias: line.Alias,
		InboundBatch: line.Inbound, BatchNumber: line.Batch, Expiry: line.Expiry,
	})
	if err != nil {
		return fmt.Errorf("approve adjustment: %w", err)
	}
	direction := domain.StockIn
	reason := domain.StockReasonAdjustmentIncrease
	if line.Kind != ports.AdjustmentIncrease {
		direction = domain.StockOut
		reason = domain.StockReasonAdjustmentDecrease
		if line.Kind == ports.AdjustmentDestruction {
			reason = domain.StockReasonDestruction
		}
	}
	source, err := domain.NewSourceFactReference(adjustment.ID.String() + "#" + itoa(index))
	if err != nil {
		return fmt.Errorf("approve adjustment: %w", err)
	}
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{
		Tenant: tenant,
		Request: domain.NewStockMovementRequest(adjustment.Owner, adjustment.Warehouse, reason, source, handler.clock.Now(), []domain.StockLineRequest{
			domain.NewStockLineRequest(direction, line.Quantity, cell, time.Time{}),
		}),
	})
	if err != nil {
		return fmt.Errorf("approve adjustment: %w", err)
	}
	if posted.Outcome() != PostStockPosted && posted.Outcome() != PostStockAlreadyPosted {
		return fmt.Errorf("approve adjustment: 入账没有成立")
	}
	return nil
}

func (handler *AdjustmentHandler) screen(ctx context.Context, command SubmitAdjustmentCommand, approving bool) ([]ports.AdjustmentLine, []domain.FreezeIssue, error) {
	lines := append([]ports.AdjustmentLine(nil), command.Lines...)
	var issues []domain.FreezeIssue
	seen := map[string]bool{}
	for index := range lines {
		line := &lines[index]
		key := line.Location.String() + "\x1f" + line.Goods.String() + "\x1f" + line.Unit.String() + "\x1f" + line.State.String()
		if seen[key] {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound))
			continue
		}
		seen[key] = true
		if line.Kind == ports.AdjustmentDestruction && line.Basis == "" {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueGoodsNotRegistered))
			continue
		}
		location, found, err := handler.locations.LoadLatestLocation(ctx, command.Tenant, command.Warehouse, line.Location)
		if err != nil {
			return nil, nil, err
		}
		if !found {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound))
			continue
		}
		zone, found, err := handler.zones.LoadLatestZone(ctx, command.Tenant, command.Warehouse, location.Content().ZoneCode())
		if err != nil {
			return nil, nil, err
		}
		if !found || zone.Content().Usage() == domain.ZoneUsageOutboundStaging {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound))
			continue
		}
		if line.Kind == ports.AdjustmentIncrease && (location.Content().Deactivated() || !line.State.AllowedIn(zone.Content().Usage())) {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound))
			continue
		}
		revision, found, err := handler.goods.LoadGoodsAt(ctx, command.Tenant, command.Owner, line.Goods, handler.clock.Now())
		if err != nil {
			return nil, nil, err
		}
		if !found {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueGoodsNotRegistered))
			continue
		}
		if _, ok := unitFactor(revision, line.Unit); !ok {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueUnitNotDeclared))
			continue
		}
		if line.Inbound.String() == "" {
			resolved, ok, err := handler.latestBatch(ctx, command, *line)
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueInboundBatchNotOnCell))
				continue
			}
			line.Inbound = resolved
		}
		batch, found, err := handler.ledger.LoadInboundBatch(ctx, command.Tenant, line.Inbound)
		if err != nil {
			return nil, nil, err
		}
		if !found || batch.Goods() != line.Goods {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueInboundBatchNotOnCell))
			continue
		}
		if line.Kind == ports.AdjustmentIncrease {
			continue
		}
		cell, err := domain.NewStockCell(domain.StockCellSpec{
			Location: line.Location, Goods: line.Goods, Unit: line.Unit, State: line.State, Alias: line.Alias,
			InboundBatch: line.Inbound, BatchNumber: line.Batch, Expiry: line.Expiry,
		})
		if err != nil {
			return nil, nil, err
		}
		balance, err := handler.ledger.CellBalance(ctx, command.Tenant, command.Owner, command.Warehouse, cell)
		if err != nil {
			return nil, nil, err
		}
		available := balance
		if approving {
			held, err := handler.holds.CellHold(ctx, command.Tenant, command.Owner, command.Warehouse, cell)
			if err != nil {
				return nil, nil, err
			}
			available -= held
		}
		if available < line.Quantity {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueInboundBatchNotOnCell))
		}
	}
	return lines, issues, nil
}

func (handler *AdjustmentHandler) latestBatch(ctx context.Context, command SubmitAdjustmentCommand, line ports.AdjustmentLine) (domain.InboundBatchID, bool, error) {
	rows, err := handler.ledger.ListOnHandCells(ctx, command.Tenant, command.Owner, command.Warehouse)
	if err != nil {
		return domain.InboundBatchID{}, false, err
	}
	for _, row := range rows {
		if row.LocationCode == line.Location.String() && row.GoodsCode == line.Goods.String() && row.Unit == line.Unit.String() && row.State == line.State.String() {
			id, err := domain.NewInboundBatchID(row.InboundBatchID)
			return id, err == nil, err
		}
	}
	return domain.InboundBatchID{}, false, nil
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func sameAdjustment(existing ports.StockAdjustment, command SubmitAdjustmentCommand) bool {
	if existing.Owner != command.Owner || existing.Warehouse != command.Warehouse || existing.Reason != command.Reason || len(existing.Lines) != len(command.Lines) {
		return false
	}
	for index, line := range existing.Lines {
		other := command.Lines[index]
		if line.Kind != other.Kind || line.Location != other.Location || line.Goods != other.Goods || line.Unit != other.Unit || line.State != other.State || line.Quantity != other.Quantity || line.Inbound != other.Inbound || line.Basis != other.Basis {
			return false
		}
	}
	return true
}
