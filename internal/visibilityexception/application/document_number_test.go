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

// Covers: AT-VE-227（编排半边）、CONTEXT「异常案件」——建立案件时在同一事务内取得案件单据号：自动建案那一轮取一次号、号随案件进同一份记录；
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

// Covers: AT-VE-228（编排半边）、ADR-0355 决定七——单据编号时区未配置时案件照常建立、不带单据号，记无号原因。
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
		established.DocumentNumber().Absence() != domain.NumberingTimeZoneNotConfigured {
		t.Fatalf("无号原因 = %q", established.DocumentNumber().Absence())
	}
}

// Covers: AT-VE-228（编排半边）、ADR-0355 决定八——案件单据号撞号：这一轮发作期、结论与案件都不落、不交分诊意图，答业务答案
// 「案件单据号撞号」（不是未决），那一次命中连同撞上的格式记进案件撞号册；记不下才答未决；取号端口出错上抛，不译成业务答案。
func TestACaseNumberCollisionLandsNothingButRecordsTheHit(t *testing.T) {
	fixture := newRaiseFixture(t)
	fixture.caseNumbers.outcome = ports.DocumentNumberingRuleCollision
	fixture.caseNumbers.number = ""
	command := raiseCommand(t, signalHitAt)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.CaseNumberingCollided || result.Outcome().String() != "CASE_NUMBERING_RULE_COLLISION" {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	if fixture.episodes.raisedSaves != 0 || len(fixture.downstream.intents) != 0 {
		t.Fatal("撞号那一轮落了记录或交了意图")
	}
	if len(fixture.collisions.recorded) != 1 {
		t.Fatalf("撞号册记了 %d 条", len(fixture.collisions.recorded))
	}
	recorded := fixture.collisions.recorded[0]
	if recorded.Tenant != command.TenantID || recorded.Hit.Parcel != command.Parcel || recorded.Hit.Cause != command.Cause ||
		!recorded.Hit.At.Equal(command.HitAt) || len(recorded.Hit.Basis) != 1 || recorded.At.IsZero() {
		t.Fatalf("记下的命中走样：%+v", recorded)
	}

	unrecorded := newRaiseFixture(t)
	unrecorded.caseNumbers.outcome = ports.DocumentNumberingRuleCollision
	unrecorded.collisions.recordErr = errors.New("collision book unavailable")
	if result, err := unrecorded.handler.Handle(context.Background(), raiseCommand(t, signalHitAt)); err != nil ||
		result.Outcome() != application.RaiseSignalUndecided || result.UndecidedReason() != application.SignalEpisodeStoreUnavailable {
		t.Fatalf("撞号册记不下：%v %q", err, result.Outcome())
	}

	failing := newRaiseFixture(t)
	failing.caseNumbers.err = errors.New("numbering unavailable")
	if _, err := failing.handler.Handle(context.Background(), raiseCommand(t, signalHitAt)); err == nil {
		t.Fatal("取号出错被吞成了业务答案")
	}
	if failing.episodes.raisedSaves != 0 || len(failing.collisions.recorded) != 0 {
		t.Fatal("取号出错仍落了记录")
	}

	odd := newRaiseFixture(t)
	odd.caseNumbers.outcome = ports.DocumentNumberIssueOutcomeInvalid
	if _, err := odd.handler.Handle(context.Background(), raiseCommand(t, signalHitAt)); !errors.Is(err, application.ErrUnexpectedDocumentNumberIssue) {
		t.Fatalf("封闭集合外的取号答案没有上抛：%v", err)
	}
}

// Covers: AT-VE-228（编排半边）——出发门禁阻断那一路不丢撞号原因：答单独一格「案件单据号撞号」（不折进搁置），撞上的格式记在撞号册；
// 冲突信号那一路记为单独的去向，派生照常。
func TestBothSignalRoutesKeepTheCaseNumberingCollisionAsItsOwnAnswer(t *testing.T) {
	gate, _, raise := newDepartureGateSignalFixture(t)
	raise.caseNumbers.outcome = ports.DocumentNumberingRuleCollision
	raise.caseNumbers.number = ""
	raise.caseNumbers.template = "SYN-{seq:5}"
	result, err := gate.Handle(context.Background(), departureGateSignalCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DepartureGateSignalCaseNumberingCollided || result.Outcome().String() != "CASE_NUMBERING_RULE_COLLISION" {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	if len(raise.collisions.recorded) != 1 || raise.collisions.recorded[0].Template != "SYN-{seq:5}" {
		t.Fatal("出发门禁那一路撞号没连同撞上的格式记进撞号册")
	}

	derive := newDeriveFixture(t)
	derive.signals.triage.answer = ports.TriageAnswer{
		Outcome: domain.AutoEstablishCase,
		Rule:    mustValue(t, domain.NewSignalRuleVersionReference, "triage-rules/v2"),
		Team:    mustValue(t, domain.NewResponsibleTeamReference, "team/exception-ops"),
	}
	derive.signals.caseNumbers.outcome = ports.DocumentNumberingRuleCollision
	forkTwice(t, derive)
	forked, err := derive.handler.Handle(context.Background(),
		deriveCommandSuperseding(t, "TRANSPORT-HANDOVER/a", "handover/v2b", "handover/v1"))
	if err != nil {
		t.Fatalf("right successor handle: %v", err)
	}
	conflicts := forked.Conflicts()
	if forked.Outcome() != application.ProjectionDerived || len(conflicts) != 1 ||
		conflicts[0].Disposition() != application.ConflictSignalCaseNumberingCollided ||
		conflicts[0].Disposition().String() != "CASE_NUMBERING_RULE_COLLISION" {
		t.Fatalf("outcome = %q conflicts = %+v", forked.Outcome(), conflicts)
	}
	if len(derive.signals.collisions.recorded) != 1 {
		t.Fatal("冲突信号那一路撞号没留痕")
	}
}

// signalRaiserDouble 记下重驱交来的分诊命令，答错误。
type signalRaiserDouble struct {
	err      error
	commands []application.RaiseSignalCommand
}

func (double *signalRaiserDouble) Handle(_ context.Context, command application.RaiseSignalCommand) (application.RaiseSignalResult, error) {
	double.commands = append(double.commands, command)
	return application.RaiseSignalResult{}, double.err
}

// Covers: AT-VE-228（重驱半边）——撞号册上到期的命中按原样再交分诊：仍撞就推后一个重驱间隔、留在册上；走通了（建案，或
// 这回不建案，或这一原因已有活跃发作期）就销掉；分诊停在依赖上答未决错误、册上原样留着。
func TestTheRedriveRetriesACollidedHitAsItWasRecorded(t *testing.T) {
	ctx := context.Background()
	raise := newRaiseFixture(t)
	raise.caseNumbers.outcome = ports.DocumentNumberingRuleCollision
	command := raiseCommand(t, signalHitAt)
	if _, err := raise.handler.Handle(ctx, command); err != nil {
		t.Fatal(err)
	}
	book := raise.collisions
	clock := fixedClock{at: signalHitAt.Add(time.Hour)}
	redrive, err := application.NewCaseNumberingRedrive(book, clock)
	if err != nil {
		t.Fatal(err)
	}
	due, err := redrive.Due(ctx, 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("到期 %d 件 %v，want 撞号那一刻就到期的一件", len(due), err)
	}

	// 格式没换：真编排又撞，推后一个间隔。
	outcome, err := redrive.Redrive(ctx, raise.handler, due[0])
	if err != nil || outcome != application.CaseNumberingStillColliding {
		t.Fatalf("仍撞：%v %q", err, outcome)
	}
	if next := book.next[due[0].Key()]; !next.Equal(clock.at.Add(application.CaseNumberingRedriveInterval)) {
		t.Fatalf("推后到 %v", next)
	}
	if again, _ := redrive.Due(ctx, 10); len(again) != 0 {
		t.Fatal("推后之后仍算到期")
	}
	// 取页之后被推后的那一行，按取页时那一份去重驱也抢不到：答跳过，什么都不做。
	if outcome, err := redrive.Redrive(ctx, raise.handler, due[0]); err != nil || outcome != application.CaseNumberingRedriveSkipped {
		t.Fatalf("未到期仍重驱了：%v %q", err, outcome)
	}

	// 别的派发进程正锁着：跳过，不等。
	key := due[0].Key()
	book.next[key] = clock.at
	book.lockedElsewhere[key] = true
	if outcome, err := redrive.Redrive(ctx, raise.handler, due[0]); err != nil || outcome != application.CaseNumberingRedriveSkipped ||
		len(raise.episodes.cases) != 0 {
		t.Fatalf("别处锁着仍重驱了：%v %q", err, outcome)
	}
	book.lockedElsewhere[key] = false

	// 换了格式：同一只真编排这回取到号，建案、销账；重建的命令就是当初那一次命中。
	raise.caseNumbers.outcome = ports.DocumentNumberAssigned
	outcome, err = redrive.Redrive(ctx, raise.handler, due[0])
	if err != nil || outcome != application.CaseNumberingRedriven || len(raise.episodes.cases) != 1 {
		t.Fatalf("换格式后：%v %q 案件 %d", err, outcome, len(raise.episodes.cases))
	}
	if len(book.resolved) != 1 || len(book.rows) != 0 {
		t.Fatal("建成之后撞号册上那一行没销掉")
	}

	// 分诊停在依赖上：答未决错误，册上那一行原样留着（调用方回滚这一笔）。
	stalled := newRaiseFixture(t)
	stalled.episodes.findErr = errors.New("episode store unavailable")
	book.rows[key] = due[0]
	book.next[key] = clock.at
	if _, err := redrive.Redrive(ctx, stalled.handler, due[0]); !errors.Is(err, application.ErrCaseNumberingRedriveUndecided) {
		t.Fatalf("分诊未决时应答未决错误：%v", err)
	}
	if _, present := book.rows[key]; !present {
		t.Fatal("未决那一回把册上那一行销掉了")
	}
	// 没重驱成的推后一个间隔，不再占着页首。
	if err := redrive.Defer(ctx, key); err != nil || !book.next[key].Equal(clock.at.Add(application.CaseNumberingRedriveInterval)) {
		t.Fatalf("推后：%v %v", err, book.next[key])
	}
	book.next[key] = clock.at

	// 交给分诊的就是当初那一次命中。
	recorder := &signalRaiserDouble{err: errors.New("stop here")}
	if _, err := redrive.Redrive(ctx, recorder, due[0]); err == nil {
		t.Fatal("分诊报错被吞了")
	}
	if got := recorder.commands[0]; got.Cause != command.Cause || !got.HitAt.Equal(command.HitAt) || got.TenantID != command.TenantID ||
		got.Rule != command.Rule || got.Confidence != command.Confidence || len(got.Basis) != len(command.Basis) {
		t.Fatalf("重驱交的命令不是当初那一次命中：%+v", got)
	}
}

// Covers: AT-VE-229（编排半边）、CONTEXT「客户索赔项」——客户提交被记录时取得索赔单据号：新项取一次号、号随项落库；同一（批次+项）重放答原项与
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

// Covers: AT-VE-230（编排半边）、ADR-0355 决定七、八——单据编号时区未配置时索赔照常记录、不带号；撞号时这一次提交不记下、答「编号规则撞号」，
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
	if result.Outcome() != application.ClaimReceived || claim.DocumentNumber().Absence() != domain.NumberingTimeZoneNotConfigured {
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

// Covers: AT-VE-229、AT-VE-230（编排半边）、ADR-0355 决定三——取到号却没落单的受理分支（写库失败、并发受理输给另一方）要调用方回滚，号不留成空号；
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
		DocumentNumber: domain.DocumentNumberAbsentForUnconfiguredTimeZone(),
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

// Covers: AT-VE-228（编排半边）——分诊每一轮先锁这一原因在撞号册上的那一行再找发作期；同一原因再撞，册上保留首次命中，
// 重驱按首次命中开期（CONTEXT「原样重新分诊」）。
func TestTheTriageHoldsTheCollisionRowFirstAndARepeatCollisionKeepsTheFirstHit(t *testing.T) {
	ctx := context.Background()
	raise := newRaiseFixture(t)
	raise.caseNumbers.outcome = ports.DocumentNumberingRuleCollision
	first := raiseCommand(t, signalHitAt)
	second := raiseCommand(t, signalHitAt.Add(30*time.Minute))
	for _, command := range []application.RaiseSignalCommand{first, second} {
		if _, err := raise.handler.Handle(ctx, command); err != nil {
			t.Fatal(err)
		}
	}
	if len(raise.collisions.held) != 2 || raise.collisions.held[0].Cause != first.Cause {
		t.Fatalf("没在找发作期之前锁那一行：%+v", raise.collisions.held)
	}
	key := ports.CaseNumberingCollisionKey{Tenant: first.TenantID, Parcel: first.Parcel, Kind: first.Kind, Cause: first.Cause}
	if row := raise.collisions.rows[key]; !row.Hit.At.Equal(first.HitAt) {
		t.Fatalf("再撞改写了首次命中：%v", row.Hit.At)
	}

	held := newRaiseFixture(t)
	held.collisions.holdErr = errors.New("collision book unavailable")
	if result, err := held.handler.Handle(ctx, first); err != nil || result.Outcome() != application.RaiseSignalUndecided ||
		held.triage.calls != 0 {
		t.Fatalf("锁不到撞号册仍往下走了：%v %q", err, result.Outcome())
	}

	clock := fixedClock{at: signalHitAt.Add(time.Hour)}
	redrive, err := application.NewCaseNumberingRedrive(raise.collisions, clock)
	if err != nil {
		t.Fatal(err)
	}
	raise.caseNumbers.outcome = ports.DocumentNumberAssigned
	due, _ := redrive.Due(ctx, 10)
	if outcome, err := redrive.Redrive(ctx, raise.handler, due[0]); err != nil || outcome != application.CaseNumberingRedriven {
		t.Fatalf("重驱：%v %q", err, outcome)
	}
	episode := raise.episodes.latest[episodeKey{tenant: first.TenantID, parcel: first.Parcel, kind: first.Kind, cause: first.Cause}]
	if episode == nil || !episode.Snapshot().StartedAt.Equal(first.HitAt) || episode.Hits() != 1 {
		t.Fatalf("重驱建出的发作期没按首次命中开期：%+v", episode)
	}
}
