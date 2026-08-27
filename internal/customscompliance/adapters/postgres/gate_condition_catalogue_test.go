package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

func newGateConditionCatalogue(t *testing.T) (*adapter.GateConditionCatalogue, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	catalogue, err := adapter.NewGateConditionCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造门禁条件册读口：%v", err)
	}
	return catalogue, fixture
}

func listGates(
	t *testing.T,
	catalogue *adapter.GateConditionCatalogue,
	tenant string,
	limit int,
) []ports.GateConditionCatalogueEntry {
	t.Helper()
	entries, err := catalogue.ListGateConditions(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), limit)
	if err != nil {
		t.Fatalf("上列门禁条件目录：%v", err)
	}
	return entries
}

// Covers: ADR-0077 Decision 四 — 空册如实答空列表，不是错误也不折成未配置。
func TestEmptyGateConditionRegisterAnswersEmptyList(t *testing.T) {
	catalogue, _ := newGateConditionCatalogue(t)
	if entries := listGates(t, catalogue, "tenant-a", 10); len(entries) != 0 {
		t.Fatalf("空库上列出 %d 份门禁目录", len(entries))
	}
}

// Covers: 0008 自注 — 门禁的「未登记」与「登记了空清单」分得开，且含义与关闭义务那对
// 相反：未登记的三维组合整行缺席（未决，无从复核），登记了零认定的组合以空 Findings
// 在场（此动作在此边界本就不受门禁）。认定父子同快照取回、封闭三值逐格如实；跨租户
// 不可见；三维键序稳定。
func TestGateConditionCataloguesSeparateUnregisteredFromNotGuarded(t *testing.T) {
	catalogue, fixture := newGateConditionCatalogue(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.gate_condition_catalog
			(tenant_id, scope_ref, action, boundary_ref, registered_at)
		 VALUES
			('tenant-a', 'scope-1', 'OUTBOUND_RELEASE',       'proc-cn', $1),
			('tenant-a', 'scope-1', 'CROSS_CUSTOMS_MOVEMENT', 'proc-cn', $1),
			('tenant-b', 'scope-9', 'FINAL_DELIVERY',         'proc-x',  $1)`,
		viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.gate_condition_finding
			(tenant_id, scope_ref, action, boundary_ref, precondition_ref, finding_state)
		 VALUES
			('tenant-a', 'scope-1', 'CROSS_CUSTOMS_MOVEMENT', 'proc-cn', 'pre-b-duty',    'UNMET'),
			('tenant-a', 'scope-1', 'CROSS_CUSTOMS_MOVEMENT', 'proc-cn', 'pre-a-release', 'MET'),
			('tenant-a', 'scope-1', 'CROSS_CUSTOMS_MOVEMENT', 'proc-cn', 'pre-c-facts',   'CONFLICTING'),
			('tenant-b', 'scope-9', 'FINAL_DELIVERY',         'proc-x',  'pre-x',         'MET')`)

	entries := listGates(t, catalogue, "tenant-a", 10)
	if len(entries) != 2 {
		t.Fatalf("上列了 %d 份门禁目录，要 2 份：%+v", len(entries), entries)
	}
	// 三维键序（范围、动作、边界）升序：CROSS_CUSTOMS_MOVEMENT 在 OUTBOUND_RELEASE 之前。
	guarded := entries[0]
	if guarded.Action != domain.CrossCustomsMovement || guarded.Scope.String() != "scope-1" ||
		guarded.Boundary.String() != "proc-cn" {
		t.Fatalf("首份不是跨关务区域移动那份：%+v", guarded)
	}
	if !guarded.RegisteredAt.Equal(viewBaseAt) {
		t.Fatalf("目录登记时间走样：%v", guarded.RegisteredAt)
	}
	if len(guarded.Findings) != 3 {
		t.Fatalf("认定父子取回走样，要 3 项：%+v", guarded.Findings)
	}
	// 项内按前置条件引用升序；封闭三值逐格如实，没有第四格。
	expected := []struct {
		precondition string
		state        domain.PreconditionState
	}{
		{"pre-a-release", domain.PreconditionMet},
		{"pre-b-duty", domain.PreconditionUnmet},
		{"pre-c-facts", domain.PreconditionConflicting},
	}
	for index, want := range expected {
		got := guarded.Findings[index]
		if got.Precondition.String() != want.precondition || got.State != want.state {
			t.Fatalf("第 %d 项认定走样：%+v（要 %+v）", index, got, want)
		}
	}

	// 登记了零认定的目录以空 Findings 在场——「此动作在此边界本就不受门禁」的如实
	// 一格；它绝不能与「未登记」（下面 LOADING_DEPARTURE 那问）混成一个信号。
	notGuarded := entries[1]
	if notGuarded.Action != domain.OutboundRelease || len(notGuarded.Findings) != 0 {
		t.Fatalf("零认定目录走样（要空 Findings 在场）：%+v", notGuarded)
	}
	for _, entry := range entries {
		if entry.Scope.String() == "scope-9" {
			t.Fatal("跨租户可见")
		}
		if entry.Action == domain.LoadingDeparture {
			t.Fatal("未登记的动作凭空在列")
		}
	}
}

// Covers: ADR-0077 Decision 五 — limit 非正拒；limit 截断行数而不是静默全量。
func TestGateConditionCatalogueGuardsItsLimit(t *testing.T) {
	catalogue, fixture := newGateConditionCatalogue(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	if _, err := catalogue.ListGateConditions(t.Context(), tenant, 0); err == nil {
		t.Fatal("limit=0 的门禁上列被接受了")
	}
	if _, err := catalogue.ListGateConditions(t.Context(), tenant, -1); err == nil {
		t.Fatal("limit=-1 的门禁上列被接受了")
	}

	fixture.seed(t,
		`INSERT INTO customs_compliance.gate_condition_catalog
			(tenant_id, scope_ref, action, boundary_ref, registered_at)
		 VALUES
			('tenant-a', 'scope-1', 'OUTBOUND_RELEASE',  'proc-cn', $1),
			('tenant-a', 'scope-2', 'FINAL_DELIVERY',    'proc-cn', $1),
			('tenant-a', 'scope-3', 'LOADING_DEPARTURE', 'proc-cn', $1)`,
		viewBaseAt)
	if entries := listGates(t, catalogue, "tenant-a", 2); len(entries) != 2 {
		t.Fatalf("limit=2 却上列了 %d 份", len(entries))
	}
}
