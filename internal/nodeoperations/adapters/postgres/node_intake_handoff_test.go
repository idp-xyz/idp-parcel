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

// 本文件对真实 PostgreSQL 16 证节点收寄意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺收寄键响亮报错。信封 ID 由收寄幂等键认领。入队走 EnqueueOnce。

type intakeHandoffClock struct{ at time.Time }

func (clock intakeHandoffClock) Now() time.Time { return clock.at }

type intakeHandoffFixture struct {
	receptions *adapter.Receptions
	handoff    *adapter.OutboxNodeIntakeHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newIntakeHandoffFixture(t *testing.T) *intakeHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxNodeIntakeHandoff(db, store, intakeHandoffClock{
		at: time.Date(2026, 8, 14, 20, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &intakeHandoffFixture{
		receptions: receptions, handoff: handoff, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *intakeHandoffFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func intakeIntent(t *testing.T, tenant, sourceID string) ports.NodeIntakeHandoffIntent {
	t.Helper()
	return ports.NodeIntakeHandoffIntent{Record: formedRecord(t, tenant, sourceID)}
}

func intakeEventID(tenant, sourceID string) string {
	return tenant + "/" + sourceID
}

func TestNodeIntakeIntentCommitsAtomicallyWithTheReception(t *testing.T) {
	fixture := newIntakeHandoffFixture(t)
	ctx := t.Context()
	intent := intakeIntent(t, "tenant-a", "delivery-1")
	eventID := intakeEventID("tenant-a", "delivery-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.receptions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffNodeIntake(txCtx, intent)
	})

	if _, exists, err := fixture.receptions.FindByKey(ctx, intent.Record.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countIntakeIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := intakeIntentType(t, fixture.pool, eventID); got != "node-operations.node-intake.formed" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestNodeIntakeIntentRollbackDropsBoth(t *testing.T) {
	fixture := newIntakeHandoffFixture(t)
	ctx := t.Context()
	intent := intakeIntent(t, "tenant-a", "delivery-1")
	eventID := intakeEventID("tenant-a", "delivery-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.receptions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffNodeIntake(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.receptions.FindByKey(ctx, intent.Record.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countIntakeIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.receptions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffNodeIntake(txCtx, intent)
	})
	if count := countIntakeIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameNodeIntakeIntentIsIdempotent(t *testing.T) {
	fixture := newIntakeHandoffFixture(t)
	ctx := t.Context()
	intent := intakeIntent(t, "tenant-a", "delivery-1")
	eventID := intakeEventID("tenant-a", "delivery-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffNodeIntake(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffNodeIntake(txCtx, intent)
	})
	if count := countIntakeIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestNodeIntakeIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newIntakeHandoffFixture(t)
	intent := intakeIntent(t, "tenant-a", "delivery-1")
	eventID := intakeEventID("tenant-a", "delivery-1")
	if err := fixture.handoff.HandOffNodeIntake(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countIntakeIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignNodeIntakeIntentIsLoud(t *testing.T) {
	fixture := newIntakeHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffNodeIntake(txCtx, ports.NodeIntakeHandoffIntent{
			Record: ports.ReceptionRecord{
				Key: ports.ReceptionKey{TenantID: ref(t, domain.NewTenantID, "tenant-a")},
			},
		})
	}); err == nil {
		t.Fatal("缺收寄键的意图必须响亮报错")
	}
}

func countIntakeIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "node-operations.node-intake.formed",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func intakeIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
