package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

func newCaseRegisterCatalogue(t *testing.T) (*adapter.CaseRegisterCatalogue, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	catalogue, err := adapter.NewCaseRegisterCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造案件配置册读口：%v", err)
	}
	return catalogue, fixture
}

func listReadiness(
	t *testing.T,
	catalogue *adapter.CaseRegisterCatalogue,
	tenant string,
	limit int,
) []domain.ReadinessJudgment {
	t.Helper()
	judgments, err := catalogue.ListReadinessJudgments(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), limit)
	if err != nil {
		t.Fatalf("上列就绪判断：%v", err)
	}
	return judgments
}

func listAuthorities(
	t *testing.T,
	catalogue *adapter.CaseRegisterCatalogue,
	tenant string,
	limit int,
) []domain.SubmissionAuthorization {
	t.Helper()
	authorizations, err := catalogue.ListSubmissionAuthorities(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), limit)
	if err != nil {
		t.Fatalf("上列提交授权：%v", err)
	}
	return authorizations
}

func listObligations(
	t *testing.T,
	catalogue *adapter.CaseRegisterCatalogue,
	tenant string,
	limit int,
) []ports.ClosureObligationCatalogueEntry {
	t.Helper()
	entries, err := catalogue.ListClosureObligations(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), limit)
	if err != nil {
		t.Fatalf("上列关闭义务目录：%v", err)
	}
	return entries
}

// Covers: ADR-0077 Decision 四 — 空册如实答空列表，不是错误也不折成未配置。
func TestEmptyCaseRegistersAnswerEmptyLists(t *testing.T) {
	catalogue, _ := newCaseRegisterCatalogue(t)
	if judgments := listReadiness(t, catalogue, "tenant-a", 10); len(judgments) != 0 {
		t.Fatalf("空库上列出 %d 条就绪判断", len(judgments))
	}
	if authorizations := listAuthorities(t, catalogue, "tenant-a", 10); len(authorizations) != 0 {
		t.Fatalf("空库上列出 %d 条提交授权", len(authorizations))
	}
	if entries := listObligations(t, catalogue, "tenant-a", 10); len(entries) != 0 {
		t.Fatalf("空库上列出 %d 份义务目录", len(entries))
	}
}

// Covers: CONTEXT「提交授权与就绪判断分别形成和失效」 / 0006 自注 — 撤销态如实读回：有效行与已失效行同册并列，原判断
// （依据与形成时间）原样在场，失效不折成未配置也不谎报成仍有效；跨租户不可见；单元
// 键序稳定。
func TestReadinessAndAuthorityListsCarryRevocationHonestly(t *testing.T) {
	catalogue, fixture := newCaseRegisterCatalogue(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.readiness_judgment
			(tenant_id, unit_id, basis_ref, judged_at, revoked_by, revoked_at)
		 VALUES
			('tenant-a', 'unit-2', 'basis-2', $1, 'RULES-CHANGED', $2),
			('tenant-a', 'unit-1', 'basis-1', $1, NULL, NULL),
			('tenant-b', 'unit-9', 'basis-9', $1, NULL, NULL)`,
		viewBaseAt, viewBaseAt.Add(24*time.Hour))
	fixture.seed(t,
		`INSERT INTO customs_compliance.submission_authority
			(tenant_id, unit_id, authority_ref, granted_at, revoked_by, revoked_at)
		 VALUES
			('tenant-a', 'unit-1', 'authority-1', $1, 'MANDATE-WITHDRAWN', $2),
			('tenant-b', 'unit-9', 'authority-9', $1, NULL, NULL)`,
		viewBaseAt, viewBaseAt.Add(48*time.Hour))

	judgments := listReadiness(t, catalogue, "tenant-a", 10)
	if len(judgments) != 2 {
		t.Fatalf("上列了 %d 条就绪判断，要 2 条：%+v", len(judgments), judgments)
	}
	// 单元键序升序：unit-1（有效）在 unit-2（已失效）之前。
	if judgments[0].Unit().String() != "unit-1" || !judgments[0].Effective() {
		t.Fatalf("首条不是仍有效的 unit-1：%+v", judgments[0])
	}
	revoked := judgments[1]
	if revoked.Unit().String() != "unit-2" || revoked.Effective() {
		t.Fatalf("次条不是已失效的 unit-2：%+v", revoked)
	}
	// 失效行原判断原样在场（撤销不是删除），失效原因与时间逐格读回。
	if revoked.Basis().String() != "basis-2" || !revoked.JudgedAt().Equal(viewBaseAt) {
		t.Fatalf("已失效行的原判断走样：basis=%s judgedAt=%v", revoked.Basis(), revoked.JudgedAt())
	}
	cause, at, isRevoked := revoked.Revocation()
	if !isRevoked || cause != "RULES-CHANGED" || !at.Equal(viewBaseAt.Add(24*time.Hour)) {
		t.Fatalf("失效两列走样：cause=%q at=%v", cause, at)
	}

	// 授权那条轨独立失效：同一租户就绪 unit-1 仍有效，而授权 unit-1 已撤销——
	// 「就绪还在、授权已撤销」正是 0006 分表要表达得出的那一格。
	authorizations := listAuthorities(t, catalogue, "tenant-a", 10)
	if len(authorizations) != 1 {
		t.Fatalf("上列了 %d 条提交授权，要 1 条：%+v", len(authorizations), authorizations)
	}
	if authorizations[0].Unit().String() != "unit-1" || authorizations[0].Effective() {
		t.Fatal("授权行没有如实读回已失效态")
	}
	if authorizations[0].Authority().String() != "authority-1" {
		t.Fatalf("失效授权的原依据走样：%+v", authorizations[0])
	}
	for _, judgment := range judgments {
		if judgment.Unit().String() == "unit-9" {
			t.Fatal("跨租户可见")
		}
	}
}

// Covers: 0008 自注 — 目录未登记与登记了空清单分得开：未登记的案件整行缺席，登记了
// 零义务项的案件以空 Items 在场；义务项父子同快照取回，区间与承接对象逐格如实
// （尚无终点=零值，不代填编造时刻）。
func TestClosureObligationCataloguesSeparateUnregisteredFromEmpty(t *testing.T) {
	catalogue, fixture := newCaseRegisterCatalogue(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.closure_obligation_catalog
			(tenant_id, case_ref, registered_at)
		 VALUES
			('tenant-a', 'case-empty', $1),
			('tenant-a', 'case-full',  $1),
			('tenant-b', 'case-other', $1)`,
		viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.closure_obligation_item
			(tenant_id, case_ref, obligation, scope, item_state, basis, handed_to, applies_from, applies_until)
		 VALUES
			('tenant-a', 'case-full', 'obl-b-handed',    'scope-1', 'HANDED_OVER', 'basis-b', 'broker-7', $1, NULL),
			('tenant-a', 'case-full', 'obl-a-concluded', 'scope-1', 'CONCLUDED',   'basis-a', NULL,       $1, $2),
			('tenant-b', 'case-other','obl-x',           'scope-9', 'UNRESOLVED',  'basis-x', NULL,       $1, NULL)`,
		viewBaseAt, viewBaseAt.Add(72*time.Hour))

	entries := listObligations(t, catalogue, "tenant-a", 10)
	if len(entries) != 2 {
		t.Fatalf("上列了 %d 份义务目录，要 2 份：%+v", len(entries), entries)
	}
	// 案件键序升序：case-empty 在 case-full 之前。
	empty := entries[0]
	if empty.Case.String() != "case-empty" || len(empty.Items) != 0 {
		t.Fatalf("登记了零义务项的目录走样（要空 Items 在场）：%+v", empty)
	}
	if !empty.RegisteredAt.Equal(viewBaseAt) {
		t.Fatalf("目录登记时间走样：%v", empty.RegisteredAt)
	}
	full := entries[1]
	if full.Case.String() != "case-full" || len(full.Items) != 2 {
		t.Fatalf("义务目录父子取回走样：%+v", full)
	}
	// 项内按义务名升序：concluded 在 handed 之前。
	concluded := full.Items[0]
	if concluded.Item.Obligation != "obl-a-concluded" || concluded.Item.State != domain.ObligationConcluded {
		t.Fatalf("已终结项走样：%+v", concluded)
	}
	if concluded.Item.HandedTo != "" {
		t.Fatalf("非承接项带了承接对象：%+v", concluded.Item)
	}
	if !concluded.AppliesFrom.Equal(viewBaseAt) || !concluded.AppliesUntil.Equal(viewBaseAt.Add(72*time.Hour)) {
		t.Fatalf("已闭合区间走样：%+v", concluded)
	}
	handed := full.Items[1]
	if handed.Item.State != domain.ObligationHandedOver || handed.Item.HandedTo != "broker-7" {
		t.Fatalf("承接项走样（承接对象必须指名）：%+v", handed.Item)
	}
	if !handed.AppliesUntil.IsZero() {
		t.Fatalf("尚无终点的区间被代填了终点：%v", handed.AppliesUntil)
	}
	for _, entry := range entries {
		if entry.Case.String() == "case-other" {
			t.Fatal("跨租户可见")
		}
	}
}

// Covers: ADR-0077 Decision 五 — limit 非正拒；limit 截断行数而不是静默全量。
func TestCaseRegisterCatalogueGuardsItsLimit(t *testing.T) {
	catalogue, fixture := newCaseRegisterCatalogue(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	if _, err := catalogue.ListReadinessJudgments(t.Context(), tenant, 0); err == nil {
		t.Fatal("limit=0 的就绪上列被接受了")
	}
	if _, err := catalogue.ListSubmissionAuthorities(t.Context(), tenant, -1); err == nil {
		t.Fatal("limit=-1 的授权上列被接受了")
	}
	if _, err := catalogue.ListClosureObligations(t.Context(), tenant, 0); err == nil {
		t.Fatal("limit=0 的义务目录上列被接受了")
	}

	fixture.seed(t,
		`INSERT INTO customs_compliance.readiness_judgment
			(tenant_id, unit_id, basis_ref, judged_at)
		 VALUES
			('tenant-a', 'unit-1', 'basis-1', $1),
			('tenant-a', 'unit-2', 'basis-2', $1),
			('tenant-a', 'unit-3', 'basis-3', $1)`,
		viewBaseAt)
	if judgments := listReadiness(t, catalogue, "tenant-a", 2); len(judgments) != 2 {
		t.Fatalf("limit=2 却上列了 %d 条", len(judgments))
	}
}
