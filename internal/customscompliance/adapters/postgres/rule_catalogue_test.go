package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

func newRuleCatalogue(t *testing.T) (*adapter.RuleCatalogue, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	catalogue, err := adapter.NewRuleCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造规则库读口：%v", err)
	}
	return catalogue, fixture
}

func listRequirementRules(
	t *testing.T,
	catalogue *adapter.RuleCatalogue,
	tenant string,
	limit int,
) []ports.CaseRequirementRuleEntry {
	t.Helper()
	entries, err := catalogue.ListCaseRequirementRules(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), limit)
	if err != nil {
		t.Fatalf("上列建案要求规则：%v", err)
	}
	return entries
}

func listInterpretationRules(
	t *testing.T,
	catalogue *adapter.RuleCatalogue,
	tenant string,
	limit int,
) []ports.InterpretationRuleEntry {
	t.Helper()
	entries, err := catalogue.ListInterpretationRules(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), limit)
	if err != nil {
		t.Fatalf("上列解释规则：%v", err)
	}
	return entries
}

// Covers: ADR-0077 Decision 四 — 空目录如实答空列表，不是错误也不折成未配置。
func TestEmptyRuleCatalogueAnswersEmptyLists(t *testing.T) {
	catalogue, _ := newRuleCatalogue(t)
	if entries := listRequirementRules(t, catalogue, "tenant-a", 10); len(entries) != 0 {
		t.Fatalf("空库上列出 %d 条建案要求规则", len(entries))
	}
	if entries := listInterpretationRules(t, catalogue, "tenant-a", 10); len(entries) != 0 {
		t.Fatalf("空库上列出 %d 条解释规则", len(entries))
	}
}

// Covers: ADR-0077 Decision 二/五 — 租户是唯一授权边界：上列只出本租户的行；键序
// 稳定，分页可重复；判断与依据逐字段读回（「不要求」也带依据，与「没登记」分得开）。
func TestCaseRequirementRulesAreListedInKeyOrderWithinTheTenant(t *testing.T) {
	catalogue, fixture := newRuleCatalogue(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.case_requirement_rule
			(tenant_id, jurisdiction_ref, direction, procedure_ref, required, basis)
		 VALUES
			('tenant-a', 'US-CBP', 'IMPORT', 'IMPORT/GENERAL', true,  'PRODUCT/DDP-REQUIRES-CASE'),
			('tenant-a', 'EU-DE',  'EXPORT', 'EXPORT/SIMPLE',  false, 'PRODUCT/CARRIER-DECLARES'),
			('tenant-a', 'EU-DE',  'IMPORT', 'IMPORT/IOSS',    true,  'PRODUCT/IOSS-REQUIRES-CASE'),
			('tenant-b', 'US-CBP', 'IMPORT', 'IMPORT/GENERAL', true,  'PRODUCT/OTHER-TENANT')`)

	entries := listRequirementRules(t, catalogue, "tenant-a", 10)
	if len(entries) != 3 {
		t.Fatalf("上列了 %d 条，要 3 条：%+v", len(entries), entries)
	}
	// 键序（辖区，方向，程序）升序：EU-DE/EXPORT → EU-DE/IMPORT → US-CBP/IMPORT。
	if entries[0].Jurisdiction.String() != "EU-DE" || entries[0].Direction != domain.ExportManifest {
		t.Fatalf("首条不是 EU-DE/EXPORT：%+v", entries[0])
	}
	if entries[1].Jurisdiction.String() != "EU-DE" || entries[1].Direction != domain.ImportManifest {
		t.Fatalf("次条不是 EU-DE/IMPORT：%+v", entries[1])
	}
	if entries[2].Jurisdiction.String() != "US-CBP" || entries[2].Procedure.String() != "IMPORT/GENERAL" {
		t.Fatalf("末条不是 US-CBP/IMPORT：%+v", entries[2])
	}
	// 「不要求」带依据读回。
	if entries[0].Judgment.Required || entries[0].Judgment.Basis != "PRODUCT/CARRIER-DECLARES" {
		t.Fatalf("不要求那行的判断走样：%+v", entries[0].Judgment)
	}
	if !entries[2].Judgment.Required || entries[2].Judgment.Basis != "PRODUCT/DDP-REQUIRES-CASE" {
		t.Fatalf("要求那行的判断走样：%+v", entries[2].Judgment)
	}
	for _, entry := range entries {
		if entry.Judgment.Basis == "PRODUCT/OTHER-TENANT" {
			t.Fatal("跨租户可见")
		}
	}
}

// Covers: ADR-0070 问一甲 — 版本区间逐行读回：开放版终点零值、已闭合版终点在场；
// 同一选择键内当前版在前；跨租户不可见。
func TestInterpretationRuleVersionsCarryTheirValidityIntervals(t *testing.T) {
	catalogue, fixture := newRuleCatalogue(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, applies_until, rule_ref)
		 VALUES
			('tenant-a', 'RELEASE_RESULT',    'US-CBP', $1, $2,   'interpret/release/v1'),
			('tenant-a', 'RELEASE_RESULT',    'US-CBP', $2, NULL, 'interpret/release/v2'),
			('tenant-a', 'REGULATORY_RECEIPT','US-CBP', $1, NULL, 'interpret/receipt/v1'),
			('tenant-b', 'RELEASE_RESULT',    'US-CBP', $1, NULL, 'interpret/other-tenant/v1')`,
		viewBaseAt, viewBaseAt.Add(48*time.Hour))

	entries := listInterpretationRules(t, catalogue, "tenant-a", 10)
	if len(entries) != 3 {
		t.Fatalf("上列了 %d 条，要 3 条：%+v", len(entries), entries)
	}
	// 层名升序：REGULATORY_RECEIPT 在 RELEASE_RESULT 之前；同键内生效起点倒序，
	// 开放的 v2 在已闭合的 v1 之前。
	if entries[0].Layer != domain.RegulatoryReceiptLayer || entries[0].Rule.String() != "interpret/receipt/v1" {
		t.Fatalf("首条不是回执层：%+v", entries[0])
	}
	if entries[1].Rule.String() != "interpret/release/v2" || !entries[1].AppliesUntil.IsZero() {
		t.Fatalf("放行层当前版走样（开放版终点应为零值）：%+v", entries[1])
	}
	if entries[2].Rule.String() != "interpret/release/v1" {
		t.Fatalf("放行层历史版走样：%+v", entries[2])
	}
	if !entries[2].AppliesUntil.Equal(viewBaseAt.Add(48 * time.Hour)) {
		t.Fatalf("已闭合版终点走样：%v", entries[2].AppliesUntil)
	}
	if !entries[2].AppliesFrom.Equal(viewBaseAt) {
		t.Fatalf("生效起点走样：%v", entries[2].AppliesFrom)
	}
	for _, entry := range entries {
		if entry.Rule.String() == "interpret/other-tenant/v1" {
			t.Fatal("跨租户可见")
		}
	}
}

// Covers: ADR-0077 Decision 五 — limit 非正拒；limit 截断行数而不是静默全量。
func TestRuleCatalogueGuardsItsLimit(t *testing.T) {
	catalogue, fixture := newRuleCatalogue(t)
	if _, err := catalogue.ListCaseRequirementRules(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), 0); err == nil {
		t.Fatal("limit=0 的建案要求规则上列被接受了")
	}
	if _, err := catalogue.ListInterpretationRules(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), -1); err == nil {
		t.Fatal("limit=-1 的解释规则上列被接受了")
	}

	fixture.seed(t,
		`INSERT INTO customs_compliance.case_requirement_rule
			(tenant_id, jurisdiction_ref, direction, procedure_ref, required, basis)
		 VALUES
			('tenant-a', 'A', 'IMPORT', 'P1', true, 'B1'),
			('tenant-a', 'B', 'IMPORT', 'P2', true, 'B2'),
			('tenant-a', 'C', 'IMPORT', 'P3', true, 'B3')`)
	if entries := listRequirementRules(t, catalogue, "tenant-a", 2); len(entries) != 2 {
		t.Fatalf("limit=2 却上列了 %d 条", len(entries))
	}
}
