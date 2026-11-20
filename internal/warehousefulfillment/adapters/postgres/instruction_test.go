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

// Covers: AT-WF-106、AT-WF-109、AT-WF-110、AT-WF-112 里整笔入账与账上不够不入账。
func TestProcessingInstructionPostsTheWholeBatchOrLeavesItPending(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-KIT-01", 1, "REQ-KIT", func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "合成套装")
		spec.Aliases = nil
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundAsFinishedGoods
		spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 2)}
	}))
	advice := world.adviseGoods(t, "REQ-PART", "SYN-GOODS-01", "件", 4)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-PART", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 4,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("组件没有入账")
	}
	handler := world.instructions(t)
	accepted := world.acceptInstruction(t, handler, processCommand(t, "REQ-PROC", 2))
	instruction, ok := accepted.Instruction()
	if accepted.Outcome() != application.InstructionAccepted || !ok || instruction.Status() != domain.InstructionPending || instruction.FixedRevision() != 1 || len(instruction.Components()) != 1 {
		t.Fatalf("受理 %s", accepted.Outcome())
	}
	again := world.acceptInstruction(t, handler, processCommand(t, "REQ-PROC", 2))
	if again.Outcome() != application.InstructionExisting {
		t.Fatalf("重投 %s", again.Outcome())
	}
	posted := world.executeInstruction(t, handler, executeProcess(t, "SYN-FACT-PROC", instruction.ID().String(), 2, 4))
	if posted.Outcome() != application.InstructionPosted || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 2 || world.stockUnitAt(t, "SYN-LOC-RCV", "件") != 0 {
		done, _ := posted.Instruction()
		t.Fatalf("执行 %s 状态 %s 成品 %d 组件 %d", posted.Outcome(), done.Status(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-RCV", "件"))
	}
	second := world.executeInstruction(t, handler, executeProcess(t, "SYN-FACT-PROC-2", instruction.ID().String(), 1, 2))
	if second.Outcome() != application.InstructionStatusMismatch {
		t.Fatalf("第二次执行 %s", second.Outcome())
	}
	more := world.adviseGoods(t, "REQ-PART-2", "SYN-GOODS-01", "件", 2)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-PART-2", more.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 2,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("第二笔组件没有入账")
	}
	shortInstruction := world.acceptInstruction(t, handler, processCommand(t, "REQ-PROC-2", 2))
	shortID, _ := shortInstruction.Instruction()
	unavailable := world.executeInstruction(t, handler, executeProcess(t, "SYN-FACT-PROC-SHORT", shortID.ID().String(), 2, 4))
	if unavailable.Outcome() != application.InstructionUnavailable || world.stockUnitAt(t, "SYN-LOC-RCV", "件") != 2 {
		t.Fatalf("不够 %s 组件 %d", unavailable.Outcome(), world.stockUnitAt(t, "SYN-LOC-RCV", "件"))
	}
	kept, _ := unavailable.Instruction()
	if kept.Status() != domain.InstructionPending {
		t.Fatalf("不够之后指令 %s", kept.Status())
	}
}

func TestInstructionsRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	store, err := adapter.NewInstructions(world.db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	key := valueOf(t, domain.NewRequestKey, "REQ-NO-TX")
	id := valueOf(t, domain.NewInstructionID, "INS-NO-TX")
	fact := valueOf(t, domain.NewInternalJobFactID, "SYN-FACT-NO-TX")
	if err := store.HoldSubmission(ctx, tenant, key); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldSubmission：%v", err)
	}
	if err := store.HoldDecision(ctx, tenant, key); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldDecision：%v", err)
	}
	if err := store.HoldInstruction(ctx, tenant, id); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldInstruction：%v", err)
	}
	if err := store.HoldExecution(ctx, tenant, fact); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldExecution：%v", err)
	}
	if err := store.Save(ctx, domain.Instruction{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Save：%v", err)
	}
	if err := store.MarkVoid(ctx, domain.Instruction{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkVoid：%v", err)
	}
	if err := store.SaveExecution(ctx, ports.InstructionExecution{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveExecution：%v", err)
	}
	if err := store.HoldUnbox(ctx, tenant, fact); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldUnbox：%v", err)
	}
	if err := store.SaveUnbox(ctx, ports.UnboxFact{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveUnbox：%v", err)
	}
}

func (world receiptWorld) instructions(t *testing.T) *application.InstructionHandler {
	t.Helper()
	store, err := adapter.NewInstructions(world.db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := whidentity.NewInstructionIdentities()
	if err != nil {
		t.Fatal(err)
	}
	return application.NewInstructionHandler(
		store, identities, world.poster(), world.ledger, world.ledger, world.warehouses, world.locations, world.zones,
		world.goods, world.goods, alwaysOwner{}, application.ZeroStockHolds{}, world.clock, world.warehouses,
		store, world.cartons, world.cartonIDs, world.stockIDs,
	)
}

func (world receiptWorld) acceptInstruction(t *testing.T, handler *application.InstructionHandler, command application.AcceptInstructionCommand) application.InstructionResult {
	t.Helper()
	var result application.InstructionResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var acceptErr error
		result, acceptErr = handler.Accept(ctx, command)
		return acceptErr
	})
	if err != nil {
		t.Fatalf("受理：%v", err)
	}
	return result
}

func (world receiptWorld) executeInstruction(t *testing.T, handler *application.InstructionHandler, command application.ExecuteInstructionCommand) application.InstructionResult {
	t.Helper()
	var result application.InstructionResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var executeErr error
		result, executeErr = handler.Execute(ctx, command)
		return executeErr
	})
	if err != nil {
		t.Fatalf("执行：%v", err)
	}
	return result
}

type alwaysOwner struct{}

func (alwaysOwner) OwnerAccountRegistered(context.Context, domain.TenantID, domain.OwnerAccountID) (bool, error) {
	return true, nil
}

func TestWithdrawAndVoidInstructions(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-KIT-01", 1, "REQ-KIT-VOID", func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "合成套装")
		spec.Aliases = nil
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundAsFinishedGoods
		spec.Components = []domain.GoodsComponent{domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 1)}
	}))
	handler := world.instructions(t)
	first := world.acceptInstruction(t, handler, processCommand(t, "REQ-VOID-1", 1))
	firstID, _ := first.Instruction()
	voided := world.withdrawInstruction(t, handler, firstID.ID(), "REQ-WD-1")
	if voided.Outcome() != application.InstructionVoided {
		t.Fatalf("撤回 %s", voided.Outcome())
	}
	again := world.withdrawInstruction(t, handler, firstID.ID(), "REQ-WD-2")
	if again.Outcome() != application.InstructionExisting {
		t.Fatalf("再撤 %s", again.Outcome())
	}
	second := world.acceptInstruction(t, handler, processCommand(t, "REQ-VOID-2", 1))
	secondID, _ := second.Instruction()
	conflict := world.withdrawInstruction(t, handler, secondID.ID(), "REQ-WD-1")
	if conflict.Outcome() != application.InstructionConflict || secondID.Status() != domain.InstructionPending {
		t.Fatalf("撤回键冲突 %s", conflict.Outcome())
	}
	advice := world.adviseGoods(t, "REQ-VOID-PART", "SYN-GOODS-01", "件", 1)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-VOID-PART", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 1,
	}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("组件没有入账")
	}
	executed := world.executeInstruction(t, handler, executeProcess(t, "SYN-FACT-VOID-DONE", secondID.ID().String(), 1, 1))
	if executed.Outcome() != application.InstructionPosted {
		t.Fatalf("执行 %s %v", executed.Outcome(), executed.Issues())
	}
	late := world.withdrawInstruction(t, handler, secondID.ID(), "REQ-WD-3")
	if late.Outcome() != application.InstructionStatusMismatch {
		t.Fatalf("已加工再撤 %s", late.Outcome())
	}
	third := world.acceptInstruction(t, handler, application.AcceptInstructionCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Kind: domain.InstructionProcess,
		Origin: domain.InstructionOriginOperator, Goods: valueOf(t, domain.NewGoodsCode, "SYN-KIT-01"), Quantity: 1,
		DecisionID: valueOf(t, domain.NewRequestKey, "DEC-PROC-1"), Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		Reason: "仓内要做",
	})
	thirdID, _ := third.Instruction()
	if third.Outcome() != application.InstructionAccepted {
		t.Fatalf("运营发起 %s", third.Outcome())
	}
	refused := world.withdrawInstruction(t, handler, thirdID.ID(), "REQ-WD-4")
	if refused.Outcome() != application.InstructionWithdrawNotAllowed {
		t.Fatalf("撤运营发起 %s", refused.Outcome())
	}
	voidResult := world.voidInstruction(t, handler, thirdID.ID(), "不做了")
	voidedInstruction, _ := voidResult.Instruction()
	if voidResult.Outcome() != application.InstructionVoided || voidedInstruction.VoidReason() != "不做了" || voidedInstruction.VoidOrigin() != domain.InstructionOriginOperator {
		t.Fatalf("作废 %s %s", voidResult.Outcome(), voidedInstruction.VoidReason())
	}
}

func (world receiptWorld) withdrawInstruction(t *testing.T, handler *application.InstructionHandler, id domain.InstructionID, key string) application.InstructionResult {
	t.Helper()
	var result application.InstructionResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var withdrawErr error
		result, withdrawErr = handler.Withdraw(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), id, valueOf(t, domain.NewRequestKey, key), []domain.InstructionKind{domain.InstructionProcess, domain.InstructionDisassemble})
		return withdrawErr
	})
	if err != nil {
		t.Fatalf("撤回：%v", err)
	}
	return result
}

func (world receiptWorld) voidInstruction(t *testing.T, handler *application.InstructionHandler, id domain.InstructionID, reason string) application.InstructionResult {
	t.Helper()
	var result application.InstructionResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var voidErr error
		result, voidErr = handler.Void(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), id, valueOf(t, domain.NewRegistrant, "operator-1"), reason, []domain.InstructionKind{domain.InstructionProcess, domain.InstructionDisassemble})
		return voidErr
	})
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	return result
}

func processCommand(t *testing.T, key string, quantity int64) application.AcceptInstructionCommand {
	t.Helper()
	return application.AcceptInstructionCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Kind: domain.InstructionProcess,
		Origin: domain.InstructionOriginCustomer, Goods: valueOf(t, domain.NewGoodsCode, "SYN-KIT-01"), Quantity: quantity,
		RequestKey: valueOf(t, domain.NewRequestKey, key),
	}
}

func executeProcess(t *testing.T, fact, instruction string, done, input int64) application.ExecuteInstructionCommand {
	t.Helper()
	return application.ExecuteInstructionCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewInternalJobFactID, fact),
		InstructionID: valueOf(t, domain.NewInstructionID, instruction), Device: valueOf(t, domain.NewReceiptDeviceID, "SYN-DEVICE-01"),
		Operator: valueOf(t, domain.NewReceiptOperatorID, "operator-1"), OccurredAt: preAdviceNumberingInstant, DeviceSequence: 1,
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), DoneQuantity: done,
		Lines: []application.InstructionExecutionLine{
			{Direction: domain.StockOut, Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
				Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood, Quantity: input},
			{Direction: domain.StockIn, Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-KIT-01"),
				Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood, Quantity: done},
		},
	}
}
