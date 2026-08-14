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
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

func newCapacityConsumptionHandoffFixture(t *testing.T) (*adapter.OutboxCapacityConsumptionHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxCapacityConsumptionHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func capacityConsumptionIntent(t *testing.T, pool, reservation string) ports.CapacityConsumptionIntent {
	t.Helper()
	return ports.CapacityConsumptionIntent{
		Pool:        establishedPoolRecord(t, "tenant-a", pool, 100),
		Reservation: deliveryValue(t, domain.NewCapacityReservationReference, reservation),
		Assignment:  deliveryValue(t, domain.NewLoadAssignmentReference, "assignment-1"),
		Quantity:    10,
	}
}

func capacityConsumptionEventID(pool, reservation string) string {
	return "tenant-a/" + pool + "/" + reservation + "/capacity-consumption"
}

// TestCapacityConsumptionFollowsTheTransactionalTemplate 证容量消耗意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 取容量池/预占键再加类型段。
func TestCapacityConsumptionFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newCapacityConsumptionHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffCapacityConsumption(txCtx, capacityConsumptionIntent(t, "pool-1", "res-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countTFIntents(t, pool, capacityConsumptionEventID("pool-1", "res-1")); count != 1 {
		t.Fatalf("pool-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffCapacityConsumption(txCtx, capacityConsumptionIntent(t, "pool-rollback", "res-rollback")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countTFIntents(t, pool, capacityConsumptionEventID("pool-rollback", "res-rollback")); count != 0 {
		t.Fatalf("回滚后 pool-rollback 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffCapacityConsumption(txCtx, capacityConsumptionIntent(t, "pool-1", "res-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countTFIntents(t, pool, capacityConsumptionEventID("pool-1", "res-1")); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffCapacityConsumption(ctx, capacityConsumptionIntent(t, "pool-ntx", "res-ntx")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestCapacityConsumptionRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newCapacityConsumptionHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffCapacityConsumption(txCtx, ports.CapacityConsumptionIntent{})
	})
	if err == nil {
		t.Fatal("缺容量池/预占键的消耗意图入了队")
	}
}
