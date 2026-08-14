package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证追踪投影意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺版本响亮报错。信封 ID 由投影版本认领。入队走 EnqueueOnce。

type projectionHandoffClock struct{ at time.Time }

func (clock projectionHandoffClock) Now() time.Time { return clock.at }

type projectionHandoffFixture struct {
	projections *adapter.Projections
	handoff     *adapter.OutboxProjectionHandoff
	transactor  bentoapp.Transactor
	pool        *pgxpool.Pool
}

func newProjectionHandoffFixture(t *testing.T) *projectionHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	projections, err := adapter.NewProjections(db)
	if err != nil {
		t.Fatalf("构造投影库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxProjectionHandoff(db, store, projectionHandoffClock{
		at: time.Date(2026, 8, 14, 17, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &projectionHandoffFixture{
		projections: projections,
		handoff:     handoff,
		transactor:  db.Transactor(),
		pool:        pool,
	}
}

func (fixture *projectionHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func projectionIntent(t *testing.T, tenant, version string) ports.ProjectionHandoffIntent {
	t.Helper()
	return ports.ProjectionHandoffIntent{
		TenantID: projectionValue(t, domain.NewTenantID, tenant),
		Projection: derivedProjection(t, version, []domain.MilestoneClassification{
			classifiedEntry(t, "scan/origin", "PICKED_UP"),
		}),
	}
}

func TestProjectionIntentCommitsAtomicallyWithTheRow(t *testing.T) {
	fixture := newProjectionHandoffFixture(t)
	ctx := t.Context()
	tenant := projectionValue(t, domain.NewTenantID, "tenant-a")
	intent := projectionIntent(t, "tenant-a", "projection-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.projections.Save(txCtx, tenant, intent.Projection); err != nil {
			return err
		}
		return fixture.handoff.HandOffProjection(txCtx, intent)
	})

	if _, exists, err := fixture.projections.FindCurrent(ctx, tenant, intent.Projection.Parcel()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countProjectionIntents(t, fixture.pool, "projection-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := projectionIntentType(t, fixture.pool, "projection-1"); got != "visibility-exception.tracking-projection.derived" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestProjectionIntentRollbackDropsBoth(t *testing.T) {
	fixture := newProjectionHandoffFixture(t)
	ctx := t.Context()
	tenant := projectionValue(t, domain.NewTenantID, "tenant-a")
	intent := projectionIntent(t, "tenant-a", "projection-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.projections.Save(txCtx, tenant, intent.Projection); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffProjection(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.projections.FindCurrent(ctx, tenant, intent.Projection.Parcel()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countProjectionIntents(t, fixture.pool, "projection-1"); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.projections.Save(txCtx, tenant, intent.Projection); err != nil {
			return err
		}
		return fixture.handoff.HandOffProjection(txCtx, intent)
	})
	if count := countProjectionIntents(t, fixture.pool, "projection-1"); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameProjectionIntentIsIdempotent(t *testing.T) {
	fixture := newProjectionHandoffFixture(t)
	ctx := t.Context()
	intent := projectionIntent(t, "tenant-a", "projection-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffProjection(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffProjection(txCtx, intent)
	})
	if count := countProjectionIntents(t, fixture.pool, "projection-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestProjectionIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newProjectionHandoffFixture(t)
	if err := fixture.handoff.HandOffProjection(t.Context(), projectionIntent(t, "tenant-a", "projection-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countProjectionIntents(t, fixture.pool, "projection-1"); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignProjectionIntentIsLoud(t *testing.T) {
	fixture := newProjectionHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffProjection(txCtx, ports.ProjectionHandoffIntent{
			TenantID: projectionValue(t, domain.NewTenantID, "tenant-a"),
		})
	}); err == nil {
		t.Fatal("缺投影版本的意图必须响亮报错")
	}
	if count := countProjectionIntents(t, fixture.pool, "projection-1"); count != 0 {
		t.Fatalf("异类意图入队了：%d 行", count)
	}
}

func countProjectionIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "visibility-exception.tracking-projection.derived",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func projectionIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
	t.Helper()
	var eventType string
	err := pool.QueryRow(t.Context(),
		`SELECT event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&eventType)
	if err != nil {
		t.Fatalf("读事件类型：%v", err)
	}
	return eventType
}
