package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证商业关系载体目录读面(ADR-0077,票
// admin-web-page-wiring-frontier/01):客户合同上列版本壳并携正文与控制约定、
// **正文缺席与零约定可分辨**、供应商协议只上列壳、两类各读各的不串 object_kind、
// 跨租户不可见、空租户答空列表、limit 生效且非正拒。

// Covers: 上列对象是版本壳;正文与控制约定随行;`无正文` / `有正文零约定` /
// `有正文有约定` 三态在结果上各不相同——这三态正是 0012 用父子两表表达的东西。
func TestCustomerContractCatalogueSeparatesMissingContentFromEmptyBindings(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	bare := effectiveContract(t, "contract-1", "v1", "digest-bare")
	empty := effectiveContract(t, "contract-2", "v1", "digest-empty")
	bound := effectiveContract(t, "contract-3", "v1", "digest-bound")
	mustSaveVersion(t, transactor, ctx, repository, bare)
	mustSaveVersion(t, transactor, ctx, repository, empty)
	mustSaveVersion(t, transactor, ctx, repository, bound)

	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, emptyContractContentOf(t, empty))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, contractContentOf(t, bound, "control-policy-1"))
	}, ports.DeclarationSaved)

	rows, err := catalogue.ListCustomerContracts(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列客户合同:%v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("上列 %d 行,want 3(正文缺席的版本也在列)", len(rows))
	}

	// 夹具的 published_at 同刻,次序由对象标识收尾。
	if rows[0].ObjectID != "contract-1" || rows[1].ObjectID != "contract-2" || rows[2].ObjectID != "contract-3" {
		t.Fatalf("次序 = %q/%q/%q", rows[0].ObjectID, rows[1].ObjectID, rows[2].ObjectID)
	}

	// 无正文:HasContent 假、无规则包、零绑定。
	if rows[0].HasContent || rows[0].RulePackageID != "" || len(rows[0].Bindings) != 0 {
		t.Fatalf("正文未登记的版本长出了正文:hasContent=%v rulePackage=%q bindings=%d",
			rows[0].HasContent, rows[0].RulePackageID, len(rows[0].Bindings))
	}

	// 有正文零约定:HasContent 真、有规则包、零绑定。这一行与上一行都是「零绑定」,
	// 分得开它们的只有 HasContent——本用例的要害就在这一对断言上。
	if !rows[1].HasContent || rows[1].RulePackageID != "rules-1" {
		t.Fatalf("已登正文的空约定合同读成了未登记:hasContent=%v rulePackage=%q",
			rows[1].HasContent, rows[1].RulePackageID)
	}
	if len(rows[1].Bindings) != 0 {
		t.Fatalf("空约定合同长出了 %d 条约定", len(rows[1].Bindings))
	}
	if rows[1].DeclaredAt.IsZero() {
		t.Fatal("已登正文却没有登记时刻")
	}

	// 有正文有约定:指名策略与显式不适用各一条,按费用范围排序。
	bindings := rows[2].Bindings
	if len(bindings) != 2 {
		t.Fatalf("约定 %d 条,want 2", len(bindings))
	}
	if bindings[0].ChargeScope != "charge-scope-1" || bindings[0].PolicyID != "control-policy-1" ||
		bindings[0].InapplicabilityBasis != "" {
		t.Fatalf("指名策略那条变形:%#v", bindings[0])
	}
	if bindings[1].ChargeScope != "charge-scope-2" || bindings[1].PolicyID != "" ||
		bindings[1].InapplicabilityBasis != "CONTRACT-CLAUSE/NO-CONTROL" {
		t.Fatalf("显式不适用那条变形:%#v", bindings[1])
	}

	// 版本壳字段照实转写。
	shell := rows[2]
	if shell.Status != "EFFECTIVE" || shell.Scope != "scope-1" || shell.VersionLabel != "v1" {
		t.Fatalf("版本壳变形:status=%q scope=%q version=%q", shell.Status, shell.Scope, shell.VersionLabel)
	}
	if !shell.EffectiveStartsAt.Equal(effectiveAtRow) || !shell.PublishedAt.Equal(publishedAtRow) {
		t.Fatalf("时间字段变形:startsAt=%v publishedAt=%v", shell.EffectiveStartsAt, shell.PublishedAt)
	}
	if !shell.HasEffectiveEnd {
		t.Fatal("有界区间读回成了开放结束")
	}
}

// Covers: 两类目录各读各的封闭集,不串 object_kind——供应商协议不进合同目录,
// 合同也不进供应商协议目录。
func TestRelationCataloguesDoNotBleedAcrossObjectKinds(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	contract := effectiveContract(t, "contract-1", "v1", "digest-contract")
	agreement := effectiveSupplierAgreement(t, "tenant-1", "agreement-1", "v1", "digest-agreement")
	product := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-product")
	mustSaveVersion(t, transactor, ctx, repository, contract)
	mustSaveVersion(t, transactor, ctx, repository, agreement)
	mustSaveVersion(t, transactor, ctx, repository, product)

	contracts, err := catalogue.ListCustomerContracts(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列客户合同:%v", err)
	}
	if len(contracts) != 1 || contracts[0].ObjectID != "contract-1" {
		t.Fatalf("合同目录串了别的类别:%+v", contracts)
	}

	agreements, err := catalogue.ListSupplierAgreements(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列供应商协议:%v", err)
	}
	if len(agreements) != 1 || agreements[0].ObjectID != "agreement-1" {
		t.Fatalf("供应商协议目录串了别的类别:%+v", agreements)
	}
	if agreements[0].Status != "EFFECTIVE" || agreements[0].Scope != "scope-1" ||
		!agreements[0].PublishedAt.Equal(publishedAtRow) {
		t.Fatalf("协议版本壳变形:%+v", agreements[0])
	}
}

// Covers: ADR-0003——租户是最高数据隔离边界,双向不可见;从未登记过的租户答空列表
// 而不是错误(ADR-0077 Decision 四,空目录是内容)。
func TestRelationCataloguesAreTenantIsolated(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	mine := effectiveContract(t, "contract-1", "v1", "digest-mine")
	theirs := contractVersionInTenant(t, "tenant-2", "contract-1", "v1", "digest-theirs")
	myAgreement := effectiveSupplierAgreement(t, "tenant-1", "agreement-1", "v1", "digest-a1")
	theirAgreement := effectiveSupplierAgreement(t, "tenant-2", "agreement-1", "v1", "digest-a2")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveVersion(t, transactor, ctx, repository, myAgreement)
	mustSaveVersion(t, transactor, ctx, repository, theirAgreement)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, contractContentOf(t, theirs, "control-policy-9"))
	}, ports.DeclarationSaved)

	// 他租户的正文不得漏进本租户那一行——两租户共用同一个对象标识与版本号,若正文
	// 的连接条件漏了租户,这一行会带上别人的规则包而其余字段毫无异样。
	contracts, err := catalogue.ListCustomerContracts(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列本租户合同:%v", err)
	}
	if len(contracts) != 1 {
		t.Fatalf("本租户上列 %d 行,want 1", len(contracts))
	}
	if contracts[0].HasContent || len(contracts[0].Bindings) != 0 {
		t.Fatalf("他租户的正文漏进了本租户:hasContent=%v bindings=%d",
			contracts[0].HasContent, len(contracts[0].Bindings))
	}

	theirContracts, err := catalogue.ListCustomerContracts(ctx, pcTenant(t, "tenant-2"), 10)
	if err != nil {
		t.Fatalf("上列他租户合同:%v", err)
	}
	if len(theirContracts) != 1 || !theirContracts[0].HasContent || len(theirContracts[0].Bindings) != 2 {
		t.Fatalf("他租户自己的行不完整:%+v", theirContracts)
	}

	agreements, err := catalogue.ListSupplierAgreements(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列本租户协议:%v", err)
	}
	if len(agreements) != 1 {
		t.Fatalf("本租户协议上列 %d 行,want 1", len(agreements))
	}

	emptyContracts, err := catalogue.ListCustomerContracts(ctx, pcTenant(t, "tenant-9"), 10)
	if err != nil {
		t.Fatalf("空租户上列合同:%v", err)
	}
	emptyAgreements, err := catalogue.ListSupplierAgreements(ctx, pcTenant(t, "tenant-9"), 10)
	if err != nil {
		t.Fatalf("空租户上列协议:%v", err)
	}
	if len(emptyContracts) != 0 || len(emptyAgreements) != 0 {
		t.Fatalf("空租户答了 %d/%d 行,want 0/0", len(emptyContracts), len(emptyAgreements))
	}
}

// Covers: ADR-0077 Decision 五——limit 生效,非正拒(静默答一页会把缺参变成没人
// 决定过的页大小)。合同那条要连绑定一起验:GROUP BY 之后 LIMIT 数的是分组行,
// 若写成对连接后的原始行取 limit,一份多绑定的合同会自己占满一页。
func TestRelationCataloguesApplyTheLimitAndRejectNonPositive(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	first := effectiveContract(t, "contract-1", "v1", "digest-1")
	second := effectiveContract(t, "contract-2", "v1", "digest-2")
	mustSaveVersion(t, transactor, ctx, repository, first)
	mustSaveVersion(t, transactor, ctx, repository, second)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, contractContentOf(t, first, "control-policy-1"))
	}, ports.DeclarationSaved)

	limited, err := catalogue.ListCustomerContracts(ctx, tenant, 1)
	if err != nil {
		t.Fatalf("limit=1 上列合同:%v", err)
	}
	if len(limited) != 1 || len(limited[0].Bindings) != 2 {
		t.Fatalf("limit 数的不是分组行:len=%d bindings=%d", len(limited), len(limited[0].Bindings))
	}

	mustSaveVersion(t, transactor, ctx, repository,
		effectiveSupplierAgreement(t, "tenant-1", "agreement-1", "v1", "digest-a1"))
	mustSaveVersion(t, transactor, ctx, repository,
		effectiveSupplierAgreement(t, "tenant-1", "agreement-2", "v1", "digest-a2"))
	limitedAgreements, err := catalogue.ListSupplierAgreements(ctx, tenant, 1)
	if err != nil {
		t.Fatalf("limit=1 上列协议:%v", err)
	}
	if len(limitedAgreements) != 1 {
		t.Fatalf("协议 limit 未生效:len=%d", len(limitedAgreements))
	}

	for _, limit := range []int{0, -1} {
		if _, err := catalogue.ListCustomerContracts(ctx, tenant, limit); err == nil {
			t.Fatalf("合同 limit=%d 未拒", limit)
		}
		if _, err := catalogue.ListSupplierAgreements(ctx, tenant, limit); err == nil {
			t.Fatalf("协议 limit=%d 未拒", limit)
		}
	}
}

// emptyContractContentOf 造一份**已登记但零约定**的合同正文。它是本批用例的要害
// 夹具:领域允许 bindings 为空,而这一态与「正文未登记」在库上分属有父行与无父行,
// 在读面上只有 HasContent 分得开。
func emptyContractContentOf(t *testing.T, contract domain.CommercialVersion) domain.CustomerContract {
	t.Helper()
	content, err := domain.NewCustomerContract(contract,
		pcValue(t, domain.NewCommercialObjectID, "rules-1"), nil)
	if err != nil {
		t.Fatalf("组零约定合同正文:%v", err)
	}
	return content
}

// effectiveSupplierAgreement 造指定租户下`已生效`的供应商协议版本。租户要能指定,
// 理由同 productVersionInTenant:跨租户那条用例的整个前提就是两个租户共用同一个对象
// 标识与版本号。合同那一侧复用既有的 contractVersionInTenant,不另造一份。
//
// 走领域重建门而不是逐步走发布生命周期,理由同 effectiveVersionOfKind:夹具要的是
// 一份合法的已发布版本,而重建门的校验正是「什么算合法」的单一权威。
func effectiveSupplierAgreement(t *testing.T, tenant, objectID, label, digest string) domain.CommercialVersion {
	t.Helper()

	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间:%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-1"),
		pcValue(t, domain.NewCommercialSourceReference, "source-1"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据:%v", err)
	}
	version, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, tenant),
		Kind:          domain.SupplierAgreementObject,
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
		t.Fatalf("重建租户 %s 的供应商协议版本:%v", tenant, err)
	}
	return version
}

// 编译期锁缝:本文件验的就是这个端口,读面换形状时这里先红。
var _ ports.CommercialRelationCatalogueRead = (*adapter.OperationsCatalogue)(nil)
