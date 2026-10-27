package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/application"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// storeDouble 是六个端口的内存替身，同时充写口与读口——用例的冲突判定靠读回，两半
// 分给两个替身就得自己保持一致，而不一致时测出来的是替身的毛病。
//
// 写口一律 DO NOTHING 语义：同键已在册就答`已登记`，绝不覆盖（与真库写口同一条代数）。
type storeDouble struct {
	instructions  map[string]domain.CollectionInstruction
	facts         map[string]domain.CollectionFact
	factOrder     []string
	discrepancies map[string]domain.DiscrepancyItem
	offsets       map[string]domain.AgreedOffset
	subledgers    map[domain.SubledgerKey]domain.Subledger
	postings      map[string]domain.SubledgerPosting
	postingOrder  []string
	batches       map[string]domain.RemittanceBatch
	locked        []domain.SubledgerKey

	writeErr error
	readErr  error
}

func newStore() *storeDouble {
	return &storeDouble{
		instructions:  map[string]domain.CollectionInstruction{},
		facts:         map[string]domain.CollectionFact{},
		discrepancies: map[string]domain.DiscrepancyItem{},
		offsets:       map[string]domain.AgreedOffset{},
		subledgers:    map[domain.SubledgerKey]domain.Subledger{},
		postings:      map[string]domain.SubledgerPosting{},
		batches:       map[string]domain.RemittanceBatch{},
	}
}

func (store *storeDouble) deps() application.Deps {
	return application.Deps{
		Collections: store,
		Collection:  store,
		Subledgers:  store,
		Subledger:   store,
		Remittances: store,
		Remittance:  store,
	}
}

func (store *storeDouble) handler() *application.Handler {
	return application.NewHandler(store.deps())
}

func (store *storeDouble) RegisterInstruction(
	_ context.Context, _ domain.TenantID, instruction domain.CollectionInstruction,
) (ports.SaveOutcome, error) {
	if store.writeErr != nil {
		return ports.SaveOutcomeInvalid, store.writeErr
	}
	key := instruction.ID().String()
	if _, exists := store.instructions[key]; exists {
		return ports.SaveAlreadyRegistered, nil
	}
	store.instructions[key] = instruction
	return ports.SaveRegistered, nil
}

func (store *storeDouble) AcceptCollectionFact(
	_ context.Context, _ domain.TenantID, fact domain.CollectionFact,
) (ports.SaveOutcome, error) {
	if store.writeErr != nil {
		return ports.SaveOutcomeInvalid, store.writeErr
	}
	key := fact.ID().String()
	if _, exists := store.facts[key]; exists {
		return ports.SaveAlreadyRegistered, nil
	}
	store.facts[key] = fact
	store.factOrder = append(store.factOrder, key)
	return ports.SaveRegistered, nil
}

func (store *storeDouble) RegisterDiscrepancyItem(
	_ context.Context, _ domain.TenantID, item domain.DiscrepancyItem,
) (ports.SaveOutcome, error) {
	if store.writeErr != nil {
		return ports.SaveOutcomeInvalid, store.writeErr
	}
	key := item.ID().String()
	if _, exists := store.discrepancies[key]; exists {
		return ports.SaveAlreadyRegistered, nil
	}
	store.discrepancies[key] = item
	return ports.SaveRegistered, nil
}

// RegisterAgreedOffset 与真库同一口径：标识撞键、或（认可引用，指令）撞键都答已登记
// 而不覆盖，后一种由编排读不回同一标识而判冲突。
func (store *storeDouble) RegisterAgreedOffset(
	_ context.Context, _ domain.TenantID, offset domain.AgreedOffset,
) (ports.SaveOutcome, error) {
	if store.writeErr != nil {
		return ports.SaveOutcomeInvalid, store.writeErr
	}
	key := offset.ID().String()
	if _, exists := store.offsets[key]; exists {
		return ports.SaveAlreadyRegistered, nil
	}
	for _, registered := range store.offsets {
		if registered.Approval() == offset.Approval() && registered.Instruction() == offset.Instruction() {
			return ports.SaveAlreadyRegistered, nil
		}
	}
	store.offsets[key] = offset
	return ports.SaveRegistered, nil
}

func (store *storeDouble) LoadInstruction(
	_ context.Context, _ domain.TenantID, id domain.CollectionInstructionID,
) (domain.CollectionInstruction, bool, error) {
	if store.readErr != nil {
		return domain.CollectionInstruction{}, false, store.readErr
	}
	instruction, found := store.instructions[id.String()]
	return instruction, found, nil
}

func (store *storeDouble) LoadCollectionFact(
	_ context.Context, _ domain.TenantID, id domain.CollectionFactID,
) (domain.CollectionFact, bool, error) {
	if store.readErr != nil {
		return domain.CollectionFact{}, false, store.readErr
	}
	fact, found := store.facts[id.String()]
	return fact, found, nil
}

func (store *storeDouble) LoadDiscrepancyItem(
	_ context.Context, _ domain.TenantID, id domain.DiscrepancyItemID,
) (domain.DiscrepancyItem, bool, error) {
	if store.readErr != nil {
		return domain.DiscrepancyItem{}, false, store.readErr
	}
	item, found := store.discrepancies[id.String()]
	return item, found, nil
}

func (store *storeDouble) LoadAgreedOffset(
	_ context.Context, _ domain.TenantID, id domain.AgreedOffsetID,
) (domain.AgreedOffset, bool, error) {
	if store.readErr != nil {
		return domain.AgreedOffset{}, false, store.readErr
	}
	offset, found := store.offsets[id.String()]
	return offset, found, nil
}

func (store *storeDouble) LoadInstructionFacts(
	_ context.Context, _ domain.TenantID, id domain.CollectionInstructionID,
) ([]domain.CollectionFact, error) {
	if store.readErr != nil {
		return nil, store.readErr
	}
	owned := []domain.CollectionFact{}
	for _, key := range store.factOrder {
		if fact := store.facts[key]; fact.Instruction() == id {
			owned = append(owned, fact)
		}
	}
	return owned, nil
}

func (store *storeDouble) OpenSubledger(
	_ context.Context, _ domain.TenantID, ledger domain.Subledger,
) (ports.SaveOutcome, error) {
	if store.writeErr != nil {
		return ports.SaveOutcomeInvalid, store.writeErr
	}
	if _, exists := store.subledgers[ledger.Key()]; exists {
		return ports.SaveAlreadyRegistered, nil
	}
	store.subledgers[ledger.Key()] = ledger
	return ports.SaveRegistered, nil
}

// LockLedger 在替身上只记一笔：内存替身没有并发，锁的效果由真库用例证。
func (store *storeDouble) LockLedger(
	_ context.Context, _ domain.TenantID, key domain.SubledgerKey,
) error {
	if store.writeErr != nil {
		return store.writeErr
	}
	store.locked = append(store.locked, key)
	return nil
}

func (store *storeDouble) AppendPosting(
	_ context.Context, _ domain.TenantID, posting domain.SubledgerPosting,
) (ports.SaveOutcome, error) {
	if store.writeErr != nil {
		return ports.SaveOutcomeInvalid, store.writeErr
	}
	key := posting.ID().String()
	if _, exists := store.postings[key]; exists {
		return ports.SaveAlreadyRegistered, nil
	}
	store.postings[key] = posting
	store.postingOrder = append(store.postingOrder, key)
	return ports.SaveRegistered, nil
}

func (store *storeDouble) LoadSubledger(
	_ context.Context, _ domain.TenantID, key domain.SubledgerKey,
) (domain.Subledger, bool, error) {
	if store.readErr != nil {
		return domain.Subledger{}, false, store.readErr
	}
	ledger, found := store.subledgers[key]
	return ledger, found, nil
}

func (store *storeDouble) LoadBalance(
	_ context.Context, _ domain.TenantID, key domain.SubledgerKey,
) (domain.SubledgerBalance, bool, error) {
	if store.readErr != nil {
		return domain.SubledgerBalance{}, false, store.readErr
	}
	if _, opened := store.subledgers[key]; !opened {
		return domain.SubledgerBalance{}, false, nil
	}
	owned := make([]domain.SubledgerPosting, 0, len(store.postingOrder))
	for _, id := range store.postingOrder {
		if posting := store.postings[id]; posting.Ledger() == key {
			owned = append(owned, posting)
		}
	}
	balance, err := domain.DeriveSubledgerBalance(key, owned)
	if err != nil {
		return domain.SubledgerBalance{}, false, err
	}
	return balance, true, nil
}

func (store *storeDouble) LoadPosting(
	_ context.Context, _ domain.TenantID, id domain.PostingID,
) (domain.SubledgerPosting, bool, error) {
	if store.readErr != nil {
		return domain.SubledgerPosting{}, false, store.readErr
	}
	posting, found := store.postings[id.String()]
	return posting, found, nil
}

// LoadCorrectionOf 交回冲正了指定记账的那一笔；一笔至多被冲正一次，所以至多一笔。
func (store *storeDouble) LoadCorrectionOf(
	_ context.Context, _ domain.TenantID, original domain.PostingID,
) (domain.SubledgerPosting, bool, error) {
	if store.readErr != nil {
		return domain.SubledgerPosting{}, false, store.readErr
	}
	for _, key := range store.postingOrder {
		posting := store.postings[key]
		if posting.BasisKind() == domain.BasisCorrection && posting.Basis().String() == original.String() {
			return posting, true, nil
		}
	}
	return domain.SubledgerPosting{}, false, nil
}

// LoadInstructionPostings 与真库读口同一口径：凭该指令的事实、差异事项、认可抵扣、清分，以及
// 沿冲正链冲正这些记账的。替身按落账次序扫一遍——冲正总在原记账之后落，一趟就够。
func (store *storeDouble) LoadInstructionPostings(
	_ context.Context, _ domain.TenantID, instruction domain.CollectionInstruction,
) ([]domain.SubledgerPosting, error) {
	if store.readErr != nil {
		return nil, store.readErr
	}
	attributed := map[string]bool{}
	owned := []domain.SubledgerPosting{}
	for _, key := range store.postingOrder {
		posting := store.postings[key]
		basis := posting.Basis().String()
		var mine bool
		switch posting.BasisKind() {
		case domain.BasisCollectionFact:
			mine = store.facts[basis].Instruction() == instruction.ID()
		case domain.BasisDiscrepancy:
			mine = store.discrepancies[basis].Instruction() == instruction.ID()
		case domain.BasisAgreedOffset:
			mine = store.offsets[basis].Instruction() == instruction.ID()
		case domain.BasisAllocation:
			mine = basis == instruction.ID().String()
		case domain.BasisCorrection:
			mine = attributed[basis]
		}
		if mine && posting.Ledger() == instruction.Ledger() {
			attributed[key] = true
			owned = append(owned, posting)
		}
	}
	return owned, nil
}

func (store *storeDouble) FormBatch(
	_ context.Context, _ domain.TenantID, batch domain.RemittanceBatch,
) (ports.SaveOutcome, error) {
	if store.writeErr != nil {
		return ports.SaveOutcomeInvalid, store.writeErr
	}
	key := batch.ID().String()
	if _, exists := store.batches[key]; exists {
		return ports.SaveAlreadyRegistered, nil
	}
	store.batches[key] = batch
	return ports.SaveRegistered, nil
}

func (store *storeDouble) HandOverBatchForPayment(
	_ context.Context, _ domain.TenantID, id domain.RemittanceBatchID,
) (ports.HandOverOutcome, error) {
	if store.writeErr != nil {
		return ports.HandOverOutcomeInvalid, store.writeErr
	}
	batch, found := store.batches[id.String()]
	if !found {
		return ports.HandOverTargetMissing, nil
	}
	handed, advanced := batch.HandOverForPayment()
	if !advanced {
		return ports.AlreadyHandedOver, nil
	}
	store.batches[id.String()] = handed
	return ports.HandedOver, nil
}

func (store *storeDouble) LoadBatch(
	_ context.Context, _ domain.TenantID, id domain.RemittanceBatchID,
) (domain.RemittanceBatch, bool, error) {
	if store.readErr != nil {
		return domain.RemittanceBatch{}, false, store.readErr
	}
	batch, found := store.batches[id.String()]
	return batch, found, nil
}

var (
	_ ports.CollectionRegistry = (*storeDouble)(nil)
	_ ports.CollectionView     = (*storeDouble)(nil)
	_ ports.SubledgerRegistry  = (*storeDouble)(nil)
	_ ports.SubledgerView      = (*storeDouble)(nil)
	_ ports.RemittanceRegistry = (*storeDouble)(nil)
	_ ports.RemittanceView     = (*storeDouble)(nil)
)

var errDependencyDown = errors.New("connection refused")

// —— 构造夹具 ——

func tenantID(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-T1")
	if err != nil {
		t.Fatalf("构造租户标识：%v", err)
	}
	return tenant
}

func at(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("解析时刻 %q：%v", value, err)
	}
	return parsed
}

func testLedgerKey(t *testing.T) domain.SubledgerKey {
	t.Helper()
	customer, err := domain.NewCustomerReference("SYN-CUST-1")
	if err != nil {
		t.Fatalf("构造客户引用：%v", err)
	}
	entity, err := domain.NewLegalEntityReference("SYN-ENTITY-1")
	if err != nil {
		t.Fatalf("构造法人引用：%v", err)
	}
	code, err := domain.NewCurrencyCode("EUR")
	if err != nil {
		t.Fatalf("构造币种：%v", err)
	}
	channel, err := domain.NewCollectionChannelReference("SYN-CHANNEL-1")
	if err != nil {
		t.Fatalf("构造渠道引用：%v", err)
	}
	key, err := domain.NewSubledgerKey(customer, entity, code, channel)
	if err != nil {
		t.Fatalf("构造分户账键：%v", err)
	}
	return key
}

func testMoney(t *testing.T, amountMinor int64) domain.Money {
	t.Helper()
	code, err := domain.NewCurrencyCode("EUR")
	if err != nil {
		t.Fatalf("构造币种：%v", err)
	}
	amount, err := domain.NewMoney(code, amountMinor)
	if err != nil {
		t.Fatalf("构造金额：%v", err)
	}
	return amount
}

func testInstruction(t *testing.T, id string, amountMinor int64) domain.CollectionInstruction {
	t.Helper()
	instructionID, err := domain.NewCollectionInstructionID(id)
	if err != nil {
		t.Fatalf("构造指令标识：%v", err)
	}
	parcel, err := domain.NewParcelReference("SYN-PARCEL-1")
	if err != nil {
		t.Fatalf("构造包裹引用：%v", err)
	}
	requirement, err := domain.NewServiceRequirementReference("SYN-COD-REQ-V1")
	if err != nil {
		t.Fatalf("构造服务要求引用：%v", err)
	}
	instruction, err := domain.IssueCollectionInstruction(domain.CollectionInstructionSpec{
		ID:           instructionID,
		Parcel:       parcel,
		Requirement:  requirement,
		Ledger:       testLedgerKey(t),
		Amount:       testMoney(t, amountMinor),
		InstructedAt: at(t, "2026-08-24T00:30:00Z"),
	})
	if err != nil {
		t.Fatalf("成立代收指令：%v", err)
	}
	return instruction
}

func testFact(
	t *testing.T, id, instruction string, layer domain.CollectionSourceLayer, amountMinor int64,
) domain.CollectionFact {
	t.Helper()
	factID, err := domain.NewCollectionFactID(id)
	if err != nil {
		t.Fatalf("构造事实标识：%v", err)
	}
	instructionID, err := domain.NewCollectionInstructionID(instruction)
	if err != nil {
		t.Fatalf("构造指令标识：%v", err)
	}
	evidence, err := domain.NewEvidenceReference("SYN-EVIDENCE-" + id)
	if err != nil {
		t.Fatalf("构造证据引用：%v", err)
	}
	fact, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID:          factID,
		Instruction: instructionID,
		Layer:       layer,
		Evidence:    evidence,
		Amount:      testMoney(t, amountMinor),
		OccurredAt:  at(t, "2026-08-24T01:00:00Z"),
	})
	if err != nil {
		t.Fatalf("接受代收事实：%v", err)
	}
	return fact
}

func testSubledger(t *testing.T) domain.Subledger {
	t.Helper()
	basis, err := domain.NewCustodyBasisReference("SYN-COD-RESPONSIBILITY-V1")
	if err != nil {
		t.Fatalf("构造受托依据：%v", err)
	}
	ledger, err := domain.OpenSubledger(testLedgerKey(t), basis, at(t, "2026-08-24T00:00:00Z"))
	if err != nil {
		t.Fatalf("开立分户账：%v", err)
	}
	return ledger
}

func testBatch(t *testing.T, id string) domain.RemittanceBatch {
	t.Helper()
	batchID, err := domain.NewRemittanceBatchID(id)
	if err != nil {
		t.Fatalf("构造批次标识：%v", err)
	}
	batch, err := domain.FormRemittanceBatch(domain.RemittanceBatchSpec{
		ID:               batchID,
		Ledger:           testLedgerKey(t),
		CollectedThrough: at(t, "2026-08-31T00:00:00Z"),
		FormedAt:         at(t, "2026-08-31T01:00:00Z"),
	})
	if err != nil {
		t.Fatalf("形成批次：%v", err)
	}
	return batch
}

func testDiscrepancy(
	t *testing.T, id, instruction string, kind domain.DiscrepancyKind, amountMinor int64,
) domain.DiscrepancyItem {
	t.Helper()
	itemID, err := domain.NewDiscrepancyItemID(id)
	if err != nil {
		t.Fatalf("构造差异标识：%v", err)
	}
	instructionID, err := domain.NewCollectionInstructionID(instruction)
	if err != nil {
		t.Fatalf("构造指令标识：%v", err)
	}
	basis, err := domain.NewBasisReference("SYN-CHANNEL-REPORT-1")
	if err != nil {
		t.Fatalf("构造依据引用：%v", err)
	}
	item, err := domain.RegisterDiscrepancyItem(domain.DiscrepancyItemSpec{
		ID:          itemID,
		Instruction: instructionID,
		Kind:        kind,
		Amount:      testMoney(t, amountMinor),
		Basis:       basis,
		ObservedAt:  at(t, "2026-08-25T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("登记差异事项：%v", err)
	}
	return item
}

func postCommand(
	t *testing.T,
	id string,
	from, to domain.FundPosition,
	amountMinor int64,
	kind domain.PostingBasisKind,
	basis string,
	postedAt string,
) application.PostCommand {
	t.Helper()
	postingID, err := domain.NewPostingID(id)
	if err != nil {
		t.Fatalf("构造记账标识：%v", err)
	}
	basisRef, err := domain.NewBasisReference(basis)
	if err != nil {
		t.Fatalf("构造依据引用：%v", err)
	}
	return application.PostCommand{
		Tenant:      tenantID(t),
		ID:          postingID,
		From:        from,
		To:          to,
		AmountMinor: amountMinor,
		BasisKind:   kind,
		Basis:       basisRef,
		PostedAt:    at(t, postedAt),
	}
}
