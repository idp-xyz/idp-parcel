package postgres_test

import (
	"context"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// Covers: AT-NR-111、AT-NR-112（真库半边）——名称随版本登记：改名是登一个更晚生效的新版本，前版照留没登名称那一格并按
// 新版生效时间接续闭合；上列逐版带各自的名称，检索词也匹配名称。同一版本号再登（带不带名称都一样）照旧由主键挡，
// 先登的那一行一格不动。选版读口不取名称：两个租户登同一份网络、只差名称，判断时点的快照逐格相同。
func TestNodeDisplayNamesAreRegisteredPerVersionAndStayOutOfTheSnapshot(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	named := scalar(t, domain.NewTenantID, "tenant-named")
	plain := scalar(t, domain.NewTenantID, "tenant-plain")
	name := scalar(t, domain.NewNodeDisplayName, "上海枢纽")
	renamedAt := catalogAsOf.Add(-time.Hour)

	for _, tenant := range []domain.TenantID{named, plain} {
		within(t, transactor, ctx, func(txCtx context.Context) error {
			return catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
				Code: "node-hub", Version: 1, BusinessTimezone: "Asia/Shanghai",
				EffectiveFrom: catalogAsOf.Add(-48 * time.Hour),
			})
		})
	}
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterNodeVersion(txCtx, named, ports.NodeDefinitionVersion{
			Code: "node-hub", Version: 2, BusinessTimezone: "Asia/Shanghai",
			EffectiveFrom: renamedAt, DisplayName: name,
		})
	})
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterNodeVersion(txCtx, plain, ports.NodeDefinitionVersion{
			Code: "node-hub", Version: 2, BusinessTimezone: "Asia/Shanghai",
			EffectiveFrom: renamedAt,
		})
	})

	page, err := catalog.ListNodeVersions(ctx, named, 10, firstPage(t, ports.NodeVersionCatalogue))
	if err != nil {
		t.Fatalf("上列节点版本：%v", err)
	}
	if len(page.Rows) != 2 {
		t.Fatalf("应列两版，实得 %+v", page.Rows)
	}
	before, after := page.Rows[0], page.Rows[1]
	if before.Version != 1 || before.DisplayName.Registered() || !before.HasEffectiveTo || !before.EffectiveTo.Equal(renamedAt) {
		t.Fatalf("前版应没登名称、按新版生效时间接续闭合，实得 %+v", before)
	}
	if after.Version != 2 || after.DisplayName != name || after.HasEffectiveTo {
		t.Fatalf("新版应带名称、未闭，实得 %+v", after)
	}

	searched := func(keyword string) []int32 {
		t.Helper()
		query, err := ports.NodeVersionCatalogue.Decode(url.Values{"q": {keyword}})
		if err != nil {
			t.Fatalf("解出检索 %q：%v", keyword, err)
		}
		page, err := catalog.ListNodeVersions(ctx, named, 10, query)
		if err != nil {
			t.Fatalf("检索 %q：%v", keyword, err)
		}
		versions := make([]int32, 0, len(page.Rows))
		for _, row := range page.Rows {
			versions = append(versions, row.Version)
		}
		if page.Total != int64(len(versions)) {
			t.Fatalf("检索 %q：total=%d 与本页 %d 行不一致", keyword, page.Total, len(versions))
		}
		return versions
	}
	if got := searched("枢纽"); !reflect.DeepEqual(got, []int32{2}) {
		t.Fatalf("按名称检索应只中登了名称的那一版，实得 %v", got)
	}
	if got := searched("NODE-HUB"); !reflect.DeepEqual(got, []int32{1, 2}) {
		t.Fatalf("按代码检索照旧两版都中，实得 %v", got)
	}

	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return catalog.RegisterNodeVersion(txCtx, plain, ports.NodeDefinitionVersion{
			Code: "node-hub", Version: 1, BusinessTimezone: "Asia/Shanghai",
			EffectiveFrom: catalogAsOf.Add(-48 * time.Hour), EffectiveTo: renamedAt, HasEffectiveTo: true,
			DisplayName: name,
		})
	})
	if err == nil {
		t.Fatal("同一版本号带上名称再登，应照旧由主键挡下")
	}
	plainPage, err := catalog.ListNodeVersions(ctx, plain, 10, firstPage(t, ports.NodeVersionCatalogue))
	if err != nil || len(plainPage.Rows) != 2 || plainPage.Rows[0].DisplayName.Registered() {
		t.Fatalf("撞主键之后先登的那一行应一格不动：err=%v rows=%+v", err, plainPage.Rows)
	}

	namedSnapshot, configured, err := catalog.LoadDefinitionsAt(ctx, named, catalogAsOf)
	if err != nil || !configured {
		t.Fatalf("读带名称租户的目录：configured=%v err=%v", configured, err)
	}
	plainSnapshot, configured, err := catalog.LoadDefinitionsAt(ctx, plain, catalogAsOf)
	if err != nil || !configured {
		t.Fatalf("读不带名称租户的目录：configured=%v err=%v", configured, err)
	}
	namedSnapshot.Revision, plainSnapshot.Revision = domain.NetworkViewRevision{}, domain.NetworkViewRevision{}
	if !reflect.DeepEqual(namedSnapshot, plainSnapshot) {
		t.Fatalf("只差名称的两份网络，选版快照应逐格相同：\n带名称 %+v\n不带   %+v", namedSnapshot, plainSnapshot)
	}
	if len(namedSnapshot.Nodes) != 1 || namedSnapshot.Nodes[0].DisplayName.Registered() {
		t.Fatalf("选版读口不取名称，实得 %+v", namedSnapshot.Nodes)
	}
}

// Covers: AT-NR-112（迁移半边）——迁移 0033 施加在已有节点版本的库上：不回填、不改写，旧行的名称一格为 NULL（读成没登），
// 其余各列逐格不变；之后库上 CHECK 作为第二道网拒空白、首尾空白、超长与控制字符，收下合格的名称。
func TestMigration0033LeavesExistingNodeVersionsUnnamedAndUnchanged(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA network_routing`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.NetworkRouting()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0033_logistics_node_version_display_name"
	var pending []migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			pending = assets[index:]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if len(pending) == 0 {
		t.Fatalf("没有找到迁移 %s", target)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO network_routing.logistics_node_version
		(tenant_id, node_code, version, business_timezone, effective_from, effective_to, basis_ref)
		VALUES ('SYN-TENANT-NR33', 'SYN-NODE-NR33', 1, 'Asia/Shanghai', '2026-01-01T00:00:00Z', '2026-06-01T00:00:00Z', 'SYN-NET-OPS/NR33')`); err != nil {
		t.Fatalf("落 0033 之前的节点版本：%v", err)
	}
	type nodeRow struct {
		timezone, basis string
		from, to        time.Time
	}
	readRow := func() nodeRow {
		var row nodeRow
		if err := conn.QueryRow(ctx, `SELECT business_timezone, basis_ref, effective_from, effective_to
			FROM network_routing.logistics_node_version WHERE node_code = 'SYN-NODE-NR33'`).
			Scan(&row.timezone, &row.basis, &row.from, &row.to); err != nil {
			t.Fatalf("读节点版本：%v", err)
		}
		return row
	}
	before := readRow()
	for _, asset := range pending {
		if _, err := conn.Exec(ctx, asset.SQL); err != nil {
			t.Fatalf("在有数据的库上施加 %s：%v", asset.Name, err)
		}
	}
	if after := readRow(); after != before {
		t.Fatalf("迁移改写了旧行：之前 %+v，之后 %+v", before, after)
	}
	var displayName *string
	if err := conn.QueryRow(ctx, `SELECT display_name FROM network_routing.logistics_node_version
		WHERE node_code = 'SYN-NODE-NR33'`).Scan(&displayName); err != nil || displayName != nil {
		t.Fatalf("旧行的名称应为 NULL：%v %v", displayName, err)
	}

	insert := func(version int, name string) error {
		_, err := conn.Exec(ctx, `INSERT INTO network_routing.logistics_node_version
			(tenant_id, node_code, version, business_timezone, effective_from, display_name)
			VALUES ('SYN-TENANT-NR33', 'SYN-NODE-NR33-CHECK', $1, 'Asia/Shanghai', '2026-01-01T00:00:00Z', $2)`, version, name)
		return err
	}
	long := ""
	for range 101 {
		long += "枢"
	}
	for version, bad := range map[int]string{11: "", 12: "  ", 13: " 上海枢纽", 14: "上海枢纽\t", 15: "上海\n枢纽", 16: long} {
		if err := insert(version, bad); err == nil {
			t.Fatalf("库上 CHECK 应拒名称 %q", bad)
		}
	}
	if err := insert(1, long[:len(long)-len("枢")]); err != nil {
		t.Fatalf("恰 100 个字符的名称应收下：%v", err)
	}
}
