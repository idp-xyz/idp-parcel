package postgres_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证时点锚声明读口：没声明即空声明（不是错误）、声明保真且
// 顺序稳定、租户与规则包版本各自圈定、库内 CHECK 镜像领域封闭集。
//
// 写入面不在这里：真实语义与政策版本属 `PAR-COM-14` 实例半边，本适配器只读，测试里直插
// 演的是「日后真有声明可登之后」的那一刻。

func TestAnUndeclaredRulePackageYieldsAnEmptyDeclaration(t *testing.T) {
	declarations, _ := newAsOfPolicyDeclarations(t)
	rulePackage := effectiveRulePackage(t, "rules-1", "v1")

	policies, err := declarations.LoadAsOfPolicies(t.Context(), pcTenant(t, "tenant-1"), rulePackage)
	if err != nil {
		t.Fatalf("没声明被当成了错误：%v", err)
	}
	if len(policies) != 0 {
		t.Fatalf("policies = %d, want 0", len(policies))
	}
	// 空声明经领域译`未配置`——首发唯一走得到的真实分支，编排据此停下而不是绕过。
	if _, err := domain.DeclareAsOfPolicies(rulePackage, policies); err == nil {
		t.Fatal("空声明被领域收下了")
	}
}

// TestDeclaredPoliciesRoundTripInAStableOrder 证声明保真：判断类型、语义引用与政策版本
// 三项一个都不能丢——少了政策版本，回显就证不出这个时点锚出自哪条策略。
func TestDeclaredPoliciesRoundTripInAStableOrder(t *testing.T) {
	declarations, pool := newAsOfPolicyDeclarations(t)
	rulePackage := effectiveRulePackage(t, "rules-1", "v1")

	declarePolicy(t, pool, "tenant-1", "rules-1", "v1",
		"PRE_ACCEPTANCE_FINANCIAL_CONTROL", "AT_SUBMISSION", "asof-policy/v3")
	declarePolicy(t, pool, "tenant-1", "rules-1", "v1",
		"NETWORK_REACHABILITY", "AT_ACCEPTANCE", "asof-policy/v2")

	policies, err := declarations.LoadAsOfPolicies(t.Context(), pcTenant(t, "tenant-1"), rulePackage)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if len(policies) != 2 {
		t.Fatalf("policies = %d, want 2", len(policies))
	}
	// 按 judgment_type 排序：NETWORK_REACHABILITY 在前，与登记先后无关。
	if policies[0].Judgment() != domain.NetworkReachabilityJudgment ||
		policies[0].Semantics().String() != "AT_ACCEPTANCE" ||
		policies[0].PolicyVersion().String() != "asof-policy/v2" {
		t.Fatalf("首条 = %#v", policies[0])
	}
	if policies[1].Judgment() != domain.PreAcceptanceFinancialControlJudgment ||
		policies[1].PolicyVersion().String() != "asof-policy/v3" {
		t.Fatalf("次条 = %#v", policies[1])
	}

	declaration, err := domain.DeclareAsOfPolicies(rulePackage, policies)
	if err != nil {
		t.Fatalf("读回的声明领域收不下：%v", err)
	}
	if _, found := declaration.PolicyFor(domain.NetworkReachabilityJudgment); !found {
		t.Fatal("读回的声明查不到已登记的判断类型")
	}
}

// TestDeclarationsAreScopedByTenantAndRulePackageVersion 证圈定：他租户的声明读不到，
// 同一对象另一个版本号的声明也不串——规则包换版本就是换一份声明，混起来会让一次判断
// 采用另一版的时点锚。
func TestDeclarationsAreScopedByTenantAndRulePackageVersion(t *testing.T) {
	declarations, pool := newAsOfPolicyDeclarations(t)
	declarePolicy(t, pool, "tenant-1", "rules-1", "v1",
		"NETWORK_REACHABILITY", "AT_ACCEPTANCE", "asof-policy/v2")

	t.Run("他租户", func(t *testing.T) {
		rulePackage := effectiveRulePackage(t, "rules-1", "v1")
		policies, err := declarations.LoadAsOfPolicies(t.Context(), pcTenant(t, "tenant-b"), rulePackage)
		if err != nil || len(policies) != 0 {
			t.Fatalf("policies = %d err = %v；他租户读到了本租户的声明", len(policies), err)
		}
	})

	t.Run("同对象另一版本", func(t *testing.T) {
		other := effectiveRulePackage(t, "rules-1", "v2")
		policies, err := declarations.LoadAsOfPolicies(t.Context(), pcTenant(t, "tenant-1"), other)
		if err != nil || len(policies) != 0 {
			t.Fatalf("policies = %d err = %v；换版本读到了上一版的时点锚", len(policies), err)
		}
	})
}

// TestTheJudgmentSetIsMirroredInTheDatabase 证两处封闭集不许分叉：库内 CHECK 拒绝领域
// 集合之外的判断类型，同一判断也不许声明两条（取哪个都是掷硬币）。
func TestTheJudgmentSetIsMirroredInTheDatabase(t *testing.T) {
	_, pool := newAsOfPolicyDeclarations(t)

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.as_of_policy_declaration
			(tenant_id, object_kind, object_id, version_label, judgment_type, semantics_ref, policy_version)
		 VALUES ('tenant-1', 4, 'rules-1', 'v1', 'SOMETHING_ELSE', 'AT_ACCEPTANCE', 'asof-policy/v2')`,
	); err == nil {
		t.Fatal("集合外的判断类型进了声明表")
	}

	declarePolicy(t, pool, "tenant-1", "rules-1", "v1",
		"NETWORK_REACHABILITY", "AT_ACCEPTANCE", "asof-policy/v2")
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.as_of_policy_declaration
			(tenant_id, object_kind, object_id, version_label, judgment_type, semantics_ref, policy_version)
		 VALUES ('tenant-1', 4, 'rules-1', 'v1', 'NETWORK_REACHABILITY', 'AT_SUBMISSION', 'asof-policy/v9')`,
	); err == nil {
		t.Fatal("同一判断的第二条时点锚进了声明表")
	}
}

// effectiveRulePackage 造一份已生效接单规则包。不复用本包的 effectiveVersionOfKind：
// 那个夹具给版本挂了一条指向接单规则包的声明引用，而本对象自己就是接单规则包，套上去
// 就成了自引用，重建门会当场拒掉。
func effectiveRulePackage(t *testing.T, objectID, label string) domain.CommercialVersion {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-"+objectID),
		pcValue(t, domain.NewCommercialSourceReference, "source-"+objectID),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, "tenant-1"),
		Kind:          domain.AcceptanceRulePackageObject,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "digest-"+objectID+"-"+label),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   effectiveAtRow,
	})
	if err != nil {
		t.Fatalf("重建接单规则包版本：%v", err)
	}
	return version
}

func newAsOfPolicyDeclarations(t *testing.T) (*adapter.AsOfPolicyDeclarations, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	declarations, err := adapter.NewAsOfPolicyDeclarations(db)
	if err != nil {
		t.Fatalf("构造时点锚声明读口：%v", err)
	}
	return declarations, pool
}

func declarePolicy(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label, judgment, semantics, policyVersion string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.as_of_policy_declaration
			(tenant_id, object_kind, object_id, version_label, judgment_type, semantics_ref, policy_version)
		 VALUES ($1, 4, $2, $3, $4, $5, $6)`,
		tenant, objectID, label, judgment, semantics, policyVersion); err != nil {
		t.Fatalf("登记时点锚：%v", err)
	}
}
