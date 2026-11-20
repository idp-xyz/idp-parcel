package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// StockView 是读口看哪一种。货主面不带库位，也不带冻结的原因与操作者。
type StockView string

const (
	StockViewCells     StockView = "cells"
	StockViewCartons   StockView = "cartons"
	StockViewSummary   StockView = "summary"
	StockViewHolds     StockView = "holds"
	StockViewMovements StockView = "movements"
)

// StockAgeNotConfigured 是仓的业务时区没登记时库龄那一格的答法。数量照答。
const StockAgeNotConfigured = "NOT_CONFIGURED"

// StockCellView 是一格的读数。货主面的库位是空的。
type StockCellView struct {
	Warehouse     string
	Location      string
	Goods         string
	Unit          string
	State         string
	Alias         string
	Batch         string
	Expiry        string
	InboundBatch  string
	InboundAt     time.Time
	Quantity      int64
	Allocatable   bool
	Frozen        int64
	Available     int64
	AgeDays       int
	AgeConfigured bool
}

// StockCartonView 是一箱的读数。货主面不带库位。
type StockCartonView struct {
	Warehouse     string
	Carton        string
	Location      string
	InboundAt     time.Time
	Held          bool
	Bill          []domain.CartonContentLine
	AgeDays       int
	AgeConfigured bool
}

// StockSummaryView 是（货主，仓，商品，计量单位，库存状态）上的汇总。可用量是可分配库存减去各项占用。
type StockSummaryView struct {
	Warehouse   string
	Goods       string
	Unit        string
	State       string
	OnHand      int64
	Allocatable int64
	Frozen      int64
	Reserved    int64
	Locked      int64
	Available   int64
}

// StockHoldView 是一份占用。货主面只有种类、商品与数量。
type StockHoldView struct {
	Kind      string
	Warehouse string
	Goods     string
	Unit      string
	State     string
	Location  string
	Carton    string
	Quantity  int64
	Reason    string
	Operator  string
	FrozenAt  time.Time
}

// StockMovementView 是一笔流水。货主面的行不带库位，也不带回来源引用。
type StockMovementView struct {
	Warehouse string
	Reason    string
	Source    string
	Occurred  time.Time
	Posted    time.Time
	Lines     []StockMovementLineView
}

// StockMovementLineView 是流水一行。
type StockMovementLineView struct {
	Location string
	Goods    string
	Unit     string
	State    string
	Delta    int64
}

// StockSnapshot 是一次查阅答出的那一种。没被点名的切片是空的。
type StockSnapshot struct {
	Cells     []StockCellView
	Cartons   []StockCartonView
	Summary   []StockSummaryView
	Holds     []StockHoldView
	Movements []StockMovementView
}

// StockQuery 是一次查阅。仓可以不点名，点了就只看那一个仓。
type StockQuery struct {
	Tenant    domain.TenantID
	Owner     domain.OwnerAccountID
	Warehouse domain.WarehouseCode
	View      StockView
	Customer  bool
	Now       time.Time
}

// QueryStockHandler 如实答读那一刻的账。一个仓都没登记时答空。
type QueryStockHandler struct {
	reads      ports.StockReadStore
	warehouses ports.WarehouseLookup
	any        ports.TenantWarehouseLookup
	locations  ports.StockLocationLookup
	zones      ports.ZoneLookup
}

func NewQueryStockHandler(
	reads ports.StockReadStore,
	warehouses ports.WarehouseLookup,
	any ports.TenantWarehouseLookup,
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
) *QueryStockHandler {
	return &QueryStockHandler{reads: reads, warehouses: warehouses, any: any, locations: locations, zones: zones}
}

func (handler *QueryStockHandler) Query(ctx context.Context, query StockQuery) (StockSnapshot, error) {
	configured, err := handler.any.HasAnyWarehouse(ctx, query.Tenant)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	if !configured {
		return StockSnapshot{}, nil
	}
	if query.Warehouse.String() != "" {
		warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, query.Tenant, query.Warehouse)
		if err != nil {
			return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
		}
		if !found || warehouse.Content().Kind() == domain.WarehouseKindAgent {
			return StockSnapshot{}, nil
		}
	}
	switch query.View {
	case StockViewCells, StockViewSummary:
		return handler.cells(ctx, query)
	case StockViewCartons:
		return handler.cartons(ctx, query)
	case StockViewHolds:
		return handler.holds(ctx, query)
	case StockViewMovements:
		return handler.movements(ctx, query)
	default:
		return StockSnapshot{}, fmt.Errorf("query stock: 不认识的看法")
	}
}

func (handler *QueryStockHandler) cells(ctx context.Context, query StockQuery) (StockSnapshot, error) {
	rows, err := handler.reads.ListCellBalances(ctx, query.Tenant, query.Owner, query.Warehouse)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	freezes, err := handler.reads.ListActiveFreezes(ctx, query.Tenant, query.Owner, query.Warehouse)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	cells := make([]StockCellView, 0, len(rows))
	for _, row := range rows {
		allocatable, err := handler.allocatable(ctx, query.Tenant, row.Warehouse, row.Location, row.State)
		if err != nil {
			return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
		}
		frozen := frozenOnCell(freezes, row)
		available := int64(0)
		if allocatable {
			available = row.Quantity - frozen
		}
		ageDays, configured, err := handler.age(ctx, query.Tenant, row.Warehouse, row.InboundAt, query.Now)
		if err != nil {
			return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
		}
		cell := StockCellView{
			Warehouse: row.Warehouse, Goods: row.Goods, Unit: row.Unit, State: row.State,
			Alias: textOf(row.Alias), Batch: textOf(row.BatchNumber), Expiry: textOf(row.Expiry),
			InboundBatch: row.InboundBatch, InboundAt: row.InboundAt, Quantity: row.Quantity,
			Allocatable: allocatable, Frozen: frozen, Available: available, AgeDays: ageDays, AgeConfigured: configured,
		}
		if !query.Customer {
			cell.Location = row.Location
		}
		cells = append(cells, cell)
	}
	if query.View == StockViewCells {
		return StockSnapshot{Cells: cells}, nil
	}
	return StockSnapshot{Summary: summarize(cells, freezes)}, nil
}

func (handler *QueryStockHandler) cartons(ctx context.Context, query StockQuery) (StockSnapshot, error) {
	rows, err := handler.reads.ListCartonBalances(ctx, query.Tenant, query.Owner, query.Warehouse)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	freezes, err := handler.reads.ListActiveFreezes(ctx, query.Tenant, query.Owner, query.Warehouse)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	cartons := make([]StockCartonView, 0, len(rows))
	for _, row := range rows {
		warehouse, err := domain.NewWarehouseCode(row.Warehouse)
		if err != nil {
			return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
		}
		carton, err := domain.NewCartonNumber(row.Carton)
		if err != nil {
			return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
		}
		bill, err := handler.reads.CartonBill(ctx, query.Tenant, query.Owner, warehouse, carton)
		if err != nil {
			return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
		}
		ageDays, configured, err := handler.age(ctx, query.Tenant, row.Warehouse, row.InboundAt, query.Now)
		if err != nil {
			return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
		}
		view := StockCartonView{
			Warehouse: row.Warehouse, Carton: row.Carton, InboundAt: row.InboundAt, Bill: bill,
			Held: cartonHeld(freezes, row), AgeDays: ageDays, AgeConfigured: configured,
		}
		if !query.Customer {
			view.Location = row.Location
		}
		cartons = append(cartons, view)
	}
	return StockSnapshot{Cartons: cartons}, nil
}

func (handler *QueryStockHandler) holds(ctx context.Context, query StockQuery) (StockSnapshot, error) {
	freezes, err := handler.reads.ListActiveFreezes(ctx, query.Tenant, query.Owner, query.Warehouse)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	holds := make([]StockHoldView, 0, len(freezes))
	for _, freeze := range freezes {
		if freeze.Target != "GOODS" && query.Customer {
			hold := StockHoldView{Kind: "FREEZE", Warehouse: freeze.Warehouse, Carton: freeze.Carton, Quantity: freeze.Quantity}
			holds = append(holds, hold)
			continue
		}
		hold := StockHoldView{
			Kind: "FREEZE", Warehouse: freeze.Warehouse, Goods: freeze.Goods, Unit: freeze.Unit, State: freeze.State,
			Quantity: freeze.Quantity,
		}
		if freeze.Target == "CARTON" {
			hold.Carton = freeze.Carton
		}
		if !query.Customer {
			hold.Location = freeze.Location
			hold.Reason = freeze.Reason
			hold.Operator = freeze.Operator
			hold.FrozenAt = freeze.FrozenAt
		}
		holds = append(holds, hold)
	}
	return StockSnapshot{Holds: holds}, nil
}

func (handler *QueryStockHandler) movements(ctx context.Context, query StockQuery) (StockSnapshot, error) {
	rows, err := handler.reads.ListMovements(ctx, query.Tenant, query.Owner, query.Warehouse, 200)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	movements := make([]StockMovementView, 0, len(rows))
	for _, row := range rows {
		view := StockMovementView{Warehouse: row.Warehouse, Reason: row.Reason, Occurred: row.Occurred}
		if !query.Customer {
			view.Source = row.Source
			view.Posted = row.Posted
		}
		for _, line := range row.Lines {
			item := StockMovementLineView{Goods: line.Goods, Unit: line.Unit, State: line.State, Delta: line.Delta}
			if !query.Customer {
				item.Location = line.Location
			}
			view.Lines = append(view.Lines, item)
		}
		movements = append(movements, view)
	}
	return StockSnapshot{Movements: movements}, nil
}

func (handler *QueryStockHandler) allocatable(ctx context.Context, tenant domain.TenantID, warehouse, location, state string) (bool, error) {
	warehouseCode, err := domain.NewWarehouseCode(warehouse)
	if err != nil {
		return false, err
	}
	locationCode, err := domain.NewLocationCode(location)
	if err != nil {
		return false, err
	}
	stockState, err := domain.NewStockState(state)
	if err != nil {
		return false, err
	}
	revision, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouseCode, locationCode)
	if err != nil || !found {
		return false, err
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouseCode, revision.Content().ZoneCode())
	if err != nil || !found {
		return false, err
	}
	return domain.AllocatableGoods(stockState, zone.Content().Usage()), nil
}

func (handler *QueryStockHandler) age(ctx context.Context, tenant domain.TenantID, warehouse string, inbound, now time.Time) (int, bool, error) {
	code, err := domain.NewWarehouseCode(warehouse)
	if err != nil {
		return 0, false, err
	}
	revision, found, err := handler.warehouses.LoadLatestWarehouse(ctx, tenant, code)
	if err != nil || !found {
		return 0, false, err
	}
	return StockAgeDays(inbound, now, revision.Content().BusinessTimeZone())
}

// StockAgeDays 按仓的业务时区把入库日到阅读日折成整天。时区没登记时第二格为假。
func StockAgeDays(inbound, now time.Time, zone domain.BusinessTimeZone) (int, bool, error) {
	if !zone.Registered() {
		return 0, false, nil
	}
	location, err := time.LoadLocation(zone.String())
	if err != nil {
		return 0, false, fmt.Errorf("stock age: %w", err)
	}
	start := dateOf(inbound.In(location))
	end := dateOf(now.In(location))
	days := int(end.Sub(start).Hours() / 24)
	if days < 0 {
		days = 0
	}
	return days, true, nil
}

func dateOf(instant time.Time) time.Time {
	year, month, day := instant.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func summarize(cells []StockCellView, freezes []ports.StockFreezeRow) []StockSummaryView {
	type key struct{ warehouse, goods, unit, state string }
	order := make([]key, 0)
	grouped := map[key]*StockSummaryView{}
	for _, cell := range cells {
		id := key{cell.Warehouse, cell.Goods, cell.Unit, cell.State}
		summary, ok := grouped[id]
		if !ok {
			summary = &StockSummaryView{Warehouse: cell.Warehouse, Goods: cell.Goods, Unit: cell.Unit, State: cell.State}
			grouped[id] = summary
			order = append(order, id)
		}
		summary.OnHand += cell.Quantity
		if cell.Allocatable {
			summary.Allocatable += cell.Quantity
			summary.Frozen += cell.Frozen
		}
	}
	_ = freezes
	views := make([]StockSummaryView, 0, len(order))
	for _, id := range order {
		summary := grouped[id]
		summary.Available = summary.Allocatable - summary.Frozen - summary.Reserved - summary.Locked
		views = append(views, *summary)
	}
	return views
}

func frozenOnCell(freezes []ports.StockFreezeRow, row ports.StockCellBalance) int64 {
	var total int64
	for _, freeze := range freezes {
		if freeze.Target == "GOODS" && freeze.Warehouse == row.Warehouse && freeze.Location == row.Location && freeze.Goods == row.Goods && freeze.Unit == row.Unit && freeze.State == row.State && freeze.InboundBatch == row.InboundBatch {
			total += freeze.Quantity
		}
	}
	return total
}

func cartonHeld(freezes []ports.StockFreezeRow, row ports.StockCartonBalance) bool {
	for _, freeze := range freezes {
		if freeze.Target == "CARTON" && freeze.Warehouse == row.Warehouse && freeze.Carton == row.Carton {
			return true
		}
	}
	return false
}

func textOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
