package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

func newCaseRequirementView(t *testing.T) (*adapter.CaseRequirementView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	view, err := adapter.NewCaseRequirementView(fixture.db)
	if err != nil {
		t.Fatalf("构造建案规则读口：%v", err)
	}
	return view, fixture
}

func judgeRequirement(
	t *testing.T,
	view *adapter.CaseRequirementView,
	tenant, jurisdiction string,
	direction domain.ManifestDirection,
	procedure string,
) (ports.CaseRequirementJudgment, bool) {
	t.Helper()
	judgment, found, err := view.JudgeCaseRequirement(t.Context(),
		viewValue(t, domain.NewTenantID, tenant),
		viewValue(t, domain.NewRegulatoryJurisdictionReference, jurisdiction),
		direction,
		viewValue(t, domain.NewCustomsProcedureReference, procedure))
	if err != nil {
		t.Fatalf("判建案：%v", err)
	}
	return judgment, found
}

// 未登记是未决，不是「不要求建案」。把这两格合并会让该建的案不建而无人察觉。
func TestCaseRequirementIsUnconfiguredWhenNoRuleWasRegistered(t *testing.T) {
	view, _ := newCaseRequirementView(t)
	if _, found := judgeRequirement(t, view, "tenant-a", "US-CBP", domain.ImportManifest, "IMPORT/GENERAL"); found {
		t.Fatal("没登记规则却答出了判断")
	}
}

func TestRegisteredCaseRequirementCarriesItsBasisForBothVerdicts(t *testing.T) {
	view, fixture := newCaseRequirementView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.case_requirement_rule
			(tenant_id, jurisdiction_ref, direction, procedure_ref, required, basis)
		 VALUES
			('tenant-a', 'US-CBP', 'IMPORT', 'IMPORT/GENERAL', true, 'PRODUCT/DDP-REQUIRES-CASE'),
			('tenant-a', 'US-CBP', 'EXPORT', 'EXPORT/SIMPLE', false, 'PRODUCT/CARRIER-DECLARES')`)

	required, found := judgeRequirement(t, view, "tenant-a", "US-CBP", domain.ImportManifest, "IMPORT/GENERAL")
	if !found || !required.Required || required.Basis != "PRODUCT/DDP-REQUIRES-CASE" {
		t.Fatalf("要求建案那格走样：found=%v %+v", found, required)
	}
	// 「不要求」也是一个有依据的答案——正因如此它和「没登记」分得开。
	notRequired, found := judgeRequirement(t, view, "tenant-a", "US-CBP", domain.ExportManifest, "EXPORT/SIMPLE")
	if !found || notRequired.Required || notRequired.Basis != "PRODUCT/CARRIER-DECLARES" {
		t.Fatalf("不要求建案那格走样：found=%v %+v", found, notRequired)
	}
}

// 四维共同构成规则身份：换一维就是另一条规则，不沿用。
func TestCaseRequirementDoesNotLeakAcrossTheFourDimensions(t *testing.T) {
	view, fixture := newCaseRequirementView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.case_requirement_rule
			(tenant_id, jurisdiction_ref, direction, procedure_ref, required, basis)
		 VALUES ('tenant-a', 'US-CBP', 'IMPORT', 'IMPORT/GENERAL', true, 'PRODUCT/DDP-REQUIRES-CASE')`)

	if _, found := judgeRequirement(t, view, "tenant-a", "US-CBP", domain.ExportManifest, "IMPORT/GENERAL"); found {
		t.Fatal("换了方向仍沿用原规则")
	}
	if _, found := judgeRequirement(t, view, "tenant-a", "EU-DE", domain.ImportManifest, "IMPORT/GENERAL"); found {
		t.Fatal("换了辖区仍沿用原规则")
	}
	if _, found := judgeRequirement(t, view, "tenant-b", "US-CBP", domain.ImportManifest, "IMPORT/GENERAL"); found {
		t.Fatal("跨租户可见")
	}
}

func TestCaseRequirementChecksRejectABlankBasisAndAnUnknownDirection(t *testing.T) {
	_, fixture := newCaseRequirementView(t)
	fixture.rejects(t, "没有依据的判断",
		`INSERT INTO customs_compliance.case_requirement_rule
			(tenant_id, jurisdiction_ref, direction, procedure_ref, required, basis)
		 VALUES ('t', 'j', 'IMPORT', 'p', false, '   ')`)
	fixture.rejects(t, "封闭二值之外的方向",
		`INSERT INTO customs_compliance.case_requirement_rule
			(tenant_id, jurisdiction_ref, direction, procedure_ref, required, basis)
		 VALUES ('t', 'j', 'TRANSIT', 'p', true, 'b')`)
}
