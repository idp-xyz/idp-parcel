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
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证网络收寄采用意图：与业务行同一提交、回滚一并消失、
// 重发同一份、无事务拒、缺采用键响亮报错。信封 ID 由采用键（含租户）再加类型段认领。
// 入队走 EnqueueOnce。

const networkIntakeEventType = "parcel-shipment.network-intake.recorded"

type networkIntakeHandoffClock struct{ at time.Time }

func (clock networkIntakeHandoffClock) Now() time.Time { return clock.at }

type networkIntakeHandoffFixture struct {
	adoptions  *adapter.IntakeAdoptions
	handoff    *adapter.OutboxNetworkIntakeHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newNetworkIntakeHandoffFixture(t *testing.T) *networkIntakeHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	adoptions, err := adapter.NewIntakeAdoptions(db)
	if err != nil {
		t.Fatalf("构造采用库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxNetworkIntakeHandoff(db, store, networkIntakeHandoffClock{
		at: time.Date(2026, 8, 14, 17, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &networkIntakeHandoffFixture{
		adoptions: adoptions, handoff: handoff, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *networkIntakeHandoffFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func networkIntakeIntent(t *testing.T) ports.NetworkIntakeHandoffIntent {
	t.Helper()
	return ports.NetworkIntakeHandoffIntent{
		Record: adoptedRecord(t, "tenant-a", "parcel-1", "SRV-1", "digest-intake-1"),
	}
}

func networkIntakeEventID(tenant, parcel, kind, version string) string {
	return tenant + "/" + parcel + "/" + kind + "/" + version + "/network-intake"
}

func TestNetworkIntakeIntentCommitsAtomicallyWithTheAdoption(t *testing.T) {
	fixture := newNetworkIntakeHandoffFixture(t)
	ctx := t.Context()
	intent := networkIntakeIntent(t)
	eventID := networkIntakeEventID("tenant-a", "parcel-1", "NODE_INTAKE", "SRV-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.adoptions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffNetworkIntake(txCtx, intent)
	})

	if _, exists, err := fixture.adoptions.FindByKey(ctx, intent.Record.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countNetworkIntakeIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := networkIntakeIntentType(t, fixture.pool, eventID); got != networkIntakeEventType {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestNetworkIntakeIntentRollbackDropsBoth(t *testing.T) {
	fixture := newNetworkIntakeHandoffFixture(t)
	ctx := t.Context()
	intent := networkIntakeIntent(t)
	eventID := networkIntakeEventID("tenant-a", "parcel-1", "NODE_INTAKE", "SRV-1")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.adoptions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffNetworkIntake(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.adoptions.FindByKey(ctx, intent.Record.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countNetworkIntakeIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.adoptions.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffNetworkIntake(txCtx, intent)
	})
	if count := countNetworkIntakeIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameNetworkIntakeIntentIsIdempotent(t *testing.T) {
	fixture := newNetworkIntakeHandoffFixture(t)
	ctx := t.Context()
	intent := networkIntakeIntent(t)
	eventID := networkIntakeEventID("tenant-a", "parcel-1", "NODE_INTAKE", "SRV-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffNetworkIntake(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffNetworkIntake(txCtx, intent)
	})
	if count := countNetworkIntakeIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

// TestACorrectedIntakeResultQueuesBehindTheOneItSupersedes 钉住两个字段的分工。
//
// 理由与终局那一口同形：ID 含来源结果版本，所以更正版本自成一份、不被 EnqueueOnce 当成重放
// 吞掉；分区键只到包裹，所以同一包裹的先后采认排在一条队里。少了后一半，更正先送达时下游
// 最后采用的是已被取代的那一版。
//
// 经变异验证：把 PartitionKey 改回 eventID 时本用例变红。门禁守「ID 与 PartitionKey 不得
// 同源」，守不住「主体取得对不对」——分区键取成 租户/包裹/种类/版本 照样过门禁，而那与逐
// 事件分区一模一样。
func TestACorrectedIntakeResultQueuesBehindTheOneItSupersedes(t *testing.T) {
	fixture := newNetworkIntakeHandoffFixture(t)
	ctx := t.Context()

	for _, version := range []string{"SRV-1", "SRV-2"} {
		intent := ports.NetworkIntakeHandoffIntent{
			Record: adoptedRecord(t, "tenant-a", "parcel-1", version, "digest-"+version),
		}
		fixture.within(t, ctx, func(txCtx context.Context) error {
			return fixture.handoff.HandOffNetworkIntake(txCtx, intent)
		})
	}

	first := networkIntakeEventID("tenant-a", "parcel-1", "NODE_INTAKE", "SRV-1")
	second := networkIntakeEventID("tenant-a", "parcel-1", "NODE_INTAKE", "SRV-2")
	for _, eventID := range []string{first, second} {
		if count := countNetworkIntakeIntents(t, fixture.pool, eventID); count != 1 {
			t.Fatalf("%s 行数 = %d，want 1——更正版本必须自成一份", eventID, count)
		}
	}

	firstPartition := partitionKeyOf(t, fixture.pool, first)
	if secondPartition := partitionKeyOf(t, fixture.pool, second); firstPartition != secondPartition {
		t.Fatalf("同一包裹的两版落在不同分区：%q 与 %q——更正会与原采认失去先后",
			firstPartition, secondPartition)
	}

	// 不同包裹必须各自成区，否则一个包裹的失败会拖住另一个。
	other := ports.NetworkIntakeHandoffIntent{
		Record: adoptedRecord(t, "tenant-a", "parcel-2", "SRV-1", "digest-parcel-2"),
	}
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffNetworkIntake(txCtx, other)
	})
	otherPartition := partitionKeyOf(t, fixture.pool,
		networkIntakeEventID("tenant-a", "parcel-2", "NODE_INTAKE", "SRV-1"))
	if otherPartition == firstPartition {
		t.Fatalf("两个包裹共用分区 %q——一个的失败会拖住另一个", otherPartition)
	}
}

func TestNetworkIntakeIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newNetworkIntakeHandoffFixture(t)
	intent := networkIntakeIntent(t)
	eventID := networkIntakeEventID("tenant-a", "parcel-1", "NODE_INTAKE", "SRV-1")
	if err := fixture.handoff.HandOffNetworkIntake(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countNetworkIntakeIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignNetworkIntakeIntentIsLoud(t *testing.T) {
	fixture := newNetworkIntakeHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffNetworkIntake(txCtx, ports.NetworkIntakeHandoffIntent{
			Record: ports.IntakeAdoptionRecord{
				Key: ports.IntakeAdoptionKey{TenantID: psTenant(t, "tenant-a")},
			},
		})
	}); err == nil {
		t.Fatal("缺采用键的意图必须响亮报错")
	}
}

func countNetworkIntakeIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, networkIntakeEventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func networkIntakeIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
