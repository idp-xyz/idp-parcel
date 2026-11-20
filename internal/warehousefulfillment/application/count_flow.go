package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (handler *CountHandler) Create(ctx context.Context, command CreateCountCommand) (CountResult, error) {
	if command.Tenant.String() == "" || command.DecisionID.String() == "" || command.Warehouse.String() == "" || command.Operator.String() == "" || len(command.Locations) == 0 {
		return CountResult{}, fmt.Errorf("create count: 命令不完整")
	}
	if err := handler.counts.HoldDecision(ctx, command.Tenant, command.DecisionID); err != nil {
		return CountResult{}, fmt.Errorf("create count: %w", err)
	}
	spec := domain.CountSpec{
		Tenant: command.Tenant, DecisionID: command.DecisionID, Warehouse: command.Warehouse, Locations: command.Locations,
		Owner: command.Owner, Goods: command.Goods, Note: command.Note, Operator: command.Operator, CreatedAt: handler.clock.Now(),
	}
	existing, found, err := handler.counts.LoadByDecision(ctx, command.Tenant, command.DecisionID)
	if err != nil {
		return CountResult{}, fmt.Errorf("create count: %w", err)
	}
	if found {
		if existing.SameCount(spec) {
			return CountResult{outcome: CountExisting, count: existing, has: true}, nil
		}
		return CountResult{outcome: CountConflict}, nil
	}
	warehouse, foundWarehouse, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return CountResult{}, fmt.Errorf("create count: %w", err)
	}
	if !foundWarehouse {
		return CountResult{outcome: CountWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return CountResult{outcome: CountWarehouseNotAccepting}, nil
	}
	seen := map[string]bool{}
	var issues []domain.FreezeIssue
	for _, location := range command.Locations {
		if seen[location.String()] {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound))
			continue
		}
		seen[location.String()] = true
		revision, foundLocation, err := handler.locations.LoadLatestLocation(ctx, command.Tenant, command.Warehouse, location)
		if err != nil {
			return CountResult{}, fmt.Errorf("create count: %w", err)
		}
		if !foundLocation {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound))
			continue
		}
		zone, foundZone, err := handler.zones.LoadLatestZone(ctx, command.Tenant, command.Warehouse, revision.Content().ZoneCode())
		if err != nil {
			return CountResult{}, fmt.Errorf("create count: %w", err)
		}
		if !foundZone || zone.Content().Usage() == domain.ZoneUsageOutboundStaging {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound))
		}
	}
	for _, goods := range command.Goods {
		_, foundGoods, err := handler.goods.LoadGoodsAt(ctx, command.Tenant, command.Owner, goods, handler.clock.Now())
		if err != nil {
			return CountResult{}, fmt.Errorf("create count: %w", err)
		}
		if !foundGoods {
			issues = append(issues, domain.NewFreezeIssue(domain.FreezeIssueGoodsNotRegistered))
		}
	}
	if len(issues) > 0 {
		return CountResult{outcome: CountContentInvalid, issues: issues}, nil
	}
	id, err := handler.identities.NextCountID()
	if err != nil {
		return CountResult{}, fmt.Errorf("create count: %w", err)
	}
	spec.ID = id
	count, err := domain.NewCount(spec)
	if err != nil {
		return CountResult{}, fmt.Errorf("create count: %w", err)
	}
	if err := handler.counts.Save(ctx, count); err != nil {
		return CountResult{}, fmt.Errorf("create count: %w", err)
	}
	return CountResult{outcome: CountCreated, count: count, has: true}, nil
}

func (handler *CountHandler) Start(ctx context.Context, command CountCommand) (CountResult, error) {
	count, result, err := handler.open(ctx, command, "start count")
	if err != nil || result.outcome != 0 {
		return result, err
	}
	switch count.Status() {
	case domain.CountCounting:
		return CountResult{outcome: CountExisting, count: count, has: true}, nil
	case domain.CountCounted, domain.CountVoid:
		return CountResult{outcome: CountStatusMismatch, count: count, has: true, status: count.Status().String()}, nil
	}
	rows, err := handler.counts.LocationBalances(ctx, command.Tenant, count.Warehouse(), count.Locations())
	if err != nil {
		return CountResult{}, fmt.Errorf("start count: %w", err)
	}
	owners := map[string]domain.OwnerAccountID{}
	for _, row := range rows {
		parsed, err := domain.NewOwnerAccountID(row.Owner)
		if err != nil {
			return CountResult{}, fmt.Errorf("start count: %w", err)
		}
		owners[parsed.String()] = parsed
	}
	for _, owner := range owners {
		if err := handler.posting.SerializePosting(ctx, command.Tenant, owner, count.Warehouse()); err != nil {
			return CountResult{}, fmt.Errorf("start count: %w", err)
		}
	}
	rows, err = handler.counts.LocationBalances(ctx, command.Tenant, count.Warehouse(), count.Locations())
	if err != nil {
		return CountResult{}, fmt.Errorf("start count: %w", err)
	}
	var locks []domain.CountLock
	type holdKey struct {
		owner domain.OwnerAccountID
		goods domain.GoodsCode
		unit  domain.GoodsUnitName
		state domain.StockState
	}
	held := map[holdKey]int64{}
	for _, row := range rows {
		owner, goods, location, cell, err := countCell(row)
		if err != nil {
			return CountResult{}, fmt.Errorf("start count: %w", err)
		}
		if !count.Covers(owner, goods, location) {
			continue
		}
		overlap, err := handler.counts.CountingOverlap(ctx, command.Tenant, count.Warehouse(), location, owner, goods)
		if err != nil {
			return CountResult{}, fmt.Errorf("start count: %w", err)
		}
		if overlap {
			return CountResult{outcome: CountCannotStart, block: CountBlockedByOther, location: location.String()}, nil
		}
		frozen, err := handler.holds.CellHold(ctx, command.Tenant, owner, count.Warehouse(), cell)
		if err != nil {
			return CountResult{}, fmt.Errorf("start count: %w", err)
		}
		free := row.Quantity - frozen
		if free < 0 {
			free = 0
		}
		if free == 0 {
			continue
		}
		allocatable, err := countAllocatable(ctx, handler, command.Tenant, count.Warehouse(), location, cell.State())
		if err != nil {
			return CountResult{}, fmt.Errorf("start count: %w", err)
		}
		if allocatable {
			held[holdKey{owner, goods, cell.Unit(), cell.State()}] += free
		}
		locks = append(locks, domain.CountLock{Owner: owner, Location: location, Cell: cell, Quantity: free})
	}
	for key, quantity := range held {
		owner, goods, unit, state := key.owner, key.goods, key.unit, key.state
		reserved, err := handler.holds.UnassignedReservation(ctx, command.Tenant, owner, count.Warehouse(), goods, state, domain.GoodsAliasCode{}, domain.BatchNumber{}, domain.ExpiryDate{})
		if err != nil {
			return CountResult{}, fmt.Errorf("start count: %w", err)
		}
		allocatable, err := allocatableOf(ctx, handler, command.Tenant, owner, count.Warehouse(), goods, unit, state)
		if err != nil {
			return CountResult{}, fmt.Errorf("start count: %w", err)
		}
		frozen, err := frozenOf(ctx, handler, command.Tenant, owner, count.Warehouse(), goods, unit, state)
		if err != nil {
			return CountResult{}, fmt.Errorf("start count: %w", err)
		}
		if allocatable < reserved+frozen+quantity {
			return CountResult{outcome: CountCannotStart, block: CountBlockedByHold}, nil
		}
	}
	started, err := count.Start(command.Operator, handler.clock.Now())
	if err != nil {
		return CountResult{}, fmt.Errorf("start count: %w", err)
	}
	if err := handler.counts.SaveStart(ctx, started, locks); err != nil {
		return CountResult{}, fmt.Errorf("start count: %w", err)
	}
	return CountResult{outcome: CountStarted, count: started, has: true}, nil
}

func (handler *CountHandler) open(ctx context.Context, command CountCommand, operation string) (domain.Count, CountResult, error) {
	if command.Tenant.String() == "" || command.ID.String() == "" || command.Operator.String() == "" {
		return domain.Count{}, CountResult{}, fmt.Errorf("%s: 命令不完整", operation)
	}
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return domain.Count{}, CountResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !configured {
		return domain.Count{}, CountResult{outcome: CountNotConfigured}, nil
	}
	if err := handler.counts.HoldCount(ctx, command.Tenant, command.ID); err != nil {
		return domain.Count{}, CountResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	count, found, err := handler.counts.Load(ctx, command.Tenant, command.ID)
	if err != nil {
		return domain.Count{}, CountResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return domain.Count{}, CountResult{outcome: CountNotFound}, nil
	}
	return count, CountResult{}, nil
}

func countCell(row ports.StockCellBalance) (domain.OwnerAccountID, domain.GoodsCode, domain.LocationCode, domain.StockCell, error) {
	owner, err := domain.NewOwnerAccountID(row.Owner)
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.LocationCode{}, domain.StockCell{}, err
	}
	goods, err := domain.NewGoodsCode(row.Goods)
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.LocationCode{}, domain.StockCell{}, err
	}
	location, err := domain.NewLocationCode(row.Location)
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.LocationCode{}, domain.StockCell{}, err
	}
	unit, err := domain.NewGoodsUnitName(row.Unit)
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.LocationCode{}, domain.StockCell{}, err
	}
	state, err := domain.NewStockState(row.State)
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.LocationCode{}, domain.StockCell{}, err
	}
	batch, err := domain.NewInboundBatchID(row.InboundBatch)
	if err != nil {
		return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.LocationCode{}, domain.StockCell{}, err
	}
	var alias domain.GoodsAliasCode
	if row.Alias != nil {
		alias, err = domain.NewGoodsAliasCode(*row.Alias)
		if err != nil {
			return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.LocationCode{}, domain.StockCell{}, err
		}
	}
	var lot domain.BatchNumber
	if row.BatchNumber != nil {
		lot, err = domain.NewBatchNumber(*row.BatchNumber)
		if err != nil {
			return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.LocationCode{}, domain.StockCell{}, err
		}
	}
	var expiry domain.ExpiryDate
	if row.Expiry != nil && *row.Expiry != "" {
		expiry, err = domain.NewExpiryDate(*row.Expiry)
		if err != nil {
			return domain.OwnerAccountID{}, domain.GoodsCode{}, domain.LocationCode{}, domain.StockCell{}, err
		}
	}
	cell, err := domain.NewStockCell(domain.StockCellSpec{
		Location: location, Goods: goods, Unit: unit, State: state, Alias: alias, InboundBatch: batch, BatchNumber: lot, Expiry: expiry,
	})
	return owner, goods, location, cell, err
}
