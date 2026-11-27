package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

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

// Covers: 波次的状态跟着它的拣货任务走。刚放出是已放出；拣了一条还剩一条是拣货中；都有了结果（拣满或少拣）是已完成；
// 任务全被截单作废是已取消。迟到的任务不算：作废之后才拣出来，波次仍是已取消；同一个波次里另有任务等着拣，
// 还是已放出，与进度写的「已拣 0」对得上。按状态筛只答那一种。按号读一张与列表的一行同形，没有的波次答找不到。
func TestAWaveStatusFollowsItsPickTasks(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 60)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	outbound := world.outbound(t)
	reserve := func(key string) domain.OutboundRequestID {
		t.Helper()
		accepted := world.acceptOutbound(t, outbound, outboundCommand(t, key, 10))
		request, ok := accepted.Request()
		if accepted.Outcome() != application.OutboundReserved || !ok {
			t.Fatalf("预占 %s：%s", key, accepted.Outcome())
		}
		return request.ID()
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	store, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	statusOf := func(id string) string {
		t.Helper()
		row, found, err := store.LoadWaveRow(t.Context(), tenant, id)
		if err != nil || !found {
			t.Fatalf("读波次 %s：found %v err %v", id, found, err)
		}
		return row.Status
	}
	listed := func(raw string) []string {
		t.Helper()
		query, err := ports.WavePages.DecodeRaw(raw)
		if err != nil {
			t.Fatal(err)
		}
		page, err := store.ListWaves(t.Context(), tenant, 20, query)
		if err != nil {
			t.Fatalf("波次列表 %s：%v", raw, err)
		}
		ids := make([]string, 0, len(page.Rows))
		for _, row := range page.Rows {
			if row.Status == "" {
				t.Fatalf("波次 %s 没带状态", row.WaveID)
			}
			ids = append(ids, row.WaveID)
		}
		return ids
	}
	openTasks := func(raw string) int {
		t.Helper()
		query, err := ports.PickTaskPages.DecodeRaw(raw)
		if err != nil {
			t.Fatal(err)
		}
		page, err := store.ListPickTasks(t.Context(), tenant, 20, query)
		if err != nil {
			t.Fatalf("拣货任务 %s：%v", raw, err)
		}
		if int(page.Total) != len(page.Rows) {
			t.Fatalf("拣货任务 %s 的总数 %d 与本页 %d 不是同一组", raw, page.Total, len(page.Rows))
		}
		return len(page.Rows)
	}
	listedWave := func(id string) ports.WaveRow {
		t.Helper()
		query, err := ports.WavePages.DecodeRaw("")
		if err != nil {
			t.Fatal(err)
		}
		page, err := store.ListWaves(t.Context(), tenant, 20, query)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Rows {
			if row.WaveID == id {
				return row
			}
		}
		t.Fatalf("列表里没有 %s", id)
		return ports.WaveRow{}
	}

	pair := []domain.OutboundRequestID{reserve("REQ-STATUS-A"), reserve("REQ-STATUS-B")}
	var released application.WaveResult
	if err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var releaseErr error
		released, releaseErr = handler.Release(ctx, application.ReleaseWaveCommand{
			Tenant: tenant, ID: valueOf(t, domain.NewWaveID, "WAVE-STATUS"),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
			Requests: pair,
		})
		return releaseErr
	}); err != nil || released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放两张单 %s 任务 %d err %v", released.Outcome(), len(released.Tasks()), err)
	}
	if got := statusOf("WAVE-STATUS"); got != ports.WaveReleased {
		t.Fatalf("刚放出 = %s", got)
	}
	if openTasks("warehouseCode=SYN-WH-01&status=OPEN") != 2 || openTasks("warehouseCode=SYN-WH-01&status=DONE") != 0 {
		t.Fatal("刚放出应有两条待执行、没有拣满")
	}
	if row := listedWave("WAVE-STATUS"); len(row.Tasks) != 0 || row.OpenTasks != 2 || row.DoneTasks != 0 {
		t.Fatalf("列表应只带条数、不嵌任务：任务 %d 条，待执行 %d，拣满 %d", len(row.Tasks), row.OpenTasks, row.DoneTasks)
	}
	if world.pick(t, handler, released.Tasks()[0].ID, 10).Outcome() != application.WavePicked {
		t.Fatal("第一条没有拣成")
	}
	if got := statusOf("WAVE-STATUS"); got != ports.WavePicking {
		t.Fatalf("拣了一条 = %s", got)
	}
	if openTasks("warehouseCode=SYN-WH-01&status=OPEN") != 1 || openTasks("warehouseCode=SYN-WH-01&status=DONE") != 1 ||
		openTasks("waveId=WAVE-STATUS&status=OPEN") != 1 || openTasks("warehouseCode=SYN-WH-02&status=OPEN") != 0 {
		t.Fatal("拣了一条之后应剩一条待执行、一条拣满，别的仓没有")
	}
	if ids := listed("status=" + ports.WavePicking); len(ids) != 1 || ids[0] != "WAVE-STATUS" {
		t.Fatalf("按拣货中筛 = %v", ids)
	}
	if world.pick(t, handler, released.Tasks()[1].ID, 10).Outcome() != application.WavePicked {
		t.Fatal("第二条没有拣成")
	}
	if got := statusOf("WAVE-STATUS"); got != ports.WaveCompleted {
		t.Fatalf("都拣了 = %s", got)
	}

	cut := reserve("REQ-STATUS-CUT")
	cutWave := world.releaseWave(t, handler, "WAVE-STATUS-CUT", cut)
	if cutWave.Outcome() != application.WaveAllocated {
		t.Fatal("要截的那张没有放出")
	}
	if world.cutoff(t, handler, cut, 0, "SYN-CUT-STATUS").Outcome() != application.CutoffCancelled {
		t.Fatal("截单没有成立")
	}
	if got := statusOf("WAVE-STATUS-CUT"); got != ports.WaveCancelled {
		t.Fatalf("任务全作废 = %s", got)
	}
	if world.pick(t, handler, cutWave.Tasks()[0].ID, 10).Outcome() != application.WaveRecorded {
		t.Fatal("作废之后的拣货没有记成迟到")
	}
	if got := statusOf("WAVE-STATUS-CUT"); got != ports.WaveCancelled {
		t.Fatalf("作废之后才拣出来 = %s", got)
	}

	late := []domain.OutboundRequestID{reserve("REQ-STATUS-LATE"), reserve("REQ-STATUS-WAIT")}
	var lateWave application.WaveResult
	if err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var releaseErr error
		lateWave, releaseErr = handler.Release(ctx, application.ReleaseWaveCommand{
			Tenant: tenant, ID: valueOf(t, domain.NewWaveID, "WAVE-STATUS-LATE"),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
			Requests: late,
		})
		return releaseErr
	}); err != nil || lateWave.Outcome() != application.WaveAllocated || len(lateWave.Tasks()) != 2 {
		t.Fatalf("放迟到那两张 %s err %v", lateWave.Outcome(), err)
	}
	if world.cutoff(t, handler, late[0], 0, "SYN-CUT-LATE").Outcome() != application.CutoffCancelled {
		t.Fatal("迟到那张的截单没有成立")
	}
	if got := statusOf("WAVE-STATUS-LATE"); got != ports.WaveReleased {
		t.Fatalf("一条作废、一条等着拣 = %s", got)
	}
	var lateTask domain.PickTaskID
	for _, task := range lateWave.Tasks() {
		if task.Request == late[0] {
			lateTask = task.ID
		}
	}
	if world.pick(t, handler, lateTask, 10).Outcome() != application.WaveRecorded {
		t.Fatal("迟到那条没有记下")
	}
	if got := statusOf("WAVE-STATUS-LATE"); got != ports.WaveReleased {
		t.Fatalf("一条迟到、一条等着拣 = %s", got)
	}

	// 少拣放在最后：差异没了结时这个库位不再分配，排在它后面的放波次都分不到货。
	short := world.releaseWave(t, handler, "WAVE-STATUS-SHORT", reserve("REQ-STATUS-SHORT"))
	if short.Outcome() != application.WaveAllocated || world.pick(t, handler, short.Tasks()[0].ID, 4).Outcome() != application.WavePicked {
		t.Fatal("少拣没有记下")
	}
	if got := statusOf("WAVE-STATUS-SHORT"); got != ports.WaveCompleted {
		t.Fatalf("少拣之后 = %s", got)
	}

	if ids := listed("status=" + ports.WaveCancelled); len(ids) != 1 || ids[0] != "WAVE-STATUS-CUT" {
		t.Fatalf("按已取消筛 = %v", ids)
	}
	if ids := listed("status=" + ports.WaveCompleted + "&sort=waveId"); len(ids) != 2 || ids[0] != "WAVE-STATUS" || ids[1] != "WAVE-STATUS-SHORT" {
		t.Fatalf("按已完成筛 = %v", ids)
	}
	if ids := listed("status=" + ports.WaveReleased); len(ids) != 1 || ids[0] != "WAVE-STATUS-LATE" {
		t.Fatalf("按已放出筛 = %v", ids)
	}
	if ids := listed("status=" + ports.WavePicking); len(ids) != 0 {
		t.Fatalf("末了按拣货中筛 = %v", ids)
	}
	if ids := listed("status=" + ports.WaveReleased + "&status=" + ports.WaveCompleted + "&sort=waveId"); len(ids) != 3 {
		t.Fatalf("两种状态一起筛 = %v", ids)
	}
	if ids := listed(""); len(ids) != 4 {
		t.Fatalf("不筛 = %v", ids)
	}

	row, found, err := store.LoadWaveRow(t.Context(), tenant, "WAVE-STATUS")
	if err != nil || !found || row.WarehouseCode != "SYN-WH-01" || len(row.Requests) != 2 || len(row.Tasks) != 2 || row.Tasks[0].Picked != 10 {
		t.Fatalf("按号读一张 %+v found %v err %v", row, found, err)
	}
	if _, found, err := store.LoadWaveRow(t.Context(), tenant, "WAVE-NOT-THERE"); err != nil || found {
		t.Fatalf("没有的波次 found %v err %v", found, err)
	}
}

func TestAPrefetchedWaveSubmitsOneEstimatedCommission(t *testing.T) {
	measured := newMeasurementWorld(t)
	measured.seedStorageStock(t, 10)
	recorded := measured.record(t, measured.clock.Now(), goodsLevelMeasureCommand(
		t, "SYN-FACT-MEASURE", "DEV-1", measured.clock.Now(), 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件",
		"10", "10", "10", "CM", "1", "KG",
	))
	if recorded.Outcome().String() == "" {
		t.Fatal("仓测没有记下")
	}
	materials, err := adapter.NewPackagingMaterials(measured.db)
	if err != nil {
		t.Fatal(err)
	}
	if savePackagingMaterial(t, packagingMaterialStores{transactor: measured.transactor, pool: measured.pool, packagingMaterials: materials}, packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-PRE", 1, nil)) != ports.RegistryRevisionSaved {
		t.Fatal("包材没有记下")
	}
	accepted := measured.acceptOutbound(t, measured.outbound(t), outboundCommand(t, "REQ-PREFETCH", 10))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	commissioner := &scriptedCommissioner{outcome: ports.CommissionSubmittedWord, id: "SHR-PRE"}
	handler := measured.waves(t).WithPrefetch(commissioner, measured.measurements, materials)
	if measured.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	var released application.WaveResult
	err = measured.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var releaseErr error
		released, releaseErr = handler.Release(ctx, application.ReleaseWaveCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: valueOf(t, domain.NewWaveID, "WAVE-PRE"),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
			Requests: []domain.OutboundRequestID{request.ID()}, Prefetch: true,
			Packaging: valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-PRE"),
		})
		return releaseErr
	})
	if err != nil || released.Outcome() != application.WaveAllocated || commissioner.called != 1 || commissioner.last.Sequence != 1 || commissioner.last.Parcel.Quantity != 10 || commissioner.last.Parcel.Weight != "10.25" {
		t.Fatalf("预取 %s 调用 %d 重量 %s err %v", released.Outcome(), commissioner.called, commissioner.last.Parcel.Weight, err)
	}
	var shipment string
	if err := measured.pool.QueryRow(t.Context(),
		`SELECT prefetch_shipment_id FROM warehouse_fulfillment.outbound_request WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1`,
		request.ID().String()).Scan(&shipment); err != nil || shipment != "SHR-PRE-1" {
		t.Fatalf("记下的委托 %s err %v", shipment, err)
	}
	wavesStore, err := adapter.NewWaves(measured.db)
	if err != nil {
		t.Fatal(err)
	}
	waveQuery, err := ports.WavePages.DecodeRaw("prefetch=true")
	if err != nil {
		t.Fatal(err)
	}
	listedWaves, err := wavesStore.ListWaves(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), 20, waveQuery)
	if err != nil || len(listedWaves.Rows) != 1 || listedWaves.Rows[0].WaveID != "WAVE-PRE" || len(listedWaves.Rows[0].Requests) != 1 || listedWaves.Rows[0].OpenTasks != 1 || len(listedWaves.Rows[0].Tasks) != 0 {
		t.Fatalf("波次列表 %+v err %v", listedWaves.Rows, err)
	}
}

func TestAPrefetchedPackRevisesOnlyWhenMeasuredWeightDiffers(t *testing.T) {
	measured := newMeasurementWorld(t)
	measured.seedStorageStock(t, 50)
	measured.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	measured.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	if measured.record(t, measured.clock.Now(), goodsLevelMeasureCommand(
		t, "SYN-FACT-M-REV", "DEV-1", measured.clock.Now(), 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件",
		"10", "10", "10", "CM", "1", "KG",
	)).Outcome().String() == "" {
		t.Fatal("仓测没有记下")
	}
	materials, err := adapter.NewPackagingMaterials(measured.db)
	if err != nil {
		t.Fatal(err)
	}
	if savePackagingMaterial(t, packagingMaterialStores{transactor: measured.transactor, pool: measured.pool, packagingMaterials: materials}, packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-PRE", 1, nil)) != ports.RegistryRevisionSaved {
		t.Fatal("包材没有记下")
	}
	commissioner := &scriptedCommissioner{outcome: ports.CommissionSubmittedWord, id: "SHR-REV"}
	handler := measured.waves(t).WithPrefetch(commissioner, measured.measurements, materials)
	if measured.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	wavesStore, err := adapter.NewWaves(measured.db)
	if err != nil {
		t.Fatal(err)
	}
	outStore, err := adapter.NewOutboundRequests(measured.db)
	if err != nil {
		t.Fatal(err)
	}
	warehouses, err := adapter.NewWarehouses(measured.db)
	if err != nil {
		t.Fatal(err)
	}

	// split 给了就把这 10 件打成两个包裹（6 件与 4 件），并接上货物明细修订口：预取的那份委托按 10 件申报，
	// 包裹序号 1 里实际只有 6 件。
	runPack := func(key, waveID, weight string, amender ports.MeasurementAmender, split ...ports.GoodsAmender) (domain.OutboundRequestID, string) {
		t.Helper()
		goods, unit := valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), valueOf(t, domain.NewGoodsUnitName, "件")
		parcels := []application.PackParcelCommand{{
			Sequence: 1, Packaging: "SYN-BOX-PRE", Weight: weight, WeightUnit: "KG",
			Length: "30", Width: "20", Height: "15.5", Dimension: "CM", Goods: goods, Unit: unit, Quantity: 10,
		}}
		if len(split) > 0 {
			parcels[0].Quantity = 6
			parcels = append(parcels, application.PackParcelCommand{
				Sequence: 2, Packaging: "SYN-BOX-PRE", Weight: "4.250", WeightUnit: "KG",
				Length: "30", Width: "20", Height: "15.5", Dimension: "CM", Goods: goods, Unit: unit, Quantity: 4,
			})
		}
		accepted := measured.acceptOutbound(t, measured.outbound(t), outboundCommand(t, key, 10))
		req, ok := accepted.Request()
		if !ok {
			t.Fatalf("预占失败 %s", accepted.Outcome())
		}
		var released application.WaveResult
		if err := measured.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			var runErr error
			released, runErr = handler.Release(ctx, application.ReleaseWaveCommand{
				Tenant: tenant, ID: valueOf(t, domain.NewWaveID, waveID),
				Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
				Requests: []domain.OutboundRequestID{req.ID()}, Prefetch: true,
				Packaging: valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-PRE"),
			})
			return runErr
		}); err != nil || released.Outcome() != application.WaveAllocated {
			t.Fatalf("放波次 %s %v", released.Outcome(), err)
		}
		if measured.pick(t, handler, released.Tasks()[0].ID, 10).Outcome() != application.WavePicked {
			t.Fatal("拣货失败")
		}
		if err := measured.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			res, runErr := handler.Check(ctx, application.CheckCommand{
				Tenant: tenant, FactID: valueOf(t, domain.NewPickFactID, "CHK-"+waveID), Request: req.ID(), Observed: 10,
			})
			if runErr != nil {
				return runErr
			}
			if res.Outcome() != application.WaveChecked {
				return fmt.Errorf("check %s", res.Outcome())
			}
			packRes, runErr := handler.Pack(ctx, application.PackCommand{
				Tenant: tenant, FactID: valueOf(t, domain.NewPickFactID, "PACK-"+waveID), Request: req.ID(),
				Parcels: parcels,
			})
			if runErr != nil {
				return runErr
			}
			if packRes.Outcome() != application.WavePacked {
				return fmt.Errorf("pack %s", packRes.Outcome())
			}
			submitter := application.NewCommissionHandler(outStore, wavesStore, commissioner, warehouses).WithMeasurementAmender(amender)
			if len(split) > 0 {
				submitter = submitter.WithGoodsAmender(split[0])
			}
			subRes, runErr := submitter.Submit(ctx, tenant, req.ID())
			if runErr != nil {
				return runErr
			}
			if subRes.Outcome() != application.CommissionSubmitted {
				return fmt.Errorf("submit %s", subRes.Outcome())
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		var state string
		if err := measured.pool.QueryRow(t.Context(),
			`SELECT coalesce(correction_state, '') FROM warehouse_fulfillment.outbound_parcel WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1 AND parcel_sequence = 1`,
			req.ID().String()).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return req.ID(), state
	}

	// 称重相同（10.250 == 10.25）：不发资料修订，不改更正态。
	sameAmender := &scriptedMeasurementAmender{outcome: ports.MeasurementAdoptedWord}
	_, sameState := runPack("REQ-PRE-SAME", "WAVE-PRE-SAME", "10.250", sameAmender)
	if sameAmender.called != 0 || sameState != "" || commissioner.called != 1 {
		t.Fatalf("相同称重不应修订 called=%d state=%q submit=%d", sameAmender.called, sameState, commissioner.called)
	}

	// 称重不同且修订已采用：发资料修订，记 ADOPTED。
	adoptedAmender := &scriptedMeasurementAmender{outcome: ports.MeasurementAdoptedWord}
	_, adoptedState := runPack("REQ-PRE-DIFF-OK", "WAVE-PRE-DIFF-OK", "11.5", adoptedAmender)
	if adoptedAmender.called != 1 || adoptedAmender.last.Weight != "11.5" || adoptedState != "ADOPTED" || commissioner.called != 2 {
		t.Fatalf("称重不同已采用 called=%d weight=%s state=%q submit=%d", adoptedAmender.called, adoptedAmender.last.Weight, adoptedState, commissioner.called)
	}

	// 称重不同且待复核：记 AWAITING_REVIEW，离线确认留下 CORRECTION_ADOPTED 缺口。
	reviewAmender := &scriptedMeasurementAmender{outcome: ports.MeasurementAwaitingReviewWord}
	reviewReq, reviewState := runPack("REQ-PRE-DIFF-HOLD", "WAVE-PRE-DIFF-HOLD", "12.0", reviewAmender)
	if reviewAmender.called != 1 || reviewState != "AWAITING_REVIEW" {
		t.Fatalf("称重不同待复核 called=%d state=%q", reviewAmender.called, reviewState)
	}
	offline := measured.confirmOutbound(t, handler, reviewReq, "SYN-FACT-PRE-OFF", application.ConfirmKindOutbound, true)
	if offline.Outcome() != application.WaveRecorded {
		t.Fatalf("离线确认 %s", offline.Outcome())
	}
	var missing string
	if err := measured.pool.QueryRow(t.Context(),
		`SELECT coalesce(missing, '') FROM warehouse_fulfillment.outbound_confirmation WHERE tenant_id = 'SYN-TENANT-01' AND fact_id = 'SYN-FACT-PRE-OFF'`,
	).Scan(&missing); err != nil || !strings.Contains(missing, "CORRECTION_ADOPTED") {
		t.Fatalf("缺口应含 CORRECTION_ADOPTED got=%q err=%v", missing, err)
	}

	// 预取的那份委托按 10 件申报，打成两个包裹之后包裹序号 1 里只有 6 件：提货物明细修订，交的是实际装的 6 件；
	// 这一个包裹的称重也与估算不同，测量修订照交。两样都已采用才记 ADOPTED；第二个包裹另交一份委托。
	before := commissioner.called
	goodsAdopted := &scriptedGoodsAmender{outcome: ports.MeasurementAdoptedWord}
	bothMeasure := &scriptedMeasurementAmender{outcome: ports.MeasurementAdoptedWord}
	_, bothState := runPack("REQ-PRE-SPLIT-OK", "WAVE-PRE-SPLIT-OK", "6.250", bothMeasure, goodsAdopted)
	if goodsAdopted.called != 1 || goodsAdopted.last.Quantity != 6 || goodsAdopted.last.Goods != "SYN-GOODS-01" || goodsAdopted.last.DeclaredParcelID == "" ||
		bothMeasure.called != 1 || bothState != "ADOPTED" || commissioner.called != before+2 {
		t.Fatalf("件数与称重都不同、都已采用：件数修订 %d 次 %+v，测量修订 %d 次，更正态 %q，委托交了 %d 份",
			goodsAdopted.called, goodsAdopted.last, bothMeasure.called, bothState, commissioner.called-before)
	}

	// 件数的修订没被采用：哪怕测量的修订已采用，这个包裹也停在待复核。
	goodsHeld := &scriptedGoodsAmender{outcome: ports.MeasurementDisallowedWord}
	heldMeasure := &scriptedMeasurementAmender{outcome: ports.MeasurementAdoptedWord}
	_, heldState := runPack("REQ-PRE-SPLIT-HOLD", "WAVE-PRE-SPLIT-HOLD", "6.250", heldMeasure, goodsHeld)
	if goodsHeld.called != 1 || heldMeasure.called != 1 || heldState != "AWAITING_REVIEW" {
		t.Fatalf("件数修订没被采用：件数修订 %d 次，测量修订 %d 次，更正态 %q", goodsHeld.called, heldMeasure.called, heldState)
	}
}

type scriptedGoodsAmender struct {
	outcome string
	called  int
	last    ports.GoodsAmendment
}

func (amender *scriptedGoodsAmender) AmendGoods(_ context.Context, amendment ports.GoodsAmendment) (ports.MeasurementAmendmentAnswer, error) {
	amender.called++
	amender.last = amendment
	return ports.MeasurementAmendmentAnswer{Outcome: amender.outcome}, nil
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

func TestTrackedGoodsKeepTheSerialsOnThePackedParcels(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 2)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-SERIAL", 2))
	request, _ := accepted.Request()
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-SERIAL", request.ID())
	if world.pick(t, handler, released.Tasks()[0].ID, 2).Outcome() != application.WavePicked {
		t.Fatal("没有拣完")
	}
	if world.checkOutbound(t, handler, request.ID(), "SYN-FACT-SERIAL-CHECK", 2).Outcome() != application.WaveChecked {
		t.Fatal("没有复核完")
	}
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	pack := func(fact string, serials []string) application.WaveResult {
		t.Helper()
		parcel := parcelOf(t, 1, 2)
		parcel.Serials = serials
		var result application.WaveResult
		err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			var packErr error
			result, packErr = handler.Pack(ctx, application.PackCommand{
				Tenant: tenant, FactID: valueOf(t, domain.NewPickFactID, fact), Request: request.ID(),
				Parcels: []application.PackParcelCommand{parcel},
			})
			return packErr
		})
		if err != nil {
			t.Fatalf("打包：%v", err)
		}
		return result
	}
	if pack("SYN-FACT-SERIAL-OFF", []string{"SN-1", "SN-2"}).Outcome() != application.WaveContentInvalid || parcelCount(t, world) != 0 {
		t.Fatal("没开跟踪序列号却收下了号")
	}
	tag, err := world.pool.Exec(t.Context(),
		`UPDATE warehouse_fulfillment.goods_revision SET track_serial = true
		  WHERE tenant_id = 'SYN-TENANT-01' AND goods_code = 'SYN-GOODS-01'`)
	if err != nil || tag.RowsAffected() == 0 {
		t.Fatalf("打开跟踪序列号：%v 行 %d", err, tag.RowsAffected())
	}
	if pack("SYN-FACT-SERIAL-SHORT", []string{"SN-1"}).Outcome() != application.WaveContentInvalid || parcelCount(t, world) != 0 {
		t.Fatal("号数对不上却记下了")
	}
	if pack("SYN-FACT-SERIAL", []string{"SN-1", "SN-2"}).Outcome() != application.WavePacked {
		t.Fatal("号齐了却没有记下")
	}
	stored, err := wavesOf(t, world).ParcelsOf(t.Context(), tenant, request.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || len(stored[0].Serials) != 2 || stored[0].Serials[0] != "SN-1" || stored[0].Serials[1] != "SN-2" {
		t.Fatalf("读回的号 %+v", stored)
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

func TestPlatformStockingSubmitsOneCommissionForEveryPackedParcel(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 10)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	command := outboundCommand(t, "REQ-PLATFORM", 10)
	command.PlatformStocking = true
	accepted := world.acceptOutbound(t, world.outbound(t), command)
	request, _ := accepted.Request()
	if !request.PlatformStocking() {
		t.Fatal("平台备货没有落在请求上")
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-PLATFORM", request.ID())
	if len(released.Tasks()) == 0 || world.pick(t, handler, released.Tasks()[0].ID, 10).Outcome() != application.WavePicked {
		t.Fatal("没有拣完")
	}
	if world.checkOutbound(t, handler, request.ID(), "SYN-FACT-PLATFORM", 10).Outcome() != application.WaveChecked {
		t.Fatal("没有复核")
	}
	if world.packOutbound(t, handler, request.ID(), "SYN-FACT-PLATFORM-PACK", 6, 4).Outcome() != application.WavePacked {
		t.Fatal("没有打包")
	}
	one := &scriptedCommissioner{outcome: ports.CommissionSubmittedWord, id: "SHR-PLATFORM"}
	if got := world.submitCommission(t, one, request.ID()); got != application.CommissionSubmitted || one.called != 1 || one.last.Round != 1 || len(one.last.Parcels) != 2 || submittedParcels(t, world, request.ID()) != 2 {
		t.Fatalf("平台备货 %s 调用 %d 轮次 %d 包裹 %d 已提交 %d", got, one.called, one.last.Round, len(one.last.Parcels), submittedParcels(t, world, request.ID()))
	}
	var shipments int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(DISTINCT shipment_request_id) FROM warehouse_fulfillment.outbound_parcel
		  WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1`, request.ID().String()).Scan(&shipments); err != nil || shipments != 1 {
		t.Fatalf("委托份数 %d err %v", shipments, err)
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
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.pick_task (
		    tenant_id, task_id, wave_id, request_id, location_code, goods_code, unit_name, stock_state,
		    inbound_batch_id, quantity, picked, status, staging_location)
		 VALUES ('SYN-TENANT-01','TASK-REJ','WAVE-REJ','OBR-REJ','SYN-LOC-STORE','SYN-GOODS-01','件','GOOD',
		         'SYN-BATCH-REJ',1,1,'DONE','SYN-LOC-OUT')`); err != nil {
		t.Fatal(err)
	}
	rejected := world.recordAcceptance(t, handler, application.AcceptanceCommand{
		Tenant: tenant, DecisionID: "ADEC-3", Source: domain.WarehouseOutboundSource, SourceRequestKey: "OBR-REJ/1", State: "REJECTED",
	})
	var returns int
	var requestStatus, cutoff string
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.return_task WHERE tenant_id = 'SYN-TENANT-01' AND request_id = 'OBR-REJ'`).Scan(&returns); err != nil {
		t.Fatal(err)
	}
	if err := world.pool.QueryRow(t.Context(),
		`SELECT request.status, parcel.cutoff_state
		   FROM warehouse_fulfillment.outbound_request AS request
		   JOIN warehouse_fulfillment.outbound_parcel AS parcel
		     ON parcel.tenant_id = request.tenant_id AND parcel.request_id = request.request_id
		  WHERE request.tenant_id = 'SYN-TENANT-01' AND request.request_id = 'OBR-REJ'`).Scan(&requestStatus, &cutoff); err != nil {
		t.Fatal(err)
	}
	if rejected != application.AcceptanceRejected || commissionState(t, world, "OBR-REJ", 1) != "REJECTED" || returns != 1 || requestStatus != "CANCELLED" || cutoff != "CANCELLED" {
		t.Fatalf("拒绝 %s 状态 %s 回库 %d 请求 %s 截单 %s", rejected, commissionState(t, world, "OBR-REJ", 1), returns, requestStatus, cutoff)
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
	if _, err := world.pool.Exec(t.Context(),
		`UPDATE warehouse_fulfillment.outbound_parcel SET declared_parcel_id = 'DPCL-SUP', shipment_request_id = 'SHR-SUP'
		  WHERE tenant_id = 'SYN-TENANT-01' AND request_id = 'OBR-SUP'`); err != nil {
		t.Fatal(err)
	}
	follow := application.NewFollowUpHandler(outboundRequests(t, world), wavesOf(t, world), scriptedReviser{outcome: "SUPPLEMENT_RECORDED"}, acceptanceClock{})
	resubmitted := world.resubmit(t, follow, tenant, "OBR-SUP")
	if resubmitted != application.FollowUpRecorded || commissionState(t, world, "OBR-SUP", 1) != "" {
		t.Fatalf("再提交 %s 状态 %q", resubmitted, commissionState(t, world, "OBR-SUP", 1))
	}
	held := world.recordSourceData(t, follow, "SDV-1", domain.WarehouseOutboundSource, "OBR-SUP/1", "AWAITING_REVIEW")
	if held != application.FollowUpAwaitingReview || correctionState(t, world, "OBR-SUP") != "AWAITING_REVIEW" {
		t.Fatalf("未采用 %s 更正 %s", held, correctionState(t, world, "OBR-SUP"))
	}
	otherData := world.recordSourceData(t, follow, "SDV-2", "CUSTOMER_CHANNEL", "REQ-1", "ADOPTED")
	if otherData != application.FollowUpPosted || correctionState(t, world, "OBR-SUP") != "AWAITING_REVIEW" {
		t.Fatalf("别的资料版本 %s 更正 %s", otherData, correctionState(t, world, "OBR-SUP"))
	}
	// 同一个包裹的另一份修订（件数与测量各一份）已采用：先到的那份还在待复核，包裹仍是待复核，不被后到的「已采用」盖掉。
	second := world.recordSourceData(t, follow, "SDV-3", domain.WarehouseOutboundSource, "OBR-SUP/1", "ADOPTED")
	if second != application.FollowUpAdopted || correctionState(t, world, "OBR-SUP") != "AWAITING_REVIEW" {
		t.Fatalf("另一份修订已采用 %s 更正 %s", second, correctionState(t, world, "OBR-SUP"))
	}
	cancelled := world.recordCancellation(t, follow, "cancel-sup", "DPCL-SUP")
	if cancelled != application.FollowUpCancelled || commissionState(t, world, "OBR-SUP", 1) != "CANCELLED" {
		t.Fatalf("取消 %s 状态 %s", cancelled, commissionState(t, world, "OBR-SUP", 1))
	}
	foreign := world.recordCancellation(t, follow, "cancel-other", "DPCL-OTHER")
	if foreign != application.FollowUpPosted {
		t.Fatalf("别的取消 %s", foreign)
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
	if err := waves.SaveWave(ctx, tenant, valueOf(t, domain.NewWaveID, "WAVE"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), valueOf(t, domain.NewRegistrant, "operator-1"), false, false, preAdviceNumberingInstant, nil, nil); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveWave：%v", err)
	}
	if err := waves.SavePick(ctx, tenant, ports.PickTask{ID: valueOf(t, domain.NewPickTaskID, "PKT-1"), Request: valueOf(t, domain.NewOutboundRequestID, "OBR-1")}, domain.OutboundPicked, "", "", 0, preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SavePick：%v", err)
	}
	if err := waves.SaveOpenTask(ctx, tenant, ports.PickTask{ID: valueOf(t, domain.NewPickTaskID, "PKT-1")}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveOpenTask：%v", err)
	}
	if err := waves.InsertOpenTask(ctx, tenant, ports.PickTask{ID: valueOf(t, domain.NewPickTaskID, "PKT-1"), WaveID: valueOf(t, domain.NewWaveID, "WAVE"), Request: valueOf(t, domain.NewOutboundRequestID, "OBR-1"), Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood, Inbound: valueOf(t, domain.NewInboundBatchID, "IB-1"), Quantity: 1}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("InsertOpenTask：%v", err)
	}
	if err := waves.SaveCheck(ctx, tenant, valueOf(t, domain.NewPickFactID, "SYN-FACT-CHK"), valueOf(t, domain.NewOutboundRequestID, "OBR-1"), 10, true, preAdviceNumberingInstant, domain.OutboundChecked); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveCheck：%v", err)
	}
	if err := waves.SavePack(ctx, tenant, valueOf(t, domain.NewPickFactID, "SYN-FACT-PACK"), valueOf(t, domain.NewOutboundRequestID, "OBR-1"), nil, preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SavePack：%v", err)
	}
	if err := waves.MarkShipment(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), 1, "SHR-1", "DPCL-1"); !errors.Is(err, bentopg.ErrTransactionRequired) {
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
	if err := waves.SaveSourceData(ctx, tenant, "SDV-1", domain.WarehouseOutboundSource, "OBR-1/1", "ADOPTED", preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveSourceData：%v", err)
	}
	if err := waves.SaveCancellation(ctx, tenant, "cancel-1", "DPCL-1", true, preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveCancellation：%v", err)
	}
	if err := waves.ClearAwaiting(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), 1); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("ClearAwaiting：%v", err)
	}
	if err := waves.SetCorrection(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), 1, "ADOPTED"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SetCorrection：%v", err)
	}
	if err := waves.SaveConfirmation(ctx, tenant, ports.OutboundConfirmation{FactID: "SYN-FACT-OUT", Request: valueOf(t, domain.NewOutboundRequestID, "OBR-1"), Sequence: 1, Kind: "OUTBOUND_CONFIRMATION", State: "CONFIRMED", OccurredAt: preAdviceNumberingInstant}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveConfirmation：%v", err)
	}
	if err := waves.SaveOwnerLabel(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), 1, "EXT-1"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveOwnerLabel：%v", err)
	}
	if err := waves.SaveCorrection(ctx, tenant, ports.ConfirmationCorrection{FactID: "SYN-FACT-OUT", Kind: "INVALIDATE", Operator: "operator-1", Status: "PENDING"}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveCorrection：%v", err)
	}
	if err := waves.PostReplenishment(ctx, tenant, "SYN-FACT-OUT", valueOf(t, domain.NewRegistrant, "operator-2"), 1, preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("PostReplenishment：%v", err)
	}
	if err := waves.SaveCutoff(ctx, tenant, "cutoff-1", "OBR-1", 0, "", "CANCELLED", preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveCutoff：%v", err)
	}
	if err := waves.ReleaseReservations(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("ReleaseReservations：%v", err)
	}
	if err := waves.CancelOpenTasks(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("CancelOpenTasks：%v", err)
	}
	if err := waves.SaveReturnTask(ctx, tenant, ports.ReturnTask{ID: "RET-1", Request: valueOf(t, domain.NewOutboundRequestID, "OBR-1"), Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-OUT"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood, Inbound: valueOf(t, domain.NewInboundBatchID, "IB-1"), Quantity: 1}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveReturnTask：%v", err)
	}
	if err := waves.SavePrefetch(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), ports.PrefetchRecord{ShipmentID: "SHR-1", DeclaredParcelID: "DPCL-1"}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SavePrefetch：%v", err)
	}
	if err := waves.SetRequestStatus(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), domain.OutboundCancelled); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SetRequestStatus：%v", err)
	}
	if _, err := waves.MarkParcelCutoff(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), 1); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkParcelCutoff：%v", err)
	}
	if err := waves.SaveLatePick(ctx, tenant, ports.PickTask{ID: valueOf(t, domain.NewPickTaskID, "PKT-1"), Picked: 1, Staging: valueOf(t, domain.NewLocationCode, "SYN-LOC-OUT")}, "SYN-FACT-LATE"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveLatePick：%v", err)
	}
	if err := waves.CompleteReturn(ctx, tenant, "RET-1", "SYN-FACT-RET"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("CompleteReturn：%v", err)
	}
	returns, err := adapter.NewReturns(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := returns.SaveAdvice(ctx, tenant, ports.ReturnAdvice{ID: valueOf(t, domain.NewReturnAdviceID, "RPA-1"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Origin: domain.ReturnOriginCustomer, Key: "k", Tracking: "t", Received: preAdviceNumberingInstant}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveAdvice：%v", err)
	}
	if err := returns.SaveReceipt(ctx, tenant, ports.ReturnReceipt{Fact: valueOf(t, domain.NewReturnFactID, "RF-1"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-RET"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood, Quantity: 1, Outcome: "COUNTED", Occurred: preAdviceNumberingInstant}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveReceipt：%v", err)
	}
	if err := returns.SaveDisposition(ctx, tenant, ports.ReturnDisposition{Decision: valueOf(t, domain.NewReturnDecisionID, "RD-1"), Fact: valueOf(t, domain.NewReturnFactID, "RF-1"), Destination: domain.ReturnDestinationDestroy, Origin: domain.ReturnOriginCustomer, Decided: preAdviceNumberingInstant}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveDisposition：%v", err)
	}
	numbers, err := adapter.NewReturnNumbers(world.db, world.assigner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := numbers.IssueReturnNumber(ctx, tenant, domain.BusinessTimeZone{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("IssueReturnNumber：%v", err)
	}
	agents, err := adapter.NewAgents(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := agents.SaveConnector(ctx, tenant, valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT"), preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveConnector：%v", err)
	}
	if err := agents.SaveStatement(ctx, tenant, ports.AgentStatement{Source: "SRC", StatedAt: preAdviceNumberingInstant, ValidUntil: preAdviceNumberingInstant, Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件")}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveStatement：%v", err)
	}
	if err := agents.SaveResult(ctx, tenant, ports.AgentResult{ID: "RES", Request: valueOf(t, domain.NewOutboundRequestID, "OBR-1"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT"), Kind: "FORWARDED", Reported: preAdviceNumberingInstant}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveResult：%v", err)
	}
	if err := agents.SaveReportedParcel(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), ports.AgentResult{Weight: "1", WeightUnit: "KG", Length: "1", Width: "1", Height: "1", Dimension: "CM"}, valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), valueOf(t, domain.NewGoodsUnitName, "件"), 1); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveReportedParcel：%v", err)
	}
	if err := agents.MarkReportedShipment(ctx, tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"), "SHR-1", "DPCL-1"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkReportedShipment：%v", err)
	}
	charges, err := adapter.NewCharges(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := charges.SaveOriginal(ctx, tenant, ports.ChargeVersion{ID: "CHO-1", Version: 1, Correction: "ORIGINAL", Kind: "STORAGE", Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01", Source: "storage:1", Occurred: preAdviceNumberingInstant}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveOriginal：%v", err)
	}
	if err := charges.SaveCorrection(ctx, tenant, ports.ChargeVersion{ID: "CHO-1", Version: 2, Supersedes: 1, Correction: "VOID", Kind: "STORAGE", Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01", Source: "storage:1", Occurred: preAdviceNumberingInstant}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveCorrection：%v", err)
	}
	if err := charges.SaveUnits(ctx, tenant, valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), "KG", "M3", preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveUnits：%v", err)
	}
	if err := charges.SaveAging(ctx, tenant, valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), "ACTUAL_BATCH", preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveAging：%v", err)
	}
	if err := charges.SaveProject(ctx, tenant, valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), "LABEL", preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveProject：%v", err)
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
	freezes, err := adapter.NewFreezes(world.db)
	if err != nil {
		t.Fatal(err)
	}
	counts, err := adapter.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	holds := adapter.LedgerHolds{
		Freezes: adapter.NewFreezeHolds(freezes), Counts: counts, Reservations: requests, Allocations: waves,
	}
	// 凡是用到波次编排的用例，收尾时都核一遍：波次上存的状态与照它此刻的任务算出来的一致。
	// 哪个写口改了任务状态却没有重算波次，在这里现形，不必每个用例各自记得去查。
	t.Cleanup(func() { world.assertWaveStatusesFollowTheirTasks(t) })
	return application.NewWaveHandler(
		waves, requests, tasks, differences, world.poster(), world.ledger, world.locations, world.zones,
		world.goods, holds, world.clock, world.warehouses,
	).WithToteWarehouses(world.warehouses)
}

// assertWaveStatusesFollowTheirTasks 把规则在这里另写一遍，不借适配器里重算用的那一句：只看待执行、拣满、少拣三种任务。
func (world receiptWorld) assertWaveStatusesFollowTheirTasks(t *testing.T) {
	t.Helper()
	rows, err := world.pool.Query(context.Background(),
		`SELECT wave.wave_id, wave.status,
		        count(task.task_id) FILTER (WHERE task.status = 'OPEN'),
		        count(task.task_id) FILTER (WHERE task.status IN ('DONE', 'SHORT'))
		   FROM warehouse_fulfillment.wave wave
		   LEFT JOIN warehouse_fulfillment.pick_task task
		     ON task.tenant_id = wave.tenant_id AND task.wave_id = wave.wave_id
		  GROUP BY wave.tenant_id, wave.wave_id, wave.status`)
	if err != nil {
		t.Errorf("核对波次状态：%v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, stored string
		var open, settled int64
		if err := rows.Scan(&id, &stored, &open, &settled); err != nil {
			t.Errorf("核对波次状态：%v", err)
			return
		}
		want := ports.WaveCancelled
		switch {
		case open > 0 && settled == 0:
			want = ports.WaveReleased
		case open > 0:
			want = ports.WavePicking
		case settled > 0:
			want = ports.WaveCompleted
		}
		if stored != want {
			t.Errorf("波次 %s 存的状态是 %s，照它的任务（待执行 %d、拣满或少拣 %d）应为 %s", id, stored, open, settled, want)
		}
	}
	if err := rows.Err(); err != nil {
		t.Errorf("核对波次状态：%v", err)
	}
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

type scriptedReviser struct{ outcome string }

func (reviser scriptedReviser) Revise(context.Context, ports.CommissionRevision) (ports.CommissionAnswer, error) {
	return ports.CommissionAnswer{Outcome: reviser.outcome}, nil
}

func outboundRequests(t *testing.T, world receiptWorld) *adapter.OutboundRequests {
	t.Helper()
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	return requests
}

func (world receiptWorld) resubmit(t *testing.T, handler *application.FollowUpHandler, tenant domain.TenantID, requestID string) application.FollowUpOutcome {
	t.Helper()
	return world.follow(t, func(ctx context.Context) (application.FollowUpResult, error) {
		return handler.Resubmit(ctx, application.ResubmitCommand{
			Tenant: tenant, Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
			Request: valueOf(t, domain.NewOutboundRequestID, requestID), Sequence: 1, RequestKey: "outbound-supplement:SYN-1",
		})
	})
}

func (world receiptWorld) recordSourceData(t *testing.T, handler *application.FollowUpHandler, version, source, key, adoption string) application.FollowUpOutcome {
	t.Helper()
	return world.follow(t, func(ctx context.Context) (application.FollowUpResult, error) {
		return handler.RecordSourceData(ctx, application.SourceDataCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Version: version, Source: source, SourceRequestKey: key, Adoption: adoption,
		})
	})
}

func (world receiptWorld) recordCancellation(t *testing.T, handler *application.FollowUpHandler, requestKey, parcelID string) application.FollowUpOutcome {
	t.Helper()
	return world.follow(t, func(ctx context.Context) (application.FollowUpResult, error) {
		return handler.RecordCancellation(ctx, application.CancellationCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), RequestKey: requestKey, ParcelID: parcelID,
		})
	})
}

func (world receiptWorld) follow(t *testing.T, run func(context.Context) (application.FollowUpResult, error)) application.FollowUpOutcome {
	t.Helper()
	var result application.FollowUpResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var runErr error
		result, runErr = run(ctx)
		return runErr
	})
	if err != nil {
		t.Fatalf("后续信封：%v", err)
	}
	return result.Outcome()
}

func correctionState(t *testing.T, world receiptWorld, requestID string) string {
	t.Helper()
	var state *string
	err := world.pool.QueryRow(t.Context(),
		`SELECT correction_state FROM warehouse_fulfillment.outbound_parcel WHERE tenant_id = $1 AND request_id = $2`,
		"SYN-TENANT-01", requestID).Scan(&state)
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
	last    ports.CommissionSubmission
}

func (commissioner *scriptedCommissioner) Submit(_ context.Context, submission ports.CommissionSubmission) (ports.CommissionAnswer, error) {
	commissioner.called++
	commissioner.last = submission
	answer := ports.CommissionAnswer{Outcome: commissioner.outcome}
	if commissioner.id != "" {
		answer.ShipmentRequestID = commissioner.id + "-" + strconv.Itoa(commissioner.called)
	}
	for _, parcel := range submission.Parcels {
		answer.Members = append(answer.Members, ports.CommissionMember{
			Sequence: parcel.Sequence, DeclaredParcelID: "DCL-" + strconv.Itoa(parcel.Sequence),
		})
	}
	if len(answer.Members) == 1 {
		answer.DeclaredParcelID = answer.Members[0].DeclaredParcelID
	} else if commissioner.id != "" && submission.Sequence > 0 {
		answer.DeclaredParcelID = "DCL-" + strconv.Itoa(commissioner.called)
	}
	return answer, nil
}

type scriptedMeasurementAmender struct {
	outcome string
	called  int
	last    ports.MeasurementAmendment
}

func (amender *scriptedMeasurementAmender) AmendMeasurement(_ context.Context, amendment ports.MeasurementAmendment) (ports.MeasurementAmendmentAnswer, error) {
	amender.called++
	amender.last = amendment
	return ports.MeasurementAmendmentAnswer{Outcome: amender.outcome}, nil
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

func TestAnOutboundConfirmationFollowsTheReleaseCells(t *testing.T) {
	world, handler, request := packedOutbound(t, "REQ-CONFIRM-NET", domain.DispatchProductNetwork, "SYN-SERVICE", 4)
	if _, err := world.pool.Exec(t.Context(),
		`UPDATE warehouse_fulfillment.outbound_parcel
		    SET commission_state = 'ACCEPTED', shipment_request_id = 'SHR-CONFIRM', declared_parcel_id = 'DPCL-CONFIRM'
		  WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1`, request.String()); err != nil {
		t.Fatal(err)
	}
	online := world.confirmOutbound(t, handler, request, "SYN-FACT-CONFIRM-ON", application.ConfirmKindOutbound, false)
	if online.Outcome() != application.WaveNotConfigured || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 4 {
		t.Fatalf("面单本体没配置：%s 待出库 %d", online.Outcome(), world.stockUnitAt(t, "SYN-LOC-OUT", "件"))
	}
	offline := world.confirmOutbound(t, handler, request, "SYN-FACT-CONFIRM-OFF", application.ConfirmKindOutbound, true)
	if offline.Outcome() != application.WaveRecorded || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 || requestStatus(t, world, request) != "SHIPPED" {
		t.Fatalf("离线照记 %s 待出库 %d 状态 %s", offline.Outcome(), world.stockUnitAt(t, "SYN-LOC-OUT", "件"), requestStatus(t, world, request))
	}
	again := world.confirmOutbound(t, handler, request, "SYN-FACT-CONFIRM-OFF", application.ConfirmKindOutbound, true)
	if again.Outcome() != application.WaveExisting {
		t.Fatalf("重放 %s", again.Outcome())
	}
}

func TestAnOwnerLabelConfirmsWhenTheLabelCountMatches(t *testing.T) {
	world, handler, request := packedOutbound(t, "REQ-CONFIRM-LABEL", domain.DispatchOwnerLabel, "", 4)
	if world.registerLabel(t, handler, request, 1, "EXT-1").Outcome() != application.WaveStrategyRegistered {
		t.Fatal("面单没有登记")
	}
	expired := confirmCommand(t, request, "SYN-FACT-LABEL-LATE", application.ConfirmKindOutbound, false)
	expired.ValidUntil = preAdviceNumberingInstant
	if world.confirmCommand(t, handler, expired).Outcome() != application.WaveWithheld || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 4 {
		t.Fatal("出区间仍交出了")
	}
	confirmed := world.confirmOutbound(t, handler, request, "SYN-FACT-LABEL", application.ConfirmKindOutbound, false)
	if confirmed.Outcome() != application.WaveConfirmed || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 {
		t.Fatalf("自备面单 %s 待出库 %d", confirmed.Outcome(), world.stockUnitAt(t, "SYN-LOC-OUT", "件"))
	}
}

func TestAConfirmationCorrectionChangesTheTimeOrWaitsForReplenishment(t *testing.T) {
	world, handler, request := packedOutbound(t, "REQ-CORRECT", domain.DispatchOwnerLabel, "", 4)
	if world.registerLabel(t, handler, request, 1, "EXT-CORRECT").Outcome() != application.WaveStrategyRegistered {
		t.Fatal("面单没有登记")
	}
	if world.confirmOutbound(t, handler, request, "SYN-FACT-CORRECT", application.ConfirmKindOutbound, false).Outcome() != application.WaveConfirmed {
		t.Fatal("确认没有成立")
	}
	counts, err := adapter.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	handler = handler.WithApprovalDuties(counts)
	corrected := world.correctConfirmation(t, handler, "SYN-FACT-CORRECT", "operator-1", preAdviceNumberingInstant.Add(time.Minute), false)
	if corrected.Outcome() != application.WaveCorrected || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 {
		t.Fatalf("改时点 %s 待出库 %d", corrected.Outcome(), world.stockUnitAt(t, "SYN-LOC-OUT", "件"))
	}
	if world.correctConfirmation(t, handler, "SYN-FACT-CORRECT", "operator-1", preAdviceNumberingInstant.Add(2*time.Minute), false).Outcome() != application.WaveConflict {
		t.Fatal("同一版又能改一次")
	}

	againWorld, againHandler, againRequest := packedOutbound(t, "REQ-INVALID", domain.DispatchOwnerLabel, "", 4)
	if againWorld.registerLabel(t, againHandler, againRequest, 1, "EXT-INVALID").Outcome() != application.WaveStrategyRegistered {
		t.Fatal("面单没有登记")
	}
	if againWorld.confirmOutbound(t, againHandler, againRequest, "SYN-FACT-INVALID", application.ConfirmKindOutbound, false).Outcome() != application.WaveConfirmed {
		t.Fatal("确认没有成立")
	}
	againCounts, err := adapter.NewCounts(againWorld.db)
	if err != nil {
		t.Fatal(err)
	}
	againHandler = againHandler.WithApprovalDuties(againCounts)
	if againWorld.correctConfirmation(t, againHandler, "SYN-FACT-INVALID", "operator-1", time.Time{}, true).Outcome() != application.WavePending {
		t.Fatal("判不成立没有停在待回补")
	}
	if againWorld.approveReplenishment(t, againHandler, "SYN-FACT-INVALID", "operator-2").Outcome() != application.WaveNotConfigured || againWorld.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 {
		t.Fatal("没登记批准职责却回补了")
	}
	if err := againWorld.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		return againCounts.SaveDuty(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ports.ApprovalDuty{
			Revision: 1, Distinct: true, Operator: valueOf(t, domain.NewRegistrant, "operator-1"), RegisteredAt: preAdviceNumberingInstant,
		})
	}); err != nil {
		t.Fatal(err)
	}
	if againWorld.approveReplenishment(t, againHandler, "SYN-FACT-INVALID", "operator-1").Outcome() != application.WaveDutyBlocked {
		t.Fatal("提交人把自己的回补批了")
	}
	posted := againWorld.approveReplenishment(t, againHandler, "SYN-FACT-INVALID", "operator-2")
	if posted.Outcome() != application.WavePosted || againWorld.stockUnitAt(t, "SYN-LOC-OUT", "件") != 4 || requestStatus(t, againWorld, againRequest) != "PACKED" {
		t.Fatalf("回补 %s 待出库 %d 状态 %s", posted.Outcome(), againWorld.stockUnitAt(t, "SYN-LOC-OUT", "件"), requestStatus(t, againWorld, againRequest))
	}
}

func TestACutoffReleasesWhatHasNotLeftAndRefusesAConfirmedParcel(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 8)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	outbound := world.outbound(t)
	reserved := world.acceptOutbound(t, outbound, outboundCommand(t, "REQ-CUT-RES", 4))
	reservedRequest, ok := reserved.Request()
	if reserved.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", reserved.Outcome())
	}
	handler := world.waves(t)
	if world.cutoff(t, handler, reservedRequest.ID(), 0, "cutoff-reserved").Outcome() != application.CutoffCancelled || reservationOf(t, world, reservedRequest.ID()) != 0 || requestStatus(t, world, reservedRequest.ID()) != "CANCELLED" {
		t.Fatalf("未分配截单 预占 %d 状态 %s", reservationOf(t, world, reservedRequest.ID()), requestStatus(t, world, reservedRequest.ID()))
	}
	if world.cutoff(t, handler, reservedRequest.ID(), 0, "cutoff-reserved").Outcome() != application.CutoffExisting {
		t.Fatal("同一撤回没有重放")
	}

	allocated := world.acceptOutbound(t, outbound, outboundCommand(t, "REQ-CUT-ALLOC", 4))
	allocatedRequest, _ := allocated.Request()
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-CUT", allocatedRequest.ID())
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 1 {
		t.Fatalf("分配 %s", released.Outcome())
	}
	if world.cutoff(t, handler, allocatedRequest.ID(), 0, "cutoff-allocated").Outcome() != application.CutoffCancelled || requestStatus(t, world, allocatedRequest.ID()) != "CANCELLED" {
		t.Fatal("已分配未拣没有整份取消")
	}
	late := world.pick(t, handler, released.Tasks()[0].ID, 4)
	if late.Outcome() != application.WaveRecorded || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 4 || returnTasks(t, world) != 1 {
		t.Fatalf("晚到拣货 %s 待出库 %d 回库 %d", late.Outcome(), world.stockUnitAt(t, "SYN-LOC-OUT", "件"), returnTasks(t, world))
	}

	packedWorld, packedHandler, packedRequest := packedOutbound(t, "REQ-CUT-PACK", domain.DispatchOwnerLabel, "", 4)
	if packedWorld.registerLabel(t, packedHandler, packedRequest, 1, "EXT-CUT").Outcome() != application.WaveStrategyRegistered {
		t.Fatal("面单没有登记")
	}
	if packedWorld.confirmOutbound(t, packedHandler, packedRequest, "SYN-FACT-CUT", application.ConfirmKindOutbound, false).Outcome() != application.WaveConfirmed {
		t.Fatal("确认没有成立")
	}
	if packedWorld.cutoff(t, packedHandler, packedRequest, 1, "cutoff-confirmed").Outcome() != application.CutoffStatusMismatch || packedWorld.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 {
		t.Fatal("已出库确认仍被截回")
	}
}

func TestAReturnMovesPickedGoodsBackAndARefusedShipmentStaysPacked(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 4)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-RETURN", 4))
	request, ok := accepted.Request()
	if !ok {
		t.Fatal("没有请求")
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-RETURN", request.ID())
	if world.pick(t, handler, released.Tasks()[0].ID, 4).Outcome() != application.WavePicked {
		t.Fatal("拣货没有成立")
	}
	if world.cutoff(t, handler, request.ID(), 0, "cutoff-return").Outcome() != application.CutoffReturnRequired || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 4 {
		t.Fatal("已拣出的没有停在待回库")
	}
	var returnID string
	if err := world.pool.QueryRow(t.Context(),
		`SELECT return_id FROM warehouse_fulfillment.return_task WHERE tenant_id = $1 AND status = 'OPEN'`,
		"SYN-TENANT-01").Scan(&returnID); err != nil {
		t.Fatal(err)
	}
	returned := world.completeReturn(t, handler, returnID, "SYN-FACT-RETURN", "SYN-LOC-STORE")
	if returned.Outcome() != application.WaveRecorded || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 4 || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 || requestStatus(t, world, request.ID()) != "CANCELLED" {
		t.Fatalf("回库 %s 存储 %d 待出库 %d 状态 %s", returned.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-OUT", "件"), requestStatus(t, world, request.ID()))
	}

	packedWorld, packedHandler, packedRequest := packedOutbound(t, "REQ-HELD", domain.DispatchProductNetwork, "SYN-SERVICE", 4)
	if _, err := packedWorld.pool.Exec(t.Context(),
		`UPDATE warehouse_fulfillment.outbound_parcel SET shipment_request_id = 'SHR-HELD'
		  WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1`, packedRequest.String()); err != nil {
		t.Fatal(err)
	}
	packedHandler = packedHandler.WithShipmentReleaser(scriptedReleaser{word: application.ShipmentRefused})
	if packedWorld.cutoff(t, packedHandler, packedRequest, 1, "cutoff-refused").Outcome() != application.CutoffRefused || packedWorld.stockUnitAt(t, "SYN-LOC-OUT", "件") != 4 {
		t.Fatal("不放行的委托被拆了")
	}
	packedHandler = packedHandler.WithShipmentReleaser(scriptedReleaser{word: application.ShipmentReleased})
	if packedWorld.cutoff(t, packedHandler, packedRequest, 1, "cutoff-released").Outcome() != application.CutoffCancelled {
		t.Fatal("放行后没有截回")
	}
}

func TestPickAndReturnRefuseDeactivatedTargets(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedStorageStock(t, 4)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
		spec.Deactivated = true
	}))
	accepted := world.acceptOutbound(t, world.outbound(t), outboundCommand(t, "REQ-PICK-OFF", 4))
	request, _ := accepted.Request()
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-PICK-OFF", request.ID())
	if world.pick(t, handler, released.Tasks()[0].ID, 4).Outcome() != application.WaveContentInvalid {
		t.Fatal("停用暂存库位不应拣入")
	}
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 2, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	if world.pick(t, handler, released.Tasks()[0].ID, 4).Outcome() != application.WavePicked {
		t.Fatal("启用后应能拣入")
	}
	if world.cutoff(t, handler, request.ID(), 0, "cutoff-pick-off").Outcome() != application.CutoffReturnRequired {
		t.Fatal("截单没有生成回库")
	}
	var returnID string
	if err := world.pool.QueryRow(t.Context(),
		`SELECT return_id FROM warehouse_fulfillment.return_task WHERE tenant_id = $1 AND status = 'OPEN'`,
		"SYN-TENANT-01").Scan(&returnID); err != nil {
		t.Fatal(err)
	}
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 2, func(spec *domain.ZoneContentSpec) {
		spec.Deactivated = true
	}))
	if world.completeReturn(t, handler, returnID, "SYN-FACT-RET-OFF", "SYN-LOC-STORE").Outcome() != application.WaveContentInvalid || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 4 {
		t.Fatal("停用存储库区不应回库入账")
	}
}

type scriptedReleaser struct{ word string }

func (releaser scriptedReleaser) Release(context.Context, string, string, string, string, string, int) (string, error) {
	return releaser.word, nil
}

func TestAPickupHandoverStaysUnconfiguredUntilACredentialIsRegistered(t *testing.T) {
	world, handler, request := packedOutbound(t, "REQ-CONFIRM-PICKUP", domain.DispatchPickup, "", 4)
	online := world.confirmOutbound(t, handler, request, "SYN-FACT-PICKUP", application.ConfirmKindPickup, false)
	if online.Outcome() != application.WaveNotConfigured || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 4 {
		t.Fatalf("自提 %s 待出库 %d", online.Outcome(), world.stockUnitAt(t, "SYN-LOC-OUT", "件"))
	}
}

func TestAPickupHandoverConfirmsWhenTheCredentialIsWrittenOnIt(t *testing.T) {
	world, handler, request := packedOutbound(t, "REQ-CONFIRM-PICKUP-OK", domain.DispatchPickup, "", 4)
	command := confirmCommand(t, request, "SYN-FACT-PICKUP-OK", application.ConfirmKindPickup, false)
	command.ValidFrom = time.Time{}
	command.ValidUntil = time.Time{}
	command.PickupParty = "演示提货人"
	command.PickupCredential = "SYN-PICKUP-1"
	if world.confirmCommand(t, handler, command).Outcome() != application.WaveConfirmed || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 || requestStatus(t, world, request) != "SHIPPED" {
		t.Fatal("写了自提凭证、没带放行区间，应当确认出库")
	}
}

func TestOutboundConfirmationHandoffRefusesToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	store, err := outbox.NewStore(world.db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := adapter.NewOutboxOutboundConfirmationHandoff(world.db, store, world.clock)
	if err != nil {
		t.Fatal(err)
	}
	err = handoff.HandOffOutboundConfirmation(context.Background(), "SYN-TENANT-01", "SYN-FACT-1", "DPCL-1", preAdviceNumberingInstant, "1", "")
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HandOffOutboundConfirmation：%v", err)
	}
}

func packedOutbound(t *testing.T, key string, dispatch domain.DispatchMethod, service string, quantity int64) (receiptWorld, *application.WaveHandler, domain.OutboundRequestID) {
	t.Helper()
	world := newReceiptWorld(t)
	world.seedStorageStock(t, quantity)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	command := outboundCommand(t, key, quantity)
	command.Dispatch = dispatch
	command.Service = service
	accepted := world.acceptOutbound(t, world.outbound(t), command)
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	released := world.releaseWave(t, handler, "WAVE-"+key, request.ID())
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 1 {
		t.Fatalf("分配 %s", released.Outcome())
	}
	if world.pick(t, handler, released.Tasks()[0].ID, quantity).Outcome() != application.WavePicked {
		t.Fatal("拣货没有成立")
	}
	if world.checkOutbound(t, handler, request.ID(), "SYN-CHECK-"+key, quantity).Outcome() != application.WaveChecked {
		t.Fatal("复核没有成立")
	}
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		result, packErr := handler.Pack(ctx, application.PackCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, "SYN-PACK-"+key),
			Request: request.ID(), Parcels: []application.PackParcelCommand{parcelOf(t, 1, quantity)},
		})
		if packErr != nil {
			return packErr
		}
		if result.Outcome() != application.WavePacked {
			return errors.New(result.Outcome().String())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("打包：%v", err)
	}
	return world, handler, request.ID()
}

func (world receiptWorld) confirmOutbound(t *testing.T, handler *application.WaveHandler, request domain.OutboundRequestID, fact, kind string, offline bool) application.WaveResult {
	t.Helper()
	return world.confirmCommand(t, handler, confirmCommand(t, request, fact, kind, offline))
}

func confirmCommand(t *testing.T, request domain.OutboundRequestID, fact, kind string, offline bool) application.ConfirmCommand {
	t.Helper()
	return application.ConfirmCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, fact),
		Request: request, Sequence: 1, Kind: kind, Offline: offline, OccurredAt: preAdviceNumberingInstant,
		ValidFrom: preAdviceNumberingInstant.Add(-time.Hour), ValidUntil: preAdviceNumberingInstant.Add(time.Hour),
	}
}

func (world receiptWorld) confirmCommand(t *testing.T, handler *application.WaveHandler, command application.ConfirmCommand) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var confirmErr error
		result, confirmErr = handler.Confirm(ctx, command)
		return confirmErr
	})
	if err != nil {
		t.Fatalf("出库确认：%v", err)
	}
	return result
}

func (world receiptWorld) correctConfirmation(t *testing.T, handler *application.WaveHandler, fact, operator string, occurred time.Time, invalidate bool) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var correctErr error
		result, correctErr = handler.Correct(ctx, application.CorrectConfirmationCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Operator: valueOf(t, domain.NewRegistrant, operator),
			FactID: valueOf(t, domain.NewPickFactID, fact), OccurredAt: occurred, Invalidate: invalidate,
		})
		return correctErr
	})
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	return result
}

func (world receiptWorld) approveReplenishment(t *testing.T, handler *application.WaveHandler, fact, operator string) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var approveErr error
		result, approveErr = handler.ApproveReplenishment(ctx, application.ApproveReplenishmentCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Operator: valueOf(t, domain.NewRegistrant, operator),
			FactID: valueOf(t, domain.NewPickFactID, fact),
		})
		return approveErr
	})
	if err != nil {
		t.Fatalf("回补：%v", err)
	}
	return result
}

func (world receiptWorld) completeReturn(t *testing.T, handler *application.WaveHandler, returnID, fact, target string) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var returnErr error
		result, returnErr = handler.Return(ctx, application.ReturnCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, fact),
			ReturnID: returnID, Target: valueOf(t, domain.NewLocationCode, target),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		})
		return returnErr
	})
	if err != nil {
		t.Fatalf("回库：%v", err)
	}
	return result
}

func (world receiptWorld) cutoff(t *testing.T, handler *application.WaveHandler, request domain.OutboundRequestID, sequence int, key string) application.CutoffResult {
	t.Helper()
	var result application.CutoffResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var cutoffErr error
		result, cutoffErr = handler.Cutoff(ctx, application.CutoffCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
			Request: request, Sequence: sequence, Key: key, Origin: domain.OutboundOriginCustomer,
		})
		return cutoffErr
	})
	if err != nil {
		t.Fatalf("截单：%v", err)
	}
	return result
}

func reservationOf(t *testing.T, world receiptWorld, request domain.OutboundRequestID) int64 {
	t.Helper()
	var quantity int64
	err := world.pool.QueryRow(t.Context(),
		`SELECT COALESCE(SUM(quantity), 0) FROM warehouse_fulfillment.outbound_reservation WHERE tenant_id = $1 AND request_id = $2`,
		"SYN-TENANT-01", request.String()).Scan(&quantity)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}

func returnTasks(t *testing.T, world receiptWorld) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.return_task WHERE tenant_id = $1`, "SYN-TENANT-01").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) registerLabel(t *testing.T, handler *application.WaveHandler, request domain.OutboundRequestID, sequence int, number string) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var registerErr error
		result, registerErr = handler.RegisterOwnerLabel(ctx, application.RegisterOwnerLabelCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Request: request, Sequence: sequence, ExternalNumber: number,
		})
		return registerErr
	})
	if err != nil {
		t.Fatalf("登记面单：%v", err)
	}
	return result
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
