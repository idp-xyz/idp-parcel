package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 与真接受链证撤回和接受链续办对同一份委托取锁的次序一致（票 docs-consistency/code/99）：
// 接受链续办事务在 freezeFunds 取到控制作用域锁 S 之后停住，客户此时撤回。夹具同 customer_supplement_pause_release_test.go：
// 控制与释放走生产 PS→SA 适配器、SA 施加与释放编排与两本账。撤回若先保存委托（持委托行锁 R）、再在释放里等 S，链放行后保存
// 委托等 R，两边互等，PostgreSQL 中止一笔；撤回先取 S 则等在 S 上、不持 R，链提交之后撤回保存撞上链落下的修订。
// 撤回、主动拒绝、授权处置三条路径对同一份委托各自的同一场景在 cmd/parcel-api 的 control_scope_lock_order_test.go。

const scopeLockRaceDeadline = 20 * time.Second

// synRGatedControl 把施加转给生产适配器，施加答复回来之后（S 已由 SA 的 freezeFunds 取到、持到事务结束）停在屏障上，
// 等 resume 才把答复交给链往下走去保存委托。屏障只拦一次，arm 之后的第一次施加。
type synRGatedControl struct {
	inner   psports.PreAcceptanceFinancialController
	armed   atomic.Bool
	reached chan struct{}
	resume  chan struct{}
}

func newSynRGatedControl() *synRGatedControl {
	return &synRGatedControl{reached: make(chan struct{}), resume: make(chan struct{})}
}

func (control *synRGatedControl) ApplyPreAcceptanceFinancialControl(
	ctx context.Context,
	request psports.FinancialControlRequest,
) (psports.PreAcceptanceControlAssessment, error) {
	assessment, err := control.inner.ApplyPreAcceptanceFinancialControl(ctx, request)
	if control.armed.CompareAndSwap(true, false) {
		close(control.reached)
		<-control.resume
	}
	return assessment, err
}

var _ psports.PreAcceptanceFinancialController = (*synRGatedControl)(nil)

// Covers: 票 docs-consistency/code/99 与 UC-PS-005 一致性——接受链续办取到 S 之后客户撤回：撤回等在 S 上、不持委托行（它若先
// 保存委托、持着行锁再在释放里等 S，链随后保存委托等它，互等）；链提交接受之后撤回拿到 S，保存撞上链抢先落下的修订，答
// 未决、委托不会既已接受又已撤回，链刚占下的冻结原样占着；客户重发，撤回读到既有的接受决定、不释放那笔合法冻结。
func TestAWithdrawalWhileTheChainHoldsTheScopeLockWaitsOnItWithoutHoldingTheRequestRow(t *testing.T) {
	gate := newSynRGatedControl()
	chainRelease := &synRFlakyRelease{}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: gate, release: chainRelease})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	gate.inner, chainRelease.inner = adapter, adapter
	withdrawalRelease := &synRFlakyRelease{inner: adapter}

	newVersion := fixture.requestID.String() + "/" + synRSupplementVersionID

	// 首版占下后暂停，客户补件形成新版本；续办信封在手，链还没驱。
	paused := fixture.pauseFirstVersion(t, ctx)
	supplemented := fixture.supplementOn(t, ctx, paused.CurrentSubmissionVersion().VersionID())
	if supplemented.Outcome() != psapplication.SupplementRecorded {
		t.Fatalf("受控补充 outcome = %q, want SUPPLEMENT_RECORDED（未决原因 %q）", supplemented.Outcome(), supplemented.PendingReason())
	}
	resume := fixture.claimDeliveryOfType(t, string(psinbox.SubmissionVersionFormedEventType)).Envelope

	// 看锁的连接单独开：两路各占池里一条，再从池里取会与停住的那一路互等。
	watcher, err := pgx.ConnectConfig(ctx, fixture.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开看锁连接：%v", err)
	}
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })

	gate.armed.Store(true)
	chainDone := make(chan error, 1)
	go func() {
		chainDone <- fixture.supplementGate.Consume(ctx, resume)
	}()
	t.Cleanup(func() {
		select {
		case <-gate.resume:
		default:
			close(gate.resume)
		}
	})
	select {
	case <-gate.reached:
	case <-time.After(scopeLockRaceDeadline):
		t.Fatal("链没在期限内走到屏障")
	}

	type withdrawalEnd struct {
		result psapplication.WithdrawShipmentRequestResult
		err    error
	}
	withdraw := func() chan withdrawalEnd {
		done := make(chan withdrawalEnd, 1)
		handler := fixture.withdrawal(t, withdrawalRelease)
		go func() {
			var result psapplication.WithdrawShipmentRequestResult
			err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
				var handleErr error
				result, handleErr = handler.Handle(txCtx, fixture.withdrawCommand(t, synRSupplementVersionID))
				return handleErr
			})
			done <- withdrawalEnd{result: result, err: err}
		}()
		return done
	}
	withdrawn := withdraw()

	// 撤回等在 S 上（池外连接看到有后端在等咨询锁）。
	waiting := false
	for deadline := time.Now().Add(scopeLockRaceDeadline); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		var count int
		if err := watcher.QueryRow(ctx,
			`SELECT count(*) FROM pg_locks
		  WHERE locktype = 'advisory' AND NOT granted
		    AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&count); err != nil {
			t.Fatalf("看咨询锁：%v", err)
		}
		if count > 0 {
			waiting = true
			break
		}
	}
	if !waiting {
		t.Error("撤回没有等在 S 上")
	}
	// 它等在 S 上时不能已持着委托行锁：NOWAIT 取得到才说明没人持着。
	probe, err := watcher.Begin(ctx)
	if err != nil {
		t.Fatalf("开探针事务：%v", err)
	}
	_, probeErr := probe.Exec(ctx,
		`SELECT 1 FROM parcel_shipment.shipment_request WHERE shipment_request_id = $1 FOR UPDATE NOWAIT`,
		fixture.requestID.String())
	_ = probe.Rollback(ctx)
	var lockNotAvailable *pgconn.PgError
	if errors.As(probeErr, &lockNotAvailable) && lockNotAvailable.Code == "55P03" {
		t.Error("撤回等在 S 上时已持着委托行锁——先保存委托、再取 S，与接受链次序相反")
	} else if probeErr != nil {
		t.Fatalf("探委托行锁：%v", probeErr)
	}

	close(gate.resume)
	var chainErr error
	select {
	case chainErr = <-chainDone:
	case <-time.After(2 * scopeLockRaceDeadline):
		t.Fatal("链没在期限内结束")
	}
	var end withdrawalEnd
	select {
	case end = <-withdrawn:
	case <-time.After(2 * scopeLockRaceDeadline):
		t.Fatal("撤回没在期限内结束")
	}

	if isDeadlockError(chainErr) || isDeadlockError(end.err) {
		t.Fatalf("同一份委托上链与撤回互等，死锁检测中止了一笔：链 %v；撤回 %v", chainErr, end.err)
	}
	if chainErr != nil {
		t.Fatalf("续办一拍：%v", chainErr)
	}
	if end.err != nil {
		t.Fatalf("撤回：%v", end.err)
	}
	if end.result.Outcome() != psapplication.WithdrawalUndecided {
		t.Fatalf("输给先持 S 的链：撤回 outcome = %q, want UNDECIDED", end.result.Outcome())
	}
	if end.result.PendingReason() != psapplication.StaleShipmentRequestRevision {
		t.Fatalf("撤回未决原因 = %q, want STALE_SHIPMENT_REQUEST_REVISION", end.result.PendingReason())
	}
	if accepted := fixture.mustLoadRequest(t, ctx); accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("链提交之后状态 = %q, want ACCEPTED", accepted.State())
	} else if _, withdrawal := accepted.Withdrawal(); withdrawal {
		t.Fatal("委托既已接受又带着撤回决定")
	}
	if got := fixture.freeze(t, ctx, newVersion); !got.present || got.status != 1 {
		t.Fatalf("新版本冻结 = %+v, want 链刚占下的那笔原样占着——输了的撤回不放合法冻结", got)
	}

	// 客户重发：委托已越过提交边界，撤回读回那份接受决定，不问授权、不释放。
	retried := <-withdraw()
	if retried.err != nil || retried.result.Outcome() != psapplication.WithdrawalDecisionAlreadyFormed {
		t.Fatalf("重发：outcome = %q err = %v, want DECISION_ALREADY_FORMED", retried.result.Outcome(), retried.err)
	}
	if _, formed := retried.result.AcceptanceDecision(); !formed {
		t.Fatal("重发的答复没带那份接受决定")
	}
	if got := fixture.freeze(t, ctx, newVersion); got.status != 1 {
		t.Fatalf("重发之后新版本冻结 = %+v, want 仍占着", got)
	}
}

func isDeadlockError(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "40P01"
}
