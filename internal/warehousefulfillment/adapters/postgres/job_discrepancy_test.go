package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: 上架没入账的数量留成差异；源头后来有货，批准才补进存储位。没规则时不改账。
func TestApprovingAShortPutAwayPostsTheRemainder(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-JOB-APR", "SYN-GOODS-01", "件", 4)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-JOB-APR-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 4,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	moved := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-JOB-APR-MOVE", 6, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 4 {
		t.Fatalf("上架 %s 存储 %d", moved.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	id := world.onlyJobDiscrepancy(t)
	handler := world.jobApprover(t, nil)
	if early := world.approveJob(t, handler, id); early.Outcome() != application.ApprovalNotConfigured || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 4 {
		t.Fatalf("没规则 %s", early.Outcome())
	}
	duties, err := postgres.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if world.registerDuty(t, world.approver(t, duties), false).Outcome() != application.ApprovalDutyRegistered {
		t.Fatal("规则没有登记")
	}
	handler = world.jobApprover(t, duties)
	if blocked := world.approveJob(t, handler, id); blocked.Outcome() != application.ApprovalNotValid || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 4 {
		t.Fatalf("源头没货 %s 存储 %d", blocked.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	more := world.adviseGoods(t, "REQ-JOB-APR-2", "SYN-GOODS-01", "件", 2)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-JOB-APR-RCV-2", more.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 2,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("补货没有入账")
	}
	posted := world.approveJob(t, handler, id)
	if posted.Outcome() != application.ApprovalPosted || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 6 || world.stockUnitAt(t, "SYN-LOC-RCV", "件") != 0 {
		t.Fatalf("补入账 %s 存储 %d 暂存 %d", posted.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-RCV", "件"))
	}
	if again := world.approveJob(t, handler, id); again.Outcome() != application.ApprovalExisting {
		t.Fatalf("重放 %s", again.Outcome())
	}
}

// Covers: 短少批准只在所在格调减，不从别处搬。
func TestApprovingAShortageDecreasesThatCell(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-SHORT-APR", "SYN-GOODS-01", "件", 4)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-SHORT-APR-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 4,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	moved := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-SHORT-APR-MOVE", 4, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 4 {
		t.Fatalf("上架 %s", moved.Outcome())
	}
	noted := world.registerJob(t, world.internalJobs(t), shortageCommand(t, "SYN-FACT-SHORT-APR", 2, "SYN-LOC-STORE"))
	if noted.Outcome() != application.RegisterInternalJobShortageRegistered || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 4 {
		t.Fatalf("短少 %s 库存 %d", noted.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
	duties, err := postgres.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if world.registerDuty(t, world.approver(t, duties), false).Outcome() != application.ApprovalDutyRegistered {
		t.Fatal("规则没有登记")
	}
	posted := world.approveJob(t, world.jobApprover(t, duties), world.onlyJobDiscrepancy(t))
	if posted.Outcome() != application.ApprovalPosted || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 2 {
		t.Fatalf("调减 %s 库存 %d", posted.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"))
	}
}

func (world receiptWorld) jobApprover(t *testing.T, duties ports.ApprovalDutyStore) *application.JobDiscrepancyHandler {
	t.Helper()
	jobs, err := postgres.NewInternalJobs(world.db)
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
	return application.NewJobDiscrepancyHandler(jobs, duties, world.poster(), world.ledger, application.ZeroStockHolds{}, world.clock, world.warehouses)
}

func (world receiptWorld) approveJob(t *testing.T, handler *application.JobDiscrepancyHandler, id domain.StockDiscrepancyID) application.ApprovalResult {
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
		t.Fatalf("批准差异：%v", err)
	}
	return result
}

func (world receiptWorld) onlyJobDiscrepancy(t *testing.T) domain.StockDiscrepancyID {
	t.Helper()
	var raw string
	err := world.pool.QueryRow(t.Context(),
		`SELECT discrepancy_id FROM warehouse_fulfillment.stock_discrepancy WHERE tenant_id = $1`, "SYN-TENANT-01").Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	return valueOf(t, domain.NewStockDiscrepancyID, raw)
}
