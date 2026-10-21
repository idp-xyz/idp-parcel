package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// 按指令判的三式（ADR-0271 决定二、四、五）：移位上限、短款上限与溢款金额都只看这一条
// 指令名下的报告、到账与差额，不看同账别的指令。

func principalInstruction(t *testing.T, id string, amountMinor int64) domain.CollectionInstruction {
	t.Helper()
	spec := instructionSpec(t, ledgerKey(t, "EUR"))
	spec.ID = instructionID(t, id)
	spec.Amount = money(t, "EUR", amountMinor)
	instruction, err := domain.IssueCollectionInstruction(spec)
	if err != nil {
		t.Fatalf("成立代收指令：%v", err)
	}
	return instruction
}

func principalFact(
	t *testing.T, id, instruction string, layer domain.CollectionSourceLayer, amountMinor int64,
) domain.CollectionFact {
	t.Helper()
	factID, err := domain.NewCollectionFactID(id)
	if err != nil {
		t.Fatalf("构造事实标识：%v", err)
	}
	evidence, err := domain.NewEvidenceReference("SYN-EVIDENCE-" + id)
	if err != nil {
		t.Fatalf("构造证据引用：%v", err)
	}
	fact, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID:          factID,
		Instruction: instructionID(t, instruction),
		Layer:       layer,
		Evidence:    evidence,
		Amount:      money(t, "EUR", amountMinor),
		OccurredAt:  instant(t, "2026-08-24T01:00:00Z"),
	})
	if err != nil {
		t.Fatalf("接受代收事实：%v", err)
	}
	return fact
}

func move(
	t *testing.T, id string, from, to domain.FundPosition, amountMinor int64,
	kind domain.PostingBasisKind, basis string,
) domain.SubledgerPosting {
	t.Helper()
	return posting(t, domain.SubledgerPostingSpec{
		ID:        postingID(t, id),
		Ledger:    ledgerKey(t, "EUR"),
		From:      from,
		To:        to,
		Amount:    money(t, "EUR", amountMinor),
		BasisKind: kind,
		Basis:     basisRef(t, basis),
		PostedAt:  instant(t, "2026-08-24T02:00:00Z"),
	})
}

func derivePrincipal(
	t *testing.T, instruction domain.CollectionInstruction,
	facts []domain.CollectionFact, postings []domain.SubledgerPosting,
) domain.InstructionPrincipal {
	t.Helper()
	principal, err := domain.DeriveInstructionPrincipal(instruction, facts, postings)
	if err != nil {
		t.Fatalf("派生指令名下本金：%v", err)
	}
	return principal
}

// 场景十七在本上下文那一半：报告 100 入`渠道在途`，到账 96 移出，短款上限是 4；认可的
// 抵扣 3 没有记账形、留在`渠道在途`，所以短款 1 落得下，5 越过上限答余额不足。
func TestTheShortfallCeilingIsTheReportLessWhatLeftInTransit(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	facts := []domain.CollectionFact{
		principalFact(t, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100),
		principalFact(t, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 96),
	}
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
		move(t, "SYN-POST-2", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
			96, domain.BasisCollectionFact, "SYN-FACT-CREDIT"),
	}
	principal := derivePrincipal(t, instruction, facts, postings)

	if got := principal.TransferCeiling(); got != 4 {
		t.Fatalf("移位上限 = %d，要 4（报告 100 − 到账移出 96）", got)
	}
	if got := principal.ShortfallCeiling(); got != 4 {
		t.Fatalf("短款上限 = %d，要 4", got)
	}

	one := move(t, "SYN-POST-3", domain.PositionInTransitAtChannel, domain.PositionShortfall,
		1, domain.BasisDiscrepancy, "SYN-DIFF-1")
	if err := principal.Admit(one); err != nil {
		t.Fatalf("短款 1 在上限内却被拒：%v", err)
	}
	five := move(t, "SYN-POST-4", domain.PositionInTransitAtChannel, domain.PositionShortfall,
		5, domain.BasisDiscrepancy, "SYN-DIFF-2")
	if err := principal.Admit(five); !errors.Is(err, domain.ErrPositionUnderfunded) {
		t.Fatalf("短款 5 越过该指令的在途款 = %v，要余额不足", err)
	}
}

// 冲正回`渠道在途`的短款把上限还回来：渠道补付走「先冲正短款、再凭新到账移位」。
func TestACorrectedShortfallGivesTheInTransitBackToItsInstruction(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
		move(t, "SYN-POST-2", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
			96, domain.BasisCollectionFact, "SYN-FACT-CREDIT"),
		move(t, "SYN-POST-3", domain.PositionInTransitAtChannel, domain.PositionShortfall,
			4, domain.BasisDiscrepancy, "SYN-DIFF-1"),
	}
	if got := derivePrincipal(t, instruction, nil, postings).TransferCeiling(); got != 0 {
		t.Fatalf("短款落账后移位上限 = %d，要 0", got)
	}
	postings = append(postings, move(t, "SYN-POST-4",
		domain.PositionShortfall, domain.PositionInTransitAtChannel,
		4, domain.BasisCorrection, "SYN-POST-3"))
	if got := derivePrincipal(t, instruction, nil, postings).TransferCeiling(); got != 4 {
		t.Fatalf("冲正短款后移位上限 = %d，要 4", got)
	}
}

// 到账多于该指令已入账的报告时整笔停住，不先移一部分。
func TestAnArrivalBeyondItsInstructionsReportIsUnderfunded(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
	}
	principal := derivePrincipal(t, instruction, nil, postings)
	tooMuch := move(t, "SYN-POST-2", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		120, domain.BasisCollectionFact, "SYN-FACT-CREDIT")
	if err := principal.Admit(tooMuch); !errors.Is(err, domain.ErrPositionUnderfunded) {
		t.Fatalf("到账 120 多于报告 100 = %v，要余额不足", err)
	}
}

// 该指令有已登记而未移位的到账时不判短款：钱已经回来，只是还没移（或移不动），此刻
// 落短款会把补报告之后就能移位的本金落进`短款`。
func TestAShortfallWaitsWhileAnArrivalOfTheInstructionIsUnmoved(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	facts := []domain.CollectionFact{
		principalFact(t, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100),
		principalFact(t, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 120),
	}
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
	}
	principal := derivePrincipal(t, instruction, facts, postings)
	if !principal.HasUnmovedArrival() {
		t.Fatal("到账已登记而未移位，派生结果却说没有")
	}
	shortfall := move(t, "SYN-POST-2", domain.PositionInTransitAtChannel, domain.PositionShortfall,
		10, domain.BasisDiscrepancy, "SYN-DIFF-1")
	if err := principal.Admit(shortfall); !errors.Is(err, domain.ErrPositionUnderfunded) {
		t.Fatalf("有未移位到账时的短款 = %v，要余额不足", err)
	}
}

// 溢款 = 移入待清分的到账 − 指令金额 − 已落入溢款：同一笔多收落过一次之后，第二项
// 事项再落就越过了；少报的事项与按指令算的溢款对不上，是事项本身错了。
func TestASurplusEqualsTheReceivedOverageAndIsCountedOnce(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			108, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
		move(t, "SYN-POST-2", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
			108, domain.BasisCollectionFact, "SYN-FACT-CREDIT"),
	}
	principal := derivePrincipal(t, instruction, nil, postings)
	if got := principal.SurplusDue(); got != 8 {
		t.Fatalf("溢款 = %d，要 8", got)
	}
	short := move(t, "SYN-POST-3", domain.PositionAwaitingAllocation, domain.PositionSurplus,
		5, domain.BasisDiscrepancy, "SYN-DIFF-1")
	if err := principal.Admit(short); !errors.Is(err, domain.ErrInvalidPosting) {
		t.Fatalf("事项 5 与按指令算的溢款 8 不符 = %v，要记账不合法", err)
	}
	exact := move(t, "SYN-POST-3", domain.PositionAwaitingAllocation, domain.PositionSurplus,
		8, domain.BasisDiscrepancy, "SYN-DIFF-1")
	if err := principal.Admit(exact); err != nil {
		t.Fatalf("溢款 8 被拒：%v", err)
	}

	landed := derivePrincipal(t, instruction, nil, append(postings, exact))
	if got := landed.SurplusDue(); got != 0 {
		t.Fatalf("溢款落账后剩余溢款 = %d，要 0", got)
	}
	again := move(t, "SYN-POST-4", domain.PositionAwaitingAllocation, domain.PositionSurplus,
		8, domain.BasisDiscrepancy, "SYN-DIFF-2")
	if err := landed.Admit(again); !errors.Is(err, domain.ErrPositionUnderfunded) {
		t.Fatalf("同一笔多收第二次落溢款 = %v，要余额不足", err)
	}
}

// 一条代收事实至多支撑一笔记账：派生结果答得出它已被哪一笔用掉。
func TestTheDerivationKnowsWhichPostingAFactAlreadySupports(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	facts := []domain.CollectionFact{
		principalFact(t, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100),
		principalFact(t, "SYN-FACT-POD", "SYN-INSTR-1", domain.RecipientPayment, 100),
	}
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
	}
	principal := derivePrincipal(t, instruction, facts, postings)
	used, found := principal.PostingOf(facts[0].ID())
	if !found || used.String() != "SYN-POST-1" {
		t.Fatalf("报告支撑的记账 = %q（found=%v），要 SYN-POST-1", used, found)
	}
	if _, found := principal.PostingOf(facts[1].ID()); found {
		t.Fatal("没被记过账的旁证被说成已入账")
	}
	if principal.HasUnmovedArrival() {
		t.Fatal("没有到账事实，派生结果却说有未移位的到账")
	}
}

// 别的指令的事实、别本账的记账混进来是调用方装错了，不是一个可以算下去的账面。
func TestTheDerivationRefusesAnotherInstructionsFactsAndAnotherLedgersPostings(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	foreignFact := principalFact(t, "SYN-FACT-Y", "SYN-INSTR-Y", domain.ChannelCollectionReport, 50)
	if _, err := domain.DeriveInstructionPrincipal(
		instruction, []domain.CollectionFact{foreignFact}, nil); !errors.Is(err, domain.ErrInvalidCollectionFact) {
		t.Fatalf("别的指令的事实被算进来了：%v", err)
	}
	foreignPosting := intake(t, ledgerKey(t, "USD"), "9", 100, domain.PositionInTransitAtChannel)
	if _, err := domain.DeriveInstructionPrincipal(
		instruction, nil, []domain.SubledgerPosting{foreignPosting}); !errors.Is(err, domain.ErrInvalidPosting) {
		t.Fatalf("别本账的记账被算进来了：%v", err)
	}
}
