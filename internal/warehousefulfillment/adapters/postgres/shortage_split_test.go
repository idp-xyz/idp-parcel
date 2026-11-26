package postgres_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (world receiptWorld) splitShortage(t *testing.T, handler *application.OutboundHandler, request domain.OutboundRequestID) application.SplitResult {
	t.Helper()
	var result application.SplitResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var splitErr error
		result, splitErr = handler.SplitShortage(ctx, application.SplitShortageCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Request: request,
			Operator: valueOf(t, domain.NewRegistrant, "operator-2"),
		})
		return splitErr
	})
	if err != nil {
		t.Fatalf("拆出缺货部分：%v", err)
	}
	return result
}

func (world receiptWorld) reservedOf(t *testing.T, request domain.OutboundRequestID) int64 {
	t.Helper()
	var quantity int64
	if err := world.pool.QueryRow(t.Context(),
		`SELECT COALESCE(SUM(quantity), 0) FROM warehouse_fulfillment.outbound_reservation
		  WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1`, request.String()).Scan(&quantity); err != nil {
		t.Fatal(err)
	}
	return quantity
}

// Covers: 拆出缺货部分只在货主许了拆单发时成立。规则没写明答部分发货未配置，许的是少发答不许，请求与预占都不动。
// 许了之后原请求没拣到的预占放开、进已拣货，按已拣数量复核；没拣到的 4 件另成一张请求，拆自原请求、由办的人代录、
// 重新预占，货主对原请求说过的规则跟过去。原样再办答已有结果，交回的还是那一张。办过之后不能再按已拣数量发货。
func TestAShortPickedRequestSplitsOffItsShortageOnlyWhenTheOwnerAllowsSplitShipment(t *testing.T) {
	world, handler, request := stuckShort(t, "REQ-SHORT-SPLIT", "WAVE-SHORT-SPLIT")
	outbound := world.outbound(t)
	untouched := func(step string) {
		t.Helper()
		if world.requestStatus(t, request) != "RESERVED" || reservedQuantity(t, world) != 4 {
			t.Fatalf("%s之后请求 %s，预占 %d", step, world.requestStatus(t, request), reservedQuantity(t, world))
		}
	}
	if got := world.splitShortage(t, outbound, request).Outcome(); got != application.OutboundPartialShipmentNotConfigured {
		t.Fatalf("规则没写明 = %s", got)
	}
	untouched("未配置")
	if world.registerShortageRules(t, outbound, 1, domain.ShortagePolicyPartial, domain.PartialShipmentShortShip).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("货主默认没有登记")
	}
	if got := world.splitShortage(t, outbound, request).Outcome(); got != application.OutboundPartialShipmentNotAllowed {
		t.Fatalf("货主许的是少发 = %s", got)
	}
	untouched("少发")
	if world.recordConsent(t, outbound, request, domain.PartialShipmentSplitShip, "SYN-CONSENT-31").Outcome() != application.OutboundConsentRecorded {
		t.Fatal("拆单发的同意没有记下")
	}
	split := world.splitShortage(t, outbound, request)
	child, ok := split.Remainder()
	if split.Outcome() != application.OutboundRemainderSplit || !ok {
		t.Fatalf("货主许了拆单发 = %s", split.Outcome())
	}
	if world.requestStatus(t, request) != "PICKED" || world.reservedOf(t, request) != 0 {
		t.Fatalf("原请求 %s，预占 %d", world.requestStatus(t, request), world.reservedOf(t, request))
	}
	parent := child.Parent()
	if child.Status() != domain.OutboundReserved || parent != request || child.Origin() != domain.OutboundOriginOperator ||
		child.Operator() != "operator-2" || child.PartialShipment() != domain.PartialShipmentSplitShip ||
		len(child.Lines()) != 1 || child.Lines()[0].Quantity() != 4 || child.Lines()[0].Goods().String() != "SYN-GOODS-01" ||
		child.Lines()[0].Unit().String() != "件" || world.reservedOf(t, child.ID()) != 4 {
		t.Fatalf("拆出的请求 %s，拆自 %s，行 %+v，预占 %d", child.Status(), parent, child.Lines(), world.reservedOf(t, child.ID()))
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	disposal, found, err := waves.LoadShortPickDisposal(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), request)
	if err != nil || !found || disposal.Rule != "SPLIT_SHIP" || disposal.Source != "ORDER_CONSENT" || disposal.Wanted != 10 || disposal.Shipped != 6 ||
		disposal.Remainder != child.ID().String() || disposal.Operator != "operator-2" {
		t.Fatalf("记下的处置 %+v found %v err %v", disposal, found, err)
	}
	again := world.splitShortage(t, outbound, request)
	same, ok := again.Remainder()
	if again.Outcome() != application.OutboundExisting || !ok || same.ID() != child.ID() {
		t.Fatalf("原样再办 = %s", again.Outcome())
	}
	if got := world.shipPicked(t, handler, request).Outcome(); got != application.WaveStatusMismatch {
		t.Fatalf("拆过之后再按已拣数量发货 = %s", got)
	}
	if got := world.checkOutbound(t, handler, request, "SYN-CHECK-SHORT-SPLIT", 6).Outcome(); got != application.WaveChecked {
		t.Fatalf("原请求按已拣数量复核 = %s", got)
	}
}

// Covers: 拆出的行按基本单位写，按组件出的套装写成缺的那种组件——3 套各 2 件拣到 4 件，缺的是 2 件组件，不是「1 套」。
// 货主的默认是拆单发、这张请求没另写：拆出的请求上不抄货主的默认。
func TestTheShortageIsSplitOffInBaseUnitsOfWhatWasNotPicked(t *testing.T) {
	world, _, outbound, kit := stuckKit(t, domain.PartialShipmentSplitShip)
	split := world.splitShortage(t, outbound, kit)
	child, ok := split.Remainder()
	if split.Outcome() != application.OutboundRemainderSplit || !ok {
		t.Fatalf("拆出 = %s", split.Outcome())
	}
	if len(child.Lines()) != 1 || child.Lines()[0].Goods().String() != "SYN-GOODS-01" || child.Lines()[0].Unit().String() != "件" ||
		child.Lines()[0].Quantity() != 2 || child.PartialShipment().Given() || world.reservedOf(t, child.ID()) != 2 {
		t.Fatalf("拆出的行 %+v，请求上的规则 %q，预占 %d", child.Lines(), child.PartialShipment(), world.reservedOf(t, child.ID()))
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	disposal, found, err := waves.LoadShortPickDisposal(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), kit)
	if err != nil || !found || disposal.Source != "OWNER" || disposal.Wanted != 6 || disposal.Shipped != 4 {
		t.Fatalf("记下的处置 %+v found %v err %v", disposal, found, err)
	}
}

// Covers: 已有货拣进待出库的请求不走代录拆单——拆开会把原请求整张作废，拣出来的货就没了归属。
// 预取过估算委托的请求也不拆出缺货部分。两种都不动请求与预占。
func TestARequestWithPickedGoodsIsNotSplitTheOldWayAndAPrefetchedOneIsNotSplitAtAll(t *testing.T) {
	world, _, request := stuckShort(t, "REQ-SHORT-OLD-SPLIT", "WAVE-SHORT-OLD-SPLIT")
	outbound := world.outbound(t)
	if world.registerShortageRules(t, outbound, 1, domain.ShortagePolicyPartial, domain.PartialShipmentSplitShip).Outcome() != application.OutboundPolicyRegistered {
		t.Fatal("货主默认没有登记")
	}
	if got := world.splitOutbound(t, outbound, request, 6, 4).Outcome(); got != application.OutboundStatusMismatch {
		t.Fatalf("已有货拣出来的请求走代录拆单 = %s", got)
	}
	if world.requestStatus(t, request) != "RESERVED" || world.reservedOf(t, request) != 4 {
		t.Fatalf("不拆之后请求 %s，预占 %d", world.requestStatus(t, request), world.reservedOf(t, request))
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
	if got := world.splitShortage(t, outbound, request).Outcome(); got != application.OutboundPrefetchStands {
		t.Fatalf("预取过的请求 = %s", got)
	}
	if world.requestStatus(t, request) != "RESERVED" || world.reservedOf(t, request) != 4 {
		t.Fatalf("不办之后请求 %s，预占 %d", world.requestStatus(t, request), world.reservedOf(t, request))
	}
}
