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

type tfHandoffClock struct{ at time.Time }

func (clock tfHandoffClock) Now() time.Time { return clock.at }

func newPickupHandoffFixture(t *testing.T) (*adapter.OutboxOffsitePickupHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxOffsitePickupHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func pickupHandoffIntent(t *testing.T, source string) ports.OffsitePickupHandoffIntent {
	t.Helper()
	return ports.OffsitePickupHandoffIntent{Record: mixedPickupRecord(t, "tenant-a", source)}
}

// TestOffsitePickupFollowsTheTransactionalTemplate 证揽收意图复现样板四条：首发一行、
// 回滚无痕、重发同一份、无事务拒。信封 ID 由揽收尝试幂等键认领。
func TestOffsitePickupFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newPickupHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffOffsitePickup(txCtx, pickupHandoffIntent(t, "source-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/source-1"); count != 1 {
		t.Fatalf("source-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffOffsitePickup(txCtx, pickupHandoffIntent(t, "source-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/source-2"); count != 0 {
		t.Fatalf("回滚后 source-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffOffsitePickup(txCtx, pickupHandoffIntent(t, "source-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/source-1"); count != 1 {
		t.Fatalf("重发后 source-1 行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffOffsitePickup(ctx, pickupHandoffIntent(t, "source-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestOffsitePickupRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newPickupHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffOffsitePickup(txCtx, ports.OffsitePickupHandoffIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的揽收意图入了队")
	}
}

func countTFIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()

	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}
