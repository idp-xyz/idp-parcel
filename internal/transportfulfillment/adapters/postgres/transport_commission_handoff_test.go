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

func newCommissionHandoffFixture(t *testing.T) (*adapter.OutboxTransportCommissionHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxTransportCommissionHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func commissionHandoffIntent(t *testing.T, id string) ports.TransportCommissionIntent {
	t.Helper()
	return ports.TransportCommissionIntent{Record: submittedCommission(t, "tenant-a", id)}
}

// TestTransportCommissionFollowsTheTransactionalTemplate 证委托意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 由委托幂等键认领。
func TestTransportCommissionFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newCommissionHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffTransportCommission(txCtx, commissionHandoffIntent(t, "commission-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/commission-1"); count != 1 {
		t.Fatalf("commission-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffTransportCommission(txCtx, commissionHandoffIntent(t, "commission-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/commission-2"); count != 0 {
		t.Fatalf("回滚后 commission-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffTransportCommission(txCtx, commissionHandoffIntent(t, "commission-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/commission-1"); count != 1 {
		t.Fatalf("重发后 commission-1 行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffTransportCommission(ctx, commissionHandoffIntent(t, "commission-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestTransportCommissionRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newCommissionHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffTransportCommission(txCtx, ports.TransportCommissionIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的委托意图入了队")
	}
}
