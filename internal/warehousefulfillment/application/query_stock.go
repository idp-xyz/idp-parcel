package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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
	Warehouse    string
	Location     string
	Goods        string
	Unit         string
	State        string
	Alias        string
	Batch        string
	Expiry       string
	InboundBatch string
	InboundAt    time.Time
	Quantity     int64
	Allocatable  bool
	// ZoneUsage 是这一格所在库区的用途。货主面不带。可发是 0 时用它说明为什么不分配。
	ZoneUsage     string
	Frozen        int64
	Locked        int64
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
	// Reference 是冻结号或盘点号。货主面不带。
	Reference string
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
	// MovementPage 只在看流水时有：流水按游标翻页。
	MovementPage *cataloguepage.Page
	// Book 只在库存格、整箱、汇总、占用按页取时有。页码从 1 起。
	Book *ports.Book
}

// StockQuery 是一次查阅。仓可以不点名，点了就只看那一个仓。
// Keyword 是运营面的检索词，空串即不收窄。先按它收窄再切页：库存格、整箱、汇总、占用认商品编码、商品名称、
// 条码与平台商品码、贴标别名、库位与箱号；流水的收窄在读面，游标摘要里带着同一个词。
type StockQuery struct {
	Tenant    domain.TenantID
	Owner     domain.OwnerAccountID
	Warehouse domain.WarehouseCode
	View      StockView
	Customer  bool
	Keyword   string
	// State 收窄库存格与汇总的库存状态。Held 收窄整箱（true 已冻结，false 可用）。Hold 收窄占用。
	// 空串即这一维不收窄。先收窄再切页。
	State string
	Held  string
	Hold  string
	Now   time.Time
	// Page 与 Limit 在看流水时是游标与页大小。Number 大于 0 时，Limit 是库存格、整箱、汇总、占用的页大小，Number 是页码。
	Page   cataloguepage.Query
	Limit  int
	Number int
}

// QueryStockHandler 如实答读那一刻的账。一个仓都没登记时答空。
type QueryStockHandler struct {
	reads        ports.StockReadStore
	warehouses   ports.WarehouseLookup
	any          ports.TenantWarehouseLookup
	locations    ports.StockLocationLookup
	zones        ports.ZoneLookup
	reservations interface {
		ListReservations(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StoredReservation, error)
	}
	goods ports.StockGoodsLookup
}

func NewQueryStockHandler(
	reads ports.StockReadStore,
	warehouses ports.WarehouseLookup,
	any ports.TenantWarehouseLookup,
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
	reservations interface {
		ListReservations(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StoredReservation, error)
	},
	goods ports.StockGoodsLookup,
) *QueryStockHandler {
	return &QueryStockHandler{
		reads: reads, warehouses: warehouses, any: any, locations: locations, zones: zones,
		reservations: reservations, goods: goods,
	}
}

func (handler *QueryStockHandler) Query(ctx context.Context, query StockQuery) (StockSnapshot, error) {
	configured, err := handler.any.HasAnyWarehouse(ctx, query.Tenant)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	if !configured {
		return emptyStock(query), nil
	}
	if query.Warehouse.String() != "" {
		warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, query.Tenant, query.Warehouse)
		if err != nil {
			return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
		}
		if !found || warehouse.Content().Kind() == domain.WarehouseKindAgent {
			return emptyStock(query), nil
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

// emptyStock 是没有账可答时的空答复。看流水时仍带 page 一格：一行也没有也是一页，调用方不必区分「没有流水」与「这一口不翻页」。
func emptyStock(query StockQuery) StockSnapshot {
	if query.View == StockViewMovements {
		page := cataloguepage.NewPage(query.Limit, "", 0)
		return StockSnapshot{MovementPage: &page}
	}
	if query.Number > 0 {
		book := ports.Book{Page: query.Number, PageSize: query.Limit}
		return StockSnapshot{Book: &book}
	}
	return StockSnapshot{}
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
	locks, err := handler.reads.ListActiveLocks(ctx, query.Tenant, query.Owner, query.Warehouse)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	rows, err = handler.narrowCells(ctx, query, rows)
	if err != nil {
		return StockSnapshot{}, err
	}
	if query.State != "" {
		rows = cellsInState(rows, query.State)
	}
	// 汇总要先看完整的账。库存格按页取时，只给这一页逐格问可分配与库龄。收窄在切页之前，book 的总数才是收窄之后的。
	if query.View == StockViewCells && query.Number > 0 {
		window, book := ports.CutBook(rows, query.Number, query.Limit)
		cells, err := handler.cellViews(ctx, query, window, freezes, locks)
		if err != nil {
			return StockSnapshot{}, err
		}
		return StockSnapshot{Cells: cells, Book: &book}, nil
	}
	cells, err := handler.cellViews(ctx, query, rows, freezes, locks)
	if err != nil {
		return StockSnapshot{}, err
	}
	if query.View == StockViewCells {
		return StockSnapshot{Cells: cells}, nil
	}
	summary := summarize(cells, freezes)
	if err := handler.applyReservations(ctx, query, summary); err != nil {
		return StockSnapshot{}, err
	}
	if query.Number > 0 {
		window, book := ports.CutBook(summary, query.Number, query.Limit)
		return StockSnapshot{Summary: window, Book: &book}, nil
	}
	return StockSnapshot{Summary: summary}, nil
}

func (handler *QueryStockHandler) cellViews(ctx context.Context, query StockQuery, rows []ports.StockCellBalance, freezes []ports.StockFreezeRow, locks []ports.StockLockRow) ([]StockCellView, error) {
	cells := make([]StockCellView, 0, len(rows))
	for _, row := range rows {
		allocatable, usage, err := handler.allocatable(ctx, query.Tenant, row.Warehouse, row.Location, row.State)
		if err != nil {
			return nil, fmt.Errorf("query stock: %w", err)
		}
		frozen := frozenOnCell(freezes, row)
		locked := lockedOnCell(locks, row)
		available := int64(0)
		if allocatable {
			available = row.Quantity - frozen - locked
			if available < 0 {
				available = 0
			}
		}
		ageDays, configured, err := handler.age(ctx, query.Tenant, row.Warehouse, row.InboundAt, query.Now)
		if err != nil {
			return nil, fmt.Errorf("query stock: %w", err)
		}
		cell := StockCellView{
			Warehouse: row.Warehouse, Goods: row.Goods, Unit: row.Unit, State: row.State,
			Alias: textOf(row.Alias), Batch: textOf(row.BatchNumber), Expiry: textOf(row.Expiry),
			InboundBatch: row.InboundBatch, InboundAt: row.InboundAt, Quantity: row.Quantity,
			Allocatable: allocatable, Frozen: frozen, Locked: locked, Available: available, AgeDays: ageDays, AgeConfigured: configured,
		}
		if !query.Customer {
			cell.Location = row.Location
			cell.ZoneUsage = usage
		}
		cells = append(cells, cell)
	}
	return cells, nil
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
	rows, err = handler.narrowCartons(ctx, query, rows)
	if err != nil {
		return StockSnapshot{}, err
	}
	if query.Held != "" {
		rows = cartonsHeld(rows, freezes, query.Held == "true")
	}
	var book *ports.Book
	if query.Number > 0 {
		window, cut := ports.CutBook(rows, query.Number, query.Limit)
		rows = window
		book = &cut
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
	return StockSnapshot{Cartons: cartons, Book: book}, nil
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
			hold.Reference = freeze.ID
		}
		holds = append(holds, hold)
	}
	locks, err := handler.reads.ListActiveLocks(ctx, query.Tenant, query.Owner, query.Warehouse)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	for _, lock := range locks {
		hold := StockHoldView{
			Kind: "LOCKED", Warehouse: lock.Warehouse, Goods: lock.Goods, Unit: lock.Unit, State: lock.State,
			Quantity: lock.Quantity,
		}
		if !query.Customer {
			hold.Location = lock.Location
			hold.Reference = lock.CountID
		}
		holds = append(holds, hold)
	}
	reserved, err := handler.reservationHolds(ctx, query)
	if err != nil {
		return StockSnapshot{}, err
	}
	holds = append(holds, reserved...)
	holds, err = handler.narrowHolds(ctx, query, holds)
	if err != nil {
		return StockSnapshot{}, err
	}
	if query.Hold != "" {
		holds = holdsOfKind(holds, query.Hold)
	}
	if query.Number > 0 {
		window, book := ports.CutBook(holds, query.Number, query.Limit)
		return StockSnapshot{Holds: window, Book: &book}, nil
	}
	return StockSnapshot{Holds: holds}, nil
}

func (handler *QueryStockHandler) movements(ctx context.Context, query StockQuery) (StockSnapshot, error) {
	listed, err := handler.reads.ListMovements(ctx, query.Tenant, query.Owner, query.Warehouse, query.Limit, query.Page)
	if err != nil {
		return StockSnapshot{}, fmt.Errorf("query stock: %w", err)
	}
	page := listed.Page(query.Limit)
	movements := make([]StockMovementView, 0, len(listed.Rows))
	for _, row := range listed.Rows {
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
	return StockSnapshot{Movements: movements, MovementPage: &page}, nil
}

func (handler *QueryStockHandler) allocatable(ctx context.Context, tenant domain.TenantID, warehouse, location, state string) (bool, string, error) {
	warehouseCode, err := domain.NewWarehouseCode(warehouse)
	if err != nil {
		return false, "", err
	}
	locationCode, err := domain.NewLocationCode(location)
	if err != nil {
		return false, "", err
	}
	stockState, err := domain.NewStockState(state)
	if err != nil {
		return false, "", err
	}
	revision, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouseCode, locationCode)
	if err != nil || !found {
		return false, "", err
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouseCode, revision.Content().ZoneCode())
	if err != nil || !found {
		return false, "", err
	}
	usage := zone.Content().Usage()
	return domain.AllocatableGoods(stockState, usage), usage.String(), nil
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

func (handler *QueryStockHandler) applyReservations(ctx context.Context, query StockQuery, summary []StockSummaryView) error {
	reserved, err := handler.reservationTotals(ctx, query)
	if err != nil || len(reserved) == 0 {
		return err
	}
	for index := range summary {
		item := &summary[index]
		base, ok, err := handler.baseUnit(ctx, query.Tenant, query.Owner, item.Goods)
		if err != nil {
			return err
		}
		if !ok || item.Unit != base {
			continue
		}
		item.Reserved = reserved[item.Warehouse+"\x1f"+item.Goods+"\x1f"+item.State]
		item.Available = item.Allocatable - item.Frozen - item.Reserved - item.Locked
		if item.Available < 0 {
			item.Available = 0
		}
	}
	return nil
}

func (handler *QueryStockHandler) reservationHolds(ctx context.Context, query StockQuery) ([]StockHoldView, error) {
	rows, err := handler.listedReservations(ctx, query)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	holds := make([]StockHoldView, 0, len(rows))
	for _, row := range rows {
		if row.Quantity == 0 {
			continue
		}
		unit := ""
		base, ok, err := handler.baseUnit(ctx, query.Tenant, query.Owner, row.Goods.String())
		if err != nil {
			return nil, err
		}
		if ok {
			unit = base
		}
		holds = append(holds, StockHoldView{
			Kind: "RESERVATION", Warehouse: row.Warehouse.String(), Goods: row.Goods.String(), Unit: unit, State: row.State.String(),
			Quantity: row.Quantity,
		})
	}
	return holds, nil
}

func (handler *QueryStockHandler) reservationTotals(ctx context.Context, query StockQuery) (map[string]int64, error) {
	rows, err := handler.listedReservations(ctx, query)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	totals := map[string]int64{}
	for _, row := range rows {
		totals[row.Warehouse.String()+"\x1f"+row.Goods.String()+"\x1f"+row.State.String()] += row.Quantity
	}
	return totals, nil
}

func (handler *QueryStockHandler) listedReservations(ctx context.Context, query StockQuery) ([]ports.StoredReservation, error) {
	if handler.reservations == nil {
		return nil, nil
	}
	rows, err := handler.reservations.ListReservations(ctx, query.Tenant, query.Owner, query.Warehouse)
	if err != nil {
		return nil, fmt.Errorf("query stock: %w", err)
	}
	return rows, nil
}

func (handler *QueryStockHandler) baseUnit(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, goods string) (string, bool, error) {
	if handler.goods == nil || goods == "" {
		return "", false, nil
	}
	code, err := domain.NewGoodsCode(goods)
	if err != nil {
		return "", false, err
	}
	revision, found, err := handler.goods.LoadLatestGoods(ctx, tenant, owner, code)
	if err != nil || !found {
		return "", false, err
	}
	return revision.Content().BaseUnit().String(), true, nil
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
			summary.Locked += cell.Locked
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

func cellsInState(rows []ports.StockCellBalance, state string) []ports.StockCellBalance {
	kept := make([]ports.StockCellBalance, 0, len(rows))
	for _, row := range rows {
		if row.State == state {
			kept = append(kept, row)
		}
	}
	return kept
}

func cartonsHeld(rows []ports.StockCartonBalance, freezes []ports.StockFreezeRow, held bool) []ports.StockCartonBalance {
	kept := make([]ports.StockCartonBalance, 0, len(rows))
	for _, row := range rows {
		if cartonHeld(freezes, row) == held {
			kept = append(kept, row)
		}
	}
	return kept
}

func holdsOfKind(holds []StockHoldView, kind string) []StockHoldView {
	kept := make([]StockHoldView, 0, len(holds))
	for _, hold := range holds {
		carton := hold.Kind == "FREEZE" && hold.Carton != ""
		switch kind {
		case "CARTON":
			if carton {
				kept = append(kept, hold)
			}
		case "FREEZE":
			if hold.Kind == "FREEZE" && !carton {
				kept = append(kept, hold)
			}
		default:
			if hold.Kind == kind {
				kept = append(kept, hold)
			}
		}
	}
	return kept
}

func lockedOnCell(locks []ports.StockLockRow, row ports.StockCellBalance) int64 {
	alias := textOf(row.Alias)
	var total int64
	for _, lock := range locks {
		if lock.Warehouse == row.Warehouse && lock.Location == row.Location && lock.Goods == row.Goods && lock.Unit == row.Unit && lock.State == row.State && lock.InboundBatch == row.InboundBatch && lock.Alias == alias {
			total += lock.Quantity
		}
	}
	return total
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

// stockWords 是一次检索。needle 已去掉首尾空白并转成小写；names 的键是商品编码，值是当前修订的名称与各识别码，同样小写。
// 登记册没接上时 names 是空的：编码、库位、箱号与贴标别名仍认，名称和条码对不上。
type stockWords struct {
	needle string
	names  map[string]string
}

func (handler *QueryStockHandler) narrowCells(ctx context.Context, query StockQuery, rows []ports.StockCellBalance) ([]ports.StockCellBalance, error) {
	words, err := handler.stockWords(ctx, query, cellGoods(rows))
	if err != nil || words.needle == "" {
		return rows, err
	}
	kept := make([]ports.StockCellBalance, 0, len(rows))
	for _, row := range rows {
		if words.matches(row.Goods, row.Warehouse, row.Location, textOf(row.Alias)) {
			kept = append(kept, row)
		}
	}
	return kept, nil
}

func (handler *QueryStockHandler) narrowCartons(ctx context.Context, query StockQuery, rows []ports.StockCartonBalance) ([]ports.StockCartonBalance, error) {
	if strings.TrimSpace(query.Keyword) == "" {
		return rows, nil
	}
	bills := make([][]domain.CartonContentLine, len(rows))
	codes := make([]string, 0)
	for index, row := range rows {
		warehouse, err := domain.NewWarehouseCode(row.Warehouse)
		if err != nil {
			return nil, fmt.Errorf("query stock: %w", err)
		}
		carton, err := domain.NewCartonNumber(row.Carton)
		if err != nil {
			return nil, fmt.Errorf("query stock: %w", err)
		}
		bill, err := handler.reads.CartonBill(ctx, query.Tenant, query.Owner, warehouse, carton)
		if err != nil {
			return nil, fmt.Errorf("query stock: %w", err)
		}
		bills[index] = bill
		for _, line := range bill {
			codes = append(codes, line.Goods().String())
		}
	}
	words, err := handler.stockWords(ctx, query, codes)
	if err != nil {
		return nil, err
	}
	kept := make([]ports.StockCartonBalance, 0, len(rows))
	for index, row := range rows {
		if words.matches("", row.Warehouse, row.Location, row.Carton) {
			kept = append(kept, row)
			continue
		}
		for _, line := range bills[index] {
			if words.matches(line.Goods().String()) {
				kept = append(kept, row)
				break
			}
		}
	}
	return kept, nil
}

func (handler *QueryStockHandler) narrowHolds(ctx context.Context, query StockQuery, holds []StockHoldView) ([]StockHoldView, error) {
	codes := make([]string, 0, len(holds))
	for _, hold := range holds {
		if hold.Goods != "" {
			codes = append(codes, hold.Goods)
		}
	}
	words, err := handler.stockWords(ctx, query, codes)
	if err != nil || words.needle == "" {
		return holds, err
	}
	kept := make([]StockHoldView, 0, len(holds))
	for _, hold := range holds {
		if words.matches(hold.Goods, hold.Warehouse, hold.Location, hold.Carton) {
			kept = append(kept, hold)
		}
	}
	return kept, nil
}

func (handler *QueryStockHandler) stockWords(ctx context.Context, query StockQuery, codes []string) (stockWords, error) {
	needle := strings.ToLower(strings.TrimSpace(query.Keyword))
	words := stockWords{needle: needle, names: map[string]string{}}
	if needle == "" || handler.goods == nil {
		return words, nil
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		goods, err := domain.NewGoodsCode(code)
		if err != nil {
			continue
		}
		revision, found, err := handler.goods.LoadLatestGoods(ctx, query.Tenant, query.Owner, goods)
		if err != nil {
			return stockWords{}, fmt.Errorf("query stock: %w", err)
		}
		if !found {
			continue
		}
		var text strings.Builder
		text.WriteString(strings.ToLower(revision.Content().Name().String()))
		for _, alias := range revision.Content().Aliases() {
			text.WriteByte(' ')
			text.WriteString(strings.ToLower(alias.Code().String()))
		}
		words.names[code] = text.String()
	}
	return words, nil
}

func cellGoods(rows []ports.StockCellBalance) []string {
	codes := make([]string, 0, len(rows))
	for _, row := range rows {
		codes = append(codes, row.Goods)
	}
	return codes
}

func (words stockWords) matches(goods string, fields ...string) bool {
	if words.needle == "" {
		return true
	}
	if goods != "" && strings.Contains(strings.ToLower(goods), words.needle) {
		return true
	}
	if name := words.names[goods]; name != "" && strings.Contains(name, words.needle) {
		return true
	}
	for _, field := range fields {
		if field != "" && strings.Contains(strings.ToLower(field), words.needle) {
			return true
		}
	}
	return false
}
