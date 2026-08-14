package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件是事务发布样板的第三个实例，也是提炼后 outboxintent.EnqueueOnce 的第一个
// 消费方门禁：四条与前两例同款，各自守自己的意图类型。

type handoffTestClock struct{ at time.Time }

func (clock handoffTestClock) Now() time.Time { return clock.at }

func newReachabilityHandoffFixture(t *testing.T) (*adapter.OutboxReachabilityHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxReachabilityHandoff(db, store, handoffTestClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func judgmentIntent(t *testing.T, correlation string) ports.ReachabilityJudgmentHandoffIntent {
	t.Helper()

	revision, err := domain.NewNetworkViewRevision("view-rev-7")
	if err != nil {
		t.Fatalf("视图修订：%v", err)
	}
	return ports.ReachabilityJudgmentHandoffIntent{
		Correlation:  scalar(t, domain.NewRequestCorrelationID, correlation),
		Key:          judgmentKey(t, "tenant-a"),
		Finding:      mixedFinding(t),
		JudgedAt:     time.Date(2026, 8, 14, 15, 30, 0, 0, time.UTC),
		ViewRevision: revision,
	}
}

// TestJudgmentIntentFollowsTheTransactionalTemplate 证第三个意图适配器（经提炼的
// EnqueueOnce）复现样板全部四条。
func TestJudgmentIntentFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newReachabilityHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffReachabilityJudgment(txCtx, judgmentIntent(t, "corr-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countJudgmentIntents(t, pool, "corr-1"); count != 1 {
		t.Fatalf("corr-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffReachabilityJudgment(txCtx, judgmentIntent(t, "corr-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countJudgmentIntents(t, pool, "corr-2"); count != 0 {
		t.Fatalf("回滚后 corr-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffReachabilityJudgment(txCtx, judgmentIntent(t, "corr-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countJudgmentIntents(t, pool, "corr-1"); count != 1 {
		t.Fatalf("重发后 corr-1 行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffReachabilityJudgment(ctx, judgmentIntent(t, "corr-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func countJudgmentIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
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
