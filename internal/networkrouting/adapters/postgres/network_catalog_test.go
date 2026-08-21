package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// catalogAsOf 是选版用的判断时点样本。目录比较一律绝对时刻（CONTEXT：跨节点时间按
// 绝对时刻比较），测试里的区间边界都从它推。
var catalogAsOf = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

// Covers: 票 01 件③「显式未配置」的目录半边与 ADR-0052「空册与登记了一个空集合是
// 两回事」——从未登记的租户答`未配置`（第二格），不是错误也不是一份空目录。
func TestCatalogAnswersUnconfiguredForUnregisteredTenant(t *testing.T) {
	catalog, _, _ := newNetworkCatalog(t)

	_, configured, err := catalog.LoadDefinitionsAt(
		t.Context(), scalar(t, domain.NewTenantID, "tenant-1"), catalogAsOf)
	if err != nil {
		t.Fatalf("读未登记租户的目录不该是错误，实得：%v", err)
	}
	if configured {
		t.Fatal("从未登记过任何定义的租户被答成了已配置")
	}
}

// Covers: ADR-0052「空册与登记了一个空集合是两回事」的另一半——登记过（修订行在）
// 而 asOf 无适用版本，答已配置带空族；修订由登记册派生（票 01 件④），首笔写入即 1。
func TestCatalogDistinguishesConfiguredEmptyFromUnconfigured(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
			Code:             "node-hub",
			Version:          1,
			BusinessTimezone: "Asia/Shanghai",
			EffectiveFrom:    catalogAsOf.Add(24 * time.Hour),
		})
	})

	snapshot, configured, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf)
	if err != nil {
		t.Fatalf("读目录：%v", err)
	}
	if !configured {
		t.Fatal("登记过定义的租户被答成了未配置——「配了但此刻无适用版本」是业务事实不是空册")
	}
	if len(snapshot.Nodes) != 0 {
		t.Fatalf("asOf 早于生效时间，不该有适用节点，实得 %d 个", len(snapshot.Nodes))
	}
	if snapshot.Revision.String() != "1" {
		t.Fatalf("首笔写入后的修订应为 1，实得 %q", snapshot.Revision.String())
	}
}

// Covers: 件②「给定判断时点选出该时点生效的那一版」与 CONTEXT「新版本自明确生效时间
// 起参与新判断」——生效当刻属新版；登记未闭新版时前版按新版生效时间接续闭合。
func TestCatalogSelectsTheVersionEffectiveAtAsOf(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
			Code: "node-hub", Version: 1, BusinessTimezone: "Asia/Shanghai",
			EffectiveFrom: catalogAsOf.Add(-48 * time.Hour),
		})
	})
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
			Code: "node-hub", Version: 2, BusinessTimezone: "Europe/Berlin",
			EffectiveFrom: catalogAsOf,
		})
	})

	before, _, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf.Add(-time.Second))
	if err != nil {
		t.Fatalf("读生效前一秒：%v", err)
	}
	if len(before.Nodes) != 1 || before.Nodes[0].Version != 1 {
		t.Fatalf("生效前一秒应选版本 1，实得 %+v", before.Nodes)
	}
	if !before.Nodes[0].HasEffectiveTo || !before.Nodes[0].EffectiveTo.Equal(catalogAsOf) {
		t.Fatalf("前版应按新版生效时间接续闭合，实得 %+v", before.Nodes[0])
	}

	at, _, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf)
	if err != nil {
		t.Fatalf("读生效当刻：%v", err)
	}
	if len(at.Nodes) != 1 || at.Nodes[0].Version != 2 || at.Nodes[0].BusinessTimezone != "Europe/Berlin" {
		t.Fatalf("生效当刻应选版本 2，实得 %+v", at.Nodes)
	}
}

// Covers: 件②「两版同时适用交回错误不挑一个」（先例：VE 映射目录 ErrAmbiguousCatalog）。
// 重叠的已闭区间适配器造不出来，按 CHECK 用例的路子直插两行坏数据，读口必须兜错。
func TestCatalogRefusesToPickBetweenTwoApplicableVersions(t *testing.T) {
	catalog, _, pool := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")

	for _, insert := range []struct {
		version int
		from    time.Time
		to      time.Time
	}{
		{1, catalogAsOf.Add(-48 * time.Hour), catalogAsOf.Add(24 * time.Hour)},
		{2, catalogAsOf.Add(-24 * time.Hour), catalogAsOf.Add(48 * time.Hour)},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO network_routing.logistics_node_version
				(tenant_id, node_code, version, business_timezone, effective_from, effective_to)
			 VALUES ($1, 'node-hub', $2, 'Asia/Shanghai', $3, $4)`,
			tenant.String(), insert.version, insert.from, insert.to,
		); err != nil {
			t.Fatalf("植入重叠版本 %d：%v", insert.version, err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.network_catalog_revision (tenant_id, revision)
		 VALUES ($1, 1)`, tenant.String()); err != nil {
		t.Fatalf("植入修订行：%v", err)
	}

	_, _, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf)
	if !errors.Is(err, adapter.ErrAmbiguousNetworkCatalog) {
		t.Fatalf("两版同时适用应交回 ErrAmbiguousNetworkCatalog，实得：%v", err)
	}
}

// Covers: 件①调整表与 CONTEXT「调整必须记录来源、范围、生效时间和解除时间」「调整的
// 形成、变化和解除历史必须保留」——解除是追加新版本陈述不改旧行；当前陈述取历史链
// 最大版本，其生效窗口判 asOf。这与稳定六族的区间选版是两套机制，正是「稳定网络定义
// 和临时网络可用性调整必须分离」的选版半边。
func TestCatalogAdjustmentLiftAppendsHistoryAndStopsApplying(t *testing.T) {
	catalog, transactor, pool := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterAvailabilityAdjustment(txCtx, tenant, ports.AvailabilityAdjustmentStatement{
			Code: "adj-line-eu-1", Version: 1,
			TargetKind: ports.TargetLine, TargetCode: "line-eu",
			Kind: ports.AdjustmentSuspension, Source: "NET-OPS/EVT-7",
			EffectiveAt: catalogAsOf.Add(-time.Hour),
		})
	})

	during, _, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf)
	if err != nil {
		t.Fatalf("读生效中的调整：%v", err)
	}
	if len(during.Adjustments) != 1 || during.Adjustments[0].Kind != ports.AdjustmentSuspension ||
		during.Adjustments[0].HasLiftedAt {
		t.Fatalf("未解除的停运应在生效中，实得 %+v", during.Adjustments)
	}

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterAvailabilityAdjustment(txCtx, tenant, ports.AvailabilityAdjustmentStatement{
			Code: "adj-line-eu-1", Version: 2,
			TargetKind: ports.TargetLine, TargetCode: "line-eu",
			Kind: ports.AdjustmentSuspension, Source: "NET-OPS/EVT-7",
			EffectiveAt: catalogAsOf.Add(-time.Hour),
			LiftedAt:    catalogAsOf.Add(time.Hour), HasLiftedAt: true,
		})
	})

	beforeLift, _, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf)
	if err != nil {
		t.Fatalf("读解除时刻之前：%v", err)
	}
	if len(beforeLift.Adjustments) != 1 || beforeLift.Adjustments[0].Version != 2 ||
		!beforeLift.Adjustments[0].HasLiftedAt {
		t.Fatalf("解除时刻之前当前陈述应为版本 2 且带解除时间，实得 %+v", beforeLift.Adjustments)
	}

	afterLift, _, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("读解除之后：%v", err)
	}
	if len(afterLift.Adjustments) != 0 {
		t.Fatalf("解除之后调整不该再生效，实得 %+v", afterLift.Adjustments)
	}

	var history int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM network_routing.availability_adjustment
		  WHERE tenant_id = $1 AND adjustment_code = 'adj-line-eu-1'`,
		tenant.String(),
	).Scan(&history); err != nil {
		t.Fatalf("数历史行：%v", err)
	}
	if history != 2 {
		t.Fatalf("解除必须追加历史而不是改写，应有 2 行实得 %d", history)
	}
}

// Covers: 件④「修订从哪里来、怎么保证与事实同版」——修订由登记册派生，随每一笔目录
// 写入（不分种类）同事务 +1，两次取回之间无写入则不换代；七类各有登记口且落进同一份
// 快照。create_initial_route 的提交前重校正是拿这个等值比对判断证据有没有换代。
func TestCatalogRevisionAdvancesWithEveryRegistrationKind(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")
	effective := catalogAsOf.Add(-time.Hour)

	within(t, transactor, ctx, func(txCtx context.Context) error {
		if err := catalog.RegisterConnectionVersion(txCtx, tenant, ports.ConnectionDefinitionVersion{
			Code: "conn-a-b", Version: 1, FromNode: "node-a", ToNode: "node-b",
			BusinessTimezone: "Asia/Shanghai", EffectiveFrom: effective,
		}); err != nil {
			return err
		}
		if err := catalog.RegisterLineVersion(txCtx, tenant, ports.LineDefinitionVersion{
			Code: "line-eu", Version: 1, Segments: []string{"conn-a-b"},
			BusinessTimezone: "Asia/Shanghai", ApplicableScope: "scope-declared",
			EffectiveFrom: effective,
		}); err != nil {
			return err
		}
		if err := catalog.RegisterServiceAreaVersion(txCtx, tenant, ports.ServiceAreaDefinitionVersion{
			Code: "area-de", Version: 1, EffectiveFrom: effective,
		}); err != nil {
			return err
		}
		if err := catalog.RegisterServiceCalendarVersion(txCtx, tenant, ports.ServiceCalendarDefinitionVersion{
			TargetKind: ports.TargetNode, TargetCode: "node-a", Version: 1,
			EffectiveFrom: effective,
		}); err != nil {
			return err
		}
		return catalog.RegisterRouteStrategyVersion(txCtx, tenant, ports.RouteStrategyDefinitionVersion{
			Code: "strategy-1", Version: 1, ApplicableScope: "scope-declared",
			EffectiveFrom: effective,
		})
	})

	first, configured, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf)
	if err != nil || !configured {
		t.Fatalf("读五类登记后：err=%v configured=%v", err, configured)
	}
	if len(first.Connections) != 1 || len(first.Lines) != 1 || len(first.ServiceAreas) != 1 ||
		len(first.Calendars) != 1 || len(first.Strategies) != 1 {
		t.Fatalf("五类各应有一行适用版本，实得 %+v", first)
	}
	if first.Lines[0].Segments[0] != "conn-a-b" {
		t.Fatalf("线路段链没有按序落回，实得 %+v", first.Lines[0].Segments)
	}
	if first.Revision.String() != "5" {
		t.Fatalf("五笔写入后的修订应为 5，实得 %q", first.Revision.String())
	}

	unchanged, _, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf)
	if err != nil {
		t.Fatalf("重读：%v", err)
	}
	if unchanged.Revision != first.Revision {
		t.Fatal("两次取回之间没有写入，修订不该换代")
	}

	within(t, transactor, ctx, func(txCtx context.Context) error {
		if err := catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
			Code: "node-a", Version: 1, BusinessTimezone: "Asia/Shanghai",
			EffectiveFrom: effective,
		}); err != nil {
			return err
		}
		return catalog.RegisterAvailabilityAdjustment(txCtx, tenant, ports.AvailabilityAdjustmentStatement{
			Code: "adj-1", Version: 1, TargetKind: ports.TargetNode, TargetCode: "node-a",
			Kind: ports.AdjustmentClosure, Source: "NET-OPS/EVT-9",
			EffectiveAt: effective,
		})
	})

	second, _, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf)
	if err != nil {
		t.Fatalf("读七类齐备后：%v", err)
	}
	if second.Revision.String() != "7" {
		t.Fatalf("七笔写入后的修订应为 7，实得 %q", second.Revision.String())
	}
	if len(second.Nodes) != 1 || len(second.Adjustments) != 1 {
		t.Fatalf("节点与调整应各有一行，实得 nodes=%d adjustments=%d",
			len(second.Nodes), len(second.Adjustments))
	}
	if second.Revision == first.Revision {
		t.Fatal("目录变了修订必须换代——重校靠它发现证据已被替代")
	}
}

func newNetworkCatalog(t *testing.T) (*adapter.NetworkCatalog, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalog, err := adapter.NewNetworkCatalog(db)
	if err != nil {
		t.Fatalf("构造网络目录：%v", err)
	}
	return catalog, db.Transactor(), pool
}
