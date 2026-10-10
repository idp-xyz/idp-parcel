package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// Covers: 迁移 0018 的登记依据格（票 routing-first-cut/11）——稳定定义各族的版本行把依据原样落库、经运营查阅上列
// 原样读回；没给依据的版本如实落 NULL、读回是缺格，不折成空串或别的默认。引用串与租户自己的依据两形都往返。
func TestCatalogVersionsKeepTheirRegistrationBasis(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")
	effective := catalogAsOf.Add(-time.Hour)
	released := referenceconfig.Released()
	if len(released) == 0 {
		t.Fatal("发布清单为空")
	}
	cited := scalar(t, domain.NewCatalogBasisReference, released[0].Citation())
	own := scalar(t, domain.NewCatalogBasisReference, "tenant-1/network-change-0001")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		if err := catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
			Code: "node-a", Version: 1, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: effective, Basis: cited,
		}); err != nil {
			return err
		}
		if err := catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
			Code: "node-b", Version: 1, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: effective,
		}); err != nil {
			return err
		}
		if err := catalog.RegisterConnectionVersion(txCtx, tenant, ports.ConnectionDefinitionVersion{
			Code: "conn-a-b", Version: 1, FromNode: "node-a", ToNode: "node-b",
			BusinessTimezone: "Asia/Shanghai", EffectiveFrom: effective, Basis: cited,
		}); err != nil {
			return err
		}
		if err := catalog.RegisterLineVersion(txCtx, tenant, ports.LineDefinitionVersion{
			Code: "line-a-b", Version: 1, Segments: []string{"conn-a-b"},
			BusinessTimezone: "Asia/Shanghai", ApplicableScope: "purpose-export", EffectiveFrom: effective, Basis: own,
		}); err != nil {
			return err
		}
		if err := catalog.RegisterServiceAreaVersion(txCtx, tenant, ports.ServiceAreaDefinitionVersion{
			Code: "area-cn", Version: 1, EffectiveFrom: effective, HasCoverage: true, CoverageCountry: "CN",
			OriginNodes: []string{"node-a"}, Basis: cited,
		}); err != nil {
			return err
		}
		if err := catalog.RegisterServiceCalendarVersion(txCtx, tenant, ports.ServiceCalendarDefinitionVersion{
			TargetKind: ports.TargetNode, TargetCode: "node-a", Version: 1, EffectiveFrom: effective, Basis: cited,
		}); err != nil {
			return err
		}
		return catalog.RegisterRouteStrategyVersion(txCtx, tenant, ports.RouteStrategyDefinitionVersion{
			Code: "strategy-a", Version: 1, ApplicableScope: "purpose-export",
			RankingForm: domain.CostSingleDimensionRanking, EffectiveFrom: effective, Basis: own,
		})
	})

	nodes, err := catalog.ListNodeVersions(ctx, tenant, 10, firstPage(t, ports.NodeVersionCatalogue))
	if err != nil || len(nodes.Rows) != 2 {
		t.Fatalf("节点上列：err=%v rows=%+v", err, nodes.Rows)
	}
	if nodes.Rows[0].Basis != cited {
		t.Fatalf("node-a 依据 = %q，想要 %q", nodes.Rows[0].Basis, cited)
	}
	if nodes.Rows[1].Basis.Present() {
		t.Fatalf("node-b 没给依据，读回却是 %q", nodes.Rows[1].Basis)
	}
	connections, err := catalog.ListConnectionVersions(ctx, tenant, 10, firstPage(t, ports.ConnectionVersionCatalogue))
	if err != nil || len(connections.Rows) != 1 || connections.Rows[0].Basis != cited {
		t.Fatalf("连接上列：err=%v rows=%+v", err, connections.Rows)
	}
	lines, err := catalog.ListLineVersions(ctx, tenant, 10, firstPage(t, ports.LineVersionCatalogue))
	if err != nil || len(lines.Rows) != 1 || lines.Rows[0].Basis != own {
		t.Fatalf("线路上列：err=%v rows=%+v", err, lines.Rows)
	}
	areas, err := catalog.ListServiceAreaVersions(ctx, tenant, 10, firstPage(t, ports.ServiceAreaVersionCatalogue))
	if err != nil || len(areas.Rows) != 1 || areas.Rows[0].Basis != cited {
		t.Fatalf("服务区域上列：err=%v rows=%+v", err, areas.Rows)
	}
	calendars, err := catalog.ListServiceCalendarVersions(ctx, tenant, 10, firstPage(t, ports.ServiceCalendarVersionCatalogue))
	if err != nil || len(calendars.Rows) != 1 || calendars.Rows[0].Basis != cited {
		t.Fatalf("服务日历上列：err=%v rows=%+v", err, calendars.Rows)
	}
	strategies, err := catalog.ListRouteStrategyVersions(ctx, tenant, 10, firstPage(t, ports.RouteStrategyVersionCatalogue))
	if err != nil || len(strategies.Rows) != 1 || strategies.Rows[0].Basis != own {
		t.Fatalf("路由策略上列：err=%v rows=%+v", err, strategies.Rows)
	}
}
