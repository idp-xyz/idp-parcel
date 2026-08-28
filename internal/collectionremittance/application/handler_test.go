package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/application"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

func TestRegisteringTheSameInstructionTwiceIsExistingAndChangingItIsConflict(t *testing.T) {
	store := newStore()
	handler := store.handler()
	ctx := t.Context()

	command := application.RegisterInstructionCommand{
		Tenant: tenantID(t), Instruction: testInstruction(t, "SYN-INSTR-1", 12000),
	}
	if outcome, _ := handler.RegisterInstruction(ctx, command); outcome != application.OutcomeRegistered {
		t.Fatalf("首次登记 = %s", outcome)
	}
	if outcome, _ := handler.RegisterInstruction(ctx, command); outcome != application.OutcomeExisting {
		t.Fatalf("重放 = %s", outcome)
	}

	changed := application.RegisterInstructionCommand{
		Tenant: tenantID(t), Instruction: testInstruction(t, "SYN-INSTR-1", 13000),
	}
	if outcome, _ := handler.RegisterInstruction(ctx, changed); outcome != application.OutcomeContentConflict {
		t.Fatalf("换金额 = %s，要 CONTENT_CONFLICT——指令是清分归属的依据，绝不覆盖", outcome)
	}
}

// 没有代收指令就没有代收义务：无来源的本金进不了库，而这是一个说得清的业务答案，
// 不该等到撞库上的外键才折成「依赖故障」。
func TestAFactWithoutItsInstructionIsBasisMissing(t *testing.T) {
	store := newStore()
	handler := store.handler()
	ctx := t.Context()

	command := application.AcceptFactCommand{
		Tenant: tenantID(t),
		Fact:   testFact(t, "SYN-FACT-1", "SYN-INSTR-NEVER", domain.RecipientPayment, 12000),
	}
	if outcome, _ := handler.AcceptFact(ctx, command); outcome != application.OutcomeBasisMissing {
		t.Fatalf("孤儿事实 = %s，要 BASIS_MISSING", outcome)
	}
}

func TestPostingIntoALedgerThatWasNeverOpenedIsBasisMissing(t *testing.T) {
	store := newStore()
	handler := store.handler()
	ctx := t.Context()

	seedInstruction(t, store, handler, "SYN-INSTR-1", 12000)
	seedFact(t, store, handler, "SYN-FACT-1", "SYN-INSTR-1", domain.RecipientPayment, 12000)

	command := postCommand(t, "SYN-POST-1",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		12000, domain.BasisCollectionFact, "SYN-FACT-1", "2026-08-24T01:10:00Z")
	if outcome, _ := handler.PostSubledger(ctx, command); outcome != application.OutcomeBasisMissing {
		t.Fatalf("未开立账上的记账 = %s，要 BASIS_MISSING", outcome)
	}
}

// 入账位置由事实的来源层级定，不由调用方挑：挑错的那一次在库里看不出来。
func TestIntakePositionComesFromTheFactNotFromTheCaller(t *testing.T) {
	store := newStore()
	handler := store.handler()
	ctx := t.Context()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 12000)
	seedFact(t, store, handler, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 12000)

	wrongSpot := postCommand(t, "SYN-POST-WRONG",
		domain.PositionExternalSource, domain.PositionAwaitingAllocation,
		12000, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z")
	if outcome, _ := handler.PostSubledger(ctx, wrongSpot); outcome != application.OutcomeNotAccepted {
		t.Fatalf("渠道报告一步落到待清分 = %s，要 NOT_ACCEPTED", outcome)
	}

	partial := postCommand(t, "SYN-POST-PARTIAL",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		5000, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z")
	if outcome, _ := handler.PostSubledger(ctx, partial); outcome != application.OutcomeNotAccepted {
		t.Fatalf("部分入账 = %s，要 NOT_ACCEPTED——余下部分无处可表达，会以「已处理」的样子消失", outcome)
	}

	right := postCommand(t, "SYN-POST-1",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		12000, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z")
	if outcome, _ := handler.PostSubledger(ctx, right); outcome != application.OutcomeRegistered {
		t.Fatalf("正确入账 = %s", outcome)
	}
	if outcome, _ := handler.PostSubledger(ctx, right); outcome != application.OutcomeExisting {
		t.Fatalf("重放入账 = %s", outcome)
	}
}

// 全链一趟：入账 → 到账过账 → 清分 → 汇付，每一步的依据都不同，且账面始终守恒。
func TestPrincipalReachesTheCustomerOnlyThroughItsOwnBasisAtEachHop(t *testing.T) {
	store := newStore()
	handler := store.handler()
	ctx := t.Context()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 12000)
	seedFact(t, store, handler, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 12000)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 12000)

	mustPost(t, handler, postCommand(t, "SYN-POST-1",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		12000, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z"))

	// 渠道在途 → 待清分 只有真实到账那一层撑得起。
	byReport := postCommand(t, "SYN-POST-BADCREDIT",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		12000, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T02:00:00Z")
	if outcome, _ := handler.PostSubledger(ctx, byReport); outcome != application.OutcomeNotAccepted {
		t.Fatalf("凭渠道报告过账到待清分 = %s，要 NOT_ACCEPTED", outcome)
	}
	mustPost(t, handler, postCommand(t, "SYN-POST-2",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		12000, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T02:00:00Z"))

	// 清分只有一跳且只凭代收指令。
	mustPost(t, handler, postCommand(t, "SYN-POST-3",
		domain.PositionAwaitingAllocation, domain.PositionPayableToCustomer,
		12000, domain.BasisAllocation, "SYN-INSTR-1", "2026-08-24T03:00:00Z"))

	// 汇付要有批次，且批次交出主张后不再收成员。
	batch := testBatch(t, "SYN-BATCH-1")
	if outcome, _ := handler.FormBatch(ctx, application.FormBatchCommand{
		Tenant: tenantID(t), Batch: batch,
	}); outcome != application.OutcomeRegistered {
		t.Fatalf("形成批次 = %s", outcome)
	}
	mustPost(t, handler, postCommand(t, "SYN-POST-4",
		domain.PositionPayableToCustomer, domain.PositionRemitted,
		12000, domain.BasisRemittanceBatch, "SYN-BATCH-1", "2026-08-31T02:00:00Z"))

	balance, opened, err := store.LoadBalance(ctx, tenantID(t), testLedgerKey(t))
	if err != nil || !opened {
		t.Fatalf("读回账面：opened=%v err=%v", opened, err)
	}
	if balance.At(domain.PositionRemitted) != 12000 || balance.IntakeTotal() != 12000 {
		t.Fatalf("终态账面不对：已汇付 %d 入账 %d",
			balance.At(domain.PositionRemitted), balance.IntakeTotal())
	}
}

func TestABatchStopsAcceptingMembersOnceItsClaimIsHandedOver(t *testing.T) {
	store := newStore()
	handler := store.handler()
	ctx := t.Context()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 12000)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 12000)
	mustPost(t, handler, postCommand(t, "SYN-POST-1",
		domain.PositionExternalSource, domain.PositionAwaitingAllocation,
		12000, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T01:10:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-2",
		domain.PositionAwaitingAllocation, domain.PositionPayableToCustomer,
		12000, domain.BasisAllocation, "SYN-INSTR-1", "2026-08-24T03:00:00Z"))

	if outcome, _ := handler.FormBatch(ctx, application.FormBatchCommand{
		Tenant: tenantID(t), Batch: testBatch(t, "SYN-BATCH-1"),
	}); outcome != application.OutcomeRegistered {
		t.Fatalf("形成批次 = %s", outcome)
	}
	mustPost(t, handler, postCommand(t, "SYN-POST-3",
		domain.PositionPayableToCustomer, domain.PositionRemitted,
		5000, domain.BasisRemittanceBatch, "SYN-BATCH-1", "2026-08-31T02:00:00Z"))

	handOver := application.HandOverBatchCommand{Tenant: tenantID(t), Batch: testBatch(t, "SYN-BATCH-1").ID()}
	if outcome, _ := handler.HandOverBatch(ctx, handOver); outcome != application.OutcomeHandedOver {
		t.Fatalf("交出汇付主张 = %s", outcome)
	}
	if outcome, _ := handler.HandOverBatch(ctx, handOver); outcome != application.OutcomeAlreadyHandedOver {
		t.Fatalf("重复交出 = %s，要 ALREADY_HANDED_OVER", outcome)
	}

	late := postCommand(t, "SYN-POST-LATE",
		domain.PositionPayableToCustomer, domain.PositionRemitted,
		7000, domain.BasisRemittanceBatch, "SYN-BATCH-1", "2026-08-31T04:00:00Z")
	if outcome, _ := handler.PostSubledger(ctx, late); outcome != application.OutcomeBatchClosed {
		t.Fatalf("已交出批次收新成员 = %s，要 BATCH_CLOSED", outcome)
	}
}

func TestPostingMoreThanThePositionHoldsIsUnderfundedNotInvalid(t *testing.T) {
	store := newStore()
	handler := store.handler()
	ctx := t.Context()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 12000)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 12000)
	mustPost(t, handler, postCommand(t, "SYN-POST-1",
		domain.PositionExternalSource, domain.PositionAwaitingAllocation,
		12000, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T01:10:00Z"))

	tooMuch := postCommand(t, "SYN-POST-BIG",
		domain.PositionAwaitingAllocation, domain.PositionPayableToCustomer,
		12001, domain.BasisAllocation, "SYN-INSTR-1", "2026-08-24T03:00:00Z")
	if outcome, _ := handler.PostSubledger(ctx, tooMuch); outcome != application.OutcomeUnderfunded {
		t.Fatalf("透支 = %s，要 UNDERFUNDED——请求没错，账上还没那么多钱", outcome)
	}
}

// 差异事项只登事项，差额落账要另有一笔以它为依据的记账；短款不得靠缩小指令抹平。
func TestADiscrepancySettlesOnlyThroughItsOwnPosting(t *testing.T) {
	store := newStore()
	handler := store.handler()
	ctx := t.Context()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 12000)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 10000)
	mustPost(t, handler, postCommand(t, "SYN-POST-1",
		domain.PositionExternalSource, domain.PositionAwaitingAllocation,
		10000, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T01:10:00Z"))

	item := testDiscrepancy(t, "SYN-DIFF-1", "SYN-INSTR-1", domain.DiscrepancyShortfall, 2000)
	if outcome, _ := handler.RegisterDiscrepancy(ctx, application.RegisterDiscrepancyCommand{
		Tenant: tenantID(t), Item: item,
	}); outcome != application.OutcomeRegistered {
		t.Fatalf("登记差异事项 = %s", outcome)
	}

	// 登记差异事项本身没动账面。
	balance, _, err := store.LoadBalance(ctx, tenantID(t), testLedgerKey(t))
	if err != nil {
		t.Fatalf("读回账面：%v", err)
	}
	if balance.At(domain.PositionShortfall) != 0 {
		t.Fatalf("差异事项自动落了账：短款 %d", balance.At(domain.PositionShortfall))
	}

	wrongAmount := postCommand(t, "SYN-POST-DIFF-BAD",
		domain.PositionAwaitingAllocation, domain.PositionShortfall,
		1500, domain.BasisDiscrepancy, "SYN-DIFF-1", "2026-08-25T01:00:00Z")
	if outcome, _ := handler.PostSubledger(ctx, wrongAmount); outcome != application.OutcomeNotAccepted {
		t.Fatalf("差额与事项不符 = %s，要 NOT_ACCEPTED", outcome)
	}
	mustPost(t, handler, postCommand(t, "SYN-POST-DIFF",
		domain.PositionAwaitingAllocation, domain.PositionShortfall,
		2000, domain.BasisDiscrepancy, "SYN-DIFF-1", "2026-08-25T01:00:00Z"))
}

// 冲正就是原记账的反向同额，形状固定死；放开成自由移动之后，账上看不出干了什么。
func TestACorrectionIsExactlyTheReverseOfItsOriginal(t *testing.T) {
	store := newStore()
	handler := store.handler()
	ctx := t.Context()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 12000)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 12000)
	mustPost(t, handler, postCommand(t, "SYN-POST-1",
		domain.PositionExternalSource, domain.PositionAwaitingAllocation,
		12000, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T01:10:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-2",
		domain.PositionAwaitingAllocation, domain.PositionPayableToCustomer,
		12000, domain.BasisAllocation, "SYN-INSTR-1", "2026-08-24T03:00:00Z"))

	partial := postCommand(t, "SYN-POST-FIX-BAD",
		domain.PositionPayableToCustomer, domain.PositionAwaitingAllocation,
		5000, domain.BasisCorrection, "SYN-POST-2", "2026-08-24T04:00:00Z")
	if outcome, _ := handler.PostSubledger(ctx, partial); outcome != application.OutcomeNotAccepted {
		t.Fatalf("部分冲正 = %s，要 NOT_ACCEPTED", outcome)
	}

	// 入账冲不了正：去向侧没有账外位置，反向那一笔落不下来。
	reverseIntake := postCommand(t, "SYN-POST-FIX-INTAKE",
		domain.PositionAwaitingAllocation, domain.PositionExternalSource,
		12000, domain.BasisCorrection, "SYN-POST-1", "2026-08-24T04:00:00Z")
	if outcome, _ := handler.PostSubledger(ctx, reverseIntake); outcome != application.OutcomeNotAccepted {
		t.Fatalf("冲正入账 = %s，要 NOT_ACCEPTED", outcome)
	}

	mustPost(t, handler, postCommand(t, "SYN-POST-FIX",
		domain.PositionPayableToCustomer, domain.PositionAwaitingAllocation,
		12000, domain.BasisCorrection, "SYN-POST-2", "2026-08-24T04:00:00Z"))

	balance, _, err := store.LoadBalance(ctx, tenantID(t), testLedgerKey(t))
	if err != nil {
		t.Fatalf("读回账面：%v", err)
	}
	if balance.At(domain.PositionPayableToCustomer) != 0 ||
		balance.At(domain.PositionAwaitingAllocation) != 12000 {
		t.Fatalf("冲正后账面不对：应付客户 %d 待清分 %d",
			balance.At(domain.PositionPayableToCustomer),
			balance.At(domain.PositionAwaitingAllocation))
	}
	// 原记账没被删：三笔都还在。
	if balance.PostingCount() != 3 {
		t.Fatalf("记账笔数 = %d，要 3——冲正是追加，不是删除", balance.PostingCount())
	}
}

func TestDependencyFailuresAreUndecidedRatherThanAnAnswer(t *testing.T) {
	ctx := t.Context()

	writerDown := newStore()
	writerDown.writeErr = errDependencyDown
	if outcome, _ := writerDown.handler().RegisterInstruction(ctx, application.RegisterInstructionCommand{
		Tenant: tenantID(t), Instruction: testInstruction(t, "SYN-INSTR-1", 12000),
	}); outcome != application.OutcomeUndecided {
		t.Fatalf("写口故障 = %s，要 UNDECIDED", outcome)
	}

	readerDown := newStore()
	readerDown.readErr = errDependencyDown
	if outcome, _ := readerDown.handler().AcceptFact(ctx, application.AcceptFactCommand{
		Tenant: tenantID(t),
		Fact:   testFact(t, "SYN-FACT-1", "SYN-INSTR-1", domain.RecipientPayment, 12000),
	}); outcome != application.OutcomeUndecided {
		t.Fatalf("读口故障 = %s，要 UNDECIDED——查不到与查不了是两件事", outcome)
	}
}

// —— 夹具动作 ——

func openLedger(t *testing.T, store *storeDouble, handler *application.Handler) {
	t.Helper()
	if outcome, _ := handler.OpenSubledger(t.Context(), application.OpenSubledgerCommand{
		Tenant: tenantID(t), Ledger: testSubledger(t),
	}); outcome != application.OutcomeRegistered {
		t.Fatalf("开立分户账 = %s", outcome)
	}
	if len(store.subledgers) != 1 {
		t.Fatalf("分户账没落到替身里")
	}
}

func seedInstruction(
	t *testing.T, _ *storeDouble, handler *application.Handler, id string, amountMinor int64,
) {
	t.Helper()
	if outcome, _ := handler.RegisterInstruction(t.Context(), application.RegisterInstructionCommand{
		Tenant: tenantID(t), Instruction: testInstruction(t, id, amountMinor),
	}); outcome != application.OutcomeRegistered {
		t.Fatalf("登记指令 %s = %s", id, outcome)
	}
}

func seedFact(
	t *testing.T, _ *storeDouble, handler *application.Handler,
	id, instruction string, layer domain.CollectionSourceLayer, amountMinor int64,
) {
	t.Helper()
	if outcome, _ := handler.AcceptFact(t.Context(), application.AcceptFactCommand{
		Tenant: tenantID(t), Fact: testFact(t, id, instruction, layer, amountMinor),
	}); outcome != application.OutcomeRegistered {
		t.Fatalf("接受事实 %s = %s", id, outcome)
	}
}

func mustPost(t *testing.T, handler *application.Handler, command application.PostCommand) {
	t.Helper()
	outcome, err := handler.PostSubledger(t.Context(), command)
	if err != nil || outcome != application.OutcomeRegistered {
		t.Fatalf("记账 %s = %s（err=%v）", command.ID, outcome, err)
	}
}
