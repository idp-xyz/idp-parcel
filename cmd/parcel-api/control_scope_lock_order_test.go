package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 证撤回、主动拒绝、授权处置与接受链对同一份委托取两把锁的次序一致（票 docs-consistency/code/99）：
// 控制作用域咨询锁 S（施加与释放都取）和委托行 R（保存委托的 UPDATE）。接受链先走财务控制取 S、再保存委托取 R；这三条
// 路径若先保存委托、再在释放里取 S，同一份委托并发时两边互等，PostgreSQL 的死锁检测中止一笔。
//
// 接受链一侧用一笔占着 S、停住、再保存委托行的事务代替：它的取锁形状就是链续办事务在 freezeFunds 取到 S 之后、形成决定
// 保存委托之前的样子，停点由用例定。真链与真账本的同一场景在 cmd/parcel-dispatch 的 control_scope_lock_order_test.go。
// 被测的三只编排走生产装配（assembleWithdrawalOrchestration、rejectionOrchestrationWith、dispositionOrchestrationWith），
// 只有授权口是合成替身。用例用池外连接轮询 pg_locks 认「等在 S 上」，都带期限，免得次序倒挂时死锁挂死。

const scopeLockRaceDeadline = 20 * time.Second

// scopeLockRace 是一份已存有占用作用域认领、冻结在账的委托，加上看锁用的池外连接。
type scopeLockRace struct {
	t          *testing.T
	db         *bentopg.DB
	watcher    *pgx.Conn
	identity   domain.SourceIdentity
	requestID  domain.ShipmentRequestID
	version    domain.SubmissionVersionID
	resultID   domain.FinancialControlResultID
	tenant     sadomain.TenantID
	scope      sadomain.SettlementScope
	requests   *pspostgres.ShipmentRequests
	scopeLocks *sapostgres.PreAcceptanceControlScopeLocks
}

func newScopeLockRace(t *testing.T, prepare func(*testing.T, *bentopg.DB) domain.SubmissionVersionID) *scopeLockRace {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	version := prepare(t, db)
	recorded, settlement := refusalSettlementScope(t)
	resultID := recordControlOnRealAssembly(t, db, version, recorded)
	freezeOnRealAssembly(t, db, resultID, settlement)

	command := submissionCommand(t)
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	scopeLocks, err := sapostgres.NewPreAcceptanceControlScopeLocks(db)
	if err != nil {
		t.Fatalf("构造控制作用域锁：%v", err)
	}
	// 看锁的连接单独开：两路各占池里一条，再从池里取会与停住的那一路互等。
	watcher, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开看锁连接：%v", err)
	}
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })
	return &scopeLockRace{
		t:          t,
		db:         db,
		watcher:    watcher,
		identity:   command.Identity,
		requestID:  command.ShipmentRequestID,
		version:    version,
		resultID:   resultID,
		tenant:     mustValue(t, sadomain.NewTenantID, command.Identity.TenantID().String()),
		scope:      settlement,
		requests:   requests,
		scopeLocks: scopeLocks,
	}
}

// chainHolder 是一笔接受链续办事务的取锁形状：取到 S，停在 reached，等 resume 之后再保存委托行并提交。
type chainHolder struct {
	reached chan struct{}
	resume  chan struct{}
	done    chan error
	once    sync.Once
}

func (race *scopeLockRace) startChainHolder() *chainHolder {
	holder := &chainHolder{
		reached: make(chan struct{}),
		resume:  make(chan struct{}),
		done:    make(chan error, 1),
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*scopeLockRaceDeadline)
		defer cancel()
		holder.done <- race.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			if err := race.scopeLocks.LockControlScope(txCtx, race.tenant, race.scope); err != nil {
				return err
			}
			close(holder.reached)
			<-holder.resume
			request, found, err := race.requests.FindBySourceIdentity(txCtx, race.identity)
			if err != nil || !found {
				return errors.Join(errors.New("链一侧读回委托"), err)
			}
			_, err = race.requests.Save(txCtx, race.identity, request)
			return err
		})
	}()
	return holder
}

func (holder *chainHolder) release() {
	holder.once.Do(func() { close(holder.resume) })
}

// contender 是被测的那一笔：在自己的 goroutine 里跑编排，交回编排的错误与读出的结局。
type contender struct {
	done chan contenderEnd
}

type contenderEnd struct {
	outcome string
	err     error
}

func startContender(run func(context.Context) (string, error)) *contender {
	started := &contender{done: make(chan contenderEnd, 1)}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*scopeLockRaceDeadline)
		defer cancel()
		outcome, err := run(ctx)
		started.done <- contenderEnd{outcome: outcome, err: err}
	}()
	return started
}

// advisoryWaiters 数池外看到的、本用例这只库里正等着咨询锁的后端：被测路径等在 S 上时恰有一个。限本库，是因为共用实例上别的用例
// 在别的库里也会等锁，不限库就会把它们认成被测路径。
func (race *scopeLockRace) advisoryWaiters(ctx context.Context) int {
	race.t.Helper()
	var waiting int
	if err := race.watcher.QueryRow(ctx,
		`SELECT count(*) FROM pg_locks
		  WHERE locktype = 'advisory' AND NOT granted
		    AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&waiting); err != nil {
		race.t.Fatalf("看咨询锁：%v", err)
	}
	return waiting
}

func (race *scopeLockRace) awaitAdvisoryWaiter() bool {
	race.t.Helper()
	deadline := time.Now().Add(scopeLockRaceDeadline)
	for time.Now().Before(deadline) {
		if race.advisoryWaiters(race.t.Context()) > 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// requestRowLockedByAnother 报告委托行此刻是否被别的事务持着行锁：NOWAIT 取不到（55P03）即有人持着。
func (race *scopeLockRace) requestRowLockedByAnother() bool {
	race.t.Helper()
	ctx := race.t.Context()
	tx, err := race.watcher.Begin(ctx)
	if err != nil {
		race.t.Fatalf("开探针事务：%v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(ctx,
		`SELECT 1 FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1 AND customer_account_id = $2 AND source = $3 AND source_request_key = $4
		    FOR UPDATE NOWAIT`,
		race.identity.TenantID().String(), race.identity.CustomerAccountID().String(),
		race.identity.Source().String(), race.identity.RequestKey().String())
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "55P03" {
		return true
	}
	if err != nil {
		race.t.Fatalf("探委托行锁：%v", err)
	}
	return false
}

func (race *scopeLockRace) freezeStatus() int16 {
	return freezeStatusOnRealAssembly(race.t, race.db, race.resultID)
}

func isDeadlock(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "40P01"
}

// scopeLockCase 是三条撤回类路径各自的一份：怎么装编排、前提的委托怎么落、跑一次得到什么结局、输了该答什么、重发该答什么。
type scopeLockCase struct {
	name    string
	prepare func(*testing.T, *bentopg.DB) domain.SubmissionVersionID
	// build 装生产编排并交回「跑一次」的闭包，闭包把结局读成字符串。
	build func(t *testing.T, db *bentopg.DB, version domain.SubmissionVersionID) func(context.Context) (string, error)
	// lost 是输给先持 S 的那一笔时该答的结局；formed 是重发之后形成时的结局。
	lost, formed string
}

func scopeLockCases() []scopeLockCase {
	return []scopeLockCase{
		{
			name:    "撤回",
			prepare: submittedRequestOnRealAssembly,
			build: func(t *testing.T, db *bentopg.DB, version domain.SubmissionVersionID) func(context.Context) (string, error) {
				handler, err := assembleWithdrawalOrchestration(db, newScriptedWithdrawalAuthorizer(t))
				if err != nil {
					t.Fatalf("装配撤回编排：%v", err)
				}
				command := withdrawalCommand(t, submissionCommand(t).Identity)
				command.ShipmentRequestID = submissionCommand(t).ShipmentRequestID
				command.SubmissionVersion = version
				return func(ctx context.Context) (string, error) {
					result, err := handler.Handle(ctx, command)
					return result.Outcome().String(), err
				}
			},
			lost:   shipmentapp.WithdrawalUndecided.String(),
			formed: shipmentapp.WithdrawalFormed.String(),
		},
		{
			name: "主动拒绝",
			prepare: func(t *testing.T, db *bentopg.DB) domain.SubmissionVersionID {
				seedSYNRejectionGrant(t, db)
				return submittedRequestOnRealAssembly(t, db)
			},
			build: func(t *testing.T, db *bentopg.DB, version domain.SubmissionVersionID) func(context.Context) (string, error) {
				handler, err := rejectionOrchestrationWith(db, synRRejectionRequestSource{t: t})
				if err != nil {
					t.Fatalf("装配主动拒绝编排：%v", err)
				}
				command := submissionCommand(t)
				rejection := shipmentapp.RejectShipmentRequestCommand{
					Identity:          command.Identity,
					ShipmentRequestID: command.ShipmentRequestID,
					SubmissionVersion: version,
					Decider:           mustValue(t, domain.NewDeciderReference, synFirstDecider),
					Reason:            mustValue(t, domain.NewRejectionReasonReference, "SYN-REJECTION-REASON-1"),
					Evidence:          mustValue(t, domain.NewRejectionEvidenceReference, "SYN-REJECTION-EVIDENCE-1"),
				}
				return func(ctx context.Context) (string, error) {
					result, err := handler.Handle(ctx, rejection)
					return result.Outcome().String(), err
				}
			},
			lost:   shipmentapp.ActiveRejectionUndecided.String(),
			formed: shipmentapp.ActiveRejectionFormed.String(),
		},
		{
			name:    "授权处置",
			prepare: awaitingDispositionOnRealAssembly,
			build: func(t *testing.T, db *bentopg.DB, version domain.SubmissionVersionID) func(context.Context) (string, error) {
				handler, err := dispositionOrchestrationWith(db, synRDispositionAuthorizer{t: t})
				if err != nil {
					t.Fatalf("装配授权处置编排：%v", err)
				}
				command := submissionCommand(t)
				disposition := shipmentapp.DisposeShipmentRequestCommand{
					Identity:          command.Identity,
					ShipmentRequestID: command.ShipmentRequestID,
					SubmissionVersion: version,
					Disposer:          mustValue(t, domain.NewDisposerReference, synFirstDecider),
					Choice:            domain.DisposeByRejection,
					Reason:            mustValue(t, domain.NewDispositionReasonReference, "SYN-DISPOSITION-REASON-1"),
					Evidence:          mustValue(t, domain.NewDispositionEvidenceReference, "SYN-DISPOSITION-EVIDENCE-1"),
				}
				return func(ctx context.Context) (string, error) {
					result, err := handler.Handle(ctx, disposition)
					return result.Outcome().String(), err
				}
			},
			lost:   shipmentapp.AuthorizedDispositionConflict.String(),
			formed: shipmentapp.AuthorizedDispositionRecorded.String(),
		},
	}
}

// Covers: 票 docs-consistency/code/99 与 UC-PS-005 一致性「撤回、主动拒绝、授权处置先于保存委托取控制作用域锁」——同一份委托上，
// 接受链续办取到 S 之后撤回类路径到达：它等在 S 上、不持委托行（它若先保存委托、持着行锁再在释放里等 S，链随后保存委托
// 等它，两边互等，PostgreSQL 中止一笔，40P01）；链提交之后撤回类路径拿到 S，保存撞上链抢先落下的修订，答确定的未决
// 或冲突格、不释放链刚占着的冻结；客户重发，同一路径形成并释放冻结。
func TestWithdrawalClassPathsTakeTheScopeLockBeforeSavingTheRequestRow(t *testing.T) {
	for _, tc := range scopeLockCases() {
		t.Run(tc.name, func(t *testing.T) {
			race := newScopeLockRace(t, tc.prepare)
			run := tc.build(t, race.db, race.version)

			holder := race.startChainHolder()
			t.Cleanup(holder.release)
			select {
			case <-holder.reached:
			case <-time.After(scopeLockRaceDeadline):
				t.Fatal("链一侧没在期限内取到 S")
			}

			first := startContender(run)
			if !race.awaitAdvisoryWaiter() {
				holder.release()
				t.Fatalf("%s没有等在 S 上", tc.name)
			}
			// 不在这里停：次序倒挂时接着往下放行，让互等真的发生，由下面的 40P01 断言把死锁本身也认出来。
			if race.requestRowLockedByAnother() {
				t.Errorf("%s等在 S 上时已持着委托行锁——先保存委托、再取 S，与接受链次序相反", tc.name)
			}
			holder.release()

			var firstEnd contenderEnd
			select {
			case firstEnd = <-first.done:
			case <-time.After(2 * scopeLockRaceDeadline):
				t.Fatalf("%s没有在期限内结束", tc.name)
			}
			select {
			case holderErr := <-holder.done:
				if holderErr != nil {
					t.Fatalf("链一侧事务：%v（死锁 = %v）", holderErr, isDeadlock(holderErr))
				}
			case <-time.After(2 * scopeLockRaceDeadline):
				t.Fatal("链一侧没有在期限内结束")
			}
			if isDeadlock(firstEnd.err) {
				t.Fatalf("%s被死锁检测中止：%v", tc.name, firstEnd.err)
			}
			if firstEnd.err != nil || firstEnd.outcome != tc.lost {
				t.Fatalf("%s输给先持 S 的链：结局 %q err=%v，want %q 且无错", tc.name, firstEnd.outcome, firstEnd.err, tc.lost)
			}
			if got := race.freezeStatus(); got != int16(sadomain.FreezeHeld) {
				t.Fatalf("输了的一路之后冻结 status = %d, want 仍占着", got)
			}

			retried := startContender(run)
			select {
			case end := <-retried.done:
				if end.err != nil || end.outcome != tc.formed {
					t.Fatalf("重发：结局 %q err=%v，want %q 且无错", end.outcome, end.err, tc.formed)
				}
			case <-time.After(2 * scopeLockRaceDeadline):
				t.Fatal("重发没有在期限内结束")
			}
			if got := race.freezeStatus(); got != int16(sadomain.FreezeReleased) {
				t.Fatalf("重发形成之后冻结 status = %d, want 已释放", got)
			}
		})
	}
}
