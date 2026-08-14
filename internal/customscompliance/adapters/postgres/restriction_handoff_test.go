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

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证监管限制意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺标识响亮报错。信封 ID 由限制标识认领。入队走 EnqueueOnce。

type restrictionHandoffClock struct{ at time.Time }

func (clock restrictionHandoffClock) Now() time.Time { return clock.at }

type restrictionHandoffFixture struct {
	restrictions *adapter.Restrictions
	handoff      *adapter.OutboxRestrictionHandoff
	transactor   bentoapp.Transactor
	pool         *pgxpool.Pool
}

func newRestrictionHandoffFixture(t *testing.T) *restrictionHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	restrictions, err := adapter.NewRestrictions(db)
	if err != nil {
		t.Fatalf("构造限制库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxRestrictionHandoff(db, store, restrictionHandoffClock{
		at: time.Date(2026, 8, 14, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &restrictionHandoffFixture{
		restrictions: restrictions,
		handoff:      handoff,
		transactor:   db.Transactor(),
		pool:         pool,
	}
}

func (fixture *restrictionHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func restrictionIntent(t *testing.T, tenant, id string) ports.RestrictionHandoffIntent {
	t.Helper()
	return ports.RestrictionHandoffIntent{
		TenantID:    crgValue(t, domain.NewTenantID, tenant),
		Restriction: establishedRestriction(t, id, "scope/parcel-1", []domain.GuardedAction{domain.OutboundRelease}),
	}
}

func TestRestrictionIntentCommitsAtomicallyWithTheRecord(t *testing.T) {
	fixture := newRestrictionHandoffFixture(t)
	ctx := t.Context()
	tenant := crgValue(t, domain.NewTenantID, "tenant-a")
	intent := restrictionIntent(t, "tenant-a", "restriction-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.restrictions.Save(txCtx, tenant, intent.Restriction); err != nil {
			return err
		}
		return fixture.handoff.HandOffRestriction(txCtx, intent)
	})

	if _, exists, err := fixture.restrictions.FindByID(ctx, tenant, intent.Restriction.ID()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countRestrictionIntents(t, fixture.pool, "restriction-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := restrictionIntentType(t, fixture.pool, "restriction-1"); got != "customs-compliance.regulatory-restriction.changed" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestRestrictionIntentRollbackDropsBoth(t *testing.T) {
	fixture := newRestrictionHandoffFixture(t)
	ctx := t.Context()
	tenant := crgValue(t, domain.NewTenantID, "tenant-a")
	intent := restrictionIntent(t, "tenant-a", "restriction-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.restrictions.Save(txCtx, tenant, intent.Restriction); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffRestriction(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.restrictions.FindByID(ctx, tenant, intent.Restriction.ID()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countRestrictionIntents(t, fixture.pool, "restriction-1"); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.restrictions.Save(txCtx, tenant, intent.Restriction); err != nil {
			return err
		}
		return fixture.handoff.HandOffRestriction(txCtx, intent)
	})
	if count := countRestrictionIntents(t, fixture.pool, "restriction-1"); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameRestrictionIntentIsIdempotent(t *testing.T) {
	fixture := newRestrictionHandoffFixture(t)
	ctx := t.Context()
	intent := restrictionIntent(t, "tenant-a", "restriction-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffRestriction(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffRestriction(txCtx, intent)
	})
	if count := countRestrictionIntents(t, fixture.pool, "restriction-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestRestrictionIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newRestrictionHandoffFixture(t)
	if err := fixture.handoff.HandOffRestriction(t.Context(), restrictionIntent(t, "tenant-a", "restriction-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countRestrictionIntents(t, fixture.pool, "restriction-1"); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignRestrictionIntentIsLoud(t *testing.T) {
	fixture := newRestrictionHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffRestriction(txCtx, ports.RestrictionHandoffIntent{
			TenantID: crgValue(t, domain.NewTenantID, "tenant-a"),
		})
	}); err == nil {
		t.Fatal("缺限制标识的意图必须响亮报错")
	}
	if count := countRestrictionIntents(t, fixture.pool, "restriction-1"); count != 0 {
		t.Fatalf("异类意图入队了：%d 行", count)
	}
}

func countRestrictionIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.regulatory-restriction.changed",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func restrictionIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
