package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: AT-WF-085、AT-WF-089、AT-WF-095、AT-WF-098、AT-WF-099 的核心。
func TestInternalJobMoveShortageAndRestore(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-MOVE", "SYN-GOODS-01", "件", 10)
	confirmed := world.confirm(t, receiptCommand(t, "SYN-FACT-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 10,
	})))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatalf("收货 %s", confirmed.Outcome())
	}
	handler := world.internalJobs(t)
	moved := world.registerJob(t, handler, moveCommand(t, "SYN-FACT-MOVE", 10, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	if moved.Outcome() != application.RegisterInternalJobRegistered {
		t.Fatalf("上架 %s 问题 %s", moved.Outcome(), jobIssueText(moved.Issues()))
	}
	job, ok := moved.Job()
	if !ok || len(job.Lines()) != 1 || job.Lines()[0].Result().Posted != 10 || job.Lines()[0].Result().Movement == "" {
		t.Fatalf("上架结果 %+v", job.Lines())
	}
	if world.stockAt(t, "SYN-LOC-STORE") != 10 || world.stockAt(t, "SYN-LOC-RCV") != 0 || world.batchCount(t, "SYN-GOODS-01") != 1 {
		t.Fatalf("上架后库存 存储 %d 暂存 %d 批次 %d", world.stockAt(t, "SYN-LOC-STORE"), world.stockAt(t, "SYN-LOC-RCV"), world.batchCount(t, "SYN-GOODS-01"))
	}
	again := world.registerJob(t, handler, moveCommand(t, "SYN-FACT-MOVE", 10, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	if again.Outcome() != application.RegisterInternalJobExisting || world.stockAt(t, "SYN-LOC-STORE") != 10 {
		t.Fatalf("重投 %s 库存 %d", again.Outcome(), world.stockAt(t, "SYN-LOC-STORE"))
	}
	conflict := world.registerJob(t, handler, moveCommand(t, "SYN-FACT-MOVE", 9, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	if conflict.Outcome() != application.RegisterInternalJobConflict || world.stockAt(t, "SYN-LOC-STORE") != 10 {
		t.Fatalf("冲突 %s 库存 %d", conflict.Outcome(), world.stockAt(t, "SYN-LOC-STORE"))
	}

	shortAdvice := world.adviseGoods(t, "REQ-SHORT", "SYN-GOODS-01", "件", 8)
	shortReceipt := world.confirm(t, receiptCommand(t, "SYN-FACT-RCV-2", shortAdvice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 8,
	})))
	if shortReceipt.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatalf("第二笔收货 %s", shortReceipt.Outcome())
	}
	partial := world.registerJob(t, handler, moveCommand(t, "SYN-FACT-PARTIAL", 10, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	partialJob, _ := partial.Job()
	if partial.Outcome() != application.RegisterInternalJobRegistered || len(partialJob.Lines()) != 1 ||
		partialJob.Lines()[0].Result().Posted != 8 || partialJob.Lines()[0].Result().ShortOnBook != 2 ||
		partialJob.Lines()[0].Result().Discrepancy.String() == "" || world.stockAt(t, "SYN-LOC-RCV") != 0 {
		t.Fatalf("短上架 %s 入账 %d 不够 %d 暂存 %d", partial.Outcome(), partialJob.Lines()[0].Result().Posted, partialJob.Lines()[0].Result().ShortOnBook, world.stockAt(t, "SYN-LOC-RCV"))
	}

	invalid := world.registerJob(t, handler, moveCommand(t, "SYN-FACT-BAD", 0, "SYN-LOC-MISSING", "SYN-LOC-STORE"))
	if invalid.Outcome() != application.RegisterInternalJobContentInvalid || len(invalid.Issues()) == 0 {
		t.Fatalf("不合 %s", invalid.Outcome())
	}
	replayInvalid := world.registerJob(t, handler, moveCommand(t, "SYN-FACT-BAD", 1, "SYN-LOC-RCV", "SYN-LOC-STORE"))
	if replayInvalid.Outcome() == application.RegisterInternalJobConflict {
		t.Fatalf("不合的事实占了身份")
	}

	before := world.stockOf(t, "SYN-GOODS-01")
	shortage := world.registerJob(t, handler, shortageCommand(t, "SYN-FACT-SHORT", 2, "SYN-LOC-STORE"))
	shortageJob, _ := shortage.Job()
	if shortage.Outcome() != application.RegisterInternalJobShortageRegistered || shortageJob.Lines()[0].Result().Discrepancy.String() == "" ||
		world.stockOf(t, "SYN-GOODS-01") != before {
		t.Fatalf("短少 %s 库存 %d want %d", shortage.Outcome(), world.stockOf(t, "SYN-GOODS-01"), before)
	}

	defectiveAdvice := world.adviseGoods(t, "REQ-DEF", "SYN-GOODS-01", "件", 5)
	defective := world.confirm(t, receiptCommand(t, "SYN-FACT-DEF", defectiveAdvice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 5, state: domain.StockStateDefective,
	})))
	if defective.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatalf("次品收货 %s", defective.Outcome())
	}
	restored := world.restore(t, handler, restoreCommand(t, "SYN-DEC-1", 5, "SYN-LOC-RCV"))
	if restored.Outcome() != application.RestoreConditionRestored {
		t.Fatalf("转良品 %s 问题 %s", restored.Outcome(), jobIssueText(restored.Issues()))
	}
	restore, ok := restored.Restore()
	if !ok || restore.Movement().String() == "" || world.stockStateAt(t, "SYN-LOC-RCV", "GOOD") != 5 || world.stockStateAt(t, "SYN-LOC-RCV", "DEFECTIVE") != 0 {
		t.Fatalf("转良品后良品 %d 次品 %d", world.stockStateAt(t, "SYN-LOC-RCV", "GOOD"), world.stockStateAt(t, "SYN-LOC-RCV", "DEFECTIVE"))
	}
	echo := world.restore(t, handler, restoreCommand(t, "SYN-DEC-1", 5, "SYN-LOC-RCV"))
	if echo.Outcome() != application.RestoreConditionExisting || world.stockStateAt(t, "SYN-LOC-RCV", "GOOD") != 5 {
		t.Fatalf("转良品重投 %s", echo.Outcome())
	}
}

func TestInternalJobFormConversion(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-FORM", "SYN-GOODS-01", "件", 48)
	confirmed := world.confirm(t, receiptCommand(t, "SYN-FACT-FORM-RCV", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 48,
	})))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatalf("收货 %s", confirmed.Outcome())
	}
	handler := world.internalJobs(t)
	unequal := world.registerJob(t, handler, formCommand(t, "SYN-FACT-FORM-BAD", 48, 1))
	if unequal.Outcome() != application.RegisterInternalJobContentInvalid {
		t.Fatalf("折合不等 %s", unequal.Outcome())
	}
	converted := world.registerJob(t, handler, formCommand(t, "SYN-FACT-FORM", 48, 2))
	job, ok := converted.Job()
	if converted.Outcome() != application.RegisterInternalJobRegistered || !ok || job.Lines()[0].Result().Posted != 48 || job.Lines()[0].Result().PostedInbound != 2 {
		t.Fatalf("形态转换 %s %+v", converted.Outcome(), job.Lines())
	}
	if world.stockUnitAt(t, "SYN-LOC-RCV", "件") != 0 || world.stockUnitAt(t, "SYN-LOC-RCV", "箱") != 2 || world.batchCount(t, "SYN-GOODS-01") != 1 {
		t.Fatalf("转换后件 %d 箱 %d 批次 %d", world.stockUnitAt(t, "SYN-LOC-RCV", "件"), world.stockUnitAt(t, "SYN-LOC-RCV", "箱"), world.batchCount(t, "SYN-GOODS-01"))
	}
}

func TestInternalJobsRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	jobs, err := adapter.NewInternalJobs(world.db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	if err := jobs.HoldFact(ctx, tenant, valueOf(t, domain.NewInternalJobFactID, "SYN-FACT")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldFact：%v", err)
	}
	if err := jobs.Save(ctx, domain.RecordedInternalJob{}, nil); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Save：%v", err)
	}
	if err := jobs.HoldDecision(ctx, tenant, valueOf(t, domain.NewConditionRestoreID, "SYN-DEC")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldDecision：%v", err)
	}
	if err := jobs.SaveDecision(ctx, domain.ConditionRestore{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveDecision：%v", err)
	}
	if err := jobs.MarkDiscrepancyApproved(ctx, tenant, valueOf(t, domain.NewStockDiscrepancyID, "SDP-1"), valueOf(t, domain.NewRegistrant, "operator-1"), preAdviceNumberingInstant, "", 1); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkDiscrepancyApproved：%v", err)
	}
	if err := jobs.MarkDiscrepancyRejected(ctx, tenant, valueOf(t, domain.NewStockDiscrepancyID, "SDP-1"), valueOf(t, domain.NewRegistrant, "operator-1"), preAdviceNumberingInstant, "不做"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkDiscrepancyRejected：%v", err)
	}
}

func (world receiptWorld) internalJobs(t *testing.T) *application.RegisterInternalJobHandler {
	t.Helper()
	return world.internalJobsHeld(t, application.ZeroStockHolds{})
}

func (world receiptWorld) internalJobsHeld(t *testing.T, holds ports.StockHolds) *application.RegisterInternalJobHandler {
	t.Helper()
	jobs, err := adapter.NewInternalJobs(world.db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := whidentity.NewStockDiscrepancyIdentities()
	if err != nil {
		t.Fatal(err)
	}
	handler := application.NewRegisterInternalJobHandler(
		jobs, jobs, world.poster(), world.ledger, world.ledger, world.cartons, world.cartonIDs,
		world.warehouses, world.locations, world.zones, world.goods, holds, identities, world.clock,
	)
	if world.charges != nil {
		handler = handler.WithCharges(world.charges)
	}
	return handler
}

func (world receiptWorld) registerJob(t *testing.T, handler *application.RegisterInternalJobHandler, command application.RegisterInternalJobCommand) application.RegisterInternalJobResult {
	t.Helper()
	var result application.RegisterInternalJobResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var registerErr error
		result, registerErr = handler.Register(ctx, command)
		return registerErr
	})
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	return result
}

func (world receiptWorld) restore(t *testing.T, handler *application.RegisterInternalJobHandler, command application.RestoreConditionCommand) application.RestoreConditionResult {
	t.Helper()
	var result application.RestoreConditionResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var restoreErr error
		result, restoreErr = handler.Restore(ctx, command)
		return restoreErr
	})
	if err != nil {
		t.Fatalf("转良品：%v", err)
	}
	return result
}

func moveCommand(t *testing.T, fact string, quantity int64, source, target string) application.RegisterInternalJobCommand {
	t.Helper()
	line, err := domain.NewInternalJobLine(domain.InternalJobLineSpec{
		Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
		Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood, Quantity: quantity,
		Source: valueOf(t, domain.NewLocationCode, source), Target: valueOf(t, domain.NewLocationCode, target),
	})
	if err != nil {
		t.Fatal(err)
	}
	return application.RegisterInternalJobCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewInternalJobFactID, fact),
		Kind: domain.InternalJobMove, Device: valueOf(t, domain.NewReceiptDeviceID, "SYN-DEVICE-01"),
		Operator: valueOf(t, domain.NewReceiptOperatorID, "operator-1"), OccurredAt: preAdviceNumberingInstant,
		DeviceSequence: 1, Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Lines: []domain.InternalJobLine{line},
	}
}

func formCommand(t *testing.T, fact string, from, to int64) application.RegisterInternalJobCommand {
	t.Helper()
	line, err := domain.NewInternalJobLine(domain.InternalJobLineSpec{
		Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
		Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood, Quantity: from,
		Source: valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"),
		ToUnit: valueOf(t, domain.NewGoodsUnitName, "箱"), ToQuantity: to,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := moveCommand(t, fact, from, "SYN-LOC-RCV", "SYN-LOC-RCV")
	command.Kind = domain.InternalJobForm
	command.Lines = []domain.InternalJobLine{line}
	return command
}

func shortageCommand(t *testing.T, fact string, quantity int64, location string) application.RegisterInternalJobCommand {
	t.Helper()
	command := moveCommand(t, fact, quantity, location, location)
	command.Kind = domain.InternalJobShortage
	line, err := domain.NewInternalJobLine(domain.InternalJobLineSpec{
		Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
		Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood, Quantity: quantity,
		Source: valueOf(t, domain.NewLocationCode, location),
	})
	if err != nil {
		t.Fatal(err)
	}
	command.Lines = []domain.InternalJobLine{line}
	return command
}

func restoreCommand(t *testing.T, id string, quantity int64, location string) application.RestoreConditionCommand {
	t.Helper()
	return application.RestoreConditionCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: valueOf(t, domain.NewConditionRestoreID, id),
		Operator: valueOf(t, domain.NewRegistrant, "operator-1"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Location: valueOf(t, domain.NewLocationCode, location),
		Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"),
		State: domain.StockStateDefective, Quantity: quantity, Reason: "质检合格",
	}
}

func (world receiptWorld) stockUnitAt(t *testing.T, location, unit string) int64 {
	t.Helper()
	var quantity int64
	err := world.pool.QueryRow(t.Context(),
		`SELECT coalesce(sum(quantity_delta), 0)
		   FROM warehouse_fulfillment.stock_movement_line
		  WHERE tenant_id = $1 AND location_code = $2 AND unit_name = $3`,
		"SYN-TENANT-01", location, unit).Scan(&quantity)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}

func (world receiptWorld) stockAt(t *testing.T, location string) int64 {
	t.Helper()
	return world.stockStateAt(t, location, "GOOD")
}

func (world receiptWorld) stockStateAt(t *testing.T, location, state string) int64 {
	t.Helper()
	var quantity int64
	err := world.pool.QueryRow(t.Context(),
		`SELECT coalesce(sum(quantity_delta), 0)
		   FROM warehouse_fulfillment.stock_movement_line
		  WHERE tenant_id = $1 AND location_code = $2 AND stock_state = $3`,
		"SYN-TENANT-01", location, state).Scan(&quantity)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}

func jobIssueText(issues []domain.InternalJobIssue) string {
	text := ""
	for _, issue := range issues {
		text += issue.Kind().String() + " "
	}
	return text
}
