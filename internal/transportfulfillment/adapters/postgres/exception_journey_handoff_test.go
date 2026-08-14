package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

func newExceptionJourneyHandoffFixture(t *testing.T) (*adapter.OutboxExceptionJourneyHandoff, *bentopg.DB, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxExceptionJourneyHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func exceptionJourneyEventID(original string) string {
	return "tenant-a/" + original + "/RETURN/DISPOSITION/decision-1/exception-journey"
}

// TestExceptionJourneyFollowsTheTransactionalTemplate 证异常旅程意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 取替代旅程键再加类型段。
func TestExceptionJourneyFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newExceptionJourneyHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffExceptionJourney(txCtx, alternateJourneyHandoffIntent(t, "journey-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countTFIntents(t, pool, exceptionJourneyEventID("journey-1")); count != 1 {
		t.Fatalf("journey-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffExceptionJourney(txCtx, alternateJourneyHandoffIntent(t, "journey-rollback")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countTFIntents(t, pool, exceptionJourneyEventID("journey-rollback")); count != 0 {
		t.Fatalf("回滚后 journey-rollback 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffExceptionJourney(txCtx, alternateJourneyHandoffIntent(t, "journey-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countTFIntents(t, pool, exceptionJourneyEventID("journey-1")); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffExceptionJourney(ctx, alternateJourneyHandoffIntent(t, "journey-ntx")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestExceptionJourneyRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newExceptionJourneyHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffExceptionJourney(txCtx, ports.AlternateJourneyIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的异常旅程意图入了队")
	}
}

// TestExceptionJourneyDoesNotCollideWithDispositionExecution 证同键监管来路两口能各入
// 一队——类型段把处置执行与异常旅程错开。
func TestExceptionJourneyDoesNotCollideWithDispositionExecution(t *testing.T) {
	exception, db, pool := newExceptionJourneyHandoffFixture(t)
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	disposition, err := adapter.NewOutboxDispositionExecutionHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造处置执行适配器：%v", err)
	}

	intent := alternateJourneyHandoffIntent(t, "journey-1")
	ctx := t.Context()
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := disposition.HandOffDispositionExecution(txCtx, intent); err != nil {
			return err
		}
		return exception.HandOffExceptionJourney(txCtx, intent)
	}); err != nil {
		t.Fatalf("同事务两口：%v", err)
	}
	if count := countTFIntents(t, pool, dispositionExecutionEventID("journey-1")); count != 1 {
		t.Fatalf("处置执行行数 = %d, want 1", count)
	}
	if count := countTFIntents(t, pool, exceptionJourneyEventID("journey-1")); count != 1 {
		t.Fatalf("异常旅程行数 = %d, want 1——应是第二份而不是被处置执行吞掉", count)
	}
}
