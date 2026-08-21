package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

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

// Covers: transactionalSubmission「答案即提交」的半边——编排给出业务性拒绝时事务照样
// 提交，Preserve 过的来源留在库里，重放因此答`已有结果`。「被拒的提交也保全来源」是
// 编排的既定语义，事务包裹不得把业务拒绝当错误回滚掉。
//
// 错误回滚的半边不在此重证：真链的五个依赖没有一个能被注入成「Preserve 之后必然报错」，
// 而 WithinTransaction 对返回错误即回滚是框架合同，PS 全部 Outbox 负向用例与 inbox
// 消费门都在真库上反复证过同一条。
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
