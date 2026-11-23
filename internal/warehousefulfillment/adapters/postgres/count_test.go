package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: AT-WF-144 的开始与再开始，AT-WF-147 的盘点中移不走，AT-WF-151 的少点成差额且账不变。
func TestCountLocksStockAndCompletesWithoutPosting(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-CNT", "SYN-GOODS-01", "件", 10)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-CNT-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 10,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	if world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-CNT-MOVE", 10, "SYN-LOC-RCV", "SYN-LOC-STORE")).Outcome() != application.RegisterInternalJobRegistered {
		t.Fatal("上架没有入账")
	}
	handler, store := world.counter(t)
	created := world.createCount(t, handler, "DEC-CNT")
	count, ok := created.Count()
	if created.Outcome() != application.CountCreated || !ok || count.Status() != domain.CountPending {
		t.Fatalf("创建 %s", created.Outcome())
	}
	if again := world.createCount(t, handler, "DEC-CNT"); again.Outcome() != application.CountExisting {
		t.Fatalf("重投 %s", again.Outcome())
	}
	movements := world.movementCount(t)
	started := world.startCount(t, handler, count.ID())
	if started.Outcome() != application.CountStarted || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 || world.movementCount(t) != movements {
		t.Fatalf("开始 %s 余额 %d", started.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	if second := world.startCount(t, handler, count.ID()); second.Outcome() != application.CountExisting {
		t.Fatalf("再开始 %s", second.Outcome())
	}
	held := world.registerJob(t, world.internalJobsHeld(t, countLockHolds{store}), moveCommand(t, "SYN-FACT-CNT-HELD", 10, "SYN-LOC-STORE", "SYN-LOC-RCV"))
	if held.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 {
		t.Fatalf("盘点中移库 %s 余额 %d", held.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	recorded := world.recordCount(t, handler, count.ID(), "SYN-FACT-CNT-ENTRY", 7)
	if recorded.Outcome() != application.CountRecorded {
		t.Fatalf("清点 %s", recorded.Outcome())
	}
	completed := world.completeCount(t, handler, count.ID())
	if completed.Outcome() != application.CountCounted || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 || world.countGap(t, count.ID()) != 3 {
		t.Fatalf("完成 %s 差额 %d", completed.Outcome(), world.countGap(t, count.ID()))
	}
}

func TestCountsRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	store, err := postgres.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	if err := store.HoldDecision(ctx, tenant, valueOf(t, domain.NewRequestKey, "DEC")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldDecision：%v", err)
	}
	if err := store.HoldCount(ctx, tenant, valueOf(t, domain.NewCountID, "CNT-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldCount：%v", err)
	}
	if err := store.Save(ctx, domain.Count{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Save：%v", err)
	}
	if err := store.SaveStart(ctx, domain.Count{}, nil); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveStart：%v", err)
	}
	if err := store.SaveEntry(ctx, ports.CountEntry{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveEntry：%v", err)
	}
	if err := store.SaveFinish(ctx, domain.Count{}, nil); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveFinish：%v", err)
	}
	if err := store.SaveVoid(ctx, domain.Count{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveVoid：%v", err)
	}
	if err := store.MarkApproved(ctx, tenant, valueOf(t, domain.NewStockDiscrepancyID, "SDP-1"), valueOf(t, domain.NewRegistrant, "operator-1"), preAdviceNumberingInstant, "", 1); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkApproved：%v", err)
	}
	if err := store.MarkRejected(ctx, tenant, valueOf(t, domain.NewStockDiscrepancyID, "SDP-1"), valueOf(t, domain.NewRegistrant, "operator-1"), preAdviceNumberingInstant, "不做"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkRejected：%v", err)
	}
	if err := store.SaveDuty(ctx, tenant, ports.ApprovalDuty{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveDuty：%v", err)
	}
}

func TestApprovingACountDifferencePostsTheGap(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-APR", "SYN-GOODS-01", "件", 10)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-APR-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 10,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	if world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-APR-MOVE", 10, "SYN-LOC-RCV", "SYN-LOC-STORE")).Outcome() != application.RegisterInternalJobRegistered {
		t.Fatal("上架没有入账")
	}
	handler, store := world.counter(t)
	created := world.createCount(t, handler, "DEC-APR")
	count, _ := created.Count()
	if world.startCount(t, handler, count.ID()).Outcome() != application.CountStarted {
		t.Fatal("没有开始")
	}
	if world.recordCount(t, handler, count.ID(), "SYN-FACT-APR-ENTRY", 7).Outcome() != application.CountRecorded {
		t.Fatal("没有清点")
	}
	if world.completeCount(t, handler, count.ID()).Outcome() != application.CountCounted {
		t.Fatal("没有完成")
	}
	approver := world.approver(t, store)
	id := world.onlyDifference(t, count.ID())
	blocked := world.approveDifference(t, approver, id)
	if blocked.Outcome() != application.ApprovalNotConfigured || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 {
		t.Fatalf("没规则 %s 余额 %d", blocked.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	registered := world.registerDuty(t, approver, false)
	if registered.Outcome() != application.ApprovalDutyRegistered {
		t.Fatalf("规则 %s", registered.Outcome())
	}
	posted := world.approveDifference(t, approver, id)
	if posted.Outcome() != application.ApprovalPosted || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 7 {
		t.Fatalf("批准 %s 余额 %d", posted.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	if again := world.approveDifference(t, approver, id); again.Outcome() != application.ApprovalExisting {
		t.Fatalf("再批 %s", again.Outcome())
	}
}

func (world receiptWorld) approver(t *testing.T, store *postgres.Counts) *application.CountApprovalHandler {
	t.Helper()
	return application.NewCountApprovalHandler(store, store, world.poster(), application.ZeroStockHolds{}, world.clock, world.warehouses)
}

func (world receiptWorld) approveDifference(t *testing.T, handler *application.CountApprovalHandler, id domain.StockDiscrepancyID) application.ApprovalResult {
	t.Helper()
	var result application.ApprovalResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var approveErr error
		result, approveErr = handler.Approve(ctx, application.ApproveCountCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: id, Operator: valueOf(t, domain.NewRegistrant, "operator-2"),
		})
		return approveErr
	})
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	return result
}

func (world receiptWorld) registerDuty(t *testing.T, handler *application.CountApprovalHandler, distinct bool) application.ApprovalResult {
	t.Helper()
	var result application.ApprovalResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var dutyErr error
		result, dutyErr = handler.RegisterDuty(ctx, application.RegisterApprovalDutyCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Revision: 1, Distinct: distinct,
			Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		})
		return dutyErr
	})
	if err != nil {
		t.Fatalf("规则：%v", err)
	}
	return result
}

func (world receiptWorld) onlyDifference(t *testing.T, id domain.CountID) domain.StockDiscrepancyID {
	t.Helper()
	var raw string
	err := world.pool.QueryRow(t.Context(),
		`SELECT difference_id FROM warehouse_fulfillment.count_difference WHERE tenant_id = $1 AND count_id = $2`,
		"SYN-TENANT-01", id.String()).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	return valueOf(t, domain.NewStockDiscrepancyID, raw)
}

type countLockHolds struct{ counts *postgres.Counts }

func (holds countLockHolds) CellHold(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, cell domain.StockCell) (int64, error) {
	return holds.counts.CellLock(ctx, tenant, owner, warehouse, cell)
}

func (countLockHolds) Allocated(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.StockCell) (int64, error) {
	return 0, nil
}

func (holds countLockHolds) CellLock(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, cell domain.StockCell) (int64, error) {
	return holds.counts.CellLock(ctx, tenant, owner, warehouse, cell)
}

func (countLockHolds) UnassignedReservation(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.GoodsCode, domain.StockState, domain.GoodsAliasCode, domain.BatchNumber, domain.ExpiryDate) (int64, error) {
	return 0, nil
}

func (countLockHolds) CartonHeld(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.CartonNumber) (bool, error) {
	return false, nil
}

func (world receiptWorld) counter(t *testing.T) (*application.CountHandler, *postgres.Counts) {
	t.Helper()
	store, err := postgres.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := identity.NewCountIdentities()
	if err != nil {
		t.Fatal(err)
	}
	differences, err := identity.NewStockDiscrepancyIdentities()
	if err != nil {
		t.Fatal(err)
	}
	return application.NewCountHandler(store, identities, differences, world.warehouses, world.locations, world.zones, world.goods, application.ZeroStockHolds{}, world.ledger, world.clock, world.warehouses), store
}

func (world receiptWorld) createCount(t *testing.T, handler *application.CountHandler, decision string) application.CountResult {
	t.Helper()
	return world.countCall(t, func(ctx context.Context) (application.CountResult, error) {
		return handler.Create(ctx, application.CreateCountCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), DecisionID: valueOf(t, domain.NewRequestKey, decision),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
			Locations: []domain.LocationCode{valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE")},
			Operator:  valueOf(t, domain.NewRegistrant, "operator-1"),
		})
	})
}

func (world receiptWorld) startCount(t *testing.T, handler *application.CountHandler, id domain.CountID) application.CountResult {
	t.Helper()
	return world.countCall(t, func(ctx context.Context) (application.CountResult, error) {
		return handler.Start(ctx, application.CountCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: id, Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		})
	})
}

func (world receiptWorld) recordCount(t *testing.T, handler *application.CountHandler, id domain.CountID, fact string, quantity int64) application.CountResult {
	t.Helper()
	return world.countCall(t, func(ctx context.Context) (application.CountResult, error) {
		return handler.Record(ctx, application.RecordCountCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewInternalJobFactID, fact), CountID: id,
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE"),
			OccurredAt: preAdviceNumberingInstant, Sequence: 1, Device: valueOf(t, domain.NewReceiptDeviceID, "SYN-DEVICE-01"),
			Operator: valueOf(t, domain.NewReceiptOperatorID, "operator-1"),
			Lines: []ports.CountEntryLine{{
				Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
				Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood, Quantity: quantity,
			}},
		})
	})
}

func (world receiptWorld) completeCount(t *testing.T, handler *application.CountHandler, id domain.CountID) application.CountResult {
	t.Helper()
	return world.countCall(t, func(ctx context.Context) (application.CountResult, error) {
		return handler.Complete(ctx, application.CountCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: id, Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		})
	})
}

func (world receiptWorld) countCall(t *testing.T, run func(context.Context) (application.CountResult, error)) application.CountResult {
	t.Helper()
	var result application.CountResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var callErr error
		result, callErr = run(ctx)
		return callErr
	})
	if err != nil {
		t.Fatalf("盘点：%v", err)
	}
	return result
}

func (world receiptWorld) countGap(t *testing.T, id domain.CountID) int64 {
	t.Helper()
	var book, counted int64
	err := world.pool.QueryRow(t.Context(),
		`SELECT book_quantity, counted_quantity FROM warehouse_fulfillment.count_difference WHERE tenant_id = $1 AND count_id = $2`,
		"SYN-TENANT-01", id.String()).Scan(&book, &counted)
	if err != nil {
		t.Fatal(err)
	}
	return book - counted
}
