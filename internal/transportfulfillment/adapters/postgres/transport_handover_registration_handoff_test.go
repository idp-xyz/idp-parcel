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

func newHandoverRegistrationHandoffFixture(t *testing.T) (*adapter.OutboxTransportHandoverRegistrationHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxTransportHandoverRegistrationHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func handoverRegistrationHandoffIntent(t *testing.T, object, version string) ports.TransportHandoverRegistrationIntent {
	t.Helper()
	handover, err := domain.FormTransportHandover(domain.TransportHandoverSpec{
		TenantID:          deliveryValue(t, domain.NewTenantID, "tenant-a"),
		Object:            deliveryValue(t, domain.NewCarriedObjectReference, object),
		Scope:             deliveryValue(t, domain.NewHandoverScopeReference, "scope-1"),
		ReleasedBy:        deliveryValue(t, domain.NewHandoverPartyReference, "node-1"),
		ReceivedBy:        deliveryValue(t, domain.NewHandoverPartyReference, "carrier-1"),
		Verdict:           domain.ObjectHandedOver,
		ReleasingEvidence: deliveryValue(t, domain.NewHandoverEvidenceReference, "evidence-release-1"),
		ReceivingEvidence: deliveryValue(t, domain.NewHandoverEvidenceReference, "evidence-receive-1"),
		Rule:              deliveryValue(t, domain.NewHandoverRuleReference, "handover-rule/v1"),
		Version:           deliveryValue(t, domain.NewHandoverResultVersion, version),
		JudgedAt:          time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造交接判断：%v", err)
	}
	return ports.TransportHandoverRegistrationIntent{
		Record: ports.TransportHandoverRecord{
			Key: ports.TransportHandoverKey{
				TenantID: handover.TenantID(),
				Object:   handover.Object(),
				Scope:    handover.Scope(),
				Version:  handover.Version(),
			},
			ContentDigest: "digest-" + object + "-" + version,
			Handover:      handover,
			RecordedAt:    time.Date(2026, 8, 13, 14, 30, 0, 0, time.UTC),
		},
	}
}

// TestTransportHandoverRegistrationFollowsTheTransactionalTemplate 证交接判断意图复现
// 样板四条：首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 由交接判断键认领。
func TestTransportHandoverRegistrationFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newHandoverRegistrationHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffTransportHandover(txCtx, handoverRegistrationHandoffIntent(t, "parcel-1", "hv-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-1/scope-1/hv-1"); count != 1 {
		t.Fatalf("parcel-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffTransportHandover(txCtx, handoverRegistrationHandoffIntent(t, "parcel-rollback", "hv-rollback")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-rollback/scope-1/hv-rollback"); count != 0 {
		t.Fatalf("回滚后 parcel-rollback 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffTransportHandover(txCtx, handoverRegistrationHandoffIntent(t, "parcel-1", "hv-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-1/scope-1/hv-1"); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffTransportHandover(ctx, handoverRegistrationHandoffIntent(t, "parcel-ntx", "hv-ntx")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestHandoverVersionsOfOneObjectShareAPartition 钉住分区键与信封 ID 的分工：ID 带
// 版本所以两代都入队（更正不丢），分区键只取（租户+对象）所以两代排同一个队（更正不会
// 先于它更正的那一版送达）。
//
// 没有这条断言，把 PartitionKey 改回 eventID 不会让任何东西变红——每份信封自成一个
// 分区，框架的顺序保证落空，而乱序投递不报任何错、只是结果错。
func TestHandoverVersionsOfOneObjectShareAPartition(t *testing.T) {
	handoff, db, pool := newHandoverRegistrationHandoffFixture(t)
	ctx := t.Context()

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffTransportHandover(txCtx, handoverRegistrationHandoffIntent(t, "parcel-1", "hv-1")); err != nil {
			return err
		}
		// 更正换出的新版本：同一对象、同一范围，另一个版本号。
		if err := handoff.HandOffTransportHandover(txCtx, handoverRegistrationHandoffIntent(t, "parcel-1", "hv-2")); err != nil {
			return err
		}
		return handoff.HandOffTransportHandover(txCtx, handoverRegistrationHandoffIntent(t, "parcel-2", "hv-1"))
	}); err != nil {
		t.Fatalf("入队三份意图：%v", err)
	}

	// 两代都在：ID 带版本，所以更正不会被 EnqueueOnce 当成重复吞掉。
	if count := countTFIntents(t, pool, "tenant-a/parcel-1/scope-1/hv-1"); count != 1 {
		t.Fatalf("首版行数 = %d, want 1", count)
	}
	if count := countTFIntents(t, pool, "tenant-a/parcel-1/scope-1/hv-2"); count != 1 {
		t.Fatalf("更正版行数 = %d, want 1——ID 不带版本时它会被当成重复静默丢掉", count)
	}

	if got := partitionKeyOf(t, pool, "tenant-a/parcel-1/scope-1/hv-1"); got != "tenant-a/parcel-1" {
		t.Fatalf("首版分区键 = %q, want tenant-a/parcel-1", got)
	}
	if got := partitionKeyOf(t, pool, "tenant-a/parcel-1/scope-1/hv-2"); got != "tenant-a/parcel-1" {
		t.Fatalf("更正版分区键 = %q；两代不同分区就没有先后可言", got)
	}
	// 不同对象不共享分区：一个对象卡住不该拖住另一个。
	if got := partitionKeyOf(t, pool, "tenant-a/parcel-2/scope-1/hv-1"); got != "tenant-a/parcel-2" {
		t.Fatalf("他对象分区键 = %q, want tenant-a/parcel-2", got)
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

func TestTransportHandoverRegistrationRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newHandoverRegistrationHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffTransportHandover(txCtx, ports.TransportHandoverRegistrationIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的交接判断意图入了队")
	}
}
