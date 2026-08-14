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

// 本文件对真实 PostgreSQL 16 证分诊结论意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺对象类型响亮报错。信封 ID 由租户加对象加类型认领。入队走 EnqueueOnce。

type triageHandoffClock struct{ at time.Time }

func (clock triageHandoffClock) Now() time.Time { return clock.at }

type triageHandoffFixture struct {
	episodes   *adapter.SignalEpisodes
	handoff    *adapter.OutboxTriageHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newTriageHandoffFixture(t *testing.T) *triageHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	episodes, err := adapter.NewSignalEpisodes(db)
	if err != nil {
		t.Fatalf("构造发作期库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxTriageHandoff(db, store, triageHandoffClock{
		at: time.Date(2026, 8, 14, 19, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &triageHandoffFixture{episodes: episodes, handoff: handoff, transactor: db.Transactor(), pool: pool}
}

func (fixture *triageHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func triageIntent(t *testing.T, tenant, parcel, kind string) (ports.TriageHandoffIntent, ports.RaisedSignalRecord) {
	t.Helper()
	episode := openedEpisode(t, "ep-1", parcel, kind, factBaseAt)
	record := raisedRecord(t, episode, tenant, parcel, kind, domain.AutoEstablishCase)
	return ports.TriageHandoffIntent{
		TenantID:   record.Tenant,
		Parcel:     record.Parcel,
		Kind:       record.Kind,
		Conclusion: record.Conclusion,
	}, record
}

func triageHandoffEventID(tenant, parcel, kind string) string {
	return tenant + "/" + parcel + "/" + kind
}

func TestTriageIntentCommitsAtomicallyWithTheEpisode(t *testing.T) {
	fixture := newTriageHandoffFixture(t)
	ctx := t.Context()
	intent, record := triageIntent(t, "tenant-a", "parcel-1", "STALLED")
	eventID := triageHandoffEventID("tenant-a", "parcel-1", "STALLED")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.episodes.SaveRaised(txCtx, record); err != nil {
			return err
		}
		return fixture.handoff.HandOffTriage(txCtx, intent)
	})

	if _, exists, err := fixture.episodes.FindLatest(ctx, record.Tenant, record.Parcel, record.Kind); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countTriageIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := triageIntentType(t, fixture.pool, eventID); got != "visibility-exception.signal-triage.concluded" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestTriageIntentRollbackDropsBoth(t *testing.T) {
	fixture := newTriageHandoffFixture(t)
	ctx := t.Context()
	intent, record := triageIntent(t, "tenant-a", "parcel-1", "STALLED")
	eventID := triageHandoffEventID("tenant-a", "parcel-1", "STALLED")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.episodes.SaveRaised(txCtx, record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffTriage(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.episodes.FindLatest(ctx, record.Tenant, record.Parcel, record.Kind); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countTriageIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.episodes.SaveRaised(txCtx, record); err != nil {
			return err
		}
		return fixture.handoff.HandOffTriage(txCtx, intent)
	})
	if count := countTriageIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameTriageIntentIsIdempotent(t *testing.T) {
	fixture := newTriageHandoffFixture(t)
	ctx := t.Context()
	intent, _ := triageIntent(t, "tenant-a", "parcel-1", "STALLED")
	eventID := triageHandoffEventID("tenant-a", "parcel-1", "STALLED")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffTriage(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffTriage(txCtx, intent)
	})
	if count := countTriageIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestTriageIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newTriageHandoffFixture(t)
	intent, _ := triageIntent(t, "tenant-a", "parcel-1", "STALLED")
	eventID := triageHandoffEventID("tenant-a", "parcel-1", "STALLED")
	if err := fixture.handoff.HandOffTriage(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countTriageIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignTriageIntentIsLoud(t *testing.T) {
	fixture := newTriageHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffTriage(txCtx, ports.TriageHandoffIntent{
			TenantID: factValue(t, domain.NewTenantID, "tenant-a"),
		})
	}); err == nil {
		t.Fatal("缺对象类型的意图必须响亮报错")
	}
	if count := countTriageIntents(t, fixture.pool, "tenant-a/parcel-1/STALLED"); count != 0 {
		t.Fatalf("异类意图入队了：%d 行", count)
	}
}

// TestCrossTenantTriageIntentsDoNotShareAnEnvelope 证信封 ID 含租户维：两租户同包裹
// 同类型各入一队。EnqueueOnce 按 (source, event_id) 去重，缺租户维会合成一份。
func TestCrossTenantTriageIntentsDoNotShareAnEnvelope(t *testing.T) {
	fixture := newTriageHandoffFixture(t)
	ctx := t.Context()
	first, _ := triageIntent(t, "tenant-a", "parcel-1", "STALLED")
	second, _ := triageIntent(t, "tenant-b", "parcel-1", "STALLED")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffTriage(txCtx, first)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffTriage(txCtx, second)
	})

	if count := countTriageIntents(t, fixture.pool, triageHandoffEventID("tenant-a", "parcel-1", "STALLED")); count != 1 {
		t.Fatalf("租户 A outbox 行数 = %d，want 1", count)
	}
	if count := countTriageIntents(t, fixture.pool, triageHandoffEventID("tenant-b", "parcel-1", "STALLED")); count != 1 {
		t.Fatalf("租户 B outbox 行数 = %d，want 1", count)
	}
}

func countTriageIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "visibility-exception.signal-triage.concluded",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func triageIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
