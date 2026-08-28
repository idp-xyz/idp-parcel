package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

func instructionID(t *testing.T, value string) domain.CollectionInstructionID {
	t.Helper()
	id, err := domain.NewCollectionInstructionID(value)
	if err != nil {
		t.Fatalf("构造指令标识 %q：%v", value, err)
	}
	return id
}

func instructionSpec(t *testing.T, key domain.SubledgerKey) domain.CollectionInstructionSpec {
	t.Helper()
	parcel, err := domain.NewParcelReference("SYN-PARCEL-1")
	if err != nil {
		t.Fatalf("构造包裹引用：%v", err)
	}
	requirement, err := domain.NewServiceRequirementReference("SYN-COD-REQ-V1")
	if err != nil {
		t.Fatalf("构造服务要求引用：%v", err)
	}
	return domain.CollectionInstructionSpec{
		ID:           instructionID(t, "SYN-INSTR-1"),
		Parcel:       parcel,
		Requirement:  requirement,
		Ledger:       key,
		Amount:       money(t, key.Currency().String(), 12000),
		InstructedAt: instant(t, "2026-08-24T00:30:00Z"),
	}
}

func TestACollectionInstructionNeedsItsServiceRequirementBasis(t *testing.T) {
	key := ledgerKey(t, "EUR")
	spec := instructionSpec(t, key)

	without := spec
	without.Requirement = domain.ServiceRequirementReference{}
	if _, err := domain.IssueCollectionInstruction(without); err == nil {
		t.Fatal("无服务要求依据的代收指令被接受——册面上它与运营企业自己要收的钱分不开")
	}

	withoutParcel := spec
	withoutParcel.Parcel = domain.ParcelReference{}
	if _, err := domain.IssueCollectionInstruction(withoutParcel); err == nil {
		t.Fatal("无包裹引用的代收指令被接受")
	}

	crossCurrency := spec
	crossCurrency.Amount = money(t, "USD", 12000)
	if _, err := domain.IssueCollectionInstruction(crossCurrency); err == nil {
		t.Fatal("指令币种与分户账币种不同也被接受")
	}

	issued, err := domain.IssueCollectionInstruction(spec)
	if err != nil {
		t.Fatalf("成立代收指令：%v", err)
	}
	if issued.Ledger() != key {
		t.Fatal("指令固定的分户账键与输入不同——记账该进哪本账在义务成立时就已确定")
	}
}

// 四层来源不得互相推导：只有真实到账那一层把本金带进`待清分`，其余三层一律停在
// `渠道在途`。这条映射错一次，未实际收到的代收款就会推进成可付客户余额。
func TestOnlyAnOperatorBankCreditTakesPrincipalPastTheChannel(t *testing.T) {
	cases := []struct {
		layer domain.CollectionSourceLayer
		want  domain.FundPosition
	}{
		{domain.RecipientPayment, domain.PositionInTransitAtChannel},
		{domain.ChannelCollectionReport, domain.PositionInTransitAtChannel},
		{domain.ChannelRemittanceNotice, domain.PositionInTransitAtChannel},
		{domain.OperatorBankCredit, domain.PositionAwaitingAllocation},
	}
	for _, testCase := range cases {
		fact := acceptedFact(t, testCase.layer)
		if got := fact.IntakePosition(); got != testCase.want {
			t.Fatalf("%s 的入账位置 = %s，要 %s", testCase.layer, got, testCase.want)
		}
		if got := testCase.layer.SettlesToOperator(); got != (testCase.layer == domain.OperatorBankCredit) {
			t.Fatalf("%s 的到账判断 = %v", testCase.layer, got)
		}
	}
	// 没有任何一格能落到`应付客户`：那一步要另有一笔清分记账。
	for _, testCase := range cases {
		if testCase.want == domain.PositionPayableToCustomer {
			t.Fatalf("%s 直接入账到应付客户", testCase.layer)
		}
	}
}

func acceptedFact(t *testing.T, layer domain.CollectionSourceLayer) domain.CollectionFact {
	t.Helper()
	factID, err := domain.NewCollectionFactID("SYN-FACT-" + layer.String())
	if err != nil {
		t.Fatalf("构造事实标识：%v", err)
	}
	evidence, err := domain.NewEvidenceReference("SYN-EVIDENCE-1")
	if err != nil {
		t.Fatalf("构造证据引用：%v", err)
	}
	fact, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID:          factID,
		Instruction: instructionID(t, "SYN-INSTR-1"),
		Layer:       layer,
		Evidence:    evidence,
		Amount:      money(t, "EUR", 12000),
		OccurredAt:  instant(t, "2026-08-24T01:00:00Z"),
	})
	if err != nil {
		t.Fatalf("接受代收事实：%v", err)
	}
	return fact
}

func TestACollectionFactMustDeclareItsSourceLayerAndEvidence(t *testing.T) {
	base := domain.CollectionFactSpec{
		ID:          mustFactID(t, "SYN-FACT-1"),
		Instruction: instructionID(t, "SYN-INSTR-1"),
		Layer:       domain.RecipientPayment,
		Evidence:    mustEvidence(t, "SYN-EVIDENCE-1"),
		Amount:      money(t, "EUR", 12000),
		OccurredAt:  instant(t, "2026-08-24T01:00:00Z"),
	}

	withoutLayer := base
	withoutLayer.Layer = domain.SourceLayerUnknown
	if _, err := domain.AcceptCollectionFact(withoutLayer); err == nil {
		t.Fatal("不声明来源层级的事实被接受——账上分不出「渠道说收到了」与「钱真的到了」")
	}

	withoutEvidence := base
	withoutEvidence.Evidence = domain.EvidenceReference{}
	if _, err := domain.AcceptCollectionFact(withoutEvidence); err == nil {
		t.Fatal("无来源证据引用的事实被接受")
	}

	withoutInstruction := base
	withoutInstruction.Instruction = domain.CollectionInstructionID{}
	if _, err := domain.AcceptCollectionFact(withoutInstruction); err == nil {
		t.Fatal("不挂在指令上的事实被接受——无来源的本金")
	}
}

func TestUnknownWordsDoNotParseIntoADefaultMember(t *testing.T) {
	if _, ok := domain.ParseCollectionSourceLayer("BANK_TRANSFER"); ok {
		t.Fatal("库里的陌生来源层级被折成了词表成员")
	}
	if _, ok := domain.ParseFundPosition("HELD"); ok {
		t.Fatal("库里的陌生资金位置被折成了词表成员")
	}
	if _, ok := domain.ParsePostingBasisKind("MANUAL"); ok {
		t.Fatal("库里的陌生依据种类被折成了词表成员")
	}
	if _, ok := domain.ParseRemittanceBatchState("CANCELLED"); ok {
		t.Fatal("库里的陌生批次状态被折成了词表成员——批次没有取消格")
	}
	if _, ok := domain.ParseDiscrepancyKind("ROUNDING"); ok {
		t.Fatal("库里的陌生差异方向被折成了词表成员")
	}
}

func mustFactID(t *testing.T, value string) domain.CollectionFactID {
	t.Helper()
	id, err := domain.NewCollectionFactID(value)
	if err != nil {
		t.Fatalf("构造事实标识 %q：%v", value, err)
	}
	return id
}

func mustEvidence(t *testing.T, value string) domain.EvidenceReference {
	t.Helper()
	ref, err := domain.NewEvidenceReference(value)
	if err != nil {
		t.Fatalf("构造证据引用 %q：%v", value, err)
	}
	return ref
}

func TestABatchFormsCollectedAndAdvancesOnlyForward(t *testing.T) {
	key := ledgerKey(t, "EUR")
	batchID, err := domain.NewRemittanceBatchID("SYN-BATCH-1")
	if err != nil {
		t.Fatalf("构造批次标识：%v", err)
	}
	spec := domain.RemittanceBatchSpec{
		ID:               batchID,
		Ledger:           key,
		CollectedThrough: instant(t, "2026-08-31T00:00:00Z"),
		FormedAt:         instant(t, "2026-08-31T01:00:00Z"),
	}
	batch, err := domain.FormRemittanceBatch(spec)
	if err != nil {
		t.Fatalf("形成批次：%v", err)
	}
	if batch.State() != domain.BatchCollected {
		t.Fatalf("新批次状态 = %s，要 COLLECTED", batch.State())
	}
	if !batch.AcceptsRemittance() {
		t.Fatal("已归集批次不收汇付记账")
	}

	handed, advanced := batch.HandOverForPayment()
	if !advanced || handed.State() != domain.BatchHandedForPayment {
		t.Fatalf("交出汇付主张没落到批次上：advanced=%v state=%s", advanced, handed.State())
	}
	// 成员集合只增不改的界就在「交出」那一刻，否则主张交出去之后金额还会变。
	if handed.AcceptsRemittance() {
		t.Fatal("已交出主张的批次仍收新成员")
	}
	if _, advancedAgain := handed.HandOverForPayment(); advancedAgain {
		t.Fatal("重复交出被记成了一次新的推进")
	}
	// 状态推进不动键、币种与归集截点。
	if handed.Ledger() != key || !handed.CollectedThrough().Equal(spec.CollectedThrough.UTC()) {
		t.Fatal("状态推进改动了批次的冻结部分")
	}

	rehydrated, err := domain.RehydrateRemittanceBatch(spec, domain.BatchHandedForPayment)
	if err != nil {
		t.Fatalf("读回批次：%v", err)
	}
	if rehydrated.State() != domain.BatchHandedForPayment {
		t.Fatal("已交出的批次读回来退成了已归集")
	}
}

func TestADiscrepancyItemDoesNotSettleItself(t *testing.T) {
	itemID, err := domain.NewDiscrepancyItemID("SYN-DIFF-1")
	if err != nil {
		t.Fatalf("构造差异标识：%v", err)
	}
	spec := domain.DiscrepancyItemSpec{
		ID:          itemID,
		Instruction: instructionID(t, "SYN-INSTR-1"),
		Kind:        domain.DiscrepancyShortfall,
		Amount:      money(t, "EUR", 200),
		Basis:       basisRef(t, "SYN-CHANNEL-REPORT-1"),
		ObservedAt:  instant(t, "2026-08-25T00:00:00Z"),
	}
	item, err := domain.RegisterDiscrepancyItem(spec)
	if err != nil {
		t.Fatalf("登记差异事项：%v", err)
	}
	if item.SettlementPosition() != domain.PositionShortfall {
		t.Fatalf("短款的落点 = %s", item.SettlementPosition())
	}

	surplus := spec
	surplus.Kind = domain.DiscrepancySurplus
	surplusItem, err := domain.RegisterDiscrepancyItem(surplus)
	if err != nil {
		t.Fatalf("登记溢款事项：%v", err)
	}
	if surplusItem.SettlementPosition() != domain.PositionSurplus {
		t.Fatalf("溢款的落点 = %s", surplusItem.SettlementPosition())
	}

	withoutBasis := spec
	withoutBasis.Basis = domain.BasisReference{}
	if _, err := domain.RegisterDiscrepancyItem(withoutBasis); err == nil {
		t.Fatal("无依据的差异事项被接受")
	}
	withoutKind := spec
	withoutKind.Kind = domain.DiscrepancyKindUnknown
	if _, err := domain.RegisterDiscrepancyItem(withoutKind); err == nil {
		t.Fatal("无方向的差异事项被接受")
	}
	if _, err := domain.RegisterDiscrepancyItem(domain.DiscrepancyItemSpec{
		ID:          itemID,
		Instruction: instructionID(t, "SYN-INSTR-1"),
		Kind:        domain.DiscrepancyShortfall,
		Amount:      money(t, "EUR", 200),
		Basis:       basisRef(t, "SYN-CHANNEL-REPORT-1"),
		ObservedAt:  time.Time{},
	}); err == nil {
		t.Fatal("无观察时刻的差异事项被接受")
	}
}
