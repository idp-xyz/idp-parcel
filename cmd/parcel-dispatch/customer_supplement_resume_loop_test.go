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

// 本文件对真实 PostgreSQL 16 证 ADR-0106 的整条闭环（票 first-tenant-runway/09）：可达性权威答
// `证据不足`时，接受链在形成决定处停成`等待受控补充`，等待态先 Save 进聚合再交回原因，提交门按入账
// 处置（不再回滚重投烧预算）；客户经受控补充编排形成同一委托的新提交版本，新版本落库与「新提交
// 版本已形成」信封同一事务；第四扇门按这封信把同一条链拿**新版本**再驱一拍，可达性重判为`可达`，
// 决定形成为接受，等待态随决定清掉；重投与同一补充的重放都不翻倍。
//
// 与另两条闭环的分工：人工复核那条证`要求人工复核`那格暂停与复核完成信封，运营登记那条证决定之前
// 的`等待运营登记`与跨上下文的登记信封；本条证的是三个外部续办态里最后一个并回入账的那格，以及
// ADR-0045 划出去、由 ADR-0106 Decision 三承接的「新版本重触发判断」。链按 synR 先例在测试内装同一
// 形状：仓储、判断账、Outbox/Inbox、两扇消费门、受控补充编排与三段接受编排全是生产实现，只有三个
// 权威口换成替身；受控补充的两层事务壳形状照 cmd/parcel-api 的 preservationBoundary /
// supplementBoundary——生产壳在装配包里不可导入，由那边的装配用例取证。
//
// 可达性权威替身按**提交版本**答：首版`证据不足`、新版本`可达`。判断时点**两版相同**（synRAsOfAnchor）
// ——这是刻意的：一份「以首次提交时刻为判断时点」的策略是规则包的合法声明，判断账不能靠时点变化才
// 认出新版本。判断账以（成员 + 提交版本 + 时点）为键、读口按当前版本取（票 first-tenant-runway/10，
// ADR-0045 Consequences 预告的版本维），新版本的`可达`因此成为自己那一版的行，决定读的也是这一版；
// 没有版本维时它会被同键的 ON CONFLICT DO NOTHING 吞掉、决定仍读到旧的`证据不足`、链再次停等补充。

const (
	customerSupplementResumeConsumer = "parcel-shipment/advance-acceptance-chain-on-submission-version-formed"
	synRSupplementVersionID          = "SYN-VER-02"
	synRSupplementTaskID             = "SYN-TASK-02"
)

type customerSupplementLoopFixture struct {
	*synVerticalFixture
	store            *outbox.Store
	views            *pspostgres.ShipmentRequestViews
	reachability     *synRPerVersionReachabilityAuthority
	submittedGate    *psinbox.ShipmentRequestSubmittedConsumer
	supplementGate   *psinbox.SubmissionVersionFormedConsumer
	submittedHandoff *pspostgres.OutboxShipmentRequestSubmittedHandoff
	supplement       *psapplication.FormNewSubmissionVersionHandler
}

func newCustomerSupplementLoopFixture(t *testing.T) *customerSupplementLoopFixture {
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
	sources, err := pspostgres.NewSourceSubmissions(base.db)
	if err != nil {
		t.Fatalf("构造来源仓储：%v", err)
	}
	views, err := pspostgres.NewShipmentRequestViews(base.db)
	if err != nil {
		t.Fatalf("构造委托查阅读口：%v", err)
	}
	decisionHandoff, err := pspostgres.NewOutboxAcceptanceDecisionHandoff(base.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造接受决定交接：%v", err)
	}
	firstVersion := mustPS(t, psdomain.NewSubmissionVersionID, "SYN-VER-01")
	// 两版同一时点（见文件头）：时点策略对哪一版都答 synRAsOfAnchor。
	commercial := &synRSwitchableAsOfCommercialBasis{inner: &synRCommercialBasis{t: t}, t: t, configured: true}
	reachability := &synRPerVersionReachabilityAuthority{t: t, firstVersion: firstVersion}
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
			commercial, reachability, base.judgments, base.requests, systemClock{}),
		FinancialControl: psapplication.NewAdvanceFinancialControlJudgmentHandler(
			commercial, synRFinancialAuthority{t: t}, synRControlDispositions{}, base.judgments, base.requests, systemClock{}),
		Decision: decision,
	})
	submittedGate, err := psinbox.NewShipmentRequestSubmittedConsumer(base.transactor, inboxStore, chain)
	if err != nil {
		t.Fatalf("构造提交门：%v", err)
	}
	supplementGate, err := psinbox.NewSubmissionVersionFormedConsumer(base.transactor, inboxStore, chain)
	if err != nil {
		t.Fatalf("构造受控补充续办门：%v", err)
	}
	submittedHandoff, err := pspostgres.NewOutboxShipmentRequestSubmittedHandoff(base.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造提交交接：%v", err)
	}
	versionFormedHandoff, err := pspostgres.NewOutboxSubmissionVersionFormedHandoff(base.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造新版本交接：%v", err)
	}
	supplement := psapplication.NewFormNewSubmissionVersionHandler(psapplication.FormNewSubmissionVersionDeps{
		Sources: synRSupplementPreservation{transactor: base.transactor, inner: sources},
		Requests: synRSupplementBoundary{
			transactor: base.transactor,
			inner:      base.requests,
			handoff:    versionFormedHandoff,
		},
		Identities: synRSupplementIdentities{},
		Clock:      systemClock{},
	})
	return &customerSupplementLoopFixture{
		synVerticalFixture: base,
		store:              store,
		views:              views,
		reachability:       reachability,
		submittedGate:      submittedGate,
		supplementGate:     supplementGate,
		submittedHandoff:   submittedHandoff,
		supplement:         supplement,
	}
}

// Covers: ADR-0106 Decision 二（`等待受控补充`先 Save 再交回原因）、Decision 一（提交门按处理完毕
// 入账，不重投）、Decision 三（新版本与信封同事务、第四扇门拿新版本把同一条链再驱一拍直到接受）、
// 重投与同一补充的重放都由账本与编排的重放规则挡住不翻倍。
func TestACustomerSupplementWaitIsResumedByTheNewSubmissionVersionEnvelope(t *testing.T) {
	fixture := newCustomerSupplementLoopFixture(t)
	ctx := t.Context()

	// 提交成立，并按生产发布路径铸「委托已提交」信封。
	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	// 第一扇门：链走完两条腿，可达性答`证据不足`，形成决定处停成`等待受控补充`——暂停不是错误，
	// 本份投递按处理完毕入账。回滚重投推不动客户，只会烧完失败预算再把等待态一起蒸发。
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	submitted := submittedDelivery.Envelope
	if err := fixture.submittedGate.Consume(ctx, submitted); err != nil {
		t.Fatalf("等待受控补充必须按处理完毕入账（ADR-0106 Decision 一）：%v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submitted.ID)); n != 1 {
		t.Fatalf("提交门 inbox 行数 = %d, want 1——暂停没有随本份投递入账", n)
	}
	if fixture.reachability.assessments != 1 {
		t.Fatalf("可达性权威被问了 %d 次, want 1——停顿不是本用例要的那一格", fixture.reachability.assessments)
	}

	paused := fixture.mustLoadRequest(t, ctx)
	if paused.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("暂停后状态 = %q, want SUBMITTED——等补充不是接受也不是拒绝", paused.State())
	}
	waiting, present := paused.AcceptanceDecisionTask().WaitingOn()
	if !present || waiting != psdomain.ResumeByCustomerSupplement {
		t.Fatalf("等待态 = %v（present=%v）, want CUSTOMER_SUPPLEMENT——暂停没落库，队列列不出它", waiting, present)
	}

	// 队列读面：真适配器按 0009 投影列（0016 部分索引）上列——ADR-0106 之前这一格在库里恒空。
	queue := fixture.listSupplementQueue(t, ctx)
	if len(queue) != 1 || queue[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("等待受控补充队列 = %+v, want 恰好这一份委托", queue)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 0 {
		t.Fatalf("暂停期间入队了 %d 封接受信封", n)
	}

	// 同一封重投：账本跳过，链不重跑，权威不再被问，决定照旧未形成。
	if err := fixture.submittedGate.Consume(ctx, submitted); err != nil {
		t.Fatalf("重投已入账的投递：%v", err)
	}
	if fixture.reachability.assessments != 1 {
		t.Fatalf("重投后可达性权威被问了 %d 次, want 仍为 1——已入账的投递不该再驱链", fixture.reachability.assessments)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 0 {
		t.Fatalf("重投后入队了 %d 封接受信封", n)
	}
	fixture.markPublished(t, submittedDelivery)

	// 受控补充：客户以首版为基准补充，新版本落库与「新提交版本已形成」信封同一事务。
	first := fixture.supplementOn(t, ctx, paused.CurrentSubmissionVersion().VersionID())
	if first.Outcome() != psapplication.SupplementRecorded {
		t.Fatalf("受控补充 outcome = %q, want SUPPLEMENT_RECORDED（未决原因 %q）", first.Outcome(), first.PendingReason())
	}
	formed, present := first.Version()
	if !present || formed.VersionID().String() != synRSupplementVersionID {
		t.Fatalf("新版本 = %v（present=%v）, want %s", formed.VersionID(), present, synRSupplementVersionID)
	}
	if n := fixture.countOutboxOfType(t, string(psinbox.SubmissionVersionFormedEventType)); n != 1 {
		t.Fatalf("「新提交版本已形成」信封 = %d 封, want 恰好 1——消费侧认的类型串与发布侧写的不是同一个", n)
	}
	supplemented := fixture.mustLoadRequest(t, ctx)
	if got := supplemented.CurrentSubmissionVersion().VersionID().String(); got != synRSupplementVersionID {
		t.Fatalf("补充后当前版本 = %q, want %s", got, synRSupplementVersionID)
	}
	if waiting, present := supplemented.AcceptanceDecisionTask().WaitingOn(); present {
		t.Fatalf("新版本重建了任务，等待态却仍是 %v——换代没有清掉上一版的停顿", waiting)
	}
	if remaining := fixture.listSupplementQueue(t, ctx); len(remaining) != 0 {
		t.Fatalf("新版本形成后队列仍列出 %d 行——出队靠事实不靠读侧折叠", len(remaining))
	}

	// 同一补充重放：编排答`已处理`，不形成第二个版本，也不铸第二封。
	replay := fixture.supplementOn(t, ctx, paused.CurrentSubmissionVersion().VersionID())
	if replay.Outcome() != psapplication.SupplementAlreadyHandled {
		t.Fatalf("重放同一补充 outcome = %q, want SUPPLEMENT_ALREADY_HANDLED", replay.Outcome())
	}
	if n := fixture.countOutboxOfType(t, string(psinbox.SubmissionVersionFormedEventType)); n != 1 {
		t.Fatalf("重放后信封 = %d 封, want 仍为 1", n)
	}

	// 第四扇门：续办信封把同一条链拿新版本再驱一拍——可达性重判为`可达`（新时点、新行），
	// 形成决定看到全部通过，接受成立并入队恰好一封接受决定信封。
	resumeDelivery := fixture.claimDeliveryOfType(t, string(psinbox.SubmissionVersionFormedEventType))
	resume := resumeDelivery.Envelope
	if err := fixture.supplementGate.Consume(ctx, resume); err != nil {
		t.Fatalf("受控补充续办一拍：%v", err)
	}
	if n := fixture.countInbox(t, customerSupplementResumeConsumer, string(resume.ID)); n != 1 {
		t.Fatalf("受控补充续办门 inbox 行数 = %d, want 1", n)
	}
	if fixture.reachability.assessments != 2 {
		t.Fatalf("续办后可达性权威被问了 %d 次, want 2——续办没有拿新版本重判", fixture.reachability.assessments)
	}
	if got := fixture.reachability.lastVersion.String(); got != synRSupplementVersionID {
		t.Fatalf("续办一拍问权威用的版本 = %q, want 新版本 %s", got, synRSupplementVersionID)
	}
	// 两版同时点，判断账里仍是两行：新版本的`可达`是自己那一版的行，不是被同键吞掉的重放。
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_reachability_judgment`); n != 2 {
		t.Fatalf("可达性判断行数 = %d, want 2——新版本同时点的判断被当成旧版的重放吞掉了（ADR-0045 后续项）", n)
	}

	accepted := fixture.mustLoadRequest(t, ctx)
	if accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
	if decision, formed := accepted.AcceptanceDecision(); !formed || decision.DecisionID().String() != synV0DecisionID {
		t.Fatalf("决定 = %v formed = %v, want %s", decision, formed, synV0DecisionID)
	}
	if waiting, present := accepted.AcceptanceDecisionTask().WaitingOn(); present {
		t.Fatalf("接受之后等待态仍是 %v——等待态没随决定清掉", waiting)
	}
	if remaining := fixture.listSupplementQueue(t, ctx); len(remaining) != 0 {
		t.Fatalf("接受之后队列仍列出 %d 行", len(remaining))
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("接受信封 = %d 封, want 恰好 1", n)
	}

	// 续办信封重投：账本跳过，接受不翻倍。
	if err := fixture.supplementGate.Consume(ctx, resume); err != nil {
		t.Fatalf("重投续办信封：%v", err)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("重投后接受信封 = %d 封, want 仍为 1", n)
	}
}

// supplementOn 以 basis 为基准发一次受控补充：补充请求有自己的来源身份与载荷摘要（与首次提交是
// 两次来源请求），成员集合不变。
func (fixture *customerSupplementLoopFixture) supplementOn(
	t *testing.T,
	ctx context.Context,
	basis psdomain.SubmissionVersionID,
) psapplication.FormNewSubmissionVersionResult {
	t.Helper()

	supplementIdentity, err := psdomain.NewSourceIdentity(
		fixture.identity.TenantID(),
		fixture.identity.CustomerAccountID(),
		fixture.identity.Source(),
		mustPS(t, psdomain.NewSourceRequestKey, "SYN-KEY-01-SUPPLEMENT"),
	)
	if err != nil {
		t.Fatalf("补充来源身份：%v", err)
	}
	now := time.Now().UTC()
	result, err := fixture.supplement.Handle(ctx, psapplication.FormNewSubmissionVersionCommand{
		Identity:           fixture.identity,
		SupplementIdentity: supplementIdentity,
		PayloadDigest:      mustPS(t, psdomain.NewPayloadDigest, "SYN-DIGEST-02"),
		OccurredAt:         now.Add(-2 * time.Second),
		ReceivedAt:         now.Add(-time.Second),
		ShipmentRequestID:  fixture.requestID,
		BasisVersion:       basis,
		DeclaredParcelIDs: []psdomain.DeclaredParcelID{
			mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01"),
		},
	})
	if err != nil {
		t.Fatalf("受控补充：%v", err)
	}
	return result
}

func (fixture *customerSupplementLoopFixture) listSupplementQueue(
	t *testing.T,
	ctx context.Context,
) []psports.CustomerSupplementQueueRecord {
	t.Helper()

	scope, err := psdomain.NewAuthorizedQueryScope(
		mustPS(t, psdomain.NewQueryScopeReference, "SYN-QS-01"),
		fixture.identity.TenantID(),
		[]psdomain.CustomerAccountID{fixture.identity.CustomerAccountID()},
	)
	if err != nil {
		t.Fatalf("作用域：%v", err)
	}
	records, err := fixture.views.ListWaitingOnCustomerSupplement(ctx, scope, 10)
	if err != nil {
		t.Fatalf("列等待受控补充队列：%v", err)
	}
	return records
}

// claimDeliveryOfType / markPublished 与另两条闭环同一手法（喂给门的正是库里那一封，自己拼等于
// 把发布侧那半换成手抄本）。
func (fixture *customerSupplementLoopFixture) claimDeliveryOfType(t *testing.T, eventType string) eventing.Delivery {
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

func (fixture *customerSupplementLoopFixture) markPublished(t *testing.T, delivery eventing.Delivery) {
	t.Helper()
	if err := fixture.store.MarkPublished(t.Context(), delivery.Ref, time.Now().UTC()); err != nil {
		t.Fatalf("定稿投递 %s：%v", delivery.Envelope.ID, err)
	}
}

// synRSupplementPreservation 是受控补充来源保全的事务壳（隔离合成 `S`）：每笔保全各开一个事务，
// 形状照 cmd/parcel-api 的 preservationBoundary。
type synRSupplementPreservation struct {
	transactor bentoapp.Transactor
	inner      psports.SourceSubmissionRepository
}

var _ psports.SourceSubmissionRepository = synRSupplementPreservation{}

func (boundary synRSupplementPreservation) FindPreserved(
	ctx context.Context,
	identity psdomain.SourceIdentity,
) (psdomain.SourceSubmissionFingerprint, bool, error) {
	return boundary.inner.FindPreserved(ctx, identity)
}

func (boundary synRSupplementPreservation) Preserve(
	ctx context.Context,
	submission psdomain.SourceSubmissionFingerprint,
) error {
	return boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return boundary.inner.Preserve(txCtx, submission)
	})
}

func (boundary synRSupplementPreservation) AppendObservation(
	ctx context.Context,
	observed psdomain.SourceSubmissionFingerprint,
) error {
	return boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return boundary.inner.AppendObservation(txCtx, observed)
	})
}

// synRSupplementBoundary 是受控补充的事务边界壳（隔离合成 `S`）：新版本落库与「新提交版本已形成」
// 信封同一事务。形状照 cmd/parcel-api 的 supplementBoundary——生产壳在装配包里不可导入，这里要的
// 只是「新版本落库即有信封」这一格，生产壳本身由那边的装配用例取证。
type synRSupplementBoundary struct {
	transactor bentoapp.Transactor
	inner      psports.ShipmentRequestRepository
	handoff    psports.SubmissionVersionFormedHandoff
}

var _ psports.ShipmentRequestRepository = synRSupplementBoundary{}

func (boundary synRSupplementBoundary) FindBySourceIdentity(
	ctx context.Context,
	identity psdomain.SourceIdentity,
) (psdomain.ShipmentRequest, bool, error) {
	return boundary.inner.FindBySourceIdentity(ctx, identity)
}

func (boundary synRSupplementBoundary) Insert(
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

func (boundary synRSupplementBoundary) Save(
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
		return boundary.handoff.HandOffSubmissionVersionFormed(txCtx,
			psports.SubmissionVersionFormedHandoffIntent{Identity: identity, Request: request})
	})
	if err != nil {
		return psports.ShipmentRequestSaveOutcomeInvalid, err
	}
	return outcome, nil
}

// synRSupplementIdentities 给新版本签发与首版不同的版本号与任务号（隔离合成 `S`）：ADR-0045 要求
// 版本编号不与任何一代重号，synS 那份对首版恒答同一个号，补充不能沿用它。
type synRSupplementIdentities struct{}

func (synRSupplementIdentities) NextSubmissionVersionID(_ context.Context) (psdomain.SubmissionVersionID, error) {
	return psdomain.NewSubmissionVersionID(synRSupplementVersionID)
}

func (synRSupplementIdentities) NextAcceptanceDecisionTaskID(_ context.Context) (psdomain.AcceptanceDecisionTaskID, error) {
	return psdomain.NewAcceptanceDecisionTaskID(synRSupplementTaskID)
}

// synRPerVersionReachabilityAuthority 是 network-routing 权威替身（隔离合成 `S`）：对首版答
// `证据不足`——正是要证的那格停顿；对之后的版本答`可达`，模拟客户补足了证据。判断标识按
// （成员 + 版本）派生，时点原样回签。记下被问次数与最近一次的版本，用例据此证「重投不再问、
// 续办拿新版本问」。
type synRPerVersionReachabilityAuthority struct {
	t            *testing.T
	firstVersion psdomain.SubmissionVersionID
	assessments  int
	lastVersion  psdomain.SubmissionVersionID
}

func (double *synRPerVersionReachabilityAuthority) AssessParcelReachability(
	_ context.Context,
	request psports.ReachabilityRequest,
) (psports.ReachabilityAssessment, error) {
	double.t.Helper()
	double.assessments++
	double.lastVersion = request.SubmissionVersion

	value := psdomain.ReachabilityReachable
	if request.SubmissionVersion == double.firstVersion {
		value = psdomain.ReachabilityInsufficientEvidence
	}
	judgment, err := psdomain.NewReachabilityJudgment(psdomain.ReachabilityJudgmentSpec{
		JudgmentID: mustPS(double.t, psdomain.NewReachabilityJudgmentID,
			"SYN-NRJ-S-"+request.DeclaredParcelID.String()+"-"+request.SubmissionVersion.String()),
		ParcelID: request.DeclaredParcelID,
		Value:    value,
		AsOf:     request.AsOf,
	})
	if err != nil {
		double.t.Fatalf("可达性判断：%v", err)
	}
	return psports.ReachabilityAssessment{
		Outcome:  psports.ReachabilityAssessed,
		Judgment: judgment,
	}, nil
}

var (
	_ psports.ReachabilityAssessor      = (*synRPerVersionReachabilityAuthority)(nil)
	_ psports.SubmissionIdentityFactory = synRSupplementIdentities{}
)
