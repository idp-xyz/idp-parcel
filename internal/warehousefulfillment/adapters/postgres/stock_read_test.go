package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
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
	return application.NewQueryStockHandler(reads, world.warehouses, world.warehouses, world.locations, world.zones)
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
	var snapshot application.StockSnapshot
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var readErr error
		snapshot, readErr = reader.Query(ctx, application.StockQuery{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, owner),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), View: view, Customer: customer, Now: world.clock.Now(),
		})
		return readErr
	})
	if err != nil {
		t.Fatalf("读库存：%v", err)
	}
	return snapshot
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
