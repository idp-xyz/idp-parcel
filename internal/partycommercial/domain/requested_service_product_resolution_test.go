package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件钉的是票 psb/17：委托声明的服务产品参与闭包解析。同一范围里挂两个产品是常规形态（一份合同覆盖快递与
// 经济两条产品线），解析键只有范围时，服务产品那一项必然`适用冲突`。

func productClosureKey(t *testing.T, declared string, required ...domain.CommercialObjectKind) domain.ClosureResolutionKey {
	t.Helper()
	key := closureKey(t, "scope-a", required...)
	if declared != "" {
		key.ServiceProduct = commercialValue(t, domain.NewCommercialObjectID, declared)
	}
	return key
}

func twoProductsInOneScope(t *testing.T) *domain.CommercialRegistry {
	t.Helper()
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", domain.CustomerContractObject)
	registerServiceProductIn(t, registry, "scope-a", "product-express")
	registerServiceProductIn(t, registry, "scope-a", "product-econ")
	return registry
}

func TestADeclaredServiceProductNarrowsTwoProductsInOneScopeToTheDeclaredOne(t *testing.T) {
	registry := twoProductsInOneScope(t)

	closure := domain.ResolveCommercialClosure(registry,
		productClosureKey(t, "product-express", domain.CustomerContractObject, domain.ServiceProductObject), nil)

	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（conflicting=%v reason=%q）",
			closure.Outcome(), closure.ConflictingBases(), closure.Reason())
	}
	adopted, present := closure.AdoptedFor(domain.ServiceProductObject)
	if !present || adopted.Version().ObjectID().String() != "product-express" {
		t.Fatalf("采用的服务产品 = %v（在场 %v），want product-express", adopted.Version().ObjectID(), present)
	}
}

// 不声明时照旧按范围解：两个候选就是`适用冲突`，不补一个默认产品。
func TestWithoutADeclaredServiceProductTwoProductsInOneScopeStillConflict(t *testing.T) {
	registry := twoProductsInOneScope(t)

	closure := domain.ResolveCommercialClosure(registry,
		productClosureKey(t, "", domain.CustomerContractObject, domain.ServiceProductObject), nil)

	if closure.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", closure.Outcome())
	}
	conflicting := closure.ConflictingBases()
	if len(conflicting) != 1 || conflicting[0] != domain.ServiceProductObject {
		t.Fatalf("conflicting = %v, want [SERVICE_PRODUCT]", conflicting)
	}
}

func TestADeclaredServiceProductWithoutAPublishedVersionIsNoApplicableBasis(t *testing.T) {
	registry := twoProductsInOneScope(t)

	closure := domain.ResolveCommercialClosure(registry,
		productClosureKey(t, "product-missing", domain.CustomerContractObject, domain.ServiceProductObject), nil)

	if closure.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS", closure.Outcome())
	}
	unresolved := closure.UnresolvedBases()
	if len(unresolved) != 1 || unresolved[0] != domain.ServiceProductObject {
		t.Fatalf("unresolved = %v, want [SERVICE_PRODUCT]", unresolved)
	}
}

// 两个产品各配一份接单规则包：只收窄服务产品而不收窄规则包，规则包那一项仍会冲突。
func TestRulePackagesNamingAnotherServiceProductDropOut(t *testing.T) {
	registry := twoProductsInOneScope(t)
	effectiveNaming(t, registry, domain.AcceptanceRulePackageObject, "rules-express", "v1", "sha256:rx", "scope-a",
		map[domain.CommercialObjectKind]string{domain.ServiceProductObject: "product-express"})
	effectiveNaming(t, registry, domain.AcceptanceRulePackageObject, "rules-econ", "v1", "sha256:re", "scope-a",
		map[domain.CommercialObjectKind]string{domain.ServiceProductObject: "product-econ"})

	closure := domain.ResolveCommercialClosure(registry, productClosureKey(t, "product-econ",
		domain.CustomerContractObject, domain.AcceptanceRulePackageObject, domain.ServiceProductObject), nil)

	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（conflicting=%v reason=%q）",
			closure.Outcome(), closure.ConflictingBases(), closure.Reason())
	}
	rules, _ := closure.AdoptedFor(domain.AcceptanceRulePackageObject)
	if rules.Version().ObjectID().String() != "rules-econ" {
		t.Fatalf("采用的规则包 = %q, want rules-econ", rules.Version().ObjectID())
	}
}

// 正文没指名服务产品的对象与产品无关，声明了产品也照旧参选。
func TestARulePackageNamingNoServiceProductStaysACandidate(t *testing.T) {
	registry := twoProductsInOneScope(t)
	effectiveIn(t, registry, domain.AcceptanceRulePackageObject, "rules-generic", "v1", "sha256:rg", "scope-a")

	closure := domain.ResolveCommercialClosure(registry, productClosureKey(t, "product-express",
		domain.CustomerContractObject, domain.AcceptanceRulePackageObject, domain.ServiceProductObject), nil)

	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（unresolved=%v reason=%q）",
			closure.Outcome(), closure.UnresolvedBases(), closure.Reason())
	}
}

// 演示种子那一格：规则包只配给快递，委托声明经济。规则包那一项没有候选，答`无适用依据`——不是未决，也不借用
// 快递那份规则。
func TestADeclaredProductWithoutItsOwnRulePackageIsNoApplicableBasis(t *testing.T) {
	registry := twoProductsInOneScope(t)
	effectiveNaming(t, registry, domain.AcceptanceRulePackageObject, "rules-express", "v1", "sha256:rx", "scope-a",
		map[domain.CommercialObjectKind]string{domain.ServiceProductObject: "product-express"})

	closure := domain.ResolveCommercialClosure(registry, productClosureKey(t, "product-econ",
		domain.CustomerContractObject, domain.AcceptanceRulePackageObject, domain.ServiceProductObject), nil)

	if closure.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS（reason=%q）", closure.Outcome(), closure.Reason())
	}
	unresolved := closure.UnresolvedBases()
	if len(unresolved) != 1 || unresolved[0] != domain.AcceptanceRulePackageObject {
		t.Fatalf("unresolved = %v, want [ACCEPTANCE_RULE_PACKAGE]", unresolved)
	}
}

// 收窄一个本次不解的产品无从谈起：键上带着声明的产品却不要服务产品依据，是一个立不起来的键。
func TestAClosureKeyCarryingAServiceProductWithoutRequestingOneIsNotAccepted(t *testing.T) {
	registry := twoProductsInOneScope(t)

	key := productClosureKey(t, "product-express", domain.CustomerContractObject)
	if key.MinimumIdentityEstablished() {
		t.Fatal("不要服务产品依据的闭包键带着声明的产品，最小身份却成立")
	}
	if closure := domain.ResolveCommercialClosure(registry, key, nil); closure.Outcome() != domain.InputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", closure.Outcome())
	}
}

// 声明的产品是请求的一维：同一范围只有一个产品时，声明与不声明都唯一解出，但它们是两次不同的请求，身份不同。
func TestTheDeclaredServiceProductIsPartOfTheClosureIdentity(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", domain.CustomerContractObject)
	registerServiceProductIn(t, registry, "scope-a", "product-express")

	undeclared := domain.ResolveCommercialClosure(registry,
		productClosureKey(t, "", domain.CustomerContractObject, domain.ServiceProductObject), nil)
	declared := domain.ResolveCommercialClosure(registry,
		productClosureKey(t, "product-express", domain.CustomerContractObject, domain.ServiceProductObject), nil)

	if undeclared.Outcome() != domain.UniquelyResolved || declared.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcomes = %q / %q, want both UNIQUELY_RESOLVED", undeclared.Outcome(), declared.Outcome())
	}
	if undeclared.ResolutionID() == declared.ResolutionID() {
		t.Fatalf("声明与不声明得到同一个解析标识 %q", declared.ResolutionID())
	}
}

// 提交前重解按原查询重跑（UC-PC-002 步骤 8）：原查询里的声明产品丢了，重解会按范围解出两个候选，一份仍然成立的
// 解析就被判成已失效。
func TestRevalidationKeepsTheDeclaredServiceProduct(t *testing.T) {
	registry := twoProductsInOneScope(t)
	prior := domain.ResolveCommercialClosure(registry,
		productClosureKey(t, "product-express", domain.CustomerContractObject, domain.ServiceProductObject), nil)
	if prior.Outcome() != domain.UniquelyResolved {
		t.Fatalf("prior outcome = %q, want UNIQUELY_RESOLVED", prior.Outcome())
	}

	current := domain.ValidateClosureBeforeDecision(registry, prior, nil)

	if current.Outcome() != domain.UniquelyResolved || current.ResolutionID() != prior.ResolutionID() {
		t.Fatalf("revalidated = %q / %q, want UNIQUELY_RESOLVED / %q",
			current.Outcome(), current.ResolutionID(), prior.ResolutionID())
	}
}
