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

func newClaimSettlementHandoffFixture(t *testing.T) (*adapter.OutboxClaimSettlementHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxClaimSettlementHandoff(db, store, saHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func claimAmountHandoffIntent(t *testing.T, id string) ports.ClaimSettlementIntent {
	t.Helper()
	return ports.ClaimSettlementIntent{
		ClaimAmount: formedClaimAmountRecord(t, "tenant-a", id, domain.CustomerCompensationPayable),
	}
}

// TestClaimSettlementFollowsTheTransactionalTemplate 证索赔金额意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 由索赔金额幂等键认领。
func TestClaimSettlementFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newClaimSettlementHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffClaimSettlement(txCtx, claimAmountHandoffIntent(t, "amount-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/claim-amount/amount-1"); count != 1 {
		t.Fatalf("amount-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffClaimSettlement(txCtx, claimAmountHandoffIntent(t, "amount-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/claim-amount/amount-2"); count != 0 {
		t.Fatalf("回滚后 amount-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffClaimSettlement(txCtx, claimAmountHandoffIntent(t, "amount-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/claim-amount/amount-1"); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffClaimSettlement(ctx, claimAmountHandoffIntent(t, "amount-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestClaimSettlementRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newClaimSettlementHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffClaimSettlement(txCtx, ports.ClaimSettlementIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的索赔结算意图入了队")
	}
}

func TestClaimSettlementReceivableUsesItsOwnEnvelope(t *testing.T) {
	handoff, db, pool := newClaimSettlementHandoffFixture(t)
	intent := ports.ClaimSettlementIntent{Receivable: formedReceivableRecord(t, "tenant-a", "receivable-1")}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffClaimSettlement(txCtx, intent)
	}); err != nil {
		t.Fatalf("应追偿入队：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/receivable/receivable-1"); count != 1 {
		t.Fatalf("receivable-1 行数 = %d, want 1", count)
	}
}
