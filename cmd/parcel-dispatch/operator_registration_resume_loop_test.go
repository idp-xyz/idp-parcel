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
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证 ADR-0094 Decision 四的整条闭环（票 first-tenant-runway/07 第二
// 笔）：时点策略未配置时，接受链在可达性那条腿停成`判断时点未配置`，D5 把`等待运营登记`Save 进
// 聚合，提交门按入账处置（不再回滚重投烧预算）；队列读口按 0013 的部分索引把这份委托列出来；
// party-commercial 侧登记动作用**真交接适配器**铸出「参数已登记」信封（契约两侧各写各的字面，
// 这里就是对得上对不上的那一格）；第三扇门按租户取回队列，把同一条链再驱一拍直到接受；重投
// 与再来一封登记信封都不翻倍。
//
// 与人工复核那条闭环的分工：那条证的是链上`要求人工复核`那格暂停与复核完成信封；本条证的是
// 决定之前的`等待运营登记`那格与跨上下文的登记信封。链同样按 synR 先例在测试内装同一形状：
// 仓储、判断账、Outbox/Inbox、消费门与三段编排全是生产实现，只有三个权威口换成替身，其中商业
// 依据替身的时点形成可以拨到`未配置`——那正是要证的那格停顿。

type operatorRegistrationLoopFixture struct {
	*synVerticalFixture
	store            *outbox.Store
	commercial       *synRSwitchableAsOfCommercialBasis
	submittedGate    *psinbox.ShipmentRequestSubmittedConsumer
	registrationGate *psinbox.OperatorRegistrationCompletedConsumer
	submittedHandoff *pspostgres.OutboxShipmentRequestSubmittedHandoff
	registration     *pcpostgres.OutboxOperatorRegistrationCompletedHandoff
}

const operatorRegistrationResumeConsumer = "parcel-shipment/advance-acceptance-chain-on-operator-registration"

func newOperatorRegistrationLoopFixture(t *testing.T) *operatorRegistrationLoopFixture {
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
	decisionHandoff, err := pspostgres.NewOutboxAcceptanceDecisionHandoff(base.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造接受决定交接：%v", err)
	}
	commercial := &synRSwitchableAsOfCommercialBasis{inner: &synRCommercialBasis{t: t}, t: t}
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
	// 队列读口装同一个 ShipmentRequests，与生产装配点同一手法。
	registrationGate, err := psinbox.NewOperatorRegistrationCompletedConsumer(
		base.transactor, inboxStore, base.requests, chain, operatorRegistrationRedrivePageSize)
	if err != nil {
		t.Fatalf("构造登记续办门：%v", err)
	}
	submittedHandoff, err := pspostgres.NewOutboxShipmentRequestSubmittedHandoff(base.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造提交交接：%v", err)
	}
	// 发信封的一侧用 party-commercial 的真适配器：跨上下文契约两边各写各的字面，只有让真发布侧
	// 铸的那一封穿过真消费门，「两串对得上、载荷两键译得出」才算验过。
	registration, err := pcpostgres.NewOutboxOperatorRegistrationCompletedHandoff(base.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造登记交接：%v", err)
	}
	return &operatorRegistrationLoopFixture{
		synVerticalFixture: base,
		store:              store,
		commercial:         commercial,
		submittedGate:      submittedGate,
		registrationGate:   registrationGate,
		submittedHandoff:   submittedHandoff,
		registration:       registration,
	}
}

// Covers: ADR-0094 Decision 五（as-of 未配置 → 等待态先 Save 再交回原因）、Decision 三（第四格
// 按处理完毕入账，不重投）、队列读口列得出、Decision 四（登记动作铸信封 → 第三扇门按租户重驱
// → 接受成立且队列出列）、重投与再来一封登记信封都由账本/空队列挡住不翻倍。
func TestAnOperatorRegistrationWaitIsResumedByTheRegistrationEnvelope(t *testing.T) {
	fixture := newOperatorRegistrationLoopFixture(t)
	ctx := t.Context()
	tenant := fixture.identity.TenantID()

	// 提交成立，按生产发布路径铸「委托已提交」信封。时点策略此刻未配置。
	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	// 第一扇门：链在可达性那条腿撞上`判断时点未配置`——D5 先把`等待运营登记`Save 进聚合，
	// 消费门按 D3 入账。回滚重投推不动一次登记，只会烧完预算再把等待态一起蒸发。
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope); err != nil {
		t.Fatalf("等待运营登记必须按处理完毕入账（ADR-0094 D3/D4）：%v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submittedDelivery.Envelope.ID)); n != 1 {
		t.Fatalf("提交门 inbox 行数 = %d, want 1——等待态没有随本份投递入账", n)
	}
	waiting := fixture.mustLoadRequest(t, ctx)
	if waiting.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("等待期状态 = %q, want SUBMITTED——等登记不是接受也不是拒绝", waiting.State())
	}
	waitingOn, present := waiting.AcceptanceDecisionTask().WaitingOn()
	if !present || waitingOn != psdomain.ResumeByOperatorRegistration {
		t.Fatalf("等待态 = %v（present=%v）, want OPERATOR_REGISTRATION——D5 没落库，队列列不出它", waitingOn, present)
	}
	queue := fixture.listRegistrationQueue(t, ctx)
	if len(queue) != 1 || queue[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("等待运营登记队列 = %+v, want 恰好这一份委托", queue)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 0 {
		t.Fatalf("等待期间入队了 %d 封接受信封", n)
	}
	if fixture.commercial.asOfQueries == 0 {
		t.Fatal("链没有走到时点形成那一步——停顿不是本用例要的那一格")
	}
	fixture.markPublished(t, submittedDelivery)

	// 登记：时点策略声明落库，同事务经真交接适配器铸「参数已登记」信封。替身从此答形成时点。
	fixture.commercial.configured = true
	version := synRulePackageVersion(t, tenant.String(), "SYN-RULES-R1", "v1")
	registeredAt := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.registration.HandOffOperatorRegistrationCompleted(txCtx, pcports.OperatorRegistrationCompletedIntent{
			Kind:         pcports.AsOfPolicyRegistered,
			Registration: version,
			RegisteredAt: registeredAt,
		})
	})
	if n := fixture.countOutboxOfType(t, string(psinbox.OperatorRegistrationCompletedEventType)); n != 1 {
		t.Fatalf("「参数已登记」信封 = %d 封, want 恰好 1——消费侧认的类型串与发布侧写的不是同一个", n)
	}

	// 第三扇门：按信封里的租户取队列，把同一条链再驱一拍——两条腿形成时点并记判断，形成决定
	// 成立接受，队列出列，恰好一封接受决定信封。
	registrationDelivery := fixture.claimDeliveryOfType(t, string(psinbox.OperatorRegistrationCompletedEventType))
	if err := fixture.registrationGate.Consume(ctx, registrationDelivery.Envelope); err != nil {
		t.Fatalf("登记续办一拍：%v", err)
	}
	if n := fixture.countInbox(t, operatorRegistrationResumeConsumer, string(registrationDelivery.Envelope.ID)); n != 1 {
		t.Fatalf("登记续办门 inbox 行数 = %d, want 1", n)
	}
	accepted := fixture.mustLoadRequest(t, ctx)
	if accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
	if decision, formed := accepted.AcceptanceDecision(); !formed || decision.DecisionID().String() != synV0DecisionID {
		t.Fatalf("决定 = %v formed = %v, want %s", decision, formed, synV0DecisionID)
	}
	if remaining := fixture.listRegistrationQueue(t, ctx); len(remaining) != 0 {
		t.Fatalf("接受之后队列仍列出 %d 行——等待态没随决定清掉", len(remaining))
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("接受信封 = %d 封, want 恰好 1", n)
	}

	// 登记信封重投：账本跳过，接受不翻倍。
	if err := fixture.registrationGate.Consume(ctx, registrationDelivery.Envelope); err != nil {
		t.Fatalf("重投登记信封：%v", err)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("重投后接受信封 = %d 封, want 仍为 1", n)
	}

	// 同一规则包发新版本再登记一次是另一封（ADR-0043）：队列已空，本封处理完毕入账，链一步
	// 不跑，接受仍不翻倍——已决的委托不在队列里，重驱天然只碰仍在等的。
	fixture.markPublished(t, registrationDelivery)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.registration.HandOffOperatorRegistrationCompleted(txCtx, pcports.OperatorRegistrationCompletedIntent{
			Kind:         pcports.AsOfPolicyRegistered,
			Registration: synRulePackageVersion(t, tenant.String(), "SYN-RULES-R1", "v2"),
			RegisteredAt: registeredAt.Add(time.Hour),
		})
	})
	second := fixture.claimDeliveryOfType(t, string(psinbox.OperatorRegistrationCompletedEventType))
	if second.Envelope.ID == registrationDelivery.Envelope.ID {
		t.Fatalf("新版本的登记信封与上一封同 ID %s——认领没带版本维", second.Envelope.ID)
	}
	if err := fixture.registrationGate.Consume(ctx, second.Envelope); err != nil {
		t.Fatalf("空队列上的登记信封应入账：%v", err)
	}
	if n := fixture.countInbox(t, operatorRegistrationResumeConsumer, string(second.Envelope.ID)); n != 1 {
		t.Fatalf("第二封 inbox 行数 = %d, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("第二封之后接受信封 = %d 封, want 仍为 1", n)
	}
}

func (fixture *operatorRegistrationLoopFixture) listRegistrationQueue(
	t *testing.T,
	ctx context.Context,
) []psports.OperatorRegistrationQueueRecord {
	t.Helper()
	records, err := fixture.requests.ListWaitingOnOperatorRegistration(ctx, fixture.identity.TenantID(), 10)
	if err != nil {
		t.Fatalf("列等待运营登记队列：%v", err)
	}
	return records
}

// claimDeliveryOfType / markPublished 与人工复核那条闭环同一手法（喂给门的正是库里那一封，
// 自己拼等于把发布侧那半换成手抄本）。
func (fixture *operatorRegistrationLoopFixture) claimDeliveryOfType(t *testing.T, eventType string) eventing.Delivery {
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

func (fixture *operatorRegistrationLoopFixture) markPublished(t *testing.T, delivery eventing.Delivery) {
	t.Helper()
	if err := fixture.store.MarkPublished(t.Context(), delivery.Ref, time.Now().UTC()); err != nil {
		t.Fatalf("定稿投递 %s：%v", delivery.Envelope.ID, err)
	}
}

// synRSwitchableAsOfCommercialBasis 是商业依据替身（隔离合成 `S`，不进生产装配）：解析与重校
// 照 synRCommercialBasis，但规则声明不要求人工复核（本条闭环要走到接受），且时点形成可拨——
// configured 为假时答`未配置`，正是 `ReachabilityAsOfNotConfigured` 的来源；拨真后照声明形成。
type synRSwitchableAsOfCommercialBasis struct {
	inner       *synRCommercialBasis
	t           *testing.T
	configured  bool
	asOfQueries int
}

func (double *synRSwitchableAsOfCommercialBasis) ResolveCommercialBasis(
	_ context.Context,
	_ psports.CommercialBasisQuery,
) (psports.CommercialBasisResolution, error) {
	return double.applicableWithoutReview()
}

func (double *synRSwitchableAsOfCommercialBasis) FormJudgmentAsOf(
	ctx context.Context,
	query psports.JudgmentAsOfQuery,
) (psports.JudgmentAsOfFormation, error) {
	double.asOfQueries++
	if !double.configured {
		return psports.JudgmentAsOfFormation{Outcome: psports.JudgmentAsOfNotConfigured}, nil
	}
	return double.inner.FormJudgmentAsOf(ctx, query)
}

func (double *synRSwitchableAsOfCommercialBasis) RevalidateCommercialBasis(
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
	return psports.CommercialRevalidation{Outcome: psports.CommercialBasisStillValid, Resolution: resolution}, nil
}

func (double *synRSwitchableAsOfCommercialBasis) applicableWithoutReview() (psports.CommercialBasisResolution, error) {
	double.t.Helper()
	groups, err := psdomain.NewApplicableCheckGroups(
		psdomain.PreAcceptanceFinancialControlCheck,
		psdomain.NetworkReachabilityCheck,
	)
	if err != nil {
		double.t.Fatalf("适用校验组：%v", err)
	}
	snapshot, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID: mustPS(double.t, psdomain.NewCommercialResolutionID, "SYN-RES-R2"),
		RulePackage:  mustPS(double.t, psdomain.NewRulePackageReference, "SYN-RULES-R1/v1"),
		ViewRevision: mustPS(double.t, psdomain.NewCommercialViewRevision, "SYN-VIEW-R2"),
		DeclaredAsOf: []psdomain.DeclaredAsOf{
			synDeclaredAsOf(double.t, psdomain.ReachabilityJudgmentKind),
			synDeclaredAsOf(double.t, psdomain.FinancialControlJudgmentKind),
		},
		Applicable:   groups,
		ManualReview: psdomain.ManualReviewNotRequiredByRules,
	})
	if err != nil {
		double.t.Fatalf("商业依据快照：%v", err)
	}
	return psports.CommercialBasisResolution{Snapshot: snapshot, Applicability: psdomain.CommerciallyApplicable}, nil
}

var _ psports.CommercialBasisResolver = (*synRSwitchableAsOfCommercialBasis)(nil)

// synRulePackageVersion 经 party-commercial 的领域重建门造一份`已生效`规则包版本——登记信封由它
// 认领（租户 + 种类 + 对象/版本）。租户与 PS 侧来源身份同一个串：跨上下文只靠这个字面对上。
func synRulePackageVersion(t *testing.T, tenant, objectID, label string) pcdomain.CommercialVersion {
	t.Helper()
	publishedAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	effectiveAt := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(effectiveAt, effectiveAt.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustPS(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-"+objectID),
		mustPS(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-"+objectID),
		publishedAt.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustPS(t, pcdomain.NewTenantID, tenant),
		Kind:          pcdomain.AcceptanceRulePackageObject,
		ObjectID:      mustPS(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       mustPS(t, pcdomain.NewCommercialVersionLabel, label),
		Scope:         mustPS(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-R1"),
		ContentDigest: mustPS(t, pcdomain.NewCommercialContentDigest, "SYN-DIGEST-"+label),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAt,
		EffectiveAt:   effectiveAt,
	})
	if err != nil {
		t.Fatalf("重建规则包版本：%v", err)
	}
	return version
}
