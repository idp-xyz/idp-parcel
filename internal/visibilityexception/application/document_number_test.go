package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// Covers: CONTEXT「异常案件」——建立案件时在同一事务内取得案件单据号：自动建案那一轮取一次号、号随案件进同一份记录；
// 同一连续影响期的重复命中不建案、不取号（同一开立意图重放不再取号）；不建案的走向不取号。
func TestAnAutoEstablishedCaseTakesOneNumberAndARepeatHitTakesNone(t *testing.T) {
	fixture := newRaiseFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, raiseCommand(t, signalHitAt))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	established, present := first.Case()
	if !present {
		t.Fatal("自动建案没有建出案件")
	}
	if number, numbered := established.DocumentNumber().Number(); !numbered || number != "EX-20260810-00001" {
		t.Fatalf("number = %q numbered = %v", number, numbered)
	}
	if len(fixture.episodes.cases) != 1 || fixture.episodes.cases[0].DocumentNumber() != established.DocumentNumber() {
		t.Fatal("单据号没有随案件进同一份记录")
	}
	if fixture.caseNumbers.calls != 1 || fixture.caseNumbers.tenants[0].String() != "tenant-1" {
		t.Fatalf("取号 %d 次、租户 %v，应为本租户一次", fixture.caseNumbers.calls, fixture.caseNumbers.tenants)
	}

	if _, err := fixture.handler.Handle(ctx, raiseCommand(t, signalHitAt.Add(time.Hour))); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if fixture.caseNumbers.calls != 1 {
		t.Fatalf("重复命中又取了号：%d", fixture.caseNumbers.calls)
	}

	manual := newRaiseFixture(t)
	manual.triage.answer = ports.TriageAnswer{
		Outcome: domain.ManualReviewRequired,
		Rule:    mustValue(t, domain.NewSignalRuleVersionReference, "triage-rules/v2"),
	}
	if _, err := manual.handler.Handle(ctx, raiseCommand(t, signalHitAt)); err != nil {
		t.Fatalf("manual: %v", err)
	}
	if manual.caseNumbers.calls != 0 {
		t.Fatal("人工复核走向取了案件单据号")
	}
}

// Covers: ADR-0355 决定七——单据编号时区未配置时案件照常建立、不带单据号，记无号原因。
func TestACaseOpenedWithoutANumberingTimeZoneIsEstablishedWithoutANumber(t *testing.T) {
	fixture := newRaiseFixture(t)
	fixture.caseNumbers.outcome = ports.DocumentNumberingTimeZoneNotConfigured

	result, err := fixture.handler.Handle(context.Background(), raiseCommand(t, signalHitAt))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	established, present := result.Case()
	if !present || result.Outcome() != application.SignalEpisodeOpened {
		t.Fatalf("outcome = %q case = %v；时区未配置不该挡住建案", result.Outcome(), present)
	}
	if _, numbered := established.DocumentNumber().Number(); numbered ||
		established.DocumentNumber().NumberlessReason() != domain.NumberingTimeZoneNotConfigured {
		t.Fatalf("无号原因 = %q", established.DocumentNumber().NumberlessReason())
	}
}

// Covers: ADR-0355 决定八——案件单据号撞号：这一轮发作期、结论与案件都不落、不交分诊意图，答未决并指名「案件单据号撞号」；
// 取号端口出错则上抛，不译成业务答案。
func TestACaseNumberCollisionLandsNothingAndAnIssuerFailureIsRaised(t *testing.T) {
	fixture := newRaiseFixture(t)
	fixture.caseNumbers.outcome = ports.DocumentNumberingRuleCollision

	result, err := fixture.handler.Handle(context.Background(), raiseCommand(t, signalHitAt))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.RaiseSignalUndecided || result.UndecidedReason() != application.CaseNumberingRuleCollision ||
		result.UndecidedReason().String() != "CASE_NUMBERING_RULE_COLLISION" {
		t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
	}
	if fixture.episodes.raisedSaves != 0 || len(fixture.downstream.intents) != 0 {
		t.Fatal("撞号那一轮落了记录或交了意图")
	}

	failing := newRaiseFixture(t)
	failing.caseNumbers.err = errors.New("numbering unavailable")
	if _, err := failing.handler.Handle(context.Background(), raiseCommand(t, signalHitAt)); err == nil {
		t.Fatal("取号出错被吞成了业务答案")
	}
	if failing.episodes.raisedSaves != 0 {
		t.Fatal("取号出错仍落了记录")
	}

	odd := newRaiseFixture(t)
	odd.caseNumbers.outcome = ports.DocumentNumberIssueOutcomeInvalid
	if _, err := odd.handler.Handle(context.Background(), raiseCommand(t, signalHitAt)); !errors.Is(err, application.ErrUnexpectedDocumentNumberIssue) {
		t.Fatalf("封闭集合外的取号答案没有上抛：%v", err)
	}
}

// Covers: CONTEXT「客户索赔项」——客户提交被记录时取得索赔单据号：新项取一次号、号随项落库；同一（批次+项）重放答原项与
// 原号，不再取号；要件不全的提交不取号。
func TestAReceivedClaimTakesOneNumberAndAReplayAnswersTheOriginal(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()

	received, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	claim, present := received.Claim()
	if !present || received.Outcome() != application.ClaimReceived {
		t.Fatalf("outcome = %q", received.Outcome())
	}
	if number, numbered := claim.DocumentNumber().Number(); !numbered || number != "CLM-202608-00001" {
		t.Fatalf("number = %q numbered = %v", number, numbered)
	}
	if received.DiscardIssuing() {
		t.Fatal("落了单的受理被要求回滚")
	}

	replay, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	original, _ := replay.Claim()
	if replay.Outcome() != application.ClaimExistingResult || original.DocumentNumber() != claim.DocumentNumber() {
		t.Fatalf("重放 outcome = %q，号 %+v", replay.Outcome(), original.DocumentNumber())
	}
	if fixture.claimNumbers.calls != 1 {
		t.Fatalf("重放又取了号：%d", fixture.claimNumbers.calls)
	}

	incomplete := receiveCommand(t, "item-2")
	incomplete.Applicant = domain.ApplicantReference{}
	if _, err := fixture.handler.ReceiveClaim(ctx, incomplete); err != nil {
		t.Fatalf("incomplete: %v", err)
	}
	if fixture.claimNumbers.calls != 1 {
		t.Fatal("未受理的提交取了号")
	}
}

// Covers: ADR-0355 决定七、八——单据编号时区未配置时索赔照常记录、不带号；撞号时这一次提交不记下、答「编号规则撞号」，
// 不并进未决；取号出错上抛。
func TestAClaimWithoutATimeZoneIsRecordedAndACollisionRecordsNothing(t *testing.T) {
	ctx := context.Background()

	unconfigured := newClaimFixture(t)
	unconfigured.claimNumbers.outcome = ports.DocumentNumberingTimeZoneNotConfigured
	result, err := unconfigured.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	claim, _ := result.Claim()
	if result.Outcome() != application.ClaimReceived || claim.DocumentNumber().NumberlessReason() != domain.NumberingTimeZoneNotConfigured {
		t.Fatalf("outcome = %q", result.Outcome())
	}

	collided := newClaimFixture(t)
	collided.claimNumbers.outcome = ports.DocumentNumberingRuleCollision
	result, err = collided.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if result.Outcome() != application.ClaimNumberingRuleCollision || result.Outcome().String() != "NUMBERING_RULE_COLLISION" {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	if _, present := result.Claim(); present || collided.claims.saves != 0 {
		t.Fatal("撞号的提交被记下了")
	}

	failing := newClaimFixture(t)
	failing.claimNumbers.err = errors.New("numbering unavailable")
	if _, err := failing.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err == nil || failing.claims.saves != 0 {
		t.Fatalf("取号出错被吞了或仍落了单：%v", err)
	}
}

// Covers: ADR-0355 决定三——取到号却没落单的受理分支（写库失败、并发受理输给另一方）要调用方回滚，号不留成空号；
// 时区未配置那一格没动序列，输掉竞争也不要求回滚。
func TestAReceiptThatTookANumberButLandedNothingAsksForARollback(t *testing.T) {
	ctx := context.Background()

	broken := newClaimFixture(t)
	broken.claims.saveErr = errors.New("store unavailable")
	result, err := broken.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if result.Outcome() != application.HandleClaimUndecided || !result.DiscardIssuing() {
		t.Fatalf("outcome = %q discard = %v", result.Outcome(), result.DiscardIssuing())
	}

	unnumbered := newClaimFixture(t)
	unnumbered.claimNumbers.outcome = ports.DocumentNumberingTimeZoneNotConfigured
	winner, err := domain.ReceiveClaimItem(domain.ClaimItemSpec{
		ID:             mustValue(t, domain.NewClaimItemID, "item-1"),
		Batch:          mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		DocumentNumber: domain.NumberlessForUnconfiguredTimeZone(),
		Customer:       mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Applicant:      mustValue(t, domain.NewApplicantReference, "applicant-1"),
		Contract:       mustValue(t, domain.NewContractScopeReference, "contract-scope/v1"),
		Target:         mustValue(t, domain.NewRequestScopeReference, "parcel-1/loss"),
		Kind:           mustValue(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt:    claimSubmittedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	unnumbered.claims.saveConflicts = 1
	unnumbered.claims.conflictWinner = winner
	result, err = unnumbered.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if result.Outcome() != application.ClaimExistingResult || result.DiscardIssuing() {
		t.Fatalf("outcome = %q discard = %v；没取号就没有要退的", result.Outcome(), result.DiscardIssuing())
	}
}
