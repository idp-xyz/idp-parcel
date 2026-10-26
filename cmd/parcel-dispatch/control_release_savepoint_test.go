package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证释放口外那一层 savepoint（pspostgres.SavepointedControlRelease，生产装配在形成决定、撤回、
// 主动拒绝与授权处置的释放口外都包它）：释放在库里报 SQL 级错误时，调用方事务里已成立的暂停或终局决定照样提交。夹具同
// superseded_version_control_release_test.go；「中毒」的释放口在调用方事务里执行 `SELECT 1/0`，像一本读写不动的账本那样，
// 把那一笔事务打进中止态。

// synRPoisonedRelease 在调用方事务里执行一条必然失败的 SQL，再像 settlement-accounting 的释放编排那样把它折成`可重试未形成`
// 正常交回——事务却已中止。
type synRPoisonedRelease struct{ db *bentopg.DB }

func (poison synRPoisonedRelease) ReleasePreAcceptanceControl(
	ctx context.Context,
	_ psports.ControlReleaseRequest,
) (psports.ControlReleaseAnswer, error) {
	executor, err := poison.db.RequireExecutor(ctx)
	if err != nil {
		return psports.ControlReleaseAnswer{}, err
	}
	if _, err := executor.Exec(ctx, `SELECT 1/0`); err == nil {
		return psports.ControlReleaseAnswer{}, errors.New("SELECT 1/0 竟然成功了")
	}
	reason, err := psdomain.NewCheckReason("SA-FREEZE_LEDGER_UNAVAILABLE")
	if err != nil {
		return psports.ControlReleaseAnswer{}, err
	}
	return psports.ControlReleaseAnswer{Outcome: psports.ControlReleaseNotFormed, Reason: reason}, nil
}

var _ psports.PreAcceptanceControlRelease = synRPoisonedRelease{}

// poisonedFixture 起受控补充闭环夹具、接好结算一侧，交回控制口与可换内层的释放口。
func poisonedFixture(t *testing.T) (*customerSupplementLoopFixture, *synRFlakyRelease, psports.PreAcceptanceControlRelease) {
	t.Helper()
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	adapter := fixture.wireSettlement(t, t.Context())
	control.inner = adapter
	return fixture, release, adapter
}

// Covers: 前提——没有 savepoint 时，暂停那一次释放在库里出错会把暂停一起带走：消费门提交答 25P02，委托回到`已提交`、
// 等待态没落库，投递没有入账。下面两条用例证的就是装饰器把这一格收住。
func TestAPoisonedReleaseWithoutTheSavepointTakesThePauseWithIt(t *testing.T) {
	fixture, release, _ := poisonedFixture(t)
	ctx := t.Context()
	release.inner = synRPoisonedRelease{db: fixture.db}

	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})
	delivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	err := fixture.submittedGate.Consume(ctx, delivery.Envelope)
	if err == nil || !strings.Contains(err.Error(), "25P02") {
		t.Fatalf("裸接中毒释放口时 Consume err = %v, want 提交答 25P02——用例的前提不成立", err)
	}
	if _, present := fixture.mustLoadRequest(t, ctx).AcceptanceDecisionTask().WaitingOn(); present {
		t.Fatal("事务已中止，等待态却落了库")
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(delivery.Envelope.ID)); n != 0 {
		t.Fatalf("提交门 inbox 行数 = %d, want 0", n)
	}
}

// Covers: PS CONTEXT 生命周期「判断遇到客户可补充缺口 → 等待受控补充」——释放失败不撤销暂停，在库里也成立：暂停那一次释放
// 在调用方事务里执行 `SELECT 1/0`，装饰器退回 savepoint，暂停照样落库、本份投递入账，首版那一笔照旧占着；直驱形成决定那一步
// 时交回补偿引用，那一笔事务照样提交。
func TestAPoisonedPauseReleaseStillCommitsThePause(t *testing.T) {
	fixture, release, _ := poisonedFixture(t)
	ctx := t.Context()
	release.inner = pspostgres.NewSavepointedControlRelease(fixture.db, synRPoisonedRelease{db: fixture.db})
	firstVersion := fixture.requestID.String() + "/SYN-VER-01"

	fixture.pauseFirstVersion(t, ctx)
	if got := fixture.freeze(t, ctx, firstVersion); !got.present || got.status != 1 {
		t.Fatalf("首版冻结 = %+v, want 仍为已冻结——释放退回了 savepoint", got)
	}

	var result psapplication.FormAcceptanceDecisionResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = fixture.decision.Handle(txCtx, psapplication.FormAcceptanceDecisionCommand{
			Identity:          fixture.identity,
			ShipmentRequestID: fixture.requestID,
			SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, "SYN-VER-01"),
		})
		return err
	})
	if result.PendingReason() != psapplication.CustomerSupplementPending {
		t.Fatalf("pending reason = %q, want CUSTOMER_SUPPLEMENT_PENDING", result.PendingReason())
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("释放没收口却没有补偿引用")
	}
	if waiting, present := fixture.mustLoadRequest(t, ctx).AcceptanceDecisionTask().WaitingOn(); !present ||
		waiting != psdomain.ResumeByCustomerSupplement {
		t.Fatalf("等待态 = %v（present=%v）, want CUSTOMER_SUPPLEMENT", waiting, present)
	}
}

// Covers: UC-PS-005「撤回提交成功后不等待财务释放才生效」与 AT-PS-073——撤回那一次释放在调用方事务里执行 `SELECT 1/0`，
// 装饰器退回 savepoint，撤回照样成立、提交，交回补偿引用；被取代那一版那一笔照旧占着，等补偿续办。
func TestAPoisonedWithdrawalReleaseStillCommitsTheWithdrawal(t *testing.T) {
	fixture, release, adapter := poisonedFixture(t)
	ctx := t.Context()
	release.inner = adapter
	firstVersion := fixture.requestID.String() + "/SYN-VER-01"

	// 暂停那一次释放放不掉，首版那一笔留着。
	release.failuresLeft = 1
	paused := fixture.pauseFirstVersion(t, ctx)
	supplemented := fixture.supplementOn(t, ctx, paused.CurrentSubmissionVersion().VersionID())
	if supplemented.Outcome() != psapplication.SupplementRecorded {
		t.Fatalf("受控补充 outcome = %q（未决原因 %q）", supplemented.Outcome(), supplemented.PendingReason())
	}

	withdrawal := fixture.withdrawal(t,
		pspostgres.NewSavepointedControlRelease(fixture.db, synRPoisonedRelease{db: fixture.db}))
	var result psapplication.WithdrawShipmentRequestResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = withdrawal.Handle(txCtx, fixture.withdrawCommand(t, synRSupplementVersionID))
		return err
	})
	if result.Outcome() != psapplication.WithdrawalFormed {
		t.Fatalf("撤回 outcome = %q, want FORMED（未决原因 %q）", result.Outcome(), result.PendingReason())
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("释放没收口却没有补偿引用")
	}
	var state int16
	if err := fixture.pool.QueryRow(ctx,
		`SELECT state FROM parcel_shipment.shipment_request WHERE shipment_request_id = $1`,
		fixture.requestID.String()).Scan(&state); err != nil {
		t.Fatalf("读状态投影：%v", err)
	}
	if state != int16(psdomain.ShipmentRequestWithdrawn) {
		t.Fatalf("库里 state = %d, want WITHDRAWN——释放出错把撤回带走了", state)
	}
	if got := fixture.freeze(t, ctx, firstVersion); got.status != 1 {
		t.Fatalf("首版冻结 = %+v, want 仍为已冻结——释放退回了 savepoint", got)
	}
}

// Covers: 停在`等待受控补充`、占用已随暂停释放的那一版若被再驱一拍，链在形成决定之前就停：控制那一步按同一请求身份重施加，
// settlement-accounting 交回那笔已释放的冻结，PS 侧译不出它，财务控制那一步响亮上抛（票 code/71「控制端口同此」，ADR-0081
// 决定三）——不折成`财务控制不可用`内部重试到失败预算耗尽，也不会在一笔已释放的冻结上形成接受；整轮回滚，等待态与冻结原样。
func TestARedrivenPausedVersionStopsAtTheReleasedFreeze(t *testing.T) {
	fixture, release, adapter := poisonedFixture(t)
	ctx := t.Context()
	release.inner = adapter
	firstVersion := fixture.requestID.String() + "/SYN-VER-01"

	fixture.pauseFirstVersion(t, ctx)
	if got := fixture.freeze(t, ctx, firstVersion); got.status != 2 {
		t.Fatalf("暂停之后首版冻结 = %+v, want 已释放", got)
	}

	var result psapplication.AdvanceAcceptanceChainResult
	err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		result, err = fixture.chain.Handle(txCtx, psapplication.AdvanceAcceptanceChainCommand{
			Identity:          fixture.identity,
			ShipmentRequestID: fixture.requestID,
			SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, "SYN-VER-01"),
			DeclaredParcelIDs: []psdomain.DeclaredParcelID{mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01")},
		})
		return err
	})
	if !errors.Is(err, psports.ErrUntranslatableProviderAnswer) {
		t.Fatalf("err = %v（outcome = %q stage = %q reason = %q）, want 包着 ErrUntranslatableProviderAnswer 的错误——译不动的答复"+
			"折成内部重试会一直重投到失败预算耗尽", err, result.Outcome(), result.Stage(), result.PendingReason())
	}
	after := fixture.mustLoadRequest(t, ctx)
	if after.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("再驱之后状态 = %q, want SUBMITTED", after.State())
	}
	if waiting, present := after.AcceptanceDecisionTask().WaitingOn(); !present || waiting != psdomain.ResumeByCustomerSupplement {
		t.Fatalf("等待态 = %v（present=%v）, want 仍为 CUSTOMER_SUPPLEMENT", waiting, present)
	}
	if got := fixture.freeze(t, ctx, firstVersion); got.status != 2 {
		t.Fatalf("再驱之后首版冻结 = %+v, want 仍为已释放", got)
	}
}
