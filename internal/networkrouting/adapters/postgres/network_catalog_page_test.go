package postgres_test

import (
	"context"
	"net/url"
	"reflect"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// pagedFamily 把一族的登记口与上列口包成同一形状，「翻页不重不漏」才能按族各跑一遍（票 03 完成判据）。
type pagedFamily struct {
	catalogue *cataloguepage.Catalogue
	// register 在 code 这个对象下登第 version 版，生效于 at；同一对象按版本号与时刻递增着登。
	register func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, code string, version int32, at time.Time) error
	// list 上列一页，交回各行的「代码/版本」键、下一游标与总数。
	list func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, limit int, query cataloguepage.Query) ([]string, string, int64, error)
}

func keyOf(code string, version int32) string {
	return code + "/" + cataloguepage.FormatInteger(int64(version))
}

func keysOf[Row any](page ports.CatalogPage[Row], err error, key func(Row) string) ([]string, string, int64, error) {
	keys := make([]string, 0, len(page.Rows))
	for _, row := range page.Rows {
		keys = append(keys, key(row))
	}
	return keys, page.Next, page.Total, err
}

func pagedFamilies() map[string]pagedFamily {
	return map[string]pagedFamily{
		"node": {
			catalogue: ports.NodeVersionCatalogue,
			register: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, code string, version int32, at time.Time) error {
				return catalog.RegisterNodeVersion(ctx, tenant, ports.NodeDefinitionVersion{
					Code: code, Version: version, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: at,
				})
			},
			list: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, limit int, query cataloguepage.Query) ([]string, string, int64, error) {
				page, err := catalog.ListNodeVersions(ctx, tenant, limit, query)
				return keysOf(page, err, func(row ports.NodeDefinitionVersion) string { return keyOf(row.Code, row.Version) })
			},
		},
		"connection": {
			catalogue: ports.ConnectionVersionCatalogue,
			register: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, code string, version int32, at time.Time) error {
				return catalog.RegisterConnectionVersion(ctx, tenant, ports.ConnectionDefinitionVersion{
					Code: code, Version: version, FromNode: "SYN-NODE-A", ToNode: "SYN-NODE-B",
					BusinessTimezone: "Asia/Shanghai", EffectiveFrom: at,
				})
			},
			list: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, limit int, query cataloguepage.Query) ([]string, string, int64, error) {
				page, err := catalog.ListConnectionVersions(ctx, tenant, limit, query)
				return keysOf(page, err, func(row ports.ConnectionDefinitionVersion) string { return keyOf(row.Code, row.Version) })
			},
		},
		"line": {
			catalogue: ports.LineVersionCatalogue,
			register: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, code string, version int32, at time.Time) error {
				return catalog.RegisterLineVersion(ctx, tenant, ports.LineDefinitionVersion{
					Code: code, Version: version, Segments: []string{"SYN-CONN-AB"},
					BusinessTimezone: "Asia/Shanghai", ApplicableScope: "SYN-SCOPE", EffectiveFrom: at,
				})
			},
			list: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, limit int, query cataloguepage.Query) ([]string, string, int64, error) {
				page, err := catalog.ListLineVersions(ctx, tenant, limit, query)
				return keysOf(page, err, func(row ports.LineDefinitionVersion) string { return keyOf(row.Code, row.Version) })
			},
		},
		"service-area": {
			catalogue: ports.ServiceAreaVersionCatalogue,
			register: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, code string, version int32, at time.Time) error {
				return catalog.RegisterServiceAreaVersion(ctx, tenant, ports.ServiceAreaDefinitionVersion{
					Code: code, Version: version, EffectiveFrom: at,
				})
			},
			list: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, limit int, query cataloguepage.Query) ([]string, string, int64, error) {
				page, err := catalog.ListServiceAreaVersions(ctx, tenant, limit, query)
				return keysOf(page, err, func(row ports.ServiceAreaDefinitionVersion) string { return keyOf(row.Code, row.Version) })
			},
		},
		"service-calendar": {
			catalogue: ports.ServiceCalendarVersionCatalogue,
			register: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, code string, version int32, at time.Time) error {
				return catalog.RegisterServiceCalendarVersion(ctx, tenant, ports.ServiceCalendarDefinitionVersion{
					TargetKind: ports.TargetNode, TargetCode: code, Version: version, EffectiveFrom: at,
				})
			},
			list: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, limit int, query cataloguepage.Query) ([]string, string, int64, error) {
				page, err := catalog.ListServiceCalendarVersions(ctx, tenant, limit, query)
				return keysOf(page, err, func(row ports.ServiceCalendarDefinitionVersion) string {
					return keyOf(row.TargetCode, row.Version)
				})
			},
		},
		"availability-adjustment": {
			catalogue: ports.AvailabilityAdjustmentCatalogue,
			register: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, code string, version int32, at time.Time) error {
				statement := ports.AvailabilityAdjustmentStatement{
					Code: code, Version: version, TargetKind: ports.TargetLine, TargetCode: "SYN-LINE",
					Kind: ports.AdjustmentSuspension, Source: "SYN-SOURCE", EffectiveAt: at,
				}
				if version > 1 {
					statement.LiftedAt, statement.HasLiftedAt = at.Add(time.Hour), true
				}
				return catalog.RegisterAvailabilityAdjustment(ctx, tenant, statement)
			},
			list: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, limit int, query cataloguepage.Query) ([]string, string, int64, error) {
				page, err := catalog.ListAvailabilityAdjustments(ctx, tenant, limit, query)
				return keysOf(page, err, func(row ports.AvailabilityAdjustmentStatement) string { return keyOf(row.Code, row.Version) })
			},
		},
		"route-strategy": {
			catalogue: ports.RouteStrategyVersionCatalogue,
			register: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, code string, version int32, at time.Time) error {
				return catalog.RegisterRouteStrategyVersion(ctx, tenant, ports.RouteStrategyDefinitionVersion{
					Code: code, Version: version, ApplicableScope: "SYN-SCOPE", EffectiveFrom: at,
				})
			},
			list: func(ctx context.Context, catalog *adapter.NetworkCatalog, tenant domain.TenantID, limit int, query cataloguepage.Query) ([]string, string, int64, error) {
				page, err := catalog.ListRouteStrategyVersions(ctx, tenant, limit, query)
				return keysOf(page, err, func(row ports.RouteStrategyDefinitionVersion) string { return keyOf(row.Code, row.Version) })
			},
		},
	}
}

func decodeWith(t *testing.T, catalogue *cataloguepage.Catalogue, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := catalogue.Decode(values)
	if err != nil {
		t.Fatalf("解码 %v：%v", values, err)
	}
	return query
}

// Covers: 票 03 完成判据「按每个 family 各跑一遍翻页不重不漏」（ADR-0144 决定一）——缺省序下两行一页地翻，
// 翻完第一页后登两行：一行排在已翻过的区间，一行排在未翻的区间。已翻过的页不重出、未翻的页不漏行，
// 新登的那行在后面的页里出现；total 随登记变化，末页 next 为空。
func TestEachFamilyPagesWithoutRepeatsOrGapsWhileRowsAreRegistered(t *testing.T) {
	for name, family := range pagedFamilies() {
		t.Run(name, func(t *testing.T) {
			catalog, transactor, _ := newNetworkCatalog(t)
			ctx := t.Context()
			tenant := scalar(t, domain.NewTenantID, "SYN-TENANT-01")
			register := func(code string, version int32, at time.Time) {
				t.Helper()
				within(t, transactor, ctx, func(txCtx context.Context) error {
					return family.register(txCtx, catalog, tenant, code, version, at)
				})
			}
			list := func(query cataloguepage.Query) ([]string, string, int64) {
				t.Helper()
				keys, next, total, err := family.list(ctx, catalog, tenant, 2, query)
				if err != nil {
					t.Fatalf("上列：%v", err)
				}
				return keys, next, total
			}

			register("SYN-A", 1, catalogAsOf)
			register("SYN-A", 2, catalogAsOf.Add(time.Hour))
			register("SYN-B", 1, catalogAsOf.Add(2*time.Hour))
			register("SYN-C", 1, catalogAsOf.Add(3*time.Hour))
			register("SYN-C", 2, catalogAsOf.Add(4*time.Hour))

			collected, next, total := list(firstPage(t, family.catalogue))
			if !reflect.DeepEqual(collected, []string{"SYN-A/1", "SYN-A/2"}) || next == "" || total != 5 {
				t.Fatalf("第一页：%v next=%q total=%d", collected, next, total)
			}

			register("SYN-0", 1, catalogAsOf.Add(5*time.Hour))
			register("SYN-D", 1, catalogAsOf.Add(6*time.Hour))

			for pages := 1; next != ""; pages++ {
				if pages > 10 {
					t.Fatal("翻页停不下来")
				}
				var keys []string
				keys, next, total = list(decodeWith(t, family.catalogue, url.Values{"after": {next}}))
				if total != 7 {
					t.Fatalf("登记之后 total 应为 7，得 %d", total)
				}
				collected = append(collected, keys...)
			}
			want := []string{"SYN-A/1", "SYN-A/2", "SYN-B/1", "SYN-C/1", "SYN-C/2", "SYN-D/1"}
			if !reflect.DeepEqual(collected, want) {
				t.Fatalf("翻完得 %v，应为 %v", collected, want)
			}
		})
	}
}

// Covers: ADR-0144 决定三——非缺省的时刻维倒序翻页：同一时刻的两行按标识同向（倒序）决胜，游标里的时刻
// 往返不失精度，页与页之间不重不漏。
func TestPagingByAnInstantDimensionDescendingBreaksTiesByIdentity(t *testing.T) {
	family := pagedFamilies()["node"]
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "SYN-TENANT-01")
	for code, offset := range map[string]time.Duration{
		"SYN-A": 0, "SYN-B": 2 * time.Hour, "SYN-C": time.Hour, "SYN-D": 3 * time.Hour, "SYN-E": 2 * time.Hour,
	} {
		within(t, transactor, ctx, func(txCtx context.Context) error {
			return family.register(txCtx, catalog, tenant, code, 1, catalogAsOf.Add(offset+123456*time.Microsecond))
		})
	}

	query := decodeWith(t, family.catalogue, url.Values{"sort": {"-effectiveFrom"}})
	var collected []string
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("翻页停不下来")
		}
		keys, next, total, err := family.list(ctx, catalog, tenant, 2, query)
		if err != nil {
			t.Fatalf("上列：%v", err)
		}
		if total != 5 {
			t.Fatalf("total 应为 5，得 %d", total)
		}
		collected = append(collected, keys...)
		if next == "" {
			break
		}
		query = decodeWith(t, family.catalogue, url.Values{"sort": {"-effectiveFrom"}, "after": {next}})
	}
	want := []string{"SYN-D/1", "SYN-E/1", "SYN-B/1", "SYN-C/1", "SYN-A/1"}
	if !reflect.DeepEqual(collected, want) {
		t.Fatalf("按生效时刻倒序翻完得 %v，应为 %v", collected, want)
	}
}

// Covers: ADR-0144 决定四、五——筛选维内为或、维间为与，q 在本族点名的几列上做不分大小写的字面包含
// 匹配（LIKE 的通配符按字面算），total 与本页出自同一组条件。
func TestFiltersAndKeywordNarrowTheRowsAndTheTotal(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "SYN-TENANT-01")
	for _, connection := range []ports.ConnectionDefinitionVersion{
		{Code: "SYN-C1", FromNode: "SYN-N1", ToNode: "SYN-N2"},
		{Code: "SYN-C2", FromNode: "SYN-N1", ToNode: "SYN-N3"},
		{Code: "SYN-C3", FromNode: "SYN-N2", ToNode: "SYN-N3"},
		{Code: "SYN_C5", FromNode: "SYN-N4", ToNode: "SYN-N5"},
	} {
		connection.Version, connection.BusinessTimezone, connection.EffectiveFrom = 1, "Asia/Shanghai", catalogAsOf
		within(t, transactor, ctx, func(txCtx context.Context) error {
			return catalog.RegisterConnectionVersion(txCtx, tenant, connection)
		})
	}

	cases := map[string]struct {
		values url.Values
		want   []string
	}{
		"不筛":         {url.Values{}, []string{"SYN-C1/1", "SYN-C2/1", "SYN-C3/1", "SYN_C5/1"}},
		"一维一值":       {url.Values{"fromNode": {"SYN-N1"}}, []string{"SYN-C1/1", "SYN-C2/1"}},
		"维内为或":       {url.Values{"fromNode": {"SYN-N1", "SYN-N2"}}, []string{"SYN-C1/1", "SYN-C2/1", "SYN-C3/1"}},
		"维间为与":       {url.Values{"fromNode": {"SYN-N1"}, "toNode": {"SYN-N3"}}, []string{"SYN-C2/1"}},
		"q 不分大小写":    {url.Values{"q": {"c3"}}, []string{"SYN-C3/1"}},
		"q 覆盖端点列":    {url.Values{"q": {"n3"}}, []string{"SYN-C2/1", "SYN-C3/1"}},
		"q 的下划线按字面算": {url.Values{"q": {"N_C"}}, []string{"SYN_C5/1"}},
		"q 与筛选同时":    {url.Values{"q": {"syn"}, "toNode": {"SYN-N2"}}, []string{"SYN-C1/1"}},
	}
	family := pagedFamilies()["connection"]
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			keys, next, total, err := family.list(ctx, catalog, tenant, 10, decodeWith(t, family.catalogue, test.values))
			if err != nil {
				t.Fatalf("上列：%v", err)
			}
			if !reflect.DeepEqual(keys, test.want) || next != "" || total != int64(len(test.want)) {
				t.Fatalf("得 %v total=%d next=%q，应为 %v", keys, total, next, test.want)
			}
		})
	}
}
