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

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证处置执行核对意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺核对键响亮报错。信封 ID 由核对幂等键认领。入队走 EnqueueOnce。

type verificationHandoffClock struct{ at time.Time }

func (clock verificationHandoffClock) Now() time.Time { return clock.at }

type verificationHandoffFixture struct {
	store      *adapter.DispositionVerifications
	handoff    *adapter.OutboxVerificationHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newVerificationHandoffFixture(t *testing.T) *verificationHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewDispositionVerifications(db)
	if err != nil {
		t.Fatalf("构造核对库：%v", err)
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxVerificationHandoff(db, outboxStore, verificationHandoffClock{
		at: time.Date(2026, 8, 14, 19, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &verificationHandoffFixture{
		store: store, handoff: handoff, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *verificationHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func verificationIntent(t *testing.T) ports.VerificationHandoffIntent {
	t.Helper()
	facts := []domain.ExecutionFact{
		verificationFact(t, "DESTRUCTION-EXEC/1", 1),
		verificationFact(t, "DESTRUCTION-EXEC/2", 1),
	}
	verification, err := domain.VerifyDispositionExecution(verificationDecision(t), facts, verificationBaseAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("核对：%v", err)
	}
	return ports.VerificationHandoffIntent{
		Key:          verificationKey(t, "tenant-a", facts),
		Verification: verification,
	}
}

func verificationHandoffEventID(key ports.VerificationKey) string {
	return key.TenantID.String() + "/" + key.Decision.String() + "/" + key.Digest
}

func TestVerificationIntentCommitsAtomicallyWithTheRecord(t *testing.T) {
	fixture := newVerificationHandoffFixture(t)
	ctx := t.Context()
	intent := verificationIntent(t)
	eventID := verificationHandoffEventID(intent.Key)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.store.Save(txCtx, intent.Key, intent.Verification); err != nil {
			return err
		}
		return fixture.handoff.HandOffVerification(txCtx, intent)
	})

	if _, exists, err := fixture.store.FindByKey(ctx, intent.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countVerificationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := verificationIntentType(t, fixture.pool, eventID); got != "customs-compliance.disposition-verification.recorded" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestVerificationIntentRollbackDropsBoth(t *testing.T) {
	fixture := newVerificationHandoffFixture(t)
	ctx := t.Context()
	intent := verificationIntent(t)
	eventID := verificationHandoffEventID(intent.Key)
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.store.Save(txCtx, intent.Key, intent.Verification); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffVerification(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.store.FindByKey(ctx, intent.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countVerificationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.store.Save(txCtx, intent.Key, intent.Verification); err != nil {
			return err
		}
		return fixture.handoff.HandOffVerification(txCtx, intent)
	})
	if count := countVerificationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameVerificationIntentIsIdempotent(t *testing.T) {
	fixture := newVerificationHandoffFixture(t)
	ctx := t.Context()
	intent := verificationIntent(t)
	eventID := verificationHandoffEventID(intent.Key)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffVerification(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffVerification(txCtx, intent)
	})
	if count := countVerificationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestVerificationIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newVerificationHandoffFixture(t)
	intent := verificationIntent(t)
	eventID := verificationHandoffEventID(intent.Key)
	if err := fixture.handoff.HandOffVerification(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countVerificationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignVerificationIntentIsLoud(t *testing.T) {
	fixture := newVerificationHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffVerification(txCtx, ports.VerificationHandoffIntent{
			Key: ports.VerificationKey{
				TenantID: verificationValue(t, domain.NewTenantID, "tenant-a"),
			},
		})
	}); err == nil {
		t.Fatal("缺核对键的意图必须响亮报错")
	}
}

// TestTwoVerificationsOfTheSameDecisionShareOnePartition 钉住两个字段的分工。
//
// 同一决定的核对随执行事实到达换指纹换版（部分覆盖 → 全覆盖），两件都要成立：**都入队**
// （ID 含事实集指纹，第二版不被 EnqueueOnce 当成重放吞掉）**且同分区**（分区键只到
// 租户+决定，后一版排在前一版后面）。分区键取整个核对键时每版自成一区，下游读到的
// 覆盖结论就没有先后可言。
func TestTwoVerificationsOfTheSameDecisionShareOnePartition(t *testing.T) {
	fixture := newVerificationHandoffFixture(t)
	ctx := t.Context()

	partialFacts := []domain.ExecutionFact{verificationFact(t, "DESTRUCTION-EXEC/1", 1)}
	partial, err := domain.VerifyDispositionExecution(verificationDecision(t), partialFacts, verificationBaseAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("部分覆盖核对：%v", err)
	}
	fullFacts := []domain.ExecutionFact{
		verificationFact(t, "DESTRUCTION-EXEC/1", 1),
		verificationFact(t, "DESTRUCTION-EXEC/2", 1),
	}
	full, err := domain.VerifyDispositionExecution(verificationDecision(t), fullFacts, verificationBaseAt.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("全覆盖核对：%v", err)
	}
	partialIntent := ports.VerificationHandoffIntent{Key: verificationKey(t, "tenant-a", partialFacts), Verification: partial}
	fullIntent := ports.VerificationHandoffIntent{Key: verificationKey(t, "tenant-a", fullFacts), Verification: full}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.handoff.HandOffVerification(txCtx, partialIntent); err != nil {
			return err
		}
		return fixture.handoff.HandOffVerification(txCtx, fullIntent)
	})

	partialID := verificationHandoffEventID(partialIntent.Key)
	fullID := verificationHandoffEventID(fullIntent.Key)
	if count := countVerificationIntents(t, fixture.pool, partialID); count != 1 {
		t.Fatalf("部分覆盖行数 = %d, want 1", count)
	}
	if count := countVerificationIntents(t, fixture.pool, fullID); count != 1 {
		t.Fatalf("全覆盖行数 = %d, want 1——ID 不带指纹时第二版会被 EnqueueOnce 静默吞掉", count)
	}

	if got := partitionKeyOf(t, fixture.pool, partialID); got != "tenant-a/decision-1" {
		t.Fatalf("部分覆盖分区键 = %q, want tenant-a/decision-1", got)
	}
	if got := partitionKeyOf(t, fixture.pool, fullID); got != "tenant-a/decision-1" {
		t.Fatalf("全覆盖分区键 = %q；两版不同分区就没有先后可言", got)
	}
}

func partitionKeyOf(t *testing.T, pool *pgxpool.Pool, eventID string) string {
	t.Helper()

	var partitionKey string
	if err := pool.QueryRow(t.Context(),
		`SELECT partition_key FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&partitionKey); err != nil {
		t.Fatalf("读取分区键：%v", err)
	}
	return partitionKey
}

func countVerificationIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.disposition-verification.recorded",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func verificationIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
	t.Helper()
	var eventType string
	err := pool.QueryRow(t.Context(),
		`SELECT event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&eventType)
	if err != nil {
		t.Fatalf("读事件类型：%v", err)
	}
	return eventType
}
