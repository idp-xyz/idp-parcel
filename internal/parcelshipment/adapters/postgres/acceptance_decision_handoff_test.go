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
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件是事务发布样板的第二个实例：形状与 source_data_handoff_test 相同，各自守
// 自己的意图类型——两处同形正是「照样板逐个换真」的证据，第三个实例出现前不提炼。

func newDecisionHandoffFixture(t *testing.T) (*adapter.OutboxAcceptanceDecisionHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxAcceptanceDecisionHandoff(db, store, handoffClock{
		at: time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func decisionIntent(t *testing.T, decisionID string) ports.AcceptanceDecisionHandoffIntent {
	t.Helper()

	requestID, err := domain.NewShipmentRequestID("request-1")
	if err != nil {
		t.Fatalf("委托标识：%v", err)
	}
	version, err := domain.NewSubmissionVersionID("submission-v1")
	if err != nil {
		t.Fatalf("提交版本：%v", err)
	}
	decision, err := domain.NewAcceptanceDecisionID(decisionID)
	if err != nil {
		t.Fatalf("决定标识：%v", err)
	}
	return ports.AcceptanceDecisionHandoffIntent{
		Identity:          identity(t, "tenant-a", "customer-a", "portal", "req-1"),
		ShipmentRequestID: requestID,
		SubmissionVersion: version,
		DecisionID:        decision,
		State:             domain.ShipmentRequestAccepted,
	}
}

// TestDecisionIntentFollowsTheTransactionalTemplate 证第二个意图适配器复现样板的
// 全部四条：同事务提交在、回滚不在、重发同一份不出第二份、无事务拒。
func TestDecisionIntentFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newDecisionHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return handoff.HandOffAcceptanceDecision(txCtx, decisionIntent(t, "decision-1"))
	})
	if count := countOutboxEventsIn(t, pool, "decision-1"); count != 1 {
		t.Fatalf("decision-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffAcceptanceDecision(txCtx, decisionIntent(t, "decision-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, "decision-2"); count != 0 {
		t.Fatalf("回滚后 decision-2 行数 = %d, want 0", count)
	}

	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return handoff.HandOffAcceptanceDecision(txCtx, decisionIntent(t, "decision-1"))
	})
	if count := countOutboxEventsIn(t, pool, "decision-1"); count != 1 {
		t.Fatalf("重发后 decision-1 行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffAcceptanceDecision(ctx, decisionIntent(t, "decision-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func countOutboxEventsIn(t *testing.T, pool *pgxpool.Pool, eventID string) int {
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
