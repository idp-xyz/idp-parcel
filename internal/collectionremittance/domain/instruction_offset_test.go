package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// 认可的协议抵扣自`渠道在途`落到`已抵扣`之后，移位上限与短款上限都随之变小：抵扣的
// 钱不是渠道还欠的，短款只能落余下的。场景十七：报告 100、到账 96、抵扣 3，余下 1。
func TestAnAppliedOffsetLeavesTheShortfallOnlyTheRemainder(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
		move(t, "SYN-POST-2", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
			96, domain.BasisCollectionFact, "SYN-FACT-CREDIT"),
		move(t, "SYN-POST-3", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
			3, domain.BasisAgreedOffset, "SYN-OFFSET-1"),
	}
	principal := derivePrincipal(t, instruction, nil, postings)

	if got := principal.TransferCeiling(); got != 1 {
		t.Fatalf("移位上限 = %d，要 1（报告 100 − 到账 96 − 抵扣 3）", got)
	}
	if got := principal.ShortfallCeiling(); got != 1 {
		t.Fatalf("短款上限 = %d，要 1", got)
	}
	if got := principal.OffsetApplied(); got != 3 {
		t.Fatalf("已抵扣 = %d，要 3", got)
	}

	exact := move(t, "SYN-POST-4", domain.PositionInTransitAtChannel, domain.PositionShortfall,
		1, domain.BasisDiscrepancy, "SYN-DIFF-1")
	if err := principal.Admit(exact); err != nil {
		t.Fatalf("短款 1 在余下的上限内却被拒：%v", err)
	}
	// 没有抵扣记账时 4 在上限内；抵扣落账之后同一笔 4 就吃掉了认可的抵扣。
	eaten := move(t, "SYN-POST-5", domain.PositionInTransitAtChannel, domain.PositionShortfall,
		4, domain.BasisDiscrepancy, "SYN-DIFF-2")
	if err := principal.Admit(eaten); !errors.Is(err, domain.ErrPositionUnderfunded) {
		t.Fatalf("短款 4 吃掉已落账的抵扣 3 = %v，要余额不足", err)
	}
}

// 抵扣额不得超过该指令的在途款，也不借同账别的指令的在途款。
func TestAnOffsetCannotExceedTheInstructionsInTransit(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
		move(t, "SYN-POST-2", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
			98, domain.BasisCollectionFact, "SYN-FACT-CREDIT"),
	}
	principal := derivePrincipal(t, instruction, nil, postings)
	over := move(t, "SYN-POST-3", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
		3, domain.BasisAgreedOffset, "SYN-OFFSET-1")
	if err := principal.Admit(over); !errors.Is(err, domain.ErrPositionUnderfunded) {
		t.Fatalf("抵扣 3 > 在途 2 = %v，要余额不足", err)
	}
	fits := move(t, "SYN-POST-3", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
		2, domain.BasisAgreedOffset, "SYN-OFFSET-1")
	if err := principal.Admit(fits); err != nil {
		t.Fatalf("抵扣 2 = 在途 2 却被拒：%v", err)
	}
}

// 到账先于抵扣：该指令有已登记而未移位的到账时不落抵扣，否则抵扣先吃掉在途款，真实
// 到账的钱反而移不动。
func TestAnOffsetWaitsWhileAnArrivalOfTheInstructionIsUnmoved(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	facts := []domain.CollectionFact{
		principalFact(t, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 100),
		principalFact(t, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 98),
	}
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
	}
	principal := derivePrincipal(t, instruction, facts, postings)
	offset := move(t, "SYN-POST-2", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
		3, domain.BasisAgreedOffset, "SYN-OFFSET-1")
	if err := principal.Admit(offset); !errors.Is(err, domain.ErrPositionUnderfunded) {
		t.Fatalf("有未移位到账时的抵扣 = %v，要余额不足", err)
	}

	moved := append(postings, move(t, "SYN-POST-3",
		domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		98, domain.BasisCollectionFact, "SYN-FACT-CREDIT"))
	after := derivePrincipal(t, instruction, facts, moved)
	if after.HasUnmovedArrival() {
		t.Fatal("到账已移位，派生结果却说还有未移位的")
	}
	two := move(t, "SYN-POST-4", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
		2, domain.BasisAgreedOffset, "SYN-OFFSET-1")
	if err := after.Admit(two); err != nil {
		t.Fatalf("到账移位之后抵扣 2 被拒：%v", err)
	}
}

// 一笔认可抵扣至多支撑一笔记账；冲正回`渠道在途`之后它仍记着那一笔，撤回的认可不能
// 再落第二次。
func TestTheDerivationKnowsWhichPostingAnOffsetAlreadyAppliedAndKeepsItAfterAReversal(t *testing.T) {
	instruction := principalInstruction(t, "SYN-INSTR-1", 100)
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
		move(t, "SYN-POST-2", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
			3, domain.BasisAgreedOffset, "SYN-OFFSET-1"),
	}
	offsetID, err := domain.NewAgreedOffsetID("SYN-OFFSET-1")
	if err != nil {
		t.Fatalf("构造抵扣标识：%v", err)
	}
	other, err := domain.NewAgreedOffsetID("SYN-OFFSET-2")
	if err != nil {
		t.Fatalf("构造抵扣标识：%v", err)
	}
	principal := derivePrincipal(t, instruction, nil, postings)
	used, found := principal.OffsetPostingOf(offsetID)
	if !found || used.String() != "SYN-POST-2" {
		t.Fatalf("认可抵扣支撑的记账 = %q（found=%v），要 SYN-POST-2", used, found)
	}
	if _, found := principal.OffsetPostingOf(other); found {
		t.Fatal("没落过账的认可抵扣被说成已落账")
	}

	postings = append(postings, move(t, "SYN-POST-3",
		domain.PositionOffsetApplied, domain.PositionInTransitAtChannel,
		3, domain.BasisCorrection, "SYN-POST-2"))
	reversed := derivePrincipal(t, instruction, nil, postings)
	if got := reversed.OffsetApplied(); got != 0 {
		t.Fatalf("冲正后已抵扣 = %d，要 0", got)
	}
	if got := reversed.TransferCeiling(); got != 100 {
		t.Fatalf("冲正后移位上限 = %d，要 100", got)
	}
	if used, found := reversed.OffsetPostingOf(offsetID); !found || used.String() != "SYN-POST-2" {
		t.Fatalf("冲正后认可抵扣忘了自己落过账：%q（found=%v）", used, found)
	}
}
