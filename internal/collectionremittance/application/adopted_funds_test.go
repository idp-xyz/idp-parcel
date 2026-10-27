package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/application"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

const fundsRef = "SYN-BANK-RECEIPT-1"

// arrivalOn 是一条引用同一条已采用事实 fundsRef 的运营企业真实到账（证据引用就是那条事实的引用）。
func arrivalOn(t *testing.T, id, instruction, funds string, amountMinor int64) domain.CollectionFact {
	t.Helper()
	fact := testFact(t, id, instruction, domain.OperatorBankCredit, amountMinor)
	evidence, err := domain.NewEvidenceReference(funds)
	if err != nil {
		t.Fatalf("构造证据引用：%v", err)
	}
	rebuilt, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID: fact.ID(), Instruction: fact.Instruction(), Layer: fact.Layer(),
		Evidence: evidence, Amount: fact.Amount(), OccurredAt: fact.OccurredAt(),
	})
	if err != nil {
		t.Fatalf("重建到账：%v", err)
	}
	return rebuilt
}

func acceptArrival(t *testing.T, handler *application.Handler, fact domain.CollectionFact) application.Outcome {
	t.Helper()
	outcome, err := handler.AcceptFact(t.Context(), application.AcceptFactCommand{Tenant: tenantID(t), Fact: fact})
	if err != nil {
		t.Fatalf("接受到账 %s：%v", fact.ID(), err)
	}
	return outcome
}

func wantOutcome(t *testing.T, got, want application.Outcome, what string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %s，要 %s", what, got, want)
	}
}

func armedWithInstructions(t *testing.T, instructions ...string) (*storeDouble, *application.Handler) {
	t.Helper()
	store := newStore()
	handler := store.handler()
	for _, instruction := range instructions {
		seedInstruction(t, store, handler, instruction, 100_000)
	}
	return store, handler
}

// Covers: AT-CR-020——真实到账的证据引用指不到结算已采用且已认作代收本金的事实：先在结算采用并认作，再登记；
// 没有就答依据不在册、什么都不落（迟到的到账等采用，不是登一份、再补依据）。
func TestAnArrivalWhoseEvidenceIsNotAnAdoptedPrincipalFactIsBasisMissing(t *testing.T) {
	store, handler := armedWithInstructions(t, "SYN-INSTR-1")

	arrival := arrivalOn(t, "SYN-FACT-A", "SYN-INSTR-1", fundsRef, 60_000)
	wantOutcome(t, acceptArrival(t, handler, arrival), application.OutcomeBasisMissing, "引用还没在结算采用")
	if len(store.facts) != 0 {
		t.Fatalf("依据不在册仍落了 %d 条事实", len(store.facts))
	}

	store.adoptFunds(t, fundsRef, 100_000)
	wantOutcome(t, acceptArrival(t, handler, arrival), application.OutcomeRegistered, "采用并认作之后重跑同一份")
	saved, found, _ := store.LoadCollectionFact(t.Context(), tenantID(t), arrival.ID())
	if !found {
		t.Fatal("登记成功却没落册")
	}
	if version, pinned := saved.AdoptedFundsVersion(); !pinned || version.String() != fundsRef+"/v1" {
		t.Fatalf("落册的到账钉的版本 = (%q, %v)，要登记时读到的链头 %s/v1", version, pinned, fundsRef)
	}
}

// Covers: AT-CR-020——币种：到账币种与代收指令所属那本账不同，或与它引用的事实不同，都是受理门拒（改请求），
// 且先于对结算的读：币种对不上的请求不该去占结算的共享锁。
func TestAnArrivalInTheWrongCurrencyIsNotAccepted(t *testing.T) {
	store, handler := armedWithInstructions(t, "SYN-INSTR-1")
	store.adoptFunds(t, fundsRef, 100_000)

	usd, err := domain.NewCurrencyCode("USD")
	if err != nil {
		t.Fatal(err)
	}
	dollars, err := domain.NewMoney(usd, 60_000)
	if err != nil {
		t.Fatal(err)
	}
	base := arrivalOn(t, "SYN-FACT-A", "SYN-INSTR-1", fundsRef, 60_000)
	wrongLedger, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID: base.ID(), Instruction: base.Instruction(), Layer: base.Layer(),
		Evidence: base.Evidence(), Amount: dollars, OccurredAt: base.OccurredAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	wantOutcome(t, acceptArrival(t, handler, wrongLedger), application.OutcomeNotAccepted, "到账币种不是账的币种")
	if len(store.fundsLog) != 0 {
		t.Fatalf("币种对不上的请求仍去取了锁或读了结算：%v", store.fundsLog)
	}

	// 账与到账同为 EUR、事实却是 USD：结算一侧的事实币种与到账不同。
	foreign, err := domain.NewMoney(usd, 100_000)
	if err != nil {
		t.Fatal(err)
	}
	version, _ := domain.NewAdoptedFundsVersion("v1")
	funds, err := domain.NewAdoptedFunds(version, foreign)
	if err != nil {
		t.Fatal(err)
	}
	store.adopted[fundsRef] = funds
	wantOutcome(t, acceptArrival(t, handler, base), application.OutcomeNotAccepted, "到账币种不是事实的币种")
	if len(store.facts) != 0 {
		t.Fatalf("币种不同仍落了 %d 条事实", len(store.facts))
	}
}

// Covers: AT-CR-020——一笔 1000.00 的回款拆给两条指令（600.00 + 400.00）合计恰好等于事实，照登；
// 再多一分的第三条答内容冲突、不落册；已登记的两条与已采用事实的合计始终等于事实金额。
func TestArrivalsSplitFromOneAdoptedFactNeverExceedItsAmount(t *testing.T) {
	store, handler := armedWithInstructions(t, "SYN-INSTR-1", "SYN-INSTR-2", "SYN-INSTR-3")
	store.adoptFunds(t, fundsRef, 100_000)

	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-A", "SYN-INSTR-1", fundsRef, 60_000)),
		application.OutcomeRegistered, "600.00")
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-B", "SYN-INSTR-2", fundsRef, 40_000)),
		application.OutcomeRegistered, "400.00，合计恰好等于事实")
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-C", "SYN-INSTR-3", fundsRef, 1)),
		application.OutcomeContentConflict, "再多一分")

	referenced, err := store.LoadAdoptedFundsReferences(t.Context(), tenantID(t), mustReference(t, fundsRef))
	if err != nil {
		t.Fatal(err)
	}
	total := int64(0)
	for _, fact := range referenced {
		total += fact.Amount().AmountMinor()
	}
	if len(referenced) != 2 || total != 100_000 {
		t.Fatalf("落册的引用 %d 条、合计 %d，要 2 条、合计 100000", len(referenced), total)
	}
}

// Covers: AT-CR-026——重复到达与换标识重登：同一份重放答已在册、合计不翻倍；同标识改金额、同一条指令换事实标识再登
// （含金额更小）都答内容冲突；重放先于一切按此刻状态作答的门——事实之后被结算更正小了，重放仍答已在册。
func TestARepeatedArrivalIsNotCountedTwiceAndAReLoggedOneIsRefused(t *testing.T) {
	store, handler := armedWithInstructions(t, "SYN-INSTR-1", "SYN-INSTR-2")
	store.adoptFunds(t, fundsRef, 100_000)
	first := arrivalOn(t, "SYN-FACT-A", "SYN-INSTR-1", fundsRef, 60_000)
	wantOutcome(t, acceptArrival(t, handler, first), application.OutcomeRegistered, "首次")

	wantOutcome(t, acceptArrival(t, handler, first), application.OutcomeExisting, "同一份重放")
	if len(store.facts) != 1 {
		t.Fatalf("重放后册上 %d 条", len(store.facts))
	}

	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-A", "SYN-INSTR-1", fundsRef, 50_000)),
		application.OutcomeContentConflict, "同标识改金额")
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-A2", "SYN-INSTR-1", fundsRef, 60_000)),
		application.OutcomeContentConflict, "同一指令换事实标识再登")
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-A3", "SYN-INSTR-1", fundsRef, 100)),
		application.OutcomeContentConflict, "同一指令换事实标识再登一笔小的")
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-B", "SYN-INSTR-2", fundsRef, 40_000)),
		application.OutcomeRegistered, "另一条指令的拆分")

	store.adoptFundsVersion(t, fundsRef, fundsRef+"/v2", 70_000)
	wantOutcome(t, acceptArrival(t, handler, first), application.OutcomeExisting,
		"结算把事实更正到 700.00 之后，已在册那份重放仍是已在册")
	if len(store.facts) != 2 {
		t.Fatalf("册上 %d 条，要 2 条（冲突的都没落册）", len(store.facts))
	}
}

// Covers: AT-CR-027——被结算更正：登记判的是此刻的链头金额：已登记 600.00 + 400.00 之后结算把事实下调到 900.00，
// 新的到账（哪怕一分）进不来；上调到 1200.00 则又能登。钉的版本是登记时读到的那一版，不是判金额的依据。
func TestARegistrationJudgesTheAdoptedFactsCurrentAmount(t *testing.T) {
	store, handler := armedWithInstructions(t, "SYN-INSTR-1", "SYN-INSTR-2", "SYN-INSTR-3")
	store.adoptFunds(t, fundsRef, 100_000)
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-A", "SYN-INSTR-1", fundsRef, 60_000)), application.OutcomeRegistered, "600.00")
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-B", "SYN-INSTR-2", fundsRef, 40_000)), application.OutcomeRegistered, "400.00")

	store.adoptFundsVersion(t, fundsRef, fundsRef+"/v2", 90_000)
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-C", "SYN-INSTR-3", fundsRef, 1)),
		application.OutcomeContentConflict, "事实被下调到 900.00，已引用 1000.00，再来一分")

	store.adoptFundsVersion(t, fundsRef, fundsRef+"/v3", 120_000)
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-C", "SYN-INSTR-3", fundsRef, 20_000)),
		application.OutcomeRegistered, "事实被上调到 1200.00 后登 200.00")
	saved, _, _ := store.LoadCollectionFact(t.Context(), tenantID(t), mustFactID(t, "SYN-FACT-C"))
	if version, _ := saved.AdoptedFundsVersion(); version.String() != fundsRef+"/v3" {
		t.Fatalf("钉的版本 = %s，要登记时读到的 v3", version)
	}
}

// Covers: AT-CR-026——取锁与读回的次序：先取本上下文的事实锁，再读结算（共享锁）；合计在锁之后读回：等锁期间另一路引用同一条
// 事实的到账提交了，这一路读到它、判超额，而不是拿着锁前的旧合计放行。
func TestTheTotalIsReadAfterTheFundsLockSoALateCompetitorIsSeen(t *testing.T) {
	store, handler := armedWithInstructions(t, "SYN-INSTR-1", "SYN-INSTR-2")
	store.adoptFunds(t, fundsRef, 100_000)

	competitor := arrivalOn(t, "SYN-FACT-B", "SYN-INSTR-2", fundsRef, 60_000)
	store.afterFundsLock = func() {
		if err := store.registerForTest(competitor); err != nil {
			t.Fatalf("竞争者落册：%v", err)
		}
	}
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-A", "SYN-INSTR-1", fundsRef, 60_000)),
		application.OutcomeContentConflict, "等锁期间别的到账提交后，合计 1200.00 超过 1000.00")
	if _, found, _ := store.LoadCollectionFact(t.Context(), tenantID(t), mustFactID(t, "SYN-FACT-A")); found {
		t.Fatal("超额的这一条仍落了册")
	}
	if want := []string{"references:" + fundsRef, "settlement:" + fundsRef}; len(store.fundsLog) != 2 ||
		store.fundsLog[0] != want[0] || store.fundsLog[1] != want[1] {
		t.Fatalf("取锁次序 = %v，要先本上下文事实锁、后结算共享锁 %v", store.fundsLog, want)
	}
}

// Covers: AT-CR-027（读不动答未决）、ADR-0029——结算读不动、本上下文取锁失败都是未决，不折成依据不在册或任何业务答案；别的三层不碰结算。
func TestAnUnreadableSettlementIsUndecidedAndOtherLayersNeverReadIt(t *testing.T) {
	store, handler := armedWithInstructions(t, "SYN-INSTR-1")
	store.adoptFunds(t, fundsRef, 100_000)
	arrival := arrivalOn(t, "SYN-FACT-A", "SYN-INSTR-1", fundsRef, 60_000)

	store.fundsReadErr = errors.New("settlement read model unavailable")
	wantOutcome(t, acceptArrival(t, handler, arrival), application.OutcomeUndecided, "结算读不动")
	store.fundsReadErr = nil
	if len(store.facts) != 0 {
		t.Fatal("未决仍落了册")
	}

	store.writeErr = errors.New("lock unavailable")
	wantOutcome(t, acceptArrival(t, handler, arrival), application.OutcomeUndecided, "取锁失败")
	store.writeErr = nil
	wantOutcome(t, acceptArrival(t, handler, arrival), application.OutcomeRegistered, "依赖恢复后重跑同一份")

	store.fundsLog = nil
	for index, layer := range []domain.CollectionSourceLayer{
		domain.RecipientPayment, domain.ChannelCollectionReport, domain.ChannelRemittanceNotice,
	} {
		other := testFact(t, "SYN-FACT-OTHER-"+string(rune('a'+index)), "SYN-INSTR-1", layer, 100)
		wantOutcome(t, acceptArrival(t, handler, other), application.OutcomeRegistered, layer.String())
	}
	if len(store.fundsLog) != 0 {
		t.Fatalf("别的三层碰了结算或事实锁：%v", store.fundsLog)
	}
}

// Covers: AT-CR-027——被结算更正之后的移位：一笔 1000.00 的事实拆给两条指令，到账 600.00 已移位；结算把事实更正到
// 900.00 之后，第二条到账 400.00 的移位答内容冲突、账面不动（待清分仍是 600.00）；已落的移位重放仍答已在册；更正回
// 1000.00 之后移位可落。冲正已落的移位不读结算。
func TestMovingAnArrivalNeedsTheAdoptedFactToStillCoverAllItsArrivals(t *testing.T) {
	store, handler := armedWithInstructions(t, "SYN-INSTR-1", "SYN-INSTR-2")
	openLedger(t, store, handler)
	store.adoptFunds(t, fundsRef, 100_000)
	seedFact(t, store, handler, "SYN-FACT-REPORT-1", "SYN-INSTR-1", domain.ChannelCollectionReport, 60_000)
	seedFact(t, store, handler, "SYN-FACT-REPORT-2", "SYN-INSTR-2", domain.ChannelCollectionReport, 40_000)
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		60_000, domain.BasisCollectionFact, "SYN-FACT-REPORT-1", "2026-08-24T01:10:00Z"))
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT-2", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		40_000, domain.BasisCollectionFact, "SYN-FACT-REPORT-2", "2026-08-24T01:11:00Z"))
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-A", "SYN-INSTR-1", fundsRef, 60_000)), application.OutcomeRegistered, "到账 A")
	wantOutcome(t, acceptArrival(t, handler, arrivalOn(t, "SYN-FACT-B", "SYN-INSTR-2", fundsRef, 40_000)), application.OutcomeRegistered, "到账 B")

	moveA := postCommand(t, "SYN-POST-MOVE-A", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		60_000, domain.BasisCollectionFact, "SYN-FACT-A", "2026-08-24T02:00:00Z")
	mustPost(t, handler, moveA)

	store.adoptFundsVersion(t, fundsRef, fundsRef+"/v2", 90_000)
	moveB := postCommand(t, "SYN-POST-MOVE-B", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		40_000, domain.BasisCollectionFact, "SYN-FACT-B", "2026-08-24T02:01:00Z")
	outcome, err := handler.PostSubledger(t.Context(), moveB)
	if err != nil {
		t.Fatal(err)
	}
	wantOutcome(t, outcome, application.OutcomeContentConflict, "事实被更正到 900.00，已引用 1000.00，B 的移位")
	wantAwaitingAllocation(t, store, 60_000)

	outcome, _ = handler.PostSubledger(t.Context(), moveA)
	wantOutcome(t, outcome, application.OutcomeExisting, "已落的移位 A 重放")

	reversal := postCommand(t, "SYN-POST-UNDO-A", domain.PositionAwaitingAllocation, domain.PositionInTransitAtChannel,
		60_000, domain.BasisCorrection, "SYN-POST-MOVE-A", "2026-08-24T03:00:00Z")
	mustPost(t, handler, reversal)
	wantAwaitingAllocation(t, store, 0)

	store.adoptFundsVersion(t, fundsRef, fundsRef+"/v3", 100_000)
	mustPost(t, handler, moveB)
	wantAwaitingAllocation(t, store, 40_000)
}

// Covers: AT-CR-027——移位的采用事实门其余格：没有版本的到账（按旧口径登记的存量）答依据不在册；事实此刻读不到答依据不在册，
// 读不动答未决；渠道报告入账不碰结算。
func TestMovingAnArrivalWithoutAnAdoptedBasisIsRefusedAndOtherPostingsNeverReadSettlement(t *testing.T) {
	store, handler := armedWithInstructions(t, "SYN-INSTR-1")
	openLedger(t, store, handler)
	seedFact(t, store, handler, "SYN-FACT-REPORT-1", "SYN-INSTR-1", domain.ChannelCollectionReport, 60_000)
	store.fundsLog = nil
	mustPost(t, handler, postCommand(t, "SYN-POST-REPORT-1", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		60_000, domain.BasisCollectionFact, "SYN-FACT-REPORT-1", "2026-08-24T01:10:00Z"))
	if len(store.fundsLog) != 0 {
		t.Fatalf("渠道报告入账碰了结算：%v", store.fundsLog)
	}

	legacy := arrivalOn(t, "SYN-FACT-LEGACY", "SYN-INSTR-1", "SYN-LEGACY-EVIDENCE", 60_000)
	if err := store.registerForTest(legacy); err != nil {
		t.Fatal(err)
	}
	move := postCommand(t, "SYN-POST-MOVE-LEGACY", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
		60_000, domain.BasisCollectionFact, "SYN-FACT-LEGACY", "2026-08-24T02:00:00Z")
	outcome, _ := handler.PostSubledger(t.Context(), move)
	wantOutcome(t, outcome, application.OutcomeBasisMissing, "没有钉版本的存量到账")

	pinned, err := legacy.PinnedToAdoptedFunds(mustVersion(t, "SYN-LEGACY-EVIDENCE/v1"))
	if err != nil {
		t.Fatal(err)
	}
	store.facts[legacy.ID().String()] = pinned
	outcome, _ = handler.PostSubledger(t.Context(), move)
	wantOutcome(t, outcome, application.OutcomeBasisMissing, "钉了版本但结算一侧读不到这条事实")

	store.fundsReadErr = errors.New("settlement read model unavailable")
	outcome, _ = handler.PostSubledger(t.Context(), move)
	wantOutcome(t, outcome, application.OutcomeUndecided, "结算读不动")
	store.fundsReadErr = nil

	store.adoptFunds(t, "SYN-LEGACY-EVIDENCE", 60_000)
	mustPost(t, handler, move)
	wantAwaitingAllocation(t, store, 60_000)
}

func mustReference(t *testing.T, value string) domain.AdoptedFundsReference {
	t.Helper()
	reference, err := domain.NewAdoptedFundsReference(value)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

func mustVersion(t *testing.T, value string) domain.AdoptedFundsVersion {
	t.Helper()
	version, err := domain.NewAdoptedFundsVersion(value)
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func mustFactID(t *testing.T, value string) domain.CollectionFactID {
	t.Helper()
	id, err := domain.NewCollectionFactID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// registerForTest 绕过编排直接落一条事实：造「别的写入方已提交」或「按旧口径登记的存量」。
func (store *storeDouble) registerForTest(fact domain.CollectionFact) error {
	_, err := store.AcceptCollectionFact(context.Background(), domain.TenantID{}, fact)
	return err
}

func wantAwaitingAllocation(t *testing.T, store *storeDouble, want int64) {
	t.Helper()
	balance, opened, err := store.LoadBalance(t.Context(), tenantID(t), testLedgerKey(t))
	if err != nil || !opened {
		t.Fatalf("读账面：opened=%v err=%v", opened, err)
	}
	if got := balance.At(domain.PositionAwaitingAllocation); got != want {
		t.Fatalf("待清分 = %d，要 %d", got, want)
	}
}
