package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ADR-0132 的整条闭环（票 sa-preacceptance-policy-view/04 场景 1、3）：策略正文把
// 不通过的控制交给授权角色时，接受链在形成控制判断那一步读回处置并采用到受限项上，形成决定处停在`等待授权处置`
// 且暂停随本份投递入账（不重投）；队列读面按 0021 投影列把这份委托连受限项一起列出来；处置命令两去向各收口
// 一次——`交客户补充`转到`等待受控补充`、`拒绝`形成授权角色拒绝决定——两者都释放本版本已成立项的占用，不发信封。
//
// 链的装法沿 manual_review_resume_loop_test：仓储、判断账、Outbox/Inbox、消费门与编排全是生产实现，只有三个权威口
// 与处置读口 / 处置授权口换成 synR 替身。synR 替身只出现在测试里，不进生产装配。

type dispositionLoopFixture struct {
	*synVerticalFixture
	store            *outbox.Store
	views            *pspostgres.ShipmentRequestViews
	submittedGate    *psinbox.ShipmentRequestSubmittedConsumer
	submittedHandoff *pspostgres.OutboxShipmentRequestSubmittedHandoff
	dispose          *psapplication.DisposeShipmentRequestHandler
	release          *synRRecordingRelease
}

func newDispositionLoopFixture(t *testing.T) *dispositionLoopFixture {
	t.Helper()

	base := newSYNVerticalFixture(t)
	store, err := outbox.NewStore(base.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inboxStore, err := inbox.NewStore(base.db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	views, err := pspostgres.NewShipmentRequestViews(base.db)
	if err != nil {
		t.Fatalf("构造委托查阅读口：%v", err)
	}
	decisionHandoff, err := pspostgres.NewOutboxAcceptanceDecisionHandoff(base.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造接受决定交接：%v", err)
	}
	release := &synRRecordingRelease{}
	commercial := &synRCommercialBasis{t: t}
	decision := psapplication.NewFormAcceptanceDecisionHandler(psapplication.FormAcceptanceDecisionDeps{
		Requests:     base.requests,
		Commercial:   commercial,
		Reachability: &synSReachabilityRevalidator{},
		Judgments:    base.judgments,
		Recorder:     base.judgments,
		Release:      release,
		Downstream:   decisionHandoff,
		Identities:   &synSDecisionIdentities{},
		Clock:        systemClock{},
	})
	chain := psapplication.NewAdvanceAcceptanceChainHandler(psapplication.AdvanceAcceptanceChainDeps{
		Reachability: psapplication.NewAdvanceAcceptanceJudgmentHandler(
			commercial, synRReachabilityAuthority{t: t}, base.judgments, base.requests, systemClock{}),
		FinancialControl: psapplication.NewAdvanceFinancialControlJudgmentHandler(
			commercial, synRRestrictedFinancialAuthority{t: t}, synRAuthorizedControlDispositions{t: t},
			base.judgments, base.requests, systemClock{}),
		Decision: decision,
	})
	submittedGate, err := psinbox.NewShipmentRequestSubmittedConsumer(base.transactor, inboxStore, chain)
	if err != nil {
		t.Fatalf("构造提交门：%v", err)
	}
	submittedHandoff, err := pspostgres.NewOutboxShipmentRequestSubmittedHandoff(base.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造提交交接：%v", err)
	}
	dispose := psapplication.NewDisposeShipmentRequestHandler(psapplication.DisposeShipmentRequestDeps{
		Requests:   base.requests,
		Authorizer: synRDispositionAuthorizer{t: t},
		Judgments:  base.judgments,
		Release:    release,
		Identities: &synSDecisionIdentities{},
		Clock:      systemClock{},
	})
	return &dispositionLoopFixture{
		synVerticalFixture: base,
		store:              store,
		views:              views,
		submittedGate:      submittedGate,
		submittedHandoff:   submittedHandoff,
		dispose:            dispose,
		release:            release,
	}
}

// pauseAtDisposition 走到`等待授权处置`并入账：提交成立、铸「委托已提交」信封、第一扇门跑完两条腿在形成决定处
// 停下。交回读回的暂停态委托。
func (fixture *dispositionLoopFixture) pauseAtDisposition(t *testing.T, ctx context.Context) psdomain.ShipmentRequest {
	t.Helper()

	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	submitted := fixture.claimSubmittedDelivery(t)
	if err := fixture.submittedGate.Consume(ctx, submitted); err != nil {
		t.Fatalf("等待授权处置必须按处理完毕入账（ADR-0132 决定二）：%v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submitted.ID)); n != 1 {
		t.Fatalf("提交门 inbox 行数 = %d, want 1——暂停没有随本份投递入账", n)
	}

	paused := fixture.mustLoadRequest(t, ctx)
	if paused.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("暂停后状态 = %q, want SUBMITTED——等处置不是接受也不是拒绝", paused.State())
	}
	waiting, present := paused.AcceptanceDecisionTask().WaitingOn()
	if !present || waiting != psdomain.ResumeByAuthorizedDisposition {
		t.Fatalf("等待态 = %v（present=%v）, want AUTHORIZED_DISPOSITION——正文登了授权处置的受限控制被自动拒绝或没落库", waiting, present)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 0 {
		t.Fatalf("暂停期间入队了 %d 封接受信封", n)
	}
	// 同一封重投：账本跳过，链不重跑。
	if err := fixture.submittedGate.Consume(ctx, submitted); err != nil {
		t.Fatalf("重投已入账的投递：%v", err)
	}
	return paused
}

// Covers: 场景 1 + `交客户补充`——受限项采用的处置随控制结果落库并在队列读面透出；处置角色选`交客户补充`后
// 等待态转到`等待受控补充`、委托仍`已提交`、不形成决定、本版本已成立的冻结按原关联释放、授权处置队列出列而
// 受控补充队列在列；不发任何信封。
func TestARestrictedRequestPausesForDispositionAndIsHandedToTheCustomer(t *testing.T) {
	fixture := newDispositionLoopFixture(t)
	ctx := t.Context()

	paused := fixture.pauseAtDisposition(t, ctx)

	recorded, err := fixture.judgments.LoadRecordedJudgments(ctx, fixture.identity.TenantID(), fixture.requestID,
		paused.CurrentSubmissionVersion().VersionID())
	if err != nil {
		t.Fatalf("读回已记录判断：%v", err)
	}
	if !recorded.FinancialControl.AwaitsAuthorizedDisposition() {
		t.Fatal("落库的受限结果没带上正文登记的`进入授权处置`")
	}

	queue := fixture.listDispositionQueue(t, ctx)
	if len(queue) != 1 || queue[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("授权处置队列 = %+v, want 恰好这一份委托", queue)
	}
	if queue[0].ControlResultID.String() != synRRestrictedResultID || len(queue[0].RestrictedItems) != 1 ||
		queue[0].RestrictedItems[0].Kind != psdomain.CreditCheckControlItem ||
		queue[0].RestrictedItems[0].FailureDisposition != psdomain.AuthorizedDispositionOnControlFailure ||
		queue[0].RestrictedItems[0].Responsibility.String() != synRResponsibility {
		t.Fatalf("队列行受限项 = %+v", queue[0])
	}

	result := fixture.disposeWithin(t, ctx, psdomain.DisposeByCustomerSupplement)
	if result.Outcome() != psapplication.AuthorizedDispositionRecorded {
		t.Fatalf("处置 outcome = %q, want RECORDED", result.Outcome())
	}
	handed := fixture.mustLoadRequest(t, ctx)
	if handed.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("交客户补充后状态 = %q, want SUBMITTED", handed.State())
	}
	if _, formed := handed.AcceptanceDecision(); formed {
		t.Fatal("交客户补充形成了一份决定")
	}
	waiting, present := handed.AcceptanceDecisionTask().WaitingOn()
	if !present || waiting != psdomain.ResumeByCustomerSupplement {
		t.Fatalf("等待态 = %v（present=%v）, want CUSTOMER_SUPPLEMENT", waiting, present)
	}
	if disposition, recorded := handed.AcceptanceDecisionTask().AuthorizedDisposition(); !recorded ||
		disposition.Authority().String() != synRDispositionAuthority {
		t.Fatalf("处置记录 = %+v（recorded=%v）", disposition, recorded)
	}
	if fixture.release.calls != 1 || fixture.release.resultID != synRRestrictedResultID {
		t.Fatalf("release calls = %d id = %q；这一版不会再被判，第一项占下的冻结要按原关联释放",
			fixture.release.calls, fixture.release.resultID)
	}
	if remaining := fixture.listDispositionQueue(t, ctx); len(remaining) != 0 {
		t.Fatalf("处置之后授权处置队列仍列出 %d 行", len(remaining))
	}
	supplement, err := fixture.views.ListWaitingOnCustomerSupplement(ctx, fixture.queueScope(t), 10)
	if err != nil {
		t.Fatalf("查受控补充队列：%v", err)
	}
	if len(supplement) != 1 || supplement[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("受控补充队列 = %+v, want 恰好这一份委托", supplement)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 0 {
		t.Fatalf("处置入队了 %d 封接受信封", n)
	}
}

// Covers: 场景 3 + `拒绝`——处置角色选`拒绝`后当场形成授权角色拒绝决定（实际决定方 = 处置方、授权引用来自
// 处置授权口），委托转`已拒绝`、本版本已成立的冻结释放、队列出列；不发信封（ADR-0086 决定三的保留条款）。
func TestARestrictedRequestPausesForDispositionAndIsRejectedByAuthority(t *testing.T) {
	fixture := newDispositionLoopFixture(t)
	ctx := t.Context()

	fixture.pauseAtDisposition(t, ctx)

	result := fixture.disposeWithin(t, ctx, psdomain.DisposeByRejection)
	if result.Outcome() != psapplication.AuthorizedDispositionRecorded {
		t.Fatalf("处置 outcome = %q, want RECORDED", result.Outcome())
	}
	if result.State() != psdomain.ShipmentRequestRejected {
		t.Fatalf("拒绝后状态 = %q, want REJECTED", result.State())
	}
	decision, formed := result.AcceptanceDecision()
	if !formed || decision.Accepted() || decision.DecisionID().String() != synV0DecisionID {
		t.Fatalf("决定 = %+v formed = %v, want the rejection %s", decision, formed, synV0DecisionID)
	}
	rejection, active := decision.ActiveRejection()
	if !active || rejection.Decider().String() != synRDisposer || rejection.Authority().String() != synRDispositionAuthority {
		t.Fatalf("拒绝留痕 = %+v（active=%v）", rejection, active)
	}
	if fixture.release.calls != 1 || fixture.release.resultID != synRRestrictedResultID {
		t.Fatalf("release calls = %d id = %q；拒绝即接受确定未成立，第一项占下的冻结要释放",
			fixture.release.calls, fixture.release.resultID)
	}
	if remaining := fixture.listDispositionQueue(t, ctx); len(remaining) != 0 {
		t.Fatalf("拒绝之后授权处置队列仍列出 %d 行", len(remaining))
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 0 {
		t.Fatalf("拒绝入队了 %d 封接受信封", n)
	}
	// 库里那一行确实是已拒绝：读回那一半由重建门挡在 ADR-0030 门外，这里只看投影。
	var state, waitingOn int16
	if err := fixture.pool.QueryRow(ctx,
		`SELECT state, task_waiting_on FROM parcel_shipment.shipment_request WHERE shipment_request_id = $1`,
		fixture.requestID.String()).Scan(&state, &waitingOn); err != nil {
		t.Fatalf("读状态投影：%v", err)
	}
	if state != int16(psdomain.ShipmentRequestRejected) || waitingOn != 0 {
		t.Fatalf("库里 state = %d waiting = %d, want REJECTED 且等待态清零", state, waitingOn)
	}
}

func (fixture *dispositionLoopFixture) disposeWithin(
	t *testing.T,
	ctx context.Context,
	choice psdomain.AuthorizedDispositionChoice,
) psapplication.DisposeShipmentRequestResult {
	t.Helper()
	request := fixture.mustLoadRequest(t, ctx)
	var result psapplication.DisposeShipmentRequestResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		handled, err := fixture.dispose.Handle(txCtx, psapplication.DisposeShipmentRequestCommand{
			Identity:          fixture.identity,
			ShipmentRequestID: fixture.requestID,
			SubmissionVersion: request.CurrentSubmissionVersion().VersionID(),
			Disposer:          mustPS(t, psdomain.NewDisposerReference, synRDisposer),
			Choice:            choice,
			Reason:            mustPS(t, psdomain.NewDispositionReasonReference, "SYN-DISPOSITION-REASON-01"),
			Evidence:          mustPS(t, psdomain.NewDispositionEvidenceReference, "SYN-DISPOSITION-EVIDENCE-01"),
		})
		if err != nil {
			return err
		}
		result = handled
		return nil
	})
	return result
}

func (fixture *dispositionLoopFixture) queueScope(t *testing.T) psdomain.AuthorizedQueryScope {
	t.Helper()
	scope, err := psdomain.NewAuthorizedQueryScope(
		mustPS(t, psdomain.NewQueryScopeReference, "SYN-QS-01"),
		fixture.identity.TenantID(),
		[]psdomain.CustomerAccountID{fixture.identity.CustomerAccountID()},
	)
	if err != nil {
		t.Fatalf("作用域：%v", err)
	}
	return scope
}

func (fixture *dispositionLoopFixture) listDispositionQueue(
	t *testing.T,
	ctx context.Context,
) []psports.AuthorizedDispositionQueueRecord {
	t.Helper()
	rows, err := fixture.views.ListAwaitingAuthorizedDisposition(ctx, fixture.queueScope(t), 10)
	if err != nil {
		t.Fatalf("查授权处置队列：%v", err)
	}
	return rows
}

func (fixture *dispositionLoopFixture) claimSubmittedDelivery(t *testing.T) eventing.Envelope {
	t.Helper()
	deliveries, err := fixture.store.Claim(t.Context(), eventing.OutboxClaim{
		Now:         time.Now().UTC().Add(time.Minute),
		Limit:       20,
		LeaseFor:    time.Minute,
		MaxAttempts: 50,
	})
	if err != nil {
		t.Fatalf("认领待发信封：%v", err)
	}
	for _, delivery := range deliveries {
		if string(delivery.Envelope.Type) == string(psinbox.ShipmentRequestSubmittedEventType) {
			return delivery.Envelope
		}
	}
	t.Fatalf("认领到 %d 封，其中没有 %s", len(deliveries), psinbox.ShipmentRequestSubmittedEventType)
	return eventing.Envelope{}
}

const (
	synRRestrictedResultID   = "SYN-SAC-D1"
	synRResponsibility       = "SYN-RESPONSIBILITY-01"
	synRDispositionAuthority = "SYN-DISPOSE-AUTH-01"
	synRDisposer             = "SYN-CREDIT-OFFICER-01"
)

// synRRestrictedFinancialAuthority 是 settlement-accounting 权威替身：预付冻结成立、信用受限——结论 RESTRICTED
// 而第一项占着钱，正是两去向都要释放的那笔。
type synRRestrictedFinancialAuthority struct{ t *testing.T }

func (double synRRestrictedFinancialAuthority) ApplyPreAcceptanceFinancialControl(
	_ context.Context,
	request psports.FinancialControlRequest,
) (psports.PreAcceptanceControlAssessment, error) {
	double.t.Helper()
	freeze, err := psdomain.NewControlItemResult(
		psdomain.PrepaidFreezeControlItem, 1, psdomain.ControlItemSatisfied, psdomain.ControlBasisReference{})
	if err != nil {
		double.t.Fatalf("冻结项：%v", err)
	}
	credit, err := psdomain.NewControlItemResult(
		psdomain.CreditCheckControlItem, 2, psdomain.ControlItemRestricted,
		mustPS(double.t, psdomain.NewControlBasisReference, "SYN-CREDIT-INSUFFICIENT"))
	if err != nil {
		double.t.Fatalf("信用项：%v", err)
	}
	result, err := psdomain.NewExecutedFinancialControlResult(psdomain.ExecutedFinancialControlSpec{
		ResultID:  mustPS(double.t, psdomain.NewFinancialControlResultID, synRRestrictedResultID),
		Items:     []psdomain.ControlItemResult{freeze, credit},
		JointPass: psdomain.AllControlsPass,
		AsOf:      request.AsOf,
	})
	if err != nil {
		double.t.Fatalf("受限控制结果：%v", err)
	}
	return psports.PreAcceptanceControlAssessment{Outcome: psports.PreAcceptanceControlFormed, Result: result}, nil
}

// synRAuthorizedControlDispositions 是处置读口替身：正文为信用校验登记`进入授权处置`。
type synRAuthorizedControlDispositions struct{ t *testing.T }

func (double synRAuthorizedControlDispositions) LoadControlDispositions(
	context.Context,
	psports.ControlDispositionQuery,
) (map[psdomain.ControlItemKind]psdomain.AdoptedControlDisposition, bool, error) {
	double.t.Helper()
	adopted, err := psdomain.NewAdoptedControlDisposition(
		psdomain.AuthorizedDispositionOnControlFailure,
		mustPS(double.t, psdomain.NewControlResponsibilityReference, synRResponsibility))
	if err != nil {
		double.t.Fatalf("采用引用：%v", err)
	}
	return map[psdomain.ControlItemKind]psdomain.AdoptedControlDisposition{psdomain.CreditCheckControlItem: adopted}, true, nil
}

// synRDispositionAuthorizer 是处置授权口替身：一律已授权并签发固定的授权引用。
type synRDispositionAuthorizer struct{ t *testing.T }

func (double synRDispositionAuthorizer) AuthorizeDisposition(
	context.Context,
	psports.AuthorizedDispositionAuthorizationQuery,
) (psports.AuthorizedDispositionAuthorization, error) {
	double.t.Helper()
	return psports.AuthorizedDispositionAuthorization{
		Outcome:   psports.AuthorizationGranted,
		Authority: mustPS(double.t, psdomain.NewDispositionAuthorityReference, synRDispositionAuthority),
	}, nil
}

// synRRecordingRelease 记下释放被叫了几次、认领的是哪一笔。
type synRRecordingRelease struct {
	calls    int
	resultID string
}

func (release *synRRecordingRelease) ReleasePreAcceptanceControl(
	_ context.Context,
	request psports.ControlReleaseRequest,
) error {
	release.calls++
	release.resultID = request.ControlResultID.String()
	return nil
}

var (
	_ psports.PreAcceptanceFinancialController = synRRestrictedFinancialAuthority{}
	_ psports.ControlDispositionView           = synRAuthorizedControlDispositions{}
	_ psports.AuthorizedDispositionAuthorizer  = synRDispositionAuthorizer{}
	_ psports.PreAcceptanceControlRelease      = (*synRRecordingRelease)(nil)
)
