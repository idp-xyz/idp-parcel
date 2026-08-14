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

// 本文件对真实 PostgreSQL 16 证案件关闭意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺关闭响亮报错。信封 ID 由租户加案件引用认领。入队走 EnqueueOnce。

type closureHandoffClock struct{ at time.Time }

func (clock closureHandoffClock) Now() time.Time { return clock.at }

type closureHandoffFixture struct {
	closures   *adapter.CaseClosures
	handoff    *adapter.OutboxCaseClosureHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newClosureHandoffFixture(t *testing.T) *closureHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	closures, err := adapter.NewCaseClosures(db)
	if err != nil {
		t.Fatalf("构造关闭库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxCaseClosureHandoff(db, store, closureHandoffClock{
		at: time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &closureHandoffFixture{
		closures:   closures,
		handoff:    handoff,
		transactor: db.Transactor(),
		pool:       pool,
	}
}

func (fixture *closureHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func closureIntent(t *testing.T, tenant string) ports.CaseClosureHandoffIntent {
	t.Helper()
	return ports.CaseClosureHandoffIntent{
		TenantID: fmcValue(t, domain.NewTenantID, tenant),
		Closure:  closedCase(t),
	}
}

func closureEventID(tenant, caseRef string) string {
	return tenant + "/" + caseRef
}

func TestCaseClosureIntentCommitsAtomicallyWithTheRecord(t *testing.T) {
	fixture := newClosureHandoffFixture(t)
	ctx := t.Context()
	tenant := fmcValue(t, domain.NewTenantID, "tenant-a")
	intent := closureIntent(t, "tenant-a")
	eventID := closureEventID("tenant-a", intent.Closure.CaseRef())

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.closures.Save(txCtx, tenant, intent.Closure); err != nil {
			return err
		}
		return fixture.handoff.HandOffClosure(txCtx, intent)
	})

	if _, exists, err := fixture.closures.FindByCase(ctx, tenant, intent.Closure.CaseRef()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countClosureIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := closureIntentType(t, fixture.pool, eventID); got != "customs-compliance.case-closure.recorded" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestCaseClosureIntentRollbackDropsBoth(t *testing.T) {
	fixture := newClosureHandoffFixture(t)
	ctx := t.Context()
	tenant := fmcValue(t, domain.NewTenantID, "tenant-a")
	intent := closureIntent(t, "tenant-a")
	eventID := closureEventID("tenant-a", intent.Closure.CaseRef())
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.closures.Save(txCtx, tenant, intent.Closure); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffClosure(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.closures.FindByCase(ctx, tenant, intent.Closure.CaseRef()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countClosureIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.closures.Save(txCtx, tenant, intent.Closure); err != nil {
			return err
		}
		return fixture.handoff.HandOffClosure(txCtx, intent)
	})
	if count := countClosureIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameCaseClosureIntentIsIdempotent(t *testing.T) {
	fixture := newClosureHandoffFixture(t)
	ctx := t.Context()
	intent := closureIntent(t, "tenant-a")
	eventID := closureEventID("tenant-a", intent.Closure.CaseRef())

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffClosure(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffClosure(txCtx, intent)
	})
	if count := countClosureIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestCaseClosureIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newClosureHandoffFixture(t)
	intent := closureIntent(t, "tenant-a")
	eventID := closureEventID("tenant-a", intent.Closure.CaseRef())
	if err := fixture.handoff.HandOffClosure(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countClosureIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignCaseClosureIntentIsLoud(t *testing.T) {
	fixture := newClosureHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffClosure(txCtx, ports.CaseClosureHandoffIntent{
			TenantID: fmcValue(t, domain.NewTenantID, "tenant-a"),
		})
	}); err == nil {
		t.Fatal("缺关闭的意图必须响亮报错")
	}
}

func countClosureIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.case-closure.recorded",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func closureIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
