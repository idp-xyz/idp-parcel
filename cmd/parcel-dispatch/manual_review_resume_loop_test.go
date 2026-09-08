package main

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
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

// 本文件对真实 PostgreSQL 16 证 ADR-0086 的整条闭环（票 09）：规则要求人工复核时，
// 接受链在形成决定处暂停且暂停随本份投递入账（不重投）；队列读面按 0009 投影列把这份
// 委托列出来；复核完成随同一事务铸出「复核已完成」信封；第二扇消费门按这封信把同一条
// 链再驱一拍，判断重录折成重放，决定形成为接受，队列出列，重投不翻倍。
//
// 与 SYN-V0 的分工：SYN-V0 走生产 wireDispatcher，证「判断齐→形成决定→派发一拍」与
// 生产依赖图的诚实未决；本文件证的是链上多出来的那格暂停与那扇续办门。链不走生产装配
// ——生产的商业依据是真 PC 权威，实例半边空着时停在解析未决，走不到`要求人工复核`——
// 因此按 synS 先例在测试内装同一形状的链：仓储、判断账、Outbox/Inbox、两扇消费门与
// 三段编排全是生产实现，只有三个权威口（商业依据、可达性、财务控制）换成 synR 替身。
// synR 替身只出现在本文件，不进生产装配。

// synRAsOfAnchor 是两类判断时点的固定读数：时点由规则声明的策略形成，不随真实时钟漂。
// 续办一拍重跑两条腿时以同一时点重录，判断账的键（成员+时点）撞上 ON CONFLICT DO
// NOTHING，重录于是折成重放——时点要是取当下，续办就会长出第二行判断。
var synRAsOfAnchor = time.Date(2026, 8, 31, 8, 0, 0, 0, time.UTC)

type manualReviewLoopFixture struct {
	*synVerticalFixture
	store            *outbox.Store
	views            *pspostgres.ShipmentRequestViews
	submittedGate    *psinbox.ShipmentRequestSubmittedConsumer
	resumeGate       *psinbox.ManualReviewCompletedConsumer
	submittedHandoff *pspostgres.OutboxShipmentRequestSubmittedHandoff
	review           *psapplication.CompleteManualReviewHandler
}

func newManualReviewLoopFixture(t *testing.T) *manualReviewLoopFixture {
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
	commercial := &synRCommercialBasis{t: t}
	decision := psapplication.NewFormAcceptanceDecisionHandler(psapplication.FormAcceptanceDecisionDeps{
		Requests:     base.requests,
		Commercial:   commercial,
		Reachability: &synSReachabilityRevalidator{},
		Judgments:    base.judgments,
		Recorder:     base.judgments,
		Release:      &synSControlRelease{},
		Downstream:   decisionHandoff,
		Identities:   &synSDecisionIdentities{},
		Clock:        systemClock{},
	})
	chain := psapplication.NewAdvanceAcceptanceChainHandler(psapplication.AdvanceAcceptanceChainDeps{
		Reachability: psapplication.NewAdvanceAcceptanceJudgmentHandler(
			commercial, synRReachabilityAuthority{t: t}, base.judgments, base.requests, systemClock{}),
		FinancialControl: psapplication.NewAdvanceFinancialControlJudgmentHandler(
			commercial, synRFinancialAuthority{t: t}, base.judgments, base.requests, systemClock{}),
		Decision: decision,
	})
	submittedGate, err := psinbox.NewShipmentRequestSubmittedConsumer(base.transactor, inboxStore, chain)
	if err != nil {
		t.Fatalf("构造提交门：%v", err)
	}
	resumeGate, err := psinbox.NewManualReviewCompletedConsumer(base.transactor, inboxStore, chain)
	if err != nil {
		t.Fatalf("构造续办门：%v", err)
	}
	submittedHandoff, err := pspostgres.NewOutboxShipmentRequestSubmittedHandoff(base.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造提交交接：%v", err)
	}
	completionHandoff, err := pspostgres.NewOutboxManualReviewCompletedHandoff(base.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造复核完成交接：%v", err)
	}
	review := psapplication.NewCompleteManualReviewHandler(psapplication.CompleteManualReviewDeps{
		Requests: synRReviewBoundary{
			transactor: base.transactor,
			inner:      base.requests,
			handoff:    completionHandoff,
		},
		Authorizer: synRReviewAuthorizer{t: t},
		Clock:      systemClock{},
	})
	return &manualReviewLoopFixture{
		synVerticalFixture: base,
		store:              store,
		views:              views,
		submittedGate:      submittedGate,
		resumeGate:         resumeGate,
		submittedHandoff:   submittedHandoff,
		review:             review,
	}
}

// Covers: ADR-0086 Decision 一（暂停与等待态同事务落库、投递入账不重投）、队列读面
// 在列（迁移 0009 投影列 + 真读适配器）、Decision 二（完成与信封同事务、续办门把同
// 一条链再驱一拍直到接受）、重投由两本账各自跳过。
func TestAManualReviewPauseIsResumedByItsCompletionEnvelope(t *testing.T) {
	fixture := newManualReviewLoopFixture(t)
	ctx := t.Context()

	// 提交成立，并按生产发布路径铸「委托已提交」信封。
	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	// 第一扇门：链走完两条腿，在形成决定处撞上`要求人工复核`——暂停不是错误，本份
	// 投递按处理完毕入账。回滚重投推不动第三方，只会烧完失败预算再把等待态一起蒸发。
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	submitted := submittedDelivery.Envelope
	if err := fixture.submittedGate.Consume(ctx, submitted); err != nil {
		t.Fatalf("等待人工复核必须按处理完毕入账（ADR-0086）：%v", err)
	}
	// 消费者名是 inbox 账本的键，这里钉字面量：改名会让已入账的投递被当成没处理过。
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submitted.ID)); n != 1 {
		t.Fatalf("提交门 inbox 行数 = %d, want 1——暂停没有随本份投递入账", n)
	}

	paused := fixture.mustLoadRequest(t, ctx)
	if paused.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("暂停后状态 = %q, want SUBMITTED——等待复核不是接受也不是拒绝", paused.State())
	}
	waiting, present := paused.AcceptanceDecisionTask().WaitingOn()
	if !present || waiting != psdomain.ResumeByManualReview {
		t.Fatalf("等待态 = %v（present=%v）, want MANUAL_REVIEW——暂停没落库，队列列不出它", waiting, present)
	}

	// 队列读面：真适配器按 0009 投影列上列，复核留痕此刻应为空。
	queue := fixture.listReviewQueue(t, ctx)
	if len(queue) != 1 || queue[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("复核队列 = %+v, want 恰好这一份委托", queue)
	}
	if queue[0].ReviewCompleted {
		t.Fatal("还没人复核，队列行却带着完成留痕")
	}

	// 决定未形成：不得有接受决定信封。
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 0 {
		t.Fatalf("暂停期间入队了 %d 封接受信封", n)
	}

	// 同一封重投：账本跳过，链不重跑，决定照旧未形成。
	if err := fixture.submittedGate.Consume(ctx, submitted); err != nil {
		t.Fatalf("重投已入账的投递：%v", err)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 0 {
		t.Fatalf("重投后入队了 %d 封接受信封", n)
	}

	// 提交信封定稿（派发一拍发布后的同一动作）：续办信封与它同分区排队（同一份委托的
	// 提交与续办不越序），头一封不定稿，认领就轮不到后面那封。
	fixture.markPublished(t, submittedDelivery)

	// 完成复核：完成落库与「复核已完成」信封同一事务（边界壳形状照 cmd/parcel-api 的
	// reviewCompletionBoundary，生产壳由那边的装配用例取证）。
	completed := fixture.completeReview(t, ctx, paused)
	if completed.Outcome() != psapplication.ManualReviewCompletionRecorded {
		t.Fatalf("复核完成 outcome = %q, want RECORDED", completed.Outcome())
	}
	if n := fixture.countOutboxOfType(t, string(psinbox.ManualReviewCompletedEventType)); n != 1 {
		t.Fatalf("「复核已完成」信封 = %d 封, want 恰好 1", n)
	}

	// 第二扇门：续办信封把同一条链再驱一拍——两条腿以同一时点重录折成重放，形成决定
	// 看到复核已完成，接受成立并入队恰好一封接受决定信封。
	resume := fixture.claimDeliveryOfType(t, string(psinbox.ManualReviewCompletedEventType)).Envelope
	if err := fixture.resumeGate.Consume(ctx, resume); err != nil {
		t.Fatalf("续办一拍：%v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain-on-review-completion", string(resume.ID)); n != 1 {
		t.Fatalf("续办门 inbox 行数 = %d, want 1", n)
	}

	accepted := fixture.mustLoadRequest(t, ctx)
	if accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
	if decision, formed := accepted.AcceptanceDecision(); !formed || decision.DecisionID().String() != synV0DecisionID {
		t.Fatalf("决定 = %v formed = %v, want %s", decision, formed, synV0DecisionID)
	}
	if remaining := fixture.listReviewQueue(t, ctx); len(remaining) != 0 {
		t.Fatalf("接受之后队列仍列出 %d 行——等待态没随决定清掉", len(remaining))
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("接受信封 = %d 封, want 恰好 1", n)
	}

	// 续办信封重投：账本跳过，接受不翻倍。
	if err := fixture.resumeGate.Consume(ctx, resume); err != nil {
		t.Fatalf("重投续办信封：%v", err)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("重投后接受信封 = %d 封, want 仍为 1", n)
	}
}

func (fixture *manualReviewLoopFixture) completeReview(
	t *testing.T,
	ctx context.Context,
	request psdomain.ShipmentRequest,
) psapplication.CompleteManualReviewResult {
	t.Helper()

	result, err := fixture.review.Handle(ctx, psapplication.CompleteManualReviewCommand{
		Identity:          fixture.identity,
		ShipmentRequestID: fixture.requestID,
		SubmissionVersion: request.CurrentSubmissionVersion().VersionID(),
		Reviewer:          mustPS(t, psdomain.NewReviewerReference, "SYN-REVIEWER-01"),
		Evidence:          mustPS(t, psdomain.NewReviewEvidenceReference, "SYN-REV-EVIDENCE-01"),
	})
	if err != nil {
		t.Fatalf("完成复核：%v", err)
	}
	return result
}

// synRReviewAuthorizer 是第四个换成替身的权威口：复核授权一律答`已授权`并交回一个合成的授权规则
// 版本引用。本文件取证的是暂停→续办→接受这条链的形状，「谁有权复核」真接到 PC 授权册的证据在
// cmd/parcel-api 的 assemble_review_test（那里种真 grant、走生产适配器）。只出现在本文件，不进
// 生产装配。
type synRReviewAuthorizer struct{ t *testing.T }

func (authorizer synRReviewAuthorizer) AuthorizeManualReview(
	_ context.Context,
	_ psports.ManualReviewAuthorizationQuery,
) (psports.ManualReviewAuthorization, error) {
	authorizer.t.Helper()
	return psports.ManualReviewAuthorization{
		Outcome:   psports.AuthorizationGranted,
		Authority: mustPS(authorizer.t, psdomain.NewReviewAuthorityReference, "SYN-REV-AUTH-01"),
	}, nil
}

var _ psports.ManualReviewAuthorizer = synRReviewAuthorizer{}

func (fixture *manualReviewLoopFixture) listReviewQueue(
	t *testing.T,
	ctx context.Context,
) []psports.AcceptanceReviewQueueRecord {
	t.Helper()

	scope, err := psdomain.NewAuthorizedQueryScope(
		mustPS(t, psdomain.NewQueryScopeReference, "SYN-QS-01"),
		fixture.identity.TenantID(),
		[]psdomain.CustomerAccountID{fixture.identity.CustomerAccountID()},
	)
	if err != nil {
		t.Fatalf("作用域：%v", err)
	}
	records, err := fixture.views.ListAwaitingManualReview(ctx, scope, 10)
	if err != nil {
		t.Fatalf("列复核队列：%v", err)
	}
	return records
}

// claimDeliveryOfType 按派发一拍的同一条认领路径把指定类型的信封取回来（手法照
// cmd/parcel-api 的 claimSubmittedEnvelope）：要喂给门的正是「库里那一封」，自己拼
// 等于把发布侧那半换成手抄本。交回整份投递，调用方定稿时要用它的 Ref。
func (fixture *manualReviewLoopFixture) claimDeliveryOfType(t *testing.T, eventType string) eventing.Delivery {
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
		if string(delivery.Envelope.Type) == eventType {
			return delivery
		}
	}
	t.Fatalf("认领到 %d 封，其中没有 %s", len(deliveries), eventType)
	return eventing.Delivery{}
}

func (fixture *manualReviewLoopFixture) markPublished(t *testing.T, delivery eventing.Delivery) {
	t.Helper()
	if err := fixture.store.MarkPublished(t.Context(), delivery.Ref, time.Now().UTC()); err != nil {
		t.Fatalf("定稿投递 %s：%v", delivery.Envelope.ID, err)
	}
}

// synRReviewBoundary 是复核完成的事务边界壳（隔离合成 `S`）：完成落库与「复核已完成」
// 信封同一事务。形状照 cmd/parcel-api 的 reviewCompletionBoundary——生产壳在装配包里
// 不可导入，这里要的只是「完成落库即有信封」这一格，生产壳本身由那边的装配用例取证。
type synRReviewBoundary struct {
	transactor bentoapp.Transactor
	inner      psports.ShipmentRequestRepository
	handoff    psports.ManualReviewCompletedHandoff
}

var _ psports.ShipmentRequestRepository = synRReviewBoundary{}

func (boundary synRReviewBoundary) FindBySourceIdentity(
	ctx context.Context,
	identity psdomain.SourceIdentity,
) (psdomain.ShipmentRequest, bool, error) {
	return boundary.inner.FindBySourceIdentity(ctx, identity)
}

func (boundary synRReviewBoundary) Insert(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	request psdomain.ShipmentRequest,
) (psports.ShipmentRequestInsertOutcome, error) {
	var outcome psports.ShipmentRequestInsertOutcome
	err := boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		inserted, err := boundary.inner.Insert(txCtx, identity, request)
		if err != nil {
			return err
		}
		outcome = inserted
		return nil
	})
	if err != nil {
		return psports.ShipmentRequestInsertOutcomeInvalid, err
	}
	return outcome, nil
}

func (boundary synRReviewBoundary) Save(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	request psdomain.ShipmentRequest,
) (psports.ShipmentRequestSaveOutcome, error) {
	var outcome psports.ShipmentRequestSaveOutcome
	err := boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, err := boundary.inner.Save(txCtx, identity, request)
		if err != nil {
			return err
		}
		outcome = saved
		if saved != psports.ShipmentRequestSaved {
			return nil
		}
		return boundary.handoff.HandOffManualReviewCompleted(txCtx,
			psports.ManualReviewCompletedHandoffIntent{Identity: identity, Request: request})
	})
	if err != nil {
		return psports.ShipmentRequestSaveOutcomeInvalid, err
	}
	return outcome, nil
}

// synRCommercialBasis 是本文件的商业依据替身（隔离合成 `S`，不进生产装配）。与 SYN-V0
// 那份的差别只有两处：规则声明`要求人工复核`（正是要证的那格暂停），且第二阶段真的按
// 声明形成判断时点——本文件的链要自己走完两条腿，SYN-V0 那份答零值就够，因为它预录
// 判断、绕开腿。
type synRCommercialBasis struct{ t *testing.T }

func (double *synRCommercialBasis) ResolveCommercialBasis(
	_ context.Context,
	_ psports.CommercialBasisQuery,
) (psports.CommercialBasisResolution, error) {
	return double.applicableSnapshot()
}

func (double *synRCommercialBasis) FormJudgmentAsOf(
	_ context.Context,
	query psports.JudgmentAsOfQuery,
) (psports.JudgmentAsOfFormation, error) {
	return psports.JudgmentAsOfFormation{
		Outcome: psports.JudgmentAsOfFormed,
		AsOf:    synJudgmentAsOf(double.t, query.Declared.Kind(), synRAsOfAnchor),
	}, nil
}

func (double *synRCommercialBasis) RevalidateCommercialBasis(
	ctx context.Context,
	query psports.CommercialRevalidationQuery,
) (psports.CommercialRevalidation, error) {
	resolution, err := double.ResolveCommercialBasis(ctx, psports.CommercialBasisQuery{
		Identity:          query.Identity,
		ShipmentRequestID: query.ShipmentRequestID,
		SubmissionVersion: query.SubmissionVersion,
	})
	if err != nil {
		return psports.CommercialRevalidation{}, err
	}
	return psports.CommercialRevalidation{
		Outcome:    psports.CommercialBasisStillValid,
		Resolution: resolution,
	}, nil
}

func (double *synRCommercialBasis) applicableSnapshot() (psports.CommercialBasisResolution, error) {
	double.t.Helper()
	groups, err := psdomain.NewApplicableCheckGroups(
		psdomain.PreAcceptanceFinancialControlCheck,
		psdomain.NetworkReachabilityCheck,
	)
	if err != nil {
		double.t.Fatalf("适用校验组：%v", err)
	}
	snapshot, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID: mustPS(double.t, psdomain.NewCommercialResolutionID, "SYN-RES-R1"),
		RulePackage:  mustPS(double.t, psdomain.NewRulePackageReference, "SYN-RULES-R1/v1"),
		ViewRevision: mustPS(double.t, psdomain.NewCommercialViewRevision, "SYN-VIEW-R1"),
		DeclaredAsOf: []psdomain.DeclaredAsOf{
			synDeclaredAsOf(double.t, psdomain.ReachabilityJudgmentKind),
			synDeclaredAsOf(double.t, psdomain.FinancialControlJudgmentKind),
		},
		Applicable:   groups,
		ManualReview: psdomain.ManualReviewRequiredByRules,
	})
	if err != nil {
		double.t.Fatalf("商业依据快照：%v", err)
	}
	return psports.CommercialBasisResolution{
		Snapshot:      snapshot,
		Applicability: psdomain.CommerciallyApplicable,
	}, nil
}

// synRReachabilityAuthority 是 network-routing 权威替身（隔离合成 `S`）：按请求交回的
// 时点答`可达`。判断标识按成员派生、时点原样回签——续办一拍以同一键重录，判断账折成
// 重放。
type synRReachabilityAuthority struct{ t *testing.T }

func (double synRReachabilityAuthority) AssessParcelReachability(
	_ context.Context,
	request psports.ReachabilityRequest,
) (psports.ReachabilityAssessment, error) {
	double.t.Helper()
	judgment, err := psdomain.NewReachabilityJudgment(psdomain.ReachabilityJudgmentSpec{
		JudgmentID: mustPS(double.t, psdomain.NewReachabilityJudgmentID, "SYN-NRJ-R-"+request.DeclaredParcelID.String()),
		ParcelID:   request.DeclaredParcelID,
		Value:      psdomain.ReachabilityReachable,
		AsOf:       request.AsOf,
	})
	if err != nil {
		double.t.Fatalf("可达性判断：%v", err)
	}
	return psports.ReachabilityAssessment{
		Outcome:  psports.ReachabilityAssessed,
		Judgment: judgment,
	}, nil
}

// synRFinancialAuthority 是 settlement-accounting 权威替身（隔离合成 `S`）：按请求交回
// 的时点答`已冻结`。
type synRFinancialAuthority struct{ t *testing.T }

func (double synRFinancialAuthority) ApplyPreAcceptanceFinancialControl(
	_ context.Context,
	request psports.FinancialControlRequest,
) (psports.PreAcceptanceControlAssessment, error) {
	double.t.Helper()
	result := synHeldControl(double.t, "SYN-SAC-R1", request.AsOf)
	return psports.PreAcceptanceControlAssessment{
		Outcome: psports.PreAcceptanceControlFormed,
		Result:  result,
	}, nil
}

var (
	_ psports.CommercialBasisResolver          = (*synRCommercialBasis)(nil)
	_ psports.ReachabilityAssessor             = synRReachabilityAuthority{}
	_ psports.PreAcceptanceFinancialController = synRFinancialAuthority{}
)
