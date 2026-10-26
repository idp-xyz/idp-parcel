package main

import (
	"context"
	"testing"
	"time"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
)

// 本文件对真实 PostgreSQL 16 证两格（票 docs-consistency/code/72、73），夹具同 superseded_version_control_release_test.go：
// 控制与释放走生产 PS→SA 适配器、作用域源、SA 施加与释放编排与两本账，余额恰好只够冻一笔。
//   - 停在`等待受控补充`的那一轮按原关联放掉停住那一版的冻结：那一版只由新版本的信封续办、不会再被判。余额因此腾出来，
//     补件之后新版本不靠链开头那一步也冻得下；链开头对同一笔再请求一次，提供方按幂等答原释放。
//   - 撤回这类终局路径放这份委托各版的占用：暂停那一次释放放不掉、首版那一笔留着时，补件之后、新版本那一轮链提交之前
//     撤回，首版那一笔由撤回放掉，不成孤儿。

// Covers: PS CONTEXT 生命周期「判断遇到客户可补充缺口 → 等待受控补充」与词条「控制项结果」、ADR-0132 决定一的理由与
// ADR-0106 决定三、ADR-0122 决定三（释放幂等由提供方账本认领）。
func TestACustomerSupplementPauseReleasesThePausedVersionsFreeze(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter

	firstVersion := fixture.requestID.String() + "/SYN-VER-01"
	newVersion := fixture.requestID.String() + "/" + synRSupplementVersionID

	paused := fixture.pauseFirstVersion(t, ctx)
	pausedRelease := fixture.freeze(t, ctx, firstVersion)
	if !pausedRelease.present || pausedRelease.status != 2 || pausedRelease.releasedAt.IsZero() {
		t.Fatalf("暂停之后首版冻结 = %+v, want 已释放——停住的那一版不会再被判，它占着的余额要腾出来", pausedRelease)
	}
	if release.released != 1 {
		t.Fatalf("暂停那一轮释放到达提供方 %d 次, want 1", release.released)
	}

	// 补件之后新版本照常进入控制：余额只够冻一笔，首版那一笔若还占着，新版本就冻不下。
	supplemented := fixture.supplementOn(t, ctx, paused.CurrentSubmissionVersion().VersionID())
	if supplemented.Outcome() != psapplication.SupplementRecorded {
		t.Fatalf("受控补充 outcome = %q, want SUPPLEMENT_RECORDED（未决原因 %q）", supplemented.Outcome(), supplemented.PendingReason())
	}
	resume := fixture.claimDeliveryOfType(t, string(psinbox.SubmissionVersionFormedEventType)).Envelope
	if err := fixture.supplementGate.Consume(ctx, resume); err != nil {
		t.Fatalf("续办信封：%v", err)
	}
	if got := fixture.freeze(t, ctx, newVersion); !got.present || got.status != 1 || got.amount != synRSupersededFreezeMinor {
		t.Fatalf("新版本冻结 = %+v, want 已冻结 %d", got, synRSupersededFreezeMinor)
	}
	if accepted := fixture.mustLoadRequest(t, ctx); accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
	// 链开头对首版那一笔再请求一次：到达提供方，答原释放，释放时刻不变。
	if release.released != 2 {
		t.Fatalf("释放到达提供方 %d 次, want 2——暂停一次、链开头再请求一次", release.released)
	}
	if again := fixture.freeze(t, ctx, firstVersion); again.status != 2 || !again.releasedAt.Equal(pausedRelease.releasedAt) {
		t.Fatalf("链开头再请求之后首版冻结 = %+v, want 仍是暂停时那一次释放（%v）", again, pausedRelease.releasedAt)
	}
}

// Covers: UC-PS-005 步骤 6「按原业务关联幂等释放已成立各项的占用」与 PS CONTEXT 生命周期「已提交期间形成新提交版本」——
// 补件之后、新版本那一轮链提交之前撤回，被取代那一版的冻结由撤回放掉。
func TestAWithdrawalBeforeTheNewVersionsChainReleasesTheSupersededFreeze(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter

	firstVersion := fixture.requestID.String() + "/SYN-VER-01"

	// 暂停那一次释放放不掉，首版那一笔留着。
	release.failuresLeft = 1
	paused := fixture.pauseFirstVersion(t, ctx)
	if got := fixture.freeze(t, ctx, firstVersion); got.status != 1 {
		t.Fatalf("首版冻结 = %+v, want 仍为已冻结——暂停那一次释放放不掉", got)
	}

	supplemented := fixture.supplementOn(t, ctx, paused.CurrentSubmissionVersion().VersionID())
	if supplemented.Outcome() != psapplication.SupplementRecorded {
		t.Fatalf("受控补充 outcome = %q, want SUPPLEMENT_RECORDED（未决原因 %q）", supplemented.Outcome(), supplemented.PendingReason())
	}

	// 新版本那一封还没驱链，客户就撤回了：撤回是终局，之后再没有轮次去放首版那一笔。
	withdrawal := fixture.withdrawal(t, release)
	var result psapplication.WithdrawShipmentRequestResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = withdrawal.Handle(txCtx, fixture.withdrawCommand(t, synRSupplementVersionID))
		return err
	})
	if result.Outcome() != psapplication.WithdrawalFormed {
		t.Fatalf("撤回 outcome = %q, want FORMED（未决原因 %q）", result.Outcome(), result.PendingReason())
	}
	if result.CompensationReference().String() != "" {
		t.Fatalf("各版都放掉了却留了补偿引用 %q", result.CompensationReference())
	}
	if got := fixture.freeze(t, ctx, firstVersion); got.status != 2 || got.releasedAt.IsZero() {
		t.Fatalf("撤回之后首版冻结 = %+v, want 已释放——被取代那一版那一笔成了孤儿", got)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.funds_freeze`); n != 1 {
		t.Fatalf("冻结行数 = %d, want 1——新版本一步没判，不该占下任何东西", n)
	}
}

// pauseFirstVersion 提交首版并驱提交门：可达性答`证据不足`，链在控制处占下首版那一笔，形成决定处停在`等待受控补充`，
// 本份投递入账。交回暂停之后读回的委托。
func (fixture *customerSupplementLoopFixture) pauseFirstVersion(t *testing.T, ctx context.Context) psdomain.ShipmentRequest {
	t.Helper()
	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})
	delivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, delivery.Envelope); err != nil {
		t.Fatalf("等待受控补充必须按处理完毕入账：%v", err)
	}
	fixture.markPublished(t, delivery)
	paused := fixture.mustLoadRequest(t, ctx)
	if waiting, present := paused.AcceptanceDecisionTask().WaitingOn(); !present || waiting != psdomain.ResumeByCustomerSupplement {
		t.Fatalf("等待态 = %v（present=%v）, want CUSTOMER_SUPPLEMENT", waiting, present)
	}
	return paused
}

// withdrawal 用生产的来源仓储、委托仓储、判断账与给定的释放口装一只撤回编排；只有授权口是替身（恒授权）。释放口传链
// 用的那一只，两边认的就是同一本账。
func (fixture *customerSupplementLoopFixture) withdrawal(
	t *testing.T,
	release psports.PreAcceptanceControlRelease,
) *psapplication.WithdrawShipmentRequestHandler {
	t.Helper()
	sources, err := pspostgres.NewSourceSubmissions(fixture.db)
	if err != nil {
		t.Fatalf("构造来源仓储：%v", err)
	}
	refusals, err := pspostgres.NewControlReleaseRefusals(fixture.db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	scopeLocks, err := sapostgres.NewPreAcceptanceControlScopeLocks(fixture.db)
	if err != nil {
		t.Fatalf("构造控制作用域锁：%v", err)
	}
	return psapplication.NewWithdrawShipmentRequestHandler(psapplication.WithdrawShipmentRequestDeps{
		Sources:    sources,
		Requests:   fixture.requests,
		Authorizer: synRGrantedWithdrawal{t: t},
		Judgments:  fixture.judgments,
		Recorder:   fixture.judgments,
		Release:    release,
		Refusals:   refusals,
		ScopeLock:  pssettlement.NewControlScopeLocks(scopeLocks),
		Identities: &synSDecisionIdentities{},
		Clock:      systemClock{},
	})
}

func (fixture *customerSupplementLoopFixture) withdrawCommand(t *testing.T, version string) psapplication.WithdrawShipmentRequestCommand {
	t.Helper()
	withdrawalIdentity, err := psdomain.NewSourceIdentity(
		fixture.identity.TenantID(),
		fixture.identity.CustomerAccountID(),
		fixture.identity.Source(),
		mustPS(t, psdomain.NewSourceRequestKey, "SYN-KEY-01-WITHDRAWAL"),
	)
	if err != nil {
		t.Fatalf("撤回来源身份：%v", err)
	}
	now := time.Now().UTC()
	return psapplication.WithdrawShipmentRequestCommand{
		Identity:           fixture.identity,
		WithdrawalIdentity: withdrawalIdentity,
		PayloadDigest:      mustPS(t, psdomain.NewPayloadDigest, "SYN-DIGEST-WITHDRAWAL"),
		OccurredAt:         now.Add(-2 * time.Second),
		ReceivedAt:         now.Add(-time.Second),
		ShipmentRequestID:  fixture.requestID,
		SubmissionVersion:  mustPS(t, psdomain.NewSubmissionVersionID, version),
		Requester:          mustPS(t, psdomain.NewWithdrawalRequesterReference, "SYN-CUSTOMER-CONTACT-01"),
		Reason:             mustPS(t, psdomain.NewWithdrawalReasonReference, "SYN-NO-LONGER-REQUIRED"),
	}
}

// synRGrantedWithdrawal 恒答已授权：本文件证的是撤回之后放了哪几笔，不是谁有撤回权。
type synRGrantedWithdrawal struct{ t *testing.T }

func (grant synRGrantedWithdrawal) AuthorizeWithdrawal(
	_ context.Context,
	_ psports.WithdrawalAuthorizationQuery,
) (psports.WithdrawalAuthorization, error) {
	return psports.WithdrawalAuthorization{
		Outcome:   psports.AuthorizationGranted,
		Authority: mustPS(grant.t, psdomain.NewWithdrawalAuthorityReference, "SYN-WITHDRAW-ROLE-01"),
	}, nil
}

var _ psports.WithdrawalAuthorizer = synRGrantedWithdrawal{}
