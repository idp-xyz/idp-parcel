package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件钉 ADR-0376 决定三在商业解析上的样子（票 declared-goods-lines/11）：一个货主账户同时用两个服务产品、各配一份
// 接单规则包，合同不指名规则包——申报哪个产品就解出哪一份，不答指名引用未确认；合同指名了其中一份，另一个产品就解不出，
// 这正是多产品合同不该指名的原因。

func twoProductsUnderOneContract(t *testing.T, contractReferences map[domain.CommercialObjectKind]string) *domain.CommercialRegistry {
	t.Helper()
	registry := domain.NewCommercialRegistry()
	effectiveNaming(t, registry, domain.CustomerContractObject, fixtureContractObjectID, "v1", "sha256:contract", "scope-a",
		contractReferences)
	registerServiceProductIn(t, registry, "scope-a", "product-parcel")
	registerServiceProductIn(t, registry, "scope-a", "product-fba")
	effectiveNaming(t, registry, domain.AcceptanceRulePackageObject, "rules-parcel", "v1", "sha256:rp", "scope-a",
		map[domain.CommercialObjectKind]string{domain.ServiceProductObject: "product-parcel"})
	effectiveNaming(t, registry, domain.AcceptanceRulePackageObject, "rules-fba", "v1", "sha256:rf", "scope-a",
		map[domain.CommercialObjectKind]string{domain.ServiceProductObject: "product-fba"})
	return registry
}

// Covers: ADR-0376 决定三——合同不指名规则包，两个产品各解各的规则包，都是唯一解出。
func TestAContractNamingNoRulePackageServesEachProductWithItsOwnPackage(t *testing.T) {
	registry := twoProductsUnderOneContract(t, nil)
	for product, want := range map[string]string{"product-parcel": "rules-parcel", "product-fba": "rules-fba"} {
		closure := domain.ResolveCommercialClosure(registry, productClosureKey(t, product,
			domain.CustomerContractObject, domain.AcceptanceRulePackageObject, domain.ServiceProductObject), nil)
		if closure.Outcome() != domain.UniquelyResolved {
			t.Fatalf("%s：outcome = %q, want UNIQUELY_RESOLVED（reason=%q conflicting=%v unresolved=%v）",
				product, closure.Outcome(), closure.Reason(), closure.ConflictingBases(), closure.UnresolvedBases())
		}
		rules, _ := closure.AdoptedFor(domain.AcceptanceRulePackageObject)
		if got := rules.Version().ObjectID().String(); got != want {
			t.Fatalf("%s：采用的规则包 = %q, want %q", product, got, want)
		}
	}
}

// Covers: 合同外壳指名了某一份规则包，只作确认（ADR-0032）——指名的那个产品照旧解出，另一个产品答指名引用未确认。多产品
// 合同指名哪一份都会把另一个产品挡在外面，所以 ADR-0376 决定三让它不指名。
func TestAContractNamingOnePackageLeavesTheOtherProductUnconfirmed(t *testing.T) {
	registry := twoProductsUnderOneContract(t, map[domain.CommercialObjectKind]string{
		domain.AcceptanceRulePackageObject: "rules-parcel",
	})

	named := domain.ResolveCommercialClosure(registry, productClosureKey(t, "product-parcel",
		domain.CustomerContractObject, domain.AcceptanceRulePackageObject, domain.ServiceProductObject), nil)
	if named.Outcome() != domain.UniquelyResolved {
		t.Fatalf("指名的那个产品：outcome = %q, want UNIQUELY_RESOLVED（reason=%q）", named.Outcome(), named.Reason())
	}

	other := domain.ResolveCommercialClosure(registry, productClosureKey(t, "product-fba",
		domain.CustomerContractObject, domain.AcceptanceRulePackageObject, domain.ServiceProductObject), nil)
	if other.Outcome() != domain.ResolutionPending || other.Reason() != domain.NamedReferenceNotConfirmed {
		t.Fatalf("另一个产品：outcome = %q reason = %q, want RESOLUTION_PENDING / NamedReferenceNotConfirmed", other.Outcome(), other.Reason())
	}
}
