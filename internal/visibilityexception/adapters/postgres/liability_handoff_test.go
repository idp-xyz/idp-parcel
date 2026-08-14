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

// 本文件对真实 PostgreSQL 16 证责任结论意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺索赔响亮报错。信封 ID 由索赔项标识认领。入队走 EnqueueOnce。

type liabilityHandoffClock struct{ at time.Time }

func (clock liabilityHandoffClock) Now() time.Time { return clock.at }

type liabilityHandoffFixture struct {
	claims     *adapter.Claims
	handoff    *adapter.OutboxLiabilityHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newLiabilityHandoffFixture(t *testing.T) *liabilityHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	claims, err := adapter.NewClaims(db)
	if err != nil {
		t.Fatalf("构造索赔库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxLiabilityHandoff(db, store, liabilityHandoffClock{
		at: time.Date(2026, 8, 14, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &liabilityHandoffFixture{
		claims:     claims,
		handoff:    handoff,
		transactor: db.Transactor(),
		pool:       pool,
	}
}

func (fixture *liabilityHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func liabilityIntent(t *testing.T, tenant, batch, item string) ports.LiabilityHandoffIntent {
	t.Helper()
	return ports.LiabilityHandoffIntent{
		TenantID: claimValue(t, domain.NewTenantID, tenant),
		Claim:    receivedClaim(t, batch, item),
	}
}

func TestLiabilityIntentCommitsAtomicallyWithTheClaim(t *testing.T) {
	fixture := newLiabilityHandoffFixture(t)
	ctx := t.Context()
	tenant := claimValue(t, domain.NewTenantID, "tenant-a")
	intent := liabilityIntent(t, "tenant-a", "batch-1", "item-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.claims.Save(txCtx, tenant, intent.Claim); err != nil {
			return err
		}
		return fixture.handoff.HandOffLiability(txCtx, intent)
	})

	if _, exists, err := fixture.claims.FindByBatchItem(ctx, tenant, intent.Claim.Batch(), intent.Claim.ID()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countLiabilityIntents(t, fixture.pool, "item-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := liabilityIntentType(t, fixture.pool, "item-1"); got != "visibility-exception.claim-liability.concluded" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestLiabilityIntentRollbackDropsBoth(t *testing.T) {
	fixture := newLiabilityHandoffFixture(t)
	ctx := t.Context()
	tenant := claimValue(t, domain.NewTenantID, "tenant-a")
	intent := liabilityIntent(t, "tenant-a", "batch-1", "item-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.claims.Save(txCtx, tenant, intent.Claim); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffLiability(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.claims.FindByBatchItem(ctx, tenant, intent.Claim.Batch(), intent.Claim.ID()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countLiabilityIntents(t, fixture.pool, "item-1"); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.claims.Save(txCtx, tenant, intent.Claim); err != nil {
			return err
		}
		return fixture.handoff.HandOffLiability(txCtx, intent)
	})
	if count := countLiabilityIntents(t, fixture.pool, "item-1"); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameLiabilityIntentIsIdempotent(t *testing.T) {
	fixture := newLiabilityHandoffFixture(t)
	ctx := t.Context()
	intent := liabilityIntent(t, "tenant-a", "batch-1", "item-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffLiability(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffLiability(txCtx, intent)
	})
	if count := countLiabilityIntents(t, fixture.pool, "item-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestLiabilityIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newLiabilityHandoffFixture(t)
	if err := fixture.handoff.HandOffLiability(t.Context(), liabilityIntent(t, "tenant-a", "batch-1", "item-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countLiabilityIntents(t, fixture.pool, "item-1"); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignLiabilityIntentIsLoud(t *testing.T) {
	fixture := newLiabilityHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffLiability(txCtx, ports.LiabilityHandoffIntent{
			TenantID: claimValue(t, domain.NewTenantID, "tenant-a"),
		})
	}); err == nil {
		t.Fatal("缺索赔的意图必须响亮报错")
	}
	if count := countLiabilityIntents(t, fixture.pool, "item-1"); count != 0 {
		t.Fatalf("异类意图入队了：%d 行", count)
	}
}

func countLiabilityIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "visibility-exception.claim-liability.concluded",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func liabilityIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
