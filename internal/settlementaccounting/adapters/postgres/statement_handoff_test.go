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

func newStatementHandoffFixture(t *testing.T) (*adapter.OutboxStatementHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxStatementHandoff(db, store, saHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func publishedStatementIntent(t *testing.T, number string) ports.StatementIntent {
	t.Helper()
	return ports.StatementIntent{Statement: statementRecord(t, "tenant-a", number, "digest-"+number)}
}

// TestStatementHandoffFollowsTheTransactionalTemplate 证对账单意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。
func TestStatementHandoffFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newStatementHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffStatement(txCtx, publishedStatementIntent(t, "st-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/statement/st-1"); count != 1 {
		t.Fatalf("st-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffStatement(txCtx, publishedStatementIntent(t, "st-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/statement/st-2"); count != 0 {
		t.Fatalf("回滚后 st-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffStatement(txCtx, publishedStatementIntent(t, "st-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countSAIntents(t, pool, "tenant-a/statement/st-1"); count != 1 {
		t.Fatalf("重发后 st-1 行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffStatement(ctx, publishedStatementIntent(t, "st-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestStatementVoidAndInclusionUseDistinctEnvelopes(t *testing.T) {
	handoff, db, pool := newStatementHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	published := publishedStatementIntent(t, "st-void")
	voided, err := published.Statement.Statement.Void(
		saValue(t, domain.NewStatementVoidBasisReference, "void-1"),
		statementVoidAt,
	)
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	voidIntent := published
	voidIntent.Statement.Statement = voided

	inclusion := ports.StatementIntent{
		Inclusion: formedInclusionRecord(t, "tenant-a", "inclusion-1", domain.IncludedLateCharge),
	}

	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffStatement(txCtx, published); err != nil {
			return err
		}
		if err := handoff.HandOffStatement(txCtx, voidIntent); err != nil {
			return err
		}
		return handoff.HandOffStatement(txCtx, inclusion)
	})

	if count := countSAIntents(t, pool, "tenant-a/statement/st-void"); count != 1 {
		t.Fatalf("发布信封 = %d", count)
	}
	if count := countSAIntents(t, pool, "tenant-a/statement/st-void/voided"); count != 1 {
		t.Fatalf("作废信封 = %d——作废被发布吞了", count)
	}
	if count := countSAIntents(t, pool, "tenant-a/inclusion/inclusion-1"); count != 1 {
		t.Fatalf("纳入信封 = %d", count)
	}

	// 两个字段的分工在这里一并钉住：**都入队**（ID 带 /voided 所以作废不被发布吞）
	// 且**落在同一分区**（分区键只到对账单，所以作废排在发布之后）。少了后半条，
	// 把 PartitionKey 改回 eventID 不会让任何东西变红——那正是这个缺陷此前的处境。
	publishedPartition := partitionKeyOf(t, pool, "tenant-a/statement/st-void")
	voidedPartition := partitionKeyOf(t, pool, "tenant-a/statement/st-void/voided")
	if publishedPartition != voidedPartition {
		t.Fatalf("发布与作废落进两个分区（%q vs %q）：作废可能先于它作废的那份送达",
			publishedPartition, voidedPartition)
	}
	if publishedPartition != "tenant-a/statement/st-void" {
		t.Fatalf("分区键 = %q，应当只到对账单这一层", publishedPartition)
	}
	if inclusionPartition := partitionKeyOf(
		t, pool, "tenant-a/inclusion/inclusion-1",
	); inclusionPartition == publishedPartition {
		t.Fatal("纳入与对账单共用了分区：两者不是同一个需要保序的对象")
	}
}

func TestEmptyStatementIntentIsRefused(t *testing.T) {
	handoff, db, _ := newStatementHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffStatement(txCtx, ports.StatementIntent{})
	})
	if err == nil {
		t.Fatal("两半都缺的意图入了队")
	}
}
