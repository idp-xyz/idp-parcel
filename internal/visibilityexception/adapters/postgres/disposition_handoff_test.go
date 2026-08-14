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

// 本文件对真实 PostgreSQL 16 证处置请求发送意图：与业务行同一提交、回滚一并消失、
// 重发同一份、无事务拒、认不得的意图形状响亮报错。入队走 EnqueueOnce，不在适配器里
// 再写一遍先查后插。

type dispositionHandoffClock struct{ at time.Time }

func (clock dispositionHandoffClock) Now() time.Time { return clock.at }

type dispositionHandoffFixture struct {
	requests   *adapter.DispositionRequests
	handoff    *adapter.OutboxDispositionHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newDispositionHandoffFixture(t *testing.T) *dispositionHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	requests, err := adapter.NewDispositionRequests(db)
	if err != nil {
		t.Fatalf("构造处置请求库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxDispositionHandoff(db, store, dispositionHandoffClock{
		at: time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &dispositionHandoffFixture{
		requests:   requests,
		handoff:    handoff,
		transactor: db.Transactor(),
		pool:       pool,
	}
}

func (fixture *dispositionHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func dispositionIntent(t *testing.T, tenant, requestID string) ports.DispositionHandoffIntent {
	t.Helper()
	return ports.DispositionHandoffIntent{
		TenantID: dispositionValue(t, domain.NewTenantID, tenant),
		Request:  sentRequest(t, requestID, 1),
	}
}

func TestDispositionIntentCommitsAtomicallyWithTheRequest(t *testing.T) {
	fixture := newDispositionHandoffFixture(t)
	ctx := t.Context()
	tenant := dispositionValue(t, domain.NewTenantID, "tenant-a")
	intent := dispositionIntent(t, "tenant-a", "request-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Save(txCtx, tenant, intent.Request); err != nil {
			return err
		}
		return fixture.handoff.HandOffDispositionRequest(txCtx, intent)
	})

	if _, exists, err := fixture.requests.FindByID(ctx, tenant, intent.Request.ID()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countDispositionIntents(t, fixture.pool, "request-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := dispositionIntentType(t, fixture.pool, "request-1"); got != "visibility-exception.disposition-request.sent" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestDispositionIntentRollbackDropsBoth(t *testing.T) {
	fixture := newDispositionHandoffFixture(t)
	ctx := t.Context()
	tenant := dispositionValue(t, domain.NewTenantID, "tenant-a")
	intent := dispositionIntent(t, "tenant-a", "request-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Save(txCtx, tenant, intent.Request); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffDispositionRequest(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.requests.FindByID(ctx, tenant, intent.Request.ID()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countDispositionIntents(t, fixture.pool, "request-1"); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Save(txCtx, tenant, intent.Request); err != nil {
			return err
		}
		return fixture.handoff.HandOffDispositionRequest(txCtx, intent)
	})
	if count := countDispositionIntents(t, fixture.pool, "request-1"); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameDispositionIntentIsIdempotent(t *testing.T) {
	fixture := newDispositionHandoffFixture(t)
	ctx := t.Context()
	intent := dispositionIntent(t, "tenant-a", "request-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDispositionRequest(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDispositionRequest(txCtx, intent)
	})
	if count := countDispositionIntents(t, fixture.pool, "request-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestDispositionIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newDispositionHandoffFixture(t)
	if err := fixture.handoff.HandOffDispositionRequest(t.Context(), dispositionIntent(t, "tenant-a", "request-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countDispositionIntents(t, fixture.pool, "request-1"); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignDispositionIntentIsLoud(t *testing.T) {
	fixture := newDispositionHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDispositionRequest(txCtx, ports.DispositionHandoffIntent{
			TenantID: dispositionValue(t, domain.NewTenantID, "tenant-a"),
		})
	}); err == nil {
		t.Fatal("缺请求的意图必须响亮报错")
	}
	if count := countDispositionIntents(t, fixture.pool, "request-1"); count != 0 {
		t.Fatalf("异类意图入队了：%d 行", count)
	}
}

func countDispositionIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "visibility-exception.disposition-request.sent",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func dispositionIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
