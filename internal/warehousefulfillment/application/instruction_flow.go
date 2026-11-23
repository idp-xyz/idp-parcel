package application

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (handler *InstructionHandler) Accept(ctx context.Context, command AcceptInstructionCommand) (InstructionResult, error) {
	if command.Tenant.String() == "" || command.Owner.String() == "" || command.Warehouse.String() == "" || command.Kind.String() == "" || command.Quantity <= 0 {
		return InstructionResult{}, fmt.Errorf("accept instruction: 命令不完整")
	}
	switch command.Origin {
	case domain.InstructionOriginCustomer:
		if err := handler.instructions.HoldSubmission(ctx, command.Tenant, command.RequestKey); err != nil {
			return InstructionResult{}, fmt.Errorf("accept instruction: %w", err)
		}
		existing, found, err := handler.instructions.LoadBySubmission(ctx, command.Tenant, command.RequestKey)
		if err != nil {
			return InstructionResult{}, fmt.Errorf("accept instruction: %w", err)
		}
		if found {
			return replayInstruction(existing, command), nil
		}
	case domain.InstructionOriginOperator:
		if handler.owners == nil {
			return InstructionResult{}, fmt.Errorf("accept instruction: %w", errOwnerLookupMissing)
		}
		registered, err := handler.owners.OwnerAccountRegistered(ctx, command.Tenant, command.Owner)
		if err != nil {
			return InstructionResult{}, fmt.Errorf("accept instruction: %w", err)
		}
		if !registered {
			return InstructionResult{}, ErrOwnerAccountNotInTenant
		}
		if err := handler.instructions.HoldDecision(ctx, command.Tenant, command.DecisionID); err != nil {
			return InstructionResult{}, fmt.Errorf("accept instruction: %w", err)
		}
		existing, found, err := handler.instructions.LoadByDecision(ctx, command.Tenant, command.DecisionID)
		if err != nil {
			return InstructionResult{}, fmt.Errorf("accept instruction: %w", err)
		}
		if found {
			return replayInstruction(existing, command), nil
		}
	default:
		return InstructionResult{}, fmt.Errorf("accept instruction: 缺发起途径")
	}
	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("accept instruction: %w", err)
	}
	if !found {
		return InstructionResult{outcome: InstructionWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return InstructionResult{outcome: InstructionWarehouseNotAccepting}, nil
	}
	revision, found, err := handler.instructionGoods(ctx, command)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("accept instruction: %w", err)
	}
	if !found {
		return InstructionResult{outcome: InstructionContentInvalid, issues: []domain.InternalJobIssue{domain.NewInternalJobIssue(0, domain.InternalJobIssueGoodsNotRegistered)}}, nil
	}
	components, issues := instructionContent(command, revision, handler.clock.Now())
	if len(issues) > 0 {
		return InstructionResult{outcome: InstructionContentInvalid, issues: issues}, nil
	}
	id, err := handler.identities.NextInstructionID()
	if err != nil {
		return InstructionResult{}, fmt.Errorf("accept instruction: %w", err)
	}
	instruction, err := domain.NewPendingInstruction(command.spec(id, revision.Revision(), components, handler.clock.Now()))
	if err != nil {
		return InstructionResult{}, fmt.Errorf("accept instruction: %w", err)
	}
	if err := handler.instructions.Save(ctx, instruction); err != nil {
		return InstructionResult{}, fmt.Errorf("accept instruction: %w", err)
	}
	return InstructionResult{outcome: InstructionAccepted, instruction: instruction, has: true}, nil
}

func replayInstruction(existing domain.Instruction, command AcceptInstructionCommand) InstructionResult {
	if existing.SameSubmission(command.spec(existing.ID(), existing.FixedRevision(), nil, existing.ReceivedAt())) {
		return InstructionResult{outcome: InstructionExisting, instruction: existing, has: true}
	}
	return InstructionResult{outcome: InstructionConflict}
}

func (handler *InstructionHandler) instructionGoods(ctx context.Context, command AcceptInstructionCommand) (domain.GoodsRevision, bool, error) {
	if command.NamedRevision > 0 {
		return handler.revisions.LoadGoodsRevision(ctx, command.Tenant, command.Owner, command.Goods, command.NamedRevision)
	}
	return handler.goods.LoadLatestGoods(ctx, command.Tenant, command.Owner, command.Goods)
}

func instructionContent(command AcceptInstructionCommand, revision domain.GoodsRevision, at time.Time) ([]domain.InstructionComponent, []domain.InternalJobIssue) {
	var issues []domain.InternalJobIssue
	add := func(kind domain.InternalJobIssueKind) {
		issues = append(issues, domain.NewInternalJobIssue(0, kind))
	}
	content := revision.Content()
	switch command.Kind {
	case domain.InstructionProcess, domain.InstructionDisassemble:
		if !content.Composite() || len(content.Components()) == 0 {
			add(domain.InternalJobIssueLineShape)
			return nil, issues
		}
		components := make([]domain.InstructionComponent, 0, len(content.Components()))
		for _, component := range content.Components() {
			built, err := domain.NewInstructionComponent(component.Code(), component.Quantity())
			if err != nil {
				add(domain.InternalJobIssueLineShape)
				return nil, issues
			}
			components = append(components, built)
		}
		return components, issues
	case domain.InstructionRelabel:
		if _, ok := unitFactor(revision, command.Unit); !ok {
			add(domain.InternalJobIssueUnitNotDeclared)
		}
		if command.FromAlias == command.ToAlias {
			add(domain.InternalJobIssueLineShape)
		}
		if command.FromAlias.String() == "" && command.ToAlias.String() == "" {
			add(domain.InternalJobIssueLineShape)
		}
		if command.ToAlias.String() != "" && !labelValid(revision, command.ToAlias, at) {
			add(domain.InternalJobIssueLabelAliasNotPlatform)
		}
		if command.FromAlias.String() != "" && !platformAlias(content, command.FromAlias) {
			add(domain.InternalJobIssueLabelAliasNotPlatform)
		}
		if content.TrackBatch() && command.Batch.Given() {
			// 带了的条件必须是跟踪的；不跟踪却带了，下面两格会指出。
		}
		if content.TrackBatch() && command.Batch.Given() == false && command.Expiry.Given() {
			add(domain.InternalJobIssueBatchRequired)
		}
		if !content.TrackBatch() && command.Batch.Given() {
			add(domain.InternalJobIssueBatchNotTracked)
		}
		if content.TrackExpiry() && !command.Expiry.Given() && command.Batch.Given() {
			add(domain.InternalJobIssueExpiryRequired)
		}
		if !content.TrackExpiry() && command.Expiry.Given() {
			add(domain.InternalJobIssueExpiryNotTracked)
		}
		return nil, issues
	default:
		add(domain.InternalJobIssueLineShape)
		return nil, issues
	}
}

func (handler *InstructionHandler) Withdraw(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, id domain.InstructionID, key domain.RequestKey, allowed []domain.InstructionKind) (InstructionResult, error) {
	if key.String() == "" {
		return InstructionResult{}, fmt.Errorf("withdraw instruction: 缺来源请求键")
	}
	if err := handler.instructions.HoldSubmission(ctx, tenant, key); err != nil {
		return InstructionResult{}, fmt.Errorf("withdraw instruction: %w", err)
	}
	previous, found, err := handler.instructions.LoadByWithdrawal(ctx, tenant, key)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("withdraw instruction: %w", err)
	}
	if found && previous.ID() != id {
		return InstructionResult{outcome: InstructionConflict}, nil
	}
	return handler.endInstruction(ctx, tenant, id, domain.InstructionOriginCustomer, "", owner, domain.Registrant{}, key, allowed)
}

func (handler *InstructionHandler) Void(ctx context.Context, tenant domain.TenantID, id domain.InstructionID, operator domain.Registrant, reason string, allowed []domain.InstructionKind) (InstructionResult, error) {
	return handler.endInstruction(ctx, tenant, id, domain.InstructionOriginOperator, reason, domain.OwnerAccountID{}, operator, domain.RequestKey{}, allowed)
}

func kindAllowed(kind domain.InstructionKind, allowed []domain.InstructionKind) bool {
	if len(allowed) == 0 {
		return true
	}
	return slices.Contains(allowed, kind)
}

func (handler *InstructionHandler) endInstruction(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.InstructionID,
	origin domain.InstructionOrigin,
	reason string,
	owner domain.OwnerAccountID,
	operator domain.Registrant,
	withdrawal domain.RequestKey,
	allowed []domain.InstructionKind,
) (InstructionResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, tenant)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("end instruction: %w", err)
	}
	if !configured {
		return InstructionResult{outcome: InstructionNotConfigured}, nil
	}
	if err := handler.instructions.HoldInstruction(ctx, tenant, id); err != nil {
		return InstructionResult{}, fmt.Errorf("end instruction: %w", err)
	}
	instruction, found, err := handler.instructions.Load(ctx, tenant, id)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("end instruction: %w", err)
	}
	if !found || (origin == domain.InstructionOriginCustomer && instruction.Owner() != owner) || !kindAllowed(instruction.Kind(), allowed) {
		return InstructionResult{outcome: InstructionNotFound}, nil
	}
	if instruction.Status() == domain.InstructionVoid {
		return InstructionResult{outcome: InstructionExisting, instruction: instruction, has: true}, nil
	}
	if origin == domain.InstructionOriginCustomer && instruction.Origin() != domain.InstructionOriginCustomer {
		return InstructionResult{outcome: InstructionWithdrawNotAllowed, instruction: instruction, has: true}, nil
	}
	if instruction.Status() != domain.InstructionPending {
		return InstructionResult{outcome: InstructionStatusMismatch, instruction: instruction, has: true}, nil
	}
	voided, err := instruction.WithVoid(origin, reason, handler.clock.Now(), operator, withdrawal)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("end instruction: %w", err)
	}
	if err := handler.instructions.MarkVoid(ctx, voided); err != nil {
		return InstructionResult{}, fmt.Errorf("end instruction: %w", err)
	}
	return InstructionResult{outcome: InstructionVoided, instruction: voided, has: true}, nil
}

func (handler *InstructionHandler) Execute(ctx context.Context, command ExecuteInstructionCommand) (InstructionResult, error) {
	if command.Tenant.String() == "" || command.FactID.String() == "" || command.InstructionID.String() == "" || command.DoneQuantity <= 0 || len(command.Lines) == 0 {
		return InstructionResult{}, fmt.Errorf("execute instruction: 命令不完整")
	}
	if err := handler.instructions.HoldExecution(ctx, command.Tenant, command.FactID); err != nil {
		return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
	}
	existing, found, err := handler.instructions.LoadExecution(ctx, command.Tenant, command.FactID)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
	}
	if found {
		if sameExecution(existing, command) {
			instruction, ok, err := handler.instructions.Load(ctx, command.Tenant, existing.InstructionID)
			if err != nil {
				return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
			}
			if !ok {
				return InstructionResult{}, fmt.Errorf("execute instruction: 执行在册，指令却不在")
			}
			return InstructionResult{outcome: InstructionExisting, instruction: instruction, has: true}, nil
		}
		return InstructionResult{outcome: InstructionConflict}, nil
	}
	if err := handler.instructions.HoldInstruction(ctx, command.Tenant, command.InstructionID); err != nil {
		return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
	}
	instruction, found, err := handler.instructions.Load(ctx, command.Tenant, command.InstructionID)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
	}
	if !found {
		return InstructionResult{outcome: InstructionNotFound}, nil
	}
	if instruction.Warehouse() != command.Warehouse {
		return InstructionResult{outcome: InstructionContentInvalid, issues: []domain.InternalJobIssue{domain.NewInternalJobIssue(0, domain.InternalJobIssueLineShape)}}, nil
	}
	if instruction.Status() != domain.InstructionPending {
		return InstructionResult{outcome: InstructionStatusMismatch, instruction: instruction, has: true}, nil
	}
	if command.DoneQuantity > instruction.Quantity() {
		return InstructionResult{outcome: InstructionContentInvalid, issues: []domain.InternalJobIssue{domain.NewInternalJobIssue(0, domain.InternalJobIssueQuantityNotPositive)}}, nil
	}
	issues, err := handler.screenExecution(ctx, command, instruction)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
	}
	if len(issues) > 0 {
		return InstructionResult{outcome: InstructionContentInvalid, issues: issues}, nil
	}
	requests, unavailable, err := handler.allocateExecution(ctx, command, instruction)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
	}
	if unavailable {
		return InstructionResult{outcome: InstructionUnavailable, instruction: instruction, has: true}, nil
	}
	reason := domain.StockReasonProcessing
	switch instruction.Kind() {
	case domain.InstructionDisassemble:
		reason = domain.StockReasonDisassembly
	case domain.InstructionRelabel:
		reason = domain.StockReasonRelabel
	}
	source, err := domain.NewSourceFactReference(command.FactID.String())
	if err != nil {
		return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
	}
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{
		Tenant:  command.Tenant,
		Request: domain.NewStockMovementRequest(instruction.Owner(), instruction.Warehouse(), reason, source, command.OccurredAt, requests),
	})
	if err != nil {
		return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
	}
	if posted.Outcome() == PostStockRefused {
		return InstructionResult{outcome: InstructionUnavailable, instruction: instruction, has: true}, nil
	}
	if posted.Outcome() != PostStockPosted {
		return InstructionResult{}, fmt.Errorf("execute instruction: 流水没有入账：%s", posted.Outcome())
	}
	movement, ok := posted.Movement()
	if !ok {
		return InstructionResult{}, fmt.Errorf("execute instruction: 流水没有交回")
	}
	done, err := instruction.WithDone(command.DoneQuantity, command.FactID)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
	}
	execution := ports.InstructionExecution{
		Tenant: command.Tenant, FactID: command.FactID, InstructionID: instruction.ID(), Device: command.Device,
		Operator: command.Operator, OccurredAt: command.OccurredAt, DeviceSequence: command.DeviceSequence,
		ReceivedAt: handler.clock.Now(), DoneQuantity: command.DoneQuantity, MovementID: movement.ID(),
	}
	for _, line := range command.Lines {
		execution.Lines = append(execution.Lines, ports.InstructionExecutionLine{
			Direction: line.Direction, Location: line.Location, Goods: line.Goods, Unit: line.Unit, State: line.State,
			Alias: line.Alias, Batch: line.Batch, Expiry: line.Expiry, NamedBatch: line.NamedBatch, Quantity: line.Quantity,
		})
	}
	if err := handler.instructions.SaveExecution(ctx, execution); err != nil {
		return InstructionResult{}, fmt.Errorf("execute instruction: %w", err)
	}
	if err := handler.formProcessingCharge(ctx, command, instruction); err != nil {
		return InstructionResult{}, err
	}
	return InstructionResult{outcome: InstructionPosted, instruction: done, has: true}, nil
}

func sameExecution(existing ports.InstructionExecution, command ExecuteInstructionCommand) bool {
	if existing.InstructionID != command.InstructionID || existing.Device != command.Device || existing.Operator != command.Operator ||
		existing.DeviceSequence != command.DeviceSequence || existing.DoneQuantity != command.DoneQuantity || len(existing.Lines) != len(command.Lines) {
		return false
	}
	if !existing.OccurredAt.Equal(command.OccurredAt.UTC().Truncate(time.Microsecond)) && !existing.OccurredAt.Equal(command.OccurredAt) {
		return false
	}
	for index := range existing.Lines {
		line := existing.Lines[index]
		other := command.Lines[index]
		if line.Direction != other.Direction || line.Location != other.Location || line.Goods != other.Goods || line.Unit != other.Unit ||
			line.State != other.State || line.Alias != other.Alias || line.Batch != other.Batch || line.Expiry != other.Expiry ||
			line.NamedBatch != other.NamedBatch || line.Quantity != other.Quantity {
			return false
		}
	}
	return true
}

func (handler *InstructionHandler) screenExecution(ctx context.Context, command ExecuteInstructionCommand, instruction domain.Instruction) ([]domain.InternalJobIssue, error) {
	var issues []domain.InternalJobIssue
	places := map[string]placeInfo{}
	for index, line := range command.Lines {
		if line.Quantity <= 0 {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueQuantityNotPositive))
			continue
		}
		usage, found, err := handler.executionPlace(ctx, command.Tenant, command.Warehouse, line.Location, places)
		if err != nil {
			return nil, err
		}
		if !found {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueLocationNotFound))
			continue
		}
		if usage == domain.ZoneUsageOutboundStaging {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueOutboundStaging))
		}
		if line.Direction == domain.StockIn && usage == domain.ZoneUsageDefective {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueZoneIncompatible))
		}
		if line.State != domain.StockStateGood {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueConditionNotAllowed))
		}
	}
	if !executionBalances(command, instruction) {
		issues = append(issues, domain.NewInternalJobIssue(0, domain.InternalJobIssueFactorsUnequal))
	}
	return issues, nil
}

func executionBalances(command ExecuteInstructionCommand, instruction domain.Instruction) bool {
	switch instruction.Kind() {
	case domain.InstructionRelabel:
		var total int64
		for _, line := range command.Lines {
			if line.Direction != domain.StockOut || line.Goods != instruction.Goods() || line.Unit != instruction.Unit() || line.Alias != instruction.FromAlias() {
				return false
			}
			if instruction.Batch().Given() && line.Batch != instruction.Batch() {
				return false
			}
			if instruction.Expiry().Given() && line.Expiry != instruction.Expiry() {
				return false
			}
			total += line.Quantity
		}
		return total == command.DoneQuantity
	case domain.InstructionProcess, domain.InstructionDisassemble:
		inputs := map[string]int64{}
		var outputs int64
		for _, line := range command.Lines {
			if line.Direction == domain.StockOut {
				inputs[line.Goods.String()] += line.Quantity
				continue
			}
			if line.Goods != instruction.Goods() {
				return false
			}
			outputs += line.Quantity
		}
		if outputs != command.DoneQuantity {
			return false
		}
		if len(inputs) != len(instruction.Components()) {
			return false
		}
		for _, component := range instruction.Components() {
			if inputs[component.Goods().String()] != component.Quantity()*command.DoneQuantity {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (handler *InstructionHandler) allocateExecution(ctx context.Context, command ExecuteInstructionCommand, instruction domain.Instruction) ([]domain.StockLineRequest, bool, error) {
	rows, err := handler.onHand.ListOnHandCells(ctx, command.Tenant, instruction.Owner(), instruction.Warehouse())
	if err != nil {
		return nil, false, err
	}
	type cell struct {
		row  ports.StockBalanceRow
		cell domain.StockCell
		at   time.Time
		hold int64
	}
	var book []cell
	for _, row := range rows {
		parsed, err := cellFromBalance(row)
		if err != nil {
			return nil, false, err
		}
		batch, found, err := handler.ledger.LoadInboundBatch(ctx, command.Tenant, parsed.InboundBatch())
		if err != nil {
			return nil, false, err
		}
		if !found {
			return nil, false, fmt.Errorf("在库的格没有入库批次 %s", parsed.InboundBatch())
		}
		hold, err := handler.holds.CellHold(ctx, command.Tenant, instruction.Owner(), instruction.Warehouse(), parsed)
		if err != nil {
			return nil, false, err
		}
		book = append(book, cell{row: row, cell: parsed, at: batch.InboundAt(), hold: hold})
	}
	slices.SortFunc(book, func(left, right cell) int {
		if left.at.Before(right.at) {
			return -1
		}
		if right.at.Before(left.at) {
			return 1
		}
		return 0
	})
	var requests []domain.StockLineRequest
	for _, line := range command.Lines {
		if line.Direction != domain.StockOut {
			parsed, err := domain.NewStockCell(domain.StockCellSpec{
				Location: line.Location, Goods: line.Goods, Unit: line.Unit, State: line.State, Alias: line.Alias,
				BatchNumber: line.Batch, Expiry: line.Expiry,
			})
			if err != nil {
				return nil, false, err
			}
			requests = append(requests, domain.NewStockLineRequest(domain.StockIn, line.Quantity, parsed, time.Time{}))
			continue
		}
		remaining := line.Quantity
		for index := range book {
			item := &book[index]
			if !executionCellMatches(item.cell, line) {
				continue
			}
			free := item.row.Quantity - item.hold
			if free <= 0 {
				continue
			}
			take := free
			if take > remaining {
				take = remaining
			}
			requests = append(requests, domain.NewStockLineRequest(domain.StockOut, take, item.cell, time.Time{}))
			if instruction.Kind() == domain.InstructionRelabel {
				dest, err := retarget(item.cell, item.cell.Location(), item.cell.Unit(), item.cell.State())
				if err != nil {
					return nil, false, err
				}
				dest, err = domain.NewStockCell(domain.StockCellSpec{
					Location: dest.Location(), Goods: dest.Goods(), Unit: dest.Unit(), State: dest.State(),
					Alias: instruction.ToAlias(), InboundBatch: dest.InboundBatch(), BatchNumber: dest.BatchNumber(), Expiry: dest.Expiry(),
				})
				if err != nil {
					return nil, false, err
				}
				requests = append(requests, domain.NewStockLineRequest(domain.StockIn, take, dest, time.Time{}))
			}
			item.row.Quantity -= take
			remaining -= take
			if remaining == 0 {
				break
			}
		}
		if remaining != 0 {
			return nil, true, nil
		}
	}
	// 取走之后，可分配库存仍不得少于没点名的预占。点名批号或效期的预占不在这一笔空条件里。
	left := map[string]int64{}
	places := map[string]placeInfo{}
	for _, item := range book {
		usage, found, err := handler.executionPlace(ctx, command.Tenant, instruction.Warehouse(), item.cell.Location(), places)
		if err != nil {
			return nil, false, err
		}
		if !found || !domain.AllocatableGoods(item.cell.State(), usage) {
			continue
		}
		left[item.cell.Goods().String()+"\x1f"+item.cell.State().String()+"\x1f"+item.cell.Alias().String()] += item.row.Quantity
	}
	for id, quantity := range left {
		goods, state, alias, err := splitHoldKey(id)
		if err != nil {
			return nil, false, err
		}
		reserved, err := handler.holds.UnassignedReservation(ctx, command.Tenant, instruction.Owner(), instruction.Warehouse(), goods, state, alias, domain.BatchNumber{}, domain.ExpiryDate{})
		if err != nil {
			return nil, false, err
		}
		if quantity < reserved {
			return nil, true, nil
		}
	}
	return requests, false, nil
}

func splitHoldKey(id string) (domain.GoodsCode, domain.StockState, domain.GoodsAliasCode, error) {
	goodsText, rest, _ := strings.Cut(id, "\x1f")
	stateText, aliasText, _ := strings.Cut(rest, "\x1f")
	goods, err := domain.NewGoodsCode(goodsText)
	if err != nil {
		return domain.GoodsCode{}, 0, domain.GoodsAliasCode{}, err
	}
	state, err := domain.NewStockState(stateText)
	if err != nil {
		return domain.GoodsCode{}, 0, domain.GoodsAliasCode{}, err
	}
	var alias domain.GoodsAliasCode
	if aliasText != "" {
		alias, err = domain.NewGoodsAliasCode(aliasText)
		if err != nil {
			return domain.GoodsCode{}, 0, domain.GoodsAliasCode{}, err
		}
	}
	return goods, state, alias, nil
}

func executionCellMatches(cell domain.StockCell, line InstructionExecutionLine) bool {
	if cell.Location() != line.Location || cell.Goods() != line.Goods || cell.Unit() != line.Unit || cell.State() != line.State || cell.Alias() != line.Alias {
		return false
	}
	if line.Batch.Given() && cell.BatchNumber() != line.Batch {
		return false
	}
	if line.Expiry.Given() && cell.Expiry() != line.Expiry {
		return false
	}
	if line.NamedBatch.String() != "" && cell.InboundBatch() != line.NamedBatch {
		return false
	}
	return true
}

func (handler *InstructionHandler) executionPlace(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.LocationCode, places map[string]placeInfo) (domain.ZoneUsage, bool, error) {
	if info, ok := places[code.String()]; ok {
		return info.usage, true, nil
	}
	location, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, code)
	if err != nil || !found {
		return 0, found, err
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, location.Content().ZoneCode())
	if err != nil || !found {
		if err == nil && !found {
			return 0, false, fmt.Errorf("库位 %s 已登记，它的库区读不到", code)
		}
		return 0, false, err
	}
	usage := zone.Content().Usage()
	places[code.String()] = placeInfo{usage: usage}
	return usage, true, nil
}
