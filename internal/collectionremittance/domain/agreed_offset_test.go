package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

func agreedOffsetSpec(t *testing.T) domain.AgreedOffsetSpec {
	t.Helper()
	id, err := domain.NewAgreedOffsetID("SYN-OFFSET-1")
	if err != nil {
		t.Fatalf("构造抵扣标识：%v", err)
	}
	approval, err := domain.NewApprovalReference("SYN-SA-APPROVAL-1")
	if err != nil {
		t.Fatalf("构造认可引用：%v", err)
	}
	return domain.AgreedOffsetSpec{
		ID:          id,
		Instruction: instructionID(t, "SYN-INSTR-1"),
		Amount:      money(t, "EUR", 3),
		Approval:    approval,
		ApprovedAt:  instant(t, "2026-08-24T05:00:00Z"),
	}
}

// 认可的协议抵扣是 settlement-accounting 认可、本上下文只引用的一笔：标识、指令、
// 金额、认可引用与认可时刻缺一不可，缺任何一格都不以默认值补齐。
func TestAnAgreedOffsetNeedsEveryCellAndKeepsTheApprovalReference(t *testing.T) {
	offset, err := domain.RegisterAgreedOffset(agreedOffsetSpec(t))
	if err != nil {
		t.Fatalf("登记完整的认可抵扣被拒：%v", err)
	}
	if offset.ID().String() != "SYN-OFFSET-1" ||
		offset.Instruction().String() != "SYN-INSTR-1" ||
		offset.Amount().AmountMinor() != 3 ||
		offset.Approval().String() != "SYN-SA-APPROVAL-1" ||
		!offset.ApprovedAt().Equal(instant(t, "2026-08-24T05:00:00Z")) {
		t.Fatalf("登记后读回的格不对：%+v", offset)
	}

	cases := map[string]func(*domain.AgreedOffsetSpec){
		"无标识":   func(spec *domain.AgreedOffsetSpec) { spec.ID = domain.AgreedOffsetID{} },
		"无指令":   func(spec *domain.AgreedOffsetSpec) { spec.Instruction = domain.CollectionInstructionID{} },
		"无金额":   func(spec *domain.AgreedOffsetSpec) { spec.Amount = domain.Money{} },
		"无认可引用": func(spec *domain.AgreedOffsetSpec) { spec.Approval = domain.ApprovalReference{} },
		"无认可时刻": func(spec *domain.AgreedOffsetSpec) { spec.ApprovedAt = time.Time{} },
	}
	for name, mutate := range cases {
		spec := agreedOffsetSpec(t)
		mutate(&spec)
		if _, err := domain.RegisterAgreedOffset(spec); !errors.Is(err, domain.ErrInvalidAgreedOffset) {
			t.Fatalf("%s 被接受：%v", name, err)
		}
	}
}

// 新词只按落库词逐字认，未知词不折成默认成员。
func TestTheOffsetPositionAndBasisKindParseOnlyByTheirStoredWords(t *testing.T) {
	position, known := domain.ParseFundPosition("OFFSET_APPLIED")
	if !known || position != domain.PositionOffsetApplied || position.String() != "OFFSET_APPLIED" {
		t.Fatalf("OFFSET_APPLIED 解析成 %v（known=%v）", position, known)
	}
	if !position.InsideLedger() {
		t.Fatal("已抵扣是账内终局位置，不是账外位置")
	}
	kind, known := domain.ParsePostingBasisKind("AGREED_OFFSET")
	if !known || kind != domain.BasisAgreedOffset || kind.String() != "AGREED_OFFSET" {
		t.Fatalf("AGREED_OFFSET 解析成 %v（known=%v）", kind, known)
	}
	if _, known := domain.ParseFundPosition("AGREED_OFFSET"); known {
		t.Fatal("依据种类的词被认成了资金位置")
	}
	if _, known := domain.ParsePostingBasisKind("OFFSET_APPLIED"); known {
		t.Fatal("资金位置的词被认成了依据种类")
	}
}

// 已抵扣的两道门：进入它必须凭认可的协议抵扣、且只从`渠道在途`来；认可的协议抵扣
// 作依据只有这一种形；离开它只有冲正。
func TestTheOffsetPositionAndItsBasisAreGatedBothWays(t *testing.T) {
	key := ledgerKey(t, "EUR")
	base := domain.SubledgerPostingSpec{
		ID:       postingID(t, "SYN-POST-1"),
		Ledger:   key,
		Amount:   money(t, "EUR", 3),
		Basis:    basisRef(t, "SYN-OFFSET-1"),
		PostedAt: instant(t, "2026-08-24T05:00:00Z"),
	}
	refused := []struct {
		name      string
		from, to  domain.FundPosition
		basisKind domain.PostingBasisKind
	}{
		{"已抵扣不凭认可的协议抵扣", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied, domain.BasisDiscrepancy},
		{"已抵扣凭代收事实", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied, domain.BasisCollectionFact},
		{"已抵扣自待清分落入", domain.PositionAwaitingAllocation, domain.PositionOffsetApplied, domain.BasisAgreedOffset},
		{"已抵扣自应付客户落入", domain.PositionPayableToCustomer, domain.PositionOffsetApplied, domain.BasisAgreedOffset},
		{"已抵扣由冲正的冲正再进", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied, domain.BasisCorrection},
		{"入账凭认可的协议抵扣", domain.PositionExternalSource, domain.PositionInTransitAtChannel, domain.BasisAgreedOffset},
		{"认可的协议抵扣落进短款", domain.PositionInTransitAtChannel, domain.PositionShortfall, domain.BasisAgreedOffset},
		{"认可的协议抵扣移进待清分", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation, domain.BasisAgreedOffset},
		{"已抵扣不凭冲正以外的依据离开", domain.PositionOffsetApplied, domain.PositionPayableToCustomer, domain.BasisAllocation},
		{"已抵扣直接汇付", domain.PositionOffsetApplied, domain.PositionRemitted, domain.BasisRemittanceBatch},
	}
	for _, testCase := range refused {
		spec := base
		spec.From, spec.To, spec.BasisKind = testCase.from, testCase.to, testCase.basisKind
		if _, err := domain.RecordSubledgerPosting(spec); !errors.Is(err, domain.ErrInvalidPosting) {
			t.Fatalf("%s 被接受：%v", testCase.name, err)
		}
	}

	accepted := []struct {
		name      string
		from, to  domain.FundPosition
		basisKind domain.PostingBasisKind
	}{
		{"认可的抵扣自渠道在途落入已抵扣", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied, domain.BasisAgreedOffset},
		{"冲正已抵扣回渠道在途", domain.PositionOffsetApplied, domain.PositionInTransitAtChannel, domain.BasisCorrection},
	}
	for _, testCase := range accepted {
		spec := base
		spec.From, spec.To, spec.BasisKind = testCase.from, testCase.to, testCase.basisKind
		if _, err := domain.RecordSubledgerPosting(spec); err != nil {
			t.Fatalf("%s 被拒：%v", testCase.name, err)
		}
	}
}

// 场景十七在账上的恒等式：报告 100 入`渠道在途`，到账移走 96，认可的抵扣 3 落`已抵扣`，
// 余下 1 落`短款`；`渠道在途`清零，各账内位置之和仍等于入账 100。
// Covers: AT-CR-017（领域半边）
func TestScenarioSeventeenBalancesWithTheOffsetInTheLedger(t *testing.T) {
	key := ledgerKey(t, "EUR")
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
		move(t, "SYN-POST-2", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
			96, domain.BasisCollectionFact, "SYN-FACT-CREDIT"),
		move(t, "SYN-POST-3", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
			3, domain.BasisAgreedOffset, "SYN-OFFSET-1"),
		move(t, "SYN-POST-4", domain.PositionInTransitAtChannel, domain.PositionShortfall,
			1, domain.BasisDiscrepancy, "SYN-DIFF-1"),
	}
	balance, err := domain.DeriveSubledgerBalance(key, postings)
	if err != nil {
		t.Fatalf("场景十七的账面派生失败：%v", err)
	}
	want := map[domain.FundPosition]int64{
		domain.PositionInTransitAtChannel: 0,
		domain.PositionAwaitingAllocation: 96,
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
	if sum != balance.IntakeTotal() || balance.IntakeTotal() != 100 {
		t.Fatalf("账内各位置之和 %d、入账 %d，要都是 100", sum, balance.IntakeTotal())
	}

	// 撤回认可：冲正把 3 退回`渠道在途`，该指令此刻重新欠着这 3，守恒不变。
	reversed := append(postings, move(t, "SYN-POST-5",
		domain.PositionOffsetApplied, domain.PositionInTransitAtChannel,
		3, domain.BasisCorrection, "SYN-POST-3"))
	after, err := domain.DeriveSubledgerBalance(key, reversed)
	if err != nil {
		t.Fatalf("冲正后的账面派生失败：%v", err)
	}
	if after.At(domain.PositionOffsetApplied) != 0 || after.At(domain.PositionInTransitAtChannel) != 3 {
		t.Fatalf("冲正后 已抵扣 %d 渠道在途 %d，要 0 与 3",
			after.At(domain.PositionOffsetApplied), after.At(domain.PositionInTransitAtChannel))
	}
}

// 一笔认可抵扣在已抵扣上没有钱可退：已抵扣的余额不足以冲正时答余额不足而不是透支。
func TestReversingMoreOffsetThanWasAppliedIsUnderfunded(t *testing.T) {
	key := ledgerKey(t, "EUR")
	postings := []domain.SubledgerPosting{
		move(t, "SYN-POST-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			100, domain.BasisCollectionFact, "SYN-FACT-REPORT"),
		move(t, "SYN-POST-3", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
			3, domain.BasisAgreedOffset, "SYN-OFFSET-1"),
	}
	balance, err := domain.DeriveSubledgerBalance(key, postings)
	if err != nil {
		t.Fatalf("派生失败：%v", err)
	}
	tooMuch := move(t, "SYN-POST-5", domain.PositionOffsetApplied, domain.PositionInTransitAtChannel,
		4, domain.BasisCorrection, "SYN-POST-3")
	if err := balance.Admit(tooMuch); !errors.Is(err, domain.ErrPositionUnderfunded) {
		t.Fatalf("冲正 4 > 已抵扣 3 = %v，要余额不足", err)
	}
}
