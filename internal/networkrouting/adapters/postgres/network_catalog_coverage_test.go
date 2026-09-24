package postgres_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// Covers: 迁移 0011 的覆盖四列（ADR-0148 决定二、五）——覆盖国家、邮编前缀与两组节点角色随区域版本落库，
// 选版快照原样读回；没登覆盖的版本读回仍是「没登覆盖」，不被读成一份空覆盖。
func TestCatalogServiceAreaCoverageRoundTrips(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "SYN-TENANT-1")
	effective := catalogAsOf.Add(-time.Hour)

	covered := ports.ServiceAreaDefinitionVersion{
		Code: "SYN-AREA-XA-10", Version: 1, EffectiveFrom: effective,
		HasCoverage: true, CoverageCountry: "XA", PostalPrefixes: []string{"10", "11"},
		OriginNodes: []string{"SYN-NODE-A"}, DestinationNodes: []string{"SYN-NODE-B", "SYN-NODE-C"},
	}
	bare := ports.ServiceAreaDefinitionVersion{Code: "SYN-AREA-BARE", Version: 1, EffectiveFrom: effective}
	within(t, transactor, ctx, func(txCtx context.Context) error {
		if err := catalog.RegisterServiceAreaVersion(txCtx, tenant, covered); err != nil {
			return err
		}
		return catalog.RegisterServiceAreaVersion(txCtx, tenant, bare)
	})

	snapshot, configured, err := catalog.LoadDefinitionsAt(ctx, tenant, catalogAsOf)
	if err != nil || !configured {
		t.Fatalf("读目录：configured=%v err=%v", configured, err)
	}
	byCode := map[string]ports.ServiceAreaDefinitionVersion{}
	for _, area := range snapshot.ServiceAreas {
		area.EffectiveFrom = area.EffectiveFrom.UTC()
		byCode[area.Code] = area
	}
	if got := byCode["SYN-AREA-XA-10"]; !reflect.DeepEqual(got, covered) {
		t.Fatalf("覆盖版本读回 %+v，想要 %+v", got, covered)
	}
	if got := byCode["SYN-AREA-BARE"]; !reflect.DeepEqual(got, bare) {
		t.Fatalf("没登覆盖的版本读回 %+v，想要原样 %+v", got, bare)
	}
}

// Covers: 运营查阅上列按族列版本行原文（ADR-0077）——覆盖四列随行列出，没登覆盖的版本列出来仍是没登覆盖。
func TestCatalogueListShowsServiceAreaCoverage(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "SYN-TENANT-1")
	effective := catalogAsOf.Add(-time.Hour)

	within(t, transactor, ctx, func(txCtx context.Context) error {
		if err := catalog.RegisterServiceAreaVersion(txCtx, tenant, ports.ServiceAreaDefinitionVersion{
			Code: "SYN-AREA-XB", Version: 1, EffectiveFrom: effective,
			HasCoverage: true, CoverageCountry: "XB", DestinationNodes: []string{"SYN-NODE-LM"},
		}); err != nil {
			return err
		}
		return catalog.RegisterServiceAreaVersion(txCtx, tenant, ports.ServiceAreaDefinitionVersion{
			Code: "SYN-AREA-BARE", Version: 1, EffectiveFrom: effective,
		})
	})

	page, err := catalog.ListServiceAreaVersions(ctx, tenant, 10, firstPage(t, ports.ServiceAreaVersionCatalogue))
	if err != nil {
		t.Fatalf("列服务区域族：%v", err)
	}
	byCode := map[string]ports.ServiceAreaDefinitionVersion{}
	for _, row := range page.Rows {
		byCode[row.Code] = row
	}
	listed := byCode["SYN-AREA-XB"]
	if !listed.HasCoverage || listed.CoverageCountry != "XB" || len(listed.PostalPrefixes) != 0 ||
		len(listed.OriginNodes) != 0 || !reflect.DeepEqual(listed.DestinationNodes, []string{"SYN-NODE-LM"}) {
		t.Fatalf("列出的覆盖走样：%+v", listed)
	}
	if bare := byCode["SYN-AREA-BARE"]; bare.HasCoverage || bare.CoverageCountry != "" {
		t.Fatalf("没登覆盖的版本列出来长出了覆盖：%+v", bare)
	}
}

// Covers: 0011 的 CHECK 只收单看一行就判得出的形状——前缀与节点角色不脱离覆盖国家单独出现，国家码两位大写。
// 登记用例是第一道门，这里证库上那道网本身不放行。
func TestCatalogServiceAreaCoverageShapeIsCheckedInTheDatabase(t *testing.T) {
	_, _, pool := newNetworkCatalog(t)
	ctx := t.Context()

	for name, columns := range map[string]string{
		"前缀缺覆盖国家":   `NULL, '["10"]'::jsonb, NULL, NULL`,
		"节点角色缺覆盖国家": `NULL, NULL, '["SYN-NODE-A"]'::jsonb, NULL`,
		"国家码小写":     `'xa', NULL, NULL, NULL`,
		"前缀是空数组":    `'XA', '[]'::jsonb, NULL, NULL`,
	} {
		_, err := pool.Exec(ctx, `INSERT INTO network_routing.service_area_version
			(tenant_id, area_code, version, effective_from,
			 coverage_country, coverage_postal_prefixes, origin_node_codes, destination_node_codes)
			VALUES ('SYN-TENANT-1', 'SYN-AREA-PROBE', 1, now(), `+columns+`)`)
		if err == nil {
			t.Fatalf("%s：坏形状被库放行了", name)
		}
	}
}
