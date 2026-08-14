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

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证承接决定意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺承接键响亮报错。信封 ID 由承接幂等键认领。入队走 EnqueueOnce。

type acceptanceHandoffClock struct{ at time.Time }

func (clock acceptanceHandoffClock) Now() time.Time { return clock.at }

type acceptanceHandoffFixture struct {
	acceptances *adapter.CollaborationAcceptances
	handoff     *adapter.OutboxCollaborationAcceptanceHandoff
	transactor  bentoapp.Transactor
	pool        *pgxpool.Pool
}

func newAcceptanceHandoffFixture(t *testing.T) *acceptanceHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	acceptances, err := adapter.NewCollaborationAcceptances(db)
	if err != nil {
		t.Fatalf("构造承接库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxCollaborationAcceptanceHandoff(db, store, acceptanceHandoffClock{
		at: time.Date(2026, 8, 14, 20, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &acceptanceHandoffFixture{
		acceptances: acceptances, handoff: handoff, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *acceptanceHandoffFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func acceptanceIntent(t *testing.T, tenant, item string) ports.CollaborationAcceptanceHandoffIntent {
	t.Helper()
	return ports.CollaborationAcceptanceHandoffIntent{Record: acceptedRecord(t, tenant, item)}
}

func acceptanceEventID(tenant, item string) string {
	return tenant + "/" + item
}

func TestCollaborationAcceptanceIntentCommitsAtomicallyWithTheDecision(t *testing.T) {
	fixture := newAcceptanceHandoffFixture(t)
	ctx := t.Context()
	intent := acceptanceIntent(t, "tenant-a", "item-1")
	eventID := acceptanceEventID("tenant-a", "item-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.acceptances.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffCollaborationAcceptance(txCtx, intent)
	})

	if _, exists, err := fixture.acceptances.FindByKey(ctx, intent.Record.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countAcceptanceIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := acceptanceIntentType(t, fixture.pool, eventID); got != "node-operations.collaboration-acceptance.decided" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestCollaborationAcceptanceIntentRollbackDropsBoth(t *testing.T) {
	fixture := newAcceptanceHandoffFixture(t)
	ctx := t.Context()
	intent := acceptanceIntent(t, "tenant-a", "item-1")
	eventID := acceptanceEventID("tenant-a", "item-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.acceptances.Save(txCtx, intent.Record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffCollaborationAcceptance(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.acceptances.FindByKey(ctx, intent.Record.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countAcceptanceIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.acceptances.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffCollaborationAcceptance(txCtx, intent)
	})
	if count := countAcceptanceIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameCollaborationAcceptanceIntentIsIdempotent(t *testing.T) {
	fixture := newAcceptanceHandoffFixture(t)
	ctx := t.Context()
	intent := acceptanceIntent(t, "tenant-a", "item-1")
	eventID := acceptanceEventID("tenant-a", "item-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffCollaborationAcceptance(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffCollaborationAcceptance(txCtx, intent)
	})
	if count := countAcceptanceIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestCollaborationAcceptanceIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newAcceptanceHandoffFixture(t)
	intent := acceptanceIntent(t, "tenant-a", "item-1")
	eventID := acceptanceEventID("tenant-a", "item-1")
	if err := fixture.handoff.HandOffCollaborationAcceptance(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countAcceptanceIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignCollaborationAcceptanceIntentIsLoud(t *testing.T) {
	fixture := newAcceptanceHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffCollaborationAcceptance(txCtx, ports.CollaborationAcceptanceHandoffIntent{
			Record: ports.CollaborationAcceptanceRecord{
				Key: ports.CollaborationAcceptanceKey{TenantID: ref(t, domain.NewTenantID, "tenant-a")},
			},
		})
	}); err == nil {
		t.Fatal("缺承接键的意图必须响亮报错")
	}
}

func countAcceptanceIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "node-operations.collaboration-acceptance.decided",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func acceptanceIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
