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

// Covers: AT-WF-069 至 AT-WF-078 里提交、重放、仓不受理、内容不合、批准后同笔入账、批准时重判与驳回不复活。
// 批准口与批准职责规则不在这一包。
func TestStockImportSubmitPostAndReject(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "存储")
		spec.Usage = domain.ZoneUsageStorage
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-PICK", 1, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "拣选")
		spec.Usage = domain.ZoneUsagePicking
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-PICK", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-PICK")
	}))
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-AGENT", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Kind = domain.WarehouseKindAgent
	}))
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-OFF", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Deactivated = true
	}))

	imports, err := adapter.NewStockImports(world.db)
	if err != nil {
		t.Fatal(err)
	}
	poster := application.NewPostStockMovementHandler(world.ledger, world.warehouses, world.locations, world.zones, world.goods, world.stockIDs, fixedClock{at: baseInstant})
	handler := application.NewStockImportHandler(
		imports, presentOwnerAccounts{}, world.warehouses, world.locations, world.zones, world.goods,
		world.cartons, world.cartonIDs, poster, fixedClock{at: baseInstant},
	)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	submitter := valueOf(t, domain.NewRegistrant, "op-submit")
	posterID := valueOf(t, domain.NewRegistrant, "op-approve")

	submit := func(command application.SubmitStockImportCommand) application.StockImportResult {
		t.Helper()
		var result application.StockImportResult
		err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			var runErr error
			result, runErr = handler.Submit(ctx, command)
			return runErr
		})
		if err != nil {
			t.Fatalf("提交：%v", err)
		}
		return result
	}
	post := func(id domain.StockImportID, at time.Time) application.StockImportResult {
		t.Helper()
		handler := application.NewStockImportHandler(
			imports, presentOwnerAccounts{}, world.warehouses, world.locations, world.zones, world.goods,
			world.cartons, world.cartonIDs,
			application.NewPostStockMovementHandler(world.ledger, world.warehouses, world.locations, world.zones, world.goods, world.stockIDs, fixedClock{at: at}),
			fixedClock{at: at},
		)
		var result application.StockImportResult
		err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			var runErr error
			result, runErr = handler.Post(ctx, application.PostStockImportCommand{Tenant: tenant, ID: id, Poster: posterID})
			return runErr
		})
		if err != nil {
			t.Fatalf("入账：%v", err)
		}
		return result
	}

	good := goodsLine(t, "SYN-LOC-STORE", "SYN-GOODS-01", "件", domain.StockStateGood, 4, "", "", "", time.Time{})
	command := application.SubmitStockImportCommand{
		Tenant: tenant, ID: valueOf(t, domain.NewStockImportID, "SYN-IMP-01"), Owner: owner,
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Reason:    valueOf(t, domain.NewStockImportReason, "系统切换"),
		Goods:     []domain.StockImportGoodsLine{good}, Submitter: submitter,
	}
	first := submit(command)
	if first.Outcome() != application.StockImportSubmitted {
		t.Fatalf("提交 = %s", first.Outcome())
	}
	if again := submit(command); again.Outcome() != application.StockImportExistingResult {
		t.Fatalf("重放 = %s", again.Outcome())
	}
	changed := command
	changed.Reason = valueOf(t, domain.NewStockImportReason, "另一原因")
	if conflict := submit(changed); conflict.Outcome() != application.StockImportSourceConflict {
		t.Fatalf("换内容 = %s", conflict.Outcome())
	}
	missing := command
	missing.ID = valueOf(t, domain.NewStockImportID, "SYN-IMP-MISSING")
	missing.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-NONE")
	if result := submit(missing); result.Outcome() != application.StockImportWarehouseNotFound {
		t.Fatalf("无仓 = %s", result.Outcome())
	}
	agent := missing
	agent.ID = valueOf(t, domain.NewStockImportID, "SYN-IMP-AGENT")
	agent.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT")
	if result := submit(agent); result.Outcome() != application.StockImportWarehouseNotAccepting {
		t.Fatalf("代理仓 = %s", result.Outcome())
	} else if reason, ok := result.NotAcceptingReason(); !ok || reason != application.WarehouseNotAcceptingAgent {
		t.Fatalf("代理仓理由 = %s", reason)
	}
	off := missing
	off.ID = valueOf(t, domain.NewStockImportID, "SYN-IMP-OFF")
	off.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-OFF")
	if result := submit(off); result.Outcome() != application.StockImportWarehouseNotAccepting {
		t.Fatalf("停用仓 = %s", result.Outcome())
	}

	badQty := command
	badQty.ID = valueOf(t, domain.NewStockImportID, "SYN-IMP-QTY")
	badQty.Goods = []domain.StockImportGoodsLine{goodsLine(t, "SYN-LOC-STORE", "SYN-GOODS-01", "件", domain.StockStateGood, 0, "", "", "", time.Time{})}
	if result := submit(badQty); result.Outcome() != application.StockImportContentInvalid || !hasIssue(result, application.StockImportIssueQuantityNotPositive) {
		t.Fatalf("数量为零 = %s %+v", result.Outcome(), result.Issues())
	}
	badZone := command
	badZone.ID = valueOf(t, domain.NewStockImportID, "SYN-IMP-ZONE")
	badZone.Goods = []domain.StockImportGoodsLine{goodsLine(t, "SYN-LOC-STORE", "SYN-GOODS-01", "件", domain.StockStateDefective, 1, "", "", "", time.Time{})}
	if result := submit(badZone); result.Outcome() != application.StockImportContentInvalid || !hasIssue(result, application.StockImportIssueZoneIncompatible) {
		t.Fatalf("次品进存储 = %s %+v", result.Outcome(), result.Issues())
	}
	receiving := command
	receiving.ID = valueOf(t, domain.NewStockImportID, "SYN-IMP-RCV")
	receiving.Goods = []domain.StockImportGoodsLine{goodsLine(t, "SYN-LOC-RCV", "SYN-GOODS-01", "件", domain.StockStateGood, 1, "", "", "", time.Time{})}
	if result := submit(receiving); result.Outcome() != application.StockImportSubmitted {
		t.Fatalf("良品进收货暂存 = %s", result.Outcome())
	}
	cartonPick := command
	cartonPick.ID = valueOf(t, domain.NewStockImportID, "SYN-IMP-CARTON-PICK")
	cartonPick.Goods = nil
	cartonPick.Cartons = []domain.StockImportCartonLine{cartonLine(t, "SYN-LOC-PICK", "SYN-CARTON-PICK", time.Time{})}
	if result := submit(cartonPick); result.Outcome() != application.StockImportContentInvalid || !hasIssue(result, application.StockImportIssueZoneIncompatible) {
		t.Fatalf("整箱进拣选 = %s %+v", result.Outcome(), result.Issues())
	}

	posted := post(command.ID, baseInstant.Add(time.Hour))
	if posted.Outcome() != application.StockImportPosted {
		t.Fatalf("入账 = %s %+v", posted.Outcome(), posted.Issues())
	}
	item, _ := posted.Import()
	if item.GoodsMovementID().String() == "" || item.Status() != domain.StockImportPosted {
		t.Fatalf("入账后的导入 = %s %s", item.Status(), item.GoodsMovementID())
	}
	var reason string
	if err := world.pool.QueryRow(t.Context(),
		`SELECT reason FROM warehouse_fulfillment.stock_movement WHERE tenant_id = $1 AND movement_id = $2`,
		"SYN-TENANT-01", item.GoodsMovementID().String()).Scan(&reason); err != nil || reason != "TAKE_ON_IMPORT" {
		t.Fatalf("流水原因 = %s (%v)", reason, err)
	}
	if again := post(command.ID, baseInstant.Add(2*time.Hour)); again.Outcome() != application.StockImportExistingResult {
		t.Fatalf("再入账 = %s", again.Outcome())
	}

	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 2, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
		spec.Deactivated = true
	}))
	stale := command
	stale.ID = valueOf(t, domain.NewStockImportID, "SYN-IMP-STALE")
	if result := submit(stale); result.Outcome() != application.StockImportContentInvalid {
		t.Fatalf("停用库位的新提交 = %s", result.Outcome())
	}
	pendingBefore := command
	pendingBefore.ID = valueOf(t, domain.NewStockImportID, "SYN-IMP-BEFORE")
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 3, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	if result := submit(pendingBefore); result.Outcome() != application.StockImportSubmitted {
		t.Fatalf("库位重新启用前的提交 = %s %+v", result.Outcome(), result.Issues())
	}
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 4, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
		spec.Deactivated = true
	}))
	if result := post(pendingBefore.ID, baseInstant.Add(3*time.Hour)); result.Outcome() != application.StockImportNoLongerValid || !hasIssue(result, application.StockImportIssueLocationDeactivated) {
		t.Fatalf("批准时库位已停用 = %s %+v", result.Outcome(), result.Issues())
	}
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 5, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 2, func(spec *domain.ZoneContentSpec) {
		spec.Deactivated = true
	}))
	staleZone := command
	staleZone.ID = valueOf(t, domain.NewStockImportID, "SYN-IMP-ZONE-OFF")
	if result := submit(staleZone); result.Outcome() != application.StockImportContentInvalid || !hasIssue(result, application.StockImportIssueLocationDeactivated) {
		t.Fatalf("停用库区的新提交 = %s %+v", result.Outcome(), result.Issues())
	}
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 3, nil))

	rejectID := valueOf(t, domain.NewStockImportID, "SYN-IMP-REJ")
	rejectCmd := command
	rejectCmd.ID = rejectID
	rejectCmd.Goods = []domain.StockImportGoodsLine{goodsLine(t, "SYN-LOC-RCV", "SYN-GOODS-01", "件", domain.StockStateGood, 1, "", "", "", time.Time{})}
	if result := submit(rejectCmd); result.Outcome() != application.StockImportSubmitted {
		t.Fatalf("待驳回 = %s", result.Outcome())
	}
	var rejected application.StockImportResult
	if err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var runErr error
		rejected, runErr = handler.Reject(ctx, application.RejectStockImportCommand{
			Tenant: tenant, ID: rejectID, Rejector: posterID, Note: "不采用",
		})
		return runErr
	}); err != nil || rejected.Outcome() != application.StockImportRejected {
		t.Fatalf("驳回 = %s (%v)", rejected.Outcome(), err)
	}
	again := submit(rejectCmd)
	if again.Outcome() != application.StockImportExistingResult {
		t.Fatalf("已驳回重投 = %s", again.Outcome())
	}
	echoed, _ := again.Import()
	if echoed.Status() != domain.StockImportRejected {
		t.Fatalf("重投状态 = %s", echoed.Status())
	}
}

// Covers: AT-WF-072、AT-WF-074、AT-WF-075、AT-WF-076——跟踪批号与效期、贴标别名按提交时刻的平台商品码、
// 声明的入库时间不晚于提交且不同时刻各成一批、整箱入到存储库区并形成自己的入库批次。
func TestStockImportBatchAliasDeclaredTimeAndCartonPost(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "存储")
		spec.Usage = domain.ZoneUsageStorage
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	tracked := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-TRACK", 1, "REQ-GOODS-TRACK", func(spec *domain.GoodsContentSpec) {
		spec.TrackBatch = true
		spec.TrackExpiry = true
		spec.Aliases = []domain.GoodsAlias{
			domain.NewGoodsAlias(
				valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-NOW"), domain.GoodsAliasPlatformCode,
				valueOf(t, domain.NewPlatformName, "SYN-PLATFORM"),
				domain.NewAliasValidity(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)),
			),
			domain.NewGoodsAlias(
				valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-OLD"), domain.GoodsAliasPlatformCode,
				valueOf(t, domain.NewPlatformName, "SYN-PLATFORM"),
				domain.NewAliasValidity(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)),
			),
		}
	})
	if err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		saved, err := world.goods.SaveGoodsRevision(ctx, tracked)
		if err != nil {
			return err
		}
		if saved.Outcome != ports.GoodsRevisionSaved {
			return errors.New(saved.Outcome.String())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	imports, err := adapter.NewStockImports(world.db)
	if err != nil {
		t.Fatal(err)
	}
	at := baseInstant.Add(2 * time.Hour)
	handler := application.NewStockImportHandler(
		imports, presentOwnerAccounts{}, world.warehouses, world.locations, world.zones, world.goods,
		world.cartons, world.cartonIDs,
		application.NewPostStockMovementHandler(world.ledger, world.warehouses, world.locations, world.zones, world.goods, world.stockIDs, fixedClock{at: at}),
		fixedClock{at: at},
	)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	submitter := valueOf(t, domain.NewRegistrant, "op-submit")
	earlier := baseInstant.Add(time.Hour)
	later := baseInstant.Add(90 * time.Minute)

	submit := func(id string, goods []domain.StockImportGoodsLine, cartons []domain.StockImportCartonLine) application.StockImportResult {
		t.Helper()
		var result application.StockImportResult
		err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			var runErr error
			result, runErr = handler.Submit(ctx, application.SubmitStockImportCommand{
				Tenant: tenant, ID: valueOf(t, domain.NewStockImportID, id), Owner: owner, Warehouse: warehouse,
				Reason: valueOf(t, domain.NewStockImportReason, "系统切换"), Goods: goods, Cartons: cartons, Submitter: submitter,
			})
			return runErr
		})
		if err != nil {
			t.Fatalf("提交 %s：%v", id, err)
		}
		return result
	}
	trackedLine := func(alias string, declared time.Time, batch, expiry string) domain.StockImportGoodsLine {
		t.Helper()
		return goodsLine(t, "SYN-LOC-STORE", "SYN-GOODS-TRACK", "件", domain.StockStateGood, 1, batch, expiry, alias, declared)
	}
	if result := submit("SYN-IMP-NO-BATCH", []domain.StockImportGoodsLine{trackedLine("", time.Time{}, "", "")}, nil); !hasIssue(result, application.StockImportIssueBatchNumberRequired) || !hasIssue(result, application.StockImportIssueExpiryRequired) {
		t.Fatalf("缺批号效期 = %s %+v", result.Outcome(), result.Issues())
	}
	if result := submit("SYN-IMP-EXTRA-BATCH", []domain.StockImportGoodsLine{
		goodsLine(t, "SYN-LOC-STORE", "SYN-GOODS-01", "件", domain.StockStateGood, 1, "LOT-X", "", "", time.Time{}),
	}, nil); !hasIssue(result, application.StockImportIssueBatchNumberNotTracked) {
		t.Fatalf("不跟踪却带批号 = %s %+v", result.Outcome(), result.Issues())
	}
	if result := submit("SYN-IMP-OLD-ALIAS", []domain.StockImportGoodsLine{
		trackedLine("SYN-FNSKU-OLD", time.Time{}, "LOT-A", "2026-12-31"),
	}, nil); !hasIssue(result, application.StockImportIssueLabelAliasNotValid) {
		t.Fatalf("过期别名 = %s %+v", result.Outcome(), result.Issues())
	}
	if result := submit("SYN-IMP-LATE", []domain.StockImportGoodsLine{
		trackedLine("SYN-FNSKU-NOW", at.Add(time.Second), "LOT-A", "2026-12-31"),
	}, nil); !hasIssue(result, application.StockImportIssueDeclaredInboundAfterSubmission) {
		t.Fatalf("入库时间晚于提交 = %s %+v", result.Outcome(), result.Issues())
	}

	accepted := submit("SYN-IMP-BATCH", []domain.StockImportGoodsLine{
		trackedLine("SYN-FNSKU-NOW", earlier, "LOT-A", "2026-12-31"),
		trackedLine("SYN-FNSKU-NOW", earlier, "LOT-A", "2026-12-31"),
		trackedLine("", later, "LOT-A", "2026-12-31"),
		trackedLine("", time.Time{}, "LOT-A", "2026-12-31"),
	}, []domain.StockImportCartonLine{cartonLine(t, "SYN-LOC-STORE", "SYN-CARTON-IMP", earlier)})
	if accepted.Outcome() != application.StockImportSubmitted {
		t.Fatalf("分批提交 = %s %+v", accepted.Outcome(), accepted.Issues())
	}
	var posted application.StockImportResult
	if err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var runErr error
		posted, runErr = handler.Post(ctx, application.PostStockImportCommand{
			Tenant: tenant, ID: valueOf(t, domain.NewStockImportID, "SYN-IMP-BATCH"), Poster: valueOf(t, domain.NewRegistrant, "op-approve"),
		})
		return runErr
	}); err != nil || posted.Outcome() != application.StockImportPosted {
		t.Fatalf("分批入账 = %s (%v) %+v", posted.Outcome(), err, posted.Issues())
	}
	item, _ := posted.Import()
	var batchCount int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.inbound_batch WHERE tenant_id = $1 AND formed_by_movement_id = $2`,
		"SYN-TENANT-01", item.GoodsMovementID().String()).Scan(&batchCount); err != nil || batchCount != 3 {
		t.Fatalf("商品入库批次 = %d (%v)，同一时刻并成一批，不同时刻与未声明的各成一批", batchCount, err)
	}
	var cartonReason string
	var cartonBatches int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT movement.reason, count(batch.inbound_batch_id)
		   FROM warehouse_fulfillment.carton_movement movement
		   JOIN warehouse_fulfillment.carton_inbound_batch batch
		     ON batch.tenant_id = movement.tenant_id AND batch.formed_by_movement_id = movement.movement_id
		  WHERE movement.tenant_id = $1 AND movement.movement_id = $2
		  GROUP BY movement.reason`,
		"SYN-TENANT-01", item.CartonMovementID().String()).Scan(&cartonReason, &cartonBatches); err != nil || cartonReason != "TAKE_ON_IMPORT" || cartonBatches != 1 {
		t.Fatalf("整箱入账 = %s 批次 %d (%v)", cartonReason, cartonBatches, err)
	}
}

func TestStockImportsRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	store, err := adapter.NewStockImports(world.db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	id := valueOf(t, domain.NewStockImportID, "SYN-IMP-01")
	if err := store.Hold(context.Background(), tenant, id); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 Hold = %v", err)
	}
	pending, err := domain.NewPendingStockImport(
		tenant, id, valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		valueOf(t, domain.NewStockImportReason, "系统切换"),
		[]domain.StockImportGoodsLine{goodsLine(t, "SYN-LOC-STORE", "SYN-GOODS-01", "件", domain.StockStateGood, 1, "", "", "", time.Time{})},
		nil, valueOf(t, domain.NewRegistrant, "op-submit"), baseInstant,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SavePending(context.Background(), pending); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 SavePending = %v", err)
	}
	posted, err := pending.Post(valueOf(t, domain.NewRegistrant, "op-approve"), baseInstant, valueOf(t, domain.NewStockMovementID, "SYN-MOVE-01"), domain.CartonMovementID{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPosted(context.Background(), posted); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 MarkPosted = %v", err)
	}
	rejected, err := pending.Reject(valueOf(t, domain.NewRegistrant, "op-approve"), baseInstant, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRejected(context.Background(), rejected); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 MarkRejected = %v", err)
	}
}

func goodsLine(t *testing.T, location, goods, unit string, state domain.StockState, quantity int64, batch, expiry, alias string, declared time.Time) domain.StockImportGoodsLine {
	t.Helper()
	var batchNumber domain.BatchNumber
	var err error
	if batch != "" {
		batchNumber, err = domain.NewBatchNumber(batch)
		if err != nil {
			t.Fatal(err)
		}
	}
	var expiryDate domain.ExpiryDate
	if expiry != "" {
		expiryDate, err = domain.NewExpiryDate(expiry)
		if err != nil {
			t.Fatal(err)
		}
	}
	var aliasCode domain.GoodsAliasCode
	if alias != "" {
		aliasCode, err = domain.NewGoodsAliasCode(alias)
		if err != nil {
			t.Fatal(err)
		}
	}
	line, err := domain.NewStockImportGoodsLine(
		valueOf(t, domain.NewLocationCode, location), valueOf(t, domain.NewGoodsCode, goods), valueOf(t, domain.NewGoodsUnitName, unit),
		state, quantity, batchNumber, expiryDate, aliasCode, declared,
	)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func cartonLine(t *testing.T, location, number string, declared time.Time) domain.StockImportCartonLine {
	t.Helper()
	content, err := domain.NewStockImportCartonContent(valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 2)
	if err != nil {
		t.Fatal(err)
	}
	line, err := domain.NewStockImportCartonLine(
		valueOf(t, domain.NewLocationCode, location), valueOf(t, domain.NewCartonNumber, number),
		[]domain.StockImportCartonContent{content}, declared,
	)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

// Covers: 批准职责没登记时批准答未配置；须不同主体时提交人不能批自己的；另一个人批准记下修订并入账；
// 已入账再批答已有结果；驳回不比职责规则、不改账；已驳回再批答状态不符；查不到答查无此导入。
func TestStockImportApprovalUsesTheDutyRule(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "存储")
		spec.Usage = domain.ZoneUsageStorage
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	imports, err := adapter.NewStockImports(world.db)
	if err != nil {
		t.Fatal(err)
	}
	counts, err := adapter.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	submitter := valueOf(t, domain.NewRegistrant, "op-submit")
	approverID := valueOf(t, domain.NewRegistrant, "op-approve")
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	handler := application.NewStockImportHandler(
		imports, presentOwnerAccounts{}, world.warehouses, world.locations, world.zones, world.goods,
		world.cartons, world.cartonIDs,
		application.NewPostStockMovementHandler(world.ledger, world.warehouses, world.locations, world.zones, world.goods, world.stockIDs, fixedClock{at: baseInstant}),
		fixedClock{at: baseInstant},
	).WithApproval(counts, world.warehouses)
	submit := func(id string) {
		t.Helper()
		err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			_, runErr := handler.Submit(ctx, application.SubmitStockImportCommand{
				Tenant: tenant, ID: valueOf(t, domain.NewStockImportID, id),
				Owner:     valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
				Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
				Reason:    valueOf(t, domain.NewStockImportReason, "系统切换"),
				Goods:     []domain.StockImportGoodsLine{goodsLine(t, "SYN-LOC-STORE", "SYN-GOODS-01", "件", domain.StockStateGood, 4, "", "", "", time.Time{})},
				Submitter: submitter,
			})
			return runErr
		})
		if err != nil {
			t.Fatalf("提交 %s：%v", id, err)
		}
	}
	approve := func(id string, poster domain.Registrant) application.StockImportResult {
		t.Helper()
		var result application.StockImportResult
		err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			var runErr error
			result, runErr = handler.Approve(ctx, application.PostStockImportCommand{
				Tenant: tenant, ID: valueOf(t, domain.NewStockImportID, id), Poster: poster,
			})
			return runErr
		})
		if err != nil {
			t.Fatalf("批准 %s：%v", id, err)
		}
		return result
	}
	reject := func(id string) application.StockImportResult {
		t.Helper()
		var result application.StockImportResult
		err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			var runErr error
			result, runErr = handler.Reject(ctx, application.RejectStockImportCommand{
				Tenant: tenant, ID: valueOf(t, domain.NewStockImportID, id), Rejector: approverID, Note: "不收",
			})
			return runErr
		})
		if err != nil {
			t.Fatalf("驳回 %s：%v", id, err)
		}
		return result
	}

	submit("SYN-IMP-DUTY")
	if result := approve("SYN-IMP-DUTY", approverID); result.Outcome() != application.StockImportNotConfigured {
		t.Fatalf("没登记规则 = %s", result.Outcome())
	}
	if world.registerDuty(t, world.approver(t, counts), true).Outcome() != application.ApprovalDutyRegistered {
		t.Fatal("规则没登记上")
	}
	if result := approve("SYN-IMP-DUTY", submitter); result.Outcome() != application.StockImportDutyBlocked {
		t.Fatalf("提交人自批 = %s", result.Outcome())
	}
	if world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 0 {
		t.Fatalf("拦住之后库存 = %d", world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	posted := approve("SYN-IMP-DUTY", approverID)
	item, ok := posted.Import()
	if posted.Outcome() != application.StockImportPosted || !ok || item.DutyRevision() != 1 || item.Poster().String() != "op-approve" {
		t.Fatalf("批准 = %s 修订 %d 批准人 %s", posted.Outcome(), item.DutyRevision(), item.Poster())
	}
	if world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 4 {
		t.Fatalf("入账后库存 = %d", world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	if again := approve("SYN-IMP-DUTY", approverID); again.Outcome() != application.StockImportExistingResult {
		t.Fatalf("再批 = %s", again.Outcome())
	}
	if missing := approve("SYN-IMP-NONE", approverID); missing.Outcome() != application.StockImportNotFound {
		t.Fatalf("查无 = %s", missing.Outcome())
	}

	submit("SYN-IMP-REJ-DUTY")
	before := world.stockUnitAt(t, "SYN-LOC-STORE", "件")
	if result := reject("SYN-IMP-REJ-DUTY"); result.Outcome() != application.StockImportRejected {
		t.Fatalf("驳回 = %s", result.Outcome())
	}
	if world.stockUnitAt(t, "SYN-LOC-STORE", "件") != before {
		t.Fatalf("驳回改了库存 %d", world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	if result := approve("SYN-IMP-REJ-DUTY", approverID); result.Outcome() != application.StockImportStatusMismatch {
		t.Fatalf("已驳回再批 = %s", result.Outcome())
	}
}

func hasIssue(result application.StockImportResult, kind application.StockImportIssueKind) bool {
	for _, issue := range result.Issues() {
		if issue.Kind == kind {
			return true
		}
	}
	return false
}
