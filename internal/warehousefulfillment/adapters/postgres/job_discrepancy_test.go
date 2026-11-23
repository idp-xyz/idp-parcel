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

// Covers: 形态转换没折完的差额，批准按原原因把后来的件折成箱。源头不够时不改账。
func TestApprovingAFormConversionPostsTheRemainder(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-FORM-APR", "SYN-GOODS-01", "件", 24)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-FORM-APR-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 24,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("收货没有入账")
	}
	converted := world.registerJob(t, world.internalJobs(t), formCommand(t, "SYN-FACT-FORM-APR", 48, 2))
	job, ok := converted.Job()
	if converted.Outcome() != application.RegisterInternalJobRegistered || !ok || job.Lines()[0].Result().PostedInbound != 1 || job.Lines()[0].Result().Discrepancy.String() == "" {
		t.Fatalf("形态转换 %s 转入 %d", converted.Outcome(), job.Lines()[0].Result().PostedInbound)
	}
	if world.stockUnitAt(t, "SYN-LOC-RCV", "件") != 0 || world.stockUnitAt(t, "SYN-LOC-RCV", "箱") != 1 {
		t.Fatalf("转换后件 %d 箱 %d", world.stockUnitAt(t, "SYN-LOC-RCV", "件"), world.stockUnitAt(t, "SYN-LOC-RCV", "箱"))
	}
	duties := world.jobDuties(t)
	handler := world.jobApprover(t, duties)
	id := job.Lines()[0].Result().Discrepancy
	if blocked := world.approveJob(t, handler, id); blocked.Outcome() != application.ApprovalNotValid || world.stockUnitAt(t, "SYN-LOC-RCV", "箱") != 1 {
		t.Fatalf("件不够 %s 箱 %d", blocked.Outcome(), world.stockUnitAt(t, "SYN-LOC-RCV", "箱"))
	}
	more := world.adviseGoods(t, "REQ-FORM-APR-2", "SYN-GOODS-01", "件", 24)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-FORM-APR-RCV-2", more.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 24,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("补件没有入账")
	}
	posted := world.approveJob(t, handler, id)
	if posted.Outcome() != application.ApprovalPosted || world.stockUnitAt(t, "SYN-LOC-RCV", "件") != 0 || world.stockUnitAt(t, "SYN-LOC-RCV", "箱") != 2 {
		t.Fatalf("补折 %s 件 %d 箱 %d", posted.Outcome(), world.stockUnitAt(t, "SYN-LOC-RCV", "件"), world.stockUnitAt(t, "SYN-LOC-RCV", "箱"))
	}
	if world.movementCountFor(t, "UNIT_CONVERSION", id.String()) != 1 {
		t.Fatal("批准没有按形态转换入账")
	}
}

// Covers: 状态转换没转完的差额，批准按原原因把后来的待检转成良品。
func TestApprovingAConditionConversionPostsTheRemainder(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-COND-APR", "SYN-GOODS-01", "件", 3)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-COND-APR-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 3, state: domain.StockStatePendingInspection,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("待检没有入账")
	}
	converted := world.registerJob(t, world.internalJobs(t), conditionCommand(t, "SYN-FACT-COND-APR", 4, domain.StockStatePendingInspection, domain.StockStateGood))
	job, ok := converted.Job()
	if converted.Outcome() != application.RegisterInternalJobRegistered || !ok || job.Lines()[0].Result().Posted != 3 || job.Lines()[0].Result().Discrepancy.String() == "" {
		t.Fatalf("状态转换 %s 入账 %d", converted.Outcome(), job.Lines()[0].Result().Posted)
	}
	duties := world.jobDuties(t)
	handler := world.jobApprover(t, duties)
	id := job.Lines()[0].Result().Discrepancy
	if blocked := world.approveJob(t, handler, id); blocked.Outcome() != application.ApprovalNotValid || world.stockStateAt(t, "SYN-LOC-RCV", "GOOD") != 3 {
		t.Fatalf("待检不够 %s 良品 %d", blocked.Outcome(), world.stockStateAt(t, "SYN-LOC-RCV", "GOOD"))
	}
	more := world.adviseGoods(t, "REQ-COND-APR-2", "SYN-GOODS-01", "件", 1)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-COND-APR-RCV-2", more.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 1, state: domain.StockStatePendingInspection,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("补待检没有入账")
	}
	posted := world.approveJob(t, handler, id)
	if posted.Outcome() != application.ApprovalPosted || world.stockStateAt(t, "SYN-LOC-RCV", "PENDING_INSPECTION") != 0 || world.stockStateAt(t, "SYN-LOC-RCV", "GOOD") != 4 {
		t.Fatalf("补转 %s 待检 %d 良品 %d", posted.Outcome(), world.stockStateAt(t, "SYN-LOC-RCV", "PENDING_INSPECTION"), world.stockStateAt(t, "SYN-LOC-RCV", "GOOD"))
	}
	if world.movementCountFor(t, "STATE_CHANGE", id.String()) != 1 {
		t.Fatal("批准没有按状态转换入账")
	}
}

// Covers: 整箱缺箱在箱还不在账上时批准不成立；箱到了之后从所在位移到报告库位，不新起入库批次。驳回不改账。占着的整箱仍不批。
func TestApprovingAMissingCartonRelocatesItOnceItIsOnTheBook(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	handler := world.internalJobs(t)
	missing := world.registerJob(t, handler, cartonMoveCommand(t, "SYN-FACT-MISS-APR", "SYN-CARTON-LATE", "SYN-LOC-STORE"))
	missingJob, ok := missing.Job()
	if missing.Outcome() != application.RegisterInternalJobRegistered || !ok || !missingJob.Lines()[0].Result().CartonMissing {
		t.Fatalf("缺箱 %s", missing.Outcome())
	}
	duties := world.jobDuties(t)
	approver := world.jobApprover(t, duties)
	id := missingJob.Lines()[0].Result().Discrepancy
	if early := world.approveJob(t, approver, id); early.Outcome() != application.ApprovalNotValid || world.cartonBalance(t, "SYN-CARTON-LATE") != 0 {
		t.Fatalf("箱不在账 %s 余额 %d", early.Outcome(), world.cartonBalance(t, "SYN-CARTON-LATE"))
	}
	advice := world.adviseCartons(t, "REQ-MISS-APR", "SYN-CARTON-LATE")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-MISS-APR-RCV", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-LATE", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("整箱收货没有入账")
	}
	batches := world.cartonBatchCount(t)
	posted := world.approveJob(t, approver, id)
	if posted.Outcome() != application.ApprovalPosted || world.cartonAt(t, "SYN-CARTON-LATE") != "SYN-LOC-STORE" || world.cartonBatchCount(t) != batches {
		t.Fatalf("移位 %s 位置 %s 批次 %d", posted.Outcome(), world.cartonAt(t, "SYN-CARTON-LATE"), world.cartonBatchCount(t))
	}
	if world.cartonMoveReason(t, id.String()) != "INTERNAL_MOVE" {
		t.Fatal("整箱移动原因不对")
	}

	rejectedJob := world.registerJob(t, handler, cartonMoveCommand(t, "SYN-FACT-MISS-REJ", "SYN-CARTON-REJ", "SYN-LOC-STORE"))
	rejected, _ := rejectedJob.Job()
	rejectedID := rejected.Lines()[0].Result().Discrepancy
	if world.rejectJob(t, approver, rejectedID).Outcome() != application.ApprovalRejected || world.cartonBalance(t, "SYN-CARTON-REJ") != 0 {
		t.Fatal("驳回改了账")
	}
	rejectedAdvice := world.adviseCartons(t, "REQ-MISS-REJ", "SYN-CARTON-REJ")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-MISS-REJ-RCV", rejectedAdvice.ID().String(), mustCartonLine(t, "SYN-CARTON-REJ", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("驳回后的箱没有入账")
	}
	if again := world.approveJob(t, approver, rejectedID); again.Outcome() != application.ApprovalStatusMismatch || world.cartonAt(t, "SYN-CARTON-REJ") != "SYN-LOC-RCV" {
		t.Fatalf("驳回后再批 %s 位置 %s", again.Outcome(), world.cartonAt(t, "SYN-CARTON-REJ"))
	}

	heldAdvice := world.adviseCartons(t, "REQ-HELD-APR", "SYN-CARTON-HELD")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-HELD-APR-RCV", heldAdvice.ID().String(), mustCartonLine(t, "SYN-CARTON-HELD", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("占着的箱没有入账")
	}
	held := world.registerJob(t, world.internalJobsHeld(t, scriptedHolds{carton: true}), cartonMoveCommand(t, "SYN-FACT-HELD-APR", "SYN-CARTON-HELD", "SYN-LOC-STORE"))
	heldJob, _ := held.Job()
	if held.Outcome() != application.RegisterInternalJobRegistered || heldJob.Lines()[0].Result().CartonMissing || heldJob.Lines()[0].Result().Discrepancy.String() == "" {
		t.Fatalf("占着的箱 %s 缺箱 %t", held.Outcome(), heldJob.Lines()[0].Result().CartonMissing)
	}
	if blocked := world.approveJob(t, approver, heldJob.Lines()[0].Result().Discrepancy); blocked.Outcome() != application.ApprovalNotValid || world.cartonAt(t, "SYN-CARTON-HELD") != "SYN-LOC-RCV" {
		t.Fatalf("占着仍批 %s 位置 %s", blocked.Outcome(), world.cartonAt(t, "SYN-CARTON-HELD"))
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
	return application.NewJobDiscrepancyHandler(jobs, duties, world.poster(), world.ledger, application.ZeroStockHolds{}, world.clock, world.warehouses).WithCatalogues(world.goods, world.cartons, world.cartonIDs)
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

func (world receiptWorld) jobDuties(t *testing.T) ports.ApprovalDutyStore {
	t.Helper()
	duties, err := postgres.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if world.registerDuty(t, world.approver(t, duties), false).Outcome() != application.ApprovalDutyRegistered {
		t.Fatal("规则没有登记")
	}
	return duties
}

func (world receiptWorld) rejectJob(t *testing.T, handler *application.JobDiscrepancyHandler, id domain.StockDiscrepancyID) application.ApprovalResult {
	t.Helper()
	var result application.ApprovalResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var rejectErr error
		result, rejectErr = handler.Reject(ctx, application.RejectCountCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: id, Operator: valueOf(t, domain.NewRegistrant, "operator-2"), Reason: "不做",
		})
		return rejectErr
	})
	if err != nil {
		t.Fatalf("驳回差异：%v", err)
	}
	return result
}

func (world receiptWorld) movementCountFor(t *testing.T, reason, source string) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.stock_movement
		  WHERE tenant_id = $1 AND reason = $2 AND source_reference = $3`,
		"SYN-TENANT-01", reason, source).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) cartonMoveReason(t *testing.T, source string) string {
	t.Helper()
	var reason string
	err := world.pool.QueryRow(t.Context(),
		`SELECT reason FROM warehouse_fulfillment.carton_movement
		  WHERE tenant_id = $1 AND source_reference = $2`,
		"SYN-TENANT-01", source).Scan(&reason)
	if err != nil {
		t.Fatal(err)
	}
	return reason
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
