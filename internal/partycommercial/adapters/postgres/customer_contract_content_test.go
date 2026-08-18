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

// 本文件对真实 PostgreSQL 16 证合同正文点读口。重点在父子表把「未登记」与「已登记
// 但零绑定」分开：无父行是未配置，有父行零子行是明确的空约定，后者对任何费用范围
// 答「不存在」而不是「不适用」。

func TestAnUnregisteredContractIsNotConfigured(t *testing.T) {
	contents, _ := newCustomerContractContents(t)
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")

	got, found, err := contents.LoadCustomerContract(t.Context(), pcTenant(t, "tenant-1"), contract)
	if err != nil {
		t.Fatalf("未配置被当成错误：%v", err)
	}
	if found {
		t.Fatal("查无父行却答已配置")
	}
	if got.AcceptanceRulePackage().String() != "" {
		t.Fatal("未配置的答复带了规则包——那等于替租户拟了一份正文")
	}
}

// TestAnExplicitEmptyBindingSetIsConfigured 证有父行零子行是登记过的空约定，不是未配置。
func TestAnExplicitEmptyBindingSetIsConfigured(t *testing.T) {
	contents, pool := newCustomerContractContents(t)
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")
	declareContractContent(t, pool, "tenant-1", "contract-1", "v1", "rules-1")

	got, found, err := contents.LoadCustomerContract(t.Context(), pcTenant(t, "tenant-1"), contract)
	if err != nil || !found {
		t.Fatalf("读回空约定：found = %v err = %v", found, err)
	}
	if got.AcceptanceRulePackage().String() != "rules-1" {
		t.Fatalf("rule package = %q", got.AcceptanceRulePackage())
	}
	if _, present := got.FinancialControlFor(pcValue(t, domain.NewChargeScopeReference, "charge-express")); present {
		t.Fatal("零绑定合同对未约定范围给出了约定")
	}
}

// TestDeclaredControlBindingsRoundTrip 证应用与显式不适用两种绑定保真回来。
func TestDeclaredControlBindingsRoundTrip(t *testing.T) {
	contents, pool := newCustomerContractContents(t)
	contract := effectiveContract(t, "contract-1", "v1", "digest-1")
	declareContractContent(t, pool, "tenant-1", "contract-1", "v1", "rules-1")
	policy := "policy-express"
	declareAppliedBinding(t, pool, "tenant-1", "contract-1", "v1", "charge-express", policy)
	basis := "CONTRACT_STATES_NO_PRE_ACCEPTANCE_CONTROL"
	declareInapplicableBinding(t, pool, "tenant-1", "contract-1", "v1", "charge-economy", basis)

	got, found, err := contents.LoadCustomerContract(t.Context(), pcTenant(t, "tenant-1"), contract)
	if err != nil || !found {
		t.Fatalf("读回绑定：found = %v err = %v", found, err)
	}

	applied, present := got.FinancialControlFor(pcValue(t, domain.NewChargeScopeReference, "charge-express"))
	if !present || !applied.Applies() {
		t.Fatalf("applied = %#v present = %v", applied, present)
	}
	named, ok := applied.Policy()
	if !ok || named.String() != policy {
		t.Fatal("应用绑定丢了策略引用")
	}

	inapplicable, present := got.FinancialControlFor(pcValue(t, domain.NewChargeScopeReference, "charge-economy"))
	if !present || !inapplicable.ExplicitlyInapplicable() {
		t.Fatalf("inapplicable = %#v present = %v", inapplicable, present)
	}
	if inapplicable.InapplicabilityBasis().String() != basis {
		t.Fatal("不适用绑定丢了依据")
	}
	if _, named := inapplicable.Policy(); named {
		t.Fatal("不适用绑定带着策略回来了")
	}
}

// TestContentRulePackageMustAgreeWithTheVersionShell 证 F-3：壳上指名的规则包与正文件
// 列不等则整次拒装，不静默选一处；壳上没指名则正文件列单独作数。
func TestContentRulePackageMustAgreeWithTheVersionShell(t *testing.T) {
	contents, pool := newCustomerContractContents(t)

	t.Run("两处分歧", func(t *testing.T) {
		contract := effectiveContract(t, "contract-1", "v1", "digest-1")
		declareContractContent(t, pool, "tenant-1", "contract-1", "v1", "rules-OTHER")
		_, found, err := contents.LoadCustomerContract(t.Context(), pcTenant(t, "tenant-1"), contract)
		if found || !errors.Is(err, domain.ErrRulePackageReferenceMismatch) {
			t.Fatalf("found = %v err = %v；分歧应 error 且不得折成未配置", found, err)
		}
	})

	t.Run("壳上没指名", func(t *testing.T) {
		unnamed := contractVersionInTenant(t, "tenant-1", "contract-2", "v1", "digest-2")
		declareContractContent(t, pool, "tenant-1", "contract-2", "v1", "rules-alone")
		got, found, err := contents.LoadCustomerContract(t.Context(), pcTenant(t, "tenant-1"), unnamed)
		if err != nil || !found {
			t.Fatalf("壳上没指名却拒了：found = %v err = %v", found, err)
		}
		if got.AcceptanceRulePackage().String() != "rules-alone" {
			t.Fatalf("rule package = %q", got.AcceptanceRulePackage())
		}
	})
}

// TestMismatchedTenantDoesNotReturnEitherTenantsContract 证租户身份闭包：两租户合法
// 同号（ADR-0040），用 A 的租户参数配 B 的合同对象不得读回任一方正文——按 A 查库再
// 用 B 重建，会把 A 的绑定装进 B 的合同。
func TestMismatchedTenantDoesNotReturnEitherTenantsContract(t *testing.T) {
	contents, pool := newCustomerContractContents(t)
	declareContractContent(t, pool, "tenant-a", "contract-1", "v1", "rules-a")
	declareAppliedBinding(t, pool, "tenant-a", "contract-1", "v1", "charge-a", "policy-a")
	declareContractContent(t, pool, "tenant-b", "contract-1", "v1", "rules-b")
	declareAppliedBinding(t, pool, "tenant-b", "contract-1", "v1", "charge-b", "policy-b")

	theirs := contractVersionInTenant(t, "tenant-b", "contract-1", "v1", "digest-b")
	got, found, err := contents.LoadCustomerContract(t.Context(), pcTenant(t, "tenant-a"), theirs)
	if err == nil || found {
		t.Fatalf("租户不一致被收下：found = %v err = %v", found, err)
	}
	if got.AcceptanceRulePackage().String() != "" {
		t.Fatalf("交回了规则包 %q——不得返回任一方内容", got.AcceptanceRulePackage())
	}
	if _, present := got.FinancialControlFor(pcValue(t, domain.NewChargeScopeReference, "charge-a")); present {
		t.Fatal("交回了 A 的绑定")
	}
	if _, present := got.FinancialControlFor(pcValue(t, domain.NewChargeScopeReference, "charge-b")); present {
		t.Fatal("交回了 B 的绑定")
	}
}

func TestCustomerContractContentsAreScopedByTenantAndVersion(t *testing.T) {
	contents, pool := newCustomerContractContents(t)
	declareContractContent(t, pool, "tenant-1", "contract-1", "v1", "rules-1")

	if _, found, err := contents.LoadCustomerContract(
		t.Context(), pcTenant(t, "tenant-b"), contractVersionInTenant(t, "tenant-b", "contract-1", "v1", "digest-1"),
	); err != nil || found {
		t.Fatalf("他租户读到了本租户的正文：found = %v err = %v", found, err)
	}
	if _, found, err := contents.LoadCustomerContract(
		t.Context(), pcTenant(t, "tenant-1"), effectiveContract(t, "contract-1", "v2", "digest-2"),
	); err != nil || found {
		t.Fatalf("换版本读到了上一版的正文：found = %v err = %v", found, err)
	}
}

func TestCustomerContractInvariantsAreMirroredInTheDatabase(t *testing.T) {
	_, pool := newCustomerContractContents(t)

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.customer_contract_content
			(tenant_id, object_kind, object_id, version_label, rule_package_id)
		 VALUES ('tenant-1', 1, 'contract-9', 'v1', 'rules-1')`,
	); err == nil {
		t.Fatal("非客户合同对象进了正文表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.customer_contract_content
			(tenant_id, object_kind, object_id, version_label, rule_package_id)
		 VALUES ('tenant-1', 2, 'contract-9', 'v1', '  ')`,
	); err == nil {
		t.Fatal("空白规则包引用进了正文表")
	}

	declareContractContent(t, pool, "tenant-1", "contract-9", "v1", "rules-1")

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.customer_contract_control_binding
			(tenant_id, object_kind, object_id, version_label, charge_scope_ref, policy_id, inapplicability_basis)
		 VALUES ('tenant-1', 2, 'contract-9', 'v1', 'charge-x', NULL, NULL)`,
	); err == nil {
		t.Fatal("两列都空的绑定进了表——读回来就是一次默认放行")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.customer_contract_control_binding
			(tenant_id, object_kind, object_id, version_label, charge_scope_ref, policy_id, inapplicability_basis)
		 VALUES ('tenant-1', 2, 'contract-9', 'v1', 'charge-x', 'policy-1', 'BOTH')`,
	); err == nil {
		t.Fatal("既适用又不适用的绑定进了表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.customer_contract_control_binding
			(tenant_id, object_kind, object_id, version_label, charge_scope_ref, policy_id, inapplicability_basis)
		 VALUES ('tenant-1', 2, 'contract-9', 'v1', 'charge-x', '  ', NULL)`,
	); err == nil {
		t.Fatal("空白策略引用进了绑定表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.customer_contract_control_binding
			(tenant_id, object_kind, object_id, version_label, charge_scope_ref, policy_id, inapplicability_basis)
		 VALUES ('tenant-1', 2, 'missing', 'v1', 'charge-x', 'policy-1', NULL)`,
	); err == nil {
		t.Fatal("没有父行的绑定进了表")
	}
}

func newCustomerContractContents(t *testing.T) (*adapter.CustomerContractContents, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	contents, err := adapter.NewCustomerContractContents(db)
	if err != nil {
		t.Fatalf("构造合同正文读口：%v", err)
	}
	return contents, pool
}

func declareContractContent(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label, rulePackage string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.customer_contract_content
			(tenant_id, object_kind, object_id, version_label, rule_package_id)
		 VALUES ($1, 2, $2, $3, $4)`,
		tenant, objectID, label, rulePackage); err != nil {
		t.Fatalf("登记合同正文：%v", err)
	}
}

func declareAppliedBinding(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label, scope, policy string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.customer_contract_control_binding
			(tenant_id, object_kind, object_id, version_label, charge_scope_ref, policy_id, inapplicability_basis)
		 VALUES ($1, 2, $2, $3, $4, $5, NULL)`,
		tenant, objectID, label, scope, policy); err != nil {
		t.Fatalf("登记应用绑定：%v", err)
	}
}

func declareInapplicableBinding(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label, scope, basis string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.customer_contract_control_binding
			(tenant_id, object_kind, object_id, version_label, charge_scope_ref, policy_id, inapplicability_basis)
		 VALUES ($1, 2, $2, $3, $4, NULL, $5)`,
		tenant, objectID, label, scope, basis); err != nil {
		t.Fatalf("登记不适用绑定：%v", err)
	}
}

// contractVersionInTenant 造一份指定租户下、壳上没有指名规则包的已生效合同。
// 跨租户对抗用例需要它：两租户合法同号，壳上不指名才能让「按 A 查、用 B 重建」在缺
// 身份校验时真的装得出来——一旦壳上指名了 B 自己的规则包，F-3 会先因分歧拒掉，
// 租户漏洞就被挡住了。
func contractVersionInTenant(t *testing.T, tenant, objectID, label, digest string) domain.CommercialVersion {
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
		Kind:          domain.CustomerContractObject,
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
		t.Fatalf("重建租户 %s 的合同版本：%v", tenant, err)
	}
	return version
}
