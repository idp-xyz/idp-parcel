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
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证客户视图意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺版本响亮报错。信封 ID 由视图版本认领。入队走 EnqueueOnce。

type customerViewHandoffClock struct{ at time.Time }

func (clock customerViewHandoffClock) Now() time.Time { return clock.at }

type customerViewHandoffFixture struct {
	views      *adapter.CustomerViews
	handoff    *adapter.OutboxCustomerViewHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newCustomerViewHandoffFixture(t *testing.T) *customerViewHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	views, err := adapter.NewCustomerViews(db)
	if err != nil {
		t.Fatalf("构造视图库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxCustomerViewHandoff(db, store, customerViewHandoffClock{
		at: time.Date(2026, 8, 14, 18, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &customerViewHandoffFixture{views: views, handoff: handoff, transactor: db.Transactor(), pool: pool}
}

func (fixture *customerViewHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func customerViewIntent(t *testing.T, tenant, version string) ports.CustomerViewHandoffIntent {
	t.Helper()
	return ports.CustomerViewHandoffIntent{
		TenantID: tenantOf(t, tenant),
		View:     firstView(t, version),
	}
}

func TestCustomerViewIntentCommitsAtomicallyWithTheView(t *testing.T) {
	fixture := newCustomerViewHandoffFixture(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")
	intent := customerViewIntent(t, "tenant-a", "view-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.views.Save(txCtx, tenant, intent.View); err != nil {
			return err
		}
		return fixture.handoff.HandOffCustomerView(txCtx, intent)
	})

	if _, exists, err := fixture.views.FindCurrent(ctx, tenant, intent.View.Customer(), intent.View.Parcel()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countCustomerViewIntents(t, fixture.pool, "view-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := customerViewIntentType(t, fixture.pool, "view-1"); got != "visibility-exception.customer-view.published" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestCustomerViewIntentRollbackDropsBoth(t *testing.T) {
	fixture := newCustomerViewHandoffFixture(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")
	intent := customerViewIntent(t, "tenant-a", "view-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.views.Save(txCtx, tenant, intent.View); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffCustomerView(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.views.FindCurrent(ctx, tenant, intent.View.Customer(), intent.View.Parcel()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countCustomerViewIntents(t, fixture.pool, "view-1"); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.views.Save(txCtx, tenant, intent.View); err != nil {
			return err
		}
		return fixture.handoff.HandOffCustomerView(txCtx, intent)
	})
	if count := countCustomerViewIntents(t, fixture.pool, "view-1"); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameCustomerViewIntentIsIdempotent(t *testing.T) {
	fixture := newCustomerViewHandoffFixture(t)
	ctx := t.Context()
	intent := customerViewIntent(t, "tenant-a", "view-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffCustomerView(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffCustomerView(txCtx, intent)
	})
	if count := countCustomerViewIntents(t, fixture.pool, "view-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestCustomerViewIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newCustomerViewHandoffFixture(t)
	if err := fixture.handoff.HandOffCustomerView(t.Context(), customerViewIntent(t, "tenant-a", "view-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countCustomerViewIntents(t, fixture.pool, "view-1"); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignCustomerViewIntentIsLoud(t *testing.T) {
	fixture := newCustomerViewHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffCustomerView(txCtx, ports.CustomerViewHandoffIntent{
			TenantID: tenantOf(t, "tenant-a"),
		})
	}); err == nil {
		t.Fatal("缺视图版本的意图必须响亮报错")
	}
	if count := countCustomerViewIntents(t, fixture.pool, "view-1"); count != 0 {
		t.Fatalf("异类意图入队了：%d 行", count)
	}
}

func countCustomerViewIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "visibility-exception.customer-view.published",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func customerViewIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
