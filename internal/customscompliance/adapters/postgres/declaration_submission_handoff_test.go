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

// 本文件对真实 PostgreSQL 16 证申报提交意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺幂等键响亮报错。信封 ID 由幂等键认领。入队走 EnqueueOnce。

type declarationHandoffClock struct{ at time.Time }

func (clock declarationHandoffClock) Now() time.Time { return clock.at }

type declarationHandoffFixture struct {
	submissions *adapter.DeclarationSubmissions
	handoff     *adapter.OutboxDeclarationSubmissionHandoff
	transactor  bentoapp.Transactor
	pool        *pgxpool.Pool
}

func newDeclarationHandoffFixture(t *testing.T) *declarationHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submissions, err := adapter.NewDeclarationSubmissions(db)
	if err != nil {
		t.Fatalf("构造申报链库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxDeclarationSubmissionHandoff(db, store, declarationHandoffClock{
		at: time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &declarationHandoffFixture{
		submissions: submissions,
		handoff:     handoff,
		transactor:  db.Transactor(),
		pool:        pool,
	}
}

func (fixture *declarationHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func declarationIntent(t *testing.T, tenant, unit, procedure, version string) ports.DeclarationSubmissionHandoffIntent {
	t.Helper()
	return ports.DeclarationSubmissionHandoffIntent{
		Record: submissionRecord(t, tenant, unit, procedure, version),
	}
}

func declarationEventID(tenant, unit, procedure string) string {
	return tenant + "/" + unit + "/" + procedure
}

func TestDeclarationSubmissionIntentCommitsAtomicallyWithTheRecord(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	ctx := t.Context()
	intent := declarationIntent(t, "tenant-a", "unit-1", "export-procedure/v1", "version-1")
	eventID := declarationEventID("tenant-a", "unit-1", "export-procedure/v1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.submissions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, intent)
	})

	if _, exists, err := fixture.submissions.FindByKey(ctx, intent.Record.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := declarationIntentType(t, fixture.pool, eventID); got != "customs-compliance.declaration-submission.formed" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestDeclarationSubmissionIntentRollbackDropsBoth(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	ctx := t.Context()
	intent := declarationIntent(t, "tenant-a", "unit-1", "export-procedure/v1", "version-1")
	eventID := declarationEventID("tenant-a", "unit-1", "export-procedure/v1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.submissions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffDeclarationSubmission(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.submissions.FindByKey(ctx, intent.Record.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.submissions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, intent)
	})
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameDeclarationSubmissionIntentIsIdempotent(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	ctx := t.Context()
	intent := declarationIntent(t, "tenant-a", "unit-1", "export-procedure/v1", "version-1")
	eventID := declarationEventID("tenant-a", "unit-1", "export-procedure/v1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, intent)
	})
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestDeclarationSubmissionIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	intent := declarationIntent(t, "tenant-a", "unit-1", "export-procedure/v1", "version-1")
	eventID := declarationEventID("tenant-a", "unit-1", "export-procedure/v1")
	if err := fixture.handoff.HandOffDeclarationSubmission(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countDeclarationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignDeclarationSubmissionIntentIsLoud(t *testing.T) {
	fixture := newDeclarationHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffDeclarationSubmission(txCtx, ports.DeclarationSubmissionHandoffIntent{})
	}); err == nil {
		t.Fatal("缺幂等键的意图必须响亮报错")
	}
}

func countDeclarationIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.declaration-submission.formed",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func declarationIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
