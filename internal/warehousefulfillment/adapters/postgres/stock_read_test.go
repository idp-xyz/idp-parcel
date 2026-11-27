package postgres_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: AT-WF-136 的格、汇总、占用与流水，AT-WF-137 货主不带库位与原因，AT-WF-138 没登时区时库龄未配置。
func TestStockReadAnswersTheLedgerWithoutMovingIt(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-READ", "SYN-GOODS-01", "件", 10)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-READ-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 10,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	if world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-READ-MOVE", 10, "SYN-LOC-RCV", "SYN-LOC-STORE")).Outcome() != application.RegisterInternalJobRegistered {
		t.Fatal("上架没有入账")
	}
	frozen := world.freeze(t, world.freezer(t), freezeGoods(t, "DEC-READ", "SYN-LOC-STORE", world.onlyBatch(t, "SYN-LOC-STORE"), 4))
	if frozen.Outcome() != application.FreezeFrozen {
		t.Fatalf("冻结 %s", frozen.Outcome())
	}
	reader := world.stockReader(t)
	cells := world.readStock(t, reader, application.StockViewCells, false)
	if cells.Book != nil {
		t.Fatal("不带页码应答整份")
	}
	first := world.readStockBook(t, reader, application.StockViewCells, 1, 1)
	if first.Book == nil || first.Book.Total != len(cells.Cells) || len(first.Cells) != 1 {
		t.Fatalf("库存格第一页 %+v 共 %d", first.Book, len(first.Cells))
	}
	past := world.readStockBook(t, reader, application.StockViewCells, first.Book.Total+1, 1)
	if past.Book == nil || past.Book.Total != len(cells.Cells) || len(past.Cells) != 0 {
		t.Fatalf("库存格越过末页 %+v 行 %d", past.Book, len(past.Cells))
	}
	store := cellNamed(t, cells.Cells, "SYN-LOC-STORE")
	if store.Quantity != 10 || store.Frozen != 4 || store.Available != 6 || !store.Allocatable || store.Location == "" || !store.AgeConfigured || store.AgeDays != 0 {
		t.Fatalf("存储格 %+v", store)
	}
	customer := world.readStock(t, reader, application.StockViewCells, true)
	hidden := cellNamed(t, customer.Cells, "SYN-LOC-STORE")
	if hidden.Location != "" || hidden.Quantity != 10 || hidden.Available != 6 {
		t.Fatalf("货主格 %+v", hidden)
	}
	holds := world.readStock(t, reader, application.StockViewHolds, false)
	if len(holds.Holds) != 1 || holds.Holds[0].Reason == "" || holds.Holds[0].Quantity != 4 {
		t.Fatalf("占用 %+v", holds.Holds)
	}
	quiet := world.readStock(t, reader, application.StockViewHolds, true)
	if len(quiet.Holds) != 1 || quiet.Holds[0].Reason != "" || quiet.Holds[0].Operator != "" || quiet.Holds[0].Location != "" {
		t.Fatalf("货主占用 %+v", quiet.Holds)
	}
	summary := world.readStock(t, reader, application.StockViewSummary, false)
	if len(summary.Summary) != 1 || summary.Summary[0].OnHand != 10 || summary.Summary[0].Allocatable != 10 || summary.Summary[0].Available != 6 {
		t.Fatalf("汇总 %+v", summary.Summary)
	}
	movements := world.readStock(t, reader, application.StockViewMovements, true)
	if !movementHas(movements.Movements, "RECEIPT") || movements.Movements[0].Source != "" || movements.Movements[0].Lines[0].Location != "" {
		t.Fatalf("货主流水 %+v", movements.Movements)
	}
	other := world.readStockOwner(t, reader, "SYN-OWNER-02")
	if len(other.Cells) != 0 {
		t.Fatalf("别的货主 %+v", other.Cells)
	}

	cartonAdvice := world.adviseCartons(t, "REQ-READ-CTN", "SYN-CARTON-R1")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-READ-CTN", cartonAdvice.ID().String(), mustCartonLine(t, "SYN-CARTON-R1", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("整箱没有入账")
	}
	cartons := world.readStock(t, reader, application.StockViewCartons, false)
	if len(cartons.Cartons) != 1 || cartons.Cartons[0].Carton != "SYN-CARTON-R1" || cartons.Cartons[0].Location != "SYN-LOC-RCV" || cartons.Cartons[0].Held || len(cartons.Cartons[0].Bill) != 1 || cartons.Cartons[0].Bill[0].Quantity() != 5 {
		t.Fatalf("整箱 %+v", cartons.Cartons)
	}
	plain := world.readStock(t, reader, application.StockViewCartons, true)
	if plain.Cartons[0].Location != "" || plain.Cartons[0].Carton != "SYN-CARTON-R1" {
		t.Fatalf("货主箱 %+v", plain.Cartons[0])
	}
}

func (world receiptWorld) stockReader(t *testing.T) *application.QueryStockHandler {
	t.Helper()
	reads, err := postgres.NewStockReads(world.db)
	if err != nil {
		t.Fatal(err)
	}
	return application.NewQueryStockHandler(reads, world.warehouses, world.warehouses, world.locations, world.zones, nil, nil)
}

func (world receiptWorld) readStockBook(t *testing.T, reader *application.QueryStockHandler, view application.StockView, page, size int) application.StockSnapshot {
	t.Helper()
	query := application.StockQuery{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), View: view, Now: world.clock.Now(),
		Number: page, Limit: size,
	}
	return world.queryStock(t, reader, query)
}

func (world receiptWorld) readStock(t *testing.T, reader *application.QueryStockHandler, view application.StockView, customer bool) application.StockSnapshot {
	t.Helper()
	return world.readStockOwner(t, reader, "SYN-OWNER-01", view, customer)
}

func (world receiptWorld) readStockOwner(t *testing.T, reader *application.QueryStockHandler, owner string, extra ...any) application.StockSnapshot {
	t.Helper()
	view := application.StockViewCells
	customer := false
	if len(extra) > 0 {
		view = extra[0].(application.StockView)
	}
	if len(extra) > 1 {
		customer = extra[1].(bool)
	}
	query := application.StockQuery{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, owner),
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), View: view, Customer: customer, Now: world.clock.Now(),
	}
	if view == application.StockViewMovements {
		query.Page, query.Limit = movementPage(t, owner, ""), 200
	}
	return world.queryStock(t, reader, query)
}

func (world receiptWorld) queryStock(t *testing.T, reader *application.QueryStockHandler, query application.StockQuery) application.StockSnapshot {
	t.Helper()
	var snapshot application.StockSnapshot
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var readErr error
		snapshot, readErr = reader.Query(ctx, query)
		return readErr
	})
	if err != nil {
		t.Fatalf("读库存：%v", err)
	}
	return snapshot
}

// movementPage 照读口的做法解出流水的翻页：货主、仓与看法进游标摘要，after 为空即第一页。
func movementPage(t *testing.T, owner, after string) cataloguepage.Query {
	t.Helper()
	values := url.Values{"ownerAccount": {owner}, "warehouseCode": {"SYN-WH-01"}, "view": {"movements"}}
	if after != "" {
		values.Set("after", after)
	}
	page, err := ports.StockMovementPages.Decode(values)
	if err != nil {
		t.Fatalf("解流水翻页：%v", err)
	}
	return page
}

// 流水越积越多，读口按入账时刻从新到旧一页一页答：各页不重不漏，末页没有下一页，总数是同一组条件下的全部。
func TestStockMovementsTurnPagesNewestFirst(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-PAGE", "SYN-GOODS-01", "件", 10)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-PAGE-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 10,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	for _, move := range []struct {
		fact     string
		quantity int64
	}{{"SYN-FACT-PAGE-MOVE-1", 4}, {"SYN-FACT-PAGE-MOVE-2", 3}} {
		// 三笔的入账时刻要各不相同：同一时刻入账的话，从新到旧与从旧到新只差在流水标识上，看不出读口按哪头排。
		world.clock.at = world.clock.at.Add(time.Hour)
		if world.registerJob(t, world.internalJobs(t), moveCommand(t, move.fact, move.quantity, "SYN-LOC-RCV", "SYN-LOC-STORE")).Outcome() != application.RegisterInternalJobRegistered {
			t.Fatalf("上架 %s 没有入账", move.fact)
		}
	}
	reader := world.stockReader(t)
	read := func(after string) application.StockSnapshot {
		return world.queryStock(t, reader, application.StockQuery{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), View: application.StockViewMovements, Now: world.clock.Now(),
			Page: movementPage(t, "SYN-OWNER-01", after), Limit: 2,
		})
	}

	first := read("")
	if len(first.Movements) != 2 || first.MovementPage == nil || first.MovementPage.Next == nil ||
		first.MovementPage.Total == nil || *first.MovementPage.Total != 3 || first.MovementPage.Size != 2 {
		t.Fatalf("第一页 %+v page %+v", first.Movements, first.MovementPage)
	}
	second := read(*first.MovementPage.Next)
	if len(second.Movements) != 1 || second.MovementPage == nil || second.MovementPage.Next != nil || *second.MovementPage.Total != 3 {
		t.Fatalf("第二页 %+v page %+v", second.Movements, second.MovementPage)
	}
	listed := append(append([]application.StockMovementView(nil), first.Movements...), second.Movements...)
	seen := map[string]bool{}
	for index, movement := range listed {
		if seen[movement.Source] {
			t.Fatalf("%s 在两页里都出现了", movement.Source)
		}
		seen[movement.Source] = true
		if len(movement.Lines) == 0 {
			t.Fatalf("%s 没带行", movement.Source)
		}
		if index > 0 && !movement.Posted.Before(listed[index-1].Posted) {
			t.Fatalf("%s 不比它前面的一笔入账更早，不是从新到旧", movement.Source)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("三笔流水只翻出 %d 笔：%+v", len(seen), listed)
	}
	// 最后入账的那次上架在第一页头一行，最早的收货在末页。
	if !strings.HasPrefix(listed[0].Source, "SYN-FACT-PAGE-MOVE-2") || listed[2].Reason != "RECEIPT" {
		t.Fatalf("次序 %s、%s、%s", listed[0].Source, listed[1].Source, listed[2].Source)
	}

	// 货主面不翻页，读口替它指定旧的在前、只取第一页：头一行是最早的收货。
	oldest, err := ports.StockMovementPages.Decode(url.Values{
		"ownerAccount": {"SYN-OWNER-01"}, "warehouseCode": {"SYN-WH-01"}, "view": {"movements"}, "sort": {"postedAt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	customer := world.queryStock(t, reader, application.StockQuery{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), View: application.StockViewMovements, Customer: true, Now: world.clock.Now(),
		Page: oldest, Limit: 2,
	})
	if len(customer.Movements) != 2 || customer.Movements[0].Reason != "RECEIPT" || customer.Movements[0].Source != "" || !customer.Movements[0].Posted.IsZero() {
		t.Fatalf("货主流水 %+v", customer.Movements)
	}

	// 别的货主名下没有流水：一行也没有仍是一页，总数为零。
	none := world.queryStock(t, reader, application.StockQuery{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02"),
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), View: application.StockViewMovements, Now: world.clock.Now(),
		Page: movementPage(t, "SYN-OWNER-02", ""), Limit: 2,
	})
	if len(none.Movements) != 0 || none.MovementPage == nil || none.MovementPage.Next != nil || *none.MovementPage.Total != 0 {
		t.Fatalf("别的货主 %+v page %+v", none.Movements, none.MovementPage)
	}
}

func cellNamed(t *testing.T, cells []application.StockCellView, location string) application.StockCellView {
	t.Helper()
	for _, cell := range cells {
		if cell.Location == location || (location == "SYN-LOC-STORE" && cell.Quantity == 10 && cell.Location == "") {
			if location == "SYN-LOC-STORE" && cell.Goods == "SYN-GOODS-01" {
				return cell
			}
		}
	}
	t.Fatalf("没有 %s：%+v", location, cells)
	return application.StockCellView{}
}

func movementHas(movements []application.StockMovementView, reason string) bool {
	for _, movement := range movements {
		if movement.Reason == reason {
			return true
		}
	}
	return false
}
