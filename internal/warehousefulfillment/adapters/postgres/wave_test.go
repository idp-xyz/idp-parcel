package postgres_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: 分配策略没采用时放波次不生成拣货任务。采用参考策略后，已预占的数量落到库位，合计不超过预占，库存先不动。
// 拣满则移入待出库，预占不再计入；实拣不足的差额留下，预占只留没拣的。
func TestAWaveAllocatesReservedStockAndAPickMovesItOut(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	outbound := world.outbound(t)
	accepted := world.acceptOutbound(t, outbound, outboundCommand(t, "REQ-WAVE", 10))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	early := world.releaseWave(t, handler, "WAVE-1", request.ID())
	if early.Outcome() != application.WaveNotConfigured || len(early.Tasks()) != 0 || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 {
		t.Fatalf("没策略 %s 任务 %d", early.Outcome(), len(early.Tasks()))
	}
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-1", request.ID())
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 1 || released.Tasks()[0].Quantity != 10 || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 {
		t.Fatalf("分配 %s %+v 库存 %d", released.Outcome(), released.Tasks(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	if again := world.releaseWave(t, handler, "WAVE-1", request.ID()); again.Outcome() != application.WaveExisting {
		t.Fatalf("重放 %s", again.Outcome())
	}
	picked := world.pick(t, handler, released.Tasks()[0].ID, 10)
	if picked.Outcome() != application.WavePicked || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 0 || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 10 || reservedQuantity(t, world) != 0 {
		t.Fatalf("拣货 %s 存储 %d 待出库 %d 占用 %d", picked.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-OUT", "件"), reservedQuantity(t, world))
	}
}

func TestAShortPickKeepsTheUnpickedReservation(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-SHORT-PICK", 10))
	request, _ := accepted.Request()
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-SHORT", request.ID())
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 1 {
		t.Fatalf("分配 %s", released.Outcome())
	}
	picked := world.pick(t, handler, released.Tasks()[0].ID, 7)
	if picked.Outcome() != application.WavePicked || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 7 || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 3 || reservedQuantity(t, world) != 3 || pickDifferences(t, world) != 1 {
		t.Fatalf("短拣 %s 待出库 %d 存储 %d 占用 %d 差额 %d", picked.Outcome(), world.stockUnitAt(t, "SYN-LOC-OUT", "件"), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), reservedQuantity(t, world), pickDifferences(t, world))
	}
}

// Covers: 复核数量相符才进入可打包。不符的不打包。相符后按包裹序号写下装箱记录，还不提交委托。
func TestACheckedPickCanBePackedAndAMismatchCannot(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-PACK", 10))
	request, _ := accepted.Request()
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-PACK", request.ID())
	if world.pick(t, handler, released.Tasks()[0].ID, 10).Outcome() != application.WavePicked {
		t.Fatal("没有拣完")
	}
	mismatch := world.checkOutbound(t, handler, request.ID(), "SYN-FACT-BAD", 9)
	if mismatch.Outcome() != application.WaveMismatch {
		t.Fatalf("不符 %s", mismatch.Outcome())
	}
	refused := world.packOutbound(t, handler, request.ID(), "SYN-FACT-PACK-NO", 6, 4)
	if refused.Outcome() != application.WaveStatusMismatch || parcelCount(t, world) != 0 {
		t.Fatalf("不符仍打包 %s 包裹 %d", refused.Outcome(), parcelCount(t, world))
	}
	checked := world.checkOutbound(t, handler, request.ID(), "SYN-FACT-OK", 10)
	if checked.Outcome() != application.WaveChecked {
		t.Fatalf("复核 %s", checked.Outcome())
	}
	packed := world.packOutbound(t, handler, request.ID(), "SYN-FACT-PACK", 6, 4)
	if packed.Outcome() != application.WavePacked || parcelCount(t, world) != 2 || requestStatus(t, world, request.ID()) != "PACKED" {
		t.Fatalf("打包 %s 包裹 %d 状态 %s", packed.Outcome(), parcelCount(t, world), requestStatus(t, world, request.ID()))
	}
}

// Covers: 本产品网络打包后按（出库请求，包裹序号）提交委托。代办没获准时不记下委托。同一包裹再提交认已有结果。自备面单不提交。
func TestAPackedNetworkParcelSubmitsOneCommissionPerParcel(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-SHIP", 10))
	request, _ := accepted.Request()
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-SHIP", request.ID())
	if world.pick(t, handler, released.Tasks()[0].ID, 10).Outcome() != application.WavePicked {
		t.Fatal("没有拣完")
	}
	if world.checkOutbound(t, handler, request.ID(), "SYN-FACT-SHIP", 10).Outcome() != application.WaveChecked {
		t.Fatal("没有复核")
	}
	if world.packOutbound(t, handler, request.ID(), "SYN-FACT-SHIP-PACK", 6, 4).Outcome() != application.WavePacked {
		t.Fatal("没有打包")
	}
	withheld := &scriptedCommissioner{outcome: ports.CommissionNotConfiguredWord}
	if got := world.submitCommission(t, withheld, request.ID()); got != application.CommissionNotConfigured || withheld.called != 1 || submittedParcels(t, world, request.ID()) != 0 {
		t.Fatalf("未配置 %s 调用 %d 已提交 %d", got, withheld.called, submittedParcels(t, world, request.ID()))
	}
	granted := &scriptedCommissioner{outcome: ports.CommissionSubmittedWord, id: "SHR-SYN"}
	if got := world.submitCommission(t, granted, request.ID()); got != application.CommissionSubmitted || granted.called != 2 || submittedParcels(t, world, request.ID()) != 2 {
		t.Fatalf("提交 %s 调用 %d 已提交 %d", got, granted.called, submittedParcels(t, world, request.ID()))
	}
	again := &scriptedCommissioner{outcome: ports.CommissionSubmittedWord, id: "SHR-AGAIN"}
	if got := world.submitCommission(t, again, request.ID()); got != application.CommissionExisting || again.called != 0 {
		t.Fatalf("重放 %s 调用 %d", got, again.called)
	}
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_request (
		    tenant_id, request_id, owner_account_id, warehouse_code, origin, request_key, dispatch,
		    recipient, address, status, numbering_absence, received_at)
		 VALUES ('SYN-TENANT-01','OBR-LABEL','SYN-OWNER-01','SYN-WH-01','CUSTOMER','REQ-LABEL','OWNER_LABEL',
		         '收件人','地址','PACKED','NUMBERING_TIME_ZONE_NOT_CONFIGURED', now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_request_line (
		    tenant_id, request_id, line_index, goods_code, unit_name, stock_state, quantity)
		 VALUES ('SYN-TENANT-01','OBR-LABEL',0,'SYN-GOODS-01','件','GOOD',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_parcel (
		    tenant_id, request_id, parcel_sequence, packaging_material, weight_value, weight_unit,
		    length_value, width_value, height_value, dimension_unit)
		 VALUES ('SYN-TENANT-01','OBR-LABEL',1,'SYN-PACK','1','KG','1','1','1','CM')`); err != nil {
		t.Fatal(err)
	}
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_parcel_line (
		    tenant_id, request_id, parcel_sequence, line_index, goods_code, unit_name, quantity)
		 VALUES ('SYN-TENANT-01','OBR-LABEL',1,0,'SYN-GOODS-01','件',1)`); err != nil {
		t.Fatal(err)
	}
	skippedCaller := &scriptedCommissioner{outcome: ports.CommissionSubmittedWord, id: "SHR-NO"}
	if got := world.submitCommission(t, skippedCaller, valueOf(t, domain.NewOutboundRequestID, "OBR-LABEL")); got != application.CommissionSkipped || skippedCaller.called != 0 {
		t.Fatalf("自备面单 %s 调用 %d", got, skippedCaller.called)
	}
}

// Covers: 接受决定信封推进装箱记录。来源不是仓配出库的入账收工，包裹不动。同一决定再投答已有结果。
func TestAnAcceptanceEnvelopeAdvancesTheMatchingParcel(t *testing.T) {
	world := newReceiptWorld(t)
	insertPackedParcel(t, world, "OBR-ACC", "PRODUCT_NETWORK", "SYN-SERVICE", "SHR-ACC", 1)
	handler := acceptanceHandler(t, world)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	accepted := world.recordAcceptance(t, handler, application.AcceptanceCommand{
		Tenant: tenant, DecisionID: "ADEC-1", Source: domain.WarehouseOutboundSource, SourceRequestKey: "OBR-ACC/1", State: "ACCEPTED",
	})
	if accepted != application.AcceptanceAccepted || commissionState(t, world, "OBR-ACC", 1) != "ACCEPTED" {
		t.Fatalf("接受 %s 状态 %s", accepted, commissionState(t, world, "OBR-ACC", 1))
	}
	again := world.recordAcceptance(t, handler, application.AcceptanceCommand{
		Tenant: tenant, DecisionID: "ADEC-1", Source: domain.WarehouseOutboundSource, SourceRequestKey: "OBR-ACC/1", State: "ACCEPTED",
	})
	if again != application.AcceptanceExisting {
		t.Fatalf("再投 %s", again)
	}
	other := world.recordAcceptance(t, handler, application.AcceptanceCommand{
		Tenant: tenant, DecisionID: "ADEC-2", Source: "CUSTOMER_CHANNEL", SourceRequestKey: "REQ-1", State: "REJECTED",
	})
	if other != application.AcceptancePosted || commissionState(t, world, "OBR-ACC", 1) != "ACCEPTED" {
		t.Fatalf("别的来源 %s 状态 %s", other, commissionState(t, world, "OBR-ACC", 1))
	}
	insertPackedParcel(t, world, "OBR-REJ", "PRODUCT_NETWORK", "SYN-SERVICE", "SHR-REJ", 1)
	rejected := world.recordAcceptance(t, handler, application.AcceptanceCommand{
		Tenant: tenant, DecisionID: "ADEC-3", Source: domain.WarehouseOutboundSource, SourceRequestKey: "OBR-REJ/1", State: "REJECTED",
	})
	if rejected != application.AcceptanceRejected || commissionState(t, world, "OBR-REJ", 1) != "REJECTED" {
		t.Fatalf("拒绝 %s 状态 %s", rejected, commissionState(t, world, "OBR-REJ", 1))
	}
	insertPackedParcel(t, world, "OBR-SUP", "PRODUCT_NETWORK", "SYN-SERVICE", "SHR-SUP", 1)
	supplements := application.NewSupplementHandler(wavesOf(t, world), acceptanceClock{})
	waiting := world.recordSupplement(t, supplements, application.SupplementCommand{
		Tenant: tenant, SubmissionVersion: "SUBV-1", Source: domain.WarehouseOutboundSource, SourceRequestKey: "OBR-SUP/1",
	})
	if waiting != application.SupplementAwaiting || commissionState(t, world, "OBR-SUP", 1) != "AWAITING_SUPPLEMENT" {
		t.Fatalf("待补充 %s 状态 %s", waiting, commissionState(t, world, "OBR-SUP", 1))
	}
	replayed := world.recordSupplement(t, supplements, application.SupplementCommand{
		Tenant: tenant, SubmissionVersion: "SUBV-1", Source: domain.WarehouseOutboundSource, SourceRequestKey: "OBR-SUP/1",
	})
	if replayed != application.SupplementExisting {
		t.Fatalf("再投 %s", replayed)
	}
	posted := world.recordSupplement(t, supplements, application.SupplementCommand{
		Tenant: tenant, SubmissionVersion: "SUBV-2", Source: "CUSTOMER_CHANNEL", SourceRequestKey: "REQ-9",
	})
	if posted != application.SupplementPosted || commissionState(t, world, "OBR-SUP", 1) != "AWAITING_SUPPLEMENT" {
		t.Fatalf("别的来源 %s 状态 %s", posted, commissionState(t, world, "OBR-SUP", 1))
	}
}

func TestWavesRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	if err := waves.SaveStrategy(ctx, tenant, ports.AllocationStrategy{Revision: 1, Operator: "operator-1", RegisteredAt: preAdviceNumberingInstant}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveStrategy：%v", err)
	}
	if err := waves.SaveWave(ctx, tenant, valueOf(t, domain.NewWaveID, "WAVE"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), valueOf(t, domain.NewRegistrant, "operator-1"), false, preAdviceNumberingInstant, nil, nil); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveWave：%v", err)
	}
	if err := waves.SavePick(ctx, tenant, ports.PickTask{ID: valueOf(t, domain.NewPickTaskID, "PKT-1"), Request: valueOf(t, domain.NewOutboundRequestID, "OBR-1")}, domain.OutboundPicked, "", "", 0); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SavePick：%v", err)
	}
	if err := waves.SaveCheck(ctx, tenant, valueOf(t, domain.NewPickFactID, "SYN-FACT-CHK"), valueOf(t, domain.NewOutboundRequestID, "OBR-1"), 10, true, preAdviceNumberingInstant, domain.OutboundChecked); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveCheck：%v", err)
	}
	if err := waves.SavePack(ctx, tenant, valueOf(t, domain.NewPickFactID, "SYN-FACT-PACK"), valueOf(t, domain.NewOutboundRequestID, "OBR-1"), nil, preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SavePack：%v", err)
	}
	if err := waves.MarkShipment(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), 1, "SHR-1"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkShipment：%v", err)
	}
	if err := waves.SaveAcceptance(ctx, tenant, "ADEC-1", domain.WarehouseOutboundSource, "OBR-1/1", "ACCEPTED", preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveAcceptance：%v", err)
	}
	if err := waves.SetCommissionState(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), 1, "ACCEPTED"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SetCommissionState：%v", err)
	}
	if err := waves.SaveSupplement(ctx, tenant, "SUBV-1", domain.WarehouseOutboundSource, "OBR-1/1", preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveSupplement：%v", err)
	}
}

func (world receiptWorld) waves(t *testing.T) *application.WaveHandler {
	t.Helper()
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := whidentity.NewPickTaskIdentities()
	if err != nil {
		t.Fatal(err)
	}
	differences, err := whidentity.NewStockDiscrepancyIdentities()
	if err != nil {
		t.Fatal(err)
	}
	return application.NewWaveHandler(
		waves, requests, tasks, differences, world.poster(), world.ledger, world.locations, world.zones,
		world.goods, application.ZeroStockHolds{}, world.clock, world.warehouses,
	)
}

func (world receiptWorld) adoptStrategy(t *testing.T, handler *application.WaveHandler) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var adoptErr error
		result, adoptErr = handler.Adopt(ctx, application.AdoptStrategyCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Revision: 1, Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		})
		return adoptErr
	})
	if err != nil {
		t.Fatalf("采用策略：%v", err)
	}
	return result
}

func (world receiptWorld) releaseWave(t *testing.T, handler *application.WaveHandler, id string, request domain.OutboundRequestID) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var releaseErr error
		result, releaseErr = handler.Release(ctx, application.ReleaseWaveCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: valueOf(t, domain.NewWaveID, id),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
			Requests: []domain.OutboundRequestID{request},
		})
		return releaseErr
	})
	if err != nil {
		t.Fatalf("放波次：%v", err)
	}
	return result
}

func (world receiptWorld) pick(t *testing.T, handler *application.WaveHandler, task domain.PickTaskID, quantity int64) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var pickErr error
		result, pickErr = handler.Pick(ctx, application.PickCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, "SYN-FACT-"+task.String()),
			TaskID: task, Quantity: quantity, Target: valueOf(t, domain.NewLocationCode, "SYN-LOC-OUT"),
		})
		return pickErr
	})
	if err != nil {
		t.Fatalf("拣货：%v", err)
	}
	return result
}

func (world receiptWorld) checkOutbound(t *testing.T, handler *application.WaveHandler, request domain.OutboundRequestID, fact string, observed int64) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var checkErr error
		result, checkErr = handler.Check(ctx, application.CheckCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, fact),
			Request: request, Observed: observed,
		})
		return checkErr
	})
	if err != nil {
		t.Fatalf("复核：%v", err)
	}
	return result
}

func (world receiptWorld) packOutbound(t *testing.T, handler *application.WaveHandler, request domain.OutboundRequestID, fact string, left, right int64) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var packErr error
		result, packErr = handler.Pack(ctx, application.PackCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, fact), Request: request,
			Parcels: []application.PackParcelCommand{
				parcelOf(t, 1, left), parcelOf(t, 2, right),
			},
		})
		return packErr
	})
	if err != nil {
		t.Fatalf("打包：%v", err)
	}
	return result
}

func parcelOf(t *testing.T, sequence int, quantity int64) application.PackParcelCommand {
	t.Helper()
	return application.PackParcelCommand{
		Sequence: sequence, Packaging: "SYN-PACK-01", Weight: "1.2", WeightUnit: "KG",
		Length: "20", Width: "15", Height: "10", Dimension: "CM",
		Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"), Quantity: quantity,
	}
}

type acceptanceClock struct{}

func (acceptanceClock) Now() time.Time { return preAdviceNumberingInstant }

func wavesOf(t *testing.T, world receiptWorld) *adapter.Waves {
	t.Helper()
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	return waves
}

func (world receiptWorld) recordSupplement(t *testing.T, handler *application.SupplementHandler, command application.SupplementCommand) application.SupplementOutcome {
	t.Helper()
	var result application.SupplementResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var recordErr error
		result, recordErr = handler.Record(ctx, command)
		return recordErr
	})
	if err != nil {
		t.Fatalf("入账待补充：%v", err)
	}
	return result.Outcome()
}

func acceptanceHandler(t *testing.T, world receiptWorld) *application.AcceptanceHandler {
	t.Helper()
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	return application.NewAcceptanceHandler(waves, acceptanceClock{})
}

func (world receiptWorld) recordAcceptance(t *testing.T, handler *application.AcceptanceHandler, command application.AcceptanceCommand) application.AcceptanceOutcome {
	t.Helper()
	var result application.AcceptanceResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var recordErr error
		result, recordErr = handler.Record(ctx, command)
		return recordErr
	})
	if err != nil {
		t.Fatalf("入账接受决定：%v", err)
	}
	return result.Outcome()
}

func insertPackedParcel(t *testing.T, world receiptWorld, requestID, dispatch, service, shipment string, sequence int) {
	t.Helper()
	var serviceArg any
	if service != "" {
		serviceArg = service
	}
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_request (
		    tenant_id, request_id, owner_account_id, warehouse_code, origin, request_key, dispatch, service_product,
		    recipient, address, status, numbering_absence, received_at)
		 VALUES ($1,$2,'SYN-OWNER-01','SYN-WH-01','CUSTOMER',$2,$3,$4,
		         '收件人','地址','PACKED','NUMBERING_TIME_ZONE_NOT_CONFIGURED', now())`,
		"SYN-TENANT-01", requestID, dispatch, serviceArg); err != nil {
		t.Fatal(err)
	}
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_request_line (
		    tenant_id, request_id, line_index, goods_code, unit_name, stock_state, quantity)
		 VALUES ($1,$2,0,'SYN-GOODS-01','件','GOOD',1)`, "SYN-TENANT-01", requestID); err != nil {
		t.Fatal(err)
	}
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_parcel (
		    tenant_id, request_id, parcel_sequence, packaging_material, weight_value, weight_unit,
		    length_value, width_value, height_value, dimension_unit, shipment_request_id)
		 VALUES ($1,$2,$3,'SYN-PACK','1','KG','1','1','1','CM',$4)`,
		"SYN-TENANT-01", requestID, sequence, shipment); err != nil {
		t.Fatal(err)
	}
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_parcel_line (
		    tenant_id, request_id, parcel_sequence, line_index, goods_code, unit_name, quantity)
		 VALUES ($1,$2,$3,0,'SYN-GOODS-01','件',1)`,
		"SYN-TENANT-01", requestID, sequence); err != nil {
		t.Fatal(err)
	}
}

func commissionState(t *testing.T, world receiptWorld, requestID string, sequence int) string {
	t.Helper()
	var state *string
	err := world.pool.QueryRow(t.Context(),
		`SELECT commission_state FROM warehouse_fulfillment.outbound_parcel
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3`,
		"SYN-TENANT-01", requestID, sequence).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	if state == nil {
		return ""
	}
	return *state
}

type scriptedCommissioner struct {
	outcome string
	id      string
	called  int
}

func (commissioner *scriptedCommissioner) Submit(context.Context, ports.CommissionSubmission) (ports.CommissionAnswer, error) {
	commissioner.called++
	answer := ports.CommissionAnswer{Outcome: commissioner.outcome}
	if commissioner.id != "" {
		answer.ShipmentRequestID = commissioner.id + "-" + strconv.Itoa(commissioner.called)
	}
	return answer, nil
}

func (world receiptWorld) submitCommission(t *testing.T, commissioner ports.OutboundCommissioner, request domain.OutboundRequestID) application.CommissionOutcome {
	t.Helper()
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	handler := application.NewCommissionHandler(requests, waves, commissioner, world.warehouses)
	var result application.CommissionResult
	err = world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var submitErr error
		result, submitErr = handler.Submit(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), request)
		return submitErr
	})
	if err != nil {
		t.Fatalf("提交委托：%v", err)
	}
	return result.Outcome()
}

func submittedParcels(t *testing.T, world receiptWorld, request domain.OutboundRequestID) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.outbound_parcel
		  WHERE tenant_id = $1 AND request_id = $2 AND shipment_request_id IS NOT NULL`,
		"SYN-TENANT-01", request.String()).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func parcelCount(t *testing.T, world receiptWorld) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.outbound_parcel WHERE tenant_id = $1`, "SYN-TENANT-01").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func requestStatus(t *testing.T, world receiptWorld, id domain.OutboundRequestID) string {
	t.Helper()
	return parentStatus(t, world, id)
}

func pickDifferences(t *testing.T, world receiptWorld) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.pick_difference WHERE tenant_id = $1`, "SYN-TENANT-01").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}
