package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 口岸目录与申报路径目录列表读面的用例（ADR-0077 Decision 四/五那三句在本册的样子）。

func newPortsPathsCatalogue(t *testing.T) (*adapter.PortsPathsCatalogue, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	catalogue, err := adapter.NewPortsPathsCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造口岸路径册列表读口：%v", err)
	}
	return catalogue, fixture
}

// Covers: 空册如实答空列表，不是错误也不折成未配置——两册各自成立。
func TestEmptyPortsPathsRegistersAnswerEmptyLists(t *testing.T) {
	catalogue, _ := newPortsPathsCatalogue(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	if entries, err := catalogue.ListCandidatePorts(t.Context(), tenant, 10); err != nil || len(entries) != 0 {
		t.Fatalf("空口岸册上列走样：err=%v entries=%+v", err, entries)
	}
	if entries, err := catalogue.ListDeclarationPaths(t.Context(), tenant, 10); err != nil || len(entries) != 0 {
		t.Fatalf("空路径册上列走样：err=%v entries=%+v", err, entries)
	}
}

// Covers: 全部版本连同区间原样上列（开放版终点零值透出），键升序、同键内起点倒序；
// 跨租户不可见。区间判读留给读者——历史版本不被过滤掉。
func TestCandidatePortCatalogueListsAllVersionsInKeyOrder(t *testing.T) {
	catalogue, fixture := newPortsPathsCatalogue(t)
	successionAt := viewBaseAt.Add(48 * time.Hour)
	fixture.seed(t,
		`INSERT INTO customs_compliance.candidate_port (tenant_id, port_ref, applies_from, applies_until)
		 VALUES
			('tenant-a', 'SYN-PORT-02', $1, NULL),
			('tenant-a', 'SYN-PORT-01', $1, $2),
			('tenant-a', 'SYN-PORT-01', $2, NULL),
			('tenant-b', 'SYN-PORT-09', $1, NULL)`,
		viewBaseAt, successionAt)

	entries, err := catalogue.ListCandidatePorts(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil {
		t.Fatalf("上列口岸目录：%v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("上列了 %d 行，要 3 行：%+v", len(entries), entries)
	}
	// SYN-PORT-01 的两版在前（键升序），版本内起点倒序：当前开放版先于历史版。
	if entries[0].Port.String() != "SYN-PORT-01" || !entries[0].AppliesFrom.Equal(successionAt) ||
		!entries[0].AppliesUntil.IsZero() {
		t.Fatalf("首行不是 SYN-PORT-01 当前版：%+v", entries[0])
	}
	if entries[1].Port.String() != "SYN-PORT-01" || !entries[1].AppliesFrom.Equal(viewBaseAt) ||
		!entries[1].AppliesUntil.Equal(successionAt) {
		t.Fatalf("次行不是 SYN-PORT-01 历史版（区间原样）：%+v", entries[1])
	}
	if entries[2].Port.String() != "SYN-PORT-02" {
		t.Fatalf("末行不是 SYN-PORT-02：%+v", entries[2])
	}
	for _, entry := range entries {
		if entry.Port.String() == "SYN-PORT-09" {
			t.Fatal("跨租户可见")
		}
	}
}

// Covers: 路径册上列同款——三维逐格如实、键升序、版本倒序、跨租户不可见。
func TestDeclarationPathCatalogueListsRoutesFaithfully(t *testing.T) {
	catalogue, fixture := newPortsPathsCatalogue(t)
	successionAt := viewBaseAt.Add(48 * time.Hour)
	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_path
			(tenant_id, path_ref, applies_from, applies_until, port_ref, direction, declaration_mode)
		 VALUES
			('tenant-a', 'SYN-PATH-02', $1, NULL, 'SYN-PORT-02', 'EXPORT', 'SYN-MODE-SIMPLIFIED'),
			('tenant-a', 'SYN-PATH-01', $1, $2,   'SYN-PORT-01', 'IMPORT', 'SYN-MODE-GENERAL'),
			('tenant-a', 'SYN-PATH-01', $2, NULL, 'SYN-PORT-02', 'IMPORT', 'SYN-MODE-GENERAL'),
			('tenant-b', 'SYN-PATH-09', $1, NULL, 'SYN-PORT-09', 'IMPORT', 'SYN-MODE-GENERAL')`,
		viewBaseAt, successionAt)

	entries, err := catalogue.ListDeclarationPaths(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil {
		t.Fatalf("上列路径目录：%v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("上列了 %d 行，要 3 行：%+v", len(entries), entries)
	}
	current := entries[0]
	if current.Path.String() != "SYN-PATH-01" || !current.AppliesFrom.Equal(successionAt) ||
		current.Route.Port().String() != "SYN-PORT-02" {
		t.Fatalf("首行不是 SYN-PATH-01 当前版：%+v", current)
	}
	historical := entries[1]
	if historical.Path.String() != "SYN-PATH-01" || !historical.AppliesUntil.Equal(successionAt) ||
		historical.Route.Port().String() != "SYN-PORT-01" ||
		historical.Route.Direction() != domain.ImportManifest ||
		historical.Route.Mode().String() != "SYN-MODE-GENERAL" {
		t.Fatalf("次行三维或区间走样：%+v", historical)
	}
	other := entries[2]
	if other.Path.String() != "SYN-PATH-02" || other.Route.Direction() != domain.ExportManifest ||
		other.Route.Mode().String() != "SYN-MODE-SIMPLIFIED" {
		t.Fatalf("末行三维走样：%+v", other)
	}
	for _, entry := range entries {
		if entry.Path.String() == "SYN-PATH-09" {
			t.Fatal("跨租户可见")
		}
	}
}

// Covers: ADR-0077 Decision 五 — limit 非正拒；limit 截断行数而不是静默全量。
func TestPortsPathsCatalogueGuardsItsLimit(t *testing.T) {
	catalogue, fixture := newPortsPathsCatalogue(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	if _, err := catalogue.ListCandidatePorts(t.Context(), tenant, 0); err == nil {
		t.Fatal("limit=0 的口岸上列被接受了")
	}
	if _, err := catalogue.ListDeclarationPaths(t.Context(), tenant, -1); err == nil {
		t.Fatal("limit=-1 的路径上列被接受了")
	}

	fixture.seed(t,
		`INSERT INTO customs_compliance.candidate_port (tenant_id, port_ref, applies_from)
		 VALUES
			('tenant-a', 'SYN-PORT-01', $1),
			('tenant-a', 'SYN-PORT-02', $1),
			('tenant-a', 'SYN-PORT-03', $1)`,
		viewBaseAt)
	entries, err := catalogue.ListCandidatePorts(t.Context(), tenant, 2)
	if err != nil || len(entries) != 2 {
		t.Fatalf("limit=2 却上列了 %d 行：err=%v", len(entries), err)
	}
}
