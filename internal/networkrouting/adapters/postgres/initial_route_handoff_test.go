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

func newInitialRouteHandoffFixture(t *testing.T) (*adapter.OutboxInitialRouteHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxInitialRouteHandoff(db, store, handoffTestClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func initialRouteHandoffIntent(t *testing.T, parcel, corr string) ports.InitialRouteHandoffIntent {
	t.Helper()
	key := routeKey(t, "tenant-a", parcel)
	return ports.InitialRouteHandoffIntent{
		Correlation: scalar(t, domain.NewRequestCorrelationID, corr),
		Record: ports.InitialRouteRecord{
			Key:     key,
			Plan:    formedPlan(t, key, "RPV-0001"),
			HasPlan: true,
		},
	}
}

func initialRouteEventID(parcel string) string {
	return "tenant-a/initial-route/request-1/" + parcel + "/baseline-v1/LAST_MILE_DELIVERY"
}

// TestInitialRouteFollowsTheTransactionalTemplate 证初始路由意图复现样板四条：首发一行、
// 回滚无痕、重发同一份、无事务拒。信封 ID 由判断键认领并加类型段。
func TestInitialRouteFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newInitialRouteHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffInitialRoute(txCtx, initialRouteHandoffIntent(t, "parcel-1", "corr-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countJudgmentIntents(t, pool, initialRouteEventID("parcel-1")); count != 1 {
		t.Fatalf("parcel-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffInitialRoute(txCtx, initialRouteHandoffIntent(t, "parcel-rollback", "corr-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countJudgmentIntents(t, pool, initialRouteEventID("parcel-rollback")); count != 0 {
		t.Fatalf("回滚后 parcel-rollback 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffInitialRoute(txCtx, initialRouteHandoffIntent(t, "parcel-1", "corr-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countJudgmentIntents(t, pool, initialRouteEventID("parcel-1")); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffInitialRoute(ctx, initialRouteHandoffIntent(t, "parcel-ntx", "corr-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestInitialRouteRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newInitialRouteHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffInitialRoute(txCtx, ports.InitialRouteHandoffIntent{})
	})
	if err == nil {
		t.Fatal("缺判断键的初始路由意图入了队")
	}
}
