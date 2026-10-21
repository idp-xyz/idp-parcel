package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件在真实 PostgreSQL 上证提交重发「接着办」（ADR-0256 决定一第 2 格；票 docs-consistency/code/39）。除归属读口换成按脚本
// 作答的替身（隔离合成 `S`，不进生产装配）、建单事务里按需注入一次失败之外，仓储、三个事务边界壳、Outbox 与标识工厂全是
// 生产件：要证的正是这几样之间的事务切分——保全与归属答案逐笔提交，建单那一笔回滚时它们留下来，重发据此接着建单。

var errInjectedBuildFailure = errors.New("injected: build transaction fails")

// Covers: `AT-PS-111`——建单那一笔事务真回滚（委托行与信封一起消失），原样重发接着建成恰好一份委托与一封信封。
func TestAReplayAfterTheBuildTransactionRolledBackBuildsTheRequestOnce(t *testing.T) {
	pool, db := replayDatabase(t)
	failing := &failingOnceHandoff{}
	submission, _ := submissionAssembledThrough(t, db, &scriptedOwnership{steps: []ownershipStep{permit}}, failing.wrap)
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

// Covers: `AT-PS-111`——判归属的依赖答不出（保全已落，归属答案一行都没留），原样重发重新判归属并照常建单。
func TestAReplayAfterTheOwnershipDependencyFailedDecidesAgain(t *testing.T) {
	pool, db := replayDatabase(t)
	ownership := &scriptedOwnership{steps: []ownershipStep{dependencyDown, permit}}
	submission, _ := submissionAssembledWith(t, db, ownership)
	command := submissionCommand(t)

	if _, err := submission.Handle(t.Context(), command); !errors.Is(err, errOwnershipDependencyDown) {
		t.Fatalf("首次提交 err = %v, want 归属依赖失败原样上抛", err)
	}
	if rows := ownershipAnswerCount(t, pool, command.Identity); rows != 0 {
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

// Covers: `AT-PS-112`——首次`生产归属未决`，原样重发重判：判得出就照常建单；两次判断都留在库里。
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
	if rows := ownershipAnswerCount(t, pool, command.Identity); rows != 2 {
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
	decision, carried := replay.OwnershipDecision()
	if !carried || decision.DecisionID().String() != "SYN-OWN-DEC-1" || decision.Authority() != domain.ProductionAuthorityIDPParcel {
		t.Fatalf("重发带的归属决定 = %v（%v），want 建单所据的 SYN-OWN-DEC-1", decision.DecisionID(), carried)
	}
	if ownership.calls() != 1 {
		t.Fatalf("判归属 %d 次, want 1", ownership.calls())
	}
}

// Covers: `AT-PS-111` 并发一支——建单那一笔回滚之后，两路重发同时续办，都判了归属、都去建单（接单入口给的委托号各不相同），
// 建单的唯一键让只有一份落下：恰好一份委托、一封信封；两路答复指名同一份，落败那一路答`已有结果`并带胜者建单所据的归属。
func TestTwoConcurrentReplaysAfterARolledBackBuildBuildOnce(t *testing.T) {
	pool, db := replayDatabase(t)
	failing := &failingOnceHandoff{}
	ownership := &scriptedOwnership{steps: []ownershipStep{permit}}
	submission, _ := submissionAssembledThrough(t, db, ownership, failing.wrap)
	command := submissionCommand(t)

	failing.armed = true
	if _, err := submission.Handle(t.Context(), command); !errors.Is(err, errInjectedBuildFailure) {
		t.Fatalf("首次提交 err = %v, want 建单事务的注入失败", err)
	}

	// 两路都要走到判归属才放行：确保两路都越过了「委托在不在」那一问，才是两次续办撞在建单上，而不是一前一后的重放。
	ownership.hold(2)
	commands := [2]shipmentapp.SubmitShipmentRequestCommand{command, command}
	commands[0].ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "syn-request-replay-a")
	commands[0].DeclaredParcelIDs = []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "syn-parcel-replay-a")}
	commands[1].ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "syn-request-replay-b")
	commands[1].DeclaredParcelIDs = []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "syn-parcel-replay-b")}

	var results [2]shipmentapp.SubmitShipmentRequestResult
	var failures [2]error
	var wait sync.WaitGroup
	for index := range commands {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index], failures[index] = submission.Handle(context.Background(), commands[index])
		}()
	}
	wait.Wait()

	for index, err := range failures {
		if err != nil {
			t.Fatalf("第 %d 路重发：%v", index, err)
		}
	}
	if ownership.timedOut() {
		t.Fatal("两路没有同时走到判归属，并发前提不成立")
	}
	if rows := requestRowCount(t, pool, command.Identity); rows != 1 {
		t.Fatalf("委托行 = %d, want 恰好 1", rows)
	}
	if got := submittedEnvelopeCount(t, pool); got != 1 {
		t.Fatalf("「委托已提交」信封 = %d 份, want 恰好 1", got)
	}
	submitted, existing := 0, 0
	var named [2]domain.ShipmentRequestID
	for index, result := range results {
		switch result.Outcome() {
		case shipmentapp.OutcomeSubmitted:
			submitted++
		case shipmentapp.OutcomeExistingResult:
			existing++
		default:
			t.Fatalf("第 %d 路 outcome = %v, want SUBMITTED 或 EXISTING_RESULT", index, result.Outcome())
		}
		id, present := result.ShipmentRequestID()
		if !present {
			t.Fatalf("第 %d 路没有指名委托", index)
		}
		named[index] = id
		if _, carried := result.OwnershipDecision(); !carried {
			t.Fatalf("第 %d 路没带归属决定", index)
		}
	}
	if submitted != 1 || existing != 1 || named[0] != named[1] {
		t.Fatalf("SUBMITTED %d 路、EXISTING_RESULT %d 路，指名 %v, want 各 1 且同一份", submitted, existing, named)
	}
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

// scriptedOwnership 按脚本逐次作答并计数；hold(n) 之后，前 n 次作答要等 n 路都到齐才放行（最多等一个固定时长，超时记下、
// 照样放行，免得用例吊死），用来让并发的几路同时越过「委托在不在」那一问。
type scriptedOwnership struct {
	steps []ownershipStep

	mutex    sync.Mutex
	count    int
	holding  int
	barrier  *sync.WaitGroup
	released chan struct{}
	late     bool
}

func (authority *scriptedOwnership) hold(parties int) {
	authority.mutex.Lock()
	defer authority.mutex.Unlock()
	authority.holding = parties
	authority.barrier = &sync.WaitGroup{}
	authority.barrier.Add(parties)
	authority.released = make(chan struct{})
	barrier, released := authority.barrier, authority.released
	go func() {
		barrier.Wait()
		close(released)
	}()
}

func (authority *scriptedOwnership) DecideProductionOwnership(
	_ context.Context,
	scope domain.AdmissionScope,
) (domain.ProductionOwnershipDecision, error) {
	authority.mutex.Lock()
	step := authority.steps[min(authority.count, len(authority.steps)-1)]
	authority.count++
	var barrier *sync.WaitGroup
	var released chan struct{}
	if authority.holding > 0 {
		authority.holding--
		barrier, released = authority.barrier, authority.released
	}
	authority.mutex.Unlock()

	if barrier != nil {
		barrier.Done()
		select {
		case <-released:
		case <-time.After(10 * time.Second):
			authority.mutex.Lock()
			authority.late = true
			authority.mutex.Unlock()
		}
	}
	return step(scope)
}

func (authority *scriptedOwnership) calls() int {
	authority.mutex.Lock()
	defer authority.mutex.Unlock()
	return authority.count
}

func (authority *scriptedOwnership) timedOut() bool {
	authority.mutex.Lock()
	defer authority.mutex.Unlock()
	return authority.late
}

func requestRowCount(t *testing.T, pool *pgxpool.Pool, identity domain.SourceIdentity) int {
	t.Helper()
	return identityRowCount(t, pool, "parcel_shipment.shipment_request", identity)
}

func ownershipAnswerCount(t *testing.T, pool *pgxpool.Pool, identity domain.SourceIdentity) int {
	t.Helper()
	return identityRowCount(t, pool, "parcel_shipment.submission_ownership_answer", identity)
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
