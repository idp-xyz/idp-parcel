package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// RestoreConditionOutcome 是次品转良品的结果。整份成立或整份不成立，不转够的那一部分。
type RestoreConditionOutcome uint8

const (
	RestoreConditionOutcomeInvalid RestoreConditionOutcome = iota
	RestoreConditionRestored
	RestoreConditionExisting
	RestoreConditionConflict
	RestoreConditionNotValid
	RestoreConditionWarehouseNotFound
	RestoreConditionWarehouseNotAccepting
)

func (outcome RestoreConditionOutcome) String() string {
	switch outcome {
	case RestoreConditionRestored:
		return "RESTORED"
	case RestoreConditionExisting:
		return "EXISTING_RESULT"
	case RestoreConditionConflict:
		return "SOURCE_CONFLICT"
	case RestoreConditionNotValid:
		return "NOT_VALID"
	case RestoreConditionWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case RestoreConditionWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	default:
		return ""
	}
}

// RestoreConditionResult 携带落点。决定只在已转良品与已有结果上有。
type RestoreConditionResult struct {
	outcome RestoreConditionOutcome
	restore domain.ConditionRestore
	has     bool
	issues  []domain.InternalJobIssue
}

func (result RestoreConditionResult) Outcome() RestoreConditionOutcome { return result.outcome }
func (result RestoreConditionResult) Restore() (domain.ConditionRestore, bool) {
	return result.restore, result.has
}
func (result RestoreConditionResult) Issues() []domain.InternalJobIssue {
	return append([]domain.InternalJobIssue(nil), result.issues...)
}

// RestoreConditionCommand 是一次次品转良品。租户与操作者取自信封。时刻由本上下文在收下时盖。
type RestoreConditionCommand struct {
	Tenant     domain.TenantID
	ID         domain.ConditionRestoreID
	Operator   domain.Registrant
	Warehouse  domain.WarehouseCode
	Owner      domain.OwnerAccountID
	Location   domain.LocationCode
	Goods      domain.GoodsCode
	Unit       domain.GoodsUnitName
	State      domain.StockState
	Alias      domain.GoodsAliasCode
	Batch      domain.BatchNumber
	Expiry     domain.ExpiryDate
	NamedBatch domain.InboundBatchID
	Quantity   int64
	Reason     string
}

func (command RestoreConditionCommand) submission() domain.ConditionRestoreSubmission {
	return domain.ConditionRestoreSubmission{
		Operator: command.Operator, Warehouse: command.Warehouse, Owner: command.Owner, Location: command.Location,
		Goods: command.Goods, Unit: command.Unit, Alias: command.Alias, Batch: command.Batch, Expiry: command.Expiry,
		NamedBatch: command.NamedBatch, Quantity: command.Quantity, Reason: command.Reason,
	}
}

func (command RestoreConditionCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.ID.String() == "":
		return errors.New("缺决定标识")
	case command.Operator.String() == "":
		return errors.New("缺操作者")
	case command.Warehouse.String() == "":
		return errors.New("缺仓")
	case command.Owner.String() == "":
		return errors.New("缺货主")
	case command.Reason == "":
		return errors.New("缺原因")
	default:
		return nil
	}
}

func (handler *RegisterInternalJobHandler) Restore(ctx context.Context, command RestoreConditionCommand) (RestoreConditionResult, error) {
	if err := command.assemblyError(); err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	if err := handler.restores.HoldDecision(ctx, command.Tenant, command.ID); err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	existing, found, err := handler.restores.LoadDecision(ctx, command.Tenant, command.ID)
	if err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	if found {
		if existing.SameContent(command.submission()) {
			return RestoreConditionResult{outcome: RestoreConditionExisting, restore: existing, has: true}, nil
		}
		return RestoreConditionResult{outcome: RestoreConditionConflict}, nil
	}
	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	if !found {
		return RestoreConditionResult{outcome: RestoreConditionWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return RestoreConditionResult{outcome: RestoreConditionWarehouseNotAccepting}, nil
	}
	issues, err := handler.screenRestore(ctx, command)
	if err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	if len(issues) > 0 {
		return RestoreConditionResult{outcome: RestoreConditionNotValid, issues: issues}, nil
	}
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, command.Owner, command.Warehouse); err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	book, err := handler.loadBook(ctx, command.Tenant, command.Owner, command.Warehouse)
	if err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	line, err := domain.NewInternalJobLine(domain.InternalJobLineSpec{
		Owner: command.Owner, Goods: command.Goods, Unit: command.Unit, State: domain.StockStateDefective,
		Alias: command.Alias, Batch: command.Batch, Expiry: command.Expiry, NamedBatch: command.NamedBatch,
		Source: command.Location, Quantity: command.Quantity,
	})
	if err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	var onBook, free int64
	for _, cell := range book.cells {
		if !matchLine(line)(cell.cell) {
			continue
		}
		onBook += cell.qty
		free += cell.free()
	}
	if onBook < command.Quantity {
		return RestoreConditionResult{outcome: RestoreConditionNotValid, issues: []domain.InternalJobIssue{
			domain.NewInternalJobIssue(0, domain.InternalJobIssueNotEnough),
		}}, nil
	}
	if free < command.Quantity {
		return RestoreConditionResult{outcome: RestoreConditionNotValid, issues: []domain.InternalJobIssue{
			domain.NewInternalJobIssue(0, domain.InternalJobIssueHeld),
		}}, nil
	}
	taken := book.plan(matchLine(line), 1, 1, command.Quantity, command.Quantity, free, true, func(domain.StockCell) bool { return false }, command.Location, command.Unit, domain.StockStateGood)
	if taken.from != command.Quantity {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: 账上够却没有取全")
	}
	var requests []domain.StockLineRequest
	for _, chunk := range taken.sources {
		out, err := stockLine(domain.StockOut, chunk.qty, chunk.cell)
		if err != nil {
			return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
		}
		inCell, err := retarget(chunk.cell, command.Location, command.Unit, domain.StockStateGood)
		if err != nil {
			return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
		}
		in, err := stockLine(domain.StockIn, chunk.qty, inCell)
		if err != nil {
			return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
		}
		requests = append(requests, out, in)
	}
	source, err := domain.NewSourceFactReference(command.ID.String())
	if err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	now := handler.clock.Now()
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{
		Tenant:  command.Tenant,
		Request: domain.NewStockMovementRequest(command.Owner, command.Warehouse, domain.StockReasonStateChange, source, now, requests),
	})
	if err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	if posted.Outcome() != PostStockPosted {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: 流水没有入账：%s", posted.Outcome())
	}
	movement, ok := posted.Movement()
	if !ok {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: 流水没有交回")
	}
	restore, err := domain.NewConditionRestore(domain.ConditionRestoreSpec{
		Tenant: command.Tenant, ID: command.ID, Operator: command.Operator, OccurredAt: now, ReceivedAt: now,
		Warehouse: command.Warehouse, Owner: command.Owner, Location: command.Location, Goods: command.Goods, Unit: command.Unit,
		Alias: command.Alias, Batch: command.Batch, Expiry: command.Expiry, NamedBatch: command.NamedBatch,
		Quantity: command.Quantity, Reason: command.Reason, Movement: movement.ID(),
	})
	if err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	if err := handler.restores.SaveDecision(ctx, restore); err != nil {
		return RestoreConditionResult{}, fmt.Errorf("restore condition: %w", err)
	}
	if err := handler.noteRestoreCharge(ctx, command); err != nil {
		return RestoreConditionResult{}, err
	}
	return RestoreConditionResult{outcome: RestoreConditionRestored, restore: restore, has: true}, nil
}

func (handler *RegisterInternalJobHandler) screenRestore(ctx context.Context, command RestoreConditionCommand) ([]domain.InternalJobIssue, error) {
	var issues []domain.InternalJobIssue
	add := func(kind domain.InternalJobIssueKind) {
		issues = append(issues, domain.NewInternalJobIssue(0, kind))
	}
	if command.Quantity <= 0 {
		add(domain.InternalJobIssueQuantityNotPositive)
	}
	if command.stockState() != domain.StockStateDefective {
		add(domain.InternalJobIssueConditionNotAllowed)
	}
	places := map[string]placeInfo{}
	usage, found, err := handler.place(ctx, command.Tenant, command.Warehouse, command.Location, places)
	if err != nil {
		return nil, err
	}
	if !found {
		add(domain.InternalJobIssueLocationNotFound)
	} else if usage != domain.ZoneUsageReceivingStaging && usage != domain.ZoneUsageReturnStaging {
		add(domain.InternalJobIssueConditionNotAllowed)
	}
	line, err := domain.NewInternalJobLine(domain.InternalJobLineSpec{
		Owner: command.Owner, Goods: command.Goods, Unit: command.Unit, State: command.stockState(),
		Alias: command.Alias, Batch: command.Batch, Expiry: command.Expiry, Source: command.Location, Quantity: command.Quantity,
	})
	if err != nil {
		return nil, err
	}
	revisions := map[string]domain.GoodsRevision{}
	revision, foundGoods, err := handler.goodsRevision(ctx, command.Tenant, line, revisions)
	if err != nil {
		return nil, err
	}
	if !foundGoods {
		add(domain.InternalJobIssueGoodsNotRegistered)
	} else {
		issues = append(issues, goodsIssues(0, revision, line)...)
	}
	return issues, nil
}

// stockState 把没给库存状态的请求看成次品：这一口只转次品，载荷可以不带状态。
func (command RestoreConditionCommand) stockState() domain.StockState {
	if command.State.String() == "" {
		return domain.StockStateDefective
	}
	return command.State
}
