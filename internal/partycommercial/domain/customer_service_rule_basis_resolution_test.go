package domain_test

import (
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// serviceRuleBases 是索赔资格要的那一份闭包：客户合同加客户服务规则。
var serviceRuleBases = []domain.CommercialObjectKind{
	domain.CustomerContractObject,
	domain.CustomerServiceRuleObject,
}

func registerContract(t *testing.T, registry *domain.CommercialRegistry, objectID string) domain.CommercialVersion {
	t.Helper()
	return effectiveIn(t, registry, domain.CustomerContractObject, objectID, "v1", "sha256:"+objectID, "scope-a")
}

// registerServiceRule 登记一版已生效的客户服务规则壳。names 是壳上的 references：解析只看壳，指名合同的
// 是挂合同的那一层，指名服务产品或什么都不指名的是挂产品的那一层。
func registerServiceRule(
	t *testing.T,
	registry *domain.CommercialRegistry,
	objectID, version string,
	names map[domain.CommercialObjectKind]string,
) domain.CommercialVersion {
	t.Helper()
	return effectiveNaming(t, registry, domain.CustomerServiceRuleObject,
		objectID, version, "sha256:"+objectID+"-"+version, "scope-a", names)
}

func namesContract(objectID string) map[domain.CommercialObjectKind]string {
	return map[domain.CommercialObjectKind]string{domain.CustomerContractObject: objectID}
}

func namesProduct(objectID string) map[domain.CommercialObjectKind]string {
	return map[domain.CommercialObjectKind]string{domain.ServiceProductObject: objectID}
}

// adoptedKinds 交回闭包的成员次序，即 Adopted() 交出与快照写下的次序。
func adoptedKinds(closure domain.CommercialClosure) []domain.CommercialObjectKind {
	kinds := make([]domain.CommercialObjectKind, 0, len(closure.Adopted()))
	for _, adopted := range closure.Adopted() {
		kinds = append(kinds, adopted.Kind())
	}
	return kinds
}

// adoptedServiceRule 交回闭包采纳的那一版客户服务规则的对象标识；没采纳就当场失败。
func adoptedServiceRule(t *testing.T, closure domain.CommercialClosure) string {
	t.Helper()
	adopted, present := closure.AdoptedFor(domain.CustomerServiceRuleObject)
	if !present {
		t.Fatalf("闭包没有采纳客户服务规则（outcome=%q，conflicting=%v，unresolved=%v）",
			closure.Outcome(), closure.ConflictingBases(), closure.UnresolvedBases())
	}
	return adopted.Version().ObjectID().String()
}

// Covers: ADR-0176 决定一「合同优先」——同一范围挂合同的一版与挂产品的一版并存，闭包解出合同之后采纳壳上
// 指名了那份合同的那一版，不再答`适用冲突`。
//
// 两种声明次序都走：客户服务规则若排在合同前面就被解，合同层永远是空的，闭包会静静回落到产品版。
func TestTheClosureAdoptsTheServiceRuleNamingTheResolvedContract(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))
	registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

	for name, bases := range map[string][]domain.CommercialObjectKind{
		"合同先声明": {domain.CustomerContractObject, domain.CustomerServiceRuleObject},
		"规则先声明": {domain.CustomerServiceRuleObject, domain.CustomerContractObject},
	} {
		t.Run(name, func(t *testing.T) {
			closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", bases...), nil)

			if closure.Outcome() != domain.UniquelyResolved {
				t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（conflicting=%v）",
					closure.Outcome(), closure.ConflictingBases())
			}
			if got := adoptedServiceRule(t, closure); got != "csr-contract" {
				t.Fatalf("采纳的客户服务规则 = %q, want csr-contract——合同版在场却回落到了产品版", got)
			}
		})
	}
}

// Covers: ADR-0176 决定一「产品回落」——没有一版指名闭包解出的合同时，采纳挂产品的那一版。
//
// 同范围另有一版挂**别的**合同：它不是这一户的合同版，也不是产品底座。只证「没有合同版就回落」的话，
// 一个把凡是挂合同的版本都当合同层的实现也能过，而那会把别户的约定套给这一户。
func TestTheClosureFallsBackToTheProductTierWhenNoVersionNamesTheResolvedContract(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	registerServiceRule(t, registry, "csr-other-contract", "v1", namesContract("contract-2"))
	registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", serviceRuleBases...), nil)

	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（reason=%q，conflicting=%v）",
			closure.Outcome(), closure.Reason(), closure.ConflictingBases())
	}
	if got := adoptedServiceRule(t, closure); got != "csr-product" {
		t.Fatalf("采纳的客户服务规则 = %q, want csr-product", got)
	}
}

// Covers: ADR-0176 决定一「同层多候选=`适用冲突`」的合同层那一半，及拆票裁定「同一合同两版只在闭包答
// `适用冲突`」：两版都指名闭包解出的合同即冲突，且不回落到产品版——合同层冲突时回落，等于把这一户已经
// 另定的条款静静换回产品标准。
func TestTwoVersionsNamingTheResolvedContractConflictWithoutFallingBack(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))
	registerServiceRule(t, registry, "csr-contract", "v2", namesContract(fixtureContractObjectID))
	registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", serviceRuleBases...), nil)

	if closure.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", closure.Outcome())
	}
	conflicting := closure.ConflictingBases()
	if len(conflicting) != 1 || conflicting[0] != domain.CustomerServiceRuleObject {
		t.Fatalf("conflicting = %v, want exactly the customer service rule", conflicting)
	}
	if len(closure.Adopted()) != 0 {
		t.Fatal("冲突的闭包仍交回了已采纳的成员")
	}
}

// Covers: ADR-0176 决定一「同层多候选=`适用冲突`」的产品层那一半：合同层零候选、产品层两版即冲突。一版
// 指名服务产品、一版什么都没指名——两种壳同在产品层，谁也不比谁更像产品版。
func TestTwoProductTierVersionsConflictWhenNoVersionNamesTheContract(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))
	registerServiceRule(t, registry, "csr-bare", "v1", nil)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", serviceRuleBases...), nil)

	if closure.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", closure.Outcome())
	}
	conflicting := closure.ConflictingBases()
	if len(conflicting) != 1 || conflicting[0] != domain.CustomerServiceRuleObject {
		t.Fatalf("conflicting = %v, want exactly the customer service rule", conflicting)
	}
}

// Covers: ADR-0176 决定一「两层皆零=`无适用依据`」：范围里只有挂别的合同的一版时，这一户两层都没有可采纳的
// 版本。它报在 unresolved 而不是前提未解——合同解出来了，这一项是真的问过。
func TestNeitherTierHoldingAVersionIsNoApplicableBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerContract(t, registry, fixtureContractObjectID)
	registerServiceRule(t, registry, "csr-other-contract", "v1", namesContract("contract-2"))

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", serviceRuleBases...), nil)

	if closure.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS（reason=%q）", closure.Outcome(), closure.Reason())
	}
	unresolved := closure.UnresolvedBases()
	if len(unresolved) != 1 || unresolved[0] != domain.CustomerServiceRuleObject {
		t.Fatalf("unresolved = %v, want exactly the customer service rule", unresolved)
	}
	if premise := closure.PremiseUnresolvedBases(); len(premise) != 0 {
		t.Fatalf("premise-unresolved = %v, want empty", premise)
	}
}

// Covers: 拆票裁定「合同被请求但冲突或无依据 → 前提未解」：合同请求了却没解出来，客户服务规则不回落到产品版
// ——这时无从知道这一户有没有挂合同的那一版，回落可能静默套上更宽的条款。
func TestAnUnresolvedContractLeavesTheServiceRuleUnasked(t *testing.T) {
	t.Run("合同无适用依据", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

		closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", serviceRuleBases...), nil)

		if closure.Outcome() != domain.NoApplicableBasis {
			t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS", closure.Outcome())
		}
		if unresolved := closure.UnresolvedBases(); len(unresolved) != 1 || unresolved[0] != domain.CustomerContractObject {
			t.Fatalf("unresolved = %v, want exactly the customer contract", unresolved)
		}
		if premise := closure.PremiseUnresolvedBases(); len(premise) != 1 || premise[0] != domain.CustomerServiceRuleObject {
			t.Fatalf("premise-unresolved = %v, want exactly the customer service rule", premise)
		}
	})

	t.Run("合同适用冲突", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		registerContract(t, registry, fixtureContractObjectID)
		registerContract(t, registry, "contract-2")
		registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

		closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", serviceRuleBases...), nil)

		if closure.Outcome() != domain.ApplicabilityConflict {
			t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", closure.Outcome())
		}
		if conflicting := closure.ConflictingBases(); len(conflicting) != 1 || conflicting[0] != domain.CustomerContractObject {
			t.Fatalf("conflicting = %v, want exactly the customer contract", conflicting)
		}
		if premise := closure.PremiseUnresolvedBases(); len(premise) != 1 || premise[0] != domain.CustomerServiceRuleObject {
			t.Fatalf("premise-unresolved = %v, want exactly the customer service rule", premise)
		}
	})
}

// Covers: 拆票裁定「合同根本没被请求 → 只看产品层」：挂合同的版本这时不是候选，既不与产品版冲突，也不会在
// 没有产品版时被采纳。
func TestWithoutTheContractInTheClosureOnlyTheProductTierIsAsked(t *testing.T) {
	t.Run("产品版在场", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))
		registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

		closure := domain.ResolveCommercialClosure(registry,
			closureKey(t, "scope-a", domain.CustomerServiceRuleObject), nil)

		if closure.Outcome() != domain.UniquelyResolved {
			t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（conflicting=%v）",
				closure.Outcome(), closure.ConflictingBases())
		}
		if got := adoptedServiceRule(t, closure); got != "csr-product" {
			t.Fatalf("采纳的客户服务规则 = %q, want csr-product", got)
		}
	})

	t.Run("只有合同版", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))

		closure := domain.ResolveCommercialClosure(registry,
			closureKey(t, "scope-a", domain.CustomerServiceRuleObject), nil)

		if closure.Outcome() != domain.NoApplicableBasis {
			t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS——没请求合同却采纳了挂合同的一版", closure.Outcome())
		}
		if unresolved := closure.UnresolvedBases(); len(unresolved) != 1 || unresolved[0] != domain.CustomerServiceRuleObject {
			t.Fatalf("unresolved = %v, want exactly the customer service rule", unresolved)
		}
		if premise := closure.PremiseUnresolvedBases(); len(premise) != 0 {
			t.Fatalf("premise-unresolved = %v, want empty——没请求合同，就没有合同这个前提", premise)
		}
	})
}

// 单依据入口与闭包同一口径：单依据键上没有合同那一维，客户服务规则只看产品层，挂合同的版本不是候选。
func TestASingleBasisServiceRuleResolutionOnlyAsksTheProductTier(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))
	registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

	result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerServiceRuleObject), nil)

	if result.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", result.Outcome())
	}
	if adopted, _ := result.AdoptedVersion(); adopted.ObjectID().String() != "csr-product" {
		t.Fatalf("采纳的客户服务规则 = %q, want csr-product", adopted.ObjectID())
	}
}

// Covers: 票 02 完成判据「只有挂产品一版的既有登记，解析结果与改动前相同」。两种在册的产品层壳各走一次：
// 指名服务产品的，与什么都没指名的（ConsistentCustomerServiceRuleApplicability 对没指名的壳放行）。
//
// 解析标识是改动前的代码（基 1d67e27c）对同一夹具实算后钉住的：键指纹、视图修订与采用集合三样不变，它就
// 不变；它一变，既有登记已固定的解析标识与续办引用就跟着改口。两种壳实算是同一个值，因为壳上的 references
// 不进解析身份。两种声明次序也是同一个值——客户服务规则挪进第二段只改解析先后，不改身份。
//
// 成员次序也照改动前，是调用方声明的次序。解析标识先把成员排序再散列，看不出次序；快照却按次序写下，
// 次序一变内容摘要就变，已固定的解析重解一次即撞`内容冲突`——落库那一层由 postgres 适配器的
// TestAResolutionFixedWithTheServiceRuleDeclaredFirstReplaysAsRecorded 钉住。
func TestAnExistingProductOnlyRegistrationResolvesAsBefore(t *testing.T) {
	const want = "CLO-d89b23824ea8f1bd"
	for name, names := range map[string]map[domain.CommercialObjectKind]string{
		"壳指名服务产品": namesProduct("product-a"),
		"壳什么都没指名": nil,
	} {
		t.Run(name, func(t *testing.T) {
			registry := domain.NewCommercialRegistry()
			registerContract(t, registry, fixtureContractObjectID)
			registerServiceRule(t, registry, "csr-product", "v1", names)

			for _, bases := range [][]domain.CommercialObjectKind{
				serviceRuleBases,
				{domain.CustomerServiceRuleObject, domain.CustomerContractObject},
			} {
				closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", bases...), nil)

				if closure.Outcome() != domain.UniquelyResolved {
					t.Fatalf("bases %v: outcome = %q, want UNIQUELY_RESOLVED", bases, closure.Outcome())
				}
				if got := adoptedServiceRule(t, closure); got != "csr-product" {
					t.Fatalf("bases %v: 采纳的客户服务规则 = %q, want csr-product", bases, got)
				}
				if got := closure.ResolutionID().String(); got != want {
					t.Fatalf("bases %v: resolution ID = %q, want %q（改动前实算）", bases, got, want)
				}
				if got := adoptedKinds(closure); !slices.Equal(got, bases) {
					t.Fatalf("bases %v: 成员次序 = %v, want %v（改动前按声明次序）", bases, got, bases)
				}
			}
		})
	}
}

// Covers: 票 02「同范围挂服务产品、锚点生效的选法抽成一处，供 03 复用」：CustomerServiceRuleProductBase 与闭包
// 回落层读同一份分层——挂合同的（不论哪份合同）不算底座，挂产品的与什么都没指名的算，多版是`适用冲突`；
// 范围、租户、锚点与声明的服务产品各自收窄。
func TestTheProductBaseIsTheTierTheClosureFallsBackTo(t *testing.T) {
	tenant := commercialValue(t, domain.NewTenantID, "tenant-1")
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")
	anchor := closureKey(t, "scope-a", domain.CustomerContractObject).Anchor
	noProduct := domain.CommercialObjectID{}

	t.Run("挂合同的不算底座", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		registerServiceRule(t, registry, "csr-contract", "v1", namesContract(fixtureContractObjectID))
		registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

		base, outcome := registry.CustomerServiceRuleProductBase(tenant, scope, anchor, noProduct)
		if outcome != domain.UniquelyResolved || base.ObjectID().String() != "csr-product" {
			t.Fatalf("底座 = %q（%q），want csr-product（UNIQUELY_RESOLVED）", base.ObjectID(), outcome)
		}
	})

	t.Run("零版是无适用依据，两版是冲突", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		registerServiceRule(t, registry, "csr-other-contract", "v1", namesContract("contract-2"))
		if _, outcome := registry.CustomerServiceRuleProductBase(tenant, scope, anchor, noProduct); outcome != domain.NoApplicableBasis {
			t.Fatalf("只有挂合同的一版：outcome = %q, want NO_APPLICABLE_BASIS", outcome)
		}

		registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))
		registerServiceRule(t, registry, "csr-bare", "v1", nil)
		if base, outcome := registry.CustomerServiceRuleProductBase(tenant, scope, anchor, noProduct); outcome != domain.ApplicabilityConflict {
			t.Fatalf("两版产品版：outcome = %q（底座 %q）, want APPLICABILITY_CONFLICT", outcome, base.ObjectID())
		}
	})

	t.Run("声明的服务产品收窄底座", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		registerServiceRule(t, registry, "csr-product-a", "v1", namesProduct("product-a"))
		registerServiceRule(t, registry, "csr-product-b", "v1", namesProduct("product-b"))

		declared := commercialValue(t, domain.NewCommercialObjectID, "product-a")
		base, outcome := registry.CustomerServiceRuleProductBase(tenant, scope, anchor, declared)
		if outcome != domain.UniquelyResolved || base.ObjectID().String() != "csr-product-a" {
			t.Fatalf("声明 product-a：底座 = %q（%q），want csr-product-a", base.ObjectID(), outcome)
		}
		if _, outcome := registry.CustomerServiceRuleProductBase(tenant, scope, anchor, noProduct); outcome != domain.ApplicabilityConflict {
			t.Fatalf("不声明服务产品：outcome = %q, want APPLICABILITY_CONFLICT", outcome)
		}
	})

	t.Run("别的范围、别的租户、锚点不在区间内都不算", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

		later, err := domain.NewSelectionAnchor(time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC), anchor.PolicyVersion())
		if err != nil {
			t.Fatalf("new selection anchor: %v", err)
		}
		otherScope := commercialValue(t, domain.NewCommercialScopeReference, "scope-b")
		otherTenant := commercialValue(t, domain.NewTenantID, "tenant-2")
		for name, ask := range map[string]func() domain.ResolutionOutcome{
			"别的范围": func() domain.ResolutionOutcome {
				_, outcome := registry.CustomerServiceRuleProductBase(tenant, otherScope, anchor, noProduct)
				return outcome
			},
			"别的租户": func() domain.ResolutionOutcome {
				_, outcome := registry.CustomerServiceRuleProductBase(otherTenant, scope, anchor, noProduct)
				return outcome
			},
			"锚点在有效区间之后": func() domain.ResolutionOutcome {
				_, outcome := registry.CustomerServiceRuleProductBase(tenant, scope, later, noProduct)
				return outcome
			},
		} {
			if got := ask(); got != domain.NoApplicableBasis {
				t.Fatalf("%s：outcome = %q, want NO_APPLICABLE_BASIS", name, got)
			}
		}
	})

	t.Run("立不住的问法不数候选", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		registerServiceRule(t, registry, "csr-product", "v1", namesProduct("product-a"))

		if _, outcome := registry.CustomerServiceRuleProductBase(domain.TenantID{}, scope, anchor, noProduct); outcome != domain.InputNotAccepted {
			t.Fatalf("租户缺席：outcome = %q, want INPUT_NOT_ACCEPTED", outcome)
		}
		if _, outcome := registry.CustomerServiceRuleProductBase(tenant, scope, domain.SelectionAnchor{}, noProduct); outcome != domain.ResolutionPending {
			t.Fatalf("锚点缺席：outcome = %q, want RESOLUTION_PENDING", outcome)
		}
		var unreadable *domain.CommercialRegistry
		if _, outcome := unreadable.CustomerServiceRuleProductBase(tenant, scope, anchor, noProduct); outcome != domain.ResolutionPending {
			t.Fatalf("权威读不到：outcome = %q, want RESOLUTION_PENDING", outcome)
		}
	})
}
