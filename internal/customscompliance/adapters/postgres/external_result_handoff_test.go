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
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证外部结果接收意图：与业务行同一提交、回滚一并消失、
// 重发同一份、无事务拒、归属不上的记录响亮报错。入队走 EnqueueOnce。

type resultHandoffClock struct{ at time.Time }

func (clock resultHandoffClock) Now() time.Time { return clock.at }

type resultHandoffFixture struct {
	results    *adapter.ExternalResults
	handoff    *adapter.OutboxExternalResultHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newResultHandoffFixture(t *testing.T) *resultHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	results, err := adapter.NewExternalResults(db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxExternalResultHandoff(db, store, resultHandoffClock{
		at: time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &resultHandoffFixture{
		results:    results,
		handoff:    handoff,
		transactor: db.Transactor(),
		pool:       pool,
	}
}

func (fixture *resultHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func resultIntent(t *testing.T, tenant, sourceID, version string) ports.ExternalResultHandoffIntent {
	t.Helper()
	return ports.ExternalResultHandoffIntent{Record: attributedRecord(t, tenant, sourceID, version)}
}

func resultEventID(tenant, sourceID string) string {
	return tenant + "/" + sourceID
}

func TestExternalResultIntentCommitsAtomicallyWithTheRecord(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	ctx := t.Context()
	intent := resultIntent(t, "tenant-a", "resp-1", "submission-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.results.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	})

	if _, exists, err := fixture.results.FindByKey(ctx, intent.Record.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	eventID := resultEventID("tenant-a", "resp-1")
	if count := countResultIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := resultIntentType(t, fixture.pool, eventID); got != "customs-compliance.external-result.received" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestExternalResultIntentRollbackDropsBoth(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	ctx := t.Context()
	intent := resultIntent(t, "tenant-a", "resp-1", "submission-1")
	rollback := errors.New("回滚")
	eventID := resultEventID("tenant-a", "resp-1")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.results.Save(txCtx, intent.Record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffExternalResult(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.results.FindByKey(ctx, intent.Record.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countResultIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.results.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	})
	if count := countResultIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameExternalResultIntentIsIdempotent(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	ctx := t.Context()
	intent := resultIntent(t, "tenant-a", "resp-1", "submission-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	})
	if count := countResultIntents(t, fixture.pool, resultEventID("tenant-a", "resp-1")); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestExternalResultIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	if err := fixture.handoff.HandOffExternalResult(t.Context(), resultIntent(t, "tenant-a", "resp-1", "submission-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countResultIntents(t, fixture.pool, resultEventID("tenant-a", "resp-1")); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAnUnattributableExternalResultIntentIsLoud(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	ctx := t.Context()
	intent := ports.ExternalResultHandoffIntent{Record: unattributableRecord(t, "tenant-a", "resp-orphan")}

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	}); err == nil {
		t.Fatal("归属不上的记录必须响亮报错")
	}
	if count := countResultIntents(t, fixture.pool, resultEventID("tenant-a", "resp-orphan")); count != 0 {
		t.Fatalf("异类意图入队了：%d 行", count)
	}
}

func countResultIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.external-result.received",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func resultIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
