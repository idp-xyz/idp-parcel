package postgres_test

import (
	"context"
	"encoding/json"
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

// TestAPODCorrectionEnqueuesItsOwnEnvelopeInTheSamePartition 是 574caf8 那个缺陷的回归
// 用例：POD 更正必须自成一份信封，并与首登排同一个队。
//
// 修之前 ID 只取（租户+对象+尝试），更正算出与首登相同的字符串，`EnqueueOnce` 先查后插
// 于是静默 return nil——第二份根本不入队，而编排收到的是「交接成功」。载荷是指针式
// （下游按键读当前版）救不了它：消费者若已处理过首登信封就不会回来重读。
func TestAPODCorrectionEnqueuesItsOwnEnvelopeInTheSamePartition(t *testing.T) {
	handoff, db, pool := newDeliveryHandoffFixture(t)
	ctx := t.Context()

	first := deliveryHandoffIntentFor(t, "parcel-1", "pod-1")
	corrected := deliveryHandoffIntentFor(t, "parcel-1", "pod-2")
	corrected.Record.Delivery, _ = first.Record.Delivery.CorrectProof(
		deliveryValue(t, domain.NewDeliveryProofReference, "pod-2"),
		deliveryValue(t, domain.NewDeliveryResultVersion, "delivery/v2"),
		deliveredAtFixture.Add(24*time.Hour),
	)

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffEffectiveDelivery(txCtx, first); err != nil {
			return err
		}
		return handoff.HandOffEffectiveDelivery(txCtx, corrected)
	}); err != nil {
		t.Fatalf("首登与更正入队：%v", err)
	}

	firstID := "tenant-1/parcel-1/attempt-1/delivery/v1/effective-delivery"
	correctedID := "tenant-1/parcel-1/attempt-1/delivery/v2/effective-delivery"
	if count := countTFIntents(t, pool, firstID); count != 1 {
		t.Fatalf("首登行数 = %d, want 1", count)
	}
	if count := countTFIntents(t, pool, correctedID); count != 1 {
		t.Fatalf("更正行数 = %d, want 1——ID 不带版本时它会被 EnqueueOnce 静默吞掉", count)
	}

	// 两份必须同分区：更正先于首登送达，下游终局会落在已被取代的那一版上。
	if got := partitionKeyOf(t, pool, firstID); got != "tenant-1/parcel-1" {
		t.Fatalf("首登分区键 = %q, want tenant-1/parcel-1", got)
	}
	if got := partitionKeyOf(t, pool, correctedID); got != "tenant-1/parcel-1" {
		t.Fatalf("更正分区键 = %q；两代不同分区就没有先后可言", got)
	}

	// 载荷各自指名自己那一代：ID 管幂等让两代都入队，载荷管指名让下游读回自己那一代。
	// 两份只在 ID 上可分而载荷同形时，消费侧按键读当前版，先到那一代照样被吞。
	first4 := deliveryPayloadOf(t, pool, firstID)
	if first4.TenantID != "tenant-1" || first4.Object != "parcel-1" ||
		first4.Attempt != "attempt-1" || first4.Version != "delivery/v1" {
		t.Fatalf("首登载荷 = %+v, want 四维引用指到 delivery/v1", first4)
	}
	corrected4 := deliveryPayloadOf(t, pool, correctedID)
	if corrected4.Version != "delivery/v2" {
		t.Fatalf("更正载荷版本 = %q, want delivery/v2——载荷不带版本时两代在引用层无从分辨",
			corrected4.Version)
	}
}

// deliveryEnvelopePayload 摹写交付信封载荷的四个引用维。字段名就是跨上下文契约：VE
// 消费侧按同名字段译码；这里不导入消费方类型，两边各自持有、靠各自的真库测试互钉。
type deliveryEnvelopePayload struct {
	TenantID string `json:"tenantId"`
	Object   string `json:"object"`
	Attempt  string `json:"attempt"`
	Version  string `json:"version"`
}

func deliveryPayloadOf(t *testing.T, pool *pgxpool.Pool, eventID string) deliveryEnvelopePayload {
	t.Helper()

	var raw []byte
	if err := pool.QueryRow(t.Context(),
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&raw); err != nil {
		t.Fatalf("读取载荷：%v", err)
	}
	var payload deliveryEnvelopePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("译载荷：%v", err)
	}
	return payload
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
	if count := countTFIntents(t, pool, "tenant-1/parcel-1/attempt-1/delivery/v1/effective-delivery"); count != 1 {
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
	if count := countTFIntents(t, pool, "tenant-1/parcel-rollback/attempt-1/delivery/v1/effective-delivery"); count != 0 {
		t.Fatalf("回滚后 parcel-rollback 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffEffectiveDelivery(txCtx, deliveryHandoffIntentFor(t, "parcel-1", "pod-replay"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countTFIntents(t, pool, "tenant-1/parcel-1/attempt-1/delivery/v1/effective-delivery"); count != 1 {
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
