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

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证评价结果意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺评价标识响亮报错。信封 ID 由评价标识认领。入队走 EnqueueOnce。

type evaluationHandoffClock struct{ at time.Time }

func (clock evaluationHandoffClock) Now() time.Time { return clock.at }

type evaluationHandoffFixture struct {
	evaluations *adapter.Evaluations
	handoff     *adapter.OutboxEvaluationHandoff
	transactor  bentoapp.Transactor
	pool        *pgxpool.Pool
}

func newEvaluationHandoffFixture(t *testing.T) *evaluationHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	evaluations, err := adapter.NewEvaluations(db)
	if err != nil {
		t.Fatalf("构造评价登记册：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxEvaluationHandoff(db, store, evaluationHandoffClock{
		at: time.Date(2026, 8, 14, 23, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &evaluationHandoffFixture{
		evaluations: evaluations, handoff: handoff, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *evaluationHandoffFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func evaluationIntent(t *testing.T) ports.EvaluationHandoffIntent {
	t.Helper()
	return ports.EvaluationHandoffIntent{Evaluation: evaluatedFixture(t, "eval-handoff", "tenant-a", "Z1")}
}

func TestEvaluationIntentCommitsAtomicallyWithTheEvaluation(t *testing.T) {
	fixture := newEvaluationHandoffFixture(t)
	ctx := t.Context()
	intent := evaluationIntent(t)
	eventID := intent.Evaluation.ID().String()

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.evaluations.Save(txCtx, intent.Evaluation); err != nil {
			return err
		}
		return fixture.handoff.HandOffEvaluation(txCtx, intent)
	})

	if _, exists, err := fixture.evaluations.FindByID(ctx, intent.Evaluation.ID()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countEvaluationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := evaluationIntentType(t, fixture.pool, eventID); got != "parcel-pricing.evaluation.recorded" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestEvaluationIntentRollbackDropsBoth(t *testing.T) {
	fixture := newEvaluationHandoffFixture(t)
	ctx := t.Context()
	intent := evaluationIntent(t)
	eventID := intent.Evaluation.ID().String()
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.evaluations.Save(txCtx, intent.Evaluation); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffEvaluation(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.evaluations.FindByID(ctx, intent.Evaluation.ID()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countEvaluationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.evaluations.Save(txCtx, intent.Evaluation); err != nil {
			return err
		}
		return fixture.handoff.HandOffEvaluation(txCtx, intent)
	})
	if count := countEvaluationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameEvaluationIntentIsIdempotent(t *testing.T) {
	fixture := newEvaluationHandoffFixture(t)
	ctx := t.Context()
	intent := evaluationIntent(t)
	eventID := intent.Evaluation.ID().String()

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffEvaluation(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffEvaluation(txCtx, intent)
	})
	if count := countEvaluationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestEvaluationIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newEvaluationHandoffFixture(t)
	intent := evaluationIntent(t)
	eventID := intent.Evaluation.ID().String()
	if err := fixture.handoff.HandOffEvaluation(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countEvaluationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignEvaluationIntentIsLoud(t *testing.T) {
	fixture := newEvaluationHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffEvaluation(txCtx, ports.EvaluationHandoffIntent{})
	}); err == nil {
		t.Fatal("缺评价标识的意图必须响亮报错")
	}
}

func countEvaluationIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "parcel-pricing.evaluation.recorded",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func evaluationIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
