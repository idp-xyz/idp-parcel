package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证两张目录行上可缺的那一节交付条件（0030；票 admin-write-faces/25 裁读回折进既有目录行）：
// 服务产品行携产品层、客户合同行携合同层（连同所收紧的产品版本）、没声明的版本那一格为 nil 而不是零值；方式按引用字面
// 稳定序；他租户同号版本的声明渗不进来；交付条件那一层与 0012 正文那一层各自可缺、互不推断。夹具全部合成。

// Covers: 服务产品目录行——声明过产品层的版本 DeliveryConditions 在场（三种方式稳定序、两条规则引用、声明时刻、无所收紧
// 的产品版本），没声明的版本为 nil；他租户同号版本的产品层不进本租户的行。
func TestServiceProductCatalogueCarriesTheProductLayerDeliveryConditions(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	declared := effectiveServiceProductVersion(t, "product-1", "v1", "digest-p1")
	bare := effectiveServiceProductVersion(t, "product-2", "v1", "digest-p2")
	foreign := productVersionInTenant(t, "tenant-2", "product-1", "v1", "digest-foreign")
	mustSaveVersion(t, transactor, ctx, repository, declared)
	mustSaveVersion(t, transactor, ctx, repository, bare)
	mustSaveVersion(t, transactor, ctx, repository, foreign)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, productDeliveryConditionsOf(t, declared, "method-c", "method-a", "method-b"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, productDeliveryConditionsOf(t, foreign, "method-z"))
	}, ports.DeclarationSaved)

	rows, err := catalogue.ListServiceProducts(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列服务产品：%v", err)
	}
	if len(rows) != 2 || rows[0].ObjectID != "product-1" || rows[1].ObjectID != "product-2" {
		t.Fatalf("上列 = %d 行 %q/%q，want product-1 / product-2", len(rows), rows[0].ObjectID, rows[1].ObjectID)
	}

	conditions := rows[0].DeliveryConditions
	if conditions == nil {
		t.Fatal("声明过产品层的版本那一格为 nil")
	}
	if len(conditions.Methods) != 3 || conditions.Methods[0] != "method-a" || conditions.Methods[1] != "method-b" || conditions.Methods[2] != "method-c" {
		t.Fatalf("方式 = %v，want 按字面稳定序三种", conditions.Methods)
	}
	if conditions.RecipientScopeRule != "RULE/recipient-scope" || conditions.ProofOfDeliveryRule != "RULE/proof-of-delivery" {
		t.Fatalf("规则引用 = %q / %q", conditions.RecipientScopeRule, conditions.ProofOfDeliveryRule)
	}
	if conditions.TightensObjectID != "" || conditions.TightensVersion != "" {
		t.Fatalf("产品层长出了所收紧的产品版本：%q/%q", conditions.TightensObjectID, conditions.TightensVersion)
	}
	if conditions.DeclaredAt.IsZero() {
		t.Fatal("已登声明却没有声明时刻")
	}
	if rows[1].DeliveryConditions != nil {
		t.Fatalf("没声明的版本长出了交付条件：%#v", rows[1].DeliveryConditions)
	}
}

// Covers: 客户合同目录行——合同层随行并带所收紧的产品版本；交付条件与 0012 正文是两层各自可缺：只登正文不登交付条件的
// 版本那一格为 nil、只登交付条件不登正文的版本 HasContent 为假而那一格在场；他租户不可见。
func TestCustomerContractCatalogueCarriesTheContractLayerDeliveryConditions(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	product := effectiveServiceProductVersion(t, "product-1", "v1", "digest-p1")
	withBoth := effectiveContract(t, "contract-1", "v1", "digest-both")
	contentOnly := effectiveContract(t, "contract-2", "v1", "digest-content")
	conditionsOnly := effectiveContract(t, "contract-3", "v1", "digest-conditions")
	mustSaveVersion(t, transactor, ctx, repository, product)
	mustSaveVersion(t, transactor, ctx, repository, withBoth)
	mustSaveVersion(t, transactor, ctx, repository, contentOnly)
	mustSaveVersion(t, transactor, ctx, repository, conditionsOnly)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, productDeliveryConditionsOf(t, product, "method-a", "method-b"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, contractContentOf(t, withBoth, "control-policy-1"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, contractDeliveryConditionsOf(t, withBoth, "product-1", "v1", "method-b"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, contractContentOf(t, contentOnly, "control-policy-1"))
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveDeliveryConditions(txCtx, contractDeliveryConditionsOf(t, conditionsOnly, "product-1", "v1", "method-a"))
	}, ports.DeclarationSaved)

	rows, err := catalogue.ListCustomerContracts(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列客户合同：%v", err)
	}
	if len(rows) != 3 || rows[0].ObjectID != "contract-1" || rows[1].ObjectID != "contract-2" || rows[2].ObjectID != "contract-3" {
		t.Fatalf("上列 = %d 行，次序或数量不对", len(rows))
	}

	both := rows[0]
	if !both.HasContent || len(both.Bindings) != 2 {
		t.Fatalf("正文那一层丢了：hasContent=%v bindings=%d", both.HasContent, len(both.Bindings))
	}
	if both.DeliveryConditions == nil {
		t.Fatal("声明过合同层的版本那一格为 nil")
	}
	if both.DeliveryConditions.TightensObjectID != "product-1" || both.DeliveryConditions.TightensVersion != "v1" {
		t.Fatalf("合同层没带所收紧的产品版本：%q/%q", both.DeliveryConditions.TightensObjectID, both.DeliveryConditions.TightensVersion)
	}
	if len(both.DeliveryConditions.Methods) != 1 || both.DeliveryConditions.Methods[0] != "method-b" {
		t.Fatalf("合同层方式 = %v，want [method-b]", both.DeliveryConditions.Methods)
	}

	if !rows[1].HasContent || rows[1].DeliveryConditions != nil {
		t.Fatalf("只登正文的版本：hasContent=%v deliveryConditions=%#v", rows[1].HasContent, rows[1].DeliveryConditions)
	}
	if rows[2].HasContent || rows[2].DeliveryConditions == nil || rows[2].DeliveryConditions.Methods[0] != "method-a" {
		t.Fatalf("只登交付条件的版本：hasContent=%v deliveryConditions=%#v", rows[2].HasContent, rows[2].DeliveryConditions)
	}

	// 正文那一层的约定行与交付条件的方式行并存时，各自不被对方数重（相关子查询各聚各的）。
	if len(both.Bindings) != 2 || len(both.DeliveryConditions.Methods) != 1 {
		t.Fatalf("两族子表互相扇出：bindings=%d methods=%d", len(both.Bindings), len(both.DeliveryConditions.Methods))
	}

	foreign, err := catalogue.ListCustomerContracts(ctx, pcTenant(t, "tenant-2"), 10)
	if err != nil {
		t.Fatalf("他租户上列：%v", err)
	}
	if len(foreign) != 0 {
		t.Fatalf("他租户看见了 %d 行", len(foreign))
	}
}
