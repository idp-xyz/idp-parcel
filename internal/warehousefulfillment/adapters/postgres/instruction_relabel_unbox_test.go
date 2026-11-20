package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: AT-WF-118、AT-WF-119 的没贴贴上，以及 AT-WF-121 实点与箱单相符的开箱。
func TestRelabelAndUnboxPostWholeFacts(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 2, "REQ-ALIAS", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = append(spec.Aliases, domain.NewGoodsAlias(
			valueOf(t, domain.NewGoodsAliasCode, "SYN-PLAT-REL"), domain.GoodsAliasPlatformCode,
			valueOf(t, domain.NewPlatformName, "SYN-PLATFORM"),
			domain.NewAliasValidity(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{}),
		))
	}))
	advice := world.adviseGoods(t, "REQ-REL", "SYN-GOODS-01", "件", 10)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-REL-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 10,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	handler := world.instructions(t)
	accepted := world.acceptInstruction(t, handler, relabelCommand(t))
	instruction, ok := accepted.Instruction()
	if accepted.Outcome() != application.InstructionAccepted || !ok {
		t.Fatalf("换标指令 %s", accepted.Outcome())
	}
	posted := world.executeInstruction(t, handler, application.ExecuteInstructionCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewInternalJobFactID, "SYN-FACT-REL"),
		InstructionID: instruction.ID(), Device: valueOf(t, domain.NewReceiptDeviceID, "SYN-DEVICE-01"),
		Operator: valueOf(t, domain.NewReceiptOperatorID, "operator-1"), OccurredAt: preAdviceNumberingInstant, DeviceSequence: 1,
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), DoneQuantity: 10,
		Lines: []application.InstructionExecutionLine{{
			Direction: domain.StockOut, Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"),
			Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"),
			State: domain.StockStateGood, Quantity: 10,
		}},
	})
	if posted.Outcome() != application.InstructionPosted || world.aliasQty(t, "SYN-LOC-RCV", "SYN-PLAT-REL") != 10 || world.batchCount(t, "SYN-GOODS-01") != 1 {
		t.Fatalf("换标 %s 贴标 %d 批次 %d", posted.Outcome(), world.aliasQty(t, "SYN-LOC-RCV", "SYN-PLAT-REL"), world.batchCount(t, "SYN-GOODS-01"))
	}

	cartonAdvice := world.adviseCartons(t, "REQ-UNBOX", "SYN-CARTON-U1")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-UNBOX-RCV", cartonAdvice.ID().String(), mustCartonLine(t, "SYN-CARTON-U1", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("整箱收货没有入账")
	}
	unboxed := world.unbox(t, handler, unboxCommand(t, "SYN-FACT-UNBOX", "SYN-CARTON-U1", 5))
	if unboxed.Outcome() != application.InstructionPosted || world.cartonBalance(t, "SYN-CARTON-U1") != 0 || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 5 {
		t.Fatalf("开箱 %s 箱余额 %d 件 %d", unboxed.Outcome(), world.cartonBalance(t, "SYN-CARTON-U1"), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	missing := world.unbox(t, handler, unboxCommand(t, "SYN-FACT-UNBOX-MISS", "SYN-CARTON-NONE", 1))
	if missing.Outcome() != application.InstructionCartonNotFound {
		t.Fatalf("查无此箱 %s", missing.Outcome())
	}
}

func relabelCommand(t *testing.T) application.AcceptInstructionCommand {
	t.Helper()
	return application.AcceptInstructionCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Kind: domain.InstructionRelabel,
		Origin: domain.InstructionOriginCustomer, Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
		Unit: valueOf(t, domain.NewGoodsUnitName, "件"), Quantity: 10,
		ToAlias:    valueOf(t, domain.NewGoodsAliasCode, "SYN-PLAT-REL"),
		RequestKey: valueOf(t, domain.NewRequestKey, "REQ-RELABEL"),
	}
}

func unboxCommand(t *testing.T, fact, carton string, quantity int64) application.UnboxCommand {
	t.Helper()
	return application.UnboxCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewInternalJobFactID, fact),
		Device: valueOf(t, domain.NewReceiptDeviceID, "SYN-DEVICE-01"), Operator: valueOf(t, domain.NewReceiptOperatorID, "operator-1"),
		OccurredAt: preAdviceNumberingInstant, DeviceSequence: 1, Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Carton: valueOf(t, domain.NewCartonNumber, carton),
		Lines: []ports.UnboxLine{{
			Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"),
			State: domain.StockStateGood, Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE"), Quantity: quantity,
		}},
	}
}

func (world receiptWorld) unbox(t *testing.T, handler *application.InstructionHandler, command application.UnboxCommand) application.InstructionResult {
	t.Helper()
	var result application.InstructionResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var unboxErr error
		result, unboxErr = handler.Unbox(ctx, command)
		return unboxErr
	})
	if err != nil {
		t.Fatalf("开箱：%v", err)
	}
	return result
}

func (world receiptWorld) aliasQty(t *testing.T, location, alias string) int64 {
	t.Helper()
	var quantity int64
	err := world.pool.QueryRow(t.Context(),
		`SELECT coalesce(sum(quantity_delta), 0)
		   FROM warehouse_fulfillment.stock_movement_line
		  WHERE tenant_id = $1 AND location_code = $2 AND label_alias = $3`,
		"SYN-TENANT-01", location, alias).Scan(&quantity)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}
