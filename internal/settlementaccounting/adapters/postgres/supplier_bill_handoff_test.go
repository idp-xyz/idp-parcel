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

func newSupplierBillHandoffFixture(t *testing.T) (*adapter.OutboxSupplierBillHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxSupplierBillHandoff(db, store, saHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func supplierBillHandoffIntent(t *testing.T, claimID string) ports.SupplierBillHandoffIntent {
	t.Helper()
	return ports.SupplierBillHandoffIntent{Record: billRecord(t, "tenant-a", claimID, "v1", "digest-"+claimID)}
}

// TestBillVersionsOfOneClaimShareAPartition 钉住两个字段的分工：同一主张的两个版本
// **都入队**（版本在信封 ID 里，后一版不被前一版吞）**且落在同一分区**（分区键只到
// 主张，所以 v2 排在 v1 之后）。不同主张不共享分区。
//
// 没有后半条，把 PartitionKey 改回 eventID 不会让任何东西变红——而那正是这一处此前
// 的处境：ID 那一半本来就对，错的只有分区键，因此「不丢」的用例全绿，谁也看不出
// 同一主张的两版其实排在两条互不相干的队里。
func TestBillVersionsOfOneClaimShareAPartition(t *testing.T) {
	handoff, db, pool := newSupplierBillHandoffFixture(t)
	ctx := t.Context()

	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffSupplierBill(txCtx, ports.SupplierBillHandoffIntent{
			Record: billRecord(t, "tenant-a", "claim-v", "v1", "digest-v1"),
		}); err != nil {
			return err
		}
		if err := handoff.HandOffSupplierBill(txCtx, ports.SupplierBillHandoffIntent{
			Record: billRecord(t, "tenant-a", "claim-v", "v2", "digest-v2"),
		}); err != nil {
			return err
		}
		return handoff.HandOffSupplierBill(txCtx, ports.SupplierBillHandoffIntent{
			Record: billRecord(t, "tenant-a", "claim-other", "v1", "digest-other"),
		})
	})

	if count := countSAIntents(t, pool, "tenant-a/bill/claim-v/v1"); count != 1 {
		t.Fatalf("v1 信封 = %d", count)
	}
	if count := countSAIntents(t, pool, "tenant-a/bill/claim-v/v2"); count != 1 {
		t.Fatalf("v2 信封 = %d——后一版被前一版吞了", count)
	}

	first := partitionKeyOf(t, pool, "tenant-a/bill/claim-v/v1")
	second := partitionKeyOf(t, pool, "tenant-a/bill/claim-v/v2")
	if first != second {
		t.Fatalf("同一主张的两版落进两个分区（%q vs %q）：v1 可能在 v2 之后被审", first, second)
	}
	if first != "tenant-a/bill/claim-v" {
		t.Fatalf("分区键 = %q，应当只到主张这一层", first)
	}
	if other := partitionKeyOf(t, pool, "tenant-a/bill/claim-other/v1"); other == first {
		t.Fatal("不同主张共用了分区：它们之间没有先后可言")
	}
}

// TestSupplierBillFollowsTheTransactionalTemplate 证供应商账单意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 由账单接收幂等键认领。
func TestSupplierBillFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newSupplierBillHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffSupplierBill(txCtx, supplierBillHandoffIntent(t, "claim-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/bill/claim-1/v1"); count != 1 {
		t.Fatalf("claim-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffSupplierBill(txCtx, supplierBillHandoffIntent(t, "claim-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/bill/claim-2/v1"); count != 0 {
		t.Fatalf("回滚后 claim-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffSupplierBill(txCtx, supplierBillHandoffIntent(t, "claim-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/bill/claim-1/v1"); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffSupplierBill(ctx, supplierBillHandoffIntent(t, "claim-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestSupplierBillRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newSupplierBillHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffSupplierBill(txCtx, ports.SupplierBillHandoffIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的账单意图入了队")
	}
}
