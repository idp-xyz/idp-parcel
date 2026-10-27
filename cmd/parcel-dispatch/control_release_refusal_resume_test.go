package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
)

// 本文件对真实 PostgreSQL 16 证释放拒收处理的续办闭环（票 docs-consistency/code/95；ADR-0293 决定二、四）：客户补件之后，接受链开头放被取代那一版时
// 被提供方确定不受理，消费门响亮失败、新版本判不了；人把册上那一笔处理完，处理动作同笔交一封续办信封，第五扇门按这封信把同一条链拿委托的当前版本再驱一拍，
// 链开头那一步不再被这一笔挡住，新版本能判、接受成立；再驱幂等。夹具同 control_release_refusal_test.go 与 superseded_version_control_release_test.go：控制与
// 释放走生产 PS→SA 适配器、SA 释放编排与两本账，释放口外面套一只会按次数拒收的替身来摆出「被拒收」。
//   - 核对后在结算侧释放：暂停那一次与链开头那一次释放都被拒收（册上一行、新版本被挡），处理人核对后在结算侧释放，提供方这一回受理，旧冻结放掉。
//   - 标记已处理：控制结果没存作用域的存量行，提供方一律拒收；运维在结算侧手工放掉旧冻结之后标记已处理，链开头那一步认出这一笔人已处理，新版本能判。

const refusalHandledResumeConsumerName = "parcel-shipment/advance-acceptance-chain-on-control-release-refusal-handled"

// refusalResumeGate 建第五扇门（释放拒收已处理），与生产装配同一条链。
func (fixture *customerSupplementLoopFixture) refusalResumeGate(t *testing.T) *psinbox.ControlReleaseRefusalHandledConsumer {
	t.Helper()
	inboxStore, err := inbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	gate, err := psinbox.NewControlReleaseRefusalHandledConsumer(fixture.transactor, inboxStore, fixture.chain)
	if err != nil {
		t.Fatalf("构造释放拒收处理续办门：%v", err)
	}
	return gate
}

// refusalHandlingHandler 用生产的仓储、册、处理留痕、判断账、续办信封交接与作用域锁装一只处理编排，释放口传 release（外面套生产的 savepoint 装饰器）。
func (fixture *customerSupplementLoopFixture) refusalHandlingHandler(
	t *testing.T,
	release psports.PreAcceptanceControlRelease,
) *psapplication.HandleControlReleaseRefusalHandler {
	t.Helper()
	refusals, err := pspostgres.NewControlReleaseRefusals(fixture.db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	handlings, err := pspostgres.NewControlReleaseRefusalHandlings(fixture.db)
	if err != nil {
		t.Fatalf("构造处理留痕：%v", err)
	}
	scopeLocks, err := sapostgres.NewPreAcceptanceControlScopeLocks(fixture.db)
	if err != nil {
		t.Fatalf("构造控制作用域锁：%v", err)
	}
	handoff, err := pspostgres.NewOutboxControlReleaseRefusalHandledHandoff(fixture.db, fixture.store, systemClock{})
	if err != nil {
		t.Fatalf("构造处理续办交接：%v", err)
	}
	return psapplication.NewHandleControlReleaseRefusalHandler(psapplication.HandleControlReleaseRefusalDeps{
		Requests:  fixture.requests,
		Refusals:  refusals,
		Handlings: handlings,
		Judgments: fixture.judgments,
		Release:   pspostgres.NewSavepointedControlRelease(fixture.db, release),
		ScopeLock: pssettlement.NewControlScopeLocks(scopeLocks),
		Handoff:   handoff,
		Clock:     systemClock{},
	})
}

func (fixture *customerSupplementLoopFixture) handleRefusal(
	t *testing.T,
	ctx context.Context,
	handler *psapplication.HandleControlReleaseRefusalHandler,
	version string,
	action psapplication.ControlReleaseRefusalAction,
) psapplication.HandleControlReleaseRefusalResult {
	t.Helper()
	var result psapplication.HandleControlReleaseRefusalResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = handler.Handle(txCtx, psapplication.HandleControlReleaseRefusalCommand{
			Identity:          fixture.identity,
			ShipmentRequestID: fixture.requestID,
			SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, version),
			ControlResultID:   mustPS(t, psdomain.NewFinancialControlResultID, fixture.requestID.String()+"/"+version),
			Action:            action,
			Handler:           mustPS(t, psdomain.NewControlReleaseRefusalHandlerReference, "https://id.syn.example/dex#SYN-OPERATOR-CODE95"),
			Basis:             mustPS(t, psdomain.NewControlReleaseRefusalHandlingBasis, "SYN-CHECKED-AGAINST-SETTLEMENT-LEDGER"),
		})
		return err
	})
	return result
}

func (fixture *customerSupplementLoopFixture) registeredRefusals(t *testing.T, ctx context.Context) []psports.ControlReleaseRefusal {
	t.Helper()
	refusals, err := pspostgres.NewControlReleaseRefusals(fixture.db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	listed, err := refusals.ListByShipmentRequest(ctx, fixture.identity.TenantID(), fixture.requestID)
	if err != nil {
		t.Fatalf("读释放未受理册：%v", err)
	}
	return listed
}

// stuckAfterSupplement 摆出票面的现象：首版停在等待受控补充，客户补件形成新版本，续办信封驱链时开头那一步被提供方拒收，消费门响亮失败（不入账、不当未决），
// 新版本一步不判。交回被拒收的旧版本控制关联。
func (fixture *customerSupplementLoopFixture) stuckAfterSupplement(t *testing.T, ctx context.Context, paused psdomain.ShipmentRequest) {
	t.Helper()
	supplemented := fixture.supplementOn(t, ctx, paused.CurrentSubmissionVersion().VersionID())
	if supplemented.Outcome() != psapplication.SupplementRecorded {
		t.Fatalf("受控补充 outcome = %q（未决原因 %q）", supplemented.Outcome(), supplemented.PendingReason())
	}
	delivery := fixture.claimDeliveryOfType(t, string(psinbox.SubmissionVersionFormedEventType))
	err := fixture.supplementGate.Consume(ctx, delivery.Envelope)
	if !errors.Is(err, psapplication.ErrControlReleaseNotAccepted) {
		t.Fatalf("消费门 = %v, want ErrControlReleaseNotAccepted（publish_failed）", err)
	}
	// 响亮失败的信封耗尽失败预算后死信（ABANDONED），分区队首随之让出；之后同一份委托上的信封才认领得到。
	if err := fixture.store.Abandon(ctx, delivery.Ref, time.Now().UTC(), "dispatch.publish_failed"); err != nil {
		t.Fatalf("信封死信：%v", err)
	}
	if fixture.reachability.assessments != 1 {
		t.Fatalf("可达性权威被问了 %d 次, want 仍为 1——拒收挡着，新版本一步不判", fixture.reachability.assessments)
	}
	if after := fixture.mustLoadRequest(t, ctx); after.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("状态 = %q, want SUBMITTED", after.State())
	}
}

// Covers: ADR-0293 决定二、四；ADR-0282 决定三——核对后在结算侧释放闭环：旧版本那一笔两次被拒收（暂停记册、链开头响亮失败），新版本判不了；册上那一笔经
// 「在结算侧释放」处理（提供方这一回受理，旧冻结放掉），处理留痕与续办信封同笔落地；第五扇门拿当前版本再驱一拍，新版本照常占下冻结、接受成立；
// 同一封重投与绕过 inbox 再驱都幂等。
func TestHandlingARefusedReleaseAtSettlementResumesTheChainOnTheCurrentVersion(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{refusalsLeft: 2}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter
	firstVersion := fixture.requestID.String() + "/SYN-VER-01"
	newVersion := fixture.requestID.String() + "/" + synRSupplementVersionID

	paused := fixture.pauseFirstVersion(t, ctx)
	listed := fixture.registeredRefusals(t, ctx)
	if len(listed) != 1 || listed[0].SubmissionVersion.String() != "SYN-VER-01" || listed[0].ControlResultID.String() != firstVersion {
		t.Fatalf("暂停那一次被拒收，册上 = %+v, want 首版那一笔", listed)
	}
	fixture.stuckAfterSupplement(t, ctx, paused)
	if got := fixture.freeze(t, ctx, firstVersion); got.status != 1 {
		t.Fatalf("首版冻结 = %+v, want 原样占着", got)
	}

	// 处理：核对后在结算侧释放。提供方这一回受理（替身的拒收次数用完，释放到达真适配器）。
	handler := fixture.refusalHandlingHandler(t, release)
	result := fixture.handleRefusal(t, ctx, handler, "SYN-VER-01", psapplication.ReleaseControlReleaseRefusalAtSettlement)
	if result.Outcome() != psapplication.ControlReleaseRefusalHandled {
		t.Fatalf("处理 outcome = %q, want HANDLED", result.Outcome())
	}
	if got := fixture.freeze(t, ctx, firstVersion); got.status != 2 || got.releasedAt.IsZero() {
		t.Fatalf("首版冻结 = %+v, want 已释放", got)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.control_release_refusal_handling WHERE handling_kind = 'RELEASED_AT_SETTLEMENT'`); n != 1 {
		t.Fatalf("处理留痕 %d 行, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, string(psinbox.ControlReleaseRefusalHandledEventType)); n != 1 {
		t.Fatalf("续办信封 %d 封, want 恰好 1——消费侧认的类型串与发布侧写的不是同一个", n)
	}
	if got := fixture.freeze(t, ctx, newVersion); got.present {
		t.Fatalf("续办之前新版本冻结 = %+v, want 没有", got)
	}

	// 第五扇门：续办信封把链拿委托当前版本（新版本）再驱一拍。
	gate := fixture.refusalResumeGate(t)
	delivery := fixture.claimDeliveryOfType(t, string(psinbox.ControlReleaseRefusalHandledEventType))
	if err := gate.Consume(ctx, delivery.Envelope); err != nil {
		t.Fatalf("释放拒收处理续办一拍：%v", err)
	}
	if n := fixture.countInbox(t, refusalHandledResumeConsumerName, string(delivery.Envelope.ID)); n != 1 {
		t.Fatalf("续办门 inbox 行数 = %d, want 1", n)
	}
	if fixture.reachability.assessments != 2 || fixture.reachability.lastVersion.String() != synRSupplementVersionID {
		t.Fatalf("续办后可达性权威被问 %d 次（版本 %q）, want 2 次且是新版本 %s",
			fixture.reachability.assessments, fixture.reachability.lastVersion, synRSupplementVersionID)
	}
	if got := fixture.freeze(t, ctx, newVersion); !got.present || got.status != 1 || got.amount != synRSupersededFreezeMinor {
		t.Fatalf("新版本冻结 = %+v, want 已冻结 %d——新版本照常进入接受前财务控制", got, synRSupersededFreezeMinor)
	}
	if accepted := fixture.mustLoadRequest(t, ctx); accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}

	// 幂等：同一封重投账本跳过；绕过 inbox 再驱一拍，已决定的委托不再占下任何东西。
	if err := gate.Consume(ctx, delivery.Envelope); err != nil {
		t.Fatalf("重投续办信封：%v", err)
	}
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, err := fixture.chain.Handle(txCtx, psapplication.AdvanceAcceptanceChainCommand{
			Identity:          fixture.identity,
			ShipmentRequestID: fixture.requestID,
			SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, synRSupplementVersionID),
			DeclaredParcelIDs: []psdomain.DeclaredParcelID{mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01")},
		})
		return err
	})
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.funds_freeze`); n != 2 {
		t.Fatalf("冻结行数 = %d, want 2（两版各一笔）——再驱占下了多余的东西", n)
	}
	if n := fixture.countOutboxOfType(t, "parcel-shipment.shipment-request.accepted"); n > 1 {
		t.Fatalf("接受信封 %d 封, want 至多 1", n)
	}
}

// Covers: ADR-0293 决定二、四——标记已处理闭环：控制结果没存作用域的存量行，提供方一律拒收，「在结算侧释放」也放不掉（答再被拒收、什么也没写）；运维在结算侧
// 手工把旧冻结放掉之后标记已处理，处理留痕与续办信封同笔落地；第五扇门再驱时，链开头对旧版本再请求一次释放仍被拒收，但认出这一笔人已处理、接着判新版本，
// 新版本能判、接受成立。
func TestMarkingAnUnscopedRefusalHandledLetsTheChainJudgeTheNewVersion(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{refusalsLeft: 1}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter
	firstVersion := fixture.requestID.String() + "/SYN-VER-01"
	newVersion := fixture.requestID.String() + "/" + synRSupplementVersionID

	paused := fixture.pauseFirstVersion(t, ctx)
	listed := fixture.registeredRefusals(t, ctx)
	if len(listed) != 1 || !listed[0].Scope.Recorded() {
		t.Fatalf("暂停那一次被拒收，册上 = %+v, want 首版那一笔（带暂停时的作用域）", listed)
	}
	recordedScope := listed[0].Scope
	fixture.forgetOccupationScopes(t, ctx)
	fixture.stuckAfterSupplement(t, ctx, paused)

	handler := fixture.refusalHandlingHandler(t, release)
	again := fixture.handleRefusal(t, ctx, handler, "SYN-VER-01", psapplication.ReleaseControlReleaseRefusalAtSettlement)
	if again.Outcome() != psapplication.ControlReleaseRefusedAgain || again.Reason().String() != "SA-RELEASE_REQUEST_NOT_ACCEPTED" {
		t.Fatalf("没存作用域的存量行选释放 = %q（原因 %q）, want RELEASE_REFUSED_AGAIN 带提供方的原因", again.Outcome(), again.Reason())
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.control_release_refusal_handling`); n != 0 {
		t.Fatalf("释放再被拒收却留了 %d 行处理留痕", n)
	}
	if n := fixture.countOutboxOfType(t, string(psinbox.ControlReleaseRefusalHandledEventType)); n != 0 {
		t.Fatalf("释放再被拒收却交了 %d 封续办信封", n)
	}

	// 运维在结算侧手工放掉旧冻结（按暂停时的作用域认领），再回来标记已处理。
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		answer, err := adapter.ReleasePreAcceptanceControl(txCtx, psports.ControlReleaseRequest{
			Identity:          fixture.identity,
			ShipmentRequestID: fixture.requestID,
			SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, "SYN-VER-01"),
			ControlResultID:   mustPS(t, psdomain.NewFinancialControlResultID, firstVersion),
			Scope:             recordedScope,
		})
		if err != nil || answer.Outcome != psports.ControlReleaseSettled {
			t.Fatalf("结算侧手工释放 = %+v err=%v", answer, err)
		}
		return nil
	})
	marked := fixture.handleRefusal(t, ctx, handler, "SYN-VER-01", psapplication.MarkControlReleaseRefusalHandled)
	if marked.Outcome() != psapplication.ControlReleaseRefusalHandled {
		t.Fatalf("标记 outcome = %q, want HANDLED", marked.Outcome())
	}
	if n := fixture.countOutboxOfType(t, string(psinbox.ControlReleaseRefusalHandledEventType)); n != 1 {
		t.Fatalf("续办信封 %d 封, want 恰好 1", n)
	}

	gate := fixture.refusalResumeGate(t)
	delivery := fixture.claimDeliveryOfType(t, string(psinbox.ControlReleaseRefusalHandledEventType))
	if err := gate.Consume(ctx, delivery.Envelope); err != nil {
		t.Fatalf("续办一拍仍被已处理的拒收挡着：%v", err)
	}
	if got := fixture.freeze(t, ctx, newVersion); !got.present || got.status != 1 {
		t.Fatalf("新版本冻结 = %+v, want 已冻结", got)
	}
	if accepted := fixture.mustLoadRequest(t, ctx); accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
}

// Covers: ADR-0282 决定三不变——没有人处理的拒收，哪怕册上有这一行，链开头照旧响亮失败：续办信封只在处理之后才有，没有处理就没有信封，也没有别的东西替链放行。
func TestAnUnhandledRefusalStillBlocksTheChainOnEveryRedrive(t *testing.T) {
	control := &synRLateBoundControl{}
	release := &synRFlakyRelease{refusalsLeft: 1}
	fixture := newCustomerSupplementLoopFixtureWith(t, customerSupplementLoopLegs{control: control, release: release})
	ctx := t.Context()
	adapter := fixture.wireSettlement(t, ctx)
	control.inner, release.inner = adapter, adapter

	paused := fixture.pauseFirstVersion(t, ctx)
	fixture.forgetOccupationScopes(t, ctx)
	fixture.stuckAfterSupplement(t, ctx, paused)

	for range 2 {
		var handleErr error
		mustWithinTXAllowingError(t, fixture, ctx, func(txCtx context.Context) error {
			_, handleErr = fixture.chain.Handle(txCtx, psapplication.AdvanceAcceptanceChainCommand{
				Identity:          fixture.identity,
				ShipmentRequestID: fixture.requestID,
				SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, synRSupplementVersionID),
				DeclaredParcelIDs: []psdomain.DeclaredParcelID{mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01")},
			})
			return handleErr
		})
		if !errors.Is(handleErr, psapplication.ErrControlReleaseNotAccepted) {
			t.Fatalf("再驱 = %v, want ErrControlReleaseNotAccepted", handleErr)
		}
	}
	if fixture.reachability.assessments != 1 {
		t.Fatalf("可达性权威被问了 %d 次, want 仍为 1", fixture.reachability.assessments)
	}
}

// mustWithinTXAllowingError 在事务里跑 fn，fn 报错时回滚但不让用例失败：错误由 fn 自己带出去断言。
func mustWithinTXAllowingError(t *testing.T, fixture *customerSupplementLoopFixture, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	_ = fixture.transactor.WithinTransaction(ctx, fn)
}
