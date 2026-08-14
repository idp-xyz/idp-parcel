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

func newSettlementApplicationHandoffFixture(t *testing.T) (*adapter.OutboxSettlementApplicationHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxSettlementApplicationHandoff(db, store, saHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func settlementApplicationHandoffIntent(t *testing.T, id string) ports.SettlementApplicationIntent {
	t.Helper()
	fact := adoptedFactRecord(t, "tenant-a", "bank-fact-"+id, domain.FundsReceiptConfirmed, 8000)
	mapping := mappingRecord(t, "tenant-a", "mapping-"+id, fact.Fact, domain.TargetPayable, "payable-1")
	return ports.SettlementApplicationIntent{
		Record: applicationRecord(t, "tenant-a", id, fact.Fact, mapping.Mapping),
	}
}

// TestSettlementApplicationFollowsTheTransactionalTemplate 证核销意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 由核销幂等键认领。
func TestSettlementApplicationFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newSettlementApplicationHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffSettlementApplication(txCtx, settlementApplicationHandoffIntent(t, "application-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/application/application-1"); count != 1 {
		t.Fatalf("application-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffSettlementApplication(txCtx, settlementApplicationHandoffIntent(t, "application-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/application/application-2"); count != 0 {
		t.Fatalf("回滚后 application-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffSettlementApplication(txCtx, settlementApplicationHandoffIntent(t, "application-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/application/application-1"); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffSettlementApplication(ctx, settlementApplicationHandoffIntent(t, "application-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestSettlementApplicationRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newSettlementApplicationHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffSettlementApplication(txCtx, ports.SettlementApplicationIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的核销意图入了队")
	}
}
