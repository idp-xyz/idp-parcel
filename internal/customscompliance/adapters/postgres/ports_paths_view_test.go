package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 口岸目录与申报路径目录点读口的语义用例。播种走 pool.Exec 直插（viewFixture 头注的
// 两条理由），与写口往返用例（ports_paths_registry_test.go）互为独立证据。

func newPortsPathsView(t *testing.T) (*adapter.PortsPathsPointView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	view, err := adapter.NewPortsPathsPointView(fixture.db)
	if err != nil {
		t.Fatalf("构造口岸路径读口：%v", err)
	}
	return view, fixture
}

func viewLoadPort(
	t *testing.T,
	view *adapter.PortsPathsPointView,
	tenant string,
	evaluatedAt time.Time,
) (ports.CandidatePortEntry, bool, error) {
	t.Helper()
	return view.LoadCandidatePort(t.Context(),
		viewValue(t, domain.NewTenantID, tenant),
		viewValue(t, domain.NewCustomsPortReference, "SYN-PORT-01"), evaluatedAt)
}

// Covers: 未登记即 found=false——实例半边未提供时停在未决，不拿开放版或当前时间兜底。
func TestPortsPathsAreUnconfiguredWhenNothingIsRegistered(t *testing.T) {
	view, _ := newPortsPathsView(t)
	if _, found, err := viewLoadPort(t, view, "tenant-a", viewBaseAt); err != nil || found {
		t.Fatalf("没登记却答出了口岸：err=%v found=%v", err, found)
	}
	if _, found, err := view.LoadDeclarationPath(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewDeclarationPathReference, "SYN-PATH-01"), viewBaseAt); err != nil || found {
		t.Fatalf("没登记却答出了路径：err=%v found=%v", err, found)
	}
}

// Covers: 半开区间解析——起点之前无版本、闭区间内取本版、终点起归后继、开放尾段答
// 当前版。终点等于评估时点的版本已不再适用（与解释规则读口同一口径）。
func TestCandidatePortResolvesByHalfOpenValidity(t *testing.T) {
	view, fixture := newPortsPathsView(t)
	successionAt := viewBaseAt.Add(48 * time.Hour)
	fixture.seed(t,
		`INSERT INTO customs_compliance.candidate_port
			(tenant_id, port_ref, applies_from, applies_until)
		 VALUES ('tenant-a', 'SYN-PORT-01', $1, $2)`,
		viewBaseAt, successionAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.candidate_port
			(tenant_id, port_ref, applies_from)
		 VALUES ('tenant-a', 'SYN-PORT-01', $1)`,
		successionAt)

	if _, found, err := viewLoadPort(t, view, "tenant-a", viewBaseAt.Add(-time.Second)); err != nil || found {
		t.Fatalf("首版起点之前竟然有版本可答：err=%v found=%v", err, found)
	}
	early, foundEarly, err := viewLoadPort(t, view, "tenant-a", viewBaseAt.Add(time.Hour))
	if err != nil || !foundEarly || !early.AppliesUntil.Equal(successionAt) {
		t.Fatalf("旧区间时点没解析回前版：err=%v found=%v entry=%+v", err, foundEarly, early)
	}
	atBoundary, foundBoundary, err := viewLoadPort(t, view, "tenant-a", successionAt)
	if err != nil || !foundBoundary || !atBoundary.AppliesFrom.Equal(successionAt) {
		t.Fatalf("换版边界该归后继（半开区间）：err=%v found=%v entry=%+v", err, foundBoundary, atBoundary)
	}
	tail, foundTail, err := viewLoadPort(t, view, "tenant-a", successionAt.Add(365*24*time.Hour))
	if err != nil || !foundTail || !tail.AppliesUntil.IsZero() {
		t.Fatalf("开放尾段没答当前开放版：err=%v found=%v entry=%+v", err, foundTail, tail)
	}
}

// Covers: 路径三维从行里逐格重建；跨租户不可见（目录是每租户自己的册子）。
func TestDeclarationPathRebuildsItsRouteAndStaysTenantScoped(t *testing.T) {
	view, fixture := newPortsPathsView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_path
			(tenant_id, path_ref, applies_from, port_ref, direction, declaration_mode)
		 VALUES ('tenant-a', 'SYN-PATH-01', $1, 'SYN-PORT-01', 'EXPORT', 'SYN-MODE-GENERAL')`,
		viewBaseAt)

	entry, found, err := view.LoadDeclarationPath(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewDeclarationPathReference, "SYN-PATH-01"), viewBaseAt.Add(time.Hour))
	if err != nil || !found {
		t.Fatalf("登记后读不回：err=%v found=%v", err, found)
	}
	route := entry.Route
	if route.Port().String() != "SYN-PORT-01" || route.Direction() != domain.ExportManifest ||
		route.Mode().String() != "SYN-MODE-GENERAL" {
		t.Fatalf("路径三维重建走样：%+v", route)
	}

	if _, found, err := view.LoadDeclarationPath(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-b"),
		viewValue(t, domain.NewDeclarationPathReference, "SYN-PATH-01"), viewBaseAt.Add(time.Hour)); err != nil || found {
		t.Fatalf("跨租户可见：err=%v found=%v", err, found)
	}
	if _, found, err := viewLoadPort(t, view, "tenant-b", viewBaseAt.Add(time.Hour)); err != nil || found {
		t.Fatalf("口岸册跨租户可见：err=%v found=%v", err, found)
	}
}

// Covers: 空键与零评估时点不静默答「未配置」——那是编程错误，与等实例参数两回事。
func TestPortsPathsBadInputsAreLoudRatherThanUnconfigured(t *testing.T) {
	view, _ := newPortsPathsView(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	if _, _, err := view.LoadCandidatePort(t.Context(),
		tenant, domain.CustomsPortReference{}, viewBaseAt); err == nil {
		t.Fatal("空口岸键被当成了未配置")
	}
	if _, _, err := view.LoadCandidatePort(t.Context(),
		tenant, viewValue(t, domain.NewCustomsPortReference, "SYN-PORT-01"), time.Time{}); err == nil {
		t.Fatal("零评估时点被当成了未配置")
	}
	if _, _, err := view.LoadDeclarationPath(t.Context(),
		tenant, domain.DeclarationPathReference{}, viewBaseAt); err == nil {
		t.Fatal("空路径键被当成了未配置")
	}
	if _, _, err := view.LoadDeclarationPath(t.Context(),
		tenant, viewValue(t, domain.NewDeclarationPathReference, "SYN-PATH-01"), time.Time{}); err == nil {
		t.Fatal("路径零评估时点被当成了未配置")
	}
}

// Covers: 0013 的约束在旁路写入下也守着——空白引用、集合外方向、倒序区间、与开放版
// 重叠的直插各自被库拒。领域不变量在库内再守一遍，绕开 Go 侧校验也进不来。
func TestPortsPathsMigrationChecksRejectBypassingInserts(t *testing.T) {
	_, fixture := newPortsPathsView(t)

	fixture.rejects(t, "空白口岸标识",
		`INSERT INTO customs_compliance.candidate_port (tenant_id, port_ref, applies_from)
		 VALUES ('tenant-a', '  ', $1)`, viewBaseAt)
	fixture.rejects(t, "倒序口岸区间",
		`INSERT INTO customs_compliance.candidate_port (tenant_id, port_ref, applies_from, applies_until)
		 VALUES ('tenant-a', 'SYN-PORT-02', $1, $2)`, viewBaseAt, viewBaseAt.Add(-time.Hour))
	fixture.rejects(t, "集合外申报方向",
		`INSERT INTO customs_compliance.declaration_path
			(tenant_id, path_ref, applies_from, port_ref, direction, declaration_mode)
		 VALUES ('tenant-a', 'SYN-PATH-02', $1, 'SYN-PORT-01', 'TRANSIT', 'SYN-MODE-GENERAL')`,
		viewBaseAt)
	fixture.rejects(t, "空白申报模式",
		`INSERT INTO customs_compliance.declaration_path
			(tenant_id, path_ref, applies_from, port_ref, direction, declaration_mode)
		 VALUES ('tenant-a', 'SYN-PATH-02', $1, 'SYN-PORT-01', 'IMPORT', '')`,
		viewBaseAt)

	fixture.seed(t,
		`INSERT INTO customs_compliance.candidate_port (tenant_id, port_ref, applies_from)
		 VALUES ('tenant-a', 'SYN-PORT-01', $1)`, viewBaseAt)
	fixture.rejects(t, "与开放版重叠的旁路直插（正道是写口换版）",
		`INSERT INTO customs_compliance.candidate_port (tenant_id, port_ref, applies_from)
		 VALUES ('tenant-a', 'SYN-PORT-01', $1)`, viewBaseAt.Add(48*time.Hour))

	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_path
			(tenant_id, path_ref, applies_from, port_ref, direction, declaration_mode)
		 VALUES ('tenant-a', 'SYN-PATH-01', $1, 'SYN-PORT-01', 'IMPORT', 'SYN-MODE-GENERAL')`,
		viewBaseAt)
	fixture.rejects(t, "与开放版重叠的路径直插",
		`INSERT INTO customs_compliance.declaration_path
			(tenant_id, path_ref, applies_from, port_ref, direction, declaration_mode)
		 VALUES ('tenant-a', 'SYN-PATH-01', $1, 'SYN-PORT-02', 'IMPORT', 'SYN-MODE-GENERAL')`,
		viewBaseAt.Add(48*time.Hour))
}
