package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (world receiptWorld) shipPicked(t *testing.T, handler *application.WaveHandler, request domain.OutboundRequestID) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var shipErr error
		result, shipErr = handler.ShipPicked(ctx, application.ShipPickedCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Request: request,
			Operator: valueOf(t, domain.NewRegistrant, "operator-2"),
		})
		return shipErr
	})
	if err != nil {
		t.Fatalf("按已拣数量发货：%v", err)
	}
	return result
}

// stuckShort 摆出拣不齐、换不出别的库位的一张请求：要 10 件，两条任务一条少拣、一条拣满，共拣到 6 件，
// 少的 4 件还在预占里，请求回到已预占。
func stuckShort(t *testing.T, key, wave string) (receiptWorld, *application.WaveHandler, domain.OutboundRequestID) {
	t.Helper()
	world, handler, reserve := twoCellWorld(t)
	request := reserve(key, 10)
	released := world.releaseRequests(t, handler, wave, request)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放出 %s，任务 %d 条", released.Outcome(), len(released.Tasks()))
	}
	if world.pick(t, handler, taskAt(t, released.Tasks(), "SYN-LOC-STORE").ID, 2).Outcome() != application.WavePicked ||
		world.pick(t, handler, taskAt(t, released.Tasks(), "SYN-LOC-STORE-2").ID, 4).Outcome() != application.WavePicked {
		t.Fatal("两条任务没有拣成")
	}
	if world.requestStatus(t, request) != "RESERVED" || reservedQuantity(t, world) != 4 {
		t.Fatalf("拣不齐之后请求 %s，预占 %d", world.requestStatus(t, request), reservedQuantity(t, world))
	}
	return world, handler, request
}

// Covers: 按已拣数量发货只在货主许了少发时成立。规则没写明答部分发货未配置，写的是整单发或拆单发答不许少发，
// 这几种请求与预占都不动。许了之后没拣到的预占放开、请求进已拣货，记下照的是哪一层的规则、要发与实发；
// 复核对的是已拣数量。原样再办答已有结果。
func TestAShortPickedRequestShipsWhatWasPickedOnlyWhenTheOwnerAllowsShortShipment(t *testing.T) {
	world, handler, request := stuckShort(t, "REQ-SHORT-SHIP", "WAVE-SHORT-SHIP")
	outbound := world.outbound(t)
	untouched := func(step string) {
		t.Helper()
		if world.requestStatus(t, request) != "RESERVED" || reservedQuantity(t, world) != 4 {
			t.Fatalf("%s之后请求 %s，预占 %d", step, world.requestStatus(t, request), reservedQuantity(t, world))
		}
	}
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WavePartialShipmentNotConfigured {
		t.Fatalf("规则没写明 = %s", got)
	}
	untouched("未配置")
	if world.registerShortageRules(t, outbound, 1, domain.ShortagePolicyPartial, domain.PartialShipmentShipComplete).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("货主默认没有登记")
	}
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WavePartialShipmentNotAllowed {
		t.Fatalf("货主默认整单发 = %s", got)
	}
	untouched("整单发")
	if world.recordConsent(t, outbound, request, domain.PartialShipmentSplitShip, "SYN-CONSENT-21").Outcome() != application.OutboundConsentRecorded {
		t.Fatal("拆单发的同意没有记下")
	}
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WavePartialShipmentNotAllowed {
		t.Fatalf("货主许的是拆单发 = %s", got)
	}
	untouched("拆单发")

	world.clock.at = world.clock.at.Add(time.Hour)
	if world.recordConsent(t, outbound, request, domain.PartialShipmentShortShip, "SYN-CONSENT-22").Outcome() != application.OutboundConsentRecorded {
		t.Fatal("少发的同意没有记下")
	}
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WaveShortShipped {
		t.Fatalf("货主许了少发 = %s", got)
	}
	if world.requestStatus(t, request) != "PICKED" || reservedQuantity(t, world) != 0 {
		t.Fatalf("少发之后请求 %s，预占 %d", world.requestStatus(t, request), reservedQuantity(t, world))
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	disposal, found, err := waves.LoadShortPickDisposal(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), request)
	if err != nil || !found || disposal.Rule != "SHORT_SHIP" || disposal.Source != "ORDER_CONSENT" || disposal.Wanted != 10 || disposal.Shipped != 6 ||
		disposal.Remainder != "" || disposal.Operator != "operator-2" || disposal.RecordedAt.IsZero() {
		t.Fatalf("记下的处置 %+v found %v err %v", disposal, found, err)
	}
	reads, err := adapter.NewWorkspaceReads(world.db)
	if err != nil {
		t.Fatal(err)
	}
	detail, found, err := reads.OutboundRequestDetail(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), request.String())
	if err != nil || !found || detail.ShortPickDisposal == nil || detail.ShortPickDisposal.Rule != "SHORT_SHIP" ||
		detail.ShortPickDisposal.Wanted != 10 || detail.ShortPickDisposal.Shipped != 6 || detail.ShortPickDisposal.RemainderRequest != "" {
		t.Fatalf("明细读口的处置 %+v found %v err %v", detail.ShortPickDisposal, found, err)
	}
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WaveExisting {
		t.Fatalf("原样再办 = %s", got)
	}
	if got := world.checkOutbound(t, handler, request, "SYN-CHECK-SHORT-SHIP-BAD", 10).Outcome(); got != application.WaveMismatch {
		t.Fatalf("按要发的数量复核 = %s", got)
	}
	if got := world.checkOutbound(t, handler, request, "SYN-CHECK-SHORT-SHIP", 6).Outcome(); got != application.WaveChecked {
		t.Fatalf("按已拣数量复核 = %s", got)
	}
}

// Covers: 只有停在「拣不齐、等处置」的请求能按已拣数量发货：没有这张请求、还没拣过的已预占请求、
// 还有任务等着拣的请求都不办——货主许了少发也不办。
func TestShippingWhatWasPickedNeedsARequestStuckOnAShortPick(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	outbound := world.outbound(t)
	if world.registerShortageRules(t, outbound, 1, domain.ShortagePolicyPartial, domain.PartialShipmentShortShip).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("货主默认没有登记")
	}
	if got := world.shipPicked(t, handler, valueOf(t, domain.NewOutboundRequestID, "OBR-NOT-THERE")).Outcome(); got != application.WaveNotFound {
		t.Fatalf("没有的请求 = %s", got)
	}
	request := reserve("REQ-SHORT-GUARD", 10)
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WaveStatusMismatch {
		t.Fatalf("还没放过波次 = %s", got)
	}
	released := world.releaseRequests(t, handler, "WAVE-SHORT-GUARD", request)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放出 %s", released.Outcome())
	}
	if world.pick(t, handler, taskAt(t, released.Tasks(), "SYN-LOC-STORE").ID, 2).Outcome() != application.WavePicked {
		t.Fatal("头一条没有少拣成")
	}
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WaveStatusMismatch {
		t.Fatalf("还有一条任务等着拣 = %s", got)
	}
	if world.requestStatus(t, request) != "ALLOCATED" || reservedQuantity(t, world) != 8 {
		t.Fatalf("不办之后请求 %s，预占 %d", world.requestStatus(t, request), reservedQuantity(t, world))
	}
	if world.pick(t, handler, taskAt(t, released.Tasks(), "SYN-LOC-STORE-2").ID, 4).Outcome() != application.WavePicked {
		t.Fatal("第二条没有拣成")
	}
	// 货主的默认是少发、这张请求没另写：照的是货主默认那一层。
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WaveShortShipped {
		t.Fatalf("任务都有了结果 = %s", got)
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	disposal, found, err := waves.LoadShortPickDisposal(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), request)
	if err != nil || !found || disposal.Source != "OWNER" || disposal.Shipped != 6 {
		t.Fatalf("记下的处置 %+v found %v err %v", disposal, found, err)
	}
}

// stuckKit 摆出一张拣不齐的套装请求：3 套、每套 2 件，按组件出，预占 6 件、拣到 4 件，另一个库位的货被别的请求分走，换不出别的格。
// partial 是货主登记的部分发货默认。
func stuckKit(t *testing.T, partial domain.PartialShipment) (receiptWorld, *application.WaveHandler, *application.OutboundHandler, domain.OutboundRequestID) {
	t.Helper()
	world, handler, _ := twoCellWorld(t)
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-KIT-01", 1, "REQ-KIT-SHORT", func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "合成套装")
		spec.Aliases = nil
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundByComponent
		spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 2)}
	}))
	outbound := world.outbound(t)
	if world.registerShortageRules(t, outbound, 1, domain.ShortagePolicyPartial, partial).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("货主默认没有登记")
	}
	command := outboundCommand(t, "REQ-KIT-SHORT-SHIP", 3)
	command.Lines = []domain.OutboundLine{mustOutboundLine(t, "SYN-KIT-01", 3)}
	accepted := world.acceptOutbound(t, outbound, command)
	kit, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("套装受理 %s", accepted.Outcome())
	}
	released := world.releaseRequests(t, handler, "WAVE-KIT-SHORT", kit.ID())
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 1 || released.Tasks()[0].Quantity != 6 {
		t.Fatalf("放出 %s %+v", released.Outcome(), released.Tasks())
	}
	blocker := world.acceptOutbound(t, outbound, outboundCommand(t, "REQ-KIT-BLOCK", 4))
	blocking, ok := blocker.Request()
	if blocker.Outcome() != application.OutboundReserved || !ok ||
		world.releaseRequests(t, handler, "WAVE-KIT-BLOCK", blocking.ID()).Outcome() != application.WaveAllocated {
		t.Fatalf("占住另一个库位 %s", blocker.Outcome())
	}
	if world.pick(t, handler, released.Tasks()[0].ID, 4).Outcome() != application.WavePicked || world.requestStatus(t, kit.ID()) != "RESERVED" {
		t.Fatalf("少拣之后请求 %s", world.requestStatus(t, kit.ID()))
	}
	return world, handler, outbound, kit.ID()
}

// Covers: 要发与实发都按基本单位记。3 套、每套 2 件的套装按组件出，预占 6 件、拣到 4 件——是要发 6 实发 4，
// 不是拿「3 套」去比「4 件」把它挡在门外。
func TestAShortShipmentCountsWhatWasWantedInBaseUnits(t *testing.T) {
	world, handler, _, kit := stuckKit(t, domain.PartialShipmentShortShip)
	if got := world.shipPicked(t, handler, kit).Outcome(); got != application.WaveShortShipped {
		t.Fatalf("3 套拣到 4 件 = %s", got)
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	disposal, found, err := waves.LoadShortPickDisposal(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), kit)
	if err != nil || !found || disposal.Wanted != 6 || disposal.Shipped != 4 {
		t.Fatalf("记下的处置 %+v found %v err %v", disposal, found, err)
	}
}

// Covers: 放波次时预取过估算委托的请求不办少发——那份委托按全量申报了件数，没有更正的路。货主许了少发也不办，请求与预占不动。
func TestAPrefetchedRequestIsNotShippedShort(t *testing.T) {
	world, handler, request := stuckShort(t, "REQ-SHORT-PREFETCH", "WAVE-SHORT-PREFETCH")
	if world.registerShortageRules(t, world.outbound(t), 1, domain.ShortagePolicyPartial, domain.PartialShipmentShortShip).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("货主默认没有登记")
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		return waves.SavePrefetch(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), request, ports.PrefetchRecord{
			ShipmentID: "SYN-SHIPMENT-PREFETCH", DeclaredParcelID: "SYN-PARCEL-PREFETCH", Weight: "1.000", WeightUnit: "KG",
			Length: "10", Width: "10", Height: "10", Dimension: "CM",
		})
	}); err != nil {
		t.Fatalf("记预取：%v", err)
	}
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WavePrefetchStands {
		t.Fatalf("预取过的请求 = %s", got)
	}
	if world.requestStatus(t, request) != "RESERVED" || reservedQuantity(t, world) != 4 {
		t.Fatalf("不办之后请求 %s，预占 %d", world.requestStatus(t, request), reservedQuantity(t, world))
	}

	// 留下了申报件数的预取照办少发：打包时按实际装的件数提货物明细修订，这里不再拦。
	if _, err := world.pool.Exec(t.Context(),
		`UPDATE warehouse_fulfillment.outbound_request SET prefetch_quantity = 10 WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1`,
		request.String()); err != nil {
		t.Fatal(err)
	}
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WaveShortShipped {
		t.Fatalf("留下了申报件数的预取请求少发 = %s", got)
	}
	if world.requestStatus(t, request) != "PICKED" || reservedQuantity(t, world) != 0 {
		t.Fatalf("少发之后请求 %s，预占 %d", world.requestStatus(t, request), reservedQuantity(t, world))
	}
}

func TestSavingAShortPickDisposalRefusesToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	err = waves.SaveShortPickDisposal(context.Background(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ports.ShortPickDisposal{
		Request: valueOf(t, domain.NewOutboundRequestID, "OBR-1"), Rule: "SHORT_SHIP", Source: "OWNER", Wanted: 2, Shipped: 1,
		Operator: "operator-2", RecordedAt: preAdviceNumberingInstant,
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveShortPickDisposal：%v", err)
	}
}
