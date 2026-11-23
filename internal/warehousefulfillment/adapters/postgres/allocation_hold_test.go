package postgres_test

import (
	"context"
	"testing"
	"time"

	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 两份都要齐预占才进同一波次。先入的那一格只有 10，装不下两份各 6，所以旁边再放后入的 6。
// 争用仍落在先入的 10 上：后一份不能把先一份已经分走的 6 再分一遍。

func TestOneWaveDoesNotAllocateTheSameTenUnitsTwice(t *testing.T) {
	world, first, second := reservedPair(t)
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseRequests(t, handler, "WAVE-BOTH", first.ID(), second.ID())
	if released.Outcome() != application.WaveAllocated {
		t.Fatalf("分配 %s", released.Outcome())
	}
	if quantityOn(released.Tasks(), first.ID(), "SYN-LOC-STORE") != 6 || quantityOn(released.Tasks(), first.ID(), "SYN-LOC-STORE-2") != 0 {
		t.Fatalf("先一份 %+v", released.Tasks())
	}
	if quantityOn(released.Tasks(), second.ID(), "SYN-LOC-STORE") != 4 || quantityOn(released.Tasks(), second.ID(), "SYN-LOC-STORE-2") != 2 {
		t.Fatalf("后一份 %+v", released.Tasks())
	}
}

func TestALaterWaveDoesNotReuseAnOpenAllocation(t *testing.T) {
	world, first, second := reservedPair(t)
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	earlier := world.releaseWave(t, handler, "WAVE-EARLIER", first.ID())
	if earlier.Outcome() != application.WaveAllocated || quantityOn(earlier.Tasks(), first.ID(), "SYN-LOC-STORE") != 6 {
		t.Fatalf("先放 %s %+v", earlier.Outcome(), earlier.Tasks())
	}
	later := world.releaseWave(t, handler, "WAVE-LATER", second.ID())
	if later.Outcome() != application.WaveAllocated || quantityOn(later.Tasks(), second.ID(), "SYN-LOC-STORE") != 4 ||
		quantityOn(later.Tasks(), second.ID(), "SYN-LOC-STORE-2") != 2 {
		t.Fatalf("后放 %s %+v", later.Outcome(), later.Tasks())
	}
}

func TestPickingAnAllocatedTaskMovesTheStockAndReleasesTheHold(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	world.saveOutboundStaging(t)
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-PICK", 6))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok || request.Reservations()[0].Quantity != 6 {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-PICK", request.ID())
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 1 || released.Tasks()[0].Quantity != 6 {
		t.Fatalf("分配 %s %+v", released.Outcome(), released.Tasks())
	}
	picked := world.pick(t, handler, released.Tasks()[0].ID, 6)
	if picked.Outcome() != application.WavePicked || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 4 || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 6 {
		t.Fatalf("拣货 %s 存储 %d 待出库 %d", picked.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-OUT", "件"))
	}
	rest := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-REST", 4))
	restRequest, restOK := rest.Request()
	if rest.Outcome() != application.OutboundReserved || !restOK {
		t.Fatalf("余量预占 %s", rest.Outcome())
	}
	again := world.releaseWave(t, handler, "WAVE-REST", restRequest.ID())
	if again.Outcome() != application.WaveAllocated || quantityOn(again.Tasks(), restRequest.ID(), "SYN-LOC-STORE") != 4 {
		t.Fatalf("拣完再分配 %s %+v", again.Outcome(), again.Tasks())
	}
}

func TestAnInternalMoveLeavesAllocatedQuantity(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE-2", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-HOLD-MOVE", 6))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	if released := world.releaseWave(t, handler, "WAVE-HOLD", request.ID()); released.Outcome() != application.WaveAllocated {
		t.Fatalf("分配 %s", released.Outcome())
	}
	moved := world.registerJob(t, world.internalJobsHeld(t, world.ledgerHolds(t)), moveCommand(t, "SYN-FACT-ALLOC-MOVE", 10, "SYN-LOC-STORE", "SYN-LOC-STORE-2"))
	job, has := moved.Job()
	if !has || moved.Outcome() != application.RegisterInternalJobRegistered {
		t.Fatalf("移库 %s", moved.Outcome())
	}
	result := job.Lines()[0].Result()
	if result.Posted != 4 || result.Held != 6 || result.Discrepancy.String() == "" ||
		world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 6 || world.stockUnitAt(t, "SYN-LOC-STORE-2", "件") != 4 {
		t.Fatalf("入账 %d 占着 %d 差异 %s 存储 %d 另一格 %d", result.Posted, result.Held, result.Discrepancy, world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-STORE-2", "件"))
	}
}

func TestExpiryTrackedGoodsAllocateTheEarliestExpiryFirst(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-EXP-01", 1, "REQ-EXP", func(spec *domain.GoodsContentSpec) {
		spec.TrackExpiry = true
		spec.Aliases = nil
	}))
	world.shelfExpiry(t, 4, "SYN-LOC-OLD", "2027-12-01", "OLD")
	world.clock.at = world.clock.at.Add(time.Hour)
	world.shelfExpiry(t, 4, "SYN-LOC-NEW", "2027-01-15", "NEW")
	command := outboundCommand(t, "REQ-EXP-OUT", 4)
	command.Lines = []domain.OutboundLine{mustOutboundLine(t, "SYN-EXP-01", 4)}
	accepted := world.acceptOutbound(t, world.outbound(t), command)
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok || request.Reservations()[0].Quantity != 4 {
		t.Fatalf("预占 %s %+v", accepted.Outcome(), request.Reservations())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-EXP", request.ID())
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 1 ||
		released.Tasks()[0].Location.String() != "SYN-LOC-NEW" || released.Tasks()[0].Quantity != 4 {
		t.Fatalf("先到期 %s %+v", released.Outcome(), released.Tasks())
	}
}

func TestANamedExpiryDoesNotReserveStockAnUnnamedReservationAlreadyCovers(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-EXP-01", 1, "REQ-EXP-NAMED", func(spec *domain.GoodsContentSpec) {
		spec.TrackExpiry = true
		spec.Aliases = nil
	}))
	world.shelfExpiry(t, 4, "SYN-LOC-OLD", "2027-12-01", "NAMED-OLD")
	world.clock.at = world.clock.at.Add(time.Hour)
	world.shelfExpiry(t, 4, "SYN-LOC-NEW", "2027-01-15", "NAMED-NEW")
	outbound := world.outbound(t)
	command := outboundCommand(t, "REQ-EXP-ALL", 8)
	command.Lines = []domain.OutboundLine{mustOutboundLine(t, "SYN-EXP-01", 8)}
	accepted := world.acceptOutbound(t, outbound, command)
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok || request.Reservations()[0].Quantity != 8 {
		t.Fatalf("没点名 %s %+v", accepted.Outcome(), request.Reservations())
	}
	if world.registerShortage(t, outbound, 1, domain.ShortagePolicyPartial).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("部分预占没有登记")
	}
	named, err := domain.NewOutboundLine(domain.OutboundLineSpec{
		Goods: valueOf(t, domain.NewGoodsCode, "SYN-EXP-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"),
		State: domain.StockStateGood, Quantity: 4, Expiry: valueOf(t, domain.NewExpiryDate, "2027-01-15"),
	})
	if err != nil {
		t.Fatal(err)
	}
	pointed := outboundCommand(t, "REQ-EXP-NAMED", 4)
	pointed.Lines = []domain.OutboundLine{named}
	second := world.acceptOutbound(t, outbound, pointed)
	landed, landedOK := second.Request()
	if second.Outcome() != application.OutboundPartiallyReserved || !landedOK || len(landed.Reservations()) != 1 || landed.Reservations()[0].Quantity != 0 {
		t.Fatalf("点名 %s %+v", second.Outcome(), landed.Reservations())
	}
}

func reservedPair(t *testing.T) (receiptWorld, domain.OutboundRequest, domain.OutboundRequest) {
	t.Helper()
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	world.clock.at = world.clock.at.Add(time.Hour)
	world.shelfOnto(t, 6, "SYN-LOC-STORE-2", "LATER")
	outbound := world.outbound(t)
	firstResult := world.acceptOutbound(t, outbound, outboundCommand(t, "REQ-FIRST", 6))
	secondResult := world.acceptOutbound(t, outbound, outboundCommand(t, "REQ-SECOND", 6))
	first, firstOK := firstResult.Request()
	second, secondOK := secondResult.Request()
	if firstResult.Outcome() != application.OutboundReserved || !firstOK || first.Reservations()[0].Quantity != 6 ||
		secondResult.Outcome() != application.OutboundReserved || !secondOK || second.Reservations()[0].Quantity != 6 {
		t.Fatalf("预占 %s %s", firstResult.Outcome(), secondResult.Outcome())
	}
	return world, first, second
}

func (world receiptWorld) shelfOnto(t *testing.T, quantity int64, location, key string) {
	t.Helper()
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", location, 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-SHELF-"+key, "SYN-GOODS-01", "件", quantity)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-RCV-"+key, advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: quantity,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	moved := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-PUT-"+key, quantity, "SYN-LOC-RCV", location))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, location, "件") != quantity {
		t.Fatalf("上架 %s 库存 %d", moved.Outcome(), world.stockUnitAt(t, location, "件"))
	}
}

func (world receiptWorld) shelfExpiry(t *testing.T, quantity int64, location, expiry, key string) {
	t.Helper()
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", location, 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-EXP-"+key, "SYN-EXP-01", "件", quantity)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-EXP-"+key, advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-EXP-01", quantity: quantity, expiry: expiry,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	line, err := domain.NewInternalJobLine(domain.InternalJobLineSpec{
		Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-EXP-01"),
		Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood, Quantity: quantity,
		Expiry: valueOf(t, domain.NewExpiryDate, expiry),
		Source: valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"), Target: valueOf(t, domain.NewLocationCode, location),
	})
	if err != nil {
		t.Fatal(err)
	}
	command := moveCommand(t, "SYN-FACT-EXP-PUT-"+key, quantity, "SYN-LOC-RCV", location)
	command.Lines = []domain.InternalJobLine{line}
	moved := world.registerJob(t, world.internalJobs(t), command)
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, location, "件") != quantity {
		t.Fatalf("上架 %s 库存 %d", moved.Outcome(), world.stockUnitAt(t, location, "件"))
	}
}

func (world receiptWorld) saveOutboundStaging(t *testing.T) {
	t.Helper()
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
}

func (world receiptWorld) ledgerHolds(t *testing.T) ports.StockHolds {
	t.Helper()
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	freezes, err := adapter.NewFreezes(world.db)
	if err != nil {
		t.Fatal(err)
	}
	counts, err := adapter.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	return adapter.LedgerHolds{
		Freezes: adapter.NewFreezeHolds(freezes), Counts: counts, Reservations: requests, Allocations: waves,
	}
}

func (world receiptWorld) releaseRequests(t *testing.T, handler *application.WaveHandler, id string, requests ...domain.OutboundRequestID) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var releaseErr error
		result, releaseErr = handler.Release(ctx, application.ReleaseWaveCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: valueOf(t, domain.NewWaveID, id),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
			Requests: requests,
		})
		return releaseErr
	})
	if err != nil {
		t.Fatalf("放波次：%v", err)
	}
	return result
}

func quantityOn(tasks []ports.PickTask, request domain.OutboundRequestID, location string) int64 {
	var sum int64
	for _, task := range tasks {
		if task.Request == request && task.Location.String() == location {
			sum += task.Quantity
		}
	}
	return sum
}

func TestFifoGoodsTakeTheOlderBatchInStorageBeforeANewerBatchInPicking(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 5)
	world.savePickingPlace(t, "SYN-LOC-PICK", nil)
	world.clock.at = world.clock.at.Add(time.Hour)
	world.receiveAndShelve(t, 5, "SYN-LOC-PICK", "PICK-NEW")
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-FIFO", 5))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-FIFO", request.ID())
	if released.Outcome() != application.WaveAllocated || quantityOn(released.Tasks(), request.ID(), "SYN-LOC-STORE") != 5 ||
		quantityOn(released.Tasks(), request.ID(), "SYN-LOC-PICK") != 0 {
		t.Fatalf("先入的存储 %+v", released.Tasks())
	}
}

func TestWithinOneBatchPickingComesBeforeStorage(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 8)
	world.savePickingPlace(t, "SYN-LOC-PICK", nil)
	moved := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-SAME-BATCH", 4, "SYN-LOC-STORE", "SYN-LOC-PICK"))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, "SYN-LOC-PICK", "件") != 4 {
		t.Fatalf("同批移到拣选 %s 拣选 %d", moved.Outcome(), world.stockUnitAt(t, "SYN-LOC-PICK", "件"))
	}
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-ZONE", 4))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-ZONE", request.ID())
	if released.Outcome() != application.WaveAllocated || quantityOn(released.Tasks(), request.ID(), "SYN-LOC-PICK") != 4 ||
		quantityOn(released.Tasks(), request.ID(), "SYN-LOC-STORE") != 0 {
		t.Fatalf("同批先拣选 %+v", released.Tasks())
	}
}

func TestWithinOneZoneAndBatchHigherPriorityComesFirst(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 3)
	five := int32(5)
	one := int32(1)
	world.saveStoragePlace(t, "SYN-LOC-HI", &five)
	world.saveStoragePlace(t, "SYN-LOC-LO", &one)
	if world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-PRI-HI", 1, "SYN-LOC-STORE", "SYN-LOC-HI")).Outcome() != application.RegisterInternalJobRegistered {
		t.Fatal("高优先级没有上架")
	}
	if world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-PRI-LO", 1, "SYN-LOC-STORE", "SYN-LOC-LO")).Outcome() != application.RegisterInternalJobRegistered {
		t.Fatal("低优先级没有上架")
	}
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-PRI", 3))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-PRI", request.ID())
	tasks := released.Tasks()
	if released.Outcome() != application.WaveAllocated || len(tasks) != 3 ||
		tasks[0].Location.String() != "SYN-LOC-HI" || tasks[1].Location.String() != "SYN-LOC-LO" || tasks[2].Location.String() != "SYN-LOC-STORE" {
		t.Fatalf("优先级 %+v", tasks)
	}
}

func TestReplenishmentCarriesAllocatedQuantityAndPickStillWorks(t *testing.T) {
	world := newReceiptWorld(t)
	world.prepareMovePlaces(t)
	noted := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-REPL-GAP", 10, "SYN-LOC-STORE", "SYN-LOC-PICK"))
	notedJob, notedOK := noted.Job()
	if noted.Outcome() != application.RegisterInternalJobRegistered || !notedOK || len(notedJob.Lines()) == 0 || notedJob.Lines()[0].Result().Discrepancy.String() == "" {
		t.Fatalf("差额 %s", noted.Outcome())
	}
	world.putAway(t, 10, "SYN-FACT-REPL-PUT")
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-REPL", 6))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-REPL", request.ID())
	if released.Outcome() != application.WaveAllocated || quantityOn(released.Tasks(), request.ID(), "SYN-LOC-STORE") != 6 {
		t.Fatalf("分配 %+v", released.Tasks())
	}
	duties := world.dutyStore(t)
	if world.registerDuty(t, world.approver(t, duties), false).Outcome() != application.ApprovalDutyRegistered {
		t.Fatal("规则没有登记")
	}
	world.saveOutboundStaging(t)
	posted := world.approveJob(t, world.allocationApprover(t, duties), world.onlyJobDiscrepancy(t))
	if posted.Outcome() != application.ApprovalPosted || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 0 || world.stockUnitAt(t, "SYN-LOC-PICK", "件") != 10 {
		t.Fatalf("补入账 %s 存储 %d 拣选 %d", posted.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-PICK", "件"))
	}
	picks := world.openPicks(t)
	if len(picks) != 1 || picks[0].location != "SYN-LOC-PICK" || picks[0].quantity != 6 {
		t.Fatalf("拣货任务 %+v", picks)
	}
	picked := world.pick(t, handler, valueOf(t, domain.NewPickTaskID, picks[0].id), 6)
	if picked.Outcome() != application.WavePicked || world.stockUnitAt(t, "SYN-LOC-PICK", "件") != 4 || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 6 {
		t.Fatalf("拣出 %s 拣选 %d 待出库 %d", picked.Outcome(), world.stockUnitAt(t, "SYN-LOC-PICK", "件"), world.stockUnitAt(t, "SYN-LOC-OUT", "件"))
	}
}

func TestReplenishmentSplitsAPickTaskWhenOnlyPartOfItMoves(t *testing.T) {
	world := newReceiptWorld(t)
	world.prepareMovePlaces(t)
	noted := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-SPLIT-GAP", 7, "SYN-LOC-STORE", "SYN-LOC-PICK"))
	notedJob, notedOK := noted.Job()
	if noted.Outcome() != application.RegisterInternalJobRegistered || !notedOK || len(notedJob.Lines()) == 0 || notedJob.Lines()[0].Result().Discrepancy.String() == "" {
		t.Fatalf("差额 %s", noted.Outcome())
	}
	world.putAway(t, 10, "SYN-FACT-SPLIT-PUT")
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-SPLIT", 6))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	if released := world.releaseWave(t, handler, "WAVE-SPLIT", request.ID()); released.Outcome() != application.WaveAllocated {
		t.Fatalf("分配 %s", released.Outcome())
	}
	duties := world.dutyStore(t)
	if world.registerDuty(t, world.approver(t, duties), false).Outcome() != application.ApprovalDutyRegistered {
		t.Fatal("规则没有登记")
	}
	posted := world.approveJob(t, world.allocationApprover(t, duties), world.onlyJobDiscrepancy(t))
	if posted.Outcome() != application.ApprovalPosted || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 3 || world.stockUnitAt(t, "SYN-LOC-PICK", "件") != 7 {
		t.Fatalf("补入账 %s 存储 %d 拣选 %d", posted.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-PICK", "件"))
	}
	picks := world.openPicks(t)
	var atStore, atPick int64
	var wave, inbound, req string
	for _, pick := range picks {
		if wave == "" {
			wave, inbound, req = pick.wave, pick.inbound, pick.request
		}
		if pick.wave != wave || pick.inbound != inbound || pick.request != req {
			t.Fatalf("拆开的任务不是同一波次 %+v", picks)
		}
		switch pick.location {
		case "SYN-LOC-STORE":
			atStore += pick.quantity
		case "SYN-LOC-PICK":
			atPick += pick.quantity
		}
	}
	if len(picks) != 2 || atStore != 3 || atPick != 3 {
		t.Fatalf("拆分 %+v", picks)
	}
}

func TestReplenishmentDoesNotStandWhenTheDestinationCannotCarryAllocation(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	noted := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-BAD-DEST", 10, "SYN-LOC-STORE", "SYN-LOC-RCV"))
	notedJob, notedOK := noted.Job()
	if noted.Outcome() != application.RegisterInternalJobRegistered || !notedOK || len(notedJob.Lines()) == 0 || notedJob.Lines()[0].Result().Discrepancy.String() == "" {
		t.Fatalf("差额 %s", noted.Outcome())
	}
	world.putAway(t, 10, "SYN-FACT-BAD-PUT")
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-BAD", 6))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	if released := world.releaseWave(t, handler, "WAVE-BAD", request.ID()); released.Outcome() != application.WaveAllocated {
		t.Fatalf("分配 %s", released.Outcome())
	}
	duties := world.dutyStore(t)
	if world.registerDuty(t, world.approver(t, duties), false).Outcome() != application.ApprovalDutyRegistered {
		t.Fatal("规则没有登记")
	}
	posted := world.approveJob(t, world.allocationApprover(t, duties), world.onlyJobDiscrepancy(t))
	if posted.Outcome() != application.ApprovalNotValid || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 {
		t.Fatalf("去向不可分配 %s 存储 %d", posted.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	picks := world.openPicks(t)
	if len(picks) != 1 || picks[0].location != "SYN-LOC-STORE" || picks[0].quantity != 6 {
		t.Fatalf("任务不该改 %+v", picks)
	}
}

func TestReplenishmentDoesNotStandWhileFrozenQuantityWouldBeLeftBehind(t *testing.T) {
	world := newReceiptWorld(t)
	world.prepareMovePlaces(t)
	noted := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-FRZ-GAP", 10, "SYN-LOC-STORE", "SYN-LOC-PICK"))
	if noted.Outcome() != application.RegisterInternalJobRegistered {
		t.Fatalf("差额 %s", noted.Outcome())
	}
	world.putAway(t, 10, "SYN-FACT-FRZ-PUT")
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-FRZ", 6))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	if released := world.releaseWave(t, handler, "WAVE-FRZ", request.ID()); released.Outcome() != application.WaveAllocated {
		t.Fatalf("分配 %s", released.Outcome())
	}
	batch := world.onlyBatch(t, "SYN-LOC-STORE")
	if frozen := world.freeze(t, world.freezer(t), freezeGoods(t, "SYN-DEC-FRZ", "SYN-LOC-STORE", batch, 4)); frozen.Outcome() != application.FreezeFrozen {
		t.Fatalf("冻结 %s", frozen.Outcome())
	}
	duties := world.dutyStore(t)
	if world.registerDuty(t, world.approver(t, duties), false).Outcome() != application.ApprovalDutyRegistered {
		t.Fatal("规则没有登记")
	}
	posted := world.approveJob(t, world.allocationApprover(t, duties), world.onlyJobDiscrepancy(t))
	if posted.Outcome() != application.ApprovalNotValid || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 || world.stockUnitAt(t, "SYN-LOC-PICK", "件") != 0 {
		t.Fatalf("冻结挡住 %s 存储 %d 拣选 %d", posted.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-PICK", "件"))
	}
}

func TestAnUnnamedReservationOfSixLeavesOnlyFourAcrossTwoBatches(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 5)
	world.clock.at = world.clock.at.Add(time.Hour)
	world.shelfOnto(t, 5, "SYN-LOC-STORE-2", "LATER")
	if world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-TWO", 6)).Outcome() != application.OutboundReserved {
		t.Fatal("没有预占")
	}
	holds := world.ledgerHolds(t)
	jobs := world.internalJobsHeld(t, holds)
	first := world.registerJob(t, jobs, moveCommand(t, "SYN-FACT-TWO-A", 5, "SYN-LOC-STORE", "SYN-LOC-RCV"))
	firstJob, _ := first.Job()
	if first.Outcome() != application.RegisterInternalJobRegistered || firstJob.Lines()[0].Result().Posted != 4 ||
		world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 1 || world.stockUnitAt(t, "SYN-LOC-STORE-2", "件") != 5 {
		t.Fatalf("先取 %s 入账 %d 甲 %d 乙 %d", first.Outcome(), firstJob.Lines()[0].Result().Posted, world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-STORE-2", "件"))
	}
	second := world.registerJob(t, jobs, moveCommand(t, "SYN-FACT-TWO-B", 5, "SYN-LOC-STORE-2", "SYN-LOC-RCV"))
	secondJob, _ := second.Job()
	if second.Outcome() != application.RegisterInternalJobRegistered || secondJob.Lines()[0].Result().Posted != 0 ||
		world.stockUnitAt(t, "SYN-LOC-STORE", "件")+world.stockUnitAt(t, "SYN-LOC-STORE-2", "件") != 6 {
		t.Fatalf("再取 %s 入账 %d 可分配 %d", second.Outcome(), secondJob.Lines()[0].Result().Posted, world.stockUnitAt(t, "SYN-LOC-STORE", "件")+world.stockUnitAt(t, "SYN-LOC-STORE-2", "件"))
	}
}

func TestANamedExpiryReservationBesideAnUnnamedOneStaysWithinOnHand(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-EXP-01", 1, "REQ-EXP-BOTH", func(spec *domain.GoodsContentSpec) {
		spec.TrackExpiry = true
		spec.Aliases = nil
	}))
	world.shelfExpiry(t, 5, "SYN-LOC-OLD", "2027-12-01", "BOTH-OLD")
	world.clock.at = world.clock.at.Add(time.Hour)
	world.shelfExpiry(t, 5, "SYN-LOC-NEW", "2027-01-15", "BOTH-NEW")
	outbound := world.outbound(t)
	command := outboundCommand(t, "REQ-BOTH-ALL", 6)
	command.Lines = []domain.OutboundLine{mustOutboundLine(t, "SYN-EXP-01", 6)}
	if world.acceptOutbound(t, outbound, command).Outcome() != application.OutboundReserved {
		t.Fatal("没点名没有预占")
	}
	if world.registerShortage(t, outbound, 1, domain.ShortagePolicyPartial).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("部分预占没有登记")
	}
	named, err := domain.NewOutboundLine(domain.OutboundLineSpec{
		Goods: valueOf(t, domain.NewGoodsCode, "SYN-EXP-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"),
		State: domain.StockStateGood, Quantity: 5, Expiry: valueOf(t, domain.NewExpiryDate, "2027-01-15"),
	})
	if err != nil {
		t.Fatal(err)
	}
	pointed := outboundCommand(t, "REQ-BOTH-NAMED", 5)
	pointed.Lines = []domain.OutboundLine{named}
	second := world.acceptOutbound(t, outbound, pointed)
	landed, landedOK := second.Request()
	if second.Outcome() != application.OutboundPartiallyReserved || !landedOK || landed.Reservations()[0].Quantity != 4 || reservedQuantity(t, world) != 10 {
		t.Fatalf("点名 %s %+v 合计 %d", second.Outcome(), landed.Reservations(), reservedQuantity(t, world))
	}
}

func TestStockReadKeepsAnUnnamedReservationOverTwoBatchesOnTheSummary(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 5)
	world.clock.at = world.clock.at.Add(time.Hour)
	world.shelfOnto(t, 5, "SYN-LOC-STORE-2", "READ")
	if world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-READ-TWO", 6)).Outcome() != application.OutboundReserved {
		t.Fatal("没有预占")
	}
	reader := world.stockReaderWithReservations(t)
	summary := world.readStock(t, reader, application.StockViewSummary, false)
	if len(summary.Summary) != 1 || summary.Summary[0].OnHand != 10 || summary.Summary[0].Reserved != 6 || summary.Summary[0].Available != 4 {
		t.Fatalf("汇总 %+v", summary.Summary)
	}
	cells := world.readStock(t, reader, application.StockViewCells, false)
	first := stockCellAt(cells.Cells, "SYN-LOC-STORE")
	second := stockCellAt(cells.Cells, "SYN-LOC-STORE-2")
	if first.Quantity != 5 || first.Available != 5 || second.Quantity != 5 || second.Available != 5 {
		t.Fatalf("格 %+v %+v", first, second)
	}
}

func stockCellAt(cells []application.StockCellView, location string) application.StockCellView {
	for _, cell := range cells {
		if cell.Location == location {
			return cell
		}
	}
	return application.StockCellView{}
}

func (world receiptWorld) savePickingPlace(t *testing.T, code string, priority *int32) {
	t.Helper()
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-PICK", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsagePicking
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", code, 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-PICK")
		if priority != nil {
			spec.Priority = domain.NewLocationPriority(*priority)
		}
	}))
}

func (world receiptWorld) saveStoragePlace(t *testing.T, code string, priority *int32) {
	t.Helper()
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", code, 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
		if priority != nil {
			spec.Priority = domain.NewLocationPriority(*priority)
		}
	}))
}

func (world receiptWorld) receiveAndShelve(t *testing.T, quantity int64, location, key string) {
	t.Helper()
	advice := world.adviseGoods(t, "REQ-SHELF-"+key, "SYN-GOODS-01", "件", quantity)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-RCV-"+key, advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: quantity,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	moved := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-PUT-"+key, quantity, "SYN-LOC-RCV", location))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, location, "件") != quantity {
		t.Fatalf("上架 %s 库存 %d", moved.Outcome(), world.stockUnitAt(t, location, "件"))
	}
}

func (world receiptWorld) prepareMovePlaces(t *testing.T) {
	t.Helper()
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.savePickingPlace(t, "SYN-LOC-PICK", nil)
}

func (world receiptWorld) putAway(t *testing.T, quantity int64, fact string) {
	t.Helper()
	advice := world.adviseGoods(t, "REQ-"+fact, "SYN-GOODS-01", "件", quantity)
	if world.confirm(t, receiptCommand(t, fact+"-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: quantity,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	moved := world.registerJob(t, world.internalJobs(t), moveCommand(t, fact, quantity, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != quantity {
		t.Fatalf("上架 %s 库存 %d", moved.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
}

func (world receiptWorld) dutyStore(t *testing.T) *adapter.Counts {
	t.Helper()
	duties, err := adapter.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	return duties
}

func (world receiptWorld) allocationApprover(t *testing.T, duties ports.ApprovalDutyStore) *application.JobDiscrepancyHandler {
	t.Helper()
	jobs, err := adapter.NewInternalJobs(world.db)
	if err != nil {
		t.Fatal(err)
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := whidentity.NewPickTaskIdentities()
	if err != nil {
		t.Fatal(err)
	}
	return application.NewJobDiscrepancyHandler(jobs, duties, world.poster(), world.ledger, world.ledgerHolds(t), world.clock, world.warehouses).
		WithAllocationFollow(waves, ids, world.locations, world.zones)
}

type openPick struct {
	id, wave, request, location, inbound string
	quantity                             int64
}

func (world receiptWorld) openPicks(t *testing.T) []openPick {
	t.Helper()
	rows, err := world.pool.Query(t.Context(),
		`SELECT task_id, wave_id, request_id, location_code, inbound_batch_id, quantity
		   FROM warehouse_fulfillment.pick_task
		  WHERE tenant_id = $1 AND status = 'OPEN'
		  ORDER BY location_code, task_id`, "SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var picks []openPick
	for rows.Next() {
		var pick openPick
		if err := rows.Scan(&pick.id, &pick.wave, &pick.request, &pick.location, &pick.inbound, &pick.quantity); err != nil {
			t.Fatal(err)
		}
		picks = append(picks, pick)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return picks
}
