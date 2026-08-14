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

// 本文件对真实 PostgreSQL 16 证客户通知意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺通知响亮报错。入队走 EnqueueOnce。

type notificationHandoffClock struct{ at time.Time }

func (clock notificationHandoffClock) Now() time.Time { return clock.at }

type notificationHandoffFixture struct {
	notifications *adapter.CustomerNotifications
	handoff       *adapter.OutboxNotificationHandoff
	transactor    bentoapp.Transactor
	pool          *pgxpool.Pool
}

func newNotificationHandoffFixture(t *testing.T) *notificationHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	notifications, err := adapter.NewCustomerNotifications(db)
	if err != nil {
		t.Fatalf("构造通知库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxNotificationHandoff(db, store, notificationHandoffClock{
		at: time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &notificationHandoffFixture{
		notifications: notifications,
		handoff:       handoff,
		transactor:    db.Transactor(),
		pool:          pool,
	}
}

func (fixture *notificationHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func notificationIntent(t *testing.T, tenant, id string) ports.NotificationHandoffIntent {
	t.Helper()
	return ports.NotificationHandoffIntent{
		TenantID:     etaGapValue(t, domain.NewTenantID, tenant),
		Notification: generatedNotification(t, id),
	}
}

func TestNotificationIntentCommitsAtomicallyWithTheRecord(t *testing.T) {
	fixture := newNotificationHandoffFixture(t)
	ctx := t.Context()
	tenant := etaGapValue(t, domain.NewTenantID, "tenant-a")
	intent := notificationIntent(t, "tenant-a", "notification-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.notifications.Save(txCtx, tenant, intent.Notification); err != nil {
			return err
		}
		return fixture.handoff.HandOffNotification(txCtx, intent)
	})

	if _, exists, err := fixture.notifications.FindByDisclosure(ctx, tenant, intent.Notification.Disclosure()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countNotificationIntents(t, fixture.pool, "notification-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := notificationIntentType(t, fixture.pool, "notification-1"); got != "visibility-exception.customer-notification.formed" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestNotificationIntentRollbackDropsBoth(t *testing.T) {
	fixture := newNotificationHandoffFixture(t)
	ctx := t.Context()
	tenant := etaGapValue(t, domain.NewTenantID, "tenant-a")
	intent := notificationIntent(t, "tenant-a", "notification-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.notifications.Save(txCtx, tenant, intent.Notification); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffNotification(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.notifications.FindByDisclosure(ctx, tenant, intent.Notification.Disclosure()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countNotificationIntents(t, fixture.pool, "notification-1"); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.notifications.Save(txCtx, tenant, intent.Notification); err != nil {
			return err
		}
		return fixture.handoff.HandOffNotification(txCtx, intent)
	})
	if count := countNotificationIntents(t, fixture.pool, "notification-1"); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameNotificationIntentIsIdempotent(t *testing.T) {
	fixture := newNotificationHandoffFixture(t)
	ctx := t.Context()
	intent := notificationIntent(t, "tenant-a", "notification-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffNotification(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffNotification(txCtx, intent)
	})
	if count := countNotificationIntents(t, fixture.pool, "notification-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestNotificationIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newNotificationHandoffFixture(t)
	if err := fixture.handoff.HandOffNotification(t.Context(), notificationIntent(t, "tenant-a", "notification-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countNotificationIntents(t, fixture.pool, "notification-1"); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignNotificationIntentIsLoud(t *testing.T) {
	fixture := newNotificationHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffNotification(txCtx, ports.NotificationHandoffIntent{
			TenantID: etaGapValue(t, domain.NewTenantID, "tenant-a"),
		})
	}); err == nil {
		t.Fatal("缺通知的意图必须响亮报错")
	}
	if count := countNotificationIntents(t, fixture.pool, "notification-1"); count != 0 {
		t.Fatalf("异类意图入队了：%d 行", count)
	}
}

func countNotificationIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "visibility-exception.customer-notification.formed",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func notificationIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
