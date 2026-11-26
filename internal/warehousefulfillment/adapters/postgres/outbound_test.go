package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: 可用量够就整份预占并取单据号，不改库存。同一键原样重投已有结果，换数量是内容冲突。
// 缺货处置没登记时停在待预占；登记部分预占后只占还剩的；再改成拒绝则不再留预占。停用的仓照常受理。
func TestOutboundRequestReservesWithoutMovingStock(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 2, func(spec *domain.WarehouseContentSpec) {
		spec.Deactivated = true
	}))
	handler := world.outbound(t)
	first := world.acceptOutbound(t, handler, outboundCommand(t, "REQ-OB-1", 6))
	request, ok := first.Request()
	number, numbered := request.DocumentNumber()
	if first.Outcome() != application.OutboundReserved || !ok || !numbered || number.String() != "OB-20261120-00001" ||
		world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 || reservedQuantity(t, world) != 6 {
		t.Fatalf("预占 %s 号 %s 库存 %d 占用 %d", first.Outcome(), number.String(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), reservedQuantity(t, world))
	}
	again := world.acceptOutbound(t, handler, outboundCommand(t, "REQ-OB-1", 6))
	if again.Outcome() != application.OutboundExisting || reservedQuantity(t, world) != 6 {
		t.Fatalf("重投 %s 占用 %d", again.Outcome(), reservedQuantity(t, world))
	}
	conflict := world.acceptOutbound(t, handler, outboundCommand(t, "REQ-OB-1", 7))
	if conflict.Outcome() != application.OutboundConflict || reservedQuantity(t, world) != 6 {
		t.Fatalf("冲突 %s 占用 %d", conflict.Outcome(), reservedQuantity(t, world))
	}
	pending := world.acceptOutbound(t, handler, outboundCommand(t, "REQ-OB-2", 6))
	if pending.Outcome() != application.OutboundPendingPolicy || reservedQuantity(t, world) != 6 {
		t.Fatalf("没规则 %s 占用 %d", pending.Outcome(), reservedQuantity(t, world))
	}
	if world.registerShortage(t, handler, 1, domain.ShortagePolicyPartial).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("部分预占没有登记")
	}
	partial := world.acceptOutbound(t, handler, outboundCommand(t, "REQ-OB-3", 6))
	partialRequest, _ := partial.Request()
	if partial.Outcome() != application.OutboundPartiallyReserved || len(partialRequest.Reservations()) != 1 ||
		partialRequest.Reservations()[0].Quantity != 4 || partialRequest.Reservations()[0].Wanted != 6 || reservedQuantity(t, world) != 10 {
		t.Fatalf("部分预占 %s %+v 占用 %d", partial.Outcome(), partialRequest.Reservations(), reservedQuantity(t, world))
	}
	if world.registerShortage(t, handler, 2, domain.ShortagePolicyReject).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("拒绝没有登记")
	}
	rejected := world.acceptOutbound(t, handler, outboundCommand(t, "REQ-OB-4", 1))
	if rejected.Outcome() != application.OutboundRejected || reservedQuantity(t, world) != 10 || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 {
		t.Fatalf("拒绝 %s 占用 %d 库存 %d", rejected.Outcome(), reservedQuantity(t, world), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
}

// Covers: 预占挡住库内作业把可分配量占穿。占了 6，再移 10，只入账剩下的 4。
func TestAnUnassignedReservationKeepsStockFromBeingMovedAway(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	if world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-OB-HOLD", 6)).Outcome() != application.OutboundReserved {
		t.Fatal("没有预占")
	}
	freezes, err := adapter.NewFreezes(world.db)
	if err != nil {
		t.Fatal(err)
	}
	counts, err := adapter.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	holds := adapter.LedgerHolds{Freezes: adapter.NewFreezeHolds(freezes), Counts: counts, Reservations: requests}
	moved := world.registerJob(t, world.internalJobsHeld(t, holds), moveCommand(t, "SYN-FACT-OB-HOLD", 10, "SYN-LOC-STORE", "SYN-LOC-RCV"))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 6 {
		t.Fatalf("移库 %s 存储 %d", moved.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
}

// Covers: 按组件出的组合商品在受理时展开，按组件预占。
func TestAComponentKitReservesItsComponents(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-KIT-01", 1, "REQ-KIT-OB", func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "合成套装")
		spec.Aliases = nil
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundByComponent
		spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 2)}
	}))
	command := outboundCommand(t, "REQ-OB-KIT", 3)
	command.Lines = []domain.OutboundLine{mustOutboundLine(t, "SYN-KIT-01", 3)}
	accepted := world.acceptOutbound(t, world.outbound(t), command)
	request, _ := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || len(request.Reservations()) != 1 ||
		request.Reservations()[0].Goods.String() != "SYN-GOODS-01" || request.Reservations()[0].Quantity != 6 {
		t.Fatalf("展开 %s %+v", accepted.Outcome(), request.Reservations())
	}
}

// Covers: 代理仓进入已转代理仓，不在自营库存上预占。没登记的仓不落册。
func TestAnAgentWarehouseIsHandedOffAndAMissingWarehouseIsNotSaved(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-AGENT", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Kind = domain.WarehouseKindAgent
	}))
	handler := world.outbound(t)
	command := outboundCommand(t, "REQ-OB-AGENT", 1)
	command.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT")
	handed := world.acceptOutbound(t, handler, command)
	if handed.Outcome() != application.OutboundHandedOff || reservedQuantity(t, world) != 0 {
		t.Fatalf("代理仓 %s 占用 %d", handed.Outcome(), reservedQuantity(t, world))
	}
	missing := outboundCommand(t, "REQ-OB-MISSING", 1)
	missing.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-MISSING")
	if world.acceptOutbound(t, handler, missing).Outcome() != application.OutboundWarehouseNotFound || reservedQuantity(t, world) != 0 {
		t.Fatal("没登记的仓不该落册")
	}
}

// Covers: 尚未分配的请求拆成两份，数量合起来才成立。原请求已拆单并放开预占，两份各自再占。合起来不等、已经拒绝的都不改原请求。
func TestSplittingAnOutboundRequestReleasesTheOriginalReservation(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	handler := world.outbound(t)
	accepted := world.acceptOutbound(t, handler, outboundCommand(t, "REQ-OB-WHOLE", 10))
	parent, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("受理 %s", accepted.Outcome())
	}
	unequal := world.splitOutbound(t, handler, parent.ID(), 6, 3)
	if unequal.Outcome() != application.OutboundContentInvalid || parentStatus(t, world, parent.ID()) != "RESERVED" || reservedQuantity(t, world) != 10 {
		t.Fatalf("不合 %s 状态 %s 占用 %d", unequal.Outcome(), parentStatus(t, world, parent.ID()), reservedQuantity(t, world))
	}
	split := world.splitOutbound(t, handler, parent.ID(), 6, 4)
	children := split.Children()
	if split.Outcome() != application.OutboundSplitResult || len(children) != 2 || parentStatus(t, world, parent.ID()) != "SPLIT" || reservedQuantity(t, world) != 10 {
		t.Fatalf("拆单 %s 孩子 %d 状态 %s 占用 %d", split.Outcome(), len(children), parentStatus(t, world, parent.ID()), reservedQuantity(t, world))
	}
	if children[0].Reservations()[0].Quantity+children[1].Reservations()[0].Quantity != 10 {
		t.Fatalf("两份预占 %+v %+v", children[0].Reservations(), children[1].Reservations())
	}
	again := world.splitOutbound(t, handler, parent.ID(), 6, 4)
	if again.Outcome() != application.OutboundExisting {
		t.Fatalf("重放 %s", again.Outcome())
	}
	other := world.splitOutbound(t, handler, parent.ID(), 5, 5)
	if other.Outcome() != application.OutboundStatusMismatch || reservedQuantity(t, world) != 10 {
		t.Fatalf("再拆 %s 占用 %d", other.Outcome(), reservedQuantity(t, world))
	}
	if world.registerShortage(t, handler, 1, domain.ShortagePolicyReject).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("拒绝没有登记")
	}
	rejected := world.acceptOutbound(t, handler, outboundCommand(t, "REQ-OB-NO", 1))
	rejectedRequest, _ := rejected.Request()
	if rejected.Outcome() != application.OutboundRejected {
		t.Fatalf("拒绝 %s", rejected.Outcome())
	}
	blocked := world.splitOutbound(t, handler, rejectedRequest.ID(), 1, 0)
	if blocked.Outcome() != application.OutboundContentInvalid && blocked.Outcome() != application.OutboundStatusMismatch {
		t.Fatalf("拒绝的请求 %s", blocked.Outcome())
	}
	if parentStatus(t, world, rejectedRequest.ID()) != "REJECTED" {
		t.Fatal("被拒的请求被改了")
	}
}

// Covers: 汇总的可用量减掉未分配预占。预占不落库位，格上的可用量不减。占用读口看得见这份预占。
func TestStockReadSubtractsTheUnassignedReservation(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	if world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-OB-READ", 6)).Outcome() != application.OutboundReserved {
		t.Fatal("没有预占")
	}
	reader := world.stockReaderWithReservations(t)
	summary := world.readStock(t, reader, application.StockViewSummary, false)
	if len(summary.Summary) != 1 || summary.Summary[0].Reserved != 6 || summary.Summary[0].Available != 4 || summary.Summary[0].OnHand != 10 {
		t.Fatalf("汇总 %+v", summary.Summary)
	}
	cells := world.readStock(t, reader, application.StockViewCells, false)
	store := cellNamed(t, cells.Cells, "SYN-LOC-STORE")
	if store.Available != 10 || store.Quantity != 10 {
		t.Fatalf("格 %+v", store)
	}
	holds := world.readStock(t, reader, application.StockViewHolds, true)
	if len(holds.Holds) != 1 || holds.Holds[0].Kind != "RESERVATION" || holds.Holds[0].Quantity != 6 || holds.Holds[0].Location != "" {
		t.Fatalf("占用 %+v", holds.Holds)
	}
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	pageQuery, err := ports.OutboundRequestPages.DecodeRaw("status=RESERVED")
	if err != nil {
		t.Fatal(err)
	}
	listed, err := requests.List(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), 20, pageQuery)
	if err != nil || len(listed.Rows) != 1 || listed.Rows[0].ReservedQuantity != 6 || len(listed.Rows[0].Lines) != 1 {
		t.Fatalf("出库列表 %+v err %v", listed.Rows, err)
	}
	_ = cataloguepage.Page{}
}

func TestOutboundWritesRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	numbers := world.outboundNumbers(t)
	ctx := context.Background()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	if err := requests.Save(ctx, domain.OutboundRequest{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Save：%v", err)
	}
	if _, err := requests.ReleaseForSplit(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("ReleaseForSplit：%v", err)
	}
	if err := requests.SavePolicy(ctx, tenant, valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), ports.StoredShortagePolicy{Revision: 1, Kind: domain.ShortagePolicyPartial}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SavePolicy：%v", err)
	}
	if _, err := numbers.IssueOutboundNumber(ctx, tenant, domain.BusinessTimeZone{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("IssueOutboundNumber：%v", err)
	}
}

func (world receiptWorld) seedStorageStock(t *testing.T, quantity int64) {
	t.Helper()
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-OB-RCV", "SYN-GOODS-01", "件", quantity)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-OB-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: quantity,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	moved := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-OB-PUT", quantity, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != quantity {
		t.Fatalf("上架 %s 库存 %d", moved.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
}

func (world receiptWorld) outbound(t *testing.T) *application.OutboundHandler {
	t.Helper()
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := whidentity.NewOutboundIdentities()
	if err != nil {
		t.Fatal(err)
	}
	picks, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	return application.NewOutboundHandler(
		requests, identities, world.outboundNumbers(t), world.ledger, world.warehouses, world.locations, world.zones,
		world.goods, application.ZeroStockHolds{}, alwaysOwner{}, world.clock, world.warehouses,
	).WithShortPicks(picks)
}

func (world receiptWorld) outboundNumbers(t *testing.T) *adapter.OutboundNumbers {
	t.Helper()
	catalogue, err := docnumber.NewCatalogue(domain.DocumentNumberTypes())
	if err != nil {
		t.Fatal(err)
	}
	book, err := docnumberpg.NewStore(world.db)
	if err != nil {
		t.Fatal(err)
	}
	sequences, err := docnumberpg.NewSequences(world.db)
	if err != nil {
		t.Fatal(err)
	}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: book, Sequences: sequences, Transactions: book, Secrets: docnumber.SecretTable{}, Clock: world.clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	numbers, err := adapter.NewOutboundNumbers(world.db, assigner)
	if err != nil {
		t.Fatal(err)
	}
	return numbers
}

func (world receiptWorld) acceptOutbound(t *testing.T, handler *application.OutboundHandler, command application.AcceptOutboundCommand) application.OutboundResult {
	t.Helper()
	var result application.OutboundResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var acceptErr error
		result, acceptErr = handler.Accept(ctx, command)
		return acceptErr
	})
	if err != nil {
		t.Fatalf("受理出库：%v", err)
	}
	return result
}

func (world receiptWorld) registerShortage(t *testing.T, handler *application.OutboundHandler, revision int, kind domain.ShortagePolicyKind) application.OutboundResult {
	t.Helper()
	var result application.OutboundResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var registerErr error
		result, registerErr = handler.RegisterPolicy(ctx, application.RegisterShortagePolicyCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
			Revision: revision, Kind: kind, Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		})
		return registerErr
	})
	if err != nil {
		t.Fatalf("缺货处置：%v", err)
	}
	return result
}

func (world receiptWorld) splitOutbound(t *testing.T, handler *application.OutboundHandler, parent domain.OutboundRequestID, left, right int64) application.SplitResult {
	t.Helper()
	parts := []application.SplitOutboundPart{{
		Key: valueOf(t, domain.NewRequestKey, parent.String()+"-A"), Lines: []domain.OutboundLine{mustOutboundLine(t, "SYN-GOODS-01", left)},
	}}
	if right > 0 {
		parts = append(parts, application.SplitOutboundPart{
			Key: valueOf(t, domain.NewRequestKey, parent.String()+"-B"), Lines: []domain.OutboundLine{mustOutboundLine(t, "SYN-GOODS-01", right)},
		})
	}
	var result application.SplitResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var splitErr error
		result, splitErr = handler.Split(ctx, application.SplitOutboundCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
			Origin: domain.OutboundOriginCustomer, Parent: parent, Parts: parts,
		})
		return splitErr
	})
	if err != nil {
		t.Fatalf("拆单：%v", err)
	}
	return result
}

func (world receiptWorld) stockReaderWithReservations(t *testing.T) *application.QueryStockHandler {
	t.Helper()
	reads, err := adapter.NewStockReads(world.db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	return application.NewQueryStockHandler(reads, world.warehouses, world.warehouses, world.locations, world.zones, requests, world.goods)
}

func parentStatus(t *testing.T, world receiptWorld, id domain.OutboundRequestID) string {
	t.Helper()
	var status string
	err := world.pool.QueryRow(t.Context(),
		`SELECT status FROM warehouse_fulfillment.outbound_request WHERE tenant_id = $1 AND request_id = $2`,
		"SYN-TENANT-01", id.String()).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func outboundCommand(t *testing.T, key string, quantity int64) application.AcceptOutboundCommand {
	t.Helper()
	return application.AcceptOutboundCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Origin: domain.OutboundOriginCustomer,
		Key: valueOf(t, domain.NewRequestKey, key), Dispatch: domain.DispatchProductNetwork, Service: "SYN-SERVICE",
		Recipient: "合成收件人", Address: "合成地址", Lines: []domain.OutboundLine{mustOutboundLine(t, "SYN-GOODS-01", quantity)},
	}
}

func mustOutboundLine(t *testing.T, goods string, quantity int64) domain.OutboundLine {
	t.Helper()
	line, err := domain.NewOutboundLine(domain.OutboundLineSpec{
		Goods: valueOf(t, domain.NewGoodsCode, goods), Unit: valueOf(t, domain.NewGoodsUnitName, "件"),
		State: domain.StockStateGood, Quantity: quantity,
	})
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func reservedQuantity(t *testing.T, world receiptWorld) int64 {
	t.Helper()
	var quantity int64
	err := world.pool.QueryRow(t.Context(),
		`SELECT COALESCE(SUM(quantity), 0) FROM warehouse_fulfillment.outbound_reservation WHERE tenant_id = $1`,
		"SYN-TENANT-01").Scan(&quantity)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}
