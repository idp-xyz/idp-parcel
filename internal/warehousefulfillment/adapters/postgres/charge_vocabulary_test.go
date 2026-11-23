package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func TestChargeUnitsFollowTheRegisteredWordAndDoNotConvert(t *testing.T) {
	world := newMeasurementWorld(t)
	world.seedReceiving(t)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	registered := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	world.charges = world.chargeHandler(t, world.clock.Now()).WithReadings(world.goods, world.measurements)
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return world.chargeHandler(t, registered).RegisterUnits(ctx, tenant, warehouse, "LB", "FT3")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("单位没有登记")
	}
	measured := world.record(t, registered, goodsLevelMeasureCommand(
		t, "SYN-MEASURE-1", "SYN-DEVICE-01", registered, 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件",
		"10", "20", "30", "CM", "2", "LB",
	))
	if measured.Outcome() != application.RecordMeasurementRecorded {
		t.Fatalf("仓测 %s", measured.Outcome())
	}
	advice := world.adviseGoods(t, "REQ-UNIT-1", "SYN-GOODS-01", "件", 4)
	confirmed := world.confirm(t, receiptCommand(t, "SYN-FACT-UNIT-1", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 4,
	})))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.stockOf(t, "SYN-GOODS-01") != 4 {
		t.Fatalf("收货 %s 库存 %d", confirmed.Outcome(), world.stockOf(t, "SYN-GOODS-01"))
	}
	first := world.headCharge(t, "INBOUND_HANDLING", "SYN-FACT-UNIT-1")
	weight := mustMeasure(t, first.Lines[0].Measures, "LB")
	if weight.Class != "WEIGHT" || weight.Quantity != "8" || weight.Unmeasured || weight.Original != "" {
		t.Fatalf("重量 %+v", weight)
	}
	volume := mustMeasure(t, first.Lines[0].Measures, "FT3")
	if volume.Class != "VOLUME" || !volume.Unmeasured || volume.Quantity != "" || volume.Original != "24000" || volume.OriginalUnit != "CM3" {
		t.Fatalf("体积 %+v", volume)
	}
	if len(first.Measures) != 2 || mustMeasure(t, first.Measures, "SHIPMENT").Quantity != "1" || mustMeasure(t, first.Measures, "PIECE").Quantity != "4" {
		t.Fatalf("发生项计量 %+v", first.Measures)
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return world.charges.RegisterUnits(ctx, tenant, warehouse, "KG", "M3")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("新一版单位没有追加")
	}
	secondAdvice := world.adviseGoods(t, "REQ-UNIT-2", "SYN-GOODS-01", "件", 1)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-UNIT-2", secondAdvice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 1,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("第二笔收货没有成立")
	}
	kept := world.headCharge(t, "INBOUND_HANDLING", "SYN-FACT-UNIT-1")
	if mustMeasure(t, kept.Lines[0].Measures, "LB").Quantity != "8" || kept.WeightUnit != "LB" || kept.VolumeUnit != "FT3" {
		t.Fatalf("更早的发生项改了单位 %+v", kept.Lines[0].Measures)
	}
	later := world.headCharge(t, "INBOUND_HANDLING", "SYN-FACT-UNIT-2")
	laterWeight := mustMeasure(t, later.Lines[0].Measures, "KG")
	if !laterWeight.Unmeasured || laterWeight.Original != "2" || laterWeight.OriginalUnit != "LB" || laterWeight.Declared != "0.5" || laterWeight.DeclaredUnit != "KG" {
		t.Fatalf("后一版重量 %+v", laterWeight)
	}
	if later.WeightUnit != "KG" || later.VolumeUnit != "M3" {
		t.Fatalf("后一版单位 %s %s", later.WeightUnit, later.VolumeUnit)
	}
	body := world.chargePayload(t, first.ID)
	if len(body) != 3 || body["tenantId"] == nil || body["occurrenceId"] == nil || body["version"] == nil {
		t.Fatalf("信封 %+v", body)
	}
	if world.chargeAmountColumns(t) != 0 || world.chargeBodyMentionsAmount(t) {
		t.Fatal("发生项带了金额")
	}
}

func TestStorageRentKeepsAgeAndRestartsLocationDaysAfterAnEmptyDay(t *testing.T) {
	world := newMeasurementWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-A", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-B", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	registered := time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC)
	charges := world.chargeHandler(t, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)).WithReadings(world.goods, world.measurements)
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return world.chargeHandler(t, registered).RegisterUnits(ctx, tenant, warehouse, "LB", "FT3")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("单位没有登记")
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return world.chargeHandler(t, registered).RegisterAging(ctx, tenant, warehouse, "ACTUAL_BATCH")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("库龄没有登记")
	}
	if world.record(t, registered, goodsLevelMeasureCommand(
		t, "SYN-MEASURE-STORE", "SYN-DEVICE-01", registered, 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件",
		"2", "2", "2", "CM", "1", "LB",
	)).Outcome() != application.RecordMeasurementRecorded {
		t.Fatal("仓测没有记下")
	}
	day1 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	receipt := world.postAt(t, day1, domain.StockReasonReceipt, "SYN-FACT-STORE-IN", domain.NewStockLineRequest(
		domain.StockIn, 4, stockCell(t, "SYN-GOODS-01", func(spec *domain.StockCellSpec) {
			spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV")
		}), time.Time{},
	))
	batch := receipt.Lines()[0].Cell().InboundBatch()
	world.postAt(t, day1.Add(time.Hour), domain.StockReasonInternalMove, "SYN-FACT-STORE-A",
		stockMoveLine(t, domain.StockOut, 4, "SYN-LOC-RCV", batch),
		stockMoveLine(t, domain.StockIn, 4, "SYN-LOC-A", batch),
	)
	world.postAt(t, time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC), domain.StockReasonInternalMove, "SYN-FACT-STORE-B",
		stockMoveLine(t, domain.StockOut, 4, "SYN-LOC-A", batch),
		stockMoveLine(t, domain.StockIn, 4, "SYN-LOC-B", batch),
	)
	world.postAt(t, time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC), domain.StockReasonInternalMove, "SYN-FACT-STORE-BACK",
		stockMoveLine(t, domain.StockOut, 4, "SYN-LOC-B", batch),
		stockMoveLine(t, domain.StockIn, 4, "SYN-LOC-A", batch),
	)
	if world.inChargeHandler(t, charges, func(ctx context.Context) (application.ChargeResult, error) {
		return charges.CloseStorageDay(ctx, tenant, owner, warehouse, time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC))
	}).Outcome() != application.ChargeNotConfigured {
		t.Fatal("登记之前的日子被补上了")
	}
	first := world.inChargeHandler(t, charges, func(ctx context.Context) (application.ChargeResult, error) {
		return charges.CloseStorageDay(ctx, tenant, owner, warehouse, time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))
	})
	if first.Outcome() != application.ChargeFormed {
		t.Fatalf("第一天 %s", first.Outcome())
	}
	if world.inChargeHandler(t, charges, func(ctx context.Context) (application.ChargeResult, error) {
		return charges.CloseStorageDay(ctx, tenant, owner, warehouse, time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))
	}).Outcome() != application.ChargeExisting {
		t.Fatal("同一计费日又形成了一份")
	}
	second := world.inChargeHandler(t, charges, func(ctx context.Context) (application.ChargeResult, error) {
		return charges.CloseStorageDay(ctx, tenant, owner, warehouse, time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC))
	})
	third := world.inChargeHandler(t, charges, func(ctx context.Context) (application.ChargeResult, error) {
		return charges.CloseStorageDay(ctx, tenant, owner, warehouse, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))
	})
	if second.Outcome() != application.ChargeFormed || third.Outcome() != application.ChargeFormed {
		t.Fatalf("后两天 %s %s", second.Outcome(), third.Outcome())
	}
	assertStorageDay(t, world, first.Occurrence(), 1, "SYN-LOC-A", 1)
	assertStorageDay(t, world, second.Occurrence(), 2, "SYN-LOC-B", 1)
	assertStorageDay(t, world, third.Occurrence(), 3, "SYN-LOC-A", 1)
}

func TestTheoreticalFifoDoesNotFormStorageRent(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedWarehouse(t, nil)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	registered := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return world.chargeHandler(t, registered).RegisterAging(ctx, tenant, warehouse, "THEORETICAL_FIFO")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("理论先进先出没有登记")
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return world.chargeHandler(t, preAdviceNumberingInstant).CloseStorageDay(ctx, tenant, owner, warehouse, time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))
	}).Outcome() != application.ChargeNotConfigured || world.chargeCountOf(t, "STORAGE") != 0 {
		t.Fatal("理论先进先出形成了仓租")
	}
}

func TestOutboundConfirmationThatDoesNotStandVoidsItsCharge(t *testing.T) {
	world, handler, request := packedOutbound(t, "REQ-VOID-CHARGE", domain.DispatchOwnerLabel, "", 4)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	charges := world.chargeHandler(t, world.clock.Now())
	handler.WithCharges(charges)
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return charges.RegisterUnits(ctx, tenant, valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), "LB", "FT3")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("单位没有登记")
	}
	if world.registerLabel(t, handler, request, 1, "EXT-VOID-CHARGE").Outcome() != application.WaveStrategyRegistered {
		t.Fatal("面单没有登记")
	}
	if world.confirmOutbound(t, handler, request, "SYN-FACT-VOID-CHARGE", application.ConfirmKindOutbound, false).Outcome() != application.WaveConfirmed {
		t.Fatal("确认没有成立")
	}
	formed := world.headCharge(t, "OUTBOUND_HANDLING", "SYN-FACT-VOID-CHARGE")
	if formed.Correction != "ORIGINAL" || len(formed.Lines) != 0 {
		t.Fatalf("出库作业 %+v", formed)
	}
	weight := mustMeasure(t, formed.Measures, "LB")
	if weight.Class != "WEIGHT" || !weight.Unmeasured || weight.Original != "1.2" || weight.OriginalUnit != "KG" {
		t.Fatalf("出库重量 %+v", weight)
	}
	volume := mustMeasure(t, formed.Measures, "FT3")
	if volume.Class != "VOLUME" || !volume.Unmeasured || volume.Original != "3000" || volume.OriginalUnit != "CM3" {
		t.Fatalf("出库体积 %+v", volume)
	}
	counts, err := adapter.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	handler = handler.WithApprovalDuties(counts)
	if world.correctConfirmation(t, handler, "SYN-FACT-VOID-CHARGE", "operator-1", time.Time{}, true).Outcome() != application.WavePending {
		t.Fatal("判不成立没有停下")
	}
	voided, found, err := charges.Read(t.Context(), tenant, formed.ID, 2)
	if err != nil || !found || voided.Correction != "VOID" || len(voided.Measures) != 0 || len(voided.Lines) != 0 || len(voided.Attributes) != 0 {
		t.Fatalf("作废版 %+v %v", voided, err)
	}
	if err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		return counts.SaveDuty(ctx, tenant, ports.ApprovalDuty{
			Revision: 1, Distinct: true, Operator: valueOf(t, domain.NewRegistrant, "operator-1"), RegisteredAt: preAdviceNumberingInstant,
		})
	}); err != nil {
		t.Fatal(err)
	}
	if world.approveReplenishment(t, handler, "SYN-FACT-VOID-CHARGE", "operator-2").Outcome() != application.WavePosted {
		t.Fatal("回补没有入账")
	}
	if world.chargeCountOf(t, "INBOUND_HANDLING") != 0 {
		t.Fatal("回补形成了入库作业")
	}
}

func TestDestroyDispositionFormsNothingAndReturnHandlingFollowsTheMove(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-RET", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageReturnStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-RET", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-RET")
	}))
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.charges = world.chargeHandler(t, world.clock.Now())
	handler := world.returns(t)
	if world.acceptReturn(t, handler, "return-destroy", "TRK-DESTROY").Outcome() != application.ReturnAccepted {
		t.Fatal("销毁那笔预报没有受理")
	}
	if world.receiveReturn(t, handler, "FACT-DESTROY", "TRK-DESTROY", "").Outcome() != application.ReturnCounted {
		t.Fatal("销毁那笔没有清点")
	}
	if world.disposeReturn(t, handler, "DEC-DESTROY", "FACT-DESTROY", domain.ReturnDestinationDestroy).Outcome() != application.ReturnDisposed {
		t.Fatal("销毁去向没有记下")
	}
	if world.chargeCountOf(t, "DESTRUCTION") != 0 || world.chargeCountOf(t, "RETURN_HANDLING") != 0 {
		t.Fatalf("销毁形成了发生项 退货 %d 销毁 %d", world.chargeCountOf(t, "RETURN_HANDLING"), world.chargeCountOf(t, "DESTRUCTION"))
	}
	if world.acceptReturn(t, handler, "return-owner", "TRK-OWNER").Outcome() != application.ReturnAccepted {
		t.Fatal("退还预报没有受理")
	}
	if world.receiveReturn(t, handler, "FACT-OWNER", "TRK-OWNER", "").Outcome() != application.ReturnCounted {
		t.Fatal("退还那笔没有清点")
	}
	if world.disposeReturn(t, handler, "DEC-OWNER", "FACT-OWNER", domain.ReturnDestinationReturnToOwner).Outcome() != application.ReturnDisposed {
		t.Fatal("退还去向没有记下")
	}
	ownerCharge := world.headCharge(t, "RETURN_HANDLING", "DEC-OWNER")
	if len(ownerCharge.Lines) != 0 || attributeValue(ownerCharge.Attributes, "RETURN_DESTINATION") != "RETURN_TO_OWNER" || mustMeasure(t, ownerCharge.Measures, "PIECE").Quantity != "2" {
		t.Fatalf("退还货主 %+v", ownerCharge)
	}
	if world.acceptReturn(t, handler, "return-put", "TRK-PUT").Outcome() != application.ReturnAccepted {
		t.Fatal("上架预报没有受理")
	}
	if world.receiveReturn(t, handler, "FACT-PUT", "TRK-PUT", "").Outcome() != application.ReturnCounted {
		t.Fatal("上架那笔没有清点")
	}
	if world.disposeReturn(t, handler, "DEC-PUT", "FACT-PUT", domain.ReturnDestinationGoodPutaway).Outcome() != application.ReturnDisposed {
		t.Fatal("上架去向没有记下")
	}
	if world.chargeCountOf(t, "RETURN_HANDLING") != 1 {
		t.Fatal("处置作出时就形成了上架发生项")
	}
	moved := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-PUTAWAY", 2, "SYN-LOC-RET", "SYN-LOC-STORE"))
	if moved.Outcome() != application.RegisterInternalJobRegistered {
		t.Fatalf("上架 %s", moved.Outcome())
	}
	putaway := world.headCharge(t, "RETURN_HANDLING", "SYN-FACT-PUTAWAY")
	if len(putaway.Lines) != 0 || attributeValue(putaway.Attributes, "RETURN_DESTINATION") != "GOOD_PUTAWAY" || mustMeasure(t, putaway.Measures, "SHIPMENT").Quantity != "1" {
		t.Fatalf("良品上架 %+v", putaway)
	}
	if world.chargeCountOf(t, "VALUE_ADDED") != 0 || world.chargeCountOf(t, "DESTRUCTION") != 0 {
		t.Fatal("退货暂存的上架另形成了别的发生项")
	}
}

func TestProcessingFormsWithoutUnitsAndCarriesInputAndOutputLines(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-KIT-01", 1, "REQ-KIT-CHARGE", func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "合成套装")
		spec.Aliases = nil
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundAsFinishedGoods
		spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 2)}
	}))
	world.charges = world.chargeHandler(t, world.clock.Now())
	advice := world.adviseGoods(t, "REQ-PART-CHARGE", "SYN-GOODS-01", "件", 4)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-PART-CHARGE", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 4,
	}))).Outcome() != application.ConfirmReceiptConfirmed || world.chargeCount(t) != 0 {
		t.Fatal("没单位时收货不该形成发生项，或者收货没有成立")
	}
	handler := world.instructions(t)
	accepted := world.acceptInstruction(t, handler, processCommand(t, "REQ-PROC-CHARGE", 2))
	instruction, ok := accepted.Instruction()
	if accepted.Outcome() != application.InstructionAccepted || !ok {
		t.Fatalf("受理 %s", accepted.Outcome())
	}
	posted := world.executeInstruction(t, handler, executeProcess(t, "SYN-FACT-PROC-CHARGE", instruction.ID().String(), 2, 4))
	if posted.Outcome() != application.InstructionPosted {
		t.Fatalf("执行 %s %v", posted.Outcome(), posted.Issues())
	}
	version := world.headCharge(t, "PROCESSING", "SYN-FACT-PROC-CHARGE")
	if version.WeightUnit != "" || version.VolumeUnit != "" || attributeValue(version.Attributes, "STOCK_FORM") != "GOODS" {
		t.Fatalf("加工 %+v", version)
	}
	if mustMeasure(t, version.Measures, "PIECE").Quantity != "2" || len(version.Lines) != 2 {
		t.Fatalf("加工计量 %+v 行 %+v", version.Measures, version.Lines)
	}
	input := lineByKind(t, version.Lines, "INPUT_LINE")
	output := lineByKind(t, version.Lines, "OUTPUT_LINE")
	if mustMeasure(t, input.Measures, "PIECE").Quantity != "4" || attributeValue(input.Attributes, "STOCK_STATE") != "GOOD" || attributeValue(input.Attributes, "STOCK_FORM") != "GOODS" {
		t.Fatalf("投入行 %+v", input)
	}
	if mustMeasure(t, output.Measures, "PIECE").Quantity != "2" || attributeValue(output.Attributes, "STOCK_FORM") != "GOODS" {
		t.Fatalf("产出行 %+v", output)
	}
}

func TestARegisteredValueAddedProjectFormsAndAnUnregisteredOneDoesNot(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-B", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.charges = world.chargeHandler(t, world.clock.Now())
	advice := world.adviseGoods(t, "REQ-VA", "SYN-GOODS-01", "件", 4)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-VA", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 4,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有成立")
	}
	jobs := world.internalJobs(t)
	if world.registerJob(t, jobs, moveCommand(t, "SYN-FACT-VA-PUT", 4, "SYN-LOC-RCV", "SYN-LOC-STORE")).Outcome() != application.RegisterInternalJobRegistered {
		t.Fatal("上架没有成立")
	}
	if world.registerJob(t, jobs, moveCommand(t, "SYN-FACT-VA-EARLY", 4, "SYN-LOC-STORE", "SYN-LOC-B")).Outcome() != application.RegisterInternalJobRegistered {
		t.Fatal("没登记项目时移库没有成立")
	}
	if world.chargeCountOf(t, "VALUE_ADDED") != 0 {
		t.Fatal("没登记的移库形成了发生项")
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return world.charges.RegisterProject(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), "移库")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("项目没有登记")
	}
	back := moveCommand(t, "SYN-FACT-VA-LATE", 4, "SYN-LOC-B", "SYN-LOC-STORE")
	back.DeviceSequence = 3
	if world.registerJob(t, jobs, back).Outcome() != application.RegisterInternalJobRegistered {
		t.Fatal("登记之后的移库没有成立")
	}
	version := world.headCharge(t, "VALUE_ADDED", "SYN-FACT-VA-LATE")
	if attributeValue(version.Attributes, "PROJECT") != "移库" || len(version.Lines) != 0 || mustMeasure(t, version.Measures, "PIECE").Quantity != "4" || mustMeasure(t, version.Measures, "SHIPMENT").Quantity != "1" {
		t.Fatalf("增值服务 %+v", version)
	}
	if world.chargeCountOf(t, "VALUE_ADDED") != 1 {
		t.Fatal("上架或更早的移库也形成了")
	}
}

func (world receiptWorld) postAt(t *testing.T, at time.Time, reason domain.StockMovementReason, source string, lines ...domain.StockLineRequest) domain.StockMovement {
	t.Helper()
	poster := application.NewPostStockMovementHandler(
		world.ledger, world.warehouses, world.locations, world.zones, world.goods, world.stockIDs, fixedClock{at: at},
	)
	var result application.PostStockMovementResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var postErr error
		result, postErr = poster.Post(ctx, application.PostStockMovementCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
			Request: domain.NewStockMovementRequest(
				valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
				valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
				reason, valueOf(t, domain.NewSourceFactReference, source), at, lines,
			),
		})
		if postErr != nil {
			return postErr
		}
		if result.Outcome() != application.PostStockPosted {
			return fmt.Errorf("%s %v", result.Outcome(), result.Violations())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("入账 %s：%v", source, err)
	}
	movement, ok := result.Movement()
	if !ok {
		t.Fatal("入账没有交回流水")
	}
	return movement
}

func stockMoveLine(t *testing.T, direction domain.StockDirection, quantity int64, location string, batch domain.InboundBatchID) domain.StockLineRequest {
	t.Helper()
	return domain.NewStockLineRequest(direction, quantity, stockCell(t, "SYN-GOODS-01", func(spec *domain.StockCellSpec) {
		spec.Location = valueOf(t, domain.NewLocationCode, location)
		spec.InboundBatch = batch
	}), time.Time{})
}

func (world receiptWorld) inChargeHandler(t *testing.T, handler *application.ChargeHandler, run func(context.Context) (application.ChargeResult, error)) application.ChargeResult {
	t.Helper()
	return world.inCharge(t, run)
}

func (world receiptWorld) headCharge(t *testing.T, kind, source string) ports.ChargeVersion {
	t.Helper()
	var id string
	var version int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT occurrence_id, head_version FROM warehouse_fulfillment.charge_occurrence
		  WHERE tenant_id = $1 AND kind = $2 AND source_fact = $3`,
		"SYN-TENANT-01", kind, source).Scan(&id, &version); err != nil {
		t.Fatalf("读 %s %s：%v", kind, source, err)
	}
	loaded, found, err := world.chargeHandler(t, world.clock.Now()).Read(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), id, version)
	if err != nil || !found {
		t.Fatalf("读版本 %s：%v", id, err)
	}
	return loaded
}

func (world receiptWorld) chargeCountOf(t *testing.T, kind string) int {
	t.Helper()
	var count int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.charge_occurrence WHERE tenant_id = $1 AND kind = $2`,
		"SYN-TENANT-01", kind).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func assertStorageDay(t *testing.T, world measurementWorld, id string, age int, location string, locationDays int) {
	t.Helper()
	version, found, err := world.chargeHandler(t, world.clock.Now()).Read(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), id, 1)
	if err != nil || !found {
		t.Fatalf("仓租 %s：%v", id, err)
	}
	if len(version.Measures) != 0 || attributeValue(version.Attributes, "AGING_BASIS") != "ACTUAL_BATCH" {
		t.Fatalf("仓租头 %+v %+v", version.Measures, version.Attributes)
	}
	var batch, place ports.ChargeLine
	batches, places := 0, 0
	for _, line := range version.Lines {
		switch line.Kind {
		case "INBOUND_BATCH":
			batch = line
			batches++
		case "LOCATION":
			place = line
			places++
		}
	}
	if batches != 1 || places != 1 {
		t.Fatalf("行 %d %d %+v", batches, places, version.Lines)
	}
	if batch.Kind != "INBOUND_BATCH" || mustMeasure(t, batch.Measures, "PIECE").Quantity != "4" || mustMeasure(t, batch.Measures, "DAY").Quantity != fmt.Sprintf("%d", age) {
		t.Fatalf("批次行 %+v", batch)
	}
	volume := mustMeasure(t, batch.Measures, "FT3")
	if volume.Class != "VOLUME" || !volume.Unmeasured || volume.Original != "32" || volume.OriginalUnit != "CM3" {
		t.Fatalf("仓租体积 %+v", volume)
	}
	if attributeValue(batch.Attributes, "STOCK_STATE") != "GOOD" || attributeValue(batch.Attributes, "STOCK_FORM") != "GOODS" ||
		attributeValue(batch.Attributes, "ZONE_USAGE") != "STORAGE" || attributeValue(batch.Attributes, "FROZEN") != "无" {
		t.Fatalf("批次属性 %+v", batch.Attributes)
	}
	if place.Kind != "LOCATION" || mustMeasure(t, place.Measures, "LOCATION_DAY").Quantity != fmt.Sprintf("%d", locationDays) {
		t.Fatalf("库位行 %s %+v", location, version.Lines)
	}
}

func mustMeasure(t *testing.T, measures []ports.ChargeMeasure, unit string) ports.ChargeMeasure {
	t.Helper()
	for _, measure := range measures {
		if measure.Unit == unit {
			return measure
		}
	}
	t.Fatalf("没有单位 %s：%+v", unit, measures)
	return ports.ChargeMeasure{}
}

func lineByKind(t *testing.T, lines []ports.ChargeLine, kind string) ports.ChargeLine {
	t.Helper()
	for _, line := range lines {
		if line.Kind == kind {
			return line
		}
	}
	t.Fatalf("没有行 %s", kind)
	return ports.ChargeLine{}
}
