package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// UnboxCommand 是一次开箱。实点可以一行都没有：空箱也出箱，箱单全数记少收。
type UnboxCommand struct {
	Tenant         domain.TenantID
	FactID         domain.InternalJobFactID
	Device         domain.ReceiptDeviceID
	Operator       domain.ReceiptOperatorID
	OccurredAt     time.Time
	DeviceSequence int64
	Warehouse      domain.WarehouseCode
	Owner          domain.OwnerAccountID
	Carton         domain.CartonNumber
	Lines          []ports.UnboxLine
}

func (handler *InstructionHandler) Unbox(ctx context.Context, command UnboxCommand) (InstructionResult, error) {
	if command.Tenant.String() == "" || command.FactID.String() == "" || command.Carton.String() == "" || command.Owner.String() == "" {
		return InstructionResult{}, fmt.Errorf("unbox: 命令不完整")
	}
	if err := handler.unboxes.HoldUnbox(ctx, command.Tenant, command.FactID); err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	existing, found, err := handler.unboxes.LoadUnbox(ctx, command.Tenant, command.FactID)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	if found {
		if sameUnbox(existing, command) {
			return InstructionResult{outcome: InstructionExisting}, nil
		}
		return InstructionResult{outcome: InstructionConflict}, nil
	}
	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	if !found {
		return InstructionResult{outcome: InstructionWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return InstructionResult{outcome: InstructionWarehouseNotAccepting}, nil
	}
	if err := handler.cartons.Serialize(ctx, command.Tenant, command.Owner, command.Warehouse); err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	place, err := handler.cartons.Locate(ctx, command.Tenant, command.Owner, command.Warehouse, command.Carton)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	if !place.Found {
		return InstructionResult{outcome: InstructionCartonNotFound}, nil
	}
	usage, foundPlace, err := handler.executionPlace(ctx, command.Tenant, command.Warehouse, place.Location, map[string]placeInfo{})
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	held, err := handler.holds.CartonHeld(ctx, command.Tenant, command.Owner, command.Warehouse, command.Carton)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	if held || (foundPlace && usage == domain.ZoneUsageOutboundStaging) {
		return InstructionResult{outcome: InstructionUnavailable}, nil
	}
	issues, err := handler.screenUnbox(ctx, command)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	if len(issues) > 0 {
		return InstructionResult{outcome: InstructionContentInvalid, issues: issues}, nil
	}
	bill, err := handler.unboxes.CartonBill(ctx, command.Tenant, command.Owner, command.Warehouse, command.Carton)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	inboundAt, foundAt, err := handler.cartons.LoadInboundAt(ctx, command.Tenant, place.Batch)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	if !foundAt {
		return InstructionResult{}, fmt.Errorf("unbox: 箱 %s 没有入库时间", command.Carton)
	}
	movementID, err := handler.postUnboxGoods(ctx, command, inboundAt)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	departureID, err := handler.cartonIDs.NextCartonMovementID()
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	out, err := domain.NewCartonMovementDeltaLine(place.Location, command.Carton, place.Batch, -1)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	source, err := domain.NewSourceFactReference(command.FactID.String())
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	departure, err := domain.NewCartonMovement(
		command.Tenant, departureID, command.Owner, command.Warehouse, domain.CartonMovementUnboxing,
		source, command.OccurredAt, handler.clock.Now(), []domain.CartonMovementLine{out},
	)
	if err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	if err := handler.cartons.AppendDeparture(ctx, departure); err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	fact := ports.UnboxFact{
		Tenant: command.Tenant, FactID: command.FactID, Owner: command.Owner, Warehouse: command.Warehouse, Carton: command.Carton,
		Device: command.Device, Operator: command.Operator, OccurredAt: command.OccurredAt, DeviceSequence: command.DeviceSequence,
		ReceivedAt: handler.clock.Now(), MovementID: movementID, Lines: command.Lines, Differences: unboxDifferences(bill, command.Lines),
	}
	if err := handler.unboxes.SaveUnbox(ctx, fact); err != nil {
		return InstructionResult{}, fmt.Errorf("unbox: %w", err)
	}
	if err := handler.noteUnboxCharge(ctx, command); err != nil {
		return InstructionResult{}, err
	}
	return InstructionResult{outcome: InstructionPosted}, nil
}

func (handler *InstructionHandler) postUnboxGoods(ctx context.Context, command UnboxCommand, inboundAt time.Time) (domain.StockMovementID, error) {
	var (
		lines   []domain.StockMovementLine
		batches []domain.InboundBatch
	)
	movementID, err := handler.stockIDs.NextStockMovementID()
	if err != nil {
		return domain.StockMovementID{}, err
	}
	seen := map[string]domain.InboundBatchID{}
	for _, line := range command.Lines {
		if line.Goods.String() == "" || line.Quantity <= 0 {
			continue
		}
		key := line.Goods.String() + "\x1f" + line.Batch.String() + "\x1f" + line.Expiry.String()
		batchID, ok := seen[key]
		if !ok {
			batchID, err = handler.stockIDs.NextInboundBatchID()
			if err != nil {
				return domain.StockMovementID{}, err
			}
			batch, err := domain.NewInboundBatch(command.Tenant, batchID, command.Owner, command.Warehouse, line.Goods, line.Batch, line.Expiry, inboundAt, movementID)
			if err != nil {
				return domain.StockMovementID{}, err
			}
			batches = append(batches, batch)
			seen[key] = batchID
		}
		cell, err := domain.NewStockCell(domain.StockCellSpec{
			Location: line.Location, Goods: line.Goods, Unit: line.Unit, State: line.State, Alias: line.Alias,
			InboundBatch: batchID, BatchNumber: line.Batch, Expiry: line.Expiry,
		})
		if err != nil {
			return domain.StockMovementID{}, err
		}
		lines = append(lines, domain.NewStockMovementLine(domain.StockIn, line.Quantity, cell, time.Time{}, true))
	}
	if len(lines) == 0 {
		return domain.StockMovementID{}, nil
	}
	source, err := domain.NewSourceFactReference(command.FactID.String())
	if err != nil {
		return domain.StockMovementID{}, err
	}
	movement, err := domain.NewStockMovement(
		command.Tenant, movementID, command.Owner, command.Warehouse, domain.StockReasonUnboxing,
		source, command.OccurredAt, handler.clock.Now(), lines,
	)
	if err != nil {
		return domain.StockMovementID{}, err
	}
	outcome, err := handler.ledger.AppendMovement(ctx, movement, batches)
	if err != nil {
		return domain.StockMovementID{}, err
	}
	if outcome != ports.StockAppended {
		return domain.StockMovementID{}, fmt.Errorf("开箱商品没有入账：%s", outcome)
	}
	return movementID, nil
}

func (handler *InstructionHandler) screenUnbox(ctx context.Context, command UnboxCommand) ([]domain.InternalJobIssue, error) {
	var issues []domain.InternalJobIssue
	places := map[string]placeInfo{}
	for index, line := range command.Lines {
		if line.Quantity <= 0 {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueQuantityNotPositive))
			continue
		}
		if line.Goods.String() == "" {
			if line.Observed == "" {
				issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueLineShape))
			}
			continue
		}
		revision, found, err := handler.goods.LoadGoodsAt(ctx, command.Tenant, command.Owner, line.Goods, command.OccurredAt)
		if err != nil {
			return nil, err
		}
		if !found {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueGoodsNotRegistered))
			continue
		}
		if _, ok := unitFactor(revision, line.Unit); !ok {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueUnitNotDeclared))
		}
		usage, foundPlace, err := handler.executionPlace(ctx, command.Tenant, command.Warehouse, line.Location, places)
		if err != nil {
			return nil, err
		}
		if !foundPlace {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueLocationNotFound))
			continue
		}
		if usage == domain.ZoneUsageOutboundStaging {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueOutboundStaging))
		}
		if !line.State.AllowedIn(usage) {
			issues = append(issues, domain.NewInternalJobIssue(index, domain.InternalJobIssueZoneIncompatible))
		}
	}
	return issues, nil
}

func unboxDifferences(bill []domain.CartonContentLine, lines []ports.UnboxLine) []ports.UnboxDifference {
	type counted struct{ good, damage int64 }
	got := map[string]counted{}
	var differences []ports.UnboxDifference
	for _, line := range lines {
		if line.Goods.String() == "" {
			differences = append(differences, ports.UnboxDifference{Kind: domain.UnboxDifferenceWrong, Quantity: line.Quantity, Observed: line.Observed})
			continue
		}
		item := got[line.Goods.String()]
		if line.State == domain.StockStateDefective {
			item.damage += line.Quantity
		} else {
			item.good += line.Quantity
		}
		got[line.Goods.String()] = item
	}
	billed := map[string]int64{}
	for _, line := range bill {
		billed[line.Goods().String()] += line.Quantity()
	}
	for goods, quantity := range billed {
		item := got[goods]
		code, _ := domain.NewGoodsCode(goods)
		if item.damage > 0 {
			differences = append(differences, ports.UnboxDifference{Kind: domain.UnboxDifferenceDamage, Quantity: item.damage, Goods: code})
		}
		gap := quantity - item.good - item.damage
		switch {
		case gap > 0:
			differences = append(differences, ports.UnboxDifference{Kind: domain.UnboxDifferenceShort, Quantity: gap, Goods: code})
		case gap < 0:
			differences = append(differences, ports.UnboxDifference{Kind: domain.UnboxDifferenceOverage, Quantity: -gap, Goods: code})
		}
		delete(got, goods)
	}
	for goods, item := range got {
		code, _ := domain.NewGoodsCode(goods)
		total := item.good + item.damage
		if total > 0 {
			differences = append(differences, ports.UnboxDifference{Kind: domain.UnboxDifferenceWrong, Quantity: total, Goods: code})
		}
	}
	return differences
}

func sameUnbox(existing ports.UnboxFact, command UnboxCommand) bool {
	if existing.Owner != command.Owner || existing.Warehouse != command.Warehouse || existing.Carton != command.Carton ||
		existing.Device != command.Device || existing.Operator != command.Operator || existing.DeviceSequence != command.DeviceSequence ||
		len(existing.Lines) != len(command.Lines) {
		return false
	}
	if !existing.OccurredAt.Equal(command.OccurredAt.UTC().Truncate(time.Microsecond)) && !existing.OccurredAt.Equal(command.OccurredAt) {
		return false
	}
	for index := range existing.Lines {
		line := existing.Lines[index]
		other := command.Lines[index]
		if line.Observed != other.Observed || line.Goods != other.Goods || line.Unit != other.Unit || line.State != other.State ||
			line.Alias != other.Alias || line.Batch != other.Batch || line.Expiry != other.Expiry || line.Location != other.Location || line.Quantity != other.Quantity {
			return false
		}
	}
	return true
}
