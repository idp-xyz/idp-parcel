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
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func newOperatingHandoffFixture(t *testing.T) (*adapter.OutboxOperatingHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxOperatingHandoff(db, store, saHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func operatingAllocationIntent(t *testing.T, id string) ports.OperatingIntent {
	t.Helper()
	return ports.OperatingIntent{Allocation: formedAllocationRecord(t, "tenant-a", id)}
}

// TestOperatingFollowsTheTransactionalTemplate 证分摊意图复现样板四条：首发一行、回滚
// 无痕、重发同一份、无事务拒。信封 ID 由分摊幂等键认领。
func TestOperatingFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newOperatingHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffOperating(txCtx, operatingAllocationIntent(t, "alloc-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/allocation/alloc-1"); count != 1 {
		t.Fatalf("alloc-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffOperating(txCtx, operatingAllocationIntent(t, "alloc-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/allocation/alloc-2"); count != 0 {
		t.Fatalf("回滚后 alloc-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffOperating(txCtx, operatingAllocationIntent(t, "alloc-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/allocation/alloc-1"); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffOperating(ctx, operatingAllocationIntent(t, "alloc-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestOperatingRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newOperatingHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffOperating(txCtx, ports.OperatingIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的经营意图入了队")
	}
}

func TestOperatingResultUsesItsOwnEnvelope(t *testing.T) {
	handoff, db, pool := newOperatingHandoffFixture(t)
	intent := ports.OperatingIntent{Result: derivedResultRecord(t, "tenant-a", domain.ConfirmedBasis)}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffOperating(txCtx, intent)
	}); err != nil {
		t.Fatalf("指标入队：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/operating-result/customer-1/period-2026-08/CONFIRMED"); count != 1 {
		t.Fatalf("经营结果行数 = %d, want 1", count)
	}
}
