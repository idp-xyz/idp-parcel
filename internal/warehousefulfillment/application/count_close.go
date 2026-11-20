package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func countAllocatable(ctx context.Context, handler *CountHandler, tenant domain.TenantID, warehouse domain.WarehouseCode, location domain.LocationCode, state domain.StockState) (bool, error) {
	revision, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, location)
	if err != nil || !found {
		return false, err
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, revision.Content().ZoneCode())
	if err != nil || !found {
		return false, err
	}
	return domain.AllocatableGoods(state, zone.Content().Usage()), nil
}

func allocatableOf(ctx context.Context, handler *CountHandler, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, goods domain.GoodsCode, unit domain.GoodsUnitName, state domain.StockState) (int64, error) {
	rows, err := handler.counts.LocationBalances(ctx, tenant, warehouse, nil)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, row := range rows {
		if row.Owner != owner.String() || row.Goods != goods.String() || row.Unit != unit.String() || row.State != state.String() {
			continue
		}
		location, err := domain.NewLocationCode(row.Location)
		if err != nil {
			return 0, err
		}
		ok, err := countAllocatable(ctx, handler, tenant, warehouse, location, state)
		if err != nil {
			return 0, err
		}
		if ok {
			total += row.Quantity
		}
	}
	return total, nil
}

func frozenOf(ctx context.Context, handler *CountHandler, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, goods domain.GoodsCode, unit domain.GoodsUnitName, state domain.StockState) (int64, error) {
	rows, err := handler.counts.LocationBalances(ctx, tenant, warehouse, nil)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, row := range rows {
		if row.Owner != owner.String() || row.Goods != goods.String() || row.Unit != unit.String() || row.State != state.String() {
			continue
		}
		_, _, location, cell, err := countCell(row)
		if err != nil {
			return 0, err
		}
		ok, err := countAllocatable(ctx, handler, tenant, warehouse, location, state)
		if err != nil {
			return 0, err
		}
		if !ok {
			continue
		}
		held, err := handler.holds.CellHold(ctx, tenant, owner, warehouse, cell)
		if err != nil {
			return 0, err
		}
		total += held
	}
	return total, nil
}

func (handler *CountHandler) Record(ctx context.Context, command RecordCountCommand) (CountResult, error) {
	if command.Tenant.String() == "" || command.FactID.String() == "" || command.CountID.String() == "" || command.Location.String() == "" {
		return CountResult{}, fmt.Errorf("record count: 命令不完整")
	}
	if err := handler.counts.HoldCount(ctx, command.Tenant, command.CountID); err != nil {
		return CountResult{}, fmt.Errorf("record count: %w", err)
	}
	count, found, err := handler.counts.Load(ctx, command.Tenant, command.CountID)
	if err != nil {
		return CountResult{}, fmt.Errorf("record count: %w", err)
	}
	if !found {
		return CountResult{outcome: CountNotFound}, nil
	}
	if count.Warehouse() != command.Warehouse {
		return CountResult{outcome: CountContentInvalid, issues: []domain.FreezeIssue{domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound)}}, nil
	}
	if count.Status() != domain.CountCounting {
		return CountResult{outcome: CountStatusMismatch, count: count, has: true, status: count.Status().String()}, nil
	}
	covers := false
	for _, location := range count.Locations() {
		if location == command.Location {
			covers = true
		}
	}
	if !covers {
		return CountResult{outcome: CountContentInvalid, issues: []domain.FreezeIssue{domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound)}}, nil
	}
	entries, err := handler.counts.LoadEntries(ctx, command.Tenant, command.CountID)
	if err != nil {
		return CountResult{}, fmt.Errorf("record count: %w", err)
	}
	for _, entry := range entries {
		if entry.FactID == command.FactID {
			if sameEntry(entry, command) {
				return CountResult{outcome: CountExisting, count: count, has: true}, nil
			}
			return CountResult{outcome: CountContentConflict}, nil
		}
	}
	entry := ports.CountEntry{
		Tenant: command.Tenant, FactID: command.FactID, CountID: command.CountID, Location: command.Location,
		OccurredAt: command.OccurredAt, Sequence: command.Sequence, Device: command.Device, Operator: command.Operator,
		ReceivedAt: handler.clock.Now(), Lines: command.Lines,
	}
	if err := handler.counts.SaveEntry(ctx, entry); err != nil {
		return CountResult{}, fmt.Errorf("record count: %w", err)
	}
	return CountResult{outcome: CountRecorded, count: count, has: true}, nil
}

func (handler *CountHandler) Complete(ctx context.Context, command CountCommand) (CountResult, error) {
	count, result, err := handler.open(ctx, command, "complete count")
	if err != nil || result.outcome != 0 {
		return result, err
	}
	switch count.Status() {
	case domain.CountCounted:
		return CountResult{outcome: CountExisting, count: count, has: true}, nil
	case domain.CountPending, domain.CountVoid:
		return CountResult{outcome: CountStatusMismatch, count: count, has: true, status: count.Status().String()}, nil
	}
	entries, err := handler.counts.LoadEntries(ctx, command.Tenant, count.ID())
	if err != nil {
		return CountResult{}, fmt.Errorf("complete count: %w", err)
	}
	current := map[string]ports.CountEntry{}
	for _, entry := range entries {
		previous, ok := current[entry.Location.String()]
		if !ok || entry.OccurredAt.After(previous.OccurredAt) || (entry.OccurredAt.Equal(previous.OccurredAt) && entry.Sequence > previous.Sequence) {
			current[entry.Location.String()] = entry
		}
	}
	var missing []string
	for _, location := range count.Locations() {
		if _, ok := current[location.String()]; !ok {
			missing = append(missing, location.String())
		}
	}
	if len(missing) > 0 {
		return CountResult{outcome: CountIncomplete, missing: missing, count: count, has: true}, nil
	}
	rows, err := handler.counts.LocationBalances(ctx, command.Tenant, count.Warehouse(), count.Locations())
	if err != nil {
		return CountResult{}, fmt.Errorf("complete count: %w", err)
	}
	book := map[countKey]int64{}
	for _, row := range rows {
		owner, goods, location, cell, err := countCell(row)
		if err != nil {
			return CountResult{}, fmt.Errorf("complete count: %w", err)
		}
		if !count.Covers(owner, goods, location) {
			continue
		}
		book[countKeyOf(owner, goods, cell, location)] += row.Quantity
	}
	counted := map[countKey]int64{}
	for _, entry := range current {
		for _, line := range entry.Lines {
			if line.Observed != "" {
				continue
			}
			counted[countKeyOfLine(line, entry.Location)] += line.Quantity
		}
	}
	var differences []ports.CountDifference
	seen := map[countKey]bool{}
	for key, bookQty := range book {
		seen[key] = true
		if bookQty != counted[key] {
			id, err := handler.differences.NextStockDiscrepancyID()
			if err != nil {
				return CountResult{}, fmt.Errorf("complete count: %w", err)
			}
			differences = append(differences, differenceOf(id, key, bookQty, counted[key]))
		}
	}
	for key, countedQty := range counted {
		if seen[key] {
			continue
		}
		id, err := handler.differences.NextStockDiscrepancyID()
		if err != nil {
			return CountResult{}, fmt.Errorf("complete count: %w", err)
		}
		differences = append(differences, differenceOf(id, key, 0, countedQty))
	}
	finished, err := count.Finish(command.Operator, handler.clock.Now())
	if err != nil {
		return CountResult{}, fmt.Errorf("complete count: %w", err)
	}
	if err := handler.counts.SaveFinish(ctx, finished, differences); err != nil {
		return CountResult{}, fmt.Errorf("complete count: %w", err)
	}
	return CountResult{outcome: CountCounted, count: finished, has: true}, nil
}

func (handler *CountHandler) Void(ctx context.Context, command CountCommand) (CountResult, error) {
	count, result, err := handler.open(ctx, command, "void count")
	if err != nil || result.outcome != 0 {
		return result, err
	}
	switch count.Status() {
	case domain.CountVoid:
		return CountResult{outcome: CountExisting, count: count, has: true}, nil
	case domain.CountCounted:
		return CountResult{outcome: CountStatusMismatch, count: count, has: true, status: count.Status().String()}, nil
	}
	voided, err := count.Void(command.Operator, handler.clock.Now(), command.Reason)
	if err != nil {
		return CountResult{}, fmt.Errorf("void count: %w", err)
	}
	if err := handler.counts.SaveVoid(ctx, voided); err != nil {
		return CountResult{}, fmt.Errorf("void count: %w", err)
	}
	return CountResult{outcome: CountVoided, count: voided, has: true}, nil
}

type countKey struct {
	owner, goods, unit, state, alias, batch, expiry, location string
}

func countKeyOf(owner domain.OwnerAccountID, goods domain.GoodsCode, cell domain.StockCell, location domain.LocationCode) countKey {
	return countKey{owner.String(), goods.String(), cell.Unit().String(), cell.State().String(), cell.Alias().String(), cell.BatchNumber().String(), cell.Expiry().String(), location.String()}
}

func countKeyOfLine(line ports.CountEntryLine, location domain.LocationCode) countKey {
	return countKey{line.Owner.String(), line.Goods.String(), line.Unit.String(), line.State.String(), line.Alias.String(), line.Batch.String(), line.Expiry.String(), location.String()}
}

func differenceOf(id domain.StockDiscrepancyID, key countKey, book, counted int64) ports.CountDifference {
	owner, _ := domain.NewOwnerAccountID(key.owner)
	goods, _ := domain.NewGoodsCode(key.goods)
	unit, _ := domain.NewGoodsUnitName(key.unit)
	state, _ := domain.NewStockState(key.state)
	location, _ := domain.NewLocationCode(key.location)
	var alias domain.GoodsAliasCode
	if key.alias != "" {
		alias, _ = domain.NewGoodsAliasCode(key.alias)
	}
	var batch domain.BatchNumber
	if key.batch != "" {
		batch, _ = domain.NewBatchNumber(key.batch)
	}
	var expiry domain.ExpiryDate
	if key.expiry != "" {
		expiry, _ = domain.NewExpiryDate(key.expiry)
	}
	return ports.CountDifference{ID: id, Owner: owner, Location: location, Goods: goods, Unit: unit, State: state, Alias: alias, Batch: batch, Expiry: expiry, Book: book, Counted: counted}
}

func sameEntry(entry ports.CountEntry, command RecordCountCommand) bool {
	if entry.Location != command.Location || len(entry.Lines) != len(command.Lines) {
		return false
	}
	for index, line := range entry.Lines {
		other := command.Lines[index]
		if line.Owner != other.Owner || line.Goods != other.Goods || line.Unit != other.Unit || line.State != other.State || line.Quantity != other.Quantity || line.Observed != other.Observed {
			return false
		}
	}
	return true
}
