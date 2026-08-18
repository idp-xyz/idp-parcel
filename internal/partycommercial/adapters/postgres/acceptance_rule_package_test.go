package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证规则包正文点读口。五维适用性照存但不进
// LoadForScope / ViewRevision（ADR-0059 / D-3）。

func TestAnUnregisteredRulePackageIsNotConfigured(t *testing.T) {
	contents, _ := newAcceptanceRulePackages(t)
	pack := effectiveRulePackage(t, "rules-1", "v1")

	got, found, err := contents.LoadAcceptanceRulePackage(t.Context(), pcTenant(t, "tenant-1"), pack)
	if err != nil {
		t.Fatalf("未配置被当成错误：%v", err)
	}
	if found {
		t.Fatal("查无父行却答已配置")
	}
	if got.Version().ObjectID().String() != "" {
		t.Fatal("未配置的答复带了规则包版本——那等于替租户拟了一份正文")
	}
}

func TestACompleteRulePackageRoundTripsFiveDimensionsAndCategories(t *testing.T) {
	contents, pool := newAcceptanceRulePackages(t)
	pack := effectiveRulePackage(t, "rules-1", "v1")
	starts := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	ends := starts.Add(30 * 24 * time.Hour)
	declareRulePackageContent(t, pool, rulePackageContentRow{
		tenant: "tenant-1", objectID: "rules-1", label: "v1",
		product: "product-a", contract: "contract-a", legal: "legal-a",
		scope: "scope-a", startsAt: starts, endsAt: &ends,
	})
	categories := []struct {
		category  domain.RuleCategory
		reference string
	}{
		{domain.MinimumIngressIdentityRules, "rule-ingress"},
		{domain.ShipmentInvariantRules, "rule-invariant"},
		{domain.ProductAndContractDocumentRules, "rule-document"},
		{domain.RegulatorySourceDocumentRules, "rule-regulatory"},
		{domain.CrossFieldConditionRules, "rule-cross"},
	}
	for _, item := range categories {
		declareAssembledRule(t, pool, "tenant-1", "rules-1", "v1", item.category.String(), item.reference)
	}

	got, found, err := contents.LoadAcceptanceRulePackage(t.Context(), pcTenant(t, "tenant-1"), pack)
	if err != nil || !found {
		t.Fatalf("读回正文：found = %v err = %v", found, err)
	}

	applicability := got.Applicability()
	if applicability.ServiceProduct().String() != "product-a" ||
		applicability.Contract().String() != "contract-a" ||
		applicability.LegalEntity().String() != "legal-a" ||
		applicability.Scope().String() != "scope-a" {
		t.Fatalf("五维往返失真：%#v", applicability)
	}
	gotStart := applicability.Effective().StartsAt()
	gotEnd, bounded := applicability.Effective().EndsAt()
	if !gotStart.Equal(starts) || !bounded || !gotEnd.Equal(ends) {
		t.Fatalf("适用期间 = %v / %v bounded=%v", gotStart, gotEnd, bounded)
	}
	for _, item := range categories {
		rules := got.RulesIn(item.category)
		if len(rules) != 1 || rules[0].Reference().String() != item.reference {
			t.Fatalf("分类 %s 往返失真：%v", item.category, rules)
		}
	}
}

func TestAParentWithoutRulesIsRejectedAsBadData(t *testing.T) {
	contents, pool := newAcceptanceRulePackages(t)
	pack := effectiveRulePackage(t, "rules-1", "v1")
	declareRulePackageContent(t, pool, rulePackageContentRow{
		tenant: "tenant-1", objectID: "rules-1", label: "v1",
		product: "product-a", contract: "contract-a", legal: "legal-a",
		scope: "scope-a", startsAt: effectiveAtRow, endsAt: nil,
	})

	got, found, err := contents.LoadAcceptanceRulePackage(t.Context(), pcTenant(t, "tenant-1"), pack)
	if found || !errors.Is(err, domain.ErrInvalidAcceptanceRulePackage) {
		t.Fatalf("found = %v err = %v；父空子应 error 且不得折成未配置", found, err)
	}
	if got.Version().ObjectID().String() != "" {
		t.Fatal("坏数据交回了规则包版本")
	}
}

func TestRulePackageInvariantsAreMirroredInTheDatabase(t *testing.T) {
	_, pool := newAcceptanceRulePackages(t)

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.acceptance_rule_package
			(tenant_id, object_kind, object_id, version_label,
			 service_product_id, contract_id, legal_entity_ref, scope_ref, effective_starts_at)
		 VALUES ('tenant-1', 2, 'rules-9', 'v1', 'product-a', 'contract-a', 'legal-a', 'scope-a', $1)`,
		effectiveAtRow,
	); err == nil {
		t.Fatal("非接单规则包对象进了正文表")
	}

	declareRulePackageContent(t, pool, rulePackageContentRow{
		tenant: "tenant-1", objectID: "rules-9", label: "v1",
		product: "product-a", contract: "contract-a", legal: "legal-a",
		scope: "scope-a", startsAt: effectiveAtRow, endsAt: nil,
	})

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.acceptance_rule_package_rule
			(tenant_id, object_kind, object_id, version_label, rule_category, rule_reference)
		 VALUES ('tenant-1', 4, 'rules-9', 'v1', 'NOT_A_CATEGORY', 'rule-x')`,
	); err == nil {
		t.Fatal("封闭集外的分类进了规则子表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.acceptance_rule_package_rule
			(tenant_id, object_kind, object_id, version_label, rule_category, rule_reference)
		 VALUES ('tenant-1', 4, 'missing', 'v1', 'SHIPMENT_INVARIANT', 'rule-x')`,
	); err == nil {
		t.Fatal("没有父行的规则进了子表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.acceptance_rule_package_rule
			(tenant_id, object_kind, object_id, version_label, rule_category, rule_reference)
		 VALUES ('tenant-1', 4, 'rules-9', 'v1', 'SHIPMENT_INVARIANT', '  ')`,
	); err == nil {
		t.Fatal("空白规则引用进了子表")
	}
}

func TestMismatchedTenantDoesNotReturnEitherTenantsRulePackage(t *testing.T) {
	contents, pool := newAcceptanceRulePackages(t)
	declareRulePackageContent(t, pool, rulePackageContentRow{
		tenant: "tenant-a", objectID: "rules-1", label: "v1",
		product: "product-a", contract: "contract-a", legal: "legal-a",
		scope: "scope-a", startsAt: effectiveAtRow, endsAt: nil,
	})
	declareAssembledRule(t, pool, "tenant-a", "rules-1", "v1", domain.ShipmentInvariantRules.String(), "rule-a")
	declareRulePackageContent(t, pool, rulePackageContentRow{
		tenant: "tenant-b", objectID: "rules-1", label: "v1",
		product: "product-b", contract: "contract-b", legal: "legal-b",
		scope: "scope-b", startsAt: effectiveAtRow, endsAt: nil,
	})
	declareAssembledRule(t, pool, "tenant-b", "rules-1", "v1", domain.ShipmentInvariantRules.String(), "rule-b")

	theirs := acceptanceRulePackageInTenant(t, "tenant-b", "rules-1", "v1")
	got, found, err := contents.LoadAcceptanceRulePackage(t.Context(), pcTenant(t, "tenant-a"), theirs)
	if err == nil || found {
		t.Fatalf("租户不一致被收下：found = %v err = %v", found, err)
	}
	if got.Applicability().ServiceProduct().String() != "" {
		t.Fatalf("交回了产品 %q——不得返回任一方内容", got.Applicability().ServiceProduct())
	}
}

func TestRulePackageContentsAreScopedByTenantAndVersion(t *testing.T) {
	contents, pool := newAcceptanceRulePackages(t)
	declareRulePackageContent(t, pool, rulePackageContentRow{
		tenant: "tenant-1", objectID: "rules-1", label: "v1",
		product: "product-a", contract: "contract-a", legal: "legal-a",
		scope: "scope-a", startsAt: effectiveAtRow, endsAt: nil,
	})
	declareAssembledRule(t, pool, "tenant-1", "rules-1", "v1", domain.ShipmentInvariantRules.String(), "rule-1")

	if _, found, err := contents.LoadAcceptanceRulePackage(
		t.Context(), pcTenant(t, "tenant-b"), acceptanceRulePackageInTenant(t, "tenant-b", "rules-1", "v1"),
	); err != nil || found {
		t.Fatalf("他租户读到了本租户的正文：found = %v err = %v", found, err)
	}
	if _, found, err := contents.LoadAcceptanceRulePackage(
		t.Context(), pcTenant(t, "tenant-1"), effectiveRulePackage(t, "rules-1", "v2"),
	); err != nil || found {
		t.Fatalf("换版本读到了上一版的正文：found = %v err = %v", found, err)
	}
}

// TestRulePackageContentDoesNotMoveTheViewRevision 证 D-3：正文登记不进 LoadForScope，
// 因而 ViewRevision 不动。选择语义不得被一次点读内容的写入暗改。
func TestRulePackageContentDoesNotMoveTheViewRevision(t *testing.T) {
	publications, transactor, pool := newPublications(t)
	contents := acceptanceRulePackagesOn(t, pool)
	version := effectiveRulePackage(t, "rules-1", "v1")
	mustSaveVersion(t, transactor, t.Context(), publications, version)

	before, err := publications.LoadForScope(t.Context(), pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("装载前：%v", err)
	}
	if before.Count() != 1 {
		t.Fatalf("装载前 count=%d", before.Count())
	}
	beforeRevision := before.ViewRevision(pcTenant(t, "tenant-1"), pcScope(t))

	declareRulePackageContent(t, pool, rulePackageContentRow{
		tenant: "tenant-1", objectID: "rules-1", label: "v1",
		product: "product-a", contract: "contract-a", legal: "legal-a",
		scope: "scope-a", startsAt: effectiveAtRow, endsAt: nil,
	})
	declareAssembledRule(t, pool, "tenant-1", "rules-1", "v1", domain.ShipmentInvariantRules.String(), "rule-1")

	after, err := publications.LoadForScope(t.Context(), pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("装载后：%v", err)
	}
	if after.Count() != 1 {
		t.Fatalf("正文登记改变了版本册 count=%d", after.Count())
	}
	if after.ViewRevision(pcTenant(t, "tenant-1"), pcScope(t)) != beforeRevision {
		t.Fatal("规则包正文登记推动了 ViewRevision——选择语义被暗改了")
	}

	got, found, err := contents.LoadAcceptanceRulePackage(t.Context(), pcTenant(t, "tenant-1"), version)
	if err != nil || !found {
		t.Fatalf("点读应读到正文：found = %v err = %v", found, err)
	}
	if len(got.RulesIn(domain.ShipmentInvariantRules)) != 1 {
		t.Fatal("点读丢了刚登记的规则")
	}
}

func newAcceptanceRulePackages(t *testing.T) (*adapter.AcceptanceRulePackages, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	return acceptanceRulePackagesOn(t, pool), pool
}

func acceptanceRulePackagesOn(t *testing.T, pool *pgxpool.Pool) *adapter.AcceptanceRulePackages {
	t.Helper()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	contents, err := adapter.NewAcceptanceRulePackages(db)
	if err != nil {
		t.Fatalf("构造规则包正文读口：%v", err)
	}
	return contents
}

type rulePackageContentRow struct {
	tenant, objectID, label  string
	product, contract, legal string
	scope                    string
	startsAt                 time.Time
	endsAt                   *time.Time
}

func declareRulePackageContent(t *testing.T, pool *pgxpool.Pool, row rulePackageContentRow) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.acceptance_rule_package
			(tenant_id, object_kind, object_id, version_label,
			 service_product_id, contract_id, legal_entity_ref, scope_ref,
			 effective_starts_at, effective_ends_at)
		 VALUES ($1, 4, $2, $3, $4, $5, $6, $7, $8, $9)`,
		row.tenant, row.objectID, row.label,
		row.product, row.contract, row.legal, row.scope,
		row.startsAt, row.endsAt); err != nil {
		t.Fatalf("登记规则包正文：%v", err)
	}
}

func declareAssembledRule(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label, category, reference string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.acceptance_rule_package_rule
			(tenant_id, object_kind, object_id, version_label, rule_category, rule_reference)
		 VALUES ($1, 4, $2, $3, $4, $5)`,
		tenant, objectID, label, category, reference); err != nil {
		t.Fatalf("登记装配规则：%v", err)
	}
}

func acceptanceRulePackageInTenant(t *testing.T, tenant, objectID, label string) domain.CommercialVersion {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-"+tenant+"-"+objectID),
		pcValue(t, domain.NewCommercialSourceReference, "source-"+tenant+"-"+objectID),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, tenant),
		Kind:          domain.AcceptanceRulePackageObject,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "digest-"+tenant+"-"+objectID+"-"+label),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
	})
	if err != nil {
		t.Fatalf("重建租户 %s 的规则包版本：%v", tenant, err)
	}
	return version
}
