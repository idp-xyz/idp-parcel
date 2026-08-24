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

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「委托已提交」意图：与建单行同一提交、回滚一并消失、
// 重发同一份、无事务拒、缺身份响亮报错。信封 ID 由来源身份（含租户）再加类型段认领，
// 入队走 EnqueueOnce。

const shipmentRequestSubmittedEventType = "parcel-shipment.shipment-request.submitted"

type submittedHandoffClock struct{ at time.Time }

func (clock submittedHandoffClock) Now() time.Time { return clock.at }

type submittedHandoffFixture struct {
	requests   *adapter.ShipmentRequests
	handoff    *adapter.OutboxShipmentRequestSubmittedHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newSubmittedHandoffFixture(t *testing.T) *submittedHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	requests, err := adapter.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxShipmentRequestSubmittedHandoff(db, store, submittedHandoffClock{
		at: submittedAtFixture.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &submittedHandoffFixture{
		requests: requests, handoff: handoff, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *submittedHandoffFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func submittedIntent(t *testing.T, key, requestID string) ports.ShipmentRequestSubmittedHandoffIntent {
	t.Helper()
	return ports.ShipmentRequestSubmittedHandoffIntent{
		Identity: requestIdentity(t, key),
		Request:  submittedShipmentRequest(t, key, requestID),
	}
}

func submittedEventID(tenant, customer, source, key string) string {
	return tenant + "/" + customer + "/" + source + "/" + key + "/shipment-request-submitted"
}

func TestSubmittedIntentCommitsAtomicallyWithTheRequestRow(t *testing.T) {
	fixture := newSubmittedHandoffFixture(t)
	ctx := t.Context()
	intent := submittedIntent(t, "req-key-1", "request-1")
	eventID := submittedEventID("tenant-1", "customer-1", "portal", "req-key-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Insert(txCtx, intent.Identity, intent.Request); err != nil {
			return err
		}
		return fixture.handoff.HandOffShipmentRequestSubmitted(txCtx, intent)
	})

	if _, exists, err := fixture.requests.FindBySourceIdentity(ctx, intent.Identity); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countSubmittedIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
}

func TestSubmittedIntentRollbackDropsBoth(t *testing.T) {
	fixture := newSubmittedHandoffFixture(t)
	ctx := t.Context()
	intent := submittedIntent(t, "req-key-1", "request-1")
	eventID := submittedEventID("tenant-1", "customer-1", "portal", "req-key-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Insert(txCtx, intent.Identity, intent.Request); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffShipmentRequestSubmitted(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.requests.FindBySourceIdentity(ctx, intent.Identity); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countSubmittedIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	// 回滚后重投：建单与意图一起重来，两侧各恰好一份——「任一步失败全部回滚」的另一半
	// 是重来时不背上一次的账。
	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Insert(txCtx, intent.Identity, intent.Request); err != nil {
			return err
		}
		return fixture.handoff.HandOffShipmentRequestSubmitted(txCtx, intent)
	})
	if count := countSubmittedIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameSubmittedIntentIsIdempotent(t *testing.T) {
	fixture := newSubmittedHandoffFixture(t)
	ctx := t.Context()
	intent := submittedIntent(t, "req-key-1", "request-1")
	eventID := submittedEventID("tenant-1", "customer-1", "portal", "req-key-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffShipmentRequestSubmitted(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffShipmentRequestSubmitted(txCtx, intent)
	})
	if count := countSubmittedIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestSubmittedIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newSubmittedHandoffFixture(t)
	intent := submittedIntent(t, "req-key-1", "request-1")
	eventID := submittedEventID("tenant-1", "customer-1", "portal", "req-key-1")
	if err := fixture.handoff.HandOffShipmentRequestSubmitted(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countSubmittedIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignSubmittedIntentIsLoud(t *testing.T) {
	fixture := newSubmittedHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffShipmentRequestSubmitted(txCtx, ports.ShipmentRequestSubmittedHandoffIntent{})
	}); err == nil {
		t.Fatal("缺来源身份的意图必须响亮报错")
	}
}

func countSubmittedIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, shipmentRequestSubmittedEventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}
