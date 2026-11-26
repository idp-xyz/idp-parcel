package application

import (
	"context"
	"fmt"
	"slices"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ApprovalOutcome 是批准或驳回一份盘点差额的结果。建账导入的批准不在这一支。
type ApprovalOutcome uint8

const (
	ApprovalOutcomeInvalid ApprovalOutcome = iota
	ApprovalPosted
	ApprovalExisting
	ApprovalNotConfigured
	ApprovalNotFound
	ApprovalStatusMismatch
	ApprovalNotValid
	ApprovalDutyBlocked
	ApprovalRejected
	ApprovalDutyRegistered
	ApprovalSubmitted
	ApprovalConflict
	ApprovalWarehouseNotFound
	ApprovalWarehouseNotAccepting
	ApprovalAdjustmentNotFound
)

func (outcome ApprovalOutcome) String() string {
	switch outcome {
	case ApprovalPosted:
		return "POSTED"
	case ApprovalExisting:
		return "EXISTING_RESULT"
	case ApprovalNotConfigured:
		return "NOT_CONFIGURED"
	case ApprovalNotFound:
		return "DIFFERENCE_NOT_FOUND"
	case ApprovalStatusMismatch:
		return "STATUS_MISMATCH"
	case ApprovalNotValid:
		return "NOT_VALID"
	case ApprovalDutyBlocked:
		return "DUTY_BLOCKED"
	case ApprovalRejected:
		return "REJECTED"
	case ApprovalDutyRegistered:
		return "REGISTERED"
	case ApprovalSubmitted:
		return "SUBMITTED"
	case ApprovalConflict:
		return "SOURCE_CONFLICT"
	case ApprovalWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case ApprovalWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	case ApprovalAdjustmentNotFound:
		return "ADJUSTMENT_NOT_FOUND"
	default:
		return ""
	}
}

// ApprovalDuty 是本租户现行的批准职责规则。没登记时批准不放行。
type ApprovalDuty struct {
	Revision int
	Distinct bool
	Operator domain.Registrant
}

// ApprovalResult 携带落点。
type ApprovalResult struct {
	outcome ApprovalOutcome
	status  string
	duty    ApprovalDuty
	hasDuty bool
}

func (result ApprovalResult) Outcome() ApprovalOutcome { return result.outcome }
func (result ApprovalResult) Status() string           { return result.status }
func (result ApprovalResult) Duty() (ApprovalDuty, bool) {
	return result.duty, result.hasDuty
}

// ApproveCountCommand 批准一份盘点差额。说明可空。货主与差额取自那一份。
type ApproveCountCommand struct {
	Tenant   domain.TenantID
	ID       domain.StockDiscrepancyID
	Operator domain.Registrant
	Note     string
}

// RejectCountCommand 驳回一份盘点差额。原因必带。
type RejectCountCommand struct {
	Tenant   domain.TenantID
	ID       domain.StockDiscrepancyID
	Operator domain.Registrant
	Reason   string
}

// RegisterApprovalDutyCommand 登记批准职责规则的一个修订。
type RegisterApprovalDutyCommand struct {
	Tenant   domain.TenantID
	Revision int
	Distinct bool
	Operator domain.Registrant
}

// CountApprovalHandler 批准或驳回盘点差额。没登记批准职责规则时批准答未配置。
type CountApprovalHandler struct {
	counts       ports.CountStore
	duties       ports.ApprovalDutyStore
	poster       *PostStockMovementHandler
	holds        ports.StockHolds
	clock        ports.Clock
	anyWarehouse ports.TenantWarehouseLookup
	// pickDifferences 缺席时差额照样批准或驳回，只是盘出差额的库位上少拣留下的拣货差异不随之了结。
	pickDifferences ports.PickDifferences
}

// WithPickDifferences 接上拣货差异：盘出差额的库位，等这份差额批准入账或驳回之后，那里的拣货差异才了结。
func (handler *CountApprovalHandler) WithPickDifferences(differences ports.PickDifferences) *CountApprovalHandler {
	handler.pickDifferences = differences
	return handler
}

// settlePickDifferences 在一份盘点差额有了结论之后，了结它那个库位上那位货主那种货的拣货差异。
func (handler *CountApprovalHandler) settlePickDifferences(ctx context.Context, difference ports.StoredCountDifference) error {
	if handler.pickDifferences == nil {
		return nil
	}
	count, found, err := handler.counts.Load(ctx, difference.Tenant, difference.CountID)
	if err != nil || !found {
		return err
	}
	return handler.pickDifferences.ClosePickDifferences(ctx, difference.Tenant, ports.PickDifferenceClosure{
		Warehouse: difference.Warehouse, Locations: []domain.LocationCode{difference.Location}, Owner: difference.Owner,
		Goods: []domain.GoodsCode{difference.Goods}, Before: count.StartedAt(), Count: difference.CountID.String(), At: handler.clock.Now(),
	})
}

func NewCountApprovalHandler(
	counts ports.CountStore,
	duties ports.ApprovalDutyStore,
	poster *PostStockMovementHandler,
	holds ports.StockHolds,
	clock ports.Clock,
	anyWarehouse ports.TenantWarehouseLookup,
) *CountApprovalHandler {
	return &CountApprovalHandler{counts: counts, duties: duties, poster: poster, holds: holds, clock: clock, anyWarehouse: anyWarehouse}
}

func (handler *CountApprovalHandler) RegisterDuty(ctx context.Context, command RegisterApprovalDutyCommand) (ApprovalResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("register approval duty: %w", err)
	}
	if !configured {
		return ApprovalResult{outcome: ApprovalNotConfigured}, nil
	}
	if command.Revision < 1 || command.Operator.String() == "" {
		return ApprovalResult{}, fmt.Errorf("register approval duty: 命令不完整")
	}
	current, found, err := handler.duties.LoadDuty(ctx, command.Tenant)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("register approval duty: %w", err)
	}
	if found && current.Revision == command.Revision && current.Distinct == command.Distinct {
		return ApprovalResult{outcome: ApprovalExisting, duty: ApprovalDuty{Revision: current.Revision, Distinct: current.Distinct, Operator: current.Operator}, hasDuty: true}, nil
	}
	if found && current.Revision >= command.Revision {
		return ApprovalResult{outcome: ApprovalStatusMismatch}, nil
	}
	if found && command.Revision != current.Revision+1 {
		return ApprovalResult{outcome: ApprovalStatusMismatch}, nil
	}
	duty := ports.ApprovalDuty{Revision: command.Revision, Distinct: command.Distinct, Operator: command.Operator, RegisteredAt: handler.clock.Now()}
	if err := handler.duties.SaveDuty(ctx, command.Tenant, duty); err != nil {
		return ApprovalResult{}, fmt.Errorf("register approval duty: %w", err)
	}
	return ApprovalResult{outcome: ApprovalDutyRegistered, duty: ApprovalDuty{Revision: duty.Revision, Distinct: duty.Distinct, Operator: duty.Operator}, hasDuty: true}, nil
}

func (handler *CountApprovalHandler) Approve(ctx context.Context, command ApproveCountCommand) (ApprovalResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve count difference: %w", err)
	}
	if !configured {
		return ApprovalResult{outcome: ApprovalNotConfigured}, nil
	}
	duty, found, err := handler.duties.LoadDuty(ctx, command.Tenant)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve count difference: %w", err)
	}
	if !found {
		return ApprovalResult{outcome: ApprovalNotConfigured}, nil
	}
	difference, found, err := handler.counts.LoadDifference(ctx, command.Tenant, command.ID)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve count difference: %w", err)
	}
	if !found {
		return ApprovalResult{outcome: ApprovalNotFound}, nil
	}
	switch difference.Status {
	case "APPROVED":
		return ApprovalResult{outcome: ApprovalExisting, status: difference.Status}, nil
	case "REJECTED":
		return ApprovalResult{outcome: ApprovalStatusMismatch, status: difference.Status}, nil
	}
	if duty.Distinct {
		submitter, err := handler.submitter(ctx, command.Tenant, difference)
		if err != nil {
			return ApprovalResult{}, fmt.Errorf("approve count difference: %w", err)
		}
		if submitter == command.Operator.String() {
			return ApprovalResult{outcome: ApprovalDutyBlocked, status: difference.Status}, nil
		}
	}
	lines, reason, err := handler.lines(ctx, difference)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve count difference: %w", err)
	}
	if reason != domain.StockReasonCountGain && reason != domain.StockReasonCountLoss {
		return ApprovalResult{outcome: ApprovalNotValid, status: difference.Status}, nil
	}
	source, err := domain.NewSourceFactReference(difference.ID.String())
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve count difference: %w", err)
	}
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{
		Tenant:  command.Tenant,
		Request: domain.NewStockMovementRequest(difference.Owner, difference.Warehouse, reason, source, handler.clock.Now(), lines),
	})
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("approve count difference: %w", err)
	}
	if posted.Outcome() != PostStockPosted && posted.Outcome() != PostStockAlreadyPosted {
		return ApprovalResult{outcome: ApprovalNotValid, status: difference.Status}, nil
	}
	if err := handler.counts.MarkApproved(ctx, command.Tenant, difference.ID, command.Operator, handler.clock.Now(), command.Note, duty.Revision); err != nil {
		return ApprovalResult{}, fmt.Errorf("approve count difference: %w", err)
	}
	if err := handler.settlePickDifferences(ctx, difference); err != nil {
		return ApprovalResult{}, fmt.Errorf("approve count difference: %w", err)
	}
	return ApprovalResult{outcome: ApprovalPosted, status: "APPROVED"}, nil
}

func (handler *CountApprovalHandler) Reject(ctx context.Context, command RejectCountCommand) (ApprovalResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("reject count difference: %w", err)
	}
	if !configured {
		return ApprovalResult{outcome: ApprovalNotConfigured}, nil
	}
	if command.Reason == "" {
		return ApprovalResult{}, fmt.Errorf("reject count difference: 缺原因")
	}
	difference, found, err := handler.counts.LoadDifference(ctx, command.Tenant, command.ID)
	if err != nil {
		return ApprovalResult{}, fmt.Errorf("reject count difference: %w", err)
	}
	if !found {
		return ApprovalResult{outcome: ApprovalNotFound}, nil
	}
	switch difference.Status {
	case "REJECTED":
		return ApprovalResult{outcome: ApprovalExisting, status: difference.Status}, nil
	case "APPROVED":
		return ApprovalResult{outcome: ApprovalStatusMismatch, status: difference.Status}, nil
	}
	if err := handler.counts.MarkRejected(ctx, command.Tenant, difference.ID, command.Operator, handler.clock.Now(), command.Reason); err != nil {
		return ApprovalResult{}, fmt.Errorf("reject count difference: %w", err)
	}
	if err := handler.settlePickDifferences(ctx, difference); err != nil {
		return ApprovalResult{}, fmt.Errorf("reject count difference: %w", err)
	}
	return ApprovalResult{outcome: ApprovalRejected, status: "REJECTED"}, nil
}

func (handler *CountApprovalHandler) submitter(ctx context.Context, tenant domain.TenantID, difference ports.StoredCountDifference) (string, error) {
	entries, err := handler.counts.LoadEntries(ctx, tenant, difference.CountID)
	if err != nil {
		return "", err
	}
	var current ports.CountEntry
	found := false
	for _, entry := range entries {
		if entry.Location != difference.Location {
			continue
		}
		if !found || entry.OccurredAt.After(current.OccurredAt) || (entry.OccurredAt.Equal(current.OccurredAt) && entry.Sequence > current.Sequence) {
			current = entry
			found = true
		}
	}
	if !found {
		return "", fmt.Errorf("count entry is missing")
	}
	return current.Operator.String(), nil
}

func (handler *CountApprovalHandler) lines(ctx context.Context, difference ports.StoredCountDifference) ([]domain.StockLineRequest, domain.StockMovementReason, error) {
	rows, err := handler.counts.LocationBalances(ctx, difference.Tenant, difference.Warehouse, []domain.LocationCode{difference.Location})
	if err != nil {
		return nil, 0, err
	}
	matched := make([]ports.StockCellBalance, 0)
	for _, row := range rows {
		if row.Owner == difference.Owner.String() && row.Goods == difference.Goods.String() && row.Unit == difference.Unit.String() && row.State == difference.State.String() && textOf(row.Alias) == difference.Alias.String() && textOf(row.BatchNumber) == difference.Batch.String() && textOf(row.Expiry) == difference.Expiry.String() {
			matched = append(matched, row)
		}
	}
	slices.SortFunc(matched, func(left, right ports.StockCellBalance) int {
		return left.InboundAt.Compare(right.InboundAt)
	})
	gap := difference.Counted - difference.Book
	if gap == 0 {
		return nil, 0, nil
	}
	if gap > 0 {
		if len(matched) == 0 {
			return nil, 0, nil
		}
		_, _, _, cell, err := countCell(matched[len(matched)-1])
		if err != nil {
			return nil, 0, err
		}
		return []domain.StockLineRequest{domain.NewStockLineRequest(domain.StockIn, gap, cell, time.Time{})}, domain.StockReasonCountGain, nil
	}
	need := -gap
	var lines []domain.StockLineRequest
	for _, row := range matched {
		if need == 0 {
			break
		}
		_, _, _, cell, err := countCell(row)
		if err != nil {
			return nil, 0, err
		}
		held, err := handler.holds.CellHold(ctx, difference.Tenant, difference.Owner, difference.Warehouse, cell)
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
		lines = append(lines, domain.NewStockLineRequest(domain.StockOut, take, cell, time.Time{}))
		need -= take
	}
	if need != 0 {
		return nil, 0, nil
	}
	return lines, domain.StockReasonCountLoss, nil
}
