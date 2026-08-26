package postgres_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证授权规则目录上列(0013 取消授权目录,票
// admin-web-page-wiring-frontier/08):三态可分辨、不串别类对象、租户隔离、limit 生效。

// Covers: 目录壳缺席、壳在而只声明一方、壳在而两方齐全,三者在上列结果上各不相同。
//
// 中间那一态是本族的要害。领域把「缺一行」定成目录说出的真话(该请求方不许取消),
// 只有零行才是缺件——因此**空缺不能一律读成未声明**。两态在数组上都表现为「找不到
// OPERATIONS」,分得开它们的只有 HasCancellationAuthority,而恢复动作相反:未声明要
// 去登记目录,声明了不含运营方则无事可做。
func TestAuthorizationRuleCatalogueSeparatesUndeclaredFromPartyAbsent(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	bare := effectiveDeclarationOwner(t, domain.AuthorizationRuleObject, "authz-bare", "v1")
	customerOnly := effectiveDeclarationOwner(t, domain.AuthorizationRuleObject, "authz-customer", "v1")
	bothParties := effectiveDeclarationOwner(t, domain.AuthorizationRuleObject, "authz-both", "v1")
	for _, version := range []domain.CommercialVersion{bare, customerOnly, bothParties} {
		mustSaveVersion(t, transactor, ctx, repository, version)
	}

	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCancellationAuthority(txCtx,
			cancellationAuthorityOf(t, customerOnly, "CANCEL/customer-before-intake"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCancellationAuthority(txCtx, bothPartyCancellationOf(t, bothParties))
	}, ports.DeclarationSaved)

	byID := authorizationRulesByID(t, catalogue, ctx, pcTenant(t, "tenant-1"), 10)

	if row := byID["authz-bare"]; row.HasCancellationAuthority || len(row.CancellationAuthorities) != 0 {
		t.Fatalf("没登记过取消授权目录的授权规则答出了目录:%+v", row)
	}

	single := byID["authz-customer"]
	if !single.HasCancellationAuthority {
		t.Fatal("已登记的取消授权目录被答成未声明——这两态的恢复动作相反")
	}
	if len(single.CancellationAuthorities) != 1 ||
		single.CancellationAuthorities[0].Party != "CUSTOMER" ||
		single.CancellationAuthorities[0].RuleReference != "CANCEL/customer-before-intake" {
		t.Fatalf("单方目录变形:%+v", single.CancellationAuthorities)
	}
	if single.DeclaredAt.IsZero() {
		t.Fatal("目录在场却没有声明时间")
	}

	both := byID["authz-both"]
	if !both.HasCancellationAuthority || len(both.CancellationAuthorities) != 2 {
		t.Fatalf("两方目录变形:%+v", both)
	}
	if both.CancellationAuthorities[0].Party != "CUSTOMER" ||
		both.CancellationAuthorities[1].Party != "OPERATIONS" {
		t.Fatalf("请求方次序不稳:%+v", both.CancellationAuthorities)
	}
	if both.CancellationAuthorities[1].RuleReference != "CANCEL/operations-any-time" {
		t.Fatalf("运营方规则引用变形:%+v", both.CancellationAuthorities[1])
	}
}

// Covers: 目录只收 object_kind=9,别类商业对象连同它们各自的正文都不串进来。
//
// 同名不同类是这条的取证形状:三个对象共用一个标识与版本号,只有授权规则那份该在列。
// 相关子查询若漏了 object_kind 这一列,别类对象的子行会顺着标识跟过来。
func TestAuthorizationRuleCatalogueDoesNotBleedAcrossObjectKinds(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	authorization := effectiveDeclarationOwner(t, domain.AuthorizationRuleObject, "shared-id", "v1")
	rulePackage := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "shared-id", "v1")
	product := effectiveDeclarationOwner(t, domain.ServiceProductObject, "shared-id", "v1")
	for _, version := range []domain.CommercialVersion{authorization, rulePackage, product} {
		mustSaveVersion(t, transactor, ctx, repository, version)
	}
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCancellationAuthority(txCtx,
			cancellationAuthorityOf(t, authorization, "CANCEL/customer-before-intake"))
	}, ports.DeclarationSaved)

	rows, err := catalogue.ListAuthorizationRules(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列授权规则:%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("上列 %d 行,want 1(别类对象串进授权规则目录)", len(rows))
	}
	if !rows[0].HasCancellationAuthority || len(rows[0].CancellationAuthorities) != 1 {
		t.Fatalf("授权规则自己的目录反倒没读回:%+v", rows[0])
	}

	// 反向也要证一次:授权规则不该出现在服务产品目录里。
	products, err := catalogue.ListServiceProducts(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列服务产品:%v", err)
	}
	if len(products) != 1 {
		t.Fatalf("服务产品目录 %d 行,want 1", len(products))
	}
}

// Covers: 授权规则目录按租户作答——同标识同版本的另一租户对象与它的目录都不越界。
func TestAuthorizationRuleCatalogueIsTenantIsolated(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	mine := authorizationRuleInTenant(t, "tenant-1", "authz-shared", "v1", "digest-mine")
	theirs := authorizationRuleInTenant(t, "tenant-2", "authz-shared", "v1", "digest-theirs")
	for _, version := range []domain.CommercialVersion{mine, theirs} {
		mustSaveVersion(t, transactor, ctx, repository, version)
	}
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCancellationAuthority(txCtx,
			cancellationAuthorityOf(t, theirs, "CANCEL/theirs-only"))
	}, ports.DeclarationSaved)

	rows, err := catalogue.ListAuthorizationRules(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列授权规则:%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("上列 %d 行,want 1", len(rows))
	}
	// 本租户那份没登过目录;邻租户同标识那份登过。
	//
	// 两条都要断言,少一条都网不住:壳的在场走 LEFT JOIN、请求方声明走相关子查询,
	// 两处各带自己的租户条件。只看布尔时,子查询漏掉租户列会让邻租户的声明照样跟
	// 过来而布尔仍是假——那正是最早这条用例放过去的漏子。
	if rows[0].HasCancellationAuthority {
		t.Fatal("读到了邻租户的取消授权目录壳")
	}
	if len(rows[0].CancellationAuthorities) != 0 {
		t.Fatalf("读到了邻租户的取消授权声明:%+v", rows[0].CancellationAuthorities)
	}
}

// Covers: limit 截断生效,非正 limit 拒答(判据同其余目录读口)。
func TestAuthorizationRuleCatalogueAppliesTheLimitAndRejectsNonPositive(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	for _, objectID := range []string{"authz-a", "authz-b"} {
		mustSaveVersion(t, transactor, ctx, repository,
			effectiveDeclarationOwner(t, domain.AuthorizationRuleObject, objectID, "v1"))
	}

	limited, err := catalogue.ListAuthorizationRules(ctx, tenant, 1)
	if err != nil {
		t.Fatalf("上列授权规则:%v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limit 未生效:len=%d", len(limited))
	}
	for _, limit := range []int{0, -1} {
		if _, err := catalogue.ListAuthorizationRules(ctx, tenant, limit); err == nil {
			t.Fatalf("limit=%d 未拒", limit)
		}
	}
}

func authorizationRulesByID(
	t *testing.T,
	catalogue *adapter.OperationsCatalogue,
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) map[string]ports.AuthorizationRuleRow {
	t.Helper()
	rows, err := catalogue.ListAuthorizationRules(ctx, tenant, limit)
	if err != nil {
		t.Fatalf("上列授权规则:%v", err)
	}
	byID := make(map[string]ports.AuthorizationRuleRow, len(rows))
	for _, row := range rows {
		byID[row.ObjectID] = row
	}
	return byID
}

// bothPartyCancellationOf 造一份两个请求方格都在场的目录。既有的
// cancellationAuthorityOf 只声明客户方,而本批用例要靠「两方齐全」与「只有客户方」
// 的对照来说明缺行是真话而非缺件。
func bothPartyCancellationOf(t *testing.T, owner domain.CommercialVersion) domain.CancellationAuthorityContent {
	t.Helper()
	content, err := domain.NewCancellationAuthorityContent(owner, []domain.CancellationAuthorityDeclaration{
		{
			Party: domain.DeclaredCustomerCancellation,
			Rule:  pcValue(t, domain.NewRuleReference, "CANCEL/customer-before-intake"),
		},
		{
			Party: domain.DeclaredOperationsCancellation,
			Rule:  pcValue(t, domain.NewRuleReference, "CANCEL/operations-any-time"),
		},
	})
	if err != nil {
		t.Fatalf("组两方取消授权目录:%v", err)
	}
	return content
}

// 编译期锁缝:本文件验的是这个端口的授权规则那一格,读面换形状时这里先红。
var _ ports.CommercialPolicyCatalogueRead = (*adapter.OperationsCatalogue)(nil)
