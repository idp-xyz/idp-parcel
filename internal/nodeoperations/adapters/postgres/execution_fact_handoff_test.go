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

// 本文件对真实 PostgreSQL 16 证执行事实意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺幂等键响亮报错。信封 ID 由执行事实幂等键认领。入队走 EnqueueOnce。

type executionFactHandoffClock struct{ at time.Time }

func (clock executionFactHandoffClock) Now() time.Time { return clock.at }

type executionFactHandoffFixture struct {
	facts      *adapter.ExecutionFacts
	handoff    *adapter.OutboxExecutionFactHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newExecutionFactHandoffFixture(t *testing.T) *executionFactHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	facts, err := adapter.NewExecutionFacts(db)
	if err != nil {
		t.Fatalf("构造执行事实库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxExecutionFactHandoff(db, store, executionFactHandoffClock{
		at: time.Date(2026, 8, 14, 22, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &executionFactHandoffFixture{
		facts: facts, handoff: handoff, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *executionFactHandoffFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func executionFactIntent(t *testing.T) ports.ExecutionFactHandoffIntent {
	t.Helper()
	return ports.ExecutionFactHandoffIntent{
		Record: executionRecord(t, "tenant-a", "item-1", "unit-1", domain.UnsealAction, "evidence-1"),
	}
}

func executionFactEventID() string {
	return "tenant-a/item-1/unit-1/UNSEAL"
}

func TestExecutionFactIntentCommitsAtomicallyWithTheFact(t *testing.T) {
	fixture := newExecutionFactHandoffFixture(t)
	ctx := t.Context()
	intent := executionFactIntent(t)
	eventID := executionFactEventID()

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.facts.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffExecutionFact(txCtx, intent)
	})

	if _, exists, err := fixture.facts.FindByKey(ctx, intent.Record.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countExecutionFactIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := executionFactIntentType(t, fixture.pool, eventID); got != "node-operations.execution-fact.recorded" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestExecutionFactIntentRollbackDropsBoth(t *testing.T) {
	fixture := newExecutionFactHandoffFixture(t)
	ctx := t.Context()
	intent := executionFactIntent(t)
	eventID := executionFactEventID()
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.facts.Save(txCtx, intent.Record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffExecutionFact(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.facts.FindByKey(ctx, intent.Record.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countExecutionFactIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.facts.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffExecutionFact(txCtx, intent)
	})
	if count := countExecutionFactIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameExecutionFactIntentIsIdempotent(t *testing.T) {
	fixture := newExecutionFactHandoffFixture(t)
	ctx := t.Context()
	intent := executionFactIntent(t)
	eventID := executionFactEventID()

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffExecutionFact(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffExecutionFact(txCtx, intent)
	})
	if count := countExecutionFactIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestExecutionFactIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newExecutionFactHandoffFixture(t)
	intent := executionFactIntent(t)
	eventID := executionFactEventID()
	if err := fixture.handoff.HandOffExecutionFact(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countExecutionFactIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignExecutionFactIntentIsLoud(t *testing.T) {
	fixture := newExecutionFactHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffExecutionFact(txCtx, ports.ExecutionFactHandoffIntent{
			Record: ports.ExecutionFactRecord{
				Key: ports.ExecutionFactKey{TenantID: ref(t, domain.NewTenantID, "tenant-a")},
			},
		})
	}); err == nil {
		t.Fatal("缺执行事实键的意图必须响亮报错")
	}
}

func countExecutionFactIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "node-operations.execution-fact.recorded",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func executionFactIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
