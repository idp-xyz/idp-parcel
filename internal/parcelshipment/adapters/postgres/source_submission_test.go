package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证来源保全适配器的行为。用真实引擎而非替身，是因为
// 要证的恰好是替身给不出的那几件：作用域隔离由 SQL 条件承担、重复保全由主键拦住、
// 事务缺失由框架的 RequireExecutor 拦住。

func TestPreservedSourceIsReadBackUnchanged(t *testing.T) {
	repository, transactor, _ := newSourceSubmissions(t)
	ctx := t.Context()

	preserved := fingerprint(t, "tenant-a", "customer-a", "portal", "req-1", "digest-1")

	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Preserve(txCtx, preserved)
	})

	found, exists, err := repository.FindPreserved(ctx, preserved.Identity())
	if err != nil {
		t.Fatalf("取回已保全来源：%v", err)
	}
	if !exists {
		t.Fatal("已保全的来源读不回来")
	}
	if found != preserved {
		t.Errorf("读回 %+v，保全的是 %+v", found, preserved)
	}
}

// TestOtherScopesAreInvisible 证否定结果不泄露其他作用域是否存在该对象。外部键相同
// 但租户或客户账户不同，必须与「不存在」长得完全一样。
func TestOtherScopesAreInvisible(t *testing.T) {
	repository, transactor, _ := newSourceSubmissions(t)
	ctx := t.Context()

	preserved := fingerprint(t, "tenant-a", "customer-a", "portal", "shared-key", "digest-1")
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Preserve(txCtx, preserved)
	})

	elsewhere := map[string]domain.SourceIdentity{
		"另一个租户":   identity(t, "tenant-b", "customer-a", "portal", "shared-key"),
		"另一个客户账户": identity(t, "tenant-a", "customer-b", "portal", "shared-key"),
		"另一个来源":   identity(t, "tenant-a", "customer-a", "api", "shared-key"),
		"另一个请求键":  identity(t, "tenant-a", "customer-a", "portal", "other-key"),
	}
	for name, scope := range elsewhere {
		_, exists, err := repository.FindPreserved(ctx, scope)
		if err != nil {
			t.Fatalf("%s：查询出错 %v", name, err)
		}
		if exists {
			t.Errorf("%s：读到了不属于该作用域的记录", name)
		}
	}
}

// TestPreservingTwiceDoesNotRewriteHistory 证并发竞态下后到者拿到错误而不是覆盖。
// 覆盖会毁掉先到者已保全的原始事实，而那正是来源保全这一步存在的理由。
func TestPreservingTwiceDoesNotRewriteHistory(t *testing.T) {
	repository, transactor, _ := newSourceSubmissions(t)
	ctx := t.Context()

	first := fingerprint(t, "tenant-a", "customer-a", "portal", "req-1", "digest-first")
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Preserve(txCtx, first)
	})

	second := fingerprint(t, "tenant-a", "customer-a", "portal", "req-1", "digest-second")
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repository.Preserve(txCtx, second)
	})
	if !errors.Is(err, adapter.ErrAlreadyPreserved) {
		t.Fatalf("重复保全应返回 ErrAlreadyPreserved，实得：%v", err)
	}

	found, exists, err := repository.FindPreserved(ctx, first.Identity())
	if err != nil || !exists {
		t.Fatalf("取回已保全来源：%v exists=%v", err, exists)
	}
	if found != first {
		t.Errorf("先到者的事实被改写：读回 %+v，应为 %+v", found, first)
	}
}

// TestObservationDoesNotTouchThePreservedFact 证追加观察不改写已保全那一行。
// 不同的 occurredAt/receivedAt 是新的观察，不是对历史的更正。
func TestObservationDoesNotTouchThePreservedFact(t *testing.T) {
	repository, transactor, pool := newSourceSubmissions(t)
	ctx := t.Context()

	preserved := fingerprint(t, "tenant-a", "customer-a", "portal", "req-1", "digest-1")
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Preserve(txCtx, preserved)
	})

	// 同键同摘要但时间不同：按用例这仍是重放，只追加本次观察。
	later, err := domain.NewSourceSubmissionFingerprint(
		preserved.Identity(),
		preserved.Digest(),
		preserved.OccurredAt().Add(2*time.Hour),
		preserved.ReceivedAt().Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("构造后续观察：%v", err)
	}
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.AppendObservation(txCtx, later)
	})
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.AppendObservation(txCtx, later)
	})

	found, exists, err := repository.FindPreserved(ctx, preserved.Identity())
	if err != nil || !exists {
		t.Fatalf("取回已保全来源：%v exists=%v", err, exists)
	}
	if found != preserved {
		t.Errorf("追加观察改写了已保全事实：读回 %+v，应为 %+v", found, preserved)
	}
	if count := countObservations(t, pool, preserved.Identity()); count != 2 {
		t.Errorf("追加了两次观察，库里有 %d 条；观察必须逐条留存而不是相互覆盖", count)
	}
}

// TestWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用连接池。
// 这是框架 RequireExecutor 的保证，也是 `PBC-08`「不绕过 DB.RequireExecutor」那一条
// 的行为面：绕过了，业务写入与 Outbox 就不再同生共死，而两端的测试都看不出来。
func TestWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newSourceSubmissions(t)
	ctx := t.Context()

	submission := fingerprint(t, "tenant-a", "customer-a", "portal", "req-1", "digest-1")

	if err := repository.Preserve(ctx, submission); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保全应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := repository.AppendObservation(ctx, submission); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加观察应返回 ErrTransactionRequired，实得：%v", err)
	}

	_, exists, err := repository.FindPreserved(ctx, submission.Identity())
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("被拒绝的写入仍然落库了")
	}
}

// TestRollbackLeavesNothingBehind 证保全与它所在的事务同生共死。
func TestRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor, _ := newSourceSubmissions(t)
	ctx := t.Context()

	submission := fingerprint(t, "tenant-a", "customer-a", "portal", "req-1", "digest-1")
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := repository.Preserve(txCtx, submission); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, exists, err := repository.FindPreserved(ctx, submission.Identity())
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("回滚后已保全记录仍在")
	}
}

func newSourceSubmissions(t *testing.T) (*adapter.SourceSubmissions, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)

	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewSourceSubmissions(db)
	if err != nil {
		t.Fatalf("构造来源仓储：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustWithinTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	fn func(context.Context) error,
) {
	t.Helper()

	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func countObservations(t *testing.T, pool *pgxpool.Pool, identity domain.SourceIdentity) int {
	t.Helper()

	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_shipment.source_submission_observation
		  WHERE tenant_id = $1 AND customer_account_id = $2
		    AND source = $3 AND source_request_key = $4`,
		identity.TenantID().String(),
		identity.CustomerAccountID().String(),
		identity.Source().String(),
		identity.RequestKey().String(),
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计观察条数：%v", err)
	}
	return count
}

func identity(t *testing.T, tenant, customer, source, key string) domain.SourceIdentity {
	t.Helper()

	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		t.Fatalf("租户标识：%v", err)
	}
	customerID, err := domain.NewCustomerAccountID(customer)
	if err != nil {
		t.Fatalf("客户账户标识：%v", err)
	}
	sourceValue, err := domain.NewSource(source)
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	requestKey, err := domain.NewSourceRequestKey(key)
	if err != nil {
		t.Fatalf("来源请求键：%v", err)
	}
	built, err := domain.NewSourceIdentity(tenantID, customerID, sourceValue, requestKey)
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	return built
}

func fingerprint(t *testing.T, tenant, customer, source, key, digest string) domain.SourceSubmissionFingerprint {
	t.Helper()

	payloadDigest, err := domain.NewPayloadDigest(digest)
	if err != nil {
		t.Fatalf("内容摘要：%v", err)
	}
	occurredAt := time.Date(2026, 8, 11, 9, 30, 0, 0, time.UTC)
	built, err := domain.NewSourceSubmissionFingerprint(
		identity(t, tenant, customer, source, key),
		payloadDigest,
		occurredAt,
		occurredAt.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("来源指纹：%v", err)
	}
	return built
}
