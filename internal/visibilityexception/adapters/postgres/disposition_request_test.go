package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件对真实 PostgreSQL 16 证处置请求库的行为：交互历史（判断/答复/替代指向）
// 原样往返、「当前至多一份」由部分唯一索引结构性承担、替代先更旧行再插新行同一
// 事务越界、交互形状由迁移 CHECK 把关（IS NULL 显式分支）、租户隔离由 SQL 条件
// 承担。断言一律在事务闭包外。

var dispositionBaseAt = time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC)

type dispositionFixture struct {
	requests   *adapter.DispositionRequests
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newDispositionRequests(t *testing.T) *dispositionFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	requests, err := adapter.NewDispositionRequests(db)
	if err != nil {
		t.Fatalf("构造处置请求库：%v", err)
	}
	return &dispositionFixture{requests: requests, transactor: db.Transactor(), pool: pool}
}

func (fixture *dispositionFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func dispositionValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func sentRequest(t *testing.T, id string, intentVersion int) *domain.DispositionRequest {
	t.Helper()
	request, err := domain.SendDispositionRequest(domain.DispositionRequestSpec{
		ID:               dispositionValue(t, domain.NewDispositionRequestID, id),
		Case:             dispositionValue(t, domain.NewCaseID, "case-1"),
		Target:           domain.SourceNetworkRouting,
		Action:           dispositionValue(t, domain.NewRequestedActionReference, "REROUTE_REMAINING_JOURNEY"),
		Scope:            dispositionValue(t, domain.NewRequestScopeReference, "parcel-1/remaining"),
		Reason:           "route deviation confirmed",
		Evidence:         dispositionValue(t, domain.NewRequestEvidenceReference, "evidence/route-deviation-1"),
		IntentVersion:    intentVersion,
		SentAt:           dispositionBaseAt,
		AcceptanceWindow: dispositionBaseAt.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造处置请求：%v", err)
	}
	return request
}

func (fixture *dispositionFixture) save(t *testing.T, ctx context.Context, tenant string, request *domain.DispositionRequest) {
	t.Helper()
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.requests.Save(txCtx, dispositionValue(t, domain.NewTenantID, tenant), request)
	})
}

func (fixture *dispositionFixture) loadByID(t *testing.T, ctx context.Context, tenant, id string) *domain.DispositionRequest {
	t.Helper()
	request, exists, err := fixture.requests.FindByID(ctx,
		dispositionValue(t, domain.NewTenantID, tenant),
		dispositionValue(t, domain.NewDispositionRequestID, id))
	if err != nil || !exists {
		t.Fatalf("按标识读回：%v exists=%v", err, exists)
	}
	return request
}

func (fixture *dispositionFixture) findCurrent(t *testing.T, ctx context.Context, tenant string) (*domain.DispositionRequest, bool) {
	t.Helper()
	request, exists, err := fixture.requests.FindCurrent(ctx,
		dispositionValue(t, domain.NewTenantID, tenant),
		dispositionValue(t, domain.NewCaseID, "case-1"),
		dispositionValue(t, domain.NewRequestedActionReference, "REROUTE_REMAINING_JOURNEY"),
		dispositionValue(t, domain.NewRequestScopeReference, "parcel-1/remaining"))
	if err != nil {
		t.Fatalf("按幂等键读回：%v", err)
	}
	return request, exists
}

// TestRequestInteractionHistoryRoundTrips 证交互历史逐步往返：发送→判断回填→取消
// 答复，每步落库后重建，在重建出的对象上继续下一步——重建不重演，生命周期方法照常
// 把门（过期不吸收接受、答过不覆盖）。
func TestRequestInteractionHistoryRoundTrips(t *testing.T) {
	fixture := newDispositionRequests(t)
	ctx := t.Context()

	fixture.save(t, ctx, "tenant-a", sentRequest(t, "request-1", 1))

	sent := fixture.loadByID(t, ctx, "tenant-a", "request-1")
	if _, judged := sent.Judgment(); judged {
		t.Fatalf("刚发送的请求凭空长出判断")
	}
	if sent.IntentVersion() != 1 || sent.Target() != domain.SourceNetworkRouting {
		t.Fatalf("请求没原样读回：%+v", sent)
	}
	if err := sent.RecordSourceJudgment(domain.RequestPartiallyAccepted, dispositionBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("重建后记判断：%v", err)
	}
	fixture.save(t, ctx, "tenant-a", sent)

	judged := fixture.loadByID(t, ctx, "tenant-a", "request-1")
	if judgment, ok := judged.Judgment(); !ok || judgment != domain.RequestPartiallyAccepted {
		t.Fatalf("判断没原样读回")
	}
	if err := judged.RecordSourceJudgment(domain.RequestRefused, dispositionBaseAt.Add(2*time.Hour)); err == nil {
		t.Fatalf("读回的已判断请求又判了第二次")
	}
	if err := judged.RecordCancellationAnswer(domain.PartiallyCancelled, dispositionBaseAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("重建后记取消答复：%v", err)
	}
	fixture.save(t, ctx, "tenant-a", judged)

	answered := fixture.loadByID(t, ctx, "tenant-a", "request-1")
	if answer, ok := answered.CancellationOutcome(); !ok || answer != domain.PartiallyCancelled {
		t.Fatalf("取消答复没原样读回")
	}
}

// TestSupersessionCrossesCommitBoundaryTogether 证替代同笔越界：被替代者与后继同一
// 事务落库，FindCurrent 换手到后继、被替代者按标识仍可读且指回后继；回滚后一切如旧。
func TestSupersessionCrossesCommitBoundaryTogether(t *testing.T) {
	fixture := newDispositionRequests(t)
	ctx := t.Context()

	prior := sentRequest(t, "request-1", 1)
	fixture.save(t, ctx, "tenant-a", prior)

	successor := sentRequest(t, "request-2", 2)
	if err := prior.SupersedeWith(successor); err != nil {
		t.Fatalf("替代：%v", err)
	}

	// 先证回滚：两行都不动——被替代者在库里仍是当前。
	rollback := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.requests.SaveSupersession(txCtx,
			dispositionValue(t, domain.NewTenantID, "tenant-a"), prior, successor); err != nil {
			return err
		}
		return context.Canceled
	})
	if rollback == nil {
		t.Fatalf("事务该失败没失败")
	}
	current, exists := fixture.findCurrent(t, ctx, "tenant-a")
	if !exists || current.ID().String() != "request-1" {
		t.Fatalf("回滚后当前不再是被替代者：%v", current.ID())
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.requests.SaveSupersession(txCtx,
			dispositionValue(t, domain.NewTenantID, "tenant-a"), prior, successor)
	})

	current, exists = fixture.findCurrent(t, ctx, "tenant-a")
	if !exists || current.ID().String() != "request-2" || current.IntentVersion() != 2 {
		t.Fatalf("替代后当前没换手：%v", current.ID())
	}
	replaced := fixture.loadByID(t, ctx, "tenant-a", "request-1")
	supersededBy, superseded := replaced.SupersededBy()
	if !superseded || supersededBy.String() != "request-2" {
		t.Fatalf("被替代者没指回后继：%v", supersededBy)
	}
}

// TestOnlyOneCurrentPerKeyIsStructural 证「当前至多一份」是结构性的：绕过适配器给
// 同（租户+案件+动作+范围）裸插第二份未被替代的行，部分唯一索引拦下。
func TestOnlyOneCurrentPerKeyIsStructural(t *testing.T) {
	fixture := newDispositionRequests(t)
	ctx := t.Context()

	fixture.save(t, ctx, "tenant-a", sentRequest(t, "request-1", 1))

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.disposition_request
			(tenant_id, request_id, case_id, target_context, action_ref, scope_ref,
			 reason, evidence_ref, intent_version, sent_at)
		 VALUES ('tenant-a', 'request-rogue', 'case-1', 'NETWORK_ROUTING',
		         'REROUTE_REMAINING_JOURNEY', 'parcel-1/remaining', 'r', 'e', 1, now())`); err == nil {
		t.Fatalf("同键第二份当前请求被库接受了")
	}

	// 未判断的请求带取消答复：形状 CHECK（IS NULL 显式分支）拦下。
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.disposition_request
			(tenant_id, request_id, case_id, target_context, action_ref, scope_ref,
			 reason, evidence_ref, intent_version, sent_at, cancellation)
		 VALUES ('tenant-a', 'request-x', 'case-2', 'NETWORK_ROUTING',
		         'HOLD', 'parcel-2/all', 'r', 'e', 1, now(), 'PARTIALLY_CANCELLED')`); err == nil {
		t.Fatalf("未判断带取消答复的行被库接受了")
	}
}

// TestRequestsOfAnotherTenantAreInvisible 证租户隔离：同名标识与同名幂等键在另一
// 租户一律不可见。
func TestRequestsOfAnotherTenantAreInvisible(t *testing.T) {
	fixture := newDispositionRequests(t)
	ctx := t.Context()

	fixture.save(t, ctx, "tenant-a", sentRequest(t, "request-1", 1))

	if _, exists, err := fixture.requests.FindByID(ctx,
		dispositionValue(t, domain.NewTenantID, "tenant-b"),
		dispositionValue(t, domain.NewDispositionRequestID, "request-1")); err != nil || exists {
		t.Fatalf("跨租户按标识可见：err=%v exists=%v", err, exists)
	}
	if _, exists := fixture.findCurrent(t, ctx, "tenant-b"); exists {
		t.Fatalf("跨租户按幂等键可见")
	}

	// 另一租户同名键各自成行——部分唯一索引带租户维。
	fixture.save(t, ctx, "tenant-b", sentRequest(t, "request-1", 1))
}

// TestDispositionWritesRequireTransaction 证事务纪律：无事务写一律拒。
func TestDispositionWritesRequireTransaction(t *testing.T) {
	fixture := newDispositionRequests(t)
	ctx := t.Context()

	tenant := dispositionValue(t, domain.NewTenantID, "tenant-a")
	if err := fixture.requests.Save(ctx, tenant, sentRequest(t, "request-1", 1)); err == nil {
		t.Fatalf("无事务 Save 被接受了")
	}
	prior := sentRequest(t, "request-1", 1)
	successor := sentRequest(t, "request-2", 2)
	if err := prior.SupersedeWith(successor); err != nil {
		t.Fatalf("替代：%v", err)
	}
	if err := fixture.requests.SaveSupersession(ctx, tenant, prior, successor); err == nil {
		t.Fatalf("无事务 SaveSupersession 被接受了")
	}
}
