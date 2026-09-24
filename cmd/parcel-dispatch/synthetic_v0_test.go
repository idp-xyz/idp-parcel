package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pshandoff "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/productionhandoff"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件是 SYN-V0：从 PS 应用编排穿过真仓储与 Outbox，再经生产 wireDispatcher 投到
// NR 接受决定消费者，停在生产装配里那个诚实未决格。它不是全链闭环。
//
// S 替身只出现在本测试文件，名字带 synS；生产 assemble.go 不种服务产品、不默认适用性、
// 不 INSERT 网络定义。已决定路径调 SYN-PC-PRODUCT 种子越过适用性，停在证据未配置。

const (
	synV0AcceptanceConsumer = "network-routing/initial-route-on-acceptance"
	synV0VERederiveConsumer = "visibility-exception/derive-customer-view-from-acceptance"
	synV0InitialRouteType   = "network-routing.initial-route.formed"
	synV0DecisionID         = "SYN-DEC-01"
)

// Covers: SYN-V0 判断未齐路径——提交成立但形成决定停在 UNDECIDED，接受信封不得入队。
func TestSYNIncompleteJudgmentsStayUndecidedWithoutAnAcceptanceEnvelope(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fixture.submit(t, ctx)

	result := fixture.formDecision(t, ctx)
	if result.Outcome() != psapplication.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED", result.State())
	}
	if result.PendingReason() != psapplication.AcceptanceJudgmentIncomplete {
		t.Fatalf("pending = %q, want ACCEPTANCE_JUDGMENT_INCOMPLETE", result.PendingReason())
	}

	request := fixture.mustLoadRequest(t, ctx)
	if request.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("stored state = %q, want SUBMITTED", request.State())
	}
	if count := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); count != 0 {
		t.Fatalf("未齐判断却入队了 %d 封接受信封", count)
	}
}

// Covers: SYN-V0 已决定路径——PS 应用 handler 形成接受并入队真实 Outbox，Dispatcher
// 把信封投进 FanOut（先 VE 客户归属确立补派生，后 NR 初始路由）；已接受重建门已开
// （ADR-0061），NR 消费者按引用读回委托并进入 CreateInitialRoute。生产装配按已接受
// 解析回指闭包（ADR-0064）；本用例调同一份 SYN-PC-PRODUCT 种子让 SYN-RES-01 采用可
// 观察的 NetworkServiceForm，适用性译成要求判断。网络目录空册，NR 腿停在
// ROUTE_EVIDENCE_NOT_CONFIGURED；VE 腿常态空转（包裹还没流转、无当前投影）成功入账
// ——两本 inbox 互不隶属，VE 成功不改变 NR 腿的未决记账，整封失败码仍是
// dispatch.consumer_undecided（仅全路未决才记未决，外部评审票 01 的分格）。不得写可
// 执行路由，也不得把未决当成已处理入账，更不得因空转发明客户视图。
func TestSYNAcceptedDecisionStopsAtUnconfiguredRouteEvidence(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)

	result := fixture.formDecision(t, ctx)
	if result.Outcome() != psapplication.AcceptanceDecided {
		t.Fatalf("outcome = %q, want DECIDED；pending = %q", result.Outcome(), result.PendingReason())
	}
	if result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", result.State())
	}
	if ref := result.DecisionHandoffReference(); ref.String() != "" {
		t.Fatalf("接受决定的发布意图没交出：%s", ref)
	}
	decision, formed := result.AcceptanceDecision()
	if !formed || decision.DecisionID().String() != synV0DecisionID {
		t.Fatalf("决定标识 = %q formed = %v, want %s", decision.DecisionID(), formed, synV0DecisionID)
	}

	ids := fixture.outboxIDsOfType(t, string(nrinbox.AcceptedDecisionEventType))
	if len(ids) != 1 || ids[0] != synV0DecisionID {
		t.Fatalf("接受信封 = %v, want 恰好一封且 ID = 决定标识 %s（ADR-0043）", ids, synV0DecisionID)
	}

	stored := fixture.mustLoadRequest(t, ctx)
	if stored.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("读回状态 = %q, want ACCEPTED（重建门已开到已接受）", stored.State())
	}
	if _, present := stored.AcceptanceBaseline(); !present {
		t.Fatal("读回已接受委托却没有接受基线")
	}

	seedSYNPCEligibility(t, fixture)
	assertSYNPCEligibilitySeeded(t, fixture)
	assertRoutingApplicabilityRequired(t, fixture)
	assertRouteEvidenceUnconfigured(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("证据未配置却定稿了 %d 条；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, synV0DecisionID))
	}
	if got := recordedFailureCode(t, fixture.db, synV0DecisionID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided（空册未配置，不是适用性也不是重建门）", got)
	}

	if n := fixture.countInbox(t, synV0AcceptanceConsumer, synV0DecisionID); n != 0 {
		t.Fatalf("inbox 行数 = %d, want 0——未决必须回滚，不能冒充已处理", n)
	}
	// VE 腿与 NR 腿两本账互不隶属：包裹还没流转、无当前投影，VE 腿空转并在自己的
	// 事务里入账；NR 腿的未决不把它的入账也卡住（FanOut 每路都调到）。
	if n := fixture.countInbox(t, synV0VERederiveConsumer, synV0DecisionID); n != 1 {
		t.Fatalf("VE 补派生 inbox 行数 = %d, want 1——空转也要入账，重投由账本跳过", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view`); n != 0 {
		t.Fatalf("customer_view 行数 = %d, want 0——无投影不得发明视图", n)
	}
	fixture.assertNoInitialRoute(t)
	if n := fixture.countRouteHandoffLogs(t); n != 0 {
		t.Fatalf("route_handoff_log 行数 = %d, want 0——未决事务回滚后登记册应无痕", n)
	}

	// 同一份接受信封再拍一次：inbox 无账，派发会再投；业务结果仍不得翻倍。
	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("重拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("重拍定稿了 %d 条", published)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("接受信封变成 %d 封——重投不得再入队一份", n)
	}
	fixture.assertNoInitialRoute(t)
	if n := fixture.countInbox(t, synV0AcceptanceConsumer, synV0DecisionID); n != 0 {
		t.Fatalf("重拍后 inbox 行数 = %d, want 仍为 0", n)
	}
	if n := fixture.countInbox(t, synV0VERederiveConsumer, synV0DecisionID); n != 1 {
		t.Fatalf("重拍后 VE 补派生 inbox 行数 = %d, want 仍为 1——重投由账本跳过，不重跑", n)
	}
}

type synVerticalFixture struct {
	pool          *pgxpool.Pool
	db            *bentopg.DB
	transactor    bentoapp.Transactor
	beat          Beat
	requests      *pspostgres.ShipmentRequests
	judgments     *pspostgres.AcceptanceJudgments
	submitHandler *psapplication.SubmitShipmentRequestHandler
	formHandler   *psapplication.FormAcceptanceDecisionHandler
	identity      psdomain.SourceIdentity
	requestID     psdomain.ShipmentRequestID
}

func newSYNVerticalFixture(t *testing.T) *synVerticalFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	sources, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		t.Fatalf("构造来源仓储：%v", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	judgments, err := pspostgres.NewAcceptanceJudgments(db)
	if err != nil {
		t.Fatalf("构造判断仓储：%v", err)
	}
	handoff, err := pspostgres.NewOutboxAcceptanceDecisionHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造接受决定交接：%v", err)
	}

	purpose, err := nrdomain.NewServicePurpose("NETWORK_SERVICE")
	if err != nil {
		t.Fatalf("服务目的：%v", err)
	}
	beat, err := wireDispatcher(db, dispatchSettings{
		purpose:         purpose,
		deliveryTimeout: 5 * time.Second,
		config: dispatch.Config{
			Limit:       10,
			LeaseFor:    time.Minute,
			MaxAttempts: 5,
			// 重拍要立刻认领同一份未决信封；生产间隔不在本测试的观察范围内。
			RetryAfter: time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("装配派发器：%v", err)
	}

	identity := synSourceIdentity(t)
	fixture := &synVerticalFixture{
		pool:       pool,
		db:         db,
		transactor: db.Transactor(),
		beat:       beat,
		requests:   requests,
		judgments:  judgments,
		identity:   identity,
		requestID:  mustPS(t, psdomain.NewShipmentRequestID, "SYN-REQ-01"),
	}
	fixture.submitHandler = psapplication.NewSubmitShipmentRequestHandler(
		sources,
		requests,
		&synSProductionOwnership{t: t},
		pshandoff.UnconfiguredOtherProductionAuthorityChannel{},
		&synSSubmissionIdentities{},
		systemClock{},
	)
	fixture.formHandler = psapplication.NewFormAcceptanceDecisionHandler(psapplication.FormAcceptanceDecisionDeps{
		Requests:     requests,
		Commercial:   &synSCommercialBasis{t: t},
		Reachability: &synSReachabilityRevalidator{},
		Judgments:    judgments,
		Recorder:     judgments,
		Release:      &synSControlRelease{},
		Downstream:   handoff,
		Identities:   &synSDecisionIdentities{},
		Clock:        systemClock{},
	})
	return fixture
}

func (fixture *synVerticalFixture) submit(t *testing.T, ctx context.Context) {
	t.Helper()

	var outcome psapplication.SubmitOutcome
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		result, err := fixture.submitHandler.Handle(txCtx, fixture.submitCommand(t))
		if err != nil {
			return err
		}
		outcome = result.Outcome()
		return nil
	})
	if outcome != psapplication.OutcomeSubmitted {
		t.Fatalf("submit outcome = %q, want SUBMITTED", outcome)
	}
}

func (fixture *synVerticalFixture) recordPassingJudgments(t *testing.T, ctx context.Context) {
	t.Helper()

	parcel := mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01")
	asOf := synJudgmentAsOf(t, psdomain.ReachabilityJudgmentKind, time.Now().UTC().Add(-time.Minute))
	reachable, err := psdomain.NewReachabilityJudgment(psdomain.ReachabilityJudgmentSpec{
		JudgmentID: mustPS(t, psdomain.NewReachabilityJudgmentID, "SYN-NRJ-01"),
		ParcelID:   parcel,
		Value:      psdomain.ReachabilityReachable,
		AsOf:       asOf,
	})
	if err != nil {
		t.Fatalf("可达性判断：%v", err)
	}
	controlAsOf := synJudgmentAsOf(t, psdomain.FinancialControlJudgmentKind, time.Now().UTC().Add(-time.Minute))
	held := synHeldControl(t, "SYN-SAC-01", controlAsOf)

	// 判断记在当前提交版本上：形成决定按版本读判断（ADR-0045 的版本维），记错版本等于没记。
	version := fixture.mustLoadRequest(t, ctx).CurrentSubmissionVersion().VersionID()
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		if err := fixture.judgments.RecordReachabilityJudgment(
			txCtx, fixture.identity.TenantID(), fixture.requestID, version, reachable); err != nil {
			return err
		}
		return fixture.judgments.RecordFinancialControlResult(
			txCtx, fixture.identity.TenantID(), fixture.requestID, version, held)
	})
}

func (fixture *synVerticalFixture) formDecision(
	t *testing.T,
	ctx context.Context,
) psapplication.FormAcceptanceDecisionResult {
	t.Helper()

	request := fixture.mustLoadRequest(t, ctx)
	command := psapplication.FormAcceptanceDecisionCommand{
		Identity:          fixture.identity,
		ShipmentRequestID: fixture.requestID,
		SubmissionVersion: request.CurrentSubmissionVersion().VersionID(),
	}

	var result psapplication.FormAcceptanceDecisionResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = fixture.formHandler.Handle(txCtx, command)
		return err
	})
	return result
}

func (fixture *synVerticalFixture) mustLoadRequest(t *testing.T, ctx context.Context) psdomain.ShipmentRequest {
	t.Helper()
	request, found, err := fixture.requests.FindBySourceIdentity(ctx, fixture.identity)
	if err != nil {
		t.Fatalf("读回委托：%v", err)
	}
	if !found {
		t.Fatal("委托读不回来")
	}
	return request
}

func (fixture *synVerticalFixture) submitCommand(t *testing.T) psapplication.SubmitShipmentRequestCommand {
	t.Helper()
	now := time.Now().UTC()
	scope, err := psdomain.NewAdmissionScope(
		mustPS(t, psdomain.NewAdmissionScopeReference, "SYN-SCOPE-01"),
		mustPS(t, psdomain.NewAdmissionScopeDigest, "SYN-SCOPE-DIGEST-01"),
	)
	if err != nil {
		t.Fatalf("准入范围：%v", err)
	}
	return psapplication.SubmitShipmentRequestCommand{
		Identity:          fixture.identity,
		PayloadDigest:     mustPS(t, psdomain.NewPayloadDigest, "SYN-DIGEST-01"),
		OccurredAt:        now.Add(-2 * time.Second),
		ReceivedAt:        now.Add(-time.Second),
		BatchID:           mustPS(t, psdomain.NewSubmissionBatchID, "SYN-BATCH-01"),
		ShipmentRequestID: fixture.requestID,
		DeclaredParcelIDs: []psdomain.DeclaredParcelID{
			mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01"),
		},
		AdmissionScope:   scope,
		ExpectedRevision: mustPS(t, psdomain.NewProductionOwnershipRevision, "SYN-OWN-REV-01"),
	}
}

func (fixture *synVerticalFixture) countOutboxOfType(t *testing.T, eventType string) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	query := `SELECT count(*) FROM ` + migrate.SchemaBento + `.outbox WHERE event_type = $1`
	if err := querier.QueryRow(t.Context(), query, eventType).Scan(&count); err != nil {
		t.Fatalf("数 Outbox %s：%v", eventType, err)
	}
	return count
}

func (fixture *synVerticalFixture) outboxIDsOfType(t *testing.T, eventType string) []string {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	query := `SELECT event_id FROM ` + migrate.SchemaBento + `.outbox WHERE event_type = $1 ORDER BY event_id`
	rows, err := querier.Query(t.Context(), query, eventType)
	if err != nil {
		t.Fatalf("列 Outbox %s：%v", eventType, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("扫 event_id：%v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历 Outbox：%v", err)
	}
	return ids
}

func (fixture *synVerticalFixture) countInbox(t *testing.T, consumer, eventID string) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	query := `SELECT count(*) FROM ` + migrate.SchemaBento + `.inbox
		WHERE consumer = $1 AND event_id = $2`
	if err := querier.QueryRow(t.Context(), query, consumer, eventID).Scan(&count); err != nil {
		t.Fatalf("数 inbox：%v", err)
	}
	return count
}

func (fixture *synVerticalFixture) countInitialRoutes(t *testing.T) int {
	t.Helper()
	return fixture.countSQL(t, `SELECT count(*) FROM network_routing.initial_route`)
}

func (fixture *synVerticalFixture) assertNoInitialRoute(t *testing.T) {
	t.Helper()
	if n := fixture.countInitialRoutes(t); n != 0 {
		t.Fatalf("initial_route 行数 = %d, want 0——未决不得写可执行路由", n)
	}
	if n := fixture.countOutboxOfType(t, synV0InitialRouteType); n != 0 {
		t.Fatalf("发出了 %d 封 %s，会堵无订阅者分区", n, synV0InitialRouteType)
	}
}

func (fixture *synVerticalFixture) countRouteHandoffLogs(t *testing.T) int {
	t.Helper()
	return fixture.countSQL(t, `SELECT count(*) FROM network_routing.route_handoff_log`)
}

func (fixture *synVerticalFixture) countSQL(t *testing.T, query string) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(t.Context(), query).Scan(&count); err != nil {
		t.Fatalf("计数：%v", err)
	}
	return count
}

func mustWithinTX(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	fn func(context.Context) error,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务：%v", err)
	}
}

func synSourceIdentity(t *testing.T) psdomain.SourceIdentity {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		mustPS(t, psdomain.NewTenantID, "SYN-TENANT-01"),
		mustPS(t, psdomain.NewCustomerAccountID, "SYN-CUSTOMER-01"),
		mustPS(t, psdomain.NewSource, "SYN-SOURCE-01"),
		mustPS(t, psdomain.NewSourceRequestKey, "SYN-KEY-01"),
	)
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	return identity
}

func synJudgmentAsOf(t *testing.T, kind psdomain.JudgmentKind, at time.Time) psdomain.JudgmentAsOf {
	t.Helper()
	echoed, err := psdomain.NewEchoedAsOfPolicy(
		kind,
		mustPS(t, psdomain.NewAsOfSemanticsReference, "SYN-ASOF-"+kind.String()),
		mustPS(t, psdomain.NewAsOfPolicyVersion, "SYN-ASOF-POLICY-1"),
	)
	if err != nil {
		t.Fatalf("回显时点政策：%v", err)
	}
	asOf, err := psdomain.NewJudgmentAsOf(at, echoed)
	if err != nil {
		t.Fatalf("判断时点：%v", err)
	}
	return asOf
}

// synHeldControl 造一份「一项预付冻结成立」的采用结果——结论 HELD 只能由逐项推出（ADR-0125）。
func synHeldControl(t *testing.T, resultID string, asOf psdomain.JudgmentAsOf) psdomain.FinancialControlResult {
	t.Helper()
	item, err := psdomain.NewControlItemResult(
		psdomain.PrepaidFreezeControlItem, 1, psdomain.ControlItemSatisfied, psdomain.ControlBasisReference{})
	if err != nil {
		t.Fatalf("控制项结果：%v", err)
	}
	held, err := psdomain.NewExecutedFinancialControlResult(psdomain.ExecutedFinancialControlSpec{
		ResultID:  mustPS(t, psdomain.NewFinancialControlResultID, resultID),
		Items:     []psdomain.ControlItemResult{item},
		JointPass: psdomain.AllControlsPass,
		AsOf:      asOf,
	})
	if err != nil {
		t.Fatalf("财务控制：%v", err)
	}
	return held
}

func mustPS[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// synSProductionOwnership 是 SYN-S 归属替身：本范围由本产品承接。生产没有实现。
type synSProductionOwnership struct{ t *testing.T }

func (double *synSProductionOwnership) DecideProductionOwnership(
	_ context.Context,
	scope psdomain.AdmissionScope,
) (psdomain.ProductionOwnershipDecision, error) {
	double.t.Helper()
	now := time.Now().UTC()
	interval, err := psdomain.NewOwnershipValidityInterval(now.Add(-time.Hour), now.Add(30*24*time.Hour))
	if err != nil {
		double.t.Fatalf("归属有效期：%v", err)
	}
	decision, err := psdomain.NewProductionOwnershipDecision(psdomain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustPS(double.t, psdomain.NewProductionOwnershipDecisionID, "SYN-OWN-DEC-01"),
		Scope:            scope,
		Authority:        psdomain.ProductionAuthorityIDPParcel,
		AdmissionControl: psdomain.AdmissionControlOpen,
		RuleVersion:      mustPS(double.t, psdomain.NewProductionOwnershipRuleVersion, "SYN-OWN-RULE-01"),
		AsOf:             now.Add(-time.Minute),
		Validity:         interval,
		Revision:         mustPS(double.t, psdomain.NewProductionOwnershipRevision, "SYN-OWN-REV-01"),
		DecisionAt:       now.Add(-time.Minute),
	})
	if err != nil {
		double.t.Fatalf("归属决定：%v", err)
	}
	return decision, nil
}

// synSCommercialBasis 是 SYN-S 商业依据替身：交回唯一适用快照，未跑 PC 持久化解析。
type synSCommercialBasis struct{ t *testing.T }

func (double *synSCommercialBasis) ResolveCommercialBasis(
	_ context.Context,
	_ psports.CommercialBasisQuery,
) (psports.CommercialBasisResolution, error) {
	double.t.Helper()
	return double.applicableSnapshot()
}

func (double *synSCommercialBasis) FormJudgmentAsOf(
	_ context.Context,
	_ psports.JudgmentAsOfQuery,
) (psports.JudgmentAsOfFormation, error) {
	return psports.JudgmentAsOfFormation{}, nil
}

func (double *synSCommercialBasis) RevalidateCommercialBasis(
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

func (double *synSCommercialBasis) applicableSnapshot() (psports.CommercialBasisResolution, error) {
	double.t.Helper()
	groups, err := psdomain.NewApplicableCheckGroups(
		psdomain.PreAcceptanceFinancialControlCheck,
		psdomain.NetworkReachabilityCheck,
	)
	if err != nil {
		double.t.Fatalf("适用校验组：%v", err)
	}
	snapshot, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID: mustPS(double.t, psdomain.NewCommercialResolutionID, "SYN-RES-01"),
		RulePackage:  mustPS(double.t, psdomain.NewRulePackageReference, "SYN-RULES-01/v1"),
		ViewRevision: mustPS(double.t, psdomain.NewCommercialViewRevision, "SYN-VIEW-01"),
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
	return psports.CommercialBasisResolution{
		Snapshot:      snapshot,
		Applicability: psdomain.CommerciallyApplicable,
	}, nil
}

func synDeclaredAsOf(t *testing.T, kind psdomain.JudgmentKind) psdomain.DeclaredAsOf {
	t.Helper()
	declared, err := psdomain.NewDeclaredAsOf(
		kind,
		mustPS(t, psdomain.NewAsOfSemanticsReference, "SYN-ASOF-"+kind.String()),
		mustPS(t, psdomain.NewAsOfPolicyVersion, "SYN-ASOF-POLICY-1"),
	)
	if err != nil {
		t.Fatalf("时点声明：%v", err)
	}
	return declared
}

// synSReachabilityRevalidator 是 SYN-S 可达性重校替身：一律仍然当前。
type synSReachabilityRevalidator struct{}

func (synSReachabilityRevalidator) RevalidateReachabilityJudgment(
	_ context.Context,
	_ psports.ReachabilityRevalidationQuery,
) (psports.ReachabilityRevalidation, error) {
	return psports.ReachabilityRevalidation{Outcome: psports.ReachabilityJudgmentStillCurrent}, nil
}

// synSControlRelease 是 SYN-S 资金释放替身：本路径走接受，不会被叫到。
type synSControlRelease struct{}

func (synSControlRelease) ReleasePreAcceptanceControl(
	_ context.Context,
	_ psports.ControlReleaseRequest,
) error {
	return nil
}

type synSSubmissionIdentities struct{}

func (synSSubmissionIdentities) NextSubmissionVersionID(_ context.Context) (psdomain.SubmissionVersionID, error) {
	return psdomain.NewSubmissionVersionID("SYN-VER-01")
}

func (synSSubmissionIdentities) NextAcceptanceDecisionTaskID(_ context.Context) (psdomain.AcceptanceDecisionTaskID, error) {
	return psdomain.NewAcceptanceDecisionTaskID("SYN-TASK-01")
}

type synSDecisionIdentities struct{}

func (synSDecisionIdentities) NextAcceptanceDecisionID(_ context.Context) (psdomain.AcceptanceDecisionID, error) {
	return psdomain.NewAcceptanceDecisionID(synV0DecisionID)
}

var (
	_ psports.ProductionOwnershipAuthority = (*synSProductionOwnership)(nil)
	_ psports.CommercialBasisResolver      = (*synSCommercialBasis)(nil)
	_ psports.ReachabilityRevalidator      = synSReachabilityRevalidator{}
	_ psports.PreAcceptanceControlRelease  = synSControlRelease{}
	_ psports.SubmissionIdentityFactory    = synSSubmissionIdentities{}
	_ psports.AcceptanceDecisionIdentity   = synSDecisionIdentities{}
	_ psports.Clock                        = systemClock{}
)
