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

// 本文件对真实 PostgreSQL 16 证放行门禁核对意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺幂等键响亮报错。信封 ID 由门禁幂等键认领。入队走 EnqueueOnce。

type gateHandoffClock struct{ at time.Time }

func (clock gateHandoffClock) Now() time.Time { return clock.at }

type gateHandoffFixture struct {
	gates      *adapter.GateVerifications
	handoff    *adapter.OutboxGateVerificationHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newGateHandoffFixture(t *testing.T) *gateHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	gates, err := adapter.NewGateVerifications(db)
	if err != nil {
		t.Fatalf("构造门禁库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxGateVerificationHandoff(db, store, gateHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &gateHandoffFixture{gates: gates, handoff: handoff, transactor: db.Transactor(), pool: pool}
}

func (fixture *gateHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func gateIntent(t *testing.T) ports.GateVerificationHandoffIntent {
	t.Helper()
	gate, key := verifiedGate(t, []domain.PreconditionFinding{
		{Precondition: crgValue(t, domain.NewPreconditionReference, "duty-paid"), State: domain.PreconditionMet},
		{Precondition: crgValue(t, domain.NewPreconditionReference, "restriction-clear"), State: domain.PreconditionUnmet},
	})
	return ports.GateVerificationHandoffIntent{Key: key, Gate: gate}
}

func gateEventID(key ports.GateVerificationKey) string {
	return key.TenantID.String() + "/" + key.Scope.String() + "/" +
		key.Action.String() + "/" + key.Boundary.String() + "/" + key.Digest
}

func TestGateVerificationIntentCommitsAtomicallyWithTheRecord(t *testing.T) {
	fixture := newGateHandoffFixture(t)
	ctx := t.Context()
	intent := gateIntent(t)
	eventID := gateEventID(intent.Key)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.gates.Save(txCtx, intent.Key, intent.Gate); err != nil {
			return err
		}
		return fixture.handoff.HandOffGate(txCtx, intent)
	})

	if _, exists, err := fixture.gates.FindByKey(ctx, intent.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countGateIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := gateIntentType(t, fixture.pool, eventID); got != "customs-compliance.gate-verification.recorded" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestGateVerificationIntentRollbackDropsBoth(t *testing.T) {
	fixture := newGateHandoffFixture(t)
	ctx := t.Context()
	intent := gateIntent(t)
	eventID := gateEventID(intent.Key)
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.gates.Save(txCtx, intent.Key, intent.Gate); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffGate(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.gates.FindByKey(ctx, intent.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countGateIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.gates.Save(txCtx, intent.Key, intent.Gate); err != nil {
			return err
		}
		return fixture.handoff.HandOffGate(txCtx, intent)
	})
	if count := countGateIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameGateVerificationIntentIsIdempotent(t *testing.T) {
	fixture := newGateHandoffFixture(t)
	ctx := t.Context()
	intent := gateIntent(t)
	eventID := gateEventID(intent.Key)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffGate(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffGate(txCtx, intent)
	})
	if count := countGateIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestGateVerificationIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newGateHandoffFixture(t)
	intent := gateIntent(t)
	eventID := gateEventID(intent.Key)
	if err := fixture.handoff.HandOffGate(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countGateIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignGateVerificationIntentIsLoud(t *testing.T) {
	fixture := newGateHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffGate(txCtx, ports.GateVerificationHandoffIntent{
			Key: ports.GateVerificationKey{
				TenantID: crgValue(t, domain.NewTenantID, "tenant-a"),
			},
		})
	}); err == nil {
		t.Fatal("缺幂等键的意图必须响亮报错")
	}
}

func countGateIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.gate-verification.recorded",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func gateIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
