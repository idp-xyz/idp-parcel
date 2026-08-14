package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func newFinalOutcomeHandoffFixture(t *testing.T) (*adapter.OutboxFinalOutcomeHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxFinalOutcomeHandoff(db, store, handoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func finalOutcomeHandoffIntent(t *testing.T, parcel, version string) ports.FinalOutcomeHandoffIntent {
	t.Helper()
	return ports.FinalOutcomeHandoffIntent{
		Record: firstFinalRecord(t, "tenant-a", parcel, version, "digest-"+parcel),
	}
}

func finalOutcomeEventID(parcel, version string) string {
	return "tenant-a/final-outcome/" + parcel + "/EFFECTIVE_DELIVERY/" + version
}

// TestFinalOutcomeFollowsTheTransactionalTemplate 证终局判断意图复现样板四条：首发一行、
// 回滚无痕、重发同一份、无事务拒。信封 ID 由采用键认领并加类型段。
func TestFinalOutcomeFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newFinalOutcomeHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffFinalOutcome(txCtx, finalOutcomeHandoffIntent(t, "parcel-1", "outcome-v1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, finalOutcomeEventID("parcel-1", "outcome-v1")); count != 1 {
		t.Fatalf("parcel-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffFinalOutcome(txCtx, finalOutcomeHandoffIntent(t, "parcel-rollback", "outcome-v-rollback")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, finalOutcomeEventID("parcel-rollback", "outcome-v-rollback")); count != 0 {
		t.Fatalf("回滚后 parcel-rollback 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffFinalOutcome(txCtx, finalOutcomeHandoffIntent(t, "parcel-1", "outcome-v1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, finalOutcomeEventID("parcel-1", "outcome-v1")); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffFinalOutcome(ctx, finalOutcomeHandoffIntent(t, "parcel-ntx", "outcome-v-ntx")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// partitionKeyOf 读回一份意图落在哪条分区队列上。
//
// 它与 countOutboxEventsIn 分开问：一个问「发出去了几份」，一个问「它们排在哪条队里」——
// 本仓这一类缺陷恰恰是两者只对了一样（ID 带区分维所以不丢，分区键却跟着 ID 走所以乱序）。
func partitionKeyOf(t *testing.T, pool *pgxpool.Pool, eventID string) string {
	t.Helper()

	var partitionKey string
	err := pool.QueryRow(t.Context(),
		`SELECT partition_key FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&partitionKey)
	if err != nil {
		t.Fatalf("读回分区键：%v", err)
	}
	return partitionKey
}

// TestARederivedFinalOutcomeQueuesBehindTheOneItSupersedes 钉住两个字段的分工。
//
// 终局是重派生翻旧插新，所以同一包裹会先后出现两个版本。两件都要成立：**都入队**（ID 含
// 版本，重派生不被 EnqueueOnce 当成重放吞掉）**且同分区**（分区键只到包裹，后派生的排在
// 先派生的后面）。少了后一半，重派生先送达时下游最后应用的是已被取代的那一份。
//
// 门禁只守「ID 与 PartitionKey 不得同源」，守不住「主体取得对不对」——把分区键取成
// 租户/包裹/种类/版本 同样能过门禁，而那与逐事件分区一模一样。这条断言补的就是那一格：
// 它经变异验证，把 PartitionKey 改回 eventID 时本用例变红。
func TestARederivedFinalOutcomeQueuesBehindTheOneItSupersedes(t *testing.T) {
	handoff, db, pool := newFinalOutcomeHandoffFixture(t)
	ctx := t.Context()

	for _, version := range []string{"outcome-v1", "outcome-v2"} {
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			return handoff.HandOffFinalOutcome(txCtx, finalOutcomeHandoffIntent(t, "parcel-1", version))
		}); err != nil {
			t.Fatalf("入队 %s：%v", version, err)
		}
	}

	first := finalOutcomeEventID("parcel-1", "outcome-v1")
	second := finalOutcomeEventID("parcel-1", "outcome-v2")
	for _, eventID := range []string{first, second} {
		if count := countOutboxEventsIn(t, pool, eventID); count != 1 {
			t.Fatalf("%s 行数 = %d, want 1——重派生必须自成一份，不能被当成重放吞掉", eventID, count)
		}
	}

	firstPartition := partitionKeyOf(t, pool, first)
	if secondPartition := partitionKeyOf(t, pool, second); firstPartition != secondPartition {
		t.Fatalf("同一包裹的两版落在不同分区：%q 与 %q——重派生会与原终局失去先后",
			firstPartition, secondPartition)
	}

	// 不同包裹必须各自成区，否则一个包裹的失败会拖住另一个。
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffFinalOutcome(txCtx, finalOutcomeHandoffIntent(t, "parcel-2", "outcome-v1"))
	}); err != nil {
		t.Fatalf("入队 parcel-2：%v", err)
	}
	if other := partitionKeyOf(t, pool, finalOutcomeEventID("parcel-2", "outcome-v1")); other == firstPartition {
		t.Fatalf("两个包裹共用分区 %q——一个的失败会拖住另一个", other)
	}
}

func TestFinalOutcomeRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newFinalOutcomeHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffFinalOutcome(txCtx, ports.FinalOutcomeHandoffIntent{})
	})
	if err == nil {
		t.Fatal("缺采用键的终局意图入了队")
	}
}
