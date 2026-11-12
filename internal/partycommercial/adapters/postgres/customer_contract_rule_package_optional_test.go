package postgres_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 证客户合同正文的规则包一格可缺（迁移 0048，ADR-0376 决定三，票 declared-goods-lines/11）：不指名
// 规则包的正文登得进、读得回「没指名」，目录行照样答正文已登记；同一版本再登一份指名了规则包的正文是内容不同，不是重放。

func contractContentNamingNoRulePackage(t *testing.T, contract domain.CommercialVersion) domain.CustomerContract {
	t.Helper()
	applied, err := domain.NewAppliedFinancialControl(
		pcValue(t, domain.NewChargeScopeReference, "charge-scope-1"),
		pcValue(t, domain.NewCommercialObjectID, "control-policy-1"))
	if err != nil {
		t.Fatalf("适用控制约定：%v", err)
	}
	content, err := domain.NewCustomerContract(contract, pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		domain.CommercialObjectID{}, []domain.FinancialControlBinding{applied})
	if err != nil {
		t.Fatalf("组合同正文：%v", err)
	}
	return content
}

func TestAContractContentNamingNoRulePackageRoundTrips(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")
	// 壳上不指名规则包的合同版本：多产品合同的常态。
	contract := effectiveDeclarationOwner(t, domain.CustomerContractObject, "contract-multi", "v1")
	mustSaveVersion(t, transactor, ctx, repository, contract)

	content := contractContentNamingNoRulePackage(t, contract)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, content)
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, content)
	}, ports.DeclarationAlreadyRegistered)

	reader, err := adapter.NewCustomerContractContents(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}
	loaded, found, err := reader.LoadCustomerContract(ctx, tenant, contract)
	if err != nil || !found {
		t.Fatalf("读回 found=%v err=%v", found, err)
	}
	if rulePackage, named := loaded.AcceptanceRulePackage(); named {
		t.Fatalf("没指名的正文读回成指名了 %q", rulePackage)
	}

	// 同一版本再登一份指名了规则包的正文：一边指名、一边没指名是内容不同。
	naming, err := domain.NewCustomerContract(contract, pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		pcValue(t, domain.NewCommercialObjectID, "rules-1"), content.Bindings())
	if err != nil {
		t.Fatalf("组合同正文：%v", err)
	}
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, naming)
	}, ports.DeclarationContentConflict)

	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读面：%v", err)
	}
	rows, err := rowsOf(catalogue.ListCustomerContracts(ctx, tenant, 10, firstPage(t, ports.CustomerContractPages)))
	if err != nil {
		t.Fatalf("上列客户合同：%v", err)
	}
	if len(rows) != 1 || !rows[0].HasContent || rows[0].RulePackageID != "" || len(rows[0].Bindings) != 1 {
		t.Fatalf("目录行 = %+v, want 正文已登记、规则包空、一条约定", rows)
	}
}

// Covers: 反方向同样是内容不同——先登了指名规则包的正文，同一版本再登一份不指名的，不能被当成重放吞掉，否则那一格的
// 指名会在重放判等时无声丢掉。
func TestAContractContentDroppingItsRulePackageIsAConflict(t *testing.T) {
	repository, transactor, _ := newDeclarationFixture(t)
	ctx := t.Context()
	contract := effectiveDeclarationOwner(t, domain.CustomerContractObject, "contract-named-first", "v1")
	mustSaveVersion(t, transactor, ctx, repository, contract)

	unnamed := contractContentNamingNoRulePackage(t, contract)
	naming, err := domain.NewCustomerContract(contract, pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		pcValue(t, domain.NewCommercialObjectID, "rules-1"), unnamed.Bindings())
	if err != nil {
		t.Fatalf("组合同正文：%v", err)
	}
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, naming)
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveCustomerContractContent(txCtx, unnamed)
	}, ports.DeclarationContentConflict)
}
