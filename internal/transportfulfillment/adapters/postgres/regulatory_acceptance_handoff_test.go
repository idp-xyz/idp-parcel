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

func newRegulatoryAcceptanceHandoffFixture(t *testing.T) (*adapter.OutboxRegulatoryAcceptanceHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxRegulatoryAcceptanceHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func regulatoryAcceptanceHandoffIntent(t *testing.T, item string) ports.RegulatoryAcceptanceHandoffIntent {
	t.Helper()
	return ports.RegulatoryAcceptanceHandoffIntent{Record: acceptedDisposition(t, "tenant-a", item)}
}

// TestRegulatoryAcceptanceFollowsTheTransactionalTemplate 证承接回执意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 由承接幂等键认领。
func TestRegulatoryAcceptanceFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newRegulatoryAcceptanceHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffRegulatoryAcceptance(txCtx, regulatoryAcceptanceHandoffIntent(t, "item-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/item-1/regulatory-acceptance"); count != 1 {
		t.Fatalf("item-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffRegulatoryAcceptance(txCtx, regulatoryAcceptanceHandoffIntent(t, "item-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/item-2/regulatory-acceptance"); count != 0 {
		t.Fatalf("回滚后 item-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffRegulatoryAcceptance(txCtx, regulatoryAcceptanceHandoffIntent(t, "item-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/item-1/regulatory-acceptance"); count != 1 {
		t.Fatalf("重发后 item-1 行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffRegulatoryAcceptance(ctx, regulatoryAcceptanceHandoffIntent(t, "item-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestRegulatoryAcceptanceRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newRegulatoryAcceptanceHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffRegulatoryAcceptance(txCtx, ports.RegulatoryAcceptanceHandoffIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的承接回执意图入了队")
	}
}
