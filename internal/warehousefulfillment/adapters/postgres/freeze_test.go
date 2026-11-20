package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// Covers: AT-WF-129、AT-WF-130 的暂存与余额不够、AT-WF-133 的存储箱与暂存箱、AT-WF-134。
func TestFreezeDoesNotMoveStockAndUnfreezeRestoresAvailability(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-FRZ", "SYN-GOODS-01", "件", 10)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-FRZ-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 10,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	handler := world.freezer(t)
	staging := world.freeze(t, handler, freezeGoods(t, "DEC-FRZ-RCV", "SYN-LOC-RCV", world.onlyBatch(t, "SYN-LOC-RCV"), 4))
	if staging.Outcome() != application.FreezeNotValid || staging.Refusal() != application.FreezeNotAllocatable {
		t.Fatalf("暂存冻结 %s %s", staging.Outcome(), staging.Refusal())
	}
	moved := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-FRZ-MOVE", 10, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 {
		t.Fatalf("上架 %s 存储 %d", moved.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	movements := world.movementCount(t)
	frozen := world.freeze(t, handler, freezeGoods(t, "DEC-FRZ-1", "SYN-LOC-STORE", world.onlyBatch(t, "SYN-LOC-STORE"), 4))
	recorded, ok := frozen.Freeze()
	if frozen.Outcome() != application.FreezeFrozen || !ok || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 || world.movementCount(t) != movements {
		t.Fatalf("冻结 %s 余额 %d 流水 %d/%d", frozen.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.movementCount(t), movements)
	}
	again := world.freeze(t, handler, freezeGoods(t, "DEC-FRZ-1", "SYN-LOC-STORE", world.onlyBatch(t, "SYN-LOC-STORE"), 4))
	if again.Outcome() != application.FreezeExisting {
		t.Fatalf("重投 %s", again.Outcome())
	}
	conflict := world.freeze(t, handler, freezeGoods(t, "DEC-FRZ-1", "SYN-LOC-STORE", world.onlyBatch(t, "SYN-LOC-STORE"), 5))
	if conflict.Outcome() != application.FreezeConflict {
		t.Fatalf("换内容 %s", conflict.Outcome())
	}
	short := world.freeze(t, handler, freezeGoods(t, "DEC-FRZ-SHORT", "SYN-LOC-STORE", world.onlyBatch(t, "SYN-LOC-STORE"), 8))
	if short.Outcome() != application.FreezeNotValid || short.Refusal() != application.FreezeHoldConflict {
		t.Fatalf("再冻 8 %s %s", short.Outcome(), short.Refusal())
	}
	unfrozen := world.unfreeze(t, handler, recorded.ID(), "可以动了")
	opened, ok := unfrozen.Freeze()
	if unfrozen.Outcome() != application.FreezeUnfrozen || !ok || opened.Status() != domain.FreezeUnfrozen || opened.Note() != "可以动了" {
		t.Fatalf("解冻 %s", unfrozen.Outcome())
	}
	second := world.unfreeze(t, handler, recorded.ID(), "再说一次")
	if second.Outcome() != application.FreezeExisting {
		t.Fatalf("再解冻 %s", second.Outcome())
	}

	cartonAdvice := world.adviseCartons(t, "REQ-FRZ-CTN", "SYN-CARTON-F1")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-FRZ-CTN", cartonAdvice.ID().String(), mustCartonLine(t, "SYN-CARTON-F1", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("整箱没有入账")
	}
	heldCarton := world.freeze(t, handler, freezeCarton(t, "DEC-FRZ-CTN-RCV", "SYN-CARTON-F1"))
	if heldCarton.Outcome() != application.FreezeNotValid || heldCarton.Refusal() != application.FreezeNotAllocatable {
		t.Fatalf("暂存箱 %s %s", heldCarton.Outcome(), heldCarton.Refusal())
	}
}

func TestFreezesRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	store, err := postgres.NewFreezes(world.db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	key := valueOf(t, domain.NewRequestKey, "DEC-FRZ")
	id := valueOf(t, domain.NewFreezeID, "FRZ-1")
	if err := store.HoldDecision(ctx, tenant, key); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldDecision：%v", err)
	}
	if err := store.HoldFreeze(ctx, tenant, id); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldFreeze：%v", err)
	}
	if err := store.Save(ctx, domain.Freeze{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Save：%v", err)
	}
	if err := store.MarkUnfrozen(ctx, domain.Freeze{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkUnfrozen：%v", err)
	}
}

func (world receiptWorld) freezer(t *testing.T) *application.FreezeHandler {
	t.Helper()
	store, err := postgres.NewFreezes(world.db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := identity.NewFreezeIdentities()
	if err != nil {
		t.Fatal(err)
	}
	return application.NewFreezeHandler(
		store, identities, world.ledger, world.cartons, world.warehouses, world.locations, world.zones, world.goods,
		postgres.NewFreezeHolds(store), world.clock, world.warehouses,
	)
}

func (world receiptWorld) freeze(t *testing.T, handler *application.FreezeHandler, command application.FreezeCommand) application.FreezeResult {
	t.Helper()
	var result application.FreezeResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var freezeErr error
		result, freezeErr = handler.Freeze(ctx, command)
		return freezeErr
	})
	if err != nil {
		t.Fatalf("冻结：%v", err)
	}
	return result
}

func (world receiptWorld) unfreeze(t *testing.T, handler *application.FreezeHandler, id domain.FreezeID, note string) application.FreezeResult {
	t.Helper()
	var result application.FreezeResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var unfreezeErr error
		result, unfreezeErr = handler.Unfreeze(ctx, application.UnfreezeCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: id,
			Operator: valueOf(t, domain.NewRegistrant, "operator-1"), Note: note,
		})
		return unfreezeErr
	})
	if err != nil {
		t.Fatalf("解冻：%v", err)
	}
	return result
}

func freezeGoods(t *testing.T, decision, location, batch string, quantity int64) application.FreezeCommand {
	t.Helper()
	cell, err := domain.NewStockCell(domain.StockCellSpec{
		Location: valueOf(t, domain.NewLocationCode, location), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
		Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood,
		InboundBatch: valueOf(t, domain.NewInboundBatchID, batch),
	})
	if err != nil {
		t.Fatal(err)
	}
	return application.FreezeCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), DecisionID: valueOf(t, domain.NewRequestKey, decision),
		Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Target: domain.FreezeGoods, Cell: cell, Quantity: quantity, Reason: "货主要求先别动",
		Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
}

func freezeCarton(t *testing.T, decision, carton string) application.FreezeCommand {
	t.Helper()
	return application.FreezeCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), DecisionID: valueOf(t, domain.NewRequestKey, decision),
		Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Target: domain.FreezeCarton, Carton: valueOf(t, domain.NewCartonNumber, carton), Reason: "这箱先留着",
		Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
}

func (world receiptWorld) onlyBatch(t *testing.T, location string) string {
	t.Helper()
	var id string
	err := world.pool.QueryRow(t.Context(),
		`SELECT inbound_batch_id
		   FROM warehouse_fulfillment.stock_movement_line
		  WHERE tenant_id = $1 AND location_code = $2
		  GROUP BY inbound_batch_id
		 HAVING sum(quantity_delta) > 0`,
		"SYN-TENANT-01", location).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
