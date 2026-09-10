package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func TestUnregisteredStageContentIsNotConfigured(t *testing.T) {
	declarations, _ := newStageContentDeclarations(t)
	rules := effectiveRulePackage(t, "rules-1", "v1")
	auth := effectiveAuthorizationRule(t, "auth-1", "v1")
	tenant := pcTenant(t, "tenant-1")

	if got, found, err := declarations.LoadIntakeQualification(t.Context(), tenant, rules); err != nil || found {
		t.Fatalf("收寄资格未配置：found = %v err = %v", found, err)
	} else if len(got.AllowedSources()) != 0 {
		t.Fatal("未配置的收寄资格带了来源")
	}

	if got, found, err := declarations.LoadFinalRule(t.Context(), tenant, rules); err != nil || found {
		t.Fatalf("终局规则未配置：found = %v err = %v", found, err)
	} else if _, declared := got.FinalKindFor(domain.DeclaredEffectiveDelivery); declared {
		t.Fatal("未配置的终局规则带了声明行")
	}

	if got, found, err := declarations.LoadCancellationAuthority(t.Context(), tenant, auth); err != nil || found {
		t.Fatalf("取消授权未配置：found = %v err = %v", found, err)
	} else if _, declared := got.RuleFor(domain.DeclaredCustomerCancellation); declared {
		t.Fatal("未配置的取消授权带了声明行")
	}
}

func TestStageContentRoundTrips(t *testing.T) {
	declarations, pool := newStageContentDeclarations(t)
	rules := effectiveRulePackage(t, "rules-1", "v1")
	auth := effectiveAuthorizationRule(t, "auth-1", "v1")
	tenant := pcTenant(t, "tenant-1")

	declareIntakeParent(t, pool, "tenant-1", "rules-1", "v1")
	declareIntakeSource(t, pool, "tenant-1", "rules-1", "v1", "NODE_INTAKE")
	declareIntakeSource(t, pool, "tenant-1", "rules-1", "v1", "OFFSITE_PICKUP")
	declareIntakeQualification(t, pool, "tenant-1", "rules-1", "v1", "INTAKE-QUAL/customs-precheck")

	intake, found, err := declarations.LoadIntakeQualification(t.Context(), tenant, rules)
	if err != nil || !found {
		t.Fatalf("读回收寄资格：found = %v err = %v", found, err)
	}
	if !intake.Allows(domain.DeclaredNodeIntake) || !intake.Allows(domain.DeclaredOffsitePickup) {
		t.Fatal("允许来源没保真回来")
	}
	if quals := intake.Qualifications(); len(quals) != 1 || quals[0].String() != "INTAKE-QUAL/customs-precheck" {
		t.Fatalf("qualifications = %#v", quals)
	}

	declareFinalParent(t, pool, "tenant-1", "rules-1", "v1")
	declareFinalRow(t, pool, "tenant-1", "rules-1", "v1", "EFFECTIVE_DELIVERY", "NETWORK_SERVICE_DELIVERED")
	final, found, err := declarations.LoadFinalRule(t.Context(), tenant, rules)
	if err != nil || !found {
		t.Fatalf("读回终局规则：found = %v err = %v", found, err)
	}
	kind, declared := final.FinalKindFor(domain.DeclaredEffectiveDelivery)
	if !declared || kind.String() != "NETWORK_SERVICE_DELIVERED" {
		t.Fatalf("kind = %v declared = %v", kind, declared)
	}
	if _, declared := final.FinalKindFor(domain.DeclaredReturnCompleted); declared {
		t.Fatal("没声明的责任结果答成了形成终局")
	}

	declareCancellationParent(t, pool, "tenant-1", "auth-1", "v1")
	declareCancellationRow(t, pool, "tenant-1", "auth-1", "v1", "CUSTOMER", "CANCEL-RULE/CUSTOMER")
	catalog, found, err := declarations.LoadCancellationAuthority(t.Context(), tenant, auth)
	if err != nil || !found {
		t.Fatalf("读回取消授权：found = %v err = %v", found, err)
	}
	rule, declared := catalog.RuleFor(domain.DeclaredCustomerCancellation)
	if !declared || rule.String() != "CANCEL-RULE/CUSTOMER" {
		t.Fatalf("rule = %v declared = %v", rule, declared)
	}
	if _, declared := catalog.RuleFor(domain.DeclaredOperationsCancellation); declared {
		t.Fatal("没声明的请求方格答成了允许")
	}
}

// Covers: pc-gaps/12 完成判据 3——终局规则声明带面单渠道两行时经 SaveFinalRule / LoadFinalRule 往返保真；0013 钉四个字面量的
// final_rule_declaration_closed_set 经 0031 放宽后不再拒 LABEL_SERVICE_COMPLETED / LABEL_SERVICE_FAILED，集外词照旧拒在
// CHECK（TestStageContentClosedSetsAreMirroredInTheDatabase 那一格不变）。只声明面单两格、不声明网络服务格的规则包，网络格读回即缺行。
func TestFinalRuleLabelServiceRowsRoundTripThroughTheDatabase(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	rules := effectiveRulePackage(t, "rules-label", "v1")
	tenant := pcTenant(t, "tenant-1")

	content, err := domain.NewFinalRuleContent(rules, []domain.FinalizationDeclaration{
		{Outcome: domain.DeclaredLabelServiceCompleted, FinalKind: pcValue(t, domain.NewRuleReference, "LABEL_SERVICE_DONE")},
		{Outcome: domain.DeclaredLabelServiceFailed, FinalKind: pcValue(t, domain.NewRuleReference, "LABEL_SERVICE_FAILED_FINAL")},
	})
	if err != nil {
		t.Fatalf("组面单渠道终局声明：%v", err)
	}
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveFinalRule(txCtx, content)
	}, ports.DeclarationSaved)

	reader, err := adapter.NewStageContentDeclarations(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}
	loaded, found, err := reader.LoadFinalRule(t.Context(), tenant, rules)
	if err != nil || !found {
		t.Fatalf("读回面单渠道终局声明：found = %v err = %v", found, err)
	}
	if kind, declared := loaded.FinalKindFor(domain.DeclaredLabelServiceCompleted); !declared || kind.String() != "LABEL_SERVICE_DONE" {
		t.Fatalf("completed: kind = %v declared = %v", kind, declared)
	}
	if kind, declared := loaded.FinalKindFor(domain.DeclaredLabelServiceFailed); !declared || kind.String() != "LABEL_SERVICE_FAILED_FINAL" {
		t.Fatalf("failed: kind = %v declared = %v", kind, declared)
	}
	if _, declared := loaded.FinalKindFor(domain.DeclaredEffectiveDelivery); declared {
		t.Fatal("只声明面单两格的规则包替有效交付答成了形成终局")
	}
}

func TestExplicitEmptyIntakeQualificationsAreConfigured(t *testing.T) {
	declarations, pool := newStageContentDeclarations(t)
	rules := effectiveRulePackage(t, "rules-1", "v1")
	declareIntakeParent(t, pool, "tenant-1", "rules-1", "v1")
	declareIntakeSource(t, pool, "tenant-1", "rules-1", "v1", "NODE_INTAKE")

	got, found, err := declarations.LoadIntakeQualification(t.Context(), pcTenant(t, "tenant-1"), rules)
	if err != nil || !found {
		t.Fatalf("读回空资格清单：found = %v err = %v", found, err)
	}
	if len(got.Qualifications()) != 0 {
		t.Fatal("显式空清单被改写了")
	}
	if !got.Allows(domain.DeclaredNodeIntake) {
		t.Fatal("允许来源丢了")
	}
}

func TestParentWithoutRequiredChildrenIsRejected(t *testing.T) {
	declarations, pool := newStageContentDeclarations(t)
	rules := effectiveRulePackage(t, "rules-1", "v1")
	auth := effectiveAuthorizationRule(t, "auth-1", "v1")
	tenant := pcTenant(t, "tenant-1")

	declareIntakeParent(t, pool, "tenant-1", "rules-1", "v1")
	if _, found, err := declarations.LoadIntakeQualification(t.Context(), tenant, rules); found || !errors.Is(err, domain.ErrIntakeContentNotConfigured) {
		t.Fatalf("收寄资格父行无来源：found = %v err = %v", found, err)
	}

	declareFinalParent(t, pool, "tenant-1", "rules-1", "v1")
	if _, found, err := declarations.LoadFinalRule(t.Context(), tenant, rules); found || !errors.Is(err, domain.ErrFinalContentNotConfigured) {
		t.Fatalf("终局规则父行无子行：found = %v err = %v", found, err)
	}

	declareCancellationParent(t, pool, "tenant-1", "auth-1", "v1")
	if _, found, err := declarations.LoadCancellationAuthority(t.Context(), tenant, auth); found || !errors.Is(err, domain.ErrCancellationAuthorityNotConfigured) {
		t.Fatalf("取消授权父行无子行：found = %v err = %v", found, err)
	}
}

func TestStageContentClosedSetsAreMirroredInTheDatabase(t *testing.T) {
	_, pool := newStageContentDeclarations(t)

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.intake_qualification_content
			(tenant_id, object_kind, object_id, version_label)
		 VALUES ('tenant-1', 2, 'rules-9', 'v1')`); err == nil {
		t.Fatal("非规则包对象进了收寄资格表")
	}

	declareIntakeParent(t, pool, "tenant-1", "rules-9", "v1")
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.intake_allowed_source
			(tenant_id, object_kind, object_id, version_label, source_kind)
		 VALUES ('tenant-1', 4, 'rules-9', 'v1', 'CHANNEL_INTAKE')`); err == nil {
		t.Fatal("封闭集外的收寄来源进了表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.final_rule_content
			(tenant_id, object_kind, object_id, version_label)
		 VALUES ('tenant-1', 1, 'rules-9', 'v1')`); err == nil {
		t.Fatal("非规则包对象进了终局规则表")
	}

	declareFinalParent(t, pool, "tenant-1", "rules-8", "v1")
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.final_rule_declaration
			(tenant_id, object_kind, object_id, version_label, outcome, final_kind)
		 VALUES ('tenant-1', 4, 'rules-8', 'v1', 'PARTIAL_DELIVERY', 'X')`); err == nil {
		t.Fatal("封闭集外的责任结果进了表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.cancellation_authority_content
			(tenant_id, object_kind, object_id, version_label)
		 VALUES ('tenant-1', 4, 'auth-9', 'v1')`); err == nil {
		t.Fatal("非授权规则对象进了取消授权表")
	}

	declareCancellationParent(t, pool, "tenant-1", "auth-8", "v1")
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.cancellation_authority_declaration
			(tenant_id, object_kind, object_id, version_label, party, rule_reference)
		 VALUES ('tenant-1', 9, 'auth-8', 'v1', 'CARRIER', 'R')`); err == nil {
		t.Fatal("封闭集外的请求方格进了表")
	}
}

func TestMismatchedTenantDoesNotReturnEitherTenantsStageContent(t *testing.T) {
	declarations, pool := newStageContentDeclarations(t)
	declareIntakeParent(t, pool, "tenant-a", "rules-1", "v1")
	declareIntakeSource(t, pool, "tenant-a", "rules-1", "v1", "NODE_INTAKE")
	declareIntakeParent(t, pool, "tenant-b", "rules-1", "v1")
	declareIntakeSource(t, pool, "tenant-b", "rules-1", "v1", "OFFSITE_PICKUP")

	theirs := rulePackageInTenant(t, "tenant-b", "rules-1", "v1", "digest-b")
	got, found, err := declarations.LoadIntakeQualification(t.Context(), pcTenant(t, "tenant-a"), theirs)
	if err == nil || found {
		t.Fatalf("租户不一致被收下：found = %v err = %v", found, err)
	}
	if got.Allows(domain.DeclaredNodeIntake) || got.Allows(domain.DeclaredOffsitePickup) {
		t.Fatal("交回了任一方的收寄来源")
	}

	declareFinalParent(t, pool, "tenant-a", "rules-1", "v1")
	declareFinalRow(t, pool, "tenant-a", "rules-1", "v1", "EFFECTIVE_DELIVERY", "KIND-A")
	declareFinalParent(t, pool, "tenant-b", "rules-1", "v1")
	declareFinalRow(t, pool, "tenant-b", "rules-1", "v1", "RETURN_COMPLETED", "KIND-B")
	gotFinal, found, err := declarations.LoadFinalRule(t.Context(), pcTenant(t, "tenant-a"), theirs)
	if err == nil || found {
		t.Fatalf("终局规则租户不一致被收下：found = %v err = %v", found, err)
	}
	if _, declared := gotFinal.FinalKindFor(domain.DeclaredEffectiveDelivery); declared {
		t.Fatal("交回了 A 的终局声明")
	}
	if _, declared := gotFinal.FinalKindFor(domain.DeclaredReturnCompleted); declared {
		t.Fatal("交回了 B 的终局声明")
	}

	declareCancellationParent(t, pool, "tenant-a", "auth-1", "v1")
	declareCancellationRow(t, pool, "tenant-a", "auth-1", "v1", "CUSTOMER", "RULE-A")
	declareCancellationParent(t, pool, "tenant-b", "auth-1", "v1")
	declareCancellationRow(t, pool, "tenant-b", "auth-1", "v1", "OPERATIONS", "RULE-B")
	theirsAuth := authorizationRuleInTenant(t, "tenant-b", "auth-1", "v1", "digest-b")
	gotAuth, found, err := declarations.LoadCancellationAuthority(t.Context(), pcTenant(t, "tenant-a"), theirsAuth)
	if err == nil || found {
		t.Fatalf("取消授权租户不一致被收下：found = %v err = %v", found, err)
	}
	if _, declared := gotAuth.RuleFor(domain.DeclaredCustomerCancellation); declared {
		t.Fatal("交回了 A 的取消授权")
	}
	if _, declared := gotAuth.RuleFor(domain.DeclaredOperationsCancellation); declared {
		t.Fatal("交回了 B 的取消授权")
	}
}

func TestStageContentIsScopedByTenantAndVersion(t *testing.T) {
	declarations, pool := newStageContentDeclarations(t)
	declareIntakeParent(t, pool, "tenant-1", "rules-1", "v1")
	declareIntakeSource(t, pool, "tenant-1", "rules-1", "v1", "NODE_INTAKE")

	if _, found, err := declarations.LoadIntakeQualification(
		t.Context(),
		pcTenant(t, "tenant-b"),
		rulePackageInTenant(t, "tenant-b", "rules-1", "v1", "digest-1"),
	); err != nil || found {
		t.Fatalf("他租户读到了本租户的收寄资格：found = %v err = %v", found, err)
	}
	if _, found, err := declarations.LoadIntakeQualification(
		t.Context(),
		pcTenant(t, "tenant-1"),
		effectiveRulePackage(t, "rules-1", "v2"),
	); err != nil || found {
		t.Fatalf("换版本读到了上一版的收寄资格：found = %v err = %v", found, err)
	}
}

func newStageContentDeclarations(t *testing.T) (*adapter.StageContentDeclarations, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	declarations, err := adapter.NewStageContentDeclarations(db)
	if err != nil {
		t.Fatalf("构造阶段内容读口：%v", err)
	}
	return declarations, pool
}

func effectiveAuthorizationRule(t *testing.T, objectID, label string) domain.CommercialVersion {
	t.Helper()
	return versionOfKindInTenant(t, "tenant-1", domain.AuthorizationRuleObject, objectID, label, "digest-"+objectID+"-"+label)
}

func rulePackageInTenant(t *testing.T, tenant, objectID, label, digest string) domain.CommercialVersion {
	t.Helper()
	return versionOfKindInTenant(t, tenant, domain.AcceptanceRulePackageObject, objectID, label, digest)
}

func authorizationRuleInTenant(t *testing.T, tenant, objectID, label, digest string) domain.CommercialVersion {
	t.Helper()
	return versionOfKindInTenant(t, tenant, domain.AuthorizationRuleObject, objectID, label, digest)
}

func versionOfKindInTenant(
	t *testing.T,
	tenant string,
	kind domain.CommercialObjectKind,
	objectID, label, digest string,
) domain.CommercialVersion {
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
		Kind:          kind,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, digest),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
	})
	if err != nil {
		t.Fatalf("重建租户 %s 的 %s 版本：%v", tenant, kind, err)
	}
	return version
}

func declareIntakeParent(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.intake_qualification_content
			(tenant_id, object_kind, object_id, version_label)
		 VALUES ($1, 4, $2, $3)`,
		tenant, objectID, label); err != nil {
		t.Fatalf("登记收寄资格父行：%v", err)
	}
}

func declareIntakeSource(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label, source string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.intake_allowed_source
			(tenant_id, object_kind, object_id, version_label, source_kind)
		 VALUES ($1, 4, $2, $3, $4)`,
		tenant, objectID, label, source); err != nil {
		t.Fatalf("登记允许来源：%v", err)
	}
}

func declareIntakeQualification(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label, ref string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.intake_qualification_ref
			(tenant_id, object_kind, object_id, version_label, rule_reference)
		 VALUES ($1, 4, $2, $3, $4)`,
		tenant, objectID, label, ref); err != nil {
		t.Fatalf("登记资格引用：%v", err)
	}
}

func declareFinalParent(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.final_rule_content
			(tenant_id, object_kind, object_id, version_label)
		 VALUES ($1, 4, $2, $3)`,
		tenant, objectID, label); err != nil {
		t.Fatalf("登记终局规则父行：%v", err)
	}
}

func declareFinalRow(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label, outcome, kind string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.final_rule_declaration
			(tenant_id, object_kind, object_id, version_label, outcome, final_kind)
		 VALUES ($1, 4, $2, $3, $4, $5)`,
		tenant, objectID, label, outcome, kind); err != nil {
		t.Fatalf("登记终局声明行：%v", err)
	}
}

func declareCancellationParent(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.cancellation_authority_content
			(tenant_id, object_kind, object_id, version_label)
		 VALUES ($1, 9, $2, $3)`,
		tenant, objectID, label); err != nil {
		t.Fatalf("登记取消授权父行：%v", err)
	}
}

func declareCancellationRow(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label, party, rule string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.cancellation_authority_declaration
			(tenant_id, object_kind, object_id, version_label, party, rule_reference)
		 VALUES ($1, 9, $2, $3, $4, $5)`,
		tenant, objectID, label, party, rule); err != nil {
		t.Fatalf("登记取消授权行：%v", err)
	}
}
