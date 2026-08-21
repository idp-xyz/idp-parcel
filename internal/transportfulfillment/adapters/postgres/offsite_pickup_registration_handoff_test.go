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

// TestTwoSuccessfulPickupsOnOneObjectShareAPartition 证同一载运对象跨尝试的第二次成功
// 登记：两次是同一条对象控制链的先后两段（TF CONTEXT 的跨段接续句、`AT-TF-098`），
// ID 各带尝试维所以两份都入队，分区键去掉尝试维所以两份排同一条队。
//
// 分区键若跟着 ID 走，第二段的登记可以先于第一段送达，而下游 parcel-shipment 的来源采用
// 是逐对象判断的——先后一乱，采用就落在已经结束的那一段上。
//
// 分区键保留类型段：并进（租户+对象）会与 VE 的（租户+包裹）投影分区合流，一封未决的揽收
// 就堵住同一包裹已经派生的追踪投影。断言里的类型段是这条边界，删了它用例照绿，
// cmd/parcel-dispatch 的揽收采用用例才会红。
func TestTwoSuccessfulPickupsOnOneObjectShareAPartition(t *testing.T) {
	handoff, db, pool := newPickupRegistrationHandoffFixture(t)
	ctx := t.Context()

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffOffsitePickupRegistration(txCtx, pickupRegistrationHandoffIntent(t, "parcel-1", "attempt-1")); err != nil {
			return err
		}
		return handoff.HandOffOffsitePickupRegistration(txCtx, pickupRegistrationHandoffIntent(t, "parcel-1", "attempt-2"))
	}); err != nil {
		t.Fatalf("同对象两尝试：%v", err)
	}

	first := "tenant-a/parcel-1/attempt-1/offsite-pickup-registration"
	second := "tenant-a/parcel-1/attempt-2/offsite-pickup-registration"
	if count := countTFIntents(t, pool, first); count != 1 {
		t.Fatalf("首段行数 = %d, want 1", count)
	}
	if count := countTFIntents(t, pool, second); count != 1 {
		t.Fatalf("后段行数 = %d, want 1——第二次成功被当成首段的重放吞掉了", count)
	}

	const want = "tenant-a/parcel-1/offsite-pickup-registration"
	if got := partitionKeyOf(t, pool, first); got != want {
		t.Fatalf("首段分区键 = %q, want %q", got, want)
	}
	if got := partitionKeyOf(t, pool, second); got != want {
		t.Fatalf("后段分区键 = %q, want %q——两段不同分区就没有先后可言", got, want)
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

	// 夹具第二参是 POD 证明，不是尝试。尝试维必须与登记相同，否则 UNIQUE(source, event_id)
	// 根本碰不到同对象同尝试的碰撞面，类型段有没有都绿。
	deliveryIntent := deliveryHandoffIntentFor(t, "parcel-1", "pod-1")
	deliveryIntent.Record.Key.TenantID = deliveryValue(t, domain.NewTenantID, "tenant-a")
	deliveryIntent.Record.Key.Attempt = deliveryValue(t, domain.NewAttemptReference, "attempt-1")

	ctx := t.Context()
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := delivery.HandOffEffectiveDelivery(txCtx, deliveryIntent); err != nil {
			return err
		}
		return pickup.HandOffOffsitePickupRegistration(txCtx, pickupRegistrationHandoffIntent(t, "parcel-1", "attempt-1"))
	}); err != nil {
		t.Fatalf("同事务两口：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-1/attempt-1/delivery/v1/effective-delivery"); count != 1 {
		t.Fatalf("交付生效行数 = %d, want 1", count)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-1/attempt-1/offsite-pickup-registration"); count != 1 {
		t.Fatalf("揽收登记行数 = %d, want 1——应是第二份而不是被交付生效吞掉", count)
	}
}
