package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 审计票 13——`/shipment-requests` 的第二参是真编排：整条链在真实 PostgreSQL
// 上装得起来；治理目录未配置时归属如实答`权威未确定`、提交停在 OWNERSHIP_UNRESOLVED
// 且带着归属决定；来源保全确实落库——重放同一份输入答`已有结果`，证明首笔事务真的
// 提交了，而不是装配在某个替身上悄悄成立。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredSubmissionAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	submission, err := buildSubmissionOrchestration(db)
	if err != nil {
		t.Fatalf("装配提交编排：%v", err)
	}

	command := submissionCommand(t)
	first, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("首次提交：%v", err)
	}
	if got := first.Outcome(); got != shipmentapp.OutcomeOwnershipUnresolved {
		t.Fatalf("outcome = %v, want OWNERSHIP_UNRESOLVED——目录未配置时归属只能答未确定", got)
	}
	if _, has := first.OwnershipDecision(); !has {
		t.Fatalf("被拦下的提交要带归属决定，调用方才有续办引用")
	}

	replay, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放同一份输入：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.OutcomeExistingResult {
		t.Fatalf("outcome = %v, want EXISTING_RESULT——重放靠的是首笔事务已提交的来源保全", got)
	}
}

// Covers: 两段事务边界的第一段（简报「事务边界」；ADR-0081）——来源保全在自己的事务里
// 独立提交，编排随后给出业务性拒绝也动不了它，重放因此答`已有结果`。「被拒的提交也
// 保全来源」由 preservationBoundary 的边界位置兑现，不再依赖「答案即提交」的大事务。
//
// 建单事务回滚的半边不在此重证：WithinTransaction 对返回错误即回滚是框架合同，
// 建单与信封同事务的原子性由 `tests/bentocontract` 的 PBC-04/05/07 在真库上取证。
func TestARefusedSubmissionStillPreservesItsSource(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	submission, err := buildSubmissionOrchestration(db)
	if err != nil {
		t.Fatalf("装配提交编排：%v", err)
	}

	// 跨客户指名按编排规则不查库直接答`查无原委托`——它发生在 Preserve 之后，正好
	// 钉住「业务拒绝不回滚保全」这一格。
	command := submissionCommand(t)
	command.Link = shipmentapp.PriorRequestClaim{
		PriorIdentity:  otherIdentity(t),
		PriorRequestID: mustValue(t, domain.NewShipmentRequestID, "request-absent"),
		Kind:           domain.LinkWithdrawnResubmission,
	}
	blocked, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("指名跨客户原委托的提交：%v", err)
	}
	if got := blocked.Outcome(); got != shipmentapp.OutcomePriorRequestNotFound {
		t.Fatalf("outcome = %v, want PRIOR_REQUEST_NOT_FOUND", got)
	}

	replay, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放被拒提交：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.OutcomeExistingResult {
		t.Fatalf("outcome = %v, want EXISTING_RESULT——被拒的提交也保全了来源", got)
	}
}

// Covers: ADR-0081 出站半边的装配证据——本包的生产边界壳 submissionBoundary 携真实
// Outbox 意图适配器：建单成功即落下恰好一份「委托已提交」信封，重放同一份输入答
// `已有结果`且不入队第二份。原子性与信封内容由 `tests/bentocontract` 的 PBC-03/04/05
// 取证，但那边跑的是夹具副本；这里钉的是 cmd/parcel-api 自己的壳真的把信封接上了——
// 谁改坏本包的 submissionBoundary，夹具那边不会红，这里会。
//
// 归属权威用放行替身（隔离合成 `S`，不进生产装配）：真实治理桥在目录未配置时把提交
// 停在 OWNERSHIP_UNRESOLVED，建单一段走不到，而要取证的恰是建单一段。除这一读口外，
// 仓储、边界壳、Outbox Store、意图适配器与标识工厂全是生产实现。
func TestASubmittedRequestHandsOffExactlyOneEnvelope(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	sources, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		t.Fatalf("构造来源保全仓储：%v", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	clock := fixedClock{at: envelopeProofAnchor}
	handoff, err := pspostgres.NewOutboxShipmentRequestSubmittedHandoff(db, store, clock)
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	identities, err := psidentity.NewSubmissionIdentities()
	if err != nil {
		t.Fatalf("构造标识工厂：%v", err)
	}
	submission := shipmentapp.NewSubmitShipmentRequestHandler(
		preservationBoundary{transactor: db.Transactor(), inner: sources},
		submissionBoundary{transactor: db.Transactor(), inner: requests, handoff: handoff},
		permittingOwnership{anchor: envelopeProofAnchor},
		identities,
		clock,
	)

	command := submissionCommand(t)
	submitted, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("首次提交：%v", err)
	}
	if got := submitted.Outcome(); got != shipmentapp.OutcomeSubmitted {
		t.Fatalf("outcome = %v, want SUBMITTED——归属替身已放行，建单该成立", got)
	}
	if got := submittedEnvelopeCount(t, pool); got != 1 {
		t.Fatalf("建单后「委托已提交」信封 = %d 份, want 恰好 1", got)
	}

	replay, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放同一份输入：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.OutcomeExistingResult {
		t.Fatalf("outcome = %v, want EXISTING_RESULT", got)
	}
	if got := submittedEnvelopeCount(t, pool); got != 1 {
		t.Fatalf("重放后信封 = %d 份——重放不得入队第二份意图", got)
	}
}

// envelopeProofAnchor 是信封取证的固定时钟读数：门禁评估时刻必须落在归属替身声明的
// 有效区间内，真实时钟会让这层证据随运行时刻漂移。
var envelopeProofAnchor = time.Date(2026, 8, 21, 10, 0, 5, 0, time.UTC)

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// permittingOwnership 是放行的归属权威替身（只记 `S`，不进生产装配）：对任何拟受理
// 范围答「本产品即当前生产权威、准入开放」。修订与 submissionCommand 的期望修订同值、
// 有效区间罩住 envelopeProofAnchor，门禁因此确定性放行。
type permittingOwnership struct{ anchor time.Time }

func (authority permittingOwnership) DecideProductionOwnership(
	_ context.Context,
	scope domain.AdmissionScope,
) (domain.ProductionOwnershipDecision, error) {
	interval, err := domain.NewOwnershipValidityInterval(
		authority.anchor.Add(-time.Hour),
		authority.anchor.Add(24*time.Hour),
	)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	decisionID, err := domain.NewProductionOwnershipDecisionID("SYN-OWN-DEC-1")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	ruleVersion, err := domain.NewProductionOwnershipRuleVersion("SYN-OWN-RULE-1")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	revision, err := domain.NewProductionOwnershipRevision("syn-rev-1")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	return domain.NewProductionOwnershipDecision(domain.ProductionOwnershipDecisionSpec{
		DecisionID:       decisionID,
		Scope:            scope,
		Authority:        domain.ProductionAuthorityIDPParcel,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      ruleVersion,
		AsOf:             authority.anchor.Add(-time.Minute),
		Validity:         interval,
		Revision:         revision,
		DecisionAt:       authority.anchor.Add(-time.Minute),
	})
}

// submittedEnvelopeType 与意图适配器的类型常量同字面（`tests/bentocontract` 同款拍法）：
// 按类型统计而不只按 EventID，一个意外身份的第二份信封才逃不掉。
const submittedEnvelopeType = "parcel-shipment.shipment-request.submitted"

func submittedEnvelopeCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		submittedEnvelopeType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计信封行数：%v", err)
	}
	return count
}

func submissionCommand(t *testing.T) shipmentapp.SubmitShipmentRequestCommand {
	t.Helper()
	scope, err := domain.NewAdmissionScope(
		mustValue(t, domain.NewAdmissionScopeReference, "scope-ref-syn-1"),
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-syn-1"),
	)
	if err != nil {
		t.Fatalf("new admission scope: %v", err)
	}
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
		mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-1"),
		mustValue(t, domain.NewSource, "SYN-SOURCE-A"),
		mustValue(t, domain.NewSourceRequestKey, "SYN-KEY-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return shipmentapp.SubmitShipmentRequestCommand{
		Identity:          identity,
		PayloadDigest:     mustValue(t, domain.NewPayloadDigest, "syn-digest-1"),
		OccurredAt:        time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC),
		ReceivedAt:        time.Date(2026, 8, 21, 10, 0, 1, 0, time.UTC),
		BatchID:           mustValue(t, domain.NewSubmissionBatchID, "syn-batch-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "syn-request-1"),
		DeclaredParcelIDs: []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "syn-parcel-1")},
		AdmissionScope:    scope,
		ExpectedRevision:  mustValue(t, domain.NewProductionOwnershipRevision, "syn-rev-1"),
	}
}

// otherIdentity 是另一位客户的来源身份：跨客户指名按编排规则不查库直接答`查无原委托`。
func otherIdentity(t *testing.T) domain.SourceIdentity {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
		mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-2"),
		mustValue(t, domain.NewSource, "SYN-SOURCE-A"),
		mustValue(t, domain.NewSourceRequestKey, "SYN-KEY-2"),
	)
	if err != nil {
		t.Fatalf("new other source identity: %v", err)
	}
	return identity
}

func mustValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %T from %q: %v", value, raw, err)
	}
	return value
}
