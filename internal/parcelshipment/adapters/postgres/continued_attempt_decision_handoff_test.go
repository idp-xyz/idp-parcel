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

// 本文件对真实 PostgreSQL 证`面单继续尝试决定`那封指针式信封（票 label-channel/30 判据 4 的适配器半边）：
// 复现样板四条、一封一决定且同包裹同分区、残缺意图拒入队。与写面编排接在一起的同事务用例在
// application 那一侧的真库用例里（cmd/parcel-api 装配测试），这里只证适配器自己。

func newContinuedAttemptDecisionHandoffFixture(t *testing.T) (*adapter.OutboxContinuedAttemptDecisionHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxContinuedAttemptDecisionHandoff(db, store, handoffClock{
		at: time.Date(2026, 9, 10, 22, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造决定判断意图适配器：%v", err)
	}
	return handoff, db, pool
}

func continuedAttemptDecisionIntent(
	t *testing.T,
	parcel, decision string,
	kind domain.ContinuedAttemptDecisionKind,
) ports.ContinuedAttemptDecisionHandoffIntent {
	t.Helper()
	return ports.ContinuedAttemptDecisionHandoffIntent{
		Tenant:     mustBuild(t, domain.NewTenantID, "tenant-a"),
		Parcel:     mustBuild(t, domain.NewDeclaredParcelID, parcel),
		Decision:   mustBuild(t, domain.NewContinuedAttemptDecisionID, decision),
		Kind:       kind,
		OccurredAt: time.Date(2026, 9, 10, 21, 30, 0, 0, time.UTC),
	}
}

func continuedAttemptDecisionEventID(parcel, decision string) string {
	return "tenant-a/continued-attempt-decision/" + parcel + "/" + decision + "/judgment-due"
}

// Covers: 复现样板四条——首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 由（租户 + 包裹 + 决定标识）
// 认领并加类型段。
func TestContinuedAttemptDecisionHandoffFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newContinuedAttemptDecisionHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	first := continuedAttemptDecisionIntent(t, "parcel-1", "CADN-1", domain.ControlledClosureDecision)
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffContinuedAttemptDecision(txCtx, first)
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, continuedAttemptDecisionEventID("parcel-1", "CADN-1")); count != 1 {
		t.Fatalf("首发行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffContinuedAttemptDecision(txCtx,
			continuedAttemptDecisionIntent(t, "parcel-1", "CADN-rollback", domain.ControlledClosureDecision)); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, continuedAttemptDecisionEventID("parcel-1", "CADN-rollback")); count != 0 {
		t.Fatalf("回滚后行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffContinuedAttemptDecision(txCtx, first)
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, continuedAttemptDecisionEventID("parcel-1", "CADN-1")); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffContinuedAttemptDecision(ctx,
		continuedAttemptDecisionIntent(t, "parcel-1", "CADN-ntx", domain.ControlledClosureDecision)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: ADR-0134 决定一——关过—重开—再关三条决定**各自入队**（决定标识进 ID）**且同分区**（分区键取租户 + 包裹，
// 同一包裹的多拍在一条队里先后消费）；不同包裹各自成区。
func TestEachContinuedAttemptDecisionEnqueuesBehindTheSameParcel(t *testing.T) {
	handoff, db, pool := newContinuedAttemptDecisionHandoffFixture(t)
	ctx := t.Context()

	decisions := []ports.ContinuedAttemptDecisionHandoffIntent{
		continuedAttemptDecisionIntent(t, "parcel-1", "CADN-close-1", domain.ControlledClosureDecision),
		continuedAttemptDecisionIntent(t, "parcel-1", "CADN-reopen-1", domain.ReopeningDecision),
		continuedAttemptDecisionIntent(t, "parcel-1", "CADN-close-2", domain.ControlledClosureDecision),
		continuedAttemptDecisionIntent(t, "parcel-2", "CADN-close-3", domain.ControlledClosureDecision),
	}
	for _, decision := range decisions {
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			return handoff.HandOffContinuedAttemptDecision(txCtx, decision)
		}); err != nil {
			t.Fatalf("入队 %s/%s：%v", decision.Parcel, decision.Decision, err)
		}
	}

	firstClosure := continuedAttemptDecisionEventID("parcel-1", "CADN-close-1")
	reopening := continuedAttemptDecisionEventID("parcel-1", "CADN-reopen-1")
	secondClosure := continuedAttemptDecisionEventID("parcel-1", "CADN-close-2")
	for _, eventID := range []string{firstClosure, reopening, secondClosure} {
		if count := countOutboxEventsIn(t, pool, eventID); count != 1 {
			t.Fatalf("%s 行数 = %d, want 1——每条决定各自成封", eventID, count)
		}
	}
	if partitionKeyOf(t, pool, firstClosure) != partitionKeyOf(t, pool, reopening) ||
		partitionKeyOf(t, pool, reopening) != partitionKeyOf(t, pool, secondClosure) {
		t.Fatal("同一包裹的三条决定落在不同分区——后一拍会与前一拍失去先后")
	}
	if other := partitionKeyOf(t, pool, continuedAttemptDecisionEventID("parcel-2", "CADN-close-3")); other == partitionKeyOf(t, pool, firstClosure) {
		t.Fatalf("两个包裹共用分区 %q——一件的失败会拖住另一件", other)
	}
	if count := countOutboxEventsByType(t, pool, adapter.ContinuedAttemptDecisionEventType); count != len(decisions) {
		t.Fatalf("类型 %s 共 %d 封, want %d", adapter.ContinuedAttemptDecisionEventType, count, len(decisions))
	}
}

func TestContinuedAttemptDecisionHandoffRefusesAnIncompleteIntent(t *testing.T) {
	handoff, db, _ := newContinuedAttemptDecisionHandoffFixture(t)
	noEffectiveTime := continuedAttemptDecisionIntent(t, "parcel-1", "CADN-1", domain.ControlledClosureDecision)
	noEffectiveTime.OccurredAt = time.Time{}
	for name, intent := range map[string]ports.ContinuedAttemptDecisionHandoffIntent{
		"blank":             {},
		"no kind":           continuedAttemptDecisionIntent(t, "parcel-1", "CADN-1", domain.ContinuedAttemptDecisionKindInvalid),
		"no effective time": noEffectiveTime,
	} {
		err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			return handoff.HandOffContinuedAttemptDecision(txCtx, intent)
		})
		if err == nil {
			t.Fatalf("%s：残缺的判断意图入了队", name)
		}
	}
}
