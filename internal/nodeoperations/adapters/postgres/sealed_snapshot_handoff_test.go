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

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证封装快照意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺认领键响亮报错。信封 ID 由租户加单元加封签认领。入队走 EnqueueOnce。

const sealedSnapshotEventType = "node-operations.sealed-snapshot.recorded"

type snapshotHandoffClock struct{ at time.Time }

func (clock snapshotHandoffClock) Now() time.Time { return clock.at }

type snapshotHandoffFixture struct {
	units      *adapter.ConsolidationUnits
	handoff    *adapter.OutboxSealedSnapshotHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newSnapshotHandoffFixture(t *testing.T) *snapshotHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	units, err := adapter.NewConsolidationUnits(db)
	if err != nil {
		t.Fatalf("构造合箱库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxSealedSnapshotHandoff(db, store, snapshotHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &snapshotHandoffFixture{
		units: units, handoff: handoff, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *snapshotHandoffFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func sealedSnapshotIntent(t *testing.T, tenant, unitID, seal string) (ports.SealedSnapshotHandoffIntent, *domain.ConsolidationUnit) {
	t.Helper()
	unit := openConsolidation(t, unitID, "asset-7")
	if err := unit.AddMember(ref(t, domain.NewHandlingUnitID, "hu-1")); err != nil {
		t.Fatalf("加入成员：%v", err)
	}
	if err := unit.Seal(
		ref(t, domain.NewSealReference, seal),
		ref(t, domain.NewWorkBasisReference, "PACK/1"),
		workSource(t, "src-seal-"+seal, consolidationAt),
	); err != nil {
		t.Fatalf("封装：%v", err)
	}
	return ports.SealedSnapshotHandoffIntent{
		TenantID: ref(t, domain.NewTenantID, tenant),
		Unit:     unit.ID(),
		Snapshot: unit.Snapshots()[0],
	}, unit
}

func snapshotEventID(tenant, unit, seal string) string {
	return tenant + "/" + unit + "/" + seal
}

func sealOpenedUnit(t *testing.T, unit *domain.ConsolidationUnit, seal string) {
	t.Helper()
	if err := unit.AddMember(ref(t, domain.NewHandlingUnitID, "hu-1")); err != nil {
		t.Fatalf("加入成员：%v", err)
	}
	if err := unit.Seal(
		ref(t, domain.NewSealReference, seal),
		ref(t, domain.NewWorkBasisReference, "PACK/1"),
		workSource(t, "src-seal-"+seal, consolidationAt),
	); err != nil {
		t.Fatalf("封装：%v", err)
	}
}

// TestTwoSnapshotsOfTheSameUnitShareOnePartition 钉住两个字段的分工。
//
// Unseal 后再 Seal 是同一单元的又一份快照（历史快照原样保留），两件都要成立：**都入队**
// （ID 含封签，第二份不被 EnqueueOnce 当成重放吞掉）**且同分区**（分区键只到租户+单元，
// 后一份排在前一份后面）。封签进分区键每份就自成一区，下游读到的成员清单就没有先后可言。
func TestTwoSnapshotsOfTheSameUnitShareOnePartition(t *testing.T) {
	fixture := newSnapshotHandoffFixture(t)
	ctx := t.Context()

	unit := openConsolidation(t, "bag-1", "asset-7")
	sealOpenedUnit(t, unit, "seal-1")
	if err := unit.Unseal(ref(t, domain.NewWorkBasisReference, "UNPACK/1"), consolidationAt.Add(time.Hour)); err != nil {
		t.Fatalf("开封：%v", err)
	}
	if err := unit.Seal(
		ref(t, domain.NewSealReference, "seal-2"),
		ref(t, domain.NewWorkBasisReference, "PACK/2"),
		workSource(t, "src-reseal-2", consolidationAt.Add(2*time.Hour)),
	); err != nil {
		t.Fatalf("再封：%v", err)
	}
	snapshots := unit.Snapshots()
	if len(snapshots) != 2 {
		t.Fatalf("快照数 = %d, want 2", len(snapshots))
	}
	tenant := ref(t, domain.NewTenantID, "tenant-a")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if err := fixture.handoff.HandOffSnapshot(txCtx, ports.SealedSnapshotHandoffIntent{
			TenantID: tenant, Unit: unit.ID(), Snapshot: snapshots[0],
		}); err != nil {
			return err
		}
		return fixture.handoff.HandOffSnapshot(txCtx, ports.SealedSnapshotHandoffIntent{
			TenantID: tenant, Unit: unit.ID(), Snapshot: snapshots[1],
		})
	})

	firstID := snapshotEventID("tenant-a", "bag-1", "seal-1")
	secondID := snapshotEventID("tenant-a", "bag-1", "seal-2")
	if count := countSnapshotIntents(t, fixture.pool, firstID); count != 1 {
		t.Fatalf("首封行数 = %d, want 1", count)
	}
	if count := countSnapshotIntents(t, fixture.pool, secondID); count != 1 {
		t.Fatalf("再封行数 = %d, want 1——ID 不带封签时第二份会被 EnqueueOnce 静默吞掉", count)
	}

	if got := partitionKeyOf(t, fixture.pool, firstID); got != "tenant-a/bag-1" {
		t.Fatalf("首封分区键 = %q, want tenant-a/bag-1", got)
	}
	if got := partitionKeyOf(t, fixture.pool, secondID); got != "tenant-a/bag-1" {
		t.Fatalf("再封分区键 = %q；两份不同分区就没有先后可言", got)
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

func TestSealedSnapshotIntentCommitsAtomicallyWithTheUnit(t *testing.T) {
	fixture := newSnapshotHandoffFixture(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	opened := openConsolidation(t, "bag-1", "asset-7")
	eventID := snapshotEventID("tenant-a", "bag-1", "seal-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.units.Save(txCtx, tenant, opened); err != nil {
			return err
		}
		sealOpenedUnit(t, opened, "seal-1")
		if err := fixture.units.Update(txCtx, tenant, opened); err != nil {
			return err
		}
		return fixture.handoff.HandOffSnapshot(txCtx, ports.SealedSnapshotHandoffIntent{
			TenantID: tenant,
			Unit:     opened.ID(),
			Snapshot: opened.Snapshots()[0],
		})
	})

	found, exists, err := fixture.units.FindByID(ctx, tenant, opened.ID())
	if err != nil || !exists || !found.Sealed() {
		t.Fatalf("业务行不在：err=%v exists=%v sealed=%v", err, exists, found != nil && found.Sealed())
	}
	if count := countSnapshotIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := snapshotIntentType(t, fixture.pool, eventID); got != sealedSnapshotEventType {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestSealedSnapshotIntentRollbackDropsBoth(t *testing.T) {
	fixture := newSnapshotHandoffFixture(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	opened := openConsolidation(t, "bag-1", "asset-7")
	sealOpenedUnit(t, opened, "seal-1")
	intent := ports.SealedSnapshotHandoffIntent{
		TenantID: tenant, Unit: opened.ID(), Snapshot: opened.Snapshots()[0],
	}
	eventID := snapshotEventID("tenant-a", "bag-1", "seal-1")
	rollback := errors.New("回滚")

	fresh := openConsolidation(t, "bag-1", "asset-7")
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.units.Save(txCtx, tenant, fresh); err != nil {
			return err
		}
		if err := fixture.units.Update(txCtx, tenant, opened); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffSnapshot(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.units.FindByID(ctx, tenant, opened.ID()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countSnapshotIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	retry := openConsolidation(t, "bag-1", "asset-7")
	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.units.Save(txCtx, tenant, retry); err != nil {
			return err
		}
		if err := fixture.units.Update(txCtx, tenant, opened); err != nil {
			return err
		}
		return fixture.handoff.HandOffSnapshot(txCtx, intent)
	})
	if count := countSnapshotIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameSealedSnapshotIntentIsIdempotent(t *testing.T) {
	fixture := newSnapshotHandoffFixture(t)
	ctx := t.Context()
	intent, _ := sealedSnapshotIntent(t, "tenant-a", "bag-1", "seal-1")
	eventID := snapshotEventID("tenant-a", "bag-1", "seal-1")

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffSnapshot(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffSnapshot(txCtx, intent)
	})
	if count := countSnapshotIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestSealedSnapshotIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newSnapshotHandoffFixture(t)
	intent, _ := sealedSnapshotIntent(t, "tenant-a", "bag-1", "seal-1")
	eventID := snapshotEventID("tenant-a", "bag-1", "seal-1")
	if err := fixture.handoff.HandOffSnapshot(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countSnapshotIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignSealedSnapshotIntentIsLoud(t *testing.T) {
	fixture := newSnapshotHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffSnapshot(txCtx, ports.SealedSnapshotHandoffIntent{
			Unit: ref(t, domain.NewConsolidationUnitID, "bag-1"),
		})
	}); err == nil {
		t.Fatal("缺租户或封签的意图必须响亮报错")
	}
}

func countSnapshotIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, sealedSnapshotEventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func snapshotIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
