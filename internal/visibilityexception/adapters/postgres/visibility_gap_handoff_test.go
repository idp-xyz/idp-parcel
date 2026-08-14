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

// 本文件对真实 PostgreSQL 16 证可见性缺口意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺身份三维响亮报错。信封 ID 由缺口身份三维认领。入队走 EnqueueOnce。

type gapHandoffClock struct{ at time.Time }

func (clock gapHandoffClock) Now() time.Time { return clock.at }

type gapHandoffFixture struct {
	gaps       *adapter.VisibilityGaps
	handoff    *adapter.OutboxVisibilityGapHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newGapHandoffFixture(t *testing.T) *gapHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	gaps, err := adapter.NewVisibilityGaps(db)
	if err != nil {
		t.Fatalf("构造缺口库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxVisibilityGapHandoff(db, store, gapHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &gapHandoffFixture{gaps: gaps, handoff: handoff, transactor: db.Transactor(), pool: pool}
}

func (fixture *gapHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func gapIntent(t *testing.T, tenant string) ports.VisibilityGapHandoffIntent {
	t.Helper()
	return ports.VisibilityGapHandoffIntent{
		TenantID: etaGapValue(t, domain.NewTenantID, tenant),
		Gap:      formedGap(t, "window-rule/v1"),
	}
}

func gapEventID(gap domain.VisibilityGap) string {
	return gap.Parcel().String() + "/" + gap.Expectation().String() + "/" + gap.WindowRule().String()
}

func TestVisibilityGapIntentCommitsAtomicallyWithTheGap(t *testing.T) {
	fixture := newGapHandoffFixture(t)
	ctx := t.Context()
	tenant := etaGapValue(t, domain.NewTenantID, "tenant-a")
	intent := gapIntent(t, "tenant-a")
	eventID := gapEventID(intent.Gap)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.gaps.Save(txCtx, tenant, intent.Gap); err != nil {
			return err
		}
		return fixture.handoff.HandOffVisibilityGap(txCtx, intent)
	})

	if _, exists, err := fixture.gaps.FindCurrent(
		ctx, tenant, intent.Gap.Parcel(), intent.Gap.Expectation(), intent.Gap.WindowRule(),
	); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countGapIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := gapIntentType(t, fixture.pool, eventID); got != "visibility-exception.visibility-gap.formed" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestVisibilityGapIntentRollbackDropsBoth(t *testing.T) {
	fixture := newGapHandoffFixture(t)
	ctx := t.Context()
	tenant := etaGapValue(t, domain.NewTenantID, "tenant-a")
	intent := gapIntent(t, "tenant-a")
	eventID := gapEventID(intent.Gap)
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.gaps.Save(txCtx, tenant, intent.Gap); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffVisibilityGap(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.gaps.FindCurrent(
		ctx, tenant, intent.Gap.Parcel(), intent.Gap.Expectation(), intent.Gap.WindowRule(),
	); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countGapIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.gaps.Save(txCtx, tenant, intent.Gap); err != nil {
			return err
		}
		return fixture.handoff.HandOffVisibilityGap(txCtx, intent)
	})
	if count := countGapIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameVisibilityGapIntentIsIdempotent(t *testing.T) {
	fixture := newGapHandoffFixture(t)
	ctx := t.Context()
	intent := gapIntent(t, "tenant-a")
	eventID := gapEventID(intent.Gap)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffVisibilityGap(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffVisibilityGap(txCtx, intent)
	})
	if count := countGapIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestVisibilityGapIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newGapHandoffFixture(t)
	intent := gapIntent(t, "tenant-a")
	eventID := gapEventID(intent.Gap)
	if err := fixture.handoff.HandOffVisibilityGap(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countGapIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignVisibilityGapIntentIsLoud(t *testing.T) {
	fixture := newGapHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffVisibilityGap(txCtx, ports.VisibilityGapHandoffIntent{
			TenantID: etaGapValue(t, domain.NewTenantID, "tenant-a"),
		})
	}); err == nil {
		t.Fatal("缺身份三维的意图必须响亮报错")
	}
}

func countGapIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "visibility-exception.visibility-gap.formed",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func gapIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
