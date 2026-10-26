package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
)

// 本文件对真实 PostgreSQL 16 与真接受链证 ADR-0290：接受链读到委托`已提交`之后，撤回或主动拒绝抢先提交，这一轮占下的
// 冻结在形成决定那一步由链自己放掉，不成孤儿。夹具同 control_scope_lock_order_test.go：控制与释放走生产 PS→SA 适配器、
// SA 施加与释放编排与两本账，余额恰好只够冻一笔；屏障把链或终局路径停在指定的那一刻。
//
// 两个窗口：
//   - 窗口（b）：链已写下冻结（未提交）、形成决定那一步还没读委托时终局路径提交。首版委托没有已提交的占用，终局路径
//     不取作用域锁、释放什么也读不到。
//   - 窗口（a）：终局路径先拿到作用域锁并停在保存之前，链在锁上排队，放行后链施加了新的一笔。
// 两格都在撤回与主动拒绝上各证一遍。

// synTNeverFirstVersion 让可达性替身对每一版都答`可达`：首版链一路走到形成决定。
const synTNeverFirstVersion = "SYN-VER-NEVER-FIRST"

// synTGatedDecisionIdentities 在领决定标识处停住，等 resume 才往下走。屏障只拦一次，arm 之后的第一次。
type synTGatedDecisionIdentities struct {
	inner   psports.AcceptanceDecisionIdentity
	armed   atomic.Bool
	reached chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func newSynTGatedDecisionIdentities() *synTGatedDecisionIdentities {
	return &synTGatedDecisionIdentities{
		inner:   &synSDecisionIdentities{},
		reached: make(chan struct{}),
		resume:  make(chan struct{}),
	}
}

func (gate *synTGatedDecisionIdentities) NextAcceptanceDecisionID(ctx context.Context) (psdomain.AcceptanceDecisionID, error) {
	if gate.armed.CompareAndSwap(true, false) {
		close(gate.reached)
		<-gate.resume
	}
	return gate.inner.NextAcceptanceDecisionID(ctx)
}

func (gate *synTGatedDecisionIdentities) release() {
	gate.once.Do(func() { close(gate.resume) })
}

var _ psports.AcceptanceDecisionIdentity = (*synTGatedDecisionIdentities)(nil)

// synTGrantedRejection 恒答已授权：本文件证的是拒绝之后放了哪几笔，不是谁有拒绝权。
type synTGrantedRejection struct{ t *testing.T }

func (grant synTGrantedRejection) AuthorizeActiveRejection(
	_ context.Context,
	_ psports.ActiveRejectionAuthorizationQuery,
) (psports.ActiveRejectionAuthorization, error) {
	return psports.ActiveRejectionAuthorization{
		Outcome:   psports.AuthorizationGranted,
		Authority: mustPS(grant.t, psdomain.NewRejectionAuthorityReference, "SYN-REJECT-ROLE-01"),
	}, nil
}

var _ psports.ActiveRejectionAuthorizer = synTGrantedRejection{}

// rejectionWith 用生产的委托仓储、判断账与给定的释放口、决定标识口装一只主动拒绝编排；只有授权口是替身。
func (fixture *customerSupplementLoopFixture) rejectionWith(
	t *testing.T,
	release psports.PreAcceptanceControlRelease,
	identities psports.AcceptanceDecisionIdentity,
) *psapplication.RejectShipmentRequestHandler {
	t.Helper()
	refusals, err := pspostgres.NewControlReleaseRefusals(fixture.db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	scopeLocks, err := sapostgres.NewPreAcceptanceControlScopeLocks(fixture.db)
	if err != nil {
		t.Fatalf("构造控制作用域锁：%v", err)
	}
	return psapplication.NewRejectShipmentRequestHandler(psapplication.RejectShipmentRequestDeps{
		Requests:   fixture.requests,
		Authorizer: synTGrantedRejection{t: t},
		Judgments:  fixture.judgments,
		Recorder:   fixture.judgments,
		Release:    release,
		Refusals:   refusals,
		ScopeLock:  pssettlement.NewControlScopeLocks(scopeLocks),
		Identities: identities,
		Clock:      systemClock{},
	})
}

func (fixture *customerSupplementLoopFixture) rejectCommand(t *testing.T, version string) psapplication.RejectShipmentRequestCommand {
	t.Helper()
	return psapplication.RejectShipmentRequestCommand{
		Identity:          fixture.identity,
		ShipmentRequestID: fixture.requestID,
		SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, version),
		Decider:           mustPS(t, psdomain.NewDeciderReference, "SYN-OPERATOR-01"),
		Reason:            mustPS(t, psdomain.NewRejectionReasonReference, "SYN-OPERATOR-DECLINED"),
		Evidence:          mustPS(t, psdomain.NewRejectionEvidenceReference, "SYN-EVIDENCE-01"),
	}
}

type terminalPathEnd struct {
	formed bool
	err    error
}

// terminalPath 是抢先提交的终局路径：撤回，或主动拒绝。两者占着同一个决定边界，放占用的次序与锁序也相同。
type terminalPath struct {
	name  string
	state psdomain.ShipmentRequestState
	// handedOff 是链读到这份终结委托后发出的决定信封数：已拒绝带决定，形成决定那一步把同一份发布意图再交一次；已撤回
	// 没有接受或拒绝决定，不发。
	handedOff int
	start     func(race *terminalOrphanRace, version string, identities psports.AcceptanceDecisionIdentity) <-chan terminalPathEnd
}

var terminalPaths = []terminalPath{
	{name: "withdrawal", state: psdomain.ShipmentRequestWithdrawn, start: (*terminalOrphanRace).startWithdrawal},
	{name: "rejection", state: psdomain.ShipmentRequestRejected, handedOff: 1, start: (*terminalOrphanRace).startRejection},
}

// terminalOrphanRace 是一份带真账本的委托，加上链与终局路径各自的释放口、控制屏障与看锁用的池外连接。
type terminalOrphanRace struct {
	t            *testing.T
	ctx          context.Context
	fixture      *customerSupplementLoopFixture
	gate         *synRGatedControl
	chainRelease *synRFlakyRelease
	pathRelease  *synRFlakyRelease
	watcher      *pgx.Conn
	consume      func(context.Context) error
	chainDone    chan error
	gateOnce     sync.Once
}

func newTerminalOrphanRace(t *testing.T, formIdentities psports.AcceptanceDecisionIdentity) *terminalOrphanRace {
	t.Helper()
	ctx := t.Context()
	gate := newSynRGatedControl()
	chainRelease := &synRFlakyRelease{}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{
		control: gate, release: chainRelease, identities: formIdentities,
	})
	adapter := fixture.wireSettlement(t, ctx)
	gate.inner, chainRelease.inner = adapter, adapter

	// 看锁的连接单独开：几路各占池里一条，再从池里取会与停住的那一路互等。
	watcher, err := pgx.ConnectConfig(ctx, fixture.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开看锁连接：%v", err)
	}
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })

	race := &terminalOrphanRace{
		t: t, ctx: ctx, fixture: fixture, gate: gate,
		chainRelease: chainRelease,
		pathRelease:  &synRFlakyRelease{inner: adapter},
		watcher:      watcher,
		chainDone:    make(chan error, 1),
	}
	t.Cleanup(race.openGate)
	return race
}

func (race *terminalOrphanRace) openGate() {
	race.gateOnce.Do(func() { close(race.gate.resume) })
}

func (race *terminalOrphanRace) firstVersionControlRequest() string {
	return race.fixture.requestID.String() + "/SYN-VER-01"
}

// holdFirstVersionChainAfterTheFreeze 提交首版（可达），驱提交门，让链停在控制施加之后：冻结已写在账本上、链事务未提交，
// 控制结果与冻结对别的事务都不可见。
func (race *terminalOrphanRace) holdFirstVersionChainAfterTheFreeze() {
	t, ctx, fixture := race.t, race.ctx, race.fixture
	t.Helper()
	fixture.reachability.firstVersion = mustPS(t, psdomain.NewSubmissionVersionID, synTNeverFirstVersion)
	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})
	delivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	race.consume = func(consumeCtx context.Context) error {
		return fixture.submittedGate.Consume(consumeCtx, delivery.Envelope)
	}
	race.gate.armed.Store(true)
	race.startChain(race.consume)
	select {
	case <-race.gate.reached:
	case <-time.After(scopeLockRaceDeadline):
		t.Fatal("链没在期限内走到控制屏障")
	}
}

func (race *terminalOrphanRace) startChain(consume func(context.Context) error) {
	go func() { race.chainDone <- consume(race.ctx) }()
}

// finishChain 放行控制屏障，等链事务结束，交回消费门的结果。
func (race *terminalOrphanRace) finishChain() error {
	race.t.Helper()
	race.openGate()
	return race.awaitChain()
}

func (race *terminalOrphanRace) awaitChain() error {
	race.t.Helper()
	select {
	case err := <-race.chainDone:
		return err
	case <-time.After(2 * scopeLockRaceDeadline):
		race.t.Fatal("链没在期限内结束")
		return nil
	}
}

func (race *terminalOrphanRace) startWithdrawal(version string, identities psports.AcceptanceDecisionIdentity) <-chan terminalPathEnd {
	handler := race.fixture.withdrawalWith(race.t, race.pathRelease, identities)
	command := race.fixture.withdrawCommand(race.t, version)
	done := make(chan terminalPathEnd, 1)
	go func() {
		var result psapplication.WithdrawShipmentRequestResult
		err := race.fixture.transactor.WithinTransaction(race.ctx, func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = handler.Handle(txCtx, command)
			return handleErr
		})
		done <- terminalPathEnd{formed: result.Outcome() == psapplication.WithdrawalFormed, err: err}
	}()
	return done
}

func (race *terminalOrphanRace) startRejection(version string, identities psports.AcceptanceDecisionIdentity) <-chan terminalPathEnd {
	handler := race.fixture.rejectionWith(race.t, race.pathRelease, identities)
	command := race.fixture.rejectCommand(race.t, version)
	done := make(chan terminalPathEnd, 1)
	go func() {
		var result psapplication.RejectShipmentRequestResult
		err := race.fixture.transactor.WithinTransaction(race.ctx, func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = handler.Handle(txCtx, command)
			return handleErr
		})
		done <- terminalPathEnd{formed: result.Outcome() == psapplication.ActiveRejectionFormed, err: err}
	}()
	return done
}

func (race *terminalOrphanRace) awaitPath(done <-chan terminalPathEnd) terminalPathEnd {
	race.t.Helper()
	select {
	case end := <-done:
		if end.err != nil || !end.formed {
			race.t.Fatalf("终局路径没形成：formed = %v err = %v", end.formed, end.err)
		}
		return end
	case <-time.After(2 * scopeLockRaceDeadline):
		race.t.Fatal("终局路径没在期限内结束")
		return terminalPathEnd{}
	}
}

// awaitAdvisoryWaiter 等池外连接在 pg_locks 里看到有后端等在咨询锁（控制作用域锁）上。
func (race *terminalOrphanRace) awaitAdvisoryWaiter() {
	race.t.Helper()
	for deadline := time.Now().Add(scopeLockRaceDeadline); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		var count int
		if err := race.watcher.QueryRow(race.ctx,
			`SELECT count(*) FROM pg_locks
		  WHERE locktype = 'advisory' AND NOT granted
		    AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&count); err != nil {
			race.t.Fatalf("看咨询锁：%v", err)
		}
		if count > 0 {
			return
		}
	}
	race.t.Fatal("链没有等在作用域锁上")
}

func (race *terminalOrphanRace) assertStored(state psdomain.ShipmentRequestState) {
	race.t.Helper()
	if stored := race.fixture.mustLoadRequest(race.t, race.ctx); stored.State() != state {
		race.t.Fatalf("库里状态 = %q, want %q", stored.State(), state)
	}
}

func (race *terminalOrphanRace) assertReleased(controlRequest string) {
	race.t.Helper()
	if got := race.fixture.freeze(race.t, race.ctx, controlRequest); !got.present || got.status != 2 || got.releasedAt.IsZero() {
		race.t.Fatalf("冻结 %s = %+v, want 已释放——这一轮在已终结的委托上占下的资金成了孤儿", controlRequest, got)
	}
}

func (race *terminalOrphanRace) inboxRows() int {
	race.t.Helper()
	return race.fixture.countSQL(race.t, `SELECT count(*) FROM `+migrate.SchemaBento+`.inbox`)
}

// Covers: AT-PS-153；ADR-0290 决定一；窗口（b）——链已写下冻结（未提交）、形成决定那一步还没读委托时撤回或主动拒绝提交。
// 首版委托没有已提交的占用，终局路径不取作用域锁、释放读不到链未提交的冻结，什么也没放；链读到已终结的委托，在自己的
// 事务里按原关联放掉这一笔，提交之后冻结已释放。
func TestATerminalDecisionCommittedBeforeTheChainReadsTheRequestReleasesTheChainsFreshFreeze(t *testing.T) {
	for _, path := range terminalPaths {
		t.Run(path.name, func(t *testing.T) {
			race := newTerminalOrphanRace(t, nil)
			race.holdFirstVersionChainAfterTheFreeze()
			first := race.firstVersionControlRequest()

			race.awaitPath(path.start(race, "SYN-VER-01", &synSDecisionIdentities{}))
			if race.pathRelease.released != 0 {
				t.Fatalf("终局路径放掉了 %d 笔, want 0——链未提交的冻结它读不到", race.pathRelease.released)
			}
			if got := race.fixture.freeze(t, race.ctx, first); got.present {
				t.Fatalf("冻结 = %+v, want 链提交之前别的事务看不到", got)
			}

			if err := race.finishChain(); err != nil {
				t.Fatalf("链：%v", err)
			}
			race.assertStored(path.state)
			race.assertReleased(first)
			if race.chainRelease.released != 1 {
				t.Fatalf("链放掉了 %d 笔, want 1", race.chainRelease.released)
			}
			if n := race.inboxRows(); n != 1 {
				t.Fatalf("inbox 行数 = %d, want 1——这一份投递按处理完毕入账", n)
			}
			if n := race.fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != path.handedOff {
				t.Fatalf("决定信封 = %d 封, want %d", n, path.handedOff)
			}
		})
	}
}

// Covers: AT-PS-153；ADR-0290 决定一；窗口（a）——终局路径先拿到作用域锁并停在保存之前，续办的接受链在锁上排队，放行后
// 链排到锁、照常施加新版本的控制，形成决定那一步读到委托已终结，把这一笔放掉。
func TestAChainQueuedOnTheScopeLockPastATerminalDecisionReleasesItsFreshFreeze(t *testing.T) {
	for _, path := range terminalPaths {
		t.Run(path.name, func(t *testing.T) {
			race := newTerminalOrphanRace(t, nil)
			fixture, ctx := race.fixture, race.ctx
			firstVersion := fixture.requestID.String() + "/SYN-VER-01"
			newVersion := fixture.requestID.String() + "/" + synRSupplementVersionID

			// 首版占下后暂停（暂停那一轮已放掉它），客户补件形成新版本；续办信封在手，链还没驱。
			paused := fixture.pauseFirstVersion(t, ctx)
			supplemented := fixture.supplementOn(t, ctx, paused.CurrentSubmissionVersion().VersionID())
			if supplemented.Outcome() != psapplication.SupplementRecorded {
				t.Fatalf("受控补充 outcome = %q, want SUPPLEMENT_RECORDED（未决原因 %q）", supplemented.Outcome(), supplemented.PendingReason())
			}
			resume := fixture.claimDeliveryOfType(t, string(psinbox.SubmissionVersionFormedEventType)).Envelope

			// 终局路径取到作用域锁，停在领决定标识处（保存之前）。
			identities := newSynTGatedDecisionIdentities()
			identities.armed.Store(true)
			t.Cleanup(identities.release)
			pathDone := path.start(race, synRSupplementVersionID, identities)
			select {
			case <-identities.reached:
			case <-time.After(scopeLockRaceDeadline):
				t.Fatal("终局路径没在期限内走到决定标识屏障")
			}

			// 续办的链读到委托`已提交`，在放被取代版本的那一步排在锁上。
			race.startChain(func(consumeCtx context.Context) error { return fixture.supplementGate.Consume(consumeCtx, resume) })
			race.awaitAdvisoryWaiter()

			identities.release()
			race.awaitPath(pathDone)
			if err := race.awaitChain(); err != nil {
				t.Fatalf("链：%v", err)
			}

			race.assertStored(path.state)
			race.assertReleased(firstVersion)
			race.assertReleased(newVersion)
			if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.funds_freeze`); n != 2 {
				t.Fatalf("冻结行数 = %d, want 2（两版各一笔）", n)
			}
			if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != path.handedOff {
				t.Fatalf("决定信封 = %d 封, want %d", n, path.handedOff)
			}
		})
	}
}

// Covers: AT-PS-153；ADR-0290 决定一「已接受绝不放」——已接受委托的链被再驱一拍：形成决定那一步读到既有的接受，原样交回，
// 那笔合法冻结一分不放。
func TestAnAcceptedRequestsChainRedrivenNeverReleasesItsFreeze(t *testing.T) {
	race := newTerminalOrphanRace(t, nil)
	fixture, ctx := race.fixture, race.ctx
	first := race.firstVersionControlRequest()
	fixture.reachability.firstVersion = mustPS(t, psdomain.NewSubmissionVersionID, synTNeverFirstVersion)

	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})
	delivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, delivery.Envelope); err != nil {
		t.Fatalf("链：%v", err)
	}
	race.assertStored(psdomain.ShipmentRequestAccepted)
	if got := fixture.freeze(t, ctx, first); !got.present || got.status != 1 {
		t.Fatalf("冻结 = %+v, want 接受占下的已冻结", got)
	}

	// 绕过 inbox 再驱一拍（另一封信封驱到同一份委托时就是这样）。
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, err := fixture.chain.Handle(txCtx, psapplication.AdvanceAcceptanceChainCommand{
			Identity:          fixture.identity,
			ShipmentRequestID: fixture.requestID,
			SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, "SYN-VER-01"),
			DeclaredParcelIDs: []psdomain.DeclaredParcelID{mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01")},
		})
		return err
	})
	if race.chainRelease.released != 0 {
		t.Fatalf("链放掉了 %d 笔, want 0——已接受的委托绝不放", race.chainRelease.released)
	}
	if got := fixture.freeze(t, ctx, first); got.status != 1 {
		t.Fatalf("冻结 = %+v, want 仍为已冻结", got)
	}
	race.assertStored(psdomain.ShipmentRequestAccepted)
}

// Covers: AT-PS-154；ADR-0290 决定二——放不收口（可重试未形成）不往下交`已决定`：整轮回滚，链刚占下的冻结随之撤回，
// 消费门不入账；重投时已撤回的委托被链开头的守卫挡住，已拒绝的委托重复这一轮、这回放掉。
func TestAChainThatCannotReleaseItsFreshFreezeRollsTheRoundBack(t *testing.T) {
	for _, path := range terminalPaths {
		t.Run(path.name, func(t *testing.T) {
			race := newTerminalOrphanRace(t, nil)
			race.holdFirstVersionChainAfterTheFreeze()
			first := race.firstVersionControlRequest()
			race.chainRelease.failuresLeft = 1

			race.awaitPath(path.start(race, "SYN-VER-01", &synSDecisionIdentities{}))
			err := race.finishChain()
			if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) || !strings.Contains(err.Error(), "CONTROL_RELEASE_PENDING") {
				t.Fatalf("链 = %v, want 未决哨兵与 CONTROL_RELEASE_PENDING——放不掉还提交这一轮，刚占下的冻结就成了孤儿", err)
			}
			if got := race.fixture.freeze(t, race.ctx, first); got.present {
				t.Fatalf("冻结 = %+v, want 随这一轮回滚而没有", got)
			}
			if n := race.inboxRows(); n != 0 {
				t.Fatalf("inbox 行数 = %d, want 0——未决那一拍不该入账", n)
			}
			race.assertStored(path.state)

			// 重投。
			if err := race.consume(race.ctx); err != nil {
				t.Fatalf("重投：%v", err)
			}
			if n := race.inboxRows(); n != 1 {
				t.Fatalf("重投之后 inbox 行数 = %d, want 1", n)
			}
			switch path.state {
			case psdomain.ShipmentRequestWithdrawn:
				if got := race.fixture.freeze(t, race.ctx, first); got.present {
					t.Fatalf("重投之后冻结 = %+v, want 没有——已撤回的委托被链开头的守卫挡住，不再占资金", got)
				}
			default:
				race.assertReleased(first)
			}
		})
	}
}

// Covers: AT-PS-155；ADR-0290 决定二、ADR-0282 决定二——提供方确定不受理：那一笔记进释放未受理册待人处理，这一轮照常提交，
// 不回滚、不留补偿续办引用；终态委托上重投换不来另一个答案。
func TestAChainWhoseReleaseOfItsFreshFreezeIsRefusedRegistersItAndCommits(t *testing.T) {
	for _, path := range terminalPaths {
		t.Run(path.name, func(t *testing.T) {
			race := newTerminalOrphanRace(t, nil)
			race.holdFirstVersionChainAfterTheFreeze()
			first := race.firstVersionControlRequest()
			race.chainRelease.refusalsLeft = 1

			race.awaitPath(path.start(race, "SYN-VER-01", &synSDecisionIdentities{}))
			if err := race.finishChain(); err != nil {
				t.Fatalf("链：%v——确定不受理不该回滚也不该上抛", err)
			}
			race.assertStored(path.state)
			if n := race.inboxRows(); n != 1 {
				t.Fatalf("inbox 行数 = %d, want 1——这一份投递按处理完毕入账", n)
			}
			refusals, err := pspostgres.NewControlReleaseRefusals(race.fixture.db)
			if err != nil {
				t.Fatalf("构造释放未受理册：%v", err)
			}
			listed, err := refusals.ListByShipmentRequest(race.ctx, race.fixture.identity.TenantID(), race.fixture.requestID)
			if err != nil {
				t.Fatalf("读释放未受理册：%v", err)
			}
			if len(listed) != 1 || listed[0].SubmissionVersion.String() != "SYN-VER-01" ||
				listed[0].ControlResultID.String() != first || !listed[0].Scope.Recorded() {
				t.Fatalf("册上 = %+v, want 首版这一笔、作用域在册", listed)
			}
			if got := race.fixture.freeze(t, race.ctx, first); !got.present || got.status != 1 {
				t.Fatalf("冻结 = %+v, want 原样占着、等人处理", got)
			}
		})
	}
}

// Covers: AT-PS-154；ADR-0290 决定二——确定不受理却记不下册（册在库里写不进）：那一笔既没放掉、也没交到人手里，提交这一轮
// 就是留下一笔谁也不知道的占用，所以整轮回滚。
func TestAChainWhoseRefusalCannotBeRegisteredRollsTheRoundBack(t *testing.T) {
	race := newTerminalOrphanRace(t, nil)
	race.fixture.failRefusalWrites(t, race.ctx)
	race.holdFirstVersionChainAfterTheFreeze()
	first := race.firstVersionControlRequest()
	race.chainRelease.refusalsLeft = 1

	race.awaitPath(terminalPaths[0].start(race, "SYN-VER-01", &synSDecisionIdentities{}))
	err := race.finishChain()
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) || !strings.Contains(err.Error(), "CONTROL_RELEASE_REFUSAL_NOT_RECORDED") {
		t.Fatalf("链 = %v, want 未决哨兵与 CONTROL_RELEASE_REFUSAL_NOT_RECORDED", err)
	}
	if got := race.fixture.freeze(t, race.ctx, first); got.present {
		t.Fatalf("冻结 = %+v, want 随这一轮回滚而没有", got)
	}
	if n := race.fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.control_release_refusal`); n != 0 {
		t.Fatalf("册上 %d 笔, want 0", n)
	}
	if n := race.inboxRows(); n != 0 {
		t.Fatalf("inbox 行数 = %d, want 0", n)
	}
}

// Covers: AT-PS-153；ADR-0290 决定三后半——终局路径在形成决定读委托之后才提交：链形成接受，保存撞上终局路径落下的修订，
// 整轮回滚，链刚占下的冻结随之撤回，不会有孤儿；重投时已撤回的委托被守卫挡住，已拒绝的委托重复这一轮、这回放掉。
func TestATerminalDecisionCommittedAfterTheChainReadTheRequestRollsTheChainBack(t *testing.T) {
	for _, path := range terminalPaths {
		t.Run(path.name, func(t *testing.T) {
			identities := newSynTGatedDecisionIdentities()
			race := newTerminalOrphanRace(t, identities)
			t.Cleanup(identities.release)
			fixture, ctx := race.fixture, race.ctx
			first := race.firstVersionControlRequest()
			fixture.reachability.firstVersion = mustPS(t, psdomain.NewSubmissionVersionID, synTNeverFirstVersion)

			fixture.submit(t, ctx)
			request := fixture.mustLoadRequest(t, ctx)
			mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
				return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
					psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
			})
			delivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
			race.consume = func(consumeCtx context.Context) error {
				return fixture.submittedGate.Consume(consumeCtx, delivery.Envelope)
			}
			identities.armed.Store(true)
			race.startChain(race.consume)
			select {
			case <-identities.reached:
			case <-time.After(scopeLockRaceDeadline):
				t.Fatal("链没在期限内走到决定标识屏障")
			}

			race.awaitPath(path.start(race, "SYN-VER-01", &synSDecisionIdentities{}))
			identities.release()
			err := race.awaitChain()
			if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) || !strings.Contains(err.Error(), "STALE_SHIPMENT_REQUEST_REVISION") {
				t.Fatalf("链 = %v, want 未决哨兵与 STALE_SHIPMENT_REQUEST_REVISION", err)
			}
			if got := fixture.freeze(t, ctx, first); got.present {
				t.Fatalf("冻结 = %+v, want 随这一轮回滚而没有", got)
			}
			race.assertStored(path.state)

			if err := race.consume(ctx); err != nil {
				t.Fatalf("重投：%v", err)
			}
			switch path.state {
			case psdomain.ShipmentRequestWithdrawn:
				if got := fixture.freeze(t, ctx, first); got.present {
					t.Fatalf("重投之后冻结 = %+v, want 没有", got)
				}
			default:
				race.assertReleased(first)
			}
		})
	}
}
