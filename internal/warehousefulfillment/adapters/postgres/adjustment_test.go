package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: 库存调整提交不改账，没登记批准职责规则时批准答未配置，登记之后调减才入账。
func TestStockAdjustmentPostsOnlyWhenApproved(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-ADJ", "SYN-GOODS-01", "件", 10)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-ADJ-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 10,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	if world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-ADJ-MOVE", 10, "SYN-LOC-RCV", "SYN-LOC-STORE")).Outcome() != application.RegisterInternalJobRegistered {
		t.Fatal("上架没有入账")
	}
	handler := world.adjustments(t)
	batch := world.onlyBatch(t, "SYN-LOC-STORE")
	submitted := world.submitAdjustment(t, handler, "ADJ-1", batch, 4)
	if submitted.Outcome() != application.ApprovalSubmitted || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 {
		t.Fatalf("提交 %s 余额 %d", submitted.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	early := world.approveAdjustment(t, handler, "ADJ-1")
	if early.Outcome() != application.ApprovalNotConfigured || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 {
		t.Fatalf("没规则 %s", early.Outcome())
	}
	duties, err := postgres.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if world.registerDuty(t, world.approver(t, duties), false).Outcome() != application.ApprovalDutyRegistered {
		t.Fatal("规则没有登记")
	}
	handler = world.adjustmentsWith(t, duties)
	posted := world.approveAdjustment(t, handler, "ADJ-1")
	if posted.Outcome() != application.ApprovalPosted || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 6 {
		t.Fatalf("批准 %s 余额 %d", posted.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	if again := world.approveAdjustment(t, handler, "ADJ-1"); again.Outcome() != application.ApprovalExisting {
		t.Fatalf("再批 %s", again.Outcome())
	}
}

func TestAdjustmentsRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	store, err := postgres.NewAdjustments(world.db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	id := valueOf(t, domain.NewRequestKey, "ADJ-1")
	if err := store.Hold(ctx, tenant, id); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Hold：%v", err)
	}
	if err := store.Save(ctx, tenant, ports.StockAdjustment{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Save：%v", err)
	}
	if err := store.MarkPosted(ctx, tenant, id, valueOf(t, domain.NewRegistrant, "operator-1"), preAdviceNumberingInstant, "", 1); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkPosted：%v", err)
	}
	if err := store.MarkRejected(ctx, tenant, id, valueOf(t, domain.NewRegistrant, "operator-1"), preAdviceNumberingInstant, "不做"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkRejected：%v", err)
	}
}

func (world receiptWorld) adjustments(t *testing.T) *application.AdjustmentHandler {
	t.Helper()
	return world.adjustmentsWith(t, nil)
}

func (world receiptWorld) adjustmentsWith(t *testing.T, duties ports.ApprovalDutyStore) *application.AdjustmentHandler {
	t.Helper()
	store, err := postgres.NewAdjustments(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if duties == nil {
		counts, err := postgres.NewCounts(world.db)
		if err != nil {
			t.Fatal(err)
		}
		duties = counts
	}
	return application.NewAdjustmentHandler(
		store, duties, world.poster(), world.ledger, world.warehouses, world.locations, world.zones, world.goods,
		alwaysOwner{}, application.ZeroStockHolds{}, world.clock, world.warehouses,
	)
}

func (world receiptWorld) submitAdjustment(t *testing.T, handler *application.AdjustmentHandler, id, batch string, quantity int64) application.AdjustmentResult {
	t.Helper()
	var result application.AdjustmentResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var submitErr error
		result, submitErr = handler.Submit(ctx, application.SubmitAdjustmentCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: valueOf(t, domain.NewRequestKey, id),
			Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
			Reason: "账多了", Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
			Lines: []ports.AdjustmentLine{{
				Kind: ports.AdjustmentDecrease, Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE"),
				Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"),
				State: domain.StockStateGood, Inbound: valueOf(t, domain.NewInboundBatchID, batch), Quantity: quantity,
			}},
		})
		return submitErr
	})
	if err != nil {
		t.Fatalf("提交：%v", err)
	}
	return result
}

func (world receiptWorld) approveAdjustment(t *testing.T, handler *application.AdjustmentHandler, id string) application.AdjustmentResult {
	t.Helper()
	var result application.AdjustmentResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var approveErr error
		result, approveErr = handler.Approve(ctx, application.ApproveAdjustmentCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: valueOf(t, domain.NewRequestKey, id),
			Operator: valueOf(t, domain.NewRegistrant, "operator-2"),
		})
		return approveErr
	})
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	return result
}
