package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/application"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// 本文件钉认可的协议抵扣在编排上的几格（ADR-0298）：登记只钉引用不动账；落账凭登记的
// 抵扣、金额等于登记额、只自`渠道在途`到`已抵扣`；按指令判上限；到账先于抵扣；同一抵扣
// 至多落一笔、撤回走冲正且一笔至多冲正一次。

func testOffset(
	t *testing.T, id, instruction, approval string, amountMinor int64,
) domain.AgreedOffset {
	t.Helper()
	return testOffsetApprovedAt(t, id, instruction, approval, amountMinor, "2026-08-25T00:00:00Z")
}

func testOffsetApprovedAt(
	t *testing.T, id, instruction, approval string, amountMinor int64, approvedAt string,
) domain.AgreedOffset {
	t.Helper()
	offsetID, err := domain.NewAgreedOffsetID(id)
	if err != nil {
		t.Fatalf("构造抵扣标识：%v", err)
	}
	instructionID, err := domain.NewCollectionInstructionID(instruction)
	if err != nil {
		t.Fatalf("构造指令标识：%v", err)
	}
	approvalRef, err := domain.NewApprovalReference(approval)
	if err != nil {
		t.Fatalf("构造认可引用：%v", err)
	}
	offset, err := domain.RegisterAgreedOffset(domain.AgreedOffsetSpec{
		ID:          offsetID,
		Instruction: instructionID,
		Amount:      testMoney(t, amountMinor),
		Approval:    approvalRef,
		ApprovedAt:  at(t, approvedAt),
	})
	if err != nil {
		t.Fatalf("登记认可抵扣：%v", err)
	}
	return offset
}

func registerOffsetOutcome(
	t *testing.T, handler *application.Handler, offset domain.AgreedOffset,
) application.Outcome {
	t.Helper()
	outcome, err := handler.RegisterAgreedOffset(t.Context(), application.RegisterAgreedOffsetCommand{
		Tenant: tenantID(t), Offset: offset,
	})
	if err != nil {
		t.Fatalf("登记认可抵扣 %s 交回错误：%v", offset.ID(), err)
	}
	return outcome
}

func mustRegisterOffset(
	t *testing.T, handler *application.Handler, id, instruction, approval string, amountMinor int64,
) {
	t.Helper()
	if outcome := registerOffsetOutcome(t, handler, testOffset(t, id, instruction, approval, amountMinor)); outcome != application.OutcomeRegistered {
		t.Fatalf("登记认可抵扣 %s = %s", id, outcome)
	}
}

func offsetPosting(t *testing.T, id, offset string, amountMinor int64) application.PostCommand {
	t.Helper()
	return postCommand(t, id,
		domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
		amountMinor, domain.BasisAgreedOffset, offset, "2026-08-25T01:00:00Z")
}

// 场景十七（报告 100、到账 96、认可的协议抵扣 3）整条走通：抵扣落`已抵扣`，余下 1 落
// `短款`，`渠道在途`清零，账内各位置之和仍是入账 100；到账的 96 清分给客户。
// Covers: AT-CR-017
func TestScenarioSeventeenSettlesTheAgreedOffsetOutOfTransit(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 100)
	seedFact(t, store, handler, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 96)
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		100, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-CREDIT",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		96, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T02:00:00Z"))

	mustRegisterOffset(t, handler, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 3)
	if got := mustBalance(t, store); got.At(domain.PositionOffsetApplied) != 0 {
		t.Fatalf("只登记认可、没落账，已抵扣却是 %d——登记不动账", got.At(domain.PositionOffsetApplied))
	}
	mustPost(t, handler, offsetPosting(t, "SYN-POST-OFFSET", "SYN-OFFSET-1", 3))

	mustRegisterDiscrepancy(t, handler, "SYN-DIFF-1", "SYN-INSTR-1", domain.DiscrepancyShortfall, 1)
	mustPost(t, handler, postCommand(t, "SYN-POST-SHORT",
		domain.PositionInTransitAtChannel, domain.PositionShortfall,
		1, domain.BasisDiscrepancy, "SYN-DIFF-1", "2026-08-25T02:00:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-ALLOC",
		domain.PositionAwaitingAllocation, domain.PositionPayableToCustomer,
		96, domain.BasisAllocation, "SYN-INSTR-1", "2026-08-25T03:00:00Z"))

	balance := mustBalance(t, store)
	want := map[domain.FundPosition]int64{
		domain.PositionInTransitAtChannel: 0,
		domain.PositionAwaitingAllocation: 0,
		domain.PositionPayableToCustomer:  96,
		domain.PositionOffsetApplied:      3,
		domain.PositionShortfall:          1,
	}
	var sum int64
	for position, amount := range want {
		if got := balance.At(position); got != amount {
			t.Fatalf("%s = %d，要 %d", position, got, amount)
		}
		sum += balance.At(position)
	}
	if sum != 100 || balance.IntakeTotal() != 100 {
		t.Fatalf("账内位置之和 %d、入账 %d，要都是 100", sum, balance.IntakeTotal())
	}
}

// 登记只钉引用：同标识同内容是重放，同标识改内容是冲突，换标识再登记同一份认可（同
// 认可引用、同指令）也是冲突——同一笔抵扣不能凭两条记录落两次。指令不在册、币种不是
// 那本账的币种、缺格都不落册。
// Covers: AT-CR-024
func TestRegisteringAnAgreedOffsetPinsTheApprovalAndRefusesADuplicate(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 100)
	seedInstruction(t, store, handler, "SYN-INSTR-2", 100)

	first := testOffset(t, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 3)
	if got := registerOffsetOutcome(t, handler, first); got != application.OutcomeRegistered {
		t.Fatalf("登记 = %s，要 REGISTERED", got)
	}
	if got := registerOffsetOutcome(t, handler, first); got != application.OutcomeExisting {
		t.Fatalf("重放 = %s，要 EXISTING", got)
	}
	if got := registerOffsetOutcome(t, handler,
		testOffset(t, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 4)); got != application.OutcomeContentConflict {
		t.Fatalf("同标识改金额 = %s，要 CONTENT_CONFLICT", got)
	}
	// 同标识的重放要逐格相同：认可引用、指令、认可时刻任一格不同，都是同键异内容。
	for _, drift := range []struct {
		name   string
		offset domain.AgreedOffset
	}{
		{"同标识换认可引用", testOffset(t, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-9", 3)},
		{"同标识换指令", testOffset(t, "SYN-OFFSET-1", "SYN-INSTR-2", "SYN-SA-APPROVAL-1", 3)},
		{"同标识换认可时刻",
			testOffsetApprovedAt(t, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 3, "2026-08-26T00:00:00Z")},
	} {
		if got := registerOffsetOutcome(t, handler, drift.offset); got != application.OutcomeContentConflict {
			t.Fatalf("%s = %s，要 CONTENT_CONFLICT", drift.name, got)
		}
	}
	if got := registerOffsetOutcome(t, handler,
		testOffset(t, "SYN-OFFSET-1B", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 3)); got != application.OutcomeContentConflict {
		t.Fatalf("换标识再登记同一份认可 = %s，要 CONTENT_CONFLICT", got)
	}
	// 同一份认可引用落到另一条指令上是另一条记录：一次回款的抵扣常按指令拆开。
	if got := registerOffsetOutcome(t, handler,
		testOffset(t, "SYN-OFFSET-2", "SYN-INSTR-2", "SYN-SA-APPROVAL-1", 3)); got != application.OutcomeRegistered {
		t.Fatalf("同认可引用另一条指令 = %s，要 REGISTERED", got)
	}
	if got := registerOffsetOutcome(t, handler,
		testOffset(t, "SYN-OFFSET-3", "SYN-INSTR-MISSING", "SYN-SA-APPROVAL-3", 3)); got != application.OutcomeBasisMissing {
		t.Fatalf("指令不在册 = %s，要 BASIS_MISSING", got)
	}

	foreign := testOffset(t, "SYN-OFFSET-4", "SYN-INSTR-1", "SYN-SA-APPROVAL-4", 3)
	usd, err := domain.NewCurrencyCode("USD")
	if err != nil {
		t.Fatalf("构造币种：%v", err)
	}
	usdAmount, err := domain.NewMoney(usd, 3)
	if err != nil {
		t.Fatalf("构造金额：%v", err)
	}
	foreign, err = domain.RegisterAgreedOffset(domain.AgreedOffsetSpec{
		ID: foreign.ID(), Instruction: foreign.Instruction(), Amount: usdAmount,
		Approval: foreign.Approval(), ApprovedAt: foreign.ApprovedAt(),
	})
	if err != nil {
		t.Fatalf("构造异币种抵扣：%v", err)
	}
	if got := registerOffsetOutcome(t, handler, foreign); got != application.OutcomeNotAccepted {
		t.Fatalf("币种不是那本账的币种 = %s，要 NOT_ACCEPTED", got)
	}
	if got := registerOffsetOutcome(t, handler, domain.AgreedOffset{}); got != application.OutcomeNotAccepted {
		t.Fatalf("空记录 = %s，要 NOT_ACCEPTED", got)
	}

	store.readErr = errDependencyDown
	if got := registerOffsetOutcome(t, handler,
		testOffset(t, "SYN-OFFSET-5", "SYN-INSTR-1", "SYN-SA-APPROVAL-5", 3)); got != application.OutcomeUndecided {
		t.Fatalf("读口故障 = %s，要 UNDECIDED", got)
	}
}

// 落账的形状固定：自`渠道在途`到`已抵扣`、金额等于登记额；依据不在册答依据不在册。
// 抵扣不落账的各种错形都不动账。
// Covers: AT-CR-024
func TestAnOffsetPostsOnlyInItsRegisteredShape(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 100)
	seedReceived(t, store, handler, "SYN-INSTR-1", 100)
	mustRegisterOffset(t, handler, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 3)
	before := mustBalance(t, store)

	for _, attempt := range []struct {
		name    string
		command application.PostCommand
		want    application.Outcome
	}{
		{"金额大于登记额", offsetPosting(t, "SYN-POST-BIG", "SYN-OFFSET-1", 4), application.OutcomeNotAccepted},
		{"金额小于登记额", offsetPosting(t, "SYN-POST-SMALL", "SYN-OFFSET-1", 2), application.OutcomeNotAccepted},
		{"自待清分落入", postCommand(t, "SYN-POST-FROM-AWAITING",
			domain.PositionAwaitingAllocation, domain.PositionOffsetApplied,
			3, domain.BasisAgreedOffset, "SYN-OFFSET-1", "2026-08-25T01:00:00Z"), application.OutcomeNotAccepted},
		{"落进短款", postCommand(t, "SYN-POST-TO-SHORTFALL",
			domain.PositionInTransitAtChannel, domain.PositionShortfall,
			3, domain.BasisAgreedOffset, "SYN-OFFSET-1", "2026-08-25T01:00:00Z"), application.OutcomeNotAccepted},
		{"依据不在册", offsetPosting(t, "SYN-POST-GHOST", "SYN-OFFSET-GHOST", 3), application.OutcomeBasisMissing},
	} {
		if got := postOutcome(t, handler, attempt.command); got != attempt.want {
			t.Fatalf("%s = %s，要 %s", attempt.name, got, attempt.want)
		}
	}
	after := mustBalance(t, store)
	if after.PostingCount() != before.PostingCount() {
		t.Fatalf("错形的抵扣动了账：记账 %d → %d 笔", before.PostingCount(), after.PostingCount())
	}
}

// 抵扣额不超过该指令名下的在途款，也不借同账别的指令的在途款；越过答余额不足、确定
// 没落账，补报告之后重跑同一命令就落。
// Covers: AT-CR-025
func TestAnOffsetCannotBorrowAnotherInstructionsInTransit(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-X", 100)
	seedInstruction(t, store, handler, "SYN-INSTR-Y", 50)
	seedFact(t, store, handler, "SYN-FACT-X-REPORT", "SYN-INSTR-X", domain.ChannelCollectionReport, 100)
	seedFact(t, store, handler, "SYN-FACT-Y-REPORT", "SYN-INSTR-Y", domain.ChannelCollectionReport, 50)
	seedFact(t, store, handler, "SYN-FACT-X-CREDIT", "SYN-INSTR-X", domain.OperatorBankCredit, 98)
	mustPost(t, handler, postCommand(t, "SYN-POST-X-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		100, domain.BasisCollectionFact, "SYN-FACT-X-REPORT", "2026-08-24T01:10:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-Y-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		50, domain.BasisCollectionFact, "SYN-FACT-Y-REPORT", "2026-08-24T01:10:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-X-CREDIT",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		98, domain.BasisCollectionFact, "SYN-FACT-X-CREDIT", "2026-08-24T02:00:00Z"))

	// X 名下只剩 2 在途；同账 Y 有 50，按账判够，按指令判不够。
	mustRegisterOffset(t, handler, "SYN-OFFSET-X", "SYN-INSTR-X", "SYN-SA-APPROVAL-X", 3)
	over := offsetPosting(t, "SYN-POST-OFFSET-X", "SYN-OFFSET-X", 3)
	if got := postOutcome(t, handler, over); got != application.OutcomeUnderfunded {
		t.Fatalf("X 的抵扣 3 > 在途 2，借 Y 的在途款 = %s，要 UNDERFUNDED", got)
	}
	if got := mustBalance(t, store).At(domain.PositionInTransitAtChannel); got != 52 {
		t.Fatalf("渠道在途 = %d，要 52——Y 的在途款一分没动", got)
	}

	// 渠道少报了 1，补一条报告之后同一命令落得下。
	seedFact(t, store, handler, "SYN-FACT-X-REPORT-2", "SYN-INSTR-X", domain.ChannelCollectionReport, 1)
	mustPost(t, handler, postCommand(t, "SYN-POST-X-REPORT-2",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		1, domain.BasisCollectionFact, "SYN-FACT-X-REPORT-2", "2026-08-26T01:10:00Z"))
	mustPost(t, handler, over)
}

// 实际到账先于认可的抵扣：该指令有已登记而没移位的到账时抵扣等着，不先吃掉在途款；
// 到账移位之后同一命令落账。
// Covers: AT-CR-025
func TestAnOffsetWaitsForTheArrivalToMoveFirst(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 100)
	seedFact(t, store, handler, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 96)
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		100, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z"))
	mustRegisterOffset(t, handler, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 3)

	offset := offsetPosting(t, "SYN-POST-OFFSET", "SYN-OFFSET-1", 3)
	if got := postOutcome(t, handler, offset); got != application.OutcomeUnderfunded {
		t.Fatalf("到账没移位时的抵扣 = %s，要 UNDERFUNDED", got)
	}
	if got := mustBalance(t, store).At(domain.PositionOffsetApplied); got != 0 {
		t.Fatalf("没落账的抵扣却有 %d 在已抵扣", got)
	}
	mustPost(t, handler, postCommand(t, "SYN-POST-CREDIT",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		96, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T02:00:00Z"))
	mustPost(t, handler, offset)
	if got := mustBalance(t, store).At(domain.PositionOffsetApplied); got != 3 {
		t.Fatalf("到账移位后抵扣落账，已抵扣 = %d，要 3", got)
	}
}

// 到账加抵扣多于该指令的报告：先移位的到账占住在途款，抵扣整笔停住，等该指令补一条
// 报告，不先落一部分；到账的钱不被抵扣挤占。
// Covers: AT-CR-025
func TestAnOffsetBeyondTheReportLessTheArrivalStaysUnderfunded(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 100)
	seedFact(t, store, handler, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 98)
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		100, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-CREDIT",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		98, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T02:00:00Z"))
	mustRegisterOffset(t, handler, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 3)

	if got := postOutcome(t, handler, offsetPosting(t, "SYN-POST-OFFSET", "SYN-OFFSET-1", 3)); got != application.OutcomeUnderfunded {
		t.Fatalf("到账 98 + 抵扣 3 > 报告 100 的抵扣 = %s，要 UNDERFUNDED", got)
	}
	balance := mustBalance(t, store)
	if balance.At(domain.PositionOffsetApplied) != 0 || balance.At(domain.PositionInTransitAtChannel) != 2 {
		t.Fatalf("已抵扣 %d 渠道在途 %d，要 0 与 2——整笔停住，不先落一部分",
			balance.At(domain.PositionOffsetApplied), balance.At(domain.PositionInTransitAtChannel))
	}
}

// 同一条认可抵扣至多落一笔：同标识重放答已有结果（先于余额守卫——来源此后被移空
// 也一样），换记账标识再落是内容冲突。
// Covers: AT-CR-025
func TestAnOffsetPostsAtMostOnceAndReplaysAnswerExisting(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 100)
	seedReceived(t, store, handler, "SYN-INSTR-1", 100)
	// seedReceived 把 100 全部移走，补一条报告让在途有余额可抵。
	seedFact(t, store, handler, "SYN-FACT-REPORT-2", "SYN-INSTR-1", domain.ChannelCollectionReport, 3)
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT-2",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		3, domain.BasisCollectionFact, "SYN-FACT-REPORT-2", "2026-08-24T03:00:00Z"))
	mustRegisterOffset(t, handler, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 3)

	first := offsetPosting(t, "SYN-POST-OFFSET", "SYN-OFFSET-1", 3)
	mustPost(t, handler, first)
	if got := postOutcome(t, handler, first); got != application.OutcomeExisting {
		t.Fatalf("重放 = %s，要 EXISTING（在途此刻已是 0，不答余额不足）", got)
	}
	again := offsetPosting(t, "SYN-POST-OFFSET-AGAIN", "SYN-OFFSET-1", 3)
	if got := postOutcome(t, handler, again); got != application.OutcomeContentConflict {
		t.Fatalf("换标识再落同一抵扣 = %s，要 CONTENT_CONFLICT（先于余额守卫）", got)
	}
	if got := mustBalance(t, store).At(domain.PositionOffsetApplied); got != 3 {
		t.Fatalf("已抵扣 = %d，要 3——同一抵扣只落一次", got)
	}
}

// 撤回：认可被撤回就冲正已落的那笔，3 回到`渠道在途`、仍是该指令的欠款；一笔至多冲正
// 一次——重复到达的撤回答冲突，不会从已抵扣里多拿，也不动同账别的指令的抵扣；撤回的
// 认可不能再落（要落走新的认可）；更正则是新认可新记录。
// Covers: AT-CR-025
func TestAWithdrawnOffsetIsReversedOnceAndCannotApplyAgain(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-X", 100)
	seedInstruction(t, store, handler, "SYN-INSTR-Y", 100)
	for _, instruction := range []string{"X", "Y"} {
		seedFact(t, store, handler, "SYN-FACT-"+instruction+"-REPORT", "SYN-INSTR-"+instruction,
			domain.ChannelCollectionReport, 100)
		mustPost(t, handler, postCommand(t, "SYN-POST-"+instruction+"-REPORT",
			domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-"+instruction+"-REPORT", "2026-08-24T01:10:00Z"))
		mustRegisterOffset(t, handler, "SYN-OFFSET-"+instruction, "SYN-INSTR-"+instruction,
			"SYN-SA-APPROVAL-"+instruction, 3)
		mustPost(t, handler, offsetPosting(t, "SYN-POST-OFFSET-"+instruction, "SYN-OFFSET-"+instruction, 3))
	}

	reverse := postCommand(t, "SYN-POST-OFFSET-X-FIX",
		domain.PositionOffsetApplied, domain.PositionInTransitAtChannel,
		3, domain.BasisCorrection, "SYN-POST-OFFSET-X", "2026-08-26T01:00:00Z")
	mustPost(t, handler, reverse)
	if got := postOutcome(t, handler, reverse); got != application.OutcomeExisting {
		t.Fatalf("撤回重放 = %s，要 EXISTING", got)
	}
	// 已抵扣里还躺着 Y 的 3：撤回再到一次，余额够扣，但那 3 不是 X 的。
	dup := postCommand(t, "SYN-POST-OFFSET-X-FIX-AGAIN",
		domain.PositionOffsetApplied, domain.PositionInTransitAtChannel,
		3, domain.BasisCorrection, "SYN-POST-OFFSET-X", "2026-08-26T02:00:00Z")
	if got := postOutcome(t, handler, dup); got != application.OutcomeContentConflict {
		t.Fatalf("同一笔抵扣被冲正两次 = %s，要 CONTENT_CONFLICT", got)
	}
	balance := mustBalance(t, store)
	if balance.At(domain.PositionOffsetApplied) != 3 || balance.At(domain.PositionInTransitAtChannel) != 197 {
		t.Fatalf("已抵扣 %d 渠道在途 %d，要 3 与 197——Y 的抵扣没被多冲",
			balance.At(domain.PositionOffsetApplied), balance.At(domain.PositionInTransitAtChannel))
	}

	// 撤回的认可不能再落，也不能靠冲正的冲正重新进入已抵扣。
	if got := postOutcome(t, handler, offsetPosting(t, "SYN-POST-OFFSET-X-RETRY", "SYN-OFFSET-X", 3)); got != application.OutcomeContentConflict {
		t.Fatalf("撤回的认可再落 = %s，要 CONTENT_CONFLICT", got)
	}
	redo := postCommand(t, "SYN-POST-OFFSET-X-REDO",
		domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
		3, domain.BasisCorrection, "SYN-POST-OFFSET-X-FIX", "2026-08-26T03:00:00Z")
	if got := postOutcome(t, handler, redo); got != application.OutcomeNotAccepted {
		t.Fatalf("冲正的冲正重新进入已抵扣 = %s，要 NOT_ACCEPTED", got)
	}

	// 认可被更正为 2：新认可新记录，落在撤回之后。
	mustRegisterOffset(t, handler, "SYN-OFFSET-X2", "SYN-INSTR-X", "SYN-SA-APPROVAL-X2", 2)
	mustPost(t, handler, offsetPosting(t, "SYN-POST-OFFSET-X2", "SYN-OFFSET-X2", 2))
	if got := mustBalance(t, store).At(domain.PositionOffsetApplied); got != 5 {
		t.Fatalf("已抵扣 = %d，要 5（Y 的 3 + X 更正后的 2）", got)
	}
}

// 短款先于抵扣落账会吃掉抵扣的额度：此后抵扣答余额不足，要先冲正多落的短款，再落抵扣，
// 最后落余下的短款。终局与「先抵扣再短款」相同。
// Covers: AT-CR-025
func TestAShortfallPostedBeforeTheOffsetBlocksItUntilItIsCorrected(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 100)
	seedFact(t, store, handler, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 96)
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		100, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-CREDIT",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		96, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T02:00:00Z"))
	mustRegisterDiscrepancy(t, handler, "SYN-DIFF-WIDE", "SYN-INSTR-1", domain.DiscrepancyShortfall, 4)
	mustPost(t, handler, postCommand(t, "SYN-POST-SHORT-WIDE",
		domain.PositionInTransitAtChannel, domain.PositionShortfall,
		4, domain.BasisDiscrepancy, "SYN-DIFF-WIDE", "2026-08-25T01:00:00Z"))

	mustRegisterOffset(t, handler, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 3)
	offset := offsetPosting(t, "SYN-POST-OFFSET", "SYN-OFFSET-1", 3)
	if got := postOutcome(t, handler, offset); got != application.OutcomeUnderfunded {
		t.Fatalf("短款已吃掉在途款时的抵扣 = %s，要 UNDERFUNDED", got)
	}

	mustPost(t, handler, postCommand(t, "SYN-POST-SHORT-WIDE-FIX",
		domain.PositionShortfall, domain.PositionInTransitAtChannel,
		4, domain.BasisCorrection, "SYN-POST-SHORT-WIDE", "2026-08-26T01:00:00Z"))
	mustPost(t, handler, offset)
	mustRegisterDiscrepancy(t, handler, "SYN-DIFF-1", "SYN-INSTR-1", domain.DiscrepancyShortfall, 1)
	mustPost(t, handler, postCommand(t, "SYN-POST-SHORT",
		domain.PositionInTransitAtChannel, domain.PositionShortfall,
		1, domain.BasisDiscrepancy, "SYN-DIFF-1", "2026-08-26T02:00:00Z"))

	balance := mustBalance(t, store)
	if balance.At(domain.PositionInTransitAtChannel) != 0 ||
		balance.At(domain.PositionOffsetApplied) != 3 ||
		balance.At(domain.PositionShortfall) != 1 ||
		balance.At(domain.PositionAwaitingAllocation) != 96 {
		t.Fatalf("终局账面：在途 %d 已抵扣 %d 短款 %d 待清分 %d，要 0 / 3 / 1 / 96",
			balance.At(domain.PositionInTransitAtChannel), balance.At(domain.PositionOffsetApplied),
			balance.At(domain.PositionShortfall), balance.At(domain.PositionAwaitingAllocation))
	}
}

// 一笔记账至多被冲正一次不限于抵扣：到账移位被重复冲正，第二笔答冲突，不从同账别的
// 指令的待清分里多拿。
// Covers: AT-CR-018（重复冲正）
func TestAnyPostingIsReversedAtMostOnce(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 100)
	seedReceived(t, store, handler, "SYN-INSTR-1", 100)
	fix := postCommand(t, "SYN-POST-CREDIT-FIX",
		domain.PositionAwaitingAllocation, domain.PositionInTransitAtChannel,
		100, domain.BasisCorrection, "SYN-POST-CREDIT", "2026-08-26T01:00:00Z")
	mustPost(t, handler, fix)
	again := fix
	again.ID = postingIDOf(t, "SYN-POST-CREDIT-FIX-AGAIN")
	if got := postOutcome(t, handler, again); got != application.OutcomeContentConflict {
		t.Fatalf("重复冲正 = %s，要 CONTENT_CONFLICT", got)
	}
}

// 依赖故障时抵扣的落账与登记一样答未决，不伪装成余额不足或冲突。
func TestOffsetPostingDependencyFailureIsUndecided(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 100)
	seedFact(t, store, handler, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100)
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		100, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z"))
	mustRegisterOffset(t, handler, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 3)

	store.readErr = errDependencyDown
	if got := postOutcome(t, handler, offsetPosting(t, "SYN-POST-OFFSET", "SYN-OFFSET-1", 3)); got != application.OutcomeUndecided {
		t.Fatalf("读口故障时的抵扣 = %s，要 UNDECIDED", got)
	}
	store.readErr = nil
	store.writeErr = errDependencyDown
	if got := postOutcome(t, handler, offsetPosting(t, "SYN-POST-OFFSET", "SYN-OFFSET-1", 3)); got != application.OutcomeUndecided {
		t.Fatalf("写口故障时的抵扣 = %s，要 UNDECIDED", got)
	}
}
