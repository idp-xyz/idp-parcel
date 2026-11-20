package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// Covers: AT-WF-088、AT-WF-092、AT-WF-094、AT-WF-097 里这一包已经能判的部分。
func TestInternalJobReplenishFifoDeactivatedAndCondition(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-PICK", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsagePicking
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-PICK", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-PICK")
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OFF", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
		spec.Deactivated = true
	}))
	advice := world.adviseGoods(t, "REQ-BOX", "SYN-GOODS-01", "箱", 1)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-BOX", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", unit: "箱", quantity: 1,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("按箱收货没有入账")
	}
	handler := world.internalJobs(t)
	putAwayCommand := moveCommand(t, "SYN-FACT-BOX-UP", 1, "SYN-LOC-RCV", "SYN-LOC-STORE")
	putAwayCommand.Lines = []domain.InternalJobLine{mustJobLine(t, domain.InternalJobLineSpec{
		Unit: valueOf(t, domain.NewGoodsUnitName, "箱"), Quantity: 1,
		Source: valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"), Target: valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE"),
	})}
	putAway := world.registerJob(t, handler, putAwayCommand)
	if putAway.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, "SYN-LOC-STORE", "箱") != 1 {
		t.Fatalf("箱上架 %s 存储 %d", putAway.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "箱"))
	}
	replenish := moveCommand(t, "SYN-FACT-REPL", 1, "SYN-LOC-STORE", "SYN-LOC-PICK")
	replenish.Lines = []domain.InternalJobLine{mustJobLine(t, domain.InternalJobLineSpec{
		Unit: valueOf(t, domain.NewGoodsUnitName, "箱"), Quantity: 1,
		Source: valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE"), Target: valueOf(t, domain.NewLocationCode, "SYN-LOC-PICK"),
		ToUnit: valueOf(t, domain.NewGoodsUnitName, "件"), ToQuantity: 24,
	})}
	posted := world.registerJob(t, handler, replenish)
	job, ok := posted.Job()
	if posted.Outcome() != application.RegisterInternalJobRegistered || !ok || job.Lines()[0].Result().Posted != 1 || job.Lines()[0].Result().PostedInbound != 24 {
		t.Fatalf("补货 %s %+v", posted.Outcome(), job.Lines())
	}
	if world.stockUnitAt(t, "SYN-LOC-PICK", "件") != 24 || world.stockUnitAt(t, "SYN-LOC-STORE", "箱") != 0 || world.batchCount(t, "SYN-GOODS-01") != 1 {
		t.Fatalf("补货后拣选 %d 存储 %d 批次 %d", world.stockUnitAt(t, "SYN-LOC-PICK", "件"), world.stockUnitAt(t, "SYN-LOC-STORE", "箱"), world.batchCount(t, "SYN-GOODS-01"))
	}
	badReplenish := moveCommand(t, "SYN-FACT-REPL-BAD", 1, "SYN-LOC-RCV", "SYN-LOC-STORE")
	badReplenish.Lines = []domain.InternalJobLine{mustJobLine(t, domain.InternalJobLineSpec{
		Quantity: 1, Source: valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"), Target: valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE"),
		ToUnit: valueOf(t, domain.NewGoodsUnitName, "箱"), ToQuantity: 1,
	})}
	if world.registerJob(t, handler, badReplenish).Outcome() != application.RegisterInternalJobContentInvalid {
		t.Fatal("上架行带了转入单位仍入账")
	}

	world.clock.at = world.clock.at.Add(time.Hour)
	early := world.adviseGoods(t, "REQ-EARLY", "SYN-GOODS-01", "件", 5)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-EARLY", early.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 5,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("早批次没收下")
	}
	world.clock.at = world.clock.at.Add(time.Hour)
	late := world.adviseGoods(t, "REQ-LATE", "SYN-GOODS-01", "件", 5)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-LATE", late.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 5,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("晚批次没收下")
	}
	handler = world.internalJobs(t)
	fifo := world.registerJob(t, handler, moveCommand(t, "SYN-FACT-FIFO", 7, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	fifoJob, _ := fifo.Job()
	if fifo.Outcome() != application.RegisterInternalJobRegistered || fifoJob.Lines()[0].Result().Posted != 7 || len(fifoJob.Lines()[0].Result().Batches) != 2 {
		t.Fatalf("没点名批次 %s 批次 %v", fifo.Outcome(), fifoJob.Lines()[0].Result().Batches)
	}
	earlyQty, lateQty := world.batchSplit(t, "SYN-LOC-STORE")
	if earlyQty != 5 || lateQty != 2 {
		t.Fatalf("从早到晚取：早 %d 晚 %d", earlyQty, lateQty)
	}
	if world.stockUnitAt(t, "SYN-LOC-RCV", "件") != 3 {
		t.Fatalf("暂存应剩晚批次 3 件，实得 %d", world.stockUnitAt(t, "SYN-LOC-RCV", "件"))
	}
	lateID := fifoJob.Lines()[0].Result().Batches[1]
	named := moveCommand(t, "SYN-FACT-NAMED", 3, "SYN-LOC-RCV", "SYN-LOC-OFF")
	namedLine := mustJobLine(t, domain.InternalJobLineSpec{
		Quantity: 3, Source: valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"),
		Target: valueOf(t, domain.NewLocationCode, "SYN-LOC-OFF"), NamedBatch: lateID,
	})
	named.Lines = []domain.InternalJobLine{namedLine}
	namedResult := world.registerJob(t, handler, named)
	namedJob, _ := namedResult.Job()
	if namedResult.Outcome() != application.RegisterInternalJobRegistered || namedJob.Lines()[0].Result().Posted != 3 || world.stockUnitAt(t, "SYN-LOC-OFF", "件") != 3 {
		t.Fatalf("点名晚批次、目标已停用 %s 入账 %d", namedResult.Outcome(), namedJob.Lines()[0].Result().Posted)
	}

	pendingAdvice := world.adviseGoods(t, "REQ-PEND", "SYN-GOODS-01", "件", 4)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-PEND", pendingAdvice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 4, state: domain.StockStatePendingInspection,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("待检没收下")
	}
	converted := world.registerJob(t, handler, conditionCommand(t, "SYN-FACT-COND", 4, domain.StockStatePendingInspection, domain.StockStateGood))
	convertedJob, _ := converted.Job()
	if converted.Outcome() != application.RegisterInternalJobRegistered || convertedJob.Lines()[0].Result().Posted != 4 || convertedJob.Lines()[0].Result().Discrepancy.String() != "" {
		t.Fatalf("待检转良品 %s", converted.Outcome())
	}
	shortAdvice := world.adviseGoods(t, "REQ-PEND-2", "SYN-GOODS-01", "件", 3)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-PEND-2", shortAdvice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 3, state: domain.StockStatePendingInspection,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("第二笔待检没收下")
	}
	partial := world.registerJob(t, handler, conditionCommand(t, "SYN-FACT-COND-2", 4, domain.StockStatePendingInspection, domain.StockStateGood))
	partialJob, _ := partial.Job()
	if partial.Outcome() != application.RegisterInternalJobRegistered || partialJob.Lines()[0].Result().Posted != 3 || partialJob.Lines()[0].Result().ShortOnBook != 1 {
		t.Fatalf("待检不够 %s 入账 %d 不够 %d", partial.Outcome(), partialJob.Lines()[0].Result().Posted, partialJob.Lines()[0].Result().ShortOnBook)
	}
}

// Covers: AT-WF-087。移入次品库区的同一笔转为次品；次品移进存储库区整条不成立。
func TestInternalJobMoveIntoDefectiveZoneConvertsState(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-DEF", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageDefective
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-DEF", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-DEF")
	}))
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-DEFZONE", "SYN-GOODS-01", "件", 4)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-DEFZONE", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 4,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	handler := world.internalJobs(t)
	moved := world.registerJob(t, handler, moveCommand(t, "SYN-FACT-INTO-DEF", 4, "SYN-LOC-RCV", "SYN-LOC-DEF"))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockStateAt(t, "SYN-LOC-DEF", "DEFECTIVE") != 4 || world.stockStateAt(t, "SYN-LOC-DEF", "GOOD") != 0 {
		t.Fatalf("移入次品库区 %s 次品 %d 良品 %d", moved.Outcome(), world.stockStateAt(t, "SYN-LOC-DEF", "DEFECTIVE"), world.stockStateAt(t, "SYN-LOC-DEF", "GOOD"))
	}
	refusedCommand := moveCommand(t, "SYN-FACT-DEF-STORE", 4, "SYN-LOC-DEF", "SYN-LOC-STORE")
	refusedCommand.Lines = []domain.InternalJobLine{mustJobLine(t, domain.InternalJobLineSpec{
		State: domain.StockStateDefective, Quantity: 4,
		Source: valueOf(t, domain.NewLocationCode, "SYN-LOC-DEF"), Target: valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE"),
	})}
	refused := world.registerJob(t, handler, refusedCommand)
	if refused.Outcome() != application.RegisterInternalJobContentInvalid || world.stockStateAt(t, "SYN-LOC-DEF", "DEFECTIVE") != 4 {
		t.Fatalf("次品进存储 %s", refused.Outcome())
	}
}

// Covers: AT-WF-093 里不依赖占用账的几格，以及短少的整箱行。
func TestInternalJobCartonMove(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-PICK", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsagePicking
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-PICK", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-PICK")
	}))
	advice := world.adviseCartons(t, "REQ-CTN", "SYN-CARTON-1")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-CTN", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("整箱收货没有入账")
	}
	handler := world.internalJobs(t)
	picking := world.registerJob(t, handler, cartonMoveCommand(t, "SYN-FACT-CTN-PICK", "SYN-CARTON-1", "SYN-LOC-PICK"))
	if picking.Outcome() != application.RegisterInternalJobContentInvalid || world.cartonAt(t, "SYN-CARTON-1") != "SYN-LOC-RCV" {
		t.Fatalf("拣选库区 %s 位置 %s", picking.Outcome(), world.cartonAt(t, "SYN-CARTON-1"))
	}
	moved := world.registerJob(t, handler, cartonMoveCommand(t, "SYN-FACT-CTN-UP", "SYN-CARTON-1", "SYN-LOC-STORE"))
	movedJob, _ := moved.Job()
	if moved.Outcome() != application.RegisterInternalJobRegistered || movedJob.Lines()[0].Result().Posted != 1 || world.cartonAt(t, "SYN-CARTON-1") != "SYN-LOC-STORE" || world.cartonBatchCount(t) != 1 {
		t.Fatalf("整箱上架 %s 位置 %s 批次 %d", moved.Outcome(), world.cartonAt(t, "SYN-CARTON-1"), world.cartonBatchCount(t))
	}
	missing := world.registerJob(t, handler, cartonMoveCommand(t, "SYN-FACT-CTN-MISS", "SYN-CARTON-NONE", "SYN-LOC-STORE"))
	missingJob, _ := missing.Job()
	if missing.Outcome() != application.RegisterInternalJobRegistered || !missingJob.Lines()[0].Result().CartonMissing || missingJob.Lines()[0].Result().Discrepancy.String() == "" {
		t.Fatalf("箱号对不上 %s", missing.Outcome())
	}
	before := world.cartonBalance(t, "SYN-CARTON-1")
	shortage := cartonMoveCommand(t, "SYN-FACT-CTN-SHORT", "SYN-CARTON-1", "")
	shortage.Kind = domain.InternalJobShortage
	shortage.Lines = []domain.InternalJobLine{mustJobLine(t, domain.InternalJobLineSpec{Carton: valueOf(t, domain.NewCartonNumber, "SYN-CARTON-1")})}
	noted := world.registerJob(t, handler, shortage)
	if noted.Outcome() != application.RegisterInternalJobShortageRegistered || world.cartonBalance(t, "SYN-CARTON-1") != before {
		t.Fatalf("整箱短少 %s 余额 %d", noted.Outcome(), world.cartonBalance(t, "SYN-CARTON-1"))
	}
}

// Covers: AT-WF-090、AT-WF-091 的减法。占用账还没有，数量由测试口交回。
func TestInternalJobSubtractsHoldsAndReservations(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE-2", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-HOLD", "SYN-GOODS-01", "件", 10)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-HOLD-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 10,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	held := world.internalJobsHeld(t, scriptedHolds{cell: 4})
	moved := world.registerJob(t, held, moveCommand(t, "SYN-FACT-HELD", 10, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	movedJob, _ := moved.Job()
	if moved.Outcome() != application.RegisterInternalJobRegistered || movedJob.Lines()[0].Result().Posted != 6 || movedJob.Lines()[0].Result().Held != 4 {
		t.Fatalf("占用 %s 入账 %d 占着 %d", moved.Outcome(), movedJob.Lines()[0].Result().Posted, movedJob.Lines()[0].Result().Held)
	}
	reserved := world.internalJobsHeld(t, scriptedHolds{reservation: 7})
	back := world.registerJob(t, reserved, moveCommand(t, "SYN-FACT-RESERVE", 6, "SYN-LOC-STORE", "SYN-LOC-RCV"))
	backJob, _ := back.Job()
	if back.Outcome() != application.RegisterInternalJobRegistered || backJob.Lines()[0].Result().Posted != 0 || backJob.Lines()[0].Result().Held != 6 {
		t.Fatalf("预占移出可分配 %s 入账 %d 占着 %d", back.Outcome(), backJob.Lines()[0].Result().Posted, backJob.Lines()[0].Result().Held)
	}
	sideways := world.registerJob(t, reserved, moveCommand(t, "SYN-FACT-SIDEWAYS", 6, "SYN-LOC-STORE", "SYN-LOC-STORE-2"))
	sideJob, _ := sideways.Job()
	if sideways.Outcome() != application.RegisterInternalJobRegistered || sideJob.Lines()[0].Result().Posted != 6 || world.stockUnitAt(t, "SYN-LOC-STORE-2", "件") != 6 {
		t.Fatalf("存储之间预占不挡 %s 入账 %d", sideways.Outcome(), sideJob.Lines()[0].Result().Posted)
	}
}

type scriptedHolds struct {
	cell        int64
	reservation int64
	carton      bool
}

func (holds scriptedHolds) CellHold(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.StockCell) (int64, error) {
	return holds.cell, nil
}

func (holds scriptedHolds) UnassignedReservation(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.GoodsCode, domain.StockState, domain.GoodsAliasCode, domain.BatchNumber, domain.ExpiryDate) (int64, error) {
	return holds.reservation, nil
}

func (holds scriptedHolds) CartonHeld(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.CartonNumber) (bool, error) {
	return holds.carton, nil
}

func mustJobLine(t *testing.T, spec domain.InternalJobLineSpec) domain.InternalJobLine {
	t.Helper()
	if spec.Owner.String() == "" {
		spec.Owner = valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	}
	if spec.Goods.String() == "" && spec.Carton.String() == "" {
		spec.Goods = valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01")
	}
	if spec.Unit.String() == "" && spec.Carton.String() == "" {
		spec.Unit = valueOf(t, domain.NewGoodsUnitName, "件")
	}
	if spec.State.String() == "" && spec.Carton.String() == "" {
		spec.State = domain.StockStateGood
	}
	line, err := domain.NewInternalJobLine(spec)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func conditionCommand(t *testing.T, fact string, quantity int64, from, to domain.StockState) application.RegisterInternalJobCommand {
	t.Helper()
	command := moveCommand(t, fact, quantity, "SYN-LOC-RCV", "SYN-LOC-RCV")
	command.Kind = domain.InternalJobCondition
	command.Lines = []domain.InternalJobLine{mustJobLine(t, domain.InternalJobLineSpec{
		State: from, ToState: to, Quantity: quantity, Source: valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"),
	})}
	return command
}

func cartonMoveCommand(t *testing.T, fact, carton, target string) application.RegisterInternalJobCommand {
	t.Helper()
	spec := domain.InternalJobLineSpec{Carton: valueOf(t, domain.NewCartonNumber, carton)}
	if target != "" {
		spec.Target = valueOf(t, domain.NewLocationCode, target)
	}
	command := moveCommand(t, fact, 1, "SYN-LOC-RCV", "SYN-LOC-STORE")
	command.Lines = []domain.InternalJobLine{mustJobLine(t, spec)}
	return command
}

func (world receiptWorld) cartonAt(t *testing.T, number string) string {
	t.Helper()
	rows, err := world.pool.Query(t.Context(),
		`SELECT location_code
		   FROM warehouse_fulfillment.carton_movement_line
		  WHERE tenant_id = $1 AND carton_number = $2
		  GROUP BY location_code
		 HAVING sum(quantity_delta) > 0`, "SYN-TENANT-01", number)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var location string
	count := 0
	for rows.Next() {
		if err := rows.Scan(&location); err != nil {
			t.Fatal(err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("%s 在 %d 个库位", number, count)
	}
	return location
}

// batchSplit 把一个库位上按入库时间排好的余额交回头两格。没有第二格时第二格是 0。
func (world receiptWorld) batchSplit(t *testing.T, location string) (int64, int64) {
	t.Helper()
	rows, err := world.pool.Query(t.Context(),
		`SELECT sum(line.quantity_delta)
		   FROM warehouse_fulfillment.stock_movement_line line
		   JOIN warehouse_fulfillment.inbound_batch batch
		     ON batch.tenant_id = line.tenant_id AND batch.inbound_batch_id = line.inbound_batch_id
		  WHERE line.tenant_id = $1 AND line.location_code = $2
		  GROUP BY batch.inbound_batch_id, batch.inbound_at
		 HAVING sum(line.quantity_delta) > 0
		  ORDER BY batch.inbound_at`, "SYN-TENANT-01", location)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var quantities []int64
	for rows.Next() {
		var quantity int64
		if err := rows.Scan(&quantity); err != nil {
			t.Fatal(err)
		}
		quantities = append(quantities, quantity)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var first, second int64
	if len(quantities) > 0 {
		first = quantities[0]
	}
	if len(quantities) > 1 {
		second = quantities[1]
	}
	return first, second
}
