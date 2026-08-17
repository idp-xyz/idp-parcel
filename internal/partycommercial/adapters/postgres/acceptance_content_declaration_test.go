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

// 本文件对真实 PostgreSQL 16 证接受内容声明读口。重点在**两个方法的 found=false 语义相反**：
// 规则包那半边的 false 是「未配置」（消费方停在未决），服务产品那半边的 false 是「未许可」
// （零值语义，照常推进只是不走待路由）。两者若被读成同一件事，一个会默认放行、一个会
// 无谓卡住。

func TestAnUndeclaredRulePackageIsNotConfigured(t *testing.T) {
	declarations, _ := newAcceptanceContentDeclarations(t)
	rulePackage := effectiveRulePackage(t, "rules-1", "v1")

	content, found, err := declarations.LoadAcceptanceRuleContent(
		t.Context(), pcTenant(t, "tenant-1"), rulePackage)
	if err != nil {
		t.Fatalf("未配置被当成错误：%v", err)
	}
	if found {
		t.Fatal("查无声明却答已配置")
	}
	if len(content.ApplicableGroups()) != 0 || content.ManualReview().Declared() {
		t.Fatal("未配置的答复带了内容——那等于替租户拟了一份声明")
	}
}

// TestDeclaredAcceptanceContentRoundTrips 证声明保真：组集合逐项回来且顺序稳定，人工复核
// 指令按声明值回来（不是按默认值）。
func TestDeclaredAcceptanceContentRoundTrips(t *testing.T) {
	declarations, pool := newAcceptanceContentDeclarations(t)
	rulePackage := effectiveRulePackage(t, "rules-1", "v1")

	declareRuleContent(t, pool, "tenant-1", "rules-1", "v1", "REQUIRED",
		"NETWORK_REACHABILITY", "CUSTOMER_RELATIONSHIP")

	content, found, err := declarations.LoadAcceptanceRuleContent(
		t.Context(), pcTenant(t, "tenant-1"), rulePackage)
	if err != nil || !found {
		t.Fatalf("读回：found = %v err = %v", found, err)
	}
	if content.ManualReview() != domain.ManualReviewRequired {
		t.Fatalf("manual review = %q，声明说 REQUIRED", content.ManualReview())
	}
	groups := content.ApplicableGroups()
	if len(groups) != 2 ||
		groups[0] != domain.CustomerRelationshipCheckGroup ||
		groups[1] != domain.NetworkReachabilityCheckGroup {
		t.Fatalf("groups = %v；应按组名稳定排序且逐项保真", groups)
	}
	if !content.Applies(domain.NetworkReachabilityCheckGroup) ||
		content.Applies(domain.MemberBaselineCheckGroup) {
		t.Fatal("适用性判反了：没声明的组不该适用")
	}
}

// TestAnUnpermittedProductIsNotUndecided 证服务产品那半边的 false 是「未许可」而不是
// 「未决」：待路由是例外许可，没有声明就是没有许可。
func TestAnUnpermittedProductIsNotUndecided(t *testing.T) {
	declarations, pool := newAcceptanceContentDeclarations(t)
	product := effectiveServiceProduct(t, "product-1", "v1")

	permission, found, err := declarations.LoadPendingRoutingPermission(
		t.Context(), pcTenant(t, "tenant-1"), product)
	if err != nil {
		t.Fatalf("未许可被当成错误——那会让消费方停在未决：%v", err)
	}
	if found || permission.Allowed() {
		t.Fatal("没有声明却答允许待路由")
	}

	declarePendingRouting(t, pool, "tenant-1", "product-1", "v1", "PENDING-ROUTING/CONTRACT-7")
	permission, found, err = declarations.LoadPendingRoutingPermission(
		t.Context(), pcTenant(t, "tenant-1"), product)
	if err != nil || !found {
		t.Fatalf("读回：found = %v err = %v", found, err)
	}
	// 依据是许可的要害：没有依据的许可与一次默认放行分不开，消费方要保存的正是这条引用。
	if !permission.Allowed() || permission.Basis().String() != "PENDING-ROUTING/CONTRACT-7" {
		t.Fatalf("permission = %#v；许可必须带依据", permission)
	}
}

// TestAcceptanceDeclarationsAreScopedByTenantAndVersion 证圈定：他租户与同对象另一版本
// 都读不到本版本的声明——换版本就是换一份正文。
func TestAcceptanceDeclarationsAreScopedByTenantAndVersion(t *testing.T) {
	declarations, pool := newAcceptanceContentDeclarations(t)
	declareRuleContent(t, pool, "tenant-1", "rules-1", "v1", "NOT_REQUIRED", "MEMBER_BASELINE")

	if _, found, err := declarations.LoadAcceptanceRuleContent(
		t.Context(), pcTenant(t, "tenant-b"), effectiveRulePackage(t, "rules-1", "v1"),
	); err != nil || found {
		t.Fatalf("他租户读到了本租户的声明：found = %v err = %v", found, err)
	}
	if _, found, err := declarations.LoadAcceptanceRuleContent(
		t.Context(), pcTenant(t, "tenant-1"), effectiveRulePackage(t, "rules-1", "v2"),
	); err != nil || found {
		t.Fatalf("换版本读到了上一版的声明：found = %v err = %v", found, err)
	}
}

// TestAcceptanceContentInvariantsAreMirroredInTheDatabase 证库内守住三条：复核指令封闭集、
// 校验组封闭集、以及**许可必须带依据**——最后一条是分表的理由之一，可空的依据列会让一行
// 无依据的许可存下来，而那一行读回来就是一次默认放行。
func TestAcceptanceContentInvariantsAreMirroredInTheDatabase(t *testing.T) {
	_, pool := newAcceptanceContentDeclarations(t)

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.acceptance_rule_content
			(tenant_id, object_kind, object_id, version_label, manual_review)
		 VALUES ('tenant-1', 4, 'rules-9', 'v1', 'MAYBE')`,
	); err == nil {
		t.Fatal("集合外的人工复核指令进了声明表")
	}

	declareRuleContent(t, pool, "tenant-1", "rules-1", "v1", "REQUIRED", "MEMBER_BASELINE")
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.acceptance_rule_check_group
			(tenant_id, object_kind, object_id, version_label, check_group)
		 VALUES ('tenant-1', 4, 'rules-1', 'v1', 'SOMETHING_ELSE')`,
	); err == nil {
		t.Fatal("集合外的校验组进了声明表")
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.pending_routing_permission
			(tenant_id, object_kind, object_id, version_label, basis_ref)
		 VALUES ('tenant-1', 1, 'product-1', 'v1', '')`,
	); err == nil {
		t.Fatal("一条没有依据的待路由许可进了表")
	}
}

func newAcceptanceContentDeclarations(t *testing.T) (*adapter.AcceptanceContentDeclarations, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	declarations, err := adapter.NewAcceptanceContentDeclarations(db)
	if err != nil {
		t.Fatalf("构造接受内容声明读口：%v", err)
	}
	return declarations, pool
}

// effectiveServiceProduct 造一份已生效服务产品。与 effectiveVersionOfKind 分开的理由同
// effectiveRulePackage：那个夹具挂的声明引用在这里没有意义。
func effectiveServiceProduct(t *testing.T, objectID, label string) domain.CommercialVersion {
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
		Kind:          domain.ServiceProductObject,
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
		t.Fatalf("重建服务产品版本：%v", err)
	}
	return version
}

func declareRuleContent(
	t *testing.T,
	pool *pgxpool.Pool,
	tenant, objectID, label, directive string,
	groups ...string,
) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.acceptance_rule_content
			(tenant_id, object_kind, object_id, version_label, manual_review)
		 VALUES ($1, 4, $2, $3, $4)`,
		tenant, objectID, label, directive); err != nil {
		t.Fatalf("登记接受内容：%v", err)
	}
	for _, group := range groups {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO party_commercial.acceptance_rule_check_group
				(tenant_id, object_kind, object_id, version_label, check_group)
			 VALUES ($1, 4, $2, $3, $4)`,
			tenant, objectID, label, group); err != nil {
			t.Fatalf("登记适用校验组：%v", err)
		}
	}
}

func declarePendingRouting(t *testing.T, pool *pgxpool.Pool, tenant, objectID, label, basis string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO party_commercial.pending_routing_permission
			(tenant_id, object_kind, object_id, version_label, basis_ref)
		 VALUES ($1, 1, $2, $3, $4)`,
		tenant, objectID, label, basis); err != nil {
		t.Fatalf("登记待路由许可：%v", err)
	}
}
