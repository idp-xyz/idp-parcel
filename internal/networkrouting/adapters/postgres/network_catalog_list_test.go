package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// Covers: ADR-0077 Decision 四 — 上列这一格空表本身就是内容：从未登记的租户七族都
// 如实答空列表，不是错误；「未配置」的分辨属证据端口（LoadDefinitionsAt 的第二格），
// 上列面没有那一格。
func TestEmptyCatalogueListsAnswerEmptyLists(t *testing.T) {
	catalog, _, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")

	if rows, err := catalog.ListNodeVersions(ctx, tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("节点族：err=%v rows=%d", err, len(rows))
	}
	if rows, err := catalog.ListConnectionVersions(ctx, tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("连接族：err=%v rows=%d", err, len(rows))
	}
	if rows, err := catalog.ListLineVersions(ctx, tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("线路族：err=%v rows=%d", err, len(rows))
	}
	if rows, err := catalog.ListServiceAreaVersions(ctx, tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("服务区域族：err=%v rows=%d", err, len(rows))
	}
	if rows, err := catalog.ListServiceCalendarVersions(ctx, tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("服务日历族：err=%v rows=%d", err, len(rows))
	}
	if rows, err := catalog.ListAvailabilityAdjustments(ctx, tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("可用性调整族：err=%v rows=%d", err, len(rows))
	}
	if rows, err := catalog.ListRouteStrategyVersions(ctx, tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("路由策略族：err=%v rows=%d", err, len(rows))
	}
}

// Covers: 上列按族列版本行原文——历史版（接续闭合后的前版）与当前版一并透出，最新版
// 在前；租户是唯一授权边界，另一租户的同名身份不可见。与 LoadDefinitionsAt 的选版
// 快照相对照：那边同一时点每身份至多一行，这边整条版本历史都在。
func TestCatalogueListsCarryFullVersionHistoryScopedToTenant(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")
	other := scalar(t, domain.NewTenantID, "tenant-2")

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
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterNodeVersion(txCtx, other, ports.NodeDefinitionVersion{
			Code: "node-hub", Version: 1, BusinessTimezone: "America/New_York",
			EffectiveFrom: catalogAsOf,
		})
	})

	rows, err := catalog.ListNodeVersions(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列节点版本：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应列出整条版本历史（2 行），实得 %d：%+v", len(rows), rows)
	}
	// 同一身份内最新版在前；接续闭合后的前版带终点，当前版未闭。
	if rows[0].Version != 2 || rows[0].HasEffectiveTo {
		t.Fatalf("首行应为未闭的版本 2，实得 %+v", rows[0])
	}
	if rows[1].Version != 1 || !rows[1].HasEffectiveTo || !rows[1].EffectiveTo.Equal(catalogAsOf) {
		t.Fatalf("次行应为按新版生效时间接续闭合的版本 1，实得 %+v", rows[1])
	}
	for _, row := range rows {
		if row.BusinessTimezone == "America/New_York" {
			t.Fatal("跨租户可见")
		}
	}
}

// Covers: 族间不串——七族各登一行后，每个上列口只出自己族的那一行；调整族列整条
// 历史链（形成与解除各一行，最新版在前）。
func TestCatalogueFamiliesDoNotBleedIntoEachOther(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")
	effective := catalogAsOf.Add(-time.Hour)

	within(t, transactor, ctx, func(txCtx context.Context) error {
		if err := catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
			Code: "node-a", Version: 1, BusinessTimezone: "Asia/Shanghai",
			EffectiveFrom: effective,
		}); err != nil {
			return err
		}
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
	// 调整历史链：形成一行、解除一行。
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterAvailabilityAdjustment(txCtx, tenant, ports.AvailabilityAdjustmentStatement{
			Code: "adj-1", Version: 1, TargetKind: ports.TargetLine, TargetCode: "line-eu",
			Kind: ports.AdjustmentSuspension, Source: "NET-OPS/EVT-7",
			EffectiveAt: effective,
		})
	})
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterAvailabilityAdjustment(txCtx, tenant, ports.AvailabilityAdjustmentStatement{
			Code: "adj-1", Version: 2, TargetKind: ports.TargetLine, TargetCode: "line-eu",
			Kind: ports.AdjustmentSuspension, Source: "NET-OPS/EVT-7",
			EffectiveAt: effective,
			LiftedAt:    catalogAsOf.Add(time.Hour), HasLiftedAt: true,
		})
	})

	nodes, err := catalog.ListNodeVersions(ctx, tenant, 10)
	if err != nil || len(nodes) != 1 || nodes[0].Code != "node-a" {
		t.Fatalf("节点族走样：err=%v %+v", err, nodes)
	}
	connections, err := catalog.ListConnectionVersions(ctx, tenant, 10)
	if err != nil || len(connections) != 1 || connections[0].FromNode != "node-a" ||
		connections[0].ToNode != "node-b" {
		t.Fatalf("连接族走样：err=%v %+v", err, connections)
	}
	lines, err := catalog.ListLineVersions(ctx, tenant, 10)
	if err != nil || len(lines) != 1 || len(lines[0].Segments) != 1 ||
		lines[0].Segments[0] != "conn-a-b" {
		t.Fatalf("线路族走样（段链应按序落回）：err=%v %+v", err, lines)
	}
	areas, err := catalog.ListServiceAreaVersions(ctx, tenant, 10)
	if err != nil || len(areas) != 1 || areas[0].Code != "area-de" {
		t.Fatalf("服务区域族走样：err=%v %+v", err, areas)
	}
	calendars, err := catalog.ListServiceCalendarVersions(ctx, tenant, 10)
	if err != nil || len(calendars) != 1 || calendars[0].TargetKind != ports.TargetNode ||
		calendars[0].TargetCode != "node-a" {
		t.Fatalf("服务日历族走样：err=%v %+v", err, calendars)
	}
	strategies, err := catalog.ListRouteStrategyVersions(ctx, tenant, 10)
	if err != nil || len(strategies) != 1 || strategies[0].ApplicableScope != "scope-declared" {
		t.Fatalf("路由策略族走样：err=%v %+v", err, strategies)
	}

	adjustments, err := catalog.ListAvailabilityAdjustments(ctx, tenant, 10)
	if err != nil || len(adjustments) != 2 {
		t.Fatalf("调整族应列整条历史链（2 行）：err=%v %+v", err, adjustments)
	}
	if adjustments[0].Version != 2 || !adjustments[0].HasLiftedAt {
		t.Fatalf("调整链首行应为带解除时间的版本 2，实得 %+v", adjustments[0])
	}
	if adjustments[1].Version != 1 || adjustments[1].HasLiftedAt {
		t.Fatalf("调整链次行应为未解除的版本 1，实得 %+v", adjustments[1])
	}
}

// Covers: ADR-0077 Decision 五 — limit 非正拒（七口同一门禁）；limit 截断行数而不是
// 静默全量。
func TestCatalogueListsGuardTheirLimit(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")

	if _, err := catalog.ListNodeVersions(ctx, tenant, 0); err == nil {
		t.Fatal("limit=0 的节点上列被接受了")
	}
	if _, err := catalog.ListConnectionVersions(ctx, tenant, 0); err == nil {
		t.Fatal("limit=0 的连接上列被接受了")
	}
	if _, err := catalog.ListLineVersions(ctx, tenant, 0); err == nil {
		t.Fatal("limit=0 的线路上列被接受了")
	}
	if _, err := catalog.ListServiceAreaVersions(ctx, tenant, -1); err == nil {
		t.Fatal("limit=-1 的服务区域上列被接受了")
	}
	if _, err := catalog.ListServiceCalendarVersions(ctx, tenant, 0); err == nil {
		t.Fatal("limit=0 的服务日历上列被接受了")
	}
	if _, err := catalog.ListAvailabilityAdjustments(ctx, tenant, 0); err == nil {
		t.Fatal("limit=0 的调整上列被接受了")
	}
	if _, err := catalog.ListRouteStrategyVersions(ctx, tenant, 0); err == nil {
		t.Fatal("limit=0 的路由策略上列被接受了")
	}

	for _, code := range []string{"area-a", "area-b", "area-c"} {
		within(t, transactor, ctx, func(txCtx context.Context) error {
			return catalog.RegisterServiceAreaVersion(txCtx, tenant, ports.ServiceAreaDefinitionVersion{
				Code: code, Version: 1, EffectiveFrom: catalogAsOf,
			})
		})
	}
	rows, err := catalog.ListServiceAreaVersions(ctx, tenant, 2)
	if err != nil {
		t.Fatalf("上列服务区域：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("limit=2 却上列了 %d 行", len(rows))
	}
}
