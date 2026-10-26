package postgres_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件在真实 PostgreSQL 上证同一包裹上越过取消边界的几处判断依次走（隔离合成 S）：取消、收寄采用、面单渠道终局判断三只
// 编排走真取消库、真采用库、真终局库、真冲突册与真边界锁，委托、授权、资格、终局规则、标识与发布意图几口用替身。屏障停在
// 先手那一路读完提交边界上的事实之后，此时它手里是「对方不在册」；排队时后手被锁挡在自己的边界读之前（看 pg_locks 认），
// 不排队时后手径直提交。两种先后各证一遍：后手读得到先手已提交的写入、落进 ADR-0254 决定二的某一格，取消与责任起点（或非
// 取消终局）不同时成立。

var (
	boundaryCancelledAt = time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	// 收寄与承运商首次有效收寄都发生在取消的业务时间之前：取消先提交时，后到的边界事实落第 3 格（事实冲突）。
	boundaryFactAt = boundaryCancelledAt.Add(-time.Hour)
)

// boundaryGate 只拦一次：arm 之后第一路读完边界事实的停在这里，等 resume 关上才往下走。
type boundaryGate struct {
	armed   atomic.Bool
	reached chan struct{}
	resume  chan struct{}
}

func newBoundaryGate() *boundaryGate {
	return &boundaryGate{reached: make(chan struct{}), resume: make(chan struct{})}
}

func (gate *boundaryGate) pass() {
	if gate.armed.CompareAndSwap(true, false) {
		close(gate.reached)
		<-gate.resume
	}
}

// boundaryRig 装三只编排。屏障只装在先手那一只的最后一道边界读上：装在哪一道由用例按先后挑，锁取在读之前还是之后由编排定，
// 屏障不替它定。
type boundaryRig struct {
	db            *bentopg.DB
	transactor    bentoapp.Transactor
	adoptions     *adapter.IntakeAdoptions
	cancellations *adapter.ParcelCancellations
	finals        *adapter.FinalOutcomes
	conflicts     *adapter.CancellationOrderConflicts
	locks         *adapter.CancellationBoundaryLocks
	requests      boundaryRequests
	gate          *boundaryGate
	watcher       *pgx.Conn
	identity      domain.SourceIdentity
	tenant        domain.TenantID
	parcel        domain.DeclaredParcelID
}

func newBoundaryRig(t *testing.T) *boundaryRig {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	rig := &boundaryRig{
		db:         db,
		transactor: db.Transactor(),
		gate:       newBoundaryGate(),
		identity:   requestIdentity(t, "boundary-key"),
		tenant:     mustBuild(t, domain.NewTenantID, "tenant-1"),
		parcel:     mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
	}
	if rig.adoptions, err = adapter.NewIntakeAdoptions(db); err != nil {
		t.Fatalf("构造采用库：%v", err)
	}
	if rig.cancellations, err = adapter.NewParcelCancellations(db); err != nil {
		t.Fatalf("构造取消库：%v", err)
	}
	if rig.finals, err = adapter.NewFinalOutcomes(db); err != nil {
		t.Fatalf("构造终局库：%v", err)
	}
	if rig.conflicts, err = adapter.NewCancellationOrderConflicts(db); err != nil {
		t.Fatalf("构造冲突册：%v", err)
	}
	if rig.locks, err = adapter.NewCancellationBoundaryLocks(db); err != nil {
		t.Fatalf("构造边界锁：%v", err)
	}
	accepted, err := submittedShipmentRequest(t, "boundary-key", "request-1").Decide(acceptanceDecisionSpec(t))
	if err != nil || accepted.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("形成已接受委托：state=%q err=%v", accepted.State(), err)
	}
	rig.requests = boundaryRequests{identity: rig.identity, request: accepted}

	// 看锁的连接单独开：两路各占池里一条，再从池里取会与停住的那一路互等。
	watcher, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开看锁连接：%v", err)
	}
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })
	rig.watcher = watcher
	return rig
}

// cancel 装取消编排；gated 为真时屏障装在它最后一道边界读（面单渠道服务的首次有效收寄）上。那一道读口是替身、答没有：
// 用例要争的是取消与本上下文自己写的责任起点或终局，不是 TF 的登记。
func (rig *boundaryRig) cancel(t *testing.T, gated bool, into *application.CancelParcelResult) func(context.Context) error {
	t.Helper()
	pickups := &boundaryPickups{}
	if gated {
		pickups.gate = rig.gate
	}
	handler := application.NewCancelParcelHandler(application.CancelParcelDeps{
		Requests:       rig.requests,
		Authority:      boundaryAuthority{basis: mustBuild(t, domain.NewCheckReason, "SYN-CANCEL-RULE/1")},
		Adoptions:      rig.adoptions,
		Finals:         rig.finals,
		Cancellations:  rig.cancellations,
		CarrierPickups: pickups,
		Store:          rig.cancellations,
		Identities:     boundaryCancellationIDs{id: mustBuild(t, domain.NewParcelCancellationID, "PCXL-1")},
		Downstream:     boundaryHandoffs{},
		Clock:          boundaryClock{},
		BoundaryLock:   rig.locks,
	})
	command := application.CancelParcelCommand{
		Identity:          rig.identity,
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "request-1"),
		Parcel:            rig.parcel,
		Requester:         mustBuild(t, domain.NewCancellationRequesterReference, "SYN-CUSTOMER-01"),
		Reason:            mustBuild(t, domain.NewCancellationReasonReference, "SYN-CHANGED-MIND"),
		RequestedAt:       boundaryCancelledAt,
	}
	return func(txCtx context.Context) error {
		result, err := handler.Handle(txCtx, command)
		*into = result
		return err
	}
}

// adopt 装收寄采用编排；gated 为真时屏障装在它最后一道边界读（责任起点）上。
func (rig *boundaryRig) adopt(t *testing.T, gated bool, into *application.AdoptNetworkIntakeResult) func(context.Context) error {
	t.Helper()
	var adoptions ports.IntakeAdoptionStore = rig.adoptions
	if gated {
		adoptions = gatedResponsibilityStarts{IntakeAdoptions: rig.adoptions, gate: rig.gate}
	}
	handler := application.NewAdoptNetworkIntakeHandler(application.AdoptNetworkIntakeDeps{
		Requests:      rig.requests,
		Eligibility:   boundaryEligibility{},
		Adoptions:     adoptions,
		Identities:    boundaryCommitmentIDs{id: mustBuild(t, domain.NewCommitmentVersionID, "CMT-1")},
		Downstream:    boundaryHandoffs{},
		Clock:         boundaryClock{},
		Cancellations: rig.cancellations,
		Conflicts:     rig.conflicts,
		BoundaryLock:  rig.locks,
	})
	command := application.AdoptNetworkIntakeCommand{
		Identity:          rig.identity,
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustBuild(t, domain.NewSubmissionVersionID, "version-1"),
		Source: domain.IntakeSourceSpec{
			Kind:       domain.NodeIntakeSource,
			Object:     mustBuild(t, domain.NewSourceObjectReference, "handling-unit-1"),
			Parcel:     rig.parcel,
			Place:      mustBuild(t, domain.NewIntakePlaceReference, "node-origin"),
			Control:    mustBuild(t, domain.NewIntakeControlReference, "NODE-CONTROL/NO-7"),
			Version:    mustBuild(t, domain.NewSourceResultVersion, "intake-result/v1"),
			OccurredAt: boundaryFactAt,
		},
	}
	return func(txCtx context.Context) error {
		result, err := handler.Handle(txCtx, command)
		*into = result
		return err
	}
}

// judge 装面单渠道终局判断，带着实际承运商首次有效收寄；gated 为真时屏障装在它唯一一道边界读（已成立的取消）上。终局采用
// 路径在同一笔事务里也读一遍取消（锁重入），那一口不包屏障：先手停在判断自己的读上。
func (rig *boundaryRig) judge(t *testing.T, gated bool, into *application.LabelServiceFinalResult) func(context.Context) error {
	t.Helper()
	var cancellations ports.ParcelCancellationView = rig.cancellations
	if gated {
		cancellations = gatedCancellations{ParcelCancellations: rig.cancellations, gate: rig.gate}
	}
	adoption := application.NewFormParcelFinalHandler(application.FormParcelFinalDeps{
		Requests: rig.requests,
		Rules: boundaryFinalRules{judgment: ports.FinalRuleJudgment{
			Satisfied:   true,
			Kind:        mustBuild(t, domain.NewFinalKindReference, "LABEL_SERVICE_FINAL"),
			RuleVersion: mustBuild(t, domain.NewFinalRuleVersionReference, "SYN-FINAL-RULE/1"),
		}},
		Finals:        rig.finals,
		Cancellations: rig.cancellations,
		Identities:    boundaryFinalIDs{id: mustBuild(t, domain.NewFinalOutcomeVersionID, "FINAL-1")},
		Downstream:    boundaryHandoffs{},
		Clock:         boundaryClock{},
		BoundaryLock:  rig.locks,
	})
	handler := application.NewJudgeLabelServiceFinalHandler(application.JudgeLabelServiceFinalDeps{
		Transactions:  boundaryLabelTransactions{},
		Registers:     boundaryRegisters{},
		Cancellations: cancellations,
		Conflicts:     rig.conflicts,
		Adoption:      adoption,
		Clock:         boundaryClock{},
		BoundaryLock:  rig.locks,
	})
	command := application.JudgeLabelServiceFinalCommand{
		Identity:          rig.identity,
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "request-1"),
		Parcel:            rig.parcel,
		FirstEffectivePickup: domain.CarrierFirstEffectivePickupSpec{
			Fact:        mustBuild(t, domain.NewCarrierFirstEffectivePickupFactReference, "TF-PICKUP-7"),
			Version:     mustBuild(t, domain.NewCarrierFirstEffectivePickupFactVersion, "TF-PICKUP-7/v1"),
			EffectiveAt: boundaryFactAt,
		},
	}
	return func(txCtx context.Context) error {
		result, err := handler.Handle(txCtx, command)
		*into = result
		return err
	}
}

// race 让 first 停在读完边界事实之后，再放 second 去跑；等到 second 要么已经提交、要么正等在咨询锁上，才放 first 走完。两路
// 各在自己的事务里，事务错误原样交回。每一步都有期限：卡住即失败，不挂死。
func (rig *boundaryRig) race(t *testing.T, first, second func(context.Context) error) (error, error) {
	t.Helper()
	ctx := t.Context()
	rig.gate.armed.Store(true)
	var once sync.Once
	resume := func() { once.Do(func() { close(rig.gate.resume) }) }
	defer resume()

	firstDone := make(chan error, 1)
	go func() { firstDone <- rig.transactor.WithinTransaction(ctx, first) }()
	select {
	case <-rig.gate.reached:
	case err := <-firstDone:
		t.Fatalf("先手那一路没读到边界事实就结束了：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("先手那一路十秒内没走到屏障")
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- rig.transactor.WithinTransaction(ctx, second) }()
	var secondErr error
	secondFinished := false
	deadline := time.Now().Add(10 * time.Second)
	for !secondFinished && !rig.waitingOnAdvisoryLock(t) {
		select {
		case secondErr = <-secondDone:
			secondFinished = true
		case <-time.After(20 * time.Millisecond):
		}
		if !secondFinished && time.Now().After(deadline) {
			t.Fatal("后手那一路十秒内既没提交、也没等在咨询锁上")
		}
	}
	resume()

	var firstErr error
	select {
	case firstErr = <-firstDone:
	case <-time.After(10 * time.Second):
		t.Fatal("放开屏障后先手那一路十秒内没走完")
	}
	if !secondFinished {
		select {
		case secondErr = <-secondDone:
		case <-time.After(10 * time.Second):
			t.Fatal("先手提交后后手那一路十秒内没走完")
		}
	}
	return firstErr, secondErr
}

// waitingOnAdvisoryLock 只看本测试库：pgtest 给每个用例一个物理库，别的用例的锁不该算进来。
func (rig *boundaryRig) waitingOnAdvisoryLock(t *testing.T) bool {
	t.Helper()
	var waiting int
	if err := rig.watcher.QueryRow(t.Context(),
		`SELECT count(*) FROM pg_locks
		  WHERE locktype = 'advisory' AND NOT granted
		    AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`,
	).Scan(&waiting); err != nil {
		t.Fatalf("看锁：%v", err)
	}
	return waiting > 0
}

// standing 读回落库后这件包裹上站着的三样：已成立的取消、责任起点、当前有效终局。
func (rig *boundaryRig) standing(t *testing.T) (cancelled, started, finalized bool) {
	t.Helper()
	ctx := t.Context()
	_, cancelled, err := rig.cancellations.FindCancellation(ctx, rig.tenant, rig.parcel)
	if err != nil {
		t.Fatalf("读回取消：%v", err)
	}
	_, started, err = rig.adoptions.FindResponsibilityStart(ctx, rig.tenant, rig.parcel)
	if err != nil {
		t.Fatalf("读回责任起点：%v", err)
	}
	final, found, err := rig.finals.FindCurrentFinal(ctx, rig.tenant, rig.parcel)
	if err != nil {
		t.Fatalf("读回当前有效终局：%v", err)
	}
	finalized = found && final.Finalized
	if cancelled && (started || finalized) {
		t.Fatalf("取消与责任起点（%t）或非取消终局（%t）同时成立——两路都按「对方不在册」提交了", started, finalized)
	}
	return cancelled, started, finalized
}

func (rig *boundaryRig) conflictsOnParcel(t *testing.T) []ports.CancellationOrderConflictEntry {
	t.Helper()
	entries, err := rig.conflicts.ListByParcel(t.Context(), rig.tenant, rig.parcel)
	if err != nil {
		t.Fatalf("按包裹查冲突：%v", err)
	}
	return entries
}

// Covers: PS CONTEXT「同一包裹的取消提交、收寄采用与面单渠道服务的终局判断依次越过提交边界」与 `AT-PS-107` 的并发半边——
// 取消先手，读完提交边界上的终局、采用链与首次有效收寄（都不在册）后停住；收寄采用后手到达，收寄发生早于取消的业务时间。
// 失败形态：采用径直读到「没有取消」、形成责任起点并提交，先手随后照它读到的提交取消，一件包裹同时带着取消终局与责任起点，
// 冲突那一格从未被触发。要的是后手读到先手已提交的取消、落第 3 格：采用停在 CANCELLATION_ORDER_CONFLICT，冲突册一行。
func TestACancellationAheadOfAnIntakeAdoptionIsSeenByTheAdoption(t *testing.T) {
	rig := newBoundaryRig(t)
	var cancelled application.CancelParcelResult
	var adopted application.AdoptNetworkIntakeResult
	firstErr, secondErr := rig.race(t, rig.cancel(t, true, &cancelled), rig.adopt(t, false, &adopted))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("取消的事务 → %v；采用的事务 → %v；两路都该提交", firstErr, secondErr)
	}
	if cancelled.Outcome() != application.ParcelCancelled {
		t.Fatalf("取消 = %s/%s，want PARCEL_CANCELLED", cancelled.Outcome(), cancelled.UndecidedReason())
	}
	if adopted.Outcome() != application.IntakeEligibilityUndecided ||
		adopted.UndecidedReason() != application.IntakeCancellationOrderConflict {
		t.Fatalf("采用 = %s/%s，want ELIGIBILITY_UNDECIDED/CANCELLATION_ORDER_CONFLICT——后手没读到先手的取消",
			adopted.Outcome(), adopted.UndecidedReason())
	}
	if isCancelled, started, _ := rig.standing(t); !isCancelled || started {
		t.Fatalf("落库：取消 %t、责任起点 %t，want 只有取消", isCancelled, started)
	}
	if entries := rig.conflictsOnParcel(t); len(entries) != 1 ||
		entries[0].Boundary.Kind != domain.NetworkIntakeBoundary {
		t.Fatalf("冲突册 = %+v，want 一行网络收寄冲突", entries)
	}
}

// Covers: PS CONTEXT 同上与 `AT-PS-079` 的并发半边——收寄采用先手，读完已成立的取消与责任起点（都不在册）后停住；取消后手
// 到达。失败形态：取消径直读到「没有责任起点」、提交取消，先手随后照它读到的形成责任起点。要的是后手读到先手已提交的责任
// 起点、落第 1 格：取消不成立，答待处置并记下越过的那一代收寄。
func TestAnIntakeAdoptionAheadOfACancellationTurnsItIntoDispositionPending(t *testing.T) {
	rig := newBoundaryRig(t)
	var adopted application.AdoptNetworkIntakeResult
	var cancelled application.CancelParcelResult
	firstErr, secondErr := rig.race(t, rig.adopt(t, true, &adopted), rig.cancel(t, false, &cancelled))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("采用的事务 → %v；取消的事务 → %v；两路都该提交", firstErr, secondErr)
	}
	if adopted.Outcome() != application.IntakeCommitmentFormed {
		t.Fatalf("采用 = %s/%s，want COMMITMENT_FORMED", adopted.Outcome(), adopted.UndecidedReason())
	}
	if cancelled.Outcome() != application.DispositionPending {
		t.Fatalf("取消 = %s/%s，want DISPOSITION_PENDING——后手没读到先手的责任起点",
			cancelled.Outcome(), cancelled.UndecidedReason())
	}
	record, _ := cancelled.Record()
	if record.IntakeVersion.String() != "intake-result/v1" {
		t.Fatalf("待处置记下的收寄版本 = %q，want 越过的那一代", record.IntakeVersion)
	}
	if isCancelled, started, _ := rig.standing(t); isCancelled || !started {
		t.Fatalf("落库：取消 %t、责任起点 %t，want 只有责任起点", isCancelled, started)
	}
}

// Covers: PS CONTEXT 同上与 `AT-PS-108` 的并发半边——取消先手，读完提交边界上的事实后停住；面单渠道终局判断后手带着实际
// 承运商首次有效收寄到达，收寄发生早于取消的业务时间。失败形态：判断径直读到「没有取消」、经采用路径形成非取消终局并提交，
// 先手随后照它读到的提交取消。要的是后手读到先手已提交的取消、落第 3 格：不形成终局、指名冲突，冲突册一行。
func TestACancellationAheadOfALabelServiceJudgmentIsSeenByTheJudgment(t *testing.T) {
	rig := newBoundaryRig(t)
	var cancelled application.CancelParcelResult
	var judged application.LabelServiceFinalResult
	firstErr, secondErr := rig.race(t, rig.cancel(t, true, &cancelled), rig.judge(t, false, &judged))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("取消的事务 → %v；判断的事务 → %v；两路都该提交", firstErr, secondErr)
	}
	if cancelled.Outcome() != application.ParcelCancelled {
		t.Fatalf("取消 = %s/%s，want PARCEL_CANCELLED", cancelled.Outcome(), cancelled.UndecidedReason())
	}
	if judged.Outcome() != application.LabelServiceNotFinalOutcome ||
		judged.Verdict().Reason() != domain.LabelServiceCancellationOrderConflict {
		t.Fatalf("判断 = %s/%s，want LABEL_SERVICE_NOT_FINAL/CANCELLATION_ORDER_CONFLICT——后手没读到先手的取消",
			judged.Outcome(), judged.Verdict().Reason())
	}
	if isCancelled, _, finalized := rig.standing(t); !isCancelled || finalized {
		t.Fatalf("落库：取消 %t、非取消终局 %t，want 只有取消", isCancelled, finalized)
	}
	if entries := rig.conflictsOnParcel(t); len(entries) != 1 ||
		entries[0].Boundary.Kind != domain.CarrierFirstEffectivePickupBoundary {
		t.Fatalf("冲突册 = %+v，want 一行承运商首次有效收寄冲突", entries)
	}
}

// Covers: PS CONTEXT 同上与 `AT-PS-086` 的并发半边——面单渠道终局判断先手，读完已成立的取消（不在册）后停住；取消后手到达。
// 失败形态：取消径直读到「没有终局」、提交取消，先手随后照它读到的形成非取消终局。要的是后手读到先手已提交的终局、落第 1 格：
// 答既有终局并指名那一份，不落取消记录。
func TestALabelServiceFinalAheadOfACancellationIsReturnedAsTheStandingFinal(t *testing.T) {
	rig := newBoundaryRig(t)
	var judged application.LabelServiceFinalResult
	var cancelled application.CancelParcelResult
	firstErr, secondErr := rig.race(t, rig.judge(t, true, &judged), rig.cancel(t, false, &cancelled))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("判断的事务 → %v；取消的事务 → %v；两路都该提交", firstErr, secondErr)
	}
	adoption, delegated := judged.Adoption()
	if judged.Outcome() != application.LabelServiceFinalAdopted || !delegated ||
		adoption.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("判断 = %s，采用 = %s，want 终局已形成", judged.Outcome(), adoption.Outcome())
	}
	if cancelled.Outcome() != application.CancellationFinalOutcomeStands ||
		cancelled.Basis().String() != "FINAL_OUTCOME/FINAL-1" {
		t.Fatalf("取消 = %s（依据 %q），want FINAL_OUTCOME_STANDS 指名 FINAL-1——后手没读到先手的终局",
			cancelled.Outcome(), cancelled.Basis())
	}
	if isCancelled, _, finalized := rig.standing(t); isCancelled || !finalized {
		t.Fatalf("落库：取消 %t、非取消终局 %t，want 只有终局", isCancelled, finalized)
	}
}

// Covers: UC-PS-006「重复请求返回原逐包裹结果」在排队之下仍成立——同一取消请求并发重发，先手读完提交边界上的事实后停住。排队
// 若只从终局读开始，后手越过幂等读时先手还没落记录，进锁后读到的却是先手已成立的取消，答成既有终局。要的是后手在幂等读之前
// 就排上队，读到先手落下的记录，答已有结果。
func TestAConcurrentReplayOfOneCancellationAnswersTheExistingResult(t *testing.T) {
	rig := newBoundaryRig(t)
	var first, second application.CancelParcelResult
	firstErr, secondErr := rig.race(t, rig.cancel(t, true, &first), rig.cancel(t, false, &second))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("两次取消的事务 → %v / %v；两路都该提交", firstErr, secondErr)
	}
	if first.Outcome() != application.ParcelCancelled {
		t.Fatalf("先手 = %s/%s，want PARCEL_CANCELLED", first.Outcome(), first.UndecidedReason())
	}
	if second.Outcome() != application.CancellationExistingResult {
		t.Fatalf("后手 = %s（依据 %q），want EXISTING_RESULT——重发没读到先手落下的记录", second.Outcome(), second.Basis())
	}
	if isCancelled, _, _ := rig.standing(t); !isCancelled {
		t.Fatal("落库后没有取消")
	}
}

// ---- 边界之外的几口替身：两路并发共用，都不记状态 ----

type boundaryRequests struct {
	identity domain.SourceIdentity
	request  domain.ShipmentRequest
}

func (double boundaryRequests) FindBySourceIdentity(_ context.Context, identity domain.SourceIdentity) (domain.ShipmentRequest, bool, error) {
	if identity != double.identity {
		return domain.ShipmentRequest{}, false, nil
	}
	return double.request, true, nil
}

func (boundaryRequests) Insert(context.Context, domain.SourceIdentity, domain.ShipmentRequest) (ports.ShipmentRequestInsertOutcome, error) {
	return ports.ShipmentRequestInsertOutcome(0), errors.New("本夹具的委托只读")
}

func (boundaryRequests) Save(context.Context, domain.SourceIdentity, domain.ShipmentRequest) (ports.ShipmentRequestSaveOutcome, error) {
	return ports.ShipmentRequestSaveOutcome(0), errors.New("本夹具的委托只读")
}

type boundaryAuthority struct{ basis domain.CheckReason }

func (double boundaryAuthority) JudgeCancellationAuthority(
	context.Context, domain.SourceIdentity, domain.CancellationRequesterReference, domain.DeclaredParcelID,
) (ports.CancellationAuthorityJudgment, bool, error) {
	return ports.CancellationAuthorityJudgment{Granted: true, Basis: double.basis}, true, nil
}

// boundaryPickups 答没有首次有效收寄；带屏障时读完停在那里。
type boundaryPickups struct{ gate *boundaryGate }

func (double *boundaryPickups) FindCurrentCarrierPickup(
	context.Context, domain.TenantID, domain.DeclaredParcelID,
) (ports.CarrierPickupBoundary, error) {
	if double.gate != nil {
		double.gate.pass()
	}
	return ports.CarrierPickupBoundary{State: ports.CarrierPickupBoundaryAbsent}, nil
}

type gatedResponsibilityStarts struct {
	*adapter.IntakeAdoptions
	gate *boundaryGate
}

func (store gatedResponsibilityStarts) FindResponsibilityStart(
	ctx context.Context, tenant domain.TenantID, parcel domain.DeclaredParcelID,
) (ports.IntakeAdoptionRecord, bool, error) {
	record, found, err := store.IntakeAdoptions.FindResponsibilityStart(ctx, tenant, parcel)
	store.gate.pass()
	return record, found, err
}

type gatedCancellations struct {
	*adapter.ParcelCancellations
	gate *boundaryGate
}

func (view gatedCancellations) FindCancellation(
	ctx context.Context, tenant domain.TenantID, parcel domain.DeclaredParcelID,
) (domain.ParcelCancellation, bool, error) {
	cancellation, found, err := view.ParcelCancellations.FindCancellation(ctx, tenant, parcel)
	view.gate.pass()
	return cancellation, found, err
}

type boundaryEligibility struct{}

func (boundaryEligibility) JudgeIntakeEligibility(
	context.Context, domain.SourceIdentity, domain.ShipmentRequestID, domain.IntakeSource,
) (ports.IntakeEligibility, bool, error) {
	return ports.IntakeEligibility{Outcome: ports.IntakeEligibilityEstablished}, true, nil
}

type boundaryFinalRules struct{ judgment ports.FinalRuleJudgment }

func (double boundaryFinalRules) JudgeFinalOutcome(
	context.Context, domain.SourceIdentity, domain.ResponsibilityOutcome,
) (ports.FinalRuleJudgment, bool, error) {
	return double.judgment, true, nil
}

type boundaryLabelTransactions struct{}

func (boundaryLabelTransactions) ListByCoveredParcel(context.Context, domain.TenantID, domain.DeclaredParcelID) ([]domain.LabelTransaction, error) {
	return nil, nil
}

type boundaryRegisters struct{}

func (boundaryRegisters) FindByParcel(context.Context, domain.TenantID, domain.DeclaredParcelID) (domain.ContinuedAttemptRegister, bool, error) {
	return domain.ContinuedAttemptRegister{}, false, nil
}

type boundaryCancellationIDs struct{ id domain.ParcelCancellationID }

func (double boundaryCancellationIDs) NextParcelCancellationID(context.Context) (domain.ParcelCancellationID, error) {
	return double.id, nil
}

type boundaryCommitmentIDs struct{ id domain.CommitmentVersionID }

func (double boundaryCommitmentIDs) NextCommitmentVersionID(context.Context) (domain.CommitmentVersionID, error) {
	return double.id, nil
}

type boundaryFinalIDs struct{ id domain.FinalOutcomeVersionID }

func (double boundaryFinalIDs) NextFinalOutcomeVersionID(context.Context) (domain.FinalOutcomeVersionID, error) {
	return double.id, nil
}

// boundaryHandoffs 收下三种发布意图、什么都不做：意图入队与本文件要证的先后无关。
type boundaryHandoffs struct{}

func (boundaryHandoffs) HandOffParcelCancellation(context.Context, ports.ParcelCancellationHandoffIntent) error {
	return nil
}

func (boundaryHandoffs) HandOffNetworkIntake(context.Context, ports.NetworkIntakeHandoffIntent) error {
	return nil
}

func (boundaryHandoffs) HandOffFinalOutcome(context.Context, ports.FinalOutcomeHandoffIntent) error {
	return nil
}

type boundaryClock struct{}

func (boundaryClock) Now() time.Time { return boundaryCancelledAt.Add(time.Hour) }
