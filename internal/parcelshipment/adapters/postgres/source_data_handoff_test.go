package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「发布意图与业务结果同一提交」——基线原子性缺口的
// 第一个真实证明。用真实引擎而非替身，因为要证的恰好只有真实事务才会暴露：提交后
// 两者都在、回滚后两者都不在、缺事务直接拒绝。

type handoffClock struct{ at time.Time }

func (clock handoffClock) Now() time.Time { return clock.at }

func newHandoffFixture(t *testing.T) (
	*adapter.SourceSubmissions,
	*adapter.OutboxSourceDataHandoff,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewSourceSubmissions(db)
	if err != nil {
		t.Fatalf("构造来源仓储：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxSourceDataHandoff(db, store, handoffClock{
		at: time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return repository, handoff, db.Transactor(), pool
}

func versionIntent(t *testing.T, version string) ports.SourceDataVersionHandoffIntent {
	t.Helper()

	versionID, err := domain.NewSourceDataVersionID(version)
	if err != nil {
		t.Fatalf("版本标识：%v", err)
	}
	requestID, err := domain.NewShipmentRequestID("request-1")
	if err != nil {
		t.Fatalf("委托标识：%v", err)
	}
	group, err := domain.NewSourceDataGroupReference("customs-declaration")
	if err != nil {
		t.Fatalf("资料组引用：%v", err)
	}
	scope, err := domain.NewShipmentScopedSourceData(requestID, group)
	if err != nil {
		t.Fatalf("资料范围：%v", err)
	}
	return ports.SourceDataVersionHandoffIntent{
		Identity: identity(t, "tenant-a", "customer-a", "portal", "req-1"),
		Version:  versionID,
		Scope:    scope,
	}
}

// TestIntentCommitsAtomicallyWithTheBusinessWrite 证同一事务里的业务写入与意图入队
// 同生：提交后来源行与 outbox 行都在。这正是全部 handoff 端口注释里「事务发布仍
// 阻断」等待的那件事的解除形态。
func TestIntentCommitsAtomicallyWithTheBusinessWrite(t *testing.T) {
	repository, handoff, transactor, pool := newHandoffFixture(t)
	ctx := t.Context()

	preserved := fingerprint(t, "tenant-a", "customer-a", "portal", "req-1", "digest-1")
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := repository.Preserve(txCtx, preserved); err != nil {
			return err
		}
		return handoff.HandOffSourceDataVersion(txCtx, versionIntent(t, "version-1"))
	})

	if _, exists, err := repository.FindPreserved(ctx, preserved.Identity()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countOutboxEvents(t, pool, "version-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
}

// TestRollbackDropsBothTheWriteAndTheIntent 证回滚时两者一并消失——不存在「版本没
// 保住而下游已被通知」的分岔，反方向（意图丢了而版本在）由重放重发同一份补。
func TestRollbackDropsBothTheWriteAndTheIntent(t *testing.T) {
	repository, handoff, transactor, pool := newHandoffFixture(t)
	ctx := t.Context()

	preserved := fingerprint(t, "tenant-a", "customer-a", "portal", "req-1", "digest-1")
	rollback := errors.New("回滚")
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := repository.Preserve(txCtx, preserved); err != nil {
			return err
		}
		if err := handoff.HandOffSourceDataVersion(txCtx, versionIntent(t, "version-1")); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, findErr := repository.FindPreserved(ctx, preserved.Identity()); findErr != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", findErr, exists)
	}
	if count := countOutboxEvents(t, pool, "version-1"); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}
}

// TestResendingTheSameIntentIsIdempotent 证重发是同一份而不是第二份：意图由版本标识
// 认领（ADR-0043），撞上已入队的同一份按成功收场——AT-PS-031「仅重试同一发布意图」
// 的持久化面。
func TestResendingTheSameIntentIsIdempotent(t *testing.T) {
	_, handoff, transactor, pool := newHandoffFixture(t)
	ctx := t.Context()

	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return handoff.HandOffSourceDataVersion(txCtx, versionIntent(t, "version-1"))
	})
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		return handoff.HandOffSourceDataVersion(txCtx, versionIntent(t, "version-1"))
	})

	if count := countOutboxEvents(t, pool, "version-1"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

// TestIntentRefusesToRunOutsideATransaction 证意图入队不会在缺少事务时改用连接池
// ——那样它就与业务写入不再同生共死，而两端的测试都看不出来（PBC-08 同一条）。
func TestIntentRefusesToRunOutsideATransaction(t *testing.T) {
	_, handoff, _, pool := newHandoffFixture(t)
	ctx := t.Context()

	if err := handoff.HandOffSourceDataVersion(ctx, versionIntent(t, "version-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countOutboxEvents(t, pool, "version-1"); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func countOutboxEvents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
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
