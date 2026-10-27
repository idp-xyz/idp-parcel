package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/application"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// 本文件钉 ADR-0271 落在记账编排上的几格：只凭渠道代收报告入账、凭代收事实按依据判重、
// 到账与短款按指令判上限、溢款只从已到账的钱里落一次。

func postOutcome(t *testing.T, handler *application.Handler, command application.PostCommand) application.Outcome {
	t.Helper()
	outcome, err := handler.PostSubledger(t.Context(), command)
	if err != nil {
		t.Fatalf("记账 %s 交回错误：%v", command.ID, err)
	}
	return outcome
}

func mustBalance(t *testing.T, store *storeDouble) domain.SubledgerBalance {
	t.Helper()
	balance, opened, err := store.LoadBalance(t.Context(), tenantID(t), testLedgerKey(t))
	if err != nil || !opened {
		t.Fatalf("读回账面：opened=%v err=%v", opened, err)
	}
	return balance
}

func mustRegisterDiscrepancy(
	t *testing.T, handler *application.Handler, id, instruction string,
	kind domain.DiscrepancyKind, amountMinor int64,
) {
	t.Helper()
	if outcome, _ := handler.RegisterDiscrepancy(t.Context(), application.RegisterDiscrepancyCommand{
		Tenant: tenantID(t), Item: testDiscrepancy(t, id, instruction, kind, amountMinor),
	}); outcome != application.OutcomeRegistered {
		t.Fatalf("登记差异事项 %s = %s", id, outcome)
	}
}

// 同一包裹的同一笔钱经收件人付款、渠道代收报告、渠道回款通知三层各报一次，各自都试
// 入账：只有报告那一笔进账，账上入账总额等于报告金额，不是三倍。真实到账也不能自外部
// 来源入账。
// Covers: AT-CR-006
func TestOnlyTheChannelReportOfThreeLayersPostsThePrincipal(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 12000)
	seedFact(t, store, handler, "SYN-FACT-POD", "SYN-INSTR-1", domain.RecipientPayment, 12000)
	seedFact(t, store, handler, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 12000)
	seedFact(t, store, handler, "SYN-FACT-NOTICE", "SYN-INSTR-1", domain.ChannelRemittanceNotice, 12000)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 12000)

	for _, attempt := range []struct {
		posting, fact string
		to            domain.FundPosition
		want          application.Outcome
	}{
		{"SYN-POST-POD", "SYN-FACT-POD", domain.PositionInTransitAtChannel, application.OutcomeNotAccepted},
		{"SYN-POST-REPORT", "SYN-FACT-REPORT", domain.PositionInTransitAtChannel, application.OutcomeRegistered},
		{"SYN-POST-NOTICE", "SYN-FACT-NOTICE", domain.PositionInTransitAtChannel, application.OutcomeNotAccepted},
		{"SYN-POST-CREDIT-IN", "SYN-FACT-CREDIT", domain.PositionAwaitingAllocation, application.OutcomeNotAccepted},
		{"SYN-POST-CREDIT-TRANSIT", "SYN-FACT-CREDIT", domain.PositionInTransitAtChannel, application.OutcomeNotAccepted},
	} {
		got := postOutcome(t, handler, postCommand(t, attempt.posting,
			domain.PositionExternalSource, attempt.to,
			12000, domain.BasisCollectionFact, attempt.fact, "2026-08-24T01:10:00Z"))
		if got != attempt.want {
			t.Fatalf("凭 %s 入账 = %s，要 %s", attempt.fact, got, attempt.want)
		}
	}

	// 旁证也撑不起移位：收件人付款与渠道回款通知不支撑任何记账。
	for _, fact := range []string{"SYN-FACT-POD", "SYN-FACT-NOTICE"} {
		got := postOutcome(t, handler, postCommand(t, "SYN-POST-MOVE-"+fact,
			domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
			12000, domain.BasisCollectionFact, fact, "2026-08-24T02:00:00Z"))
		if got != application.OutcomeNotAccepted {
			t.Fatalf("凭旁证 %s 移位 = %s，要 NOT_ACCEPTED", fact, got)
		}
	}

	balance := mustBalance(t, store)
	if balance.IntakeTotal() != 12000 || balance.PostingCount() != 1 {
		t.Fatalf("入账总额 %d、记账 %d 笔，要 12000 与 1 笔——同一笔钱只入一次",
			balance.IntakeTotal(), balance.PostingCount())
	}
}

// 凭同一条代收事实换一个记账标识再记是内容冲突，不是已有结果：重放（同标识）照旧答
// EXISTING。入账与到账移位各钉一次；判重先于余额守卫——移位的来源已被第一笔移空，
// 第二笔仍答冲突而不是余额不足。
// Covers: AT-CR-008
func TestPostingAFactAgainUnderAnotherPostingIDIsAConflict(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 12000)
	seedReceived(t, store, handler, "SYN-INSTR-1", 12000)

	replay := postCommand(t, "SYN-POST-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		12000, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z")
	if got := postOutcome(t, handler, replay); got != application.OutcomeExisting {
		t.Fatalf("同标识重放入账 = %s，要 EXISTING", got)
	}
	again := replay
	again.ID = postingIDOf(t, "SYN-POST-REPORT-AGAIN")
	if got := postOutcome(t, handler, again); got != application.OutcomeContentConflict {
		t.Fatalf("换标识再凭报告入账 = %s，要 CONTENT_CONFLICT", got)
	}

	moveAgain := postCommand(t, "SYN-POST-CREDIT-AGAIN",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		12000, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T02:00:00Z")
	if got := postOutcome(t, handler, moveAgain); got != application.OutcomeContentConflict {
		t.Fatalf("换标识再凭到账移位 = %s，要 CONTENT_CONFLICT（先于余额守卫）", got)
	}

	balance := mustBalance(t, store)
	if balance.PostingCount() != 2 || balance.IntakeTotal() != 12000 {
		t.Fatalf("记账 %d 笔、入账 %d，要 2 笔与 12000", balance.PostingCount(), balance.IntakeTotal())
	}
}

// 同账两条指令 X、Y。X 报告 100、到账 96、短款 4 落账之后，X 的迟到款 4 不能借 Y 的在途
// 款 50 移进待清分——按账判的话账上`渠道在途`有 50，够；按指令判，X 名下已是 0。到账
// 多于该指令报告的也一样整笔停住。
// Covers: AT-CR-007
func TestAnArrivalCannotBorrowAnotherInstructionsInTransit(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-X", 100)
	seedInstruction(t, store, handler, "SYN-INSTR-Y", 50)
	seedFact(t, store, handler, "SYN-FACT-X-REPORT", "SYN-INSTR-X", domain.ChannelCollectionReport, 100)
	seedFact(t, store, handler, "SYN-FACT-Y-REPORT", "SYN-INSTR-Y", domain.ChannelCollectionReport, 50)
	seedFact(t, store, handler, "SYN-FACT-X-CREDIT", "SYN-INSTR-X", domain.OperatorBankCredit, 96)
	mustPost(t, handler, postCommand(t, "SYN-POST-X-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		100, domain.BasisCollectionFact, "SYN-FACT-X-REPORT", "2026-08-24T01:10:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-Y-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		50, domain.BasisCollectionFact, "SYN-FACT-Y-REPORT", "2026-08-24T01:10:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-X-CREDIT",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		96, domain.BasisCollectionFact, "SYN-FACT-X-CREDIT", "2026-08-24T02:00:00Z"))
	mustRegisterDiscrepancy(t, handler, "SYN-DIFF-X", "SYN-INSTR-X", domain.DiscrepancyShortfall, 4)
	mustPost(t, handler, postCommand(t, "SYN-POST-X-SHORT",
		domain.PositionInTransitAtChannel, domain.PositionShortfall,
		4, domain.BasisDiscrepancy, "SYN-DIFF-X", "2026-08-25T01:00:00Z"))

	seedFact(t, store, handler, "SYN-FACT-X-LATE", "SYN-INSTR-X", domain.OperatorBankCredit, 4)
	late := postCommand(t, "SYN-POST-X-LATE",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		4, domain.BasisCollectionFact, "SYN-FACT-X-LATE", "2026-08-26T01:00:00Z")
	if got := postOutcome(t, handler, late); got != application.OutcomeUnderfunded {
		t.Fatalf("X 的迟到款借 Y 的在途款 = %s，要 UNDERFUNDED", got)
	}
	if got := mustBalance(t, store).At(domain.PositionInTransitAtChannel); got != 50 {
		t.Fatalf("渠道在途 = %d，要 50——Y 的在途款一分没动", got)
	}

	// 补回：冲正 X 的短款回到`渠道在途`，迟到款才移得动（ADR-0271 决定四）。
	mustPost(t, handler, postCommand(t, "SYN-POST-X-SHORT-FIX",
		domain.PositionShortfall, domain.PositionInTransitAtChannel,
		4, domain.BasisCorrection, "SYN-POST-X-SHORT", "2026-08-26T02:00:00Z"))
	mustPost(t, handler, late)

	seedFact(t, store, handler, "SYN-FACT-Y-CREDIT", "SYN-INSTR-Y", domain.OperatorBankCredit, 60)
	over := postCommand(t, "SYN-POST-Y-CREDIT",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		60, domain.BasisCollectionFact, "SYN-FACT-Y-CREDIT", "2026-08-26T03:00:00Z")
	if got := postOutcome(t, handler, over); got != application.OutcomeUnderfunded {
		t.Fatalf("Y 到账 60 多于报告 50 = %s，要 UNDERFUNDED——整笔停住，不先移一部分", got)
	}
}

// 认可的协议抵扣还没落账时，短款按「报告 − 移出」判上限：报告 100 入`渠道在途`，到账 96
// 移进`待清分`，短款事项 5 越过该指令名下的 4，答余额不足，1 落得下。抵扣落账之后的
// 整条场景见 TestScenarioSeventeenSettlesTheAgreedOffsetOutOfTransit。
// Covers: AT-CR-017（抵扣未落账的短款上限）
func TestScenarioSeventeenShortfallStaysWithinTheReportLessWhatMoved(t *testing.T) {
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

	mustRegisterDiscrepancy(t, handler, "SYN-DIFF-TOO-BIG", "SYN-INSTR-1", domain.DiscrepancyShortfall, 5)
	if got := postOutcome(t, handler, postCommand(t, "SYN-POST-SHORT-BIG",
		domain.PositionInTransitAtChannel, domain.PositionShortfall,
		5, domain.BasisDiscrepancy, "SYN-DIFF-TOO-BIG", "2026-08-25T01:00:00Z")); got != application.OutcomeUnderfunded {
		t.Fatalf("短款 5 越过报告 − 移出 = %s，要 UNDERFUNDED", got)
	}

	mustRegisterDiscrepancy(t, handler, "SYN-DIFF-1", "SYN-INSTR-1", domain.DiscrepancyShortfall, 1)
	mustPost(t, handler, postCommand(t, "SYN-POST-SHORT",
		domain.PositionInTransitAtChannel, domain.PositionShortfall,
		1, domain.BasisDiscrepancy, "SYN-DIFF-1", "2026-08-25T01:00:00Z"))

	balance := mustBalance(t, store)
	if balance.At(domain.PositionInTransitAtChannel) != 3 ||
		balance.At(domain.PositionAwaitingAllocation) != 96 ||
		balance.At(domain.PositionShortfall) != 1 {
		t.Fatalf("抵扣未落账的账面：渠道在途 %d 待清分 %d 短款 %d，要 3 / 96 / 1",
			balance.At(domain.PositionInTransitAtChannel),
			balance.At(domain.PositionAwaitingAllocation),
			balance.At(domain.PositionShortfall))
	}
}

// 该指令有已登记而移不动的到账时不判短款：钱已经回来、报告还没补上，此刻落短款会把
// 补报告之后就能移位的本金落进`短款`。补报告、移位之后再判。
func TestAShortfallWaitsWhileTheInstructionHasAnUnmovedArrival(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 120)
	seedFact(t, store, handler, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100)
	seedFact(t, store, handler, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 110)
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		100, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z"))
	credit := postCommand(t, "SYN-POST-CREDIT",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		110, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T02:00:00Z")
	if got := postOutcome(t, handler, credit); got != application.OutcomeUnderfunded {
		t.Fatalf("到账 110 多于报告 100 = %s，要 UNDERFUNDED", got)
	}

	mustRegisterDiscrepancy(t, handler, "SYN-DIFF-1", "SYN-INSTR-1", domain.DiscrepancyShortfall, 10)
	shortfall := postCommand(t, "SYN-POST-SHORT",
		domain.PositionInTransitAtChannel, domain.PositionShortfall,
		10, domain.BasisDiscrepancy, "SYN-DIFF-1", "2026-08-25T01:00:00Z")
	if got := postOutcome(t, handler, shortfall); got != application.OutcomeUnderfunded {
		t.Fatalf("有停在余额不足的到账时判短款 = %s，要 UNDERFUNDED", got)
	}

	// 补报告 20 之后到账移得动；此时报告 120 − 移出 110 = 10，短款才落得下。
	seedFact(t, store, handler, "SYN-FACT-REPORT-2", "SYN-INSTR-1", domain.ChannelCollectionReport, 20)
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT-2",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		20, domain.BasisCollectionFact, "SYN-FACT-REPORT-2", "2026-08-26T01:10:00Z"))
	mustPost(t, handler, credit)
	mustPost(t, handler, shortfall)
}

// 溢款只从`待清分`落入，且同一笔多收只落一次：指令 100、报告与到账各 108，报少的事项
// 答未受理；溢款 8 自待清分落入；从`渠道在途`落入答未受理；第二项事项再落 8 越过按指令算的溢款，答余额
// 不足；归属确认 溢款 → 应付客户 照旧。
// Covers: AT-CR-016（溢款半边）
func TestASurplusLandsOnlyFromReceivedFundsAndOnlyOnce(t *testing.T) {
	store := newStore()
	handler := store.handler()
	openLedger(t, store, handler)
	seedInstruction(t, store, handler, "SYN-INSTR-1", 100)
	seedReceived(t, store, handler, "SYN-INSTR-1", 108)

	// 事项报少：多收的 8 已经全在待清分里，报 5 等也等不对，答未受理。
	mustRegisterDiscrepancy(t, handler, "SYN-DIFF-UNDER", "SYN-INSTR-1", domain.DiscrepancySurplus, 5)
	if got := postOutcome(t, handler, postCommand(t, "SYN-POST-SURPLUS-UNDER",
		domain.PositionAwaitingAllocation, domain.PositionSurplus,
		5, domain.BasisDiscrepancy, "SYN-DIFF-UNDER", "2026-08-25T00:30:00Z")); got != application.OutcomeNotAccepted {
		t.Fatalf("溢款事项报少 = %s，要 NOT_ACCEPTED", got)
	}

	mustRegisterDiscrepancy(t, handler, "SYN-DIFF-1", "SYN-INSTR-1", domain.DiscrepancySurplus, 8)
	fromTransit := postCommand(t, "SYN-POST-SURPLUS-TRANSIT",
		domain.PositionInTransitAtChannel, domain.PositionSurplus,
		8, domain.BasisDiscrepancy, "SYN-DIFF-1", "2026-08-25T01:00:00Z")
	if got := postOutcome(t, handler, fromTransit); got != application.OutcomeNotAccepted {
		t.Fatalf("溢款自渠道在途落入 = %s，要 NOT_ACCEPTED", got)
	}
	mustPost(t, handler, postCommand(t, "SYN-POST-SURPLUS",
		domain.PositionAwaitingAllocation, domain.PositionSurplus,
		8, domain.BasisDiscrepancy, "SYN-DIFF-1", "2026-08-25T01:00:00Z"))

	mustRegisterDiscrepancy(t, handler, "SYN-DIFF-2", "SYN-INSTR-1", domain.DiscrepancySurplus, 8)
	if got := postOutcome(t, handler, postCommand(t, "SYN-POST-SURPLUS-AGAIN",
		domain.PositionAwaitingAllocation, domain.PositionSurplus,
		8, domain.BasisDiscrepancy, "SYN-DIFF-2", "2026-08-25T02:00:00Z")); got != application.OutcomeUnderfunded {
		t.Fatalf("同一笔多收第二次落溢款 = %s，要 UNDERFUNDED", got)
	}

	mustPost(t, handler, postCommand(t, "SYN-POST-SURPLUS-CONFIRM",
		domain.PositionSurplus, domain.PositionPayableToCustomer,
		3, domain.BasisDiscrepancy, "SYN-DIFF-1", "2026-08-26T01:00:00Z"))

	balance := mustBalance(t, store)
	if balance.At(domain.PositionSurplus) != 5 ||
		balance.At(domain.PositionPayableToCustomer) != 3 ||
		balance.At(domain.PositionAwaitingAllocation) != 100 {
		t.Fatalf("溢款账面：溢款 %d 应付客户 %d 待清分 %d，要 5 / 3 / 100",
			balance.At(domain.PositionSurplus),
			balance.At(domain.PositionPayableToCustomer),
			balance.At(domain.PositionAwaitingAllocation))
	}
}

func postingIDOf(t *testing.T, value string) domain.PostingID {
	t.Helper()
	id, err := domain.NewPostingID(value)
	if err != nil {
		t.Fatalf("构造记账标识：%v", err)
	}
	return id
}
