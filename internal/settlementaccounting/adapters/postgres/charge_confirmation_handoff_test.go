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
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type saHandoffClock struct{ at time.Time }

func (clock saHandoffClock) Now() time.Time { return clock.at }

func newChargeConfirmationHandoffFixture(t *testing.T) (*adapter.OutboxChargeConfirmationHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxChargeConfirmationHandoff(db, store, saHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func chargeConfirmationIntent(t *testing.T, chargeID string) ports.ChargeConfirmationHandoffIntent {
	t.Helper()
	return ports.ChargeConfirmationHandoffIntent{
		TenantID: saTenant(t, "tenant-a"),
		Charge:   confirmedCharge(t, chargeID, "DELIVERY_FINALIZED/final-1"),
	}
}

// TestChargeConfirmationFollowsTheTransactionalTemplate 证费用确认意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。
func TestChargeConfirmationFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newChargeConfirmationHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffChargeConfirmation(txCtx, chargeConfirmationIntent(t, "charge-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/charge/charge-1"); count != 1 {
		t.Fatalf("charge-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffChargeConfirmation(txCtx, chargeConfirmationIntent(t, "charge-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/charge/charge-2"); count != 0 {
		t.Fatalf("回滚后 charge-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffChargeConfirmation(txCtx, chargeConfirmationIntent(t, "charge-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/charge/charge-1"); count != 1 {
		t.Fatalf("重发后 charge-1 行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffChargeConfirmation(ctx, chargeConfirmationIntent(t, "charge-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func countSAIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
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
