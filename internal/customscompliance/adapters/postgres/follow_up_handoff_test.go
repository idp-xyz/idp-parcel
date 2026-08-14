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

// 本文件对真实 PostgreSQL 16 证后续动作目标意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺目标键响亮报错。信封 ID 由后续动作目标键认领。入队走 EnqueueOnce。

type followUpHandoffClock struct{ at time.Time }

func (clock followUpHandoffClock) Now() time.Time { return clock.at }

type followUpHandoffFixture struct {
	followUps  *adapter.FollowUps
	handoff    *adapter.OutboxFollowUpHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newFollowUpHandoffFixture(t *testing.T) *followUpHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	followUps, err := adapter.NewFollowUps(db)
	if err != nil {
		t.Fatalf("构造后续动作库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxFollowUpHandoff(db, store, followUpHandoffClock{
		at: time.Date(2026, 8, 14, 17, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &followUpHandoffFixture{
		followUps:  followUps,
		handoff:    handoff,
		transactor: db.Transactor(),
		pool:       pool,
	}
}

func (fixture *followUpHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func followUpIntent(t *testing.T, tenant string) ports.FollowUpHandoffIntent {
	t.Helper()
	return ports.FollowUpHandoffIntent{
		Key:    followUpKey(t, tenant),
		Target: formedTarget(t),
	}
}

func followUpHandoffEventID(key ports.FollowUpTargetKey) string {
	return key.TenantID.String() + "/" + key.Trigger.String() + "/" +
		key.Version.String() + "/" + key.Kind.String()
}

func TestFollowUpIntentCommitsAtomicallyWithTheTarget(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	ctx := t.Context()
	intent := followUpIntent(t, "tenant-a")
	eventID := followUpHandoffEventID(intent.Key)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.followUps.SaveTarget(txCtx, intent.Key, intent.Target); err != nil {
			return err
		}
		return fixture.handoff.HandOffFollowUp(txCtx, intent)
	})

	if _, exists, err := fixture.followUps.FindTarget(ctx, intent.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countFollowUpIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := followUpIntentType(t, fixture.pool, eventID); got != "customs-compliance.follow-up.recorded" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestFollowUpIntentRollbackDropsBoth(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	ctx := t.Context()
	intent := followUpIntent(t, "tenant-a")
	eventID := followUpHandoffEventID(intent.Key)
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.followUps.SaveTarget(txCtx, intent.Key, intent.Target); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffFollowUp(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.followUps.FindTarget(ctx, intent.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countFollowUpIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.followUps.SaveTarget(txCtx, intent.Key, intent.Target); err != nil {
			return err
		}
		return fixture.handoff.HandOffFollowUp(txCtx, intent)
	})
	if count := countFollowUpIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameFollowUpIntentIsIdempotent(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	ctx := t.Context()
	intent := followUpIntent(t, "tenant-a")
	eventID := followUpHandoffEventID(intent.Key)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffFollowUp(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffFollowUp(txCtx, intent)
	})
	if count := countFollowUpIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestFollowUpIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	intent := followUpIntent(t, "tenant-a")
	eventID := followUpHandoffEventID(intent.Key)
	if err := fixture.handoff.HandOffFollowUp(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countFollowUpIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignFollowUpIntentIsLoud(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffFollowUp(txCtx, ports.FollowUpHandoffIntent{
			Key: ports.FollowUpTargetKey{
				TenantID: fmcValue(t, domain.NewTenantID, "tenant-a"),
			},
		})
	}); err == nil {
		t.Fatal("缺目标键的意图必须响亮报错")
	}
}

func countFollowUpIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.follow-up.recorded",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func followUpIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
