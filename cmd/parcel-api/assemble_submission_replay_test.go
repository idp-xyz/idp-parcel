package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件在真实 PostgreSQL 上证提交重发的格局（ADR-0256 决定一第 2 格、ADR-0266；票 docs-consistency/code/39）。除归属读口与
// 出向通道换成按脚本作答的替身（隔离合成 `S`，不进生产装配）、建单事务里按需注入一次失败之外，仓储、各个事务边界壳、续办
// 守卫、Outbox 与标识工厂全是生产件：要证的正是这几样之间的事务切分与排队——保全与答案逐笔提交，建单那一笔回滚时它们留下来，
// 重发据此接着建单；同一来源身份的两路续办一次只走一路。

var errInjectedBuildFailure = errors.New("injected: build transaction fails")

// Covers: `AT-PS-111`——建单那一笔事务真回滚（委托行与信封一起消失），原样重发重判仍放行，接着建成恰好一份委托与一封信封。
func TestAReplayAfterTheBuildTransactionRolledBackBuildsTheRequestOnce(t *testing.T) {
	pool, db := replayDatabase(t)
	failing := &failingOnceHandoff{}
	submission, _ := submissionAssembledThrough(t, db, &scriptedOwnership{steps: []ownershipStep{permit}}, failing.wrap, nil)
	command := submissionCommand(t)

	failing.armed = true
	if _, err := submission.Handle(t.Context(), command); !errors.Is(err, errInjectedBuildFailure) {
		t.Fatalf("首次提交 err = %v, want 建单事务的注入失败", err)
	}
	if rows := requestRowCount(t, pool, command.Identity); rows != 0 {
		t.Fatalf("建单事务回滚后委托行 = %d, want 0", rows)
	}

	replay, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重发：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.OutcomeSubmitted {
		t.Fatalf("重发 outcome = %v, want SUBMITTED——保全已落、委托不在，重发要接着把单建起来", got)
	}
	if named, present := replay.ShipmentRequestID(); !present || named != command.ShipmentRequestID {
		t.Fatalf("重发指名 %q（%v），want %q", named, present, command.ShipmentRequestID)
	}
	if rows := requestRowCount(t, pool, command.Identity); rows != 1 {
		t.Fatalf("委托行 = %d, want 1", rows)
	}
	if got := submittedEnvelopeCount(t, pool); got != 1 {
		t.Fatalf("「委托已提交」信封 = %d 份, want 1", got)
	}
}

// Covers: `AT-PS-111`——判归属的依赖答不出（保全已落，答案一行都没留，没有形成答案），原样重发重新判归属，仍放行就照常建单。
func TestAReplayAfterTheOwnershipDependencyFailedDecidesAgain(t *testing.T) {
	pool, db := replayDatabase(t)
	ownership := &scriptedOwnership{steps: []ownershipStep{dependencyDown, permit}}
	submission, _ := submissionAssembledWith(t, db, ownership)
	command := submissionCommand(t)

	if _, err := submission.Handle(t.Context(), command); !errors.Is(err, errOwnershipDependencyDown) {
		t.Fatalf("首次提交 err = %v, want 归属依赖失败原样上抛", err)
	}
	if rows := submissionAnswerCount(t, pool, command.Identity); rows != 0 {
		t.Fatalf("归属答案 = %d 行, want 0", rows)
	}

	replay, err := submission.Handle(t.Context(), command)
	if err != nil || replay.Outcome() != shipmentapp.OutcomeSubmitted {
		t.Fatalf("重发 outcome = %v, err = %v, want SUBMITTED", replay.Outcome(), err)
	}
	if ownership.calls() != 2 || requestRowCount(t, pool, command.Identity) != 1 {
		t.Fatalf("判归属 %d 次、委托 %d 行, want 2 与 1", ownership.calls(), requestRowCount(t, pool, command.Identity))
	}
}

// Covers: `AT-PS-112`——首次`生产归属未决`，原样重发重判，按这一次的结果作答：这一次放行，就建单、答`已提交`；两次判断都留在库里。
func TestAReplayAfterOwnershipUnresolvedDecidesAgainAndBuilds(t *testing.T) {
	pool, db := replayDatabase(t)
	ownership := &scriptedOwnership{steps: []ownershipStep{unresolved, permit}}
	submission, _ := submissionAssembledWith(t, db, ownership)
	command := submissionCommand(t)

	first, err := submission.Handle(t.Context(), command)
	if err != nil || first.Outcome() != shipmentapp.OutcomeOwnershipUnresolved {
		t.Fatalf("首次 outcome = %v, err = %v, want OWNERSHIP_UNRESOLVED", first.Outcome(), err)
	}
	replay, err := submission.Handle(t.Context(), command)
	if err != nil || replay.Outcome() != shipmentapp.OutcomeSubmitted {
		t.Fatalf("重发 outcome = %v, err = %v, want SUBMITTED", replay.Outcome(), err)
	}
	if decision, carried := replay.OwnershipDecision(); !carried || decision.DecisionID().String() != "SYN-OWN-DEC-1" {
		t.Fatalf("建单答复带的归属决定 = %v（%v），want 这一次放行的 SYN-OWN-DEC-1", decision.DecisionID(), carried)
	}
	if rows := submissionAnswerCount(t, pool, command.Identity); rows != 2 {
		t.Fatalf("归属答案 = %d 行, want 2——未决那一次也留下", rows)
	}
	if rows := requestRowCount(t, pool, command.Identity); rows != 1 {
		t.Fatalf("委托行 = %d, want 1", rows)
	}
}

// Covers: `AT-PS-113`——首次`准入暂停`，其间暂停已解除，原样重发仍答`已有结果`、带原归属结果与门禁原因，不重判、不建单。
func TestAReplayAfterAdmissionPausedAnswersTheOriginal(t *testing.T) {
	pool, db := replayDatabase(t)
	ownership := &scriptedOwnership{steps: []ownershipStep{paused, permit}}
	submission, _ := submissionAssembledWith(t, db, ownership)
	command := submissionCommand(t)

	first, err := submission.Handle(t.Context(), command)
	if err != nil || first.Outcome() != shipmentapp.OutcomeAdmissionPaused {
		t.Fatalf("首次 outcome = %v, err = %v, want ADMISSION_PAUSED", first.Outcome(), err)
	}
	replay, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重发：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.OutcomeExistingResult {
		t.Fatalf("重发 outcome = %v, want EXISTING_RESULT", got)
	}
	if prior, present := replay.PriorOutcome(); !present || prior != shipmentapp.OutcomeAdmissionPaused {
		t.Fatalf("原结果 = %v（%v），want ADMISSION_PAUSED", prior, present)
	}
	decision, carried := replay.OwnershipDecision()
	if !carried || decision.DecisionID().String() != "SYN-OWN-DEC-PAUSED-1" || decision.AdmissionControl() != domain.AdmissionControlPaused {
		t.Fatalf("重发带的归属决定 = %v / %v（%v），want 原来那份暂停决定", decision.DecisionID(), decision.AdmissionControl(), carried)
	}
	if suspension, present := decision.SuspensionReference(); !present || suspension.String() != "SYN-SUSPENSION-1" {
		t.Fatalf("原归属结果丢了暂停引用：%v（%v）", suspension, present)
	}
	reasons := replay.GateBlockReasons()
	if len(reasons) != 1 || reasons[0] != domain.FutureSubmissionAdmissionPaused {
		t.Fatalf("重发带的门禁原因 = %v, want [ADMISSION_PAUSED]", reasons)
	}
	if ownership.calls() != 1 {
		t.Fatalf("判归属 %d 次, want 1——暂停之后的重发不重判", ownership.calls())
	}
	if rows := requestRowCount(t, pool, command.Identity); rows != 0 {
		t.Fatalf("委托行 = %d, want 0", rows)
	}
}

// Covers: `AT-PS-003` 带原归属结果一半（UC-PS-001「已有结果」一行）——委托已建，重发答`已有结果`、指名原委托，并带建它所据的
// 那一次归属决定；不重判。
func TestAReplayOfABuiltRequestNamesItWithTheOwnershipItWasBuiltOn(t *testing.T) {
	_, db := replayDatabase(t)
	ownership := &scriptedOwnership{steps: []ownershipStep{permit}}
	submission, _ := submissionAssembledWith(t, db, ownership)
	command := submissionCommand(t)

	if _, err := submission.Handle(t.Context(), command); err != nil {
		t.Fatalf("首次提交：%v", err)
	}
	replay, err := submission.Handle(t.Context(), command)
	if err != nil || replay.Outcome() != shipmentapp.OutcomeExistingResult {
		t.Fatalf("重发 outcome = %v, err = %v, want EXISTING_RESULT", replay.Outcome(), err)
	}
	if named, present := replay.ShipmentRequestID(); !present || named != command.ShipmentRequestID {
		t.Fatalf("重发指名 %q（%v），want 原委托 %q", named, present, command.ShipmentRequestID)
	}
	if prior, present := replay.PriorOutcome(); !present || prior != shipmentapp.OutcomeSubmitted {
		t.Fatalf("原结果 = %v（%v），want SUBMITTED", prior, present)
	}
	decision, carried := replay.OwnershipDecision()
	if !carried || decision.DecisionID().String() != "SYN-OWN-DEC-1" || decision.Authority() != domain.ProductionAuthorityIDPParcel {
		t.Fatalf("重发带的归属决定 = %v（%v），want 建单所据的 SYN-OWN-DEC-1", decision.DecisionID(), carried)
	}
	if ownership.calls() != 1 {
		t.Fatalf("判归属 %d 次, want 1", ownership.calls())
	}
}

// Covers: `AT-PS-111` 并发一支——建单那一笔回滚之后，两路重发同时到达：先进守卫的那一路判归属、建单；后一路在守卫外等，进门
// 时委托已在，答`已有结果`指名同一份并带胜者建单所据的归属，自己一次归属都不判。脚本让后一路若真去判就会判成他方——守卫没排住
// 时，同一范围就会一边在本地建单、一边投给他方（ADR-0266 决定二）。用屏障而不靠时间窗：第一路在判归属里停住，直到库里看见
// 第二路在等同一把咨询锁，才放第一路落笔。
func TestTwoConcurrentReplaysAfterARolledBackBuildBuildOnceAndJudgeOnce(t *testing.T) {
	pool, db := replayDatabase(t)
	failing := &failingOnceHandoff{}
	entered := make(chan struct{})
	release := make(chan struct{})
	heldPermit := func(scope domain.AdmissionScope) (domain.ProductionOwnershipDecision, error) {
		close(entered)
		<-release
		return permit(scope)
	}
	ownership := &scriptedOwnership{steps: []ownershipStep{permit, heldPermit, other}}
	channel := &scriptedChannel{observe: []handoffObservationStep{confirmHandoff}}
	submission, _ := submissionAssembledThrough(t, db, ownership, failing.wrap, channel)
	command := submissionCommand(t)

	failing.armed = true
	if _, err := submission.Handle(t.Context(), command); !errors.Is(err, errInjectedBuildFailure) {
		t.Fatalf("首次提交 err = %v, want 建单事务的注入失败", err)
	}

	commands := [2]shipmentapp.SubmitShipmentRequestCommand{command, command}
	commands[0].ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "syn-request-replay-a")
	commands[0].DeclaredParcelIDs = []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "syn-parcel-replay-a")}
	commands[1].ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "syn-request-replay-b")
	commands[1].DeclaredParcelIDs = []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "syn-parcel-replay-b")}

	var results [2]shipmentapp.SubmitShipmentRequestResult
	var failures [2]error
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		results[0], failures[0] = submission.Handle(context.Background(), commands[0])
	}()
	// 第一路已在守卫里判归属，第二路才出发：它必须在守卫外等到第一路落笔。
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("第一路没有走到判归属")
	}
	go func() {
		defer wait.Done()
		results[1], failures[1] = submission.Handle(context.Background(), commands[1])
	}()
	waited := sawSecondReplayWaiting(t, pool)
	close(release)
	wait.Wait()
	if !waited {
		t.Fatal("第二路没有在第一路持锁期间等那把锁，并发前提不成立")
	}

	for index, err := range failures {
		if err != nil {
			t.Fatalf("第 %d 路重发：%v", index, err)
		}
	}
	if got := results[0].Outcome(); got != shipmentapp.OutcomeSubmitted {
		t.Fatalf("第一路 outcome = %v, want SUBMITTED", got)
	}
	if got := results[1].Outcome(); got != shipmentapp.OutcomeExistingResult {
		t.Fatalf("第二路 outcome = %v, want EXISTING_RESULT", got)
	}
	first, _ := results[0].ShipmentRequestID()
	second, present := results[1].ShipmentRequestID()
	if !present || first != second || first != commands[0].ShipmentRequestID {
		t.Fatalf("两路指名 %q / %q（%v），want 都是第一路建成的 %q", first, second, present, commands[0].ShipmentRequestID)
	}
	if decision, carried := results[1].OwnershipDecision(); !carried || decision.DecisionID().String() != "SYN-OWN-DEC-1" {
		t.Fatalf("第二路带的归属决定 = %v（%v），want 胜者建单所据的 SYN-OWN-DEC-1", decision.DecisionID(), carried)
	}
	if got := ownership.calls(); got != 2 {
		t.Fatalf("判归属 %d 次, want 2（首次一次、第一路续办一次）——第二路不该再判", got)
	}
	if got := channel.deliveryCount(); got != 0 {
		t.Fatalf("投给他方 %d 次, want 0", got)
	}
	if rows := requestRowCount(t, pool, command.Identity); rows != 1 {
		t.Fatalf("委托行 = %d, want 恰好 1", rows)
	}
	if got := submittedEnvelopeCount(t, pool); got != 1 {
		t.Fatalf("「委托已提交」信封 = %d 份, want 恰好 1", got)
	}
}

// Covers: `AT-PS-114`——首次判为他方、交接没确认（通道未配置），其间登记册翻成本产品放行。重发不重判归属，按记着的那次尝试重投：
// 仍未确认就仍答`生产归属未决`、不在本地建单；确认了才答`非本产品生产归属`；此后再重发答`已有结果`带原结果（ADR-0266 决定二，
// ADR-0128 决定二「同一份决定重复走到这里必须得到同一次尝试」）。
func TestAReplayAfterAnUnconfirmedHandoffRedeliversTheSameAttemptInsteadOfDeciding(t *testing.T) {
	pool, db := replayDatabase(t)
	ownership := &scriptedOwnership{steps: []ownershipStep{other, permit}}
	channel := &scriptedChannel{observe: []handoffObservationStep{unconfiguredHandoff, unconfiguredHandoff, confirmHandoff}}
	submission, _ := submissionAssembledThrough(t, db, ownership, nil, channel)
	command := submissionCommand(t)

	first, err := submission.Handle(t.Context(), command)
	if err != nil || first.Outcome() != shipmentapp.OutcomeOwnershipUnresolved {
		t.Fatalf("首次 outcome = %v, err = %v, want OWNERSHIP_UNRESOLVED", first.Outcome(), err)
	}
	stillPending, err := submission.Handle(t.Context(), command)
	if err != nil || stillPending.Outcome() != shipmentapp.OutcomeOwnershipUnresolved {
		t.Fatalf("第二次 outcome = %v, err = %v, want OWNERSHIP_UNRESOLVED——交接仍未确认，登记册翻了也不在本地建单", stillPending.Outcome(), err)
	}
	confirmed, err := submission.Handle(t.Context(), command)
	if err != nil || confirmed.Outcome() != shipmentapp.OutcomeOtherProductionAuthority {
		t.Fatalf("第三次 outcome = %v, err = %v, want OTHER_PRODUCTION_AUTHORITY", confirmed.Outcome(), err)
	}
	if decision, _ := confirmed.OwnershipDecision(); decision.DecisionID().String() != "SYN-OWN-DEC-OTHER-1" {
		t.Fatalf("确认的是 %v, want 首次那份他方决定", decision.DecisionID())
	}
	settled, err := submission.Handle(t.Context(), command)
	if err != nil || settled.Outcome() != shipmentapp.OutcomeExistingResult {
		t.Fatalf("第四次 outcome = %v, err = %v, want EXISTING_RESULT", settled.Outcome(), err)
	}
	if prior, _ := settled.PriorOutcome(); prior != shipmentapp.OutcomeOtherProductionAuthority {
		t.Fatalf("原结果 = %v, want OTHER_PRODUCTION_AUTHORITY", prior)
	}

	if got := ownership.calls(); got != 1 {
		t.Fatalf("判归属 %d 次, want 1——交接未确认的重发不重判", got)
	}
	attempts := channel.attempts()
	if len(attempts) != 3 || attempts[0] != attempts[1] || attempts[1] != attempts[2] {
		t.Fatalf("投递尝试 = %v, want 同一次尝试投三回", attempts)
	}
	if rows := requestRowCount(t, pool, command.Identity); rows != 0 {
		t.Fatalf("委托行 = %d, want 0", rows)
	}
	if rows := submissionAnswerCount(t, pool, command.Identity); rows != 3 {
		t.Fatalf("答案 = %d 行, want 3——每一次投递的评估都留下", rows)
	}
}

// sawSecondReplayWaiting 另开一条连接（测试池只有两条，两路续办各占一条），用本包的 waitForAdvisoryLockWaiter 看本库里有没有
// 一个事务在等咨询锁。不在这里 Fatal：调用方要先放行持锁那一路，否则用例吊死。
func sawSecondReplayWaiting(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	conn, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Errorf("另开观察连接：%v", err)
		return false
	}
	defer conn.Close(context.Background())
	return waitForAdvisoryLockWaiter(conn, 10*time.Second)
}

func replayDatabase(t *testing.T) (*pgxpool.Pool, *bentopg.DB) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return pool, db
}

// failingOnceHandoff 在建单那一笔事务里让「委托已提交」意图失败一次：WithinTransaction 见错即回滚，刚插下的委托行跟着消失。
// armed 只在测试主协程置位，消费时已在单路提交之内。
type failingOnceHandoff struct {
	inner ports.ShipmentRequestSubmittedHandoff
	armed bool
}

func (handoff *failingOnceHandoff) wrap(inner ports.ShipmentRequestSubmittedHandoff) ports.ShipmentRequestSubmittedHandoff {
	handoff.inner = inner
	return handoff
}

func (handoff *failingOnceHandoff) HandOffShipmentRequestSubmitted(
	ctx context.Context,
	intent ports.ShipmentRequestSubmittedHandoffIntent,
) error {
	if handoff.armed {
		handoff.armed = false
		return errInjectedBuildFailure
	}
	return handoff.inner.HandOffShipmentRequestSubmitted(ctx, intent)
}

var errOwnershipDependencyDown = errors.New("injected: ownership dependency down")

// ownershipStep 是归属替身一次作答的样子；脚本走完之后一直重复最后一步。
type ownershipStep func(scope domain.AdmissionScope) (domain.ProductionOwnershipDecision, error)

func permit(scope domain.AdmissionScope) (domain.ProductionOwnershipDecision, error) {
	return permittingOwnership{anchor: envelopeProofAnchor}.DecideProductionOwnership(context.Background(), scope)
}

func dependencyDown(domain.AdmissionScope) (domain.ProductionOwnershipDecision, error) {
	return domain.ProductionOwnershipDecision{}, errOwnershipDependencyDown
}

func paused(scope domain.AdmissionScope) (domain.ProductionOwnershipDecision, error) {
	return syntheticOwnershipDecision(scope, "SYN-OWN-DEC-PAUSED-1", func(spec *domain.ProductionOwnershipDecisionSpec) error {
		spec.Authority = domain.ProductionAuthorityIDPParcel
		spec.AdmissionControl = domain.AdmissionControlPaused
		suspension, err := domain.NewOwnershipSuspensionReference("SYN-SUSPENSION-1")
		spec.SuspensionRef = suspension
		return err
	})
}

func unresolved(scope domain.AdmissionScope) (domain.ProductionOwnershipDecision, error) {
	return syntheticOwnershipDecision(scope, "SYN-OWN-DEC-UNRESOLVED-1", func(spec *domain.ProductionOwnershipDecisionSpec) error {
		spec.Authority = domain.ProductionAuthorityUnresolved
		spec.AdmissionControl = domain.AdmissionControlOpen
		spec.UnresolvedReason = domain.OwnershipUnresolvedRuleUnavailable
		continuation, err := domain.NewOwnershipContinuationReference("SYN-CONT-OWN-1")
		spec.ContinuationRef = continuation
		return err
	})
}

// syntheticOwnershipDecision 与 permittingOwnership 同一组时刻、规则与修订，只换权威与准入控制那几格。
func syntheticOwnershipDecision(
	scope domain.AdmissionScope,
	decisionID string,
	fill func(*domain.ProductionOwnershipDecisionSpec) error,
) (domain.ProductionOwnershipDecision, error) {
	interval, err := domain.NewOwnershipValidityInterval(envelopeProofAnchor.Add(-time.Hour), envelopeProofAnchor.Add(24*time.Hour))
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	spec := domain.ProductionOwnershipDecisionSpec{
		Scope:      scope,
		AsOf:       envelopeProofAnchor.Add(-time.Minute),
		Validity:   interval,
		DecisionAt: envelopeProofAnchor.Add(-time.Minute),
	}
	if spec.DecisionID, err = domain.NewProductionOwnershipDecisionID(decisionID); err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	if spec.RuleVersion, err = domain.NewProductionOwnershipRuleVersion("SYN-OWN-RULE-1"); err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	if spec.Revision, err = domain.NewProductionOwnershipRevision("syn-rev-1"); err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	if err := fill(&spec); err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	return domain.NewProductionOwnershipDecision(spec)
}

func other(scope domain.AdmissionScope) (domain.ProductionOwnershipDecision, error) {
	return otherAuthorityOwnership{anchor: envelopeProofAnchor}.DecideProductionOwnership(context.Background(), scope)
}

// scriptedOwnership 按脚本逐次作答并计数；并发的几路共用它，计数与取步要上锁。
type scriptedOwnership struct {
	steps []ownershipStep

	mutex sync.Mutex
	count int
}

func (authority *scriptedOwnership) DecideProductionOwnership(
	_ context.Context,
	scope domain.AdmissionScope,
) (domain.ProductionOwnershipDecision, error) {
	authority.mutex.Lock()
	step := authority.steps[min(authority.count, len(authority.steps)-1)]
	authority.count++
	authority.mutex.Unlock()
	return step(scope)
}

func (authority *scriptedOwnership) calls() int {
	authority.mutex.Lock()
	defer authority.mutex.Unlock()
	return authority.count
}

// handoffObservationStep 是出向通道替身一次作答的样子；脚本走完之后一直重复最后一步。
type handoffObservationStep func(ports.ProductionHandoffDelivery) ports.ProductionHandoffObservation

func unconfiguredHandoff(ports.ProductionHandoffDelivery) ports.ProductionHandoffObservation {
	return ports.ProductionHandoffObservation{Observation: domain.HandoffObservationChannelUnconfigured}
}

func confirmHandoff(delivery ports.ProductionHandoffDelivery) ports.ProductionHandoffObservation {
	confirmation, _ := domain.NewHandoffConfirmationReference("SYN-HANDOFF-CONFIRMATION-1")
	query, _ := domain.NewHandoffQueryReference("SYN-HANDOFF-QUERY-1")
	return ports.ProductionHandoffObservation{
		Observation:          domain.HandoffObservationCompleteConfirmation,
		ConfirmedScopeDigest: delivery.Scope.Digest(),
		ConfirmationRef:      confirmation,
		QueryRef:             query,
		EffectiveAt:          envelopeProofAnchor.Add(-30 * time.Second),
	}
}

// scriptedChannel 记下每一次投递的尝试身份，按脚本作答。
type scriptedChannel struct {
	observe []handoffObservationStep

	mutex     sync.Mutex
	delivered []domain.HandoffAttemptID
}

func (channel *scriptedChannel) DeliverAdmissionScope(
	_ context.Context,
	delivery ports.ProductionHandoffDelivery,
) (ports.ProductionHandoffObservation, error) {
	channel.mutex.Lock()
	step := channel.observe[min(len(channel.delivered), len(channel.observe)-1)]
	channel.delivered = append(channel.delivered, delivery.AttemptID)
	channel.mutex.Unlock()
	return step(delivery), nil
}

func (channel *scriptedChannel) deliveryCount() int {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()
	return len(channel.delivered)
}

func (channel *scriptedChannel) attempts() []domain.HandoffAttemptID {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()
	return append([]domain.HandoffAttemptID(nil), channel.delivered...)
}

func requestRowCount(t *testing.T, pool *pgxpool.Pool, identity domain.SourceIdentity) int {
	t.Helper()
	return identityRowCount(t, pool, "parcel_shipment.shipment_request", identity)
}

func submissionAnswerCount(t *testing.T, pool *pgxpool.Pool, identity domain.SourceIdentity) int {
	t.Helper()
	return identityRowCount(t, pool, "parcel_shipment.submission_answer", identity)
}

func identityRowCount(t *testing.T, pool *pgxpool.Pool, table string, identity domain.SourceIdentity) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+table+`
		  WHERE tenant_id = $1 AND customer_account_id = $2 AND source = $3 AND source_request_key = $4`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 %s 行数：%v", table, err)
	}
	return count
}
