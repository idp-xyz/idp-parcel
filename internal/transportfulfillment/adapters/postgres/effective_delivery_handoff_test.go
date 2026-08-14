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
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

func newDeliveryHandoffFixture(t *testing.T) (*adapter.OutboxEffectiveDeliveryHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxEffectiveDeliveryHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func deliveryHandoffIntentFor(t *testing.T, object, proof string) ports.EffectiveDeliveryHandoffIntent {
	t.Helper()
	record := deliveryRecord(t, proof, "delivery/v1")
	record.Key.Object = deliveryValue(t, domain.NewCarriedObjectReference, object)
	return ports.EffectiveDeliveryHandoffIntent{Record: record}
}

// TestEffectiveDeliveryFollowsTheTransactionalTemplate 证交付生效意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 由交付生效键认领。
func TestEffectiveDeliveryFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newDeliveryHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffEffectiveDelivery(txCtx, deliveryHandoffIntentFor(t, "parcel-1", "pod-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-1/parcel-1/attempt-1/effective-delivery"); count != 1 {
		t.Fatalf("交付信封行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffEffectiveDelivery(txCtx, deliveryHandoffIntentFor(t, "parcel-rollback", "pod-rollback")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-1/parcel-rollback/attempt-1/effective-delivery"); count != 0 {
		t.Fatalf("回滚后 parcel-rollback 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffEffectiveDelivery(txCtx, deliveryHandoffIntentFor(t, "parcel-1", "pod-replay"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-1/parcel-1/attempt-1/effective-delivery"); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffEffectiveDelivery(ctx, deliveryHandoffIntentFor(t, "parcel-ntx", "pod-ntx")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestEffectiveDeliveryRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newDeliveryHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffEffectiveDelivery(txCtx, ports.EffectiveDeliveryHandoffIntent{})
	})
	if err == nil {
		t.Fatal("缺交付生效键的意图入了队")
	}
}
