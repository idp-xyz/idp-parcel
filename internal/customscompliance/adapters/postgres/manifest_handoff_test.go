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

// 本文件对真实 PostgreSQL 16 证舱单引用意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺舱单标识响亮报错。信封 ID 由舱单身份加来源版本认领，分区键只到
// 舱单身份。入队走 EnqueueOnce。

type manifestHandoffClock struct{ at time.Time }

func (clock manifestHandoffClock) Now() time.Time { return clock.at }

type manifestHandoffFixture struct {
	manifests  *adapter.Manifests
	handoff    *adapter.OutboxManifestHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newManifestHandoffFixture(t *testing.T) *manifestHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	manifests, err := adapter.NewManifests(db)
	if err != nil {
		t.Fatalf("构造舱单库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxManifestHandoff(db, store, manifestHandoffClock{
		at: time.Date(2026, 8, 14, 18, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &manifestHandoffFixture{
		manifests:  manifests,
		handoff:    handoff,
		transactor: db.Transactor(),
		pool:       pool,
	}
}

func (fixture *manifestHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func manifestIntent(t *testing.T, tenant string) ports.ManifestHandoffIntent {
	t.Helper()
	return ports.ManifestHandoffIntent{
		TenantID:  fmcValue(t, domain.NewTenantID, tenant),
		Reference: acceptedManifest(t),
	}
}

func manifestHandoffEventID(tenant string, intent ports.ManifestHandoffIntent) string {
	return tenant + "/" + intent.Reference.Manifest().String() +
		"/" + intent.Reference.Version().String()
}

// TestARevisedManifestEnqueuesItsOwnEnvelopeInTheSamePartition 钉住两个字段的分工。
//
// 承运商更正推进来源版本走的是同一个舱单身份（ReceiveManifestHandler.Revise），两件都要
// 成立：**都入队**（ID 含来源版本，修订版不被 EnqueueOnce 当成重放吞掉——否则修订后的
// 舱单永远到不了下游）**且同分区**（分区键只到租户+舱单，修订版排在首版后面）。
func TestARevisedManifestEnqueuesItsOwnEnvelopeInTheSamePartition(t *testing.T) {
	fixture := newManifestHandoffFixture(t)
	ctx := t.Context()

	first := manifestIntent(t, "tenant-a")
	revised, err := first.Reference.Revise(
		fmcValue(t, domain.NewManifestSourceVersion, "manifest/v2"),
		fmcValue(t, domain.NewDecisionScopeReference, "manifest-scope-2"),
		"carrier-report/corrected",
		fmcBaseAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("修订舱单：%v", err)
	}
	second := ports.ManifestHandoffIntent{TenantID: first.TenantID, Reference: revised}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.handoff.HandOffManifest(txCtx, first); err != nil {
			return err
		}
		return fixture.handoff.HandOffManifest(txCtx, second)
	})

	firstID := "tenant-a/carrier-manifest/MAWB-123/manifest/v1"
	revisedID := "tenant-a/carrier-manifest/MAWB-123/manifest/v2"
	if count := countManifestIntents(t, fixture.pool, firstID); count != 1 {
		t.Fatalf("首版行数 = %d, want 1", count)
	}
	if count := countManifestIntents(t, fixture.pool, revisedID); count != 1 {
		t.Fatalf("修订版行数 = %d, want 1——ID 不带版本时它会被 EnqueueOnce 静默吞掉", count)
	}

	if got := partitionKeyOf(t, fixture.pool, firstID); got != "tenant-a/carrier-manifest/MAWB-123" {
		t.Fatalf("首版分区键 = %q, want tenant-a/carrier-manifest/MAWB-123", got)
	}
	if got := partitionKeyOf(t, fixture.pool, revisedID); got != "tenant-a/carrier-manifest/MAWB-123" {
		t.Fatalf("修订版分区键 = %q；两版不同分区就没有先后可言", got)
	}
}

func TestManifestIntentCommitsAtomicallyWithTheReference(t *testing.T) {
	fixture := newManifestHandoffFixture(t)
	ctx := t.Context()
	intent := manifestIntent(t, "tenant-a")
	eventID := manifestHandoffEventID("tenant-a", intent)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.manifests.Save(txCtx, intent.TenantID, intent.Reference); err != nil {
			return err
		}
		return fixture.handoff.HandOffManifest(txCtx, intent)
	})

	if _, exists, err := fixture.manifests.FindByManifest(ctx, intent.TenantID, intent.Reference.Manifest()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countManifestIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := manifestIntentType(t, fixture.pool, eventID); got != "customs-compliance.manifest.recorded" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestManifestIntentRollbackDropsBoth(t *testing.T) {
	fixture := newManifestHandoffFixture(t)
	ctx := t.Context()
	intent := manifestIntent(t, "tenant-a")
	eventID := manifestHandoffEventID("tenant-a", intent)
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.manifests.Save(txCtx, intent.TenantID, intent.Reference); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffManifest(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.manifests.FindByManifest(ctx, intent.TenantID, intent.Reference.Manifest()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countManifestIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.manifests.Save(txCtx, intent.TenantID, intent.Reference); err != nil {
			return err
		}
		return fixture.handoff.HandOffManifest(txCtx, intent)
	})
	if count := countManifestIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameManifestIntentIsIdempotent(t *testing.T) {
	fixture := newManifestHandoffFixture(t)
	ctx := t.Context()
	intent := manifestIntent(t, "tenant-a")
	eventID := manifestHandoffEventID("tenant-a", intent)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffManifest(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffManifest(txCtx, intent)
	})
	if count := countManifestIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestManifestIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newManifestHandoffFixture(t)
	intent := manifestIntent(t, "tenant-a")
	eventID := manifestHandoffEventID("tenant-a", intent)
	if err := fixture.handoff.HandOffManifest(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countManifestIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignManifestIntentIsLoud(t *testing.T) {
	fixture := newManifestHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffManifest(txCtx, ports.ManifestHandoffIntent{
			TenantID: fmcValue(t, domain.NewTenantID, "tenant-a"),
		})
	}); err == nil {
		t.Fatal("缺舱单标识的意图必须响亮报错")
	}
}

func countManifestIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.manifest.recorded",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func manifestIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
