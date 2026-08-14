package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证交接登记册：首份指纹往返、撞关联不换写（冲突不会被
// 抹成重放）、未登记只回 false、作用域隔离、无事务拒、回滚无痕。

func TestAHandoffDigestRoundTrips(t *testing.T) {
	log, transactor, _ := newRouteHandoffLog(t)
	ctx := t.Context()

	mustAppendDigest(t, transactor, ctx, log, "tenant-1", "corr-1", "digest-a")

	digest, found, err := log.FindDigest(ctx,
		logValue(t, domain.NewTenantID, "tenant-1"),
		logValue(t, domain.NewRequestCorrelationID, "corr-1"))
	if err != nil {
		t.Fatalf("取回指纹：%v", err)
	}
	if !found {
		t.Fatal("已登记的指纹读不回来")
	}
	if digest != "digest-a" {
		t.Fatalf("digest = %q, want digest-a", digest)
	}
}

// TestAnUnloggedCorrelationIsNotAnError 证「这份交接头一次来」只回 false：编排据此
// 走首次受理，与「登记册读不到」（error → 未决）是两条不同的路。
func TestAnUnloggedCorrelationIsNotAnError(t *testing.T) {
	log, _, _ := newRouteHandoffLog(t)

	digest, found, err := log.FindDigest(t.Context(),
		logValue(t, domain.NewTenantID, "tenant-1"),
		logValue(t, domain.NewRequestCorrelationID, "corr-absent"))
	if err != nil {
		t.Fatalf("未登记不该报错，实得：%v", err)
	}
	if found || digest != "" {
		t.Fatalf("未登记却答 found=%v digest=%q", found, digest)
	}
}

// TestASecondAppendDoesNotOverwriteTheFirst 是这张表的要害：换写会把一次冲突悄悄
// 变成重放——第二份异指纹压掉首份之后，编排再比对就永远相等。
func TestASecondAppendDoesNotOverwriteTheFirst(t *testing.T) {
	log, transactor, _ := newRouteHandoffLog(t)
	ctx := t.Context()

	mustAppendDigest(t, transactor, ctx, log, "tenant-1", "corr-1", "digest-a")
	mustAppendDigest(t, transactor, ctx, log, "tenant-1", "corr-1", "digest-b")

	digest, found, err := log.FindDigest(ctx,
		logValue(t, domain.NewTenantID, "tenant-1"),
		logValue(t, domain.NewRequestCorrelationID, "corr-1"))
	if err != nil || !found {
		t.Fatalf("读回指纹：%v found=%v", err, found)
	}
	if digest != "digest-a" {
		t.Fatalf("digest = %q，后到的异指纹覆盖了首份——冲突被抹成了重放", digest)
	}
}

func TestHandoffLogScopesAreInvisibleToEachOther(t *testing.T) {
	log, transactor, _ := newRouteHandoffLog(t)
	ctx := t.Context()

	mustAppendDigest(t, transactor, ctx, log, "tenant-1", "corr-1", "digest-a")

	_, found, err := log.FindDigest(ctx,
		logValue(t, domain.NewTenantID, "tenant-b"),
		logValue(t, domain.NewRequestCorrelationID, "corr-1"))
	if err != nil {
		t.Fatalf("他租户查询出错：%v", err)
	}
	if found {
		t.Error("他租户读到了本租户的交接登记")
	}
}

// TestABlankDigestIsRefused 证空指纹进不去：一份「登记过但没有内容」的行会让下一次
// 比对拿空串当依据。领域构造门在这条链上不经手指纹，所以适配器与库各守一道。
func TestABlankDigestIsRefused(t *testing.T) {
	log, transactor, pool := newRouteHandoffLog(t)
	ctx := t.Context()

	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return log.Append(txCtx,
			logValue(t, domain.NewTenantID, "tenant-1"),
			logValue(t, domain.NewRequestCorrelationID, "corr-1"),
			"")
	})
	if err == nil {
		t.Fatal("空指纹被登记了")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_handoff_log (tenant_id, correlation_id, digest)
		 VALUES ('tenant-1', 'corr-1', '   ')`); err == nil {
		t.Fatal("绕过适配器的空白指纹进了库")
	}
}

func TestHandoffLogWritesRefuseToRunOutsideATransaction(t *testing.T) {
	log, _, _ := newRouteHandoffLog(t)

	err := log.Append(t.Context(),
		logValue(t, domain.NewTenantID, "tenant-1"),
		logValue(t, domain.NewRequestCorrelationID, "corr-1"),
		"digest-a")
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestHandoffLogRollbackLeavesNothingBehind(t *testing.T) {
	log, transactor, _ := newRouteHandoffLog(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := log.Append(txCtx,
			logValue(t, domain.NewTenantID, "tenant-1"),
			logValue(t, domain.NewRequestCorrelationID, "corr-1"),
			"digest-a"); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, found, err := log.FindDigest(ctx,
		logValue(t, domain.NewTenantID, "tenant-1"),
		logValue(t, domain.NewRequestCorrelationID, "corr-1"))
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if found {
		t.Error("回滚后登记仍在")
	}
}

// ---- 夹具 ----

func newRouteHandoffLog(t *testing.T) (*adapter.RouteHandoffLogs, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	log, err := adapter.NewRouteHandoffLogs(db)
	if err != nil {
		t.Fatalf("构造交接登记册：%v", err)
	}
	return log, db.Transactor(), pool
}

func mustAppendDigest(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	log *adapter.RouteHandoffLogs,
	tenant, correlation, digest string,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return log.Append(txCtx,
			logValue(t, domain.NewTenantID, tenant),
			logValue(t, domain.NewRequestCorrelationID, correlation),
			digest)
	}); err != nil {
		t.Fatalf("登记指纹：%v", err)
	}
}

func logValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}
