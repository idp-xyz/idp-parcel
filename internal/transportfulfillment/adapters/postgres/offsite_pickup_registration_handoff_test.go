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

func newPickupRegistrationHandoffFixture(t *testing.T) (*adapter.OutboxOffsitePickupRegistrationHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxOffsitePickupRegistrationHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func pickupRegistrationHandoffIntent(t *testing.T, object, attempt string) ports.OffsitePickupRegistrationIntent {
	t.Helper()
	pickup, err := domain.FormOffsitePickup(domain.OffsitePickupSpec{
		TenantID:   deliveryValue(t, domain.NewTenantID, "tenant-a"),
		Object:     deliveryValue(t, domain.NewCarriedObjectReference, object),
		Task:       deliveryValue(t, domain.NewPickupTaskReference, "pickup-task-1"),
		Attempt:    deliveryValue(t, domain.NewAttemptReference, attempt),
		Place:      deliveryValue(t, domain.NewPickupPlaceReference, "customer-warehouse-1"),
		Control:    deliveryValue(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-1"),
		ExecutedBy: deliveryValue(t, domain.NewExecutingPartyReference, "courier-1"),
		Version:    deliveryValue(t, domain.NewPickupResultVersion, "pickup-result/v1"),
		OccurredAt: time.Date(2026, 8, 13, 8, 15, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造揽收登记：%v", err)
	}
	return ports.OffsitePickupRegistrationIntent{
		Record: ports.OffsitePickupRecord{
			Key: ports.OffsitePickupKey{
				TenantID: pickup.TenantID(),
				Object:   pickup.Object(),
				Attempt:  pickup.Attempt(),
			},
			ContentDigest: "digest-" + object,
			Pickup:        pickup,
			RecordedAt:    time.Date(2026, 8, 13, 8, 45, 0, 0, time.UTC),
		},
	}
}

// TestOffsitePickupRegistrationFollowsTheTransactionalTemplate 证对象级揽收登记意图
// 复现样板四条：首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 由揽收幂等键认领。
func TestOffsitePickupRegistrationFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newPickupRegistrationHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffOffsitePickupRegistration(txCtx, pickupRegistrationHandoffIntent(t, "parcel-1", "attempt-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-1/attempt-1/offsite-pickup-registration"); count != 1 {
		t.Fatalf("parcel-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffOffsitePickupRegistration(txCtx, pickupRegistrationHandoffIntent(t, "parcel-rollback", "attempt-1")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-rollback/attempt-1/offsite-pickup-registration"); count != 0 {
		t.Fatalf("回滚后 parcel-rollback 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffOffsitePickupRegistration(txCtx, pickupRegistrationHandoffIntent(t, "parcel-1", "attempt-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-1/attempt-1/offsite-pickup-registration"); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffOffsitePickupRegistration(ctx, pickupRegistrationHandoffIntent(t, "parcel-ntx", "attempt-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestOffsitePickupRegistrationRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newPickupRegistrationHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffOffsitePickupRegistration(txCtx, ports.OffsitePickupRegistrationIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的揽收登记意图入了队")
	}
}

// TestOffsitePickupRegistrationDoesNotCollideWithEffectiveDelivery 证同对象同尝试的
// 交付生效已入队后，揽收登记仍能插入第二份——类型段把两口错开。
func TestOffsitePickupRegistrationDoesNotCollideWithEffectiveDelivery(t *testing.T) {
	pickup, db, pool := newPickupRegistrationHandoffFixture(t)
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	delivery, err := adapter.NewOutboxEffectiveDeliveryHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造交付生效适配器：%v", err)
	}

	deliveryIntent := deliveryHandoffIntentFor(t, "parcel-1", "pod-1")
	deliveryIntent.Record.Key.TenantID = deliveryValue(t, domain.NewTenantID, "tenant-a")

	ctx := t.Context()
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := delivery.HandOffEffectiveDelivery(txCtx, deliveryIntent); err != nil {
			return err
		}
		return pickup.HandOffOffsitePickupRegistration(txCtx, pickupRegistrationHandoffIntent(t, "parcel-1", "attempt-1"))
	}); err != nil {
		t.Fatalf("同事务两口：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-1/attempt-1"); count != 1 {
		t.Fatalf("交付生效行数 = %d, want 1", count)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-1/attempt-1/offsite-pickup-registration"); count != 1 {
		t.Fatalf("揽收登记行数 = %d, want 1——应是第二份而不是被交付生效吞掉", count)
	}
}
