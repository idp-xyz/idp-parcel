package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func newFinalOutcomeHandoffFixture(t *testing.T) (*adapter.OutboxFinalOutcomeHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxFinalOutcomeHandoff(db, store, handoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func finalOutcomeHandoffIntent(t *testing.T, parcel, version string) ports.FinalOutcomeHandoffIntent {
	t.Helper()
	return ports.FinalOutcomeHandoffIntent{
		Record: firstFinalRecord(t, "tenant-a", parcel, version, "digest-"+parcel),
	}
}

func finalOutcomeEventID(parcel, version string) string {
	return "tenant-a/final-outcome/" + parcel + "/EFFECTIVE_DELIVERY/" + version
}

// TestFinalOutcomeFollowsTheTransactionalTemplate 证终局判断意图复现样板四条：首发一行、
// 回滚无痕、重发同一份、无事务拒。信封 ID 由采用键认领并加类型段。
func TestFinalOutcomeFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newFinalOutcomeHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffFinalOutcome(txCtx, finalOutcomeHandoffIntent(t, "parcel-1", "outcome-v1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, finalOutcomeEventID("parcel-1", "outcome-v1")); count != 1 {
		t.Fatalf("parcel-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffFinalOutcome(txCtx, finalOutcomeHandoffIntent(t, "parcel-rollback", "outcome-v-rollback")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, finalOutcomeEventID("parcel-rollback", "outcome-v-rollback")); count != 0 {
		t.Fatalf("回滚后 parcel-rollback 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffFinalOutcome(txCtx, finalOutcomeHandoffIntent(t, "parcel-1", "outcome-v1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, finalOutcomeEventID("parcel-1", "outcome-v1")); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffFinalOutcome(ctx, finalOutcomeHandoffIntent(t, "parcel-ntx", "outcome-v-ntx")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestFinalOutcomeRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newFinalOutcomeHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffFinalOutcome(txCtx, ports.FinalOutcomeHandoffIntent{})
	})
	if err == nil {
		t.Fatal("缺采用键的终局意图入了队")
	}
}
