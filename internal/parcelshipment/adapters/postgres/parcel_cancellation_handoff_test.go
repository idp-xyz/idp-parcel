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

// 本文件对真实 PostgreSQL 16 证包裹取消决定意图：与业务行同一提交、回滚一并消失、
// 重发同一份、无事务拒、缺取消键响亮报错。信封 ID 由取消键（含租户）认领。入队走
// EnqueueOnce。

const parcelCancellationEventType = "parcel-shipment.parcel-cancellation.recorded"

type cancellationHandoffClock struct{ at time.Time }

func (clock cancellationHandoffClock) Now() time.Time { return clock.at }

type cancellationHandoffFixture struct {
	cancellations *adapter.ParcelCancellations
	handoff       *adapter.OutboxParcelCancellationHandoff
	transactor    bentoapp.Transactor
	pool          *pgxpool.Pool
}

func newCancellationHandoffFixture(t *testing.T) *cancellationHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	cancellations, err := adapter.NewParcelCancellations(db)
	if err != nil {
		t.Fatalf("构造取消库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxParcelCancellationHandoff(db, store, cancellationHandoffClock{
		at: time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &cancellationHandoffFixture{
		cancellations: cancellations, handoff: handoff, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *cancellationHandoffFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func cancellationIntent(t *testing.T, tenant, requestKey, parcel string) ports.ParcelCancellationHandoffIntent {
	t.Helper()
	return ports.ParcelCancellationHandoffIntent{
		Record: cancelledRecord(t, tenant, requestKey, parcel, "digest-cancel-1"),
	}
}

func cancellationEventID(tenant, requestKey, parcel string) string {
	return tenant + "/" + requestKey + "/" + parcel
}

func TestParcelCancellationIntentCommitsAtomicallyWithTheDecision(t *testing.T) {
	fixture := newCancellationHandoffFixture(t)
	ctx := t.Context()
	intent := cancellationIntent(t, "tenant-a", "req-1", "parcel-1")
	eventID := cancellationEventID("tenant-a", "req-1", "parcel-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.cancellations.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffParcelCancellation(txCtx, intent)
	})

	if _, exists, err := fixture.cancellations.FindByKey(ctx, intent.Record.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countCancellationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := cancellationIntentType(t, fixture.pool, eventID); got != parcelCancellationEventType {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestParcelCancellationIntentRollbackDropsBoth(t *testing.T) {
	fixture := newCancellationHandoffFixture(t)
	ctx := t.Context()
	intent := cancellationIntent(t, "tenant-a", "req-1", "parcel-1")
	eventID := cancellationEventID("tenant-a", "req-1", "parcel-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.cancellations.Save(txCtx, intent.Record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffParcelCancellation(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.cancellations.FindByKey(ctx, intent.Record.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countCancellationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.cancellations.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffParcelCancellation(txCtx, intent)
	})
	if count := countCancellationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameParcelCancellationIntentIsIdempotent(t *testing.T) {
	fixture := newCancellationHandoffFixture(t)
	ctx := t.Context()
	intent := cancellationIntent(t, "tenant-a", "req-1", "parcel-1")
	eventID := cancellationEventID("tenant-a", "req-1", "parcel-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffParcelCancellation(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffParcelCancellation(txCtx, intent)
	})
	if count := countCancellationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

// TestTwoCancellationRequestsForOneParcelShareAPartition 钉住一件今天还看不出后果的事。
//
// 本口与另两口不同：它**今天没有更正入口**（同一请求身份返回原结果），所以按分区键那套判据
// 它属「无先后」，改不改分区键眼下都无害。改了、并且钉在这里，是因为**无害只是当下的事实**：
// 同一包裹的第二次取消请求一旦出现（另一个请求身份、另一个信封 ID），两条若各自成区就再也
// 说不清哪次在先，而取消的先后正是判定边界要用的。
//
// 所以这条断言防的不是回归，是**将来某天这口从「无先后」变成「有先后」时不会悄悄失序**。
// 经变异验证：把 PartitionKey 改回 eventID 时本用例变红。
func TestTwoCancellationRequestsForOneParcelShareAPartition(t *testing.T) {
	fixture := newCancellationHandoffFixture(t)
	ctx := t.Context()

	for _, requestKey := range []string{"req-1", "req-2"} {
		intent := cancellationIntent(t, "tenant-a", requestKey, "parcel-1")
		fixture.within(t, ctx, func(txCtx context.Context) error {
			return fixture.handoff.HandOffParcelCancellation(txCtx, intent)
		})
	}

	first := cancellationEventID("tenant-a", "req-1", "parcel-1")
	second := cancellationEventID("tenant-a", "req-2", "parcel-1")
	for _, eventID := range []string{first, second} {
		if count := countCancellationIntents(t, fixture.pool, eventID); count != 1 {
			t.Fatalf("%s 行数 = %d，want 1——两次请求是两份意图", eventID, count)
		}
	}

	firstPartition := partitionKeyOf(t, fixture.pool, first)
	if secondPartition := partitionKeyOf(t, fixture.pool, second); firstPartition != secondPartition {
		t.Fatalf("同一包裹的两次取消请求落在不同分区：%q 与 %q——先后就此说不清",
			firstPartition, secondPartition)
	}

	other := cancellationIntent(t, "tenant-a", "req-1", "parcel-2")
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffParcelCancellation(txCtx, other)
	})
	otherPartition := partitionKeyOf(t, fixture.pool, cancellationEventID("tenant-a", "req-1", "parcel-2"))
	if otherPartition == firstPartition {
		t.Fatalf("两个包裹共用分区 %q——一个的失败会拖住另一个", otherPartition)
	}
}

func TestParcelCancellationIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newCancellationHandoffFixture(t)
	intent := cancellationIntent(t, "tenant-a", "req-1", "parcel-1")
	eventID := cancellationEventID("tenant-a", "req-1", "parcel-1")
	if err := fixture.handoff.HandOffParcelCancellation(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countCancellationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignParcelCancellationIntentIsLoud(t *testing.T) {
	fixture := newCancellationHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffParcelCancellation(txCtx, ports.ParcelCancellationHandoffIntent{
			Record: ports.CancellationRecord{
				Key: ports.CancellationRequestKey{TenantID: psTenant(t, "tenant-a")},
			},
		})
	}); err == nil {
		t.Fatal("缺取消键的意图必须响亮报错")
	}
}

func countCancellationIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, parcelCancellationEventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func cancellationIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
