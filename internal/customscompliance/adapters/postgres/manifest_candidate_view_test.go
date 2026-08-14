package postgres_test

import (
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func newManifestCandidateView(t *testing.T) (*adapter.ManifestCandidateView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	view, err := adapter.NewManifestCandidateView(fixture.db)
	if err != nil {
		t.Fatalf("构造候选读口：%v", err)
	}
	return view, fixture
}

func loadCandidates(
	t *testing.T,
	view *adapter.ManifestCandidateView,
	tenant, procedure string,
	direction domain.ManifestDirection,
) []domain.AssociationCandidate {
	t.Helper()
	candidates, err := view.LoadAssociationCandidates(t.Context(),
		viewValue(t, domain.NewTenantID, tenant),
		viewValue(t, domain.NewCustomsProcedureReference, procedure),
		direction)
	if err != nil {
		t.Fatalf("盘候选：%v", err)
	}
	return candidates
}

// 空清单是如实答案，不是错误：引用保持待关联。
func TestNoCandidateIsAnEmptyListRatherThanAnError(t *testing.T) {
	view, _ := newManifestCandidateView(t)
	if candidates := loadCandidates(t, view, "tenant-a", "IMPORT/GENERAL", domain.ImportManifest); len(candidates) != 0 {
		t.Fatalf("空库盘出了 %d 个候选", len(candidates))
	}
}

func TestCandidatesAreFilteredByProcedureDirectionAndTenant(t *testing.T) {
	view, fixture := newManifestCandidateView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.association_candidate
			(tenant_id, unit_id, procedure_ref, direction, scope_ref)
		 VALUES
			('tenant-a', 'unit-1', 'IMPORT/GENERAL', 'IMPORT', 'parcel-1'),
			('tenant-a', 'unit-2', 'IMPORT/GENERAL', 'EXPORT', 'parcel-1'),
			('tenant-a', 'unit-3', 'IMPORT/EXPRESS', 'IMPORT', 'parcel-1'),
			('tenant-b', 'unit-4', 'IMPORT/GENERAL', 'IMPORT', 'parcel-1')`)

	candidates := loadCandidates(t, view, "tenant-a", "IMPORT/GENERAL", domain.ImportManifest)
	if len(candidates) != 1 || candidates[0].Unit.String() != "unit-1" {
		t.Fatalf("过滤走样：%+v", candidates)
	}
	if candidates[0].Procedure.String() != "IMPORT/GENERAL" || candidates[0].Direction != domain.ImportManifest {
		t.Fatalf("候选的程序或方向走样：%+v", candidates[0])
	}
}

// 唯一匹配由领域判定，适配器只管把候选原样交出。这条把两侧连起来：同范围两个候选
// 时保持待关联，恰一个才关联。
func TestLoadedCandidatesDriveTheDomainsUniqueMatch(t *testing.T) {
	view, fixture := newManifestCandidateView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.association_candidate
			(tenant_id, unit_id, procedure_ref, direction, scope_ref)
		 VALUES
			('tenant-a', 'unit-1', 'IMPORT/GENERAL', 'IMPORT', 'parcel-1'),
			('tenant-a', 'unit-2', 'IMPORT/GENERAL', 'IMPORT', 'parcel-1')`)

	reference, err := domain.AcceptManifestReference(domain.ExternalManifestReferenceSpec{
		Manifest:   viewValue(t, domain.NewExternalManifestID, "manifest-1"),
		Version:    viewValue(t, domain.NewManifestSourceVersion, "v1"),
		Carrier:    viewValue(t, domain.NewCarrierResponsibilityReference, "carrier-x"),
		Procedure:  viewValue(t, domain.NewCustomsProcedureReference, "IMPORT/GENERAL"),
		Direction:  domain.ImportManifest,
		Scope:      viewValue(t, domain.NewDecisionScopeReference, "parcel-1"),
		SourceFact: "carrier-report/1",
		AcceptedAt: viewBaseAt,
	})
	if err != nil {
		t.Fatalf("接受舱单引用：%v", err)
	}

	ambiguous := loadCandidates(t, view, "tenant-a", "IMPORT/GENERAL", domain.ImportManifest)
	if _, err := reference.Associate(ambiguous); !errors.Is(err, domain.ErrManifestNotUniquelyMatched) {
		t.Fatalf("两个候选却关联成功了：%v", err)
	}

	// 去掉一个候选，剩下的恰一个即可关联。
	fixture.seed(t,
		`DELETE FROM customs_compliance.association_candidate
		  WHERE tenant_id = 'tenant-a' AND unit_id = 'unit-2'`)
	unique := loadCandidates(t, view, "tenant-a", "IMPORT/GENERAL", domain.ImportManifest)
	associated, err := reference.Associate(unique)
	if err != nil {
		t.Fatalf("唯一候选却关联失败：%v", err)
	}
	unit, ok := associated.Association()
	if !ok || unit.String() != "unit-1" {
		t.Fatalf("关联到了别的单元：ok=%v unit=%s", ok, unit)
	}
}

func TestAnUnknownDirectionIsLoudRatherThanEmpty(t *testing.T) {
	view, _ := newManifestCandidateView(t)
	if _, err := view.LoadAssociationCandidates(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewCustomsProcedureReference, "IMPORT/GENERAL"),
		domain.ManifestDirectionInvalid); err == nil {
		t.Fatal("非法方向被当成了「没有候选」")
	}
}

func TestAssociationCandidateCheckRejectsAnUnknownDirection(t *testing.T) {
	_, fixture := newManifestCandidateView(t)
	fixture.rejects(t, "封闭二值之外的方向",
		`INSERT INTO customs_compliance.association_candidate
			(tenant_id, unit_id, procedure_ref, direction, scope_ref)
		 VALUES ('t', 'u', 'p', 'TRANSIT', 's')`)
}
