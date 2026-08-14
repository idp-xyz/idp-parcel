package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func newGateConditionView(t *testing.T) (*adapter.GateConditionView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	view, err := adapter.NewGateConditionView(fixture.db)
	if err != nil {
		t.Fatalf("构造门禁条件读口：%v", err)
	}
	return view, fixture
}

func loadFindings(
	t *testing.T,
	view *adapter.GateConditionView,
	tenant, scope string,
	action domain.GuardedAction,
	boundary string,
) ([]domain.PreconditionFinding, bool) {
	t.Helper()
	findings, configured, err := view.LoadPreconditionFindings(t.Context(),
		viewValue(t, domain.NewTenantID, tenant),
		viewValue(t, domain.NewDecisionScopeReference, scope),
		action,
		viewValue(t, domain.NewCustomsProcedureReference, boundary))
	if err != nil {
		t.Fatalf("盘前置条件：%v", err)
	}
	return findings, configured
}

func seedGateCatalog(t *testing.T, fixture *viewFixture, tenant, scope, action, boundary string) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO customs_compliance.gate_condition_catalog
			(tenant_id, scope_ref, action, boundary_ref, registered_at)
		 VALUES ($1, $2, $3, $4, $5)`, tenant, scope, action, boundary, viewBaseAt)
}

// 目录未登记 → 未决：没有清单的门禁判断无从复核。
func TestGateConditionsAreUnconfiguredWithoutACatalog(t *testing.T) {
	view, _ := newGateConditionView(t)
	findings, configured := loadFindings(t, view, "tenant-a", "parcel-1", domain.OutboundRelease, "EXPORT/GENERAL")
	if configured || len(findings) != 0 {
		t.Fatalf("没有目录却答出了清单：configured=%v findings=%d", configured, len(findings))
	}
}

// 目录登记了而清单为空 → 「此动作在此边界本就不受门禁」，领域折成`不适用`。这一格
// 与上一格相反：合并两者就是用「查不到」冒充「不受管」，等于把门禁放开。
func TestARegisteredCatalogWithNoConditionFoldsToNotApplicable(t *testing.T) {
	view, fixture := newGateConditionView(t)
	seedGateCatalog(t, fixture, "tenant-a", "parcel-1", "OUTBOUND_RELEASE", "EXPORT/GENERAL")

	findings, configured := loadFindings(t, view, "tenant-a", "parcel-1", domain.OutboundRelease, "EXPORT/GENERAL")
	if !configured || len(findings) != 0 {
		t.Fatalf("空目录走样：configured=%v findings=%d", configured, len(findings))
	}
	conclusion, err := domain.FoldGateConclusion(findings)
	if err != nil || conclusion != domain.GateNotApplicable {
		t.Fatalf("空清单没折成不适用：err=%v conclusion=%s", err, conclusion)
	}
}

func TestGateFindingsAreReadBackAndFoldByTheDomain(t *testing.T) {
	view, fixture := newGateConditionView(t)
	seedGateCatalog(t, fixture, "tenant-a", "parcel-1", "OUTBOUND_RELEASE", "EXPORT/GENERAL")
	fixture.seed(t,
		`INSERT INTO customs_compliance.gate_condition_finding
			(tenant_id, scope_ref, action, boundary_ref, precondition_ref, finding_state)
		 VALUES
			('tenant-a', 'parcel-1', 'OUTBOUND_RELEASE', 'EXPORT/GENERAL', 'DUTY-PAID', 'MET'),
			('tenant-a', 'parcel-1', 'OUTBOUND_RELEASE', 'EXPORT/GENERAL', 'NO-RESTRICTION', 'UNMET')`)

	findings, configured := loadFindings(t, view, "tenant-a", "parcel-1", domain.OutboundRelease, "EXPORT/GENERAL")
	if !configured || len(findings) != 2 {
		t.Fatalf("清单没读回：configured=%v findings=%d", configured, len(findings))
	}
	conclusion, err := domain.FoldGateConclusion(findings)
	if err != nil || conclusion != domain.GatePartiallyMet {
		t.Fatalf("一满足一未满足没折成部分满足：err=%v conclusion=%s", err, conclusion)
	}
}

// 事实冲突压过满足与未满足：读口把 CONFLICTING 原样交出，折叠才判得出整体冲突。
func TestAConflictingFindingSurvivesTheRoundTrip(t *testing.T) {
	view, fixture := newGateConditionView(t)
	seedGateCatalog(t, fixture, "tenant-a", "parcel-1", "OUTBOUND_RELEASE", "EXPORT/GENERAL")
	fixture.seed(t,
		`INSERT INTO customs_compliance.gate_condition_finding
			(tenant_id, scope_ref, action, boundary_ref, precondition_ref, finding_state)
		 VALUES
			('tenant-a', 'parcel-1', 'OUTBOUND_RELEASE', 'EXPORT/GENERAL', 'DUTY-PAID', 'MET'),
			('tenant-a', 'parcel-1', 'OUTBOUND_RELEASE', 'EXPORT/GENERAL', 'CUSTODY', 'CONFLICTING')`)

	findings, _ := loadFindings(t, view, "tenant-a", "parcel-1", domain.OutboundRelease, "EXPORT/GENERAL")
	conclusion, err := domain.FoldGateConclusion(findings)
	if err != nil || conclusion != domain.GateConflicting {
		t.Fatalf("冲突没压过满足：err=%v conclusion=%s", err, conclusion)
	}
}

// 门禁判断不能复用于其他动作或监管边界（CONTEXT 硬句 216）。动作在主键里，因此换
// 一个动作就是另一份目录——这条把它钉在读口上。
func TestGateConditionsDoNotLeakAcrossActionsOrBoundaries(t *testing.T) {
	view, fixture := newGateConditionView(t)
	seedGateCatalog(t, fixture, "tenant-a", "parcel-1", "OUTBOUND_RELEASE", "EXPORT/GENERAL")
	fixture.seed(t,
		`INSERT INTO customs_compliance.gate_condition_finding
			(tenant_id, scope_ref, action, boundary_ref, precondition_ref, finding_state)
		 VALUES ('tenant-a', 'parcel-1', 'OUTBOUND_RELEASE', 'EXPORT/GENERAL', 'DUTY-PAID', 'MET')`)

	if _, configured := loadFindings(t, view, "tenant-a", "parcel-1", domain.FinalDelivery, "EXPORT/GENERAL"); configured {
		t.Fatal("出库的门禁目录替交付作了答")
	}
	if _, configured := loadFindings(t, view, "tenant-a", "parcel-1", domain.OutboundRelease, "IMPORT/GENERAL"); configured {
		t.Fatal("一个监管边界的目录替另一个作了答")
	}
	if _, configured := loadFindings(t, view, "tenant-b", "parcel-1", domain.OutboundRelease, "EXPORT/GENERAL"); configured {
		t.Fatal("跨租户可见")
	}
}

func TestAnUnknownGuardedActionIsLoudRatherThanUnconfigured(t *testing.T) {
	view, _ := newGateConditionView(t)
	_, _, err := view.LoadPreconditionFindings(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewDecisionScopeReference, "parcel-1"),
		domain.GuardedActionInvalid,
		viewValue(t, domain.NewCustomsProcedureReference, "EXPORT/GENERAL"))
	if err == nil {
		t.Fatal("非法动作被当成了目录未登记")
	}
}

func TestGateConditionChecksPinTheClosedSets(t *testing.T) {
	_, fixture := newGateConditionView(t)
	fixture.rejects(t, "封闭四值之外的动作",
		`INSERT INTO customs_compliance.gate_condition_catalog
			(tenant_id, scope_ref, action, boundary_ref, registered_at)
		 VALUES ('t', 's', 'ANYTHING_GOES', 'b', now())`)

	seedGateCatalog(t, fixture, "tenant-a", "parcel-1", "OUTBOUND_RELEASE", "EXPORT/GENERAL")
	fixture.rejects(t, "前置条件的「未知」格",
		`INSERT INTO customs_compliance.gate_condition_finding
			(tenant_id, scope_ref, action, boundary_ref, precondition_ref, finding_state)
		 VALUES ('tenant-a', 'parcel-1', 'OUTBOUND_RELEASE', 'EXPORT/GENERAL', 'P', 'UNKNOWN')`)
	fixture.rejects(t, "逐项判断先于目录存在",
		`INSERT INTO customs_compliance.gate_condition_finding
			(tenant_id, scope_ref, action, boundary_ref, precondition_ref, finding_state)
		 VALUES ('tenant-a', 'parcel-9', 'OUTBOUND_RELEASE', 'EXPORT/GENERAL', 'P', 'MET')`)
}
