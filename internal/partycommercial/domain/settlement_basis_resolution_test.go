package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func settlementSelectorFor(t *testing.T, chargeScope string) domain.SettlementSelector {
	t.Helper()
	selector := settlementSelector(t)
	selector.ChargeScope = commercialValue(t, domain.NewChargeScopeReference, chargeScope)
	return selector
}

func registerSettlementPolicyOn(
	t *testing.T,
	registry *domain.CommercialRegistry,
	scope, objectID, chargeScope string,
	method domain.SettlementMethod,
) domain.SettlementPolicy {
	t.Helper()
	return registerSettlementPolicyUnder(
		t, registry, scope, objectID, fixtureContractLabel(t).String(), chargeScope, method)
}

func registerSettlementPolicyUnder(
	t *testing.T,
	registry *domain.CommercialRegistry,
	scope, objectID, contract, chargeScope string,
	method domain.SettlementMethod,
) domain.SettlementPolicy {
	t.Helper()
	version := effectiveIn(t, registry, domain.SettlementPolicyObject, objectID, "v1", "sha256:"+objectID, scope)
	policy, err := domain.NewSettlementPolicy(version, method,
		applicability(t, "customer-1", contract, chargeScope, "SYN"))
	if err != nil {
		t.Fatalf("new settlement policy: %v", err)
	}
	registry.RegisterSettlementPolicy(policy)
	return policy
}

// Covers: UC-PC-002「结算政策结果必须携带解析得到的预付/账期方式和明确适用范围，调用方
// 不得预选方式」（ADR-0044）。方式与六维范围经采用政策可观察，不再只剩一份裸版本。
//
// Covers: `AT-PC-031` 在解析入口的那一半：同一商业范围内预付与账期各守自己的费用范围，
// 互不冲突、各自唯一。
func TestSettlementBasisCarriesMethodAndScopeAndKeepsDisjointChargeScopesApart(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerSettlementPolicyOn(t, registry, "scope-a", "policy-prepaid", "charge-express", domain.PrepaidMethod)
	registerSettlementPolicyOn(t, registry, "scope-a", "policy-terms", "charge-economy", domain.TermsMethod)

	expected := map[string]domain.SettlementMethod{
		"charge-express": domain.PrepaidMethod,
		"charge-economy": domain.TermsMethod,
	}
	identities := map[string]domain.ResolutionID{}
	for chargeScope, method := range expected {
		t.Run(chargeScope, func(t *testing.T) {
			key := resolutionKey(t, "scope-a", domain.SettlementPolicyObject)
			key.Settlement = settlementSelectorFor(t, chargeScope)

			result := domain.ResolveCommercialBasis(registry, key, nil)
			if result.Outcome() != domain.UniquelyResolved {
				t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（不同费用范围不得互相冲突）", result.Outcome())
			}
			policy, ok := result.AdoptedSettlementPolicy()
			if !ok {
				t.Fatal("结算依据解出后方式与范围不可观察——只剩裸版本")
			}
			if policy.Method() != method {
				t.Fatalf("method = %q, want %q", policy.Method(), method)
			}
			if policy.Applicability().ChargeScope().String() != chargeScope {
				t.Fatalf("charge scope = %q, want %q", policy.Applicability().ChargeScope(), chargeScope)
			}
			if policy.Applicability().Currency().String() != "SYN" {
				t.Fatalf("currency = %q, want SYN", policy.Applicability().Currency())
			}
			identities[chargeScope] = result.ResolutionID()
		})
	}
	if identities["charge-express"] == identities["charge-economy"] {
		t.Fatal("两个费用范围共用同一解析身份，一个范围的结果就能回答另一个范围")
	}
}

// Covers: `AT-PC-032` 在解析入口的那一半：同一精确范围同时命中预付与账期 → 适用冲突，
// 不任选一种方式。
func TestSettlementBasisConflictsWhenOneChargeScopeIsHitByBothMethods(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerSettlementPolicyOn(t, registry, "scope-a", "policy-prepaid", "charge-express", domain.PrepaidMethod)
	registerSettlementPolicyOn(t, registry, "scope-a", "policy-terms", "charge-express", domain.TermsMethod)

	key := resolutionKey(t, "scope-a", domain.SettlementPolicyObject)
	result := domain.ResolveCommercialBasis(registry, key, nil)
	if result.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", result.Outcome())
	}
	if _, present := result.AdoptedSettlementPolicy(); present {
		t.Fatal("冲突仍任选了一种结算方式")
	}
	if result.CandidateCount() != 2 {
		t.Fatalf("candidate count = %d, want 2", result.CandidateCount())
	}
}

// 光有已登记的结算政策**版本**没有政策内容 → 无适用依据：通用版本解析产不出方式与范围，
// 正是 ADR-0044 要修的洞（镜像 ADR-0034「光有价格规则版本无法计价」）。
func TestABareSettlementVersionWithoutAPolicyIsNoBasis(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.SettlementPolicyObject, "policy-bare", "v1", "sha256:bare", "scope-a")

	result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.SettlementPolicyObject), nil)
	if result.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS——裸版本被当成了可用结算依据", result.Outcome())
	}
	if _, present := result.AdoptedVersion(); present {
		t.Fatal("没有政策内容却采用了版本")
	}
}

// 镜像 PriceDirection 的键纪律（ADR-0044）：请求结算依据必须给出选择器，其余请求必须缺席；
// 部分给出同样不受理。
func TestSettlementSelectorIsRequiredForSettlementAndForbiddenOtherwise(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerSettlementPolicyOn(t, registry, "scope-a", "policy-prepaid", "charge-express", domain.PrepaidMethod)
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	t.Run("settlement without a selector is not accepted", func(t *testing.T) {
		key := resolutionKey(t, "scope-a", domain.SettlementPolicyObject)
		key.Settlement = domain.SettlementSelector{}
		if got := domain.ResolveCommercialBasis(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})

	t.Run("a partially given selector is not accepted", func(t *testing.T) {
		key := resolutionKey(t, "scope-a", domain.SettlementPolicyObject)
		key.Settlement.Currency = domain.CurrencyCode{}
		if got := domain.ResolveCommercialBasis(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})

	t.Run("a contract request carrying a selector is not accepted", func(t *testing.T) {
		key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
		key.Settlement = settlementSelector(t)
		if got := domain.ResolveCommercialBasis(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})

	t.Run("a closure not asking for settlement must not carry a selector", func(t *testing.T) {
		key := closureKey(t, "scope-a", domain.CustomerContractObject)
		key.Settlement = settlementSelector(t)
		if got := domain.ResolveCommercialClosure(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})
}

// 闭包采用的结算依据同样携带政策：SA 消费的是闭包，不是单依据结果（ADR-0044）。
func TestClosureCarriesTheAdoptedSettlementPolicy(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...), nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	basis, present := closure.AdoptedFor(domain.SettlementPolicyObject)
	if !present {
		t.Fatal("闭包没有采用结算依据")
	}
	policy, ok := basis.SettlementPolicy()
	if !ok {
		t.Fatal("闭包采用了结算依据却丢了政策——方式与范围不可观察")
	}
	if policy.Method() != domain.PrepaidMethod {
		t.Fatalf("method = %q, want PREPAID", policy.Method())
	}
	if policy.Applicability().ChargeScope().String() != "charge-express" {
		t.Fatalf("charge scope = %q, want charge-express", policy.Applicability().ChargeScope())
	}
	if contractBasis, ok := closure.AdoptedFor(domain.CustomerContractObject); !ok {
		t.Fatal("closure lost the contract member")
	} else if _, leaked := contractBasis.SettlementPolicy(); leaked {
		t.Fatal("非结算成员也带上了结算政策")
	}
}

// Covers: ADR-0080 —— 闭包键不得预选合同维。
//
// 合同版本是同一个闭包正在解的另一项依据；由调用方在键上指名一个，就是消费方在指定该
// 选中哪个商业版本（`psports.CommercialBasisQuery` 明禁的那件事），而且它与闭包实际解出的
// 那一版可以不同——那时闭包会「按 A 版合同接单、按 B 版合同的结算政策控制」，两边都不报错。
func TestAClosureKeyMayNotPreselectTheContractDimension(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)

	key := closureKey(t, "scope-a", closureBases...)
	key.Settlement.Contract = fixtureContractLabel(t)

	// 指名的正是本次会解出的那一版：连这个都要拒，否则「预选」只是碰巧对了的那些次不报错。
	if got := domain.ResolveCommercialClosure(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
	}
}

// Covers: ADR-0080 —— 要结算依据就必须在同一个闭包里要客户合同。
//
// 合同维要由本闭包解出的合同来填；不请求合同就永远没人填得上。放这样的键进来，结果会是
// 一句像「没有适用结算政策」的话，而实情是这个请求本身立不起来。
func TestSettlementBasisRequiresTheContractInTheSameClosure(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)

	key := closureKey(t, "scope-a", domain.SettlementPolicyObject)

	if got := domain.ResolveCommercialClosure(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
	}
}

// Covers: ADR-0080 的「前提未解析」那一格——合同解不出时，结算政策不得落成`无适用依据`。
//
// 这是本决定要买的东西。两句话的恢复动作不同：`无适用依据`是权威说了这个范围没有结算约定，
// 照它去做就是「让商业侧登一份结算政策」——登完闭包照样解不开，因为缺的是合同。
func TestAnUnresolvedContractLeavesTheSettlementBasisUnasked(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	// 合同不登，其余两项齐备：结算政策在库、能命中，只差没人告诉它是哪一版合同。
	seedClosure(t, registry, "scope-a", domain.AcceptanceRulePackageObject, domain.SettlementPolicyObject)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...), nil)

	if closure.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS（合同缺项）", closure.Outcome())
	}
	unresolved := closure.UnresolvedBases()
	if len(unresolved) != 1 || unresolved[0] != domain.CustomerContractObject {
		t.Fatalf("unresolved = %v, want exactly the customer contract——结算政策不是权威说没有的",
			unresolved)
	}
	premise := closure.PremiseUnresolvedBases()
	if len(premise) != 1 || premise[0] != domain.SettlementPolicyObject {
		t.Fatalf("premise-unresolved = %v, want exactly the settlement policy", premise)
	}
}

// Covers: ADR-0080 —— 解析顺序由依赖决定，不由调用方的声明顺序决定。
//
// 必需依据集合是调用方给的，它按什么次序写下来不构成不同的请求（闭包指纹先排序正是这个
// 意思）。结算政策排在合同之前声明时若解不出来，这条纪律就有一个静默的例外。
func TestSettlementResolvesEvenWhenDeclaredBeforeTheContract(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)

	reversed := []domain.CommercialObjectKind{
		domain.SettlementPolicyObject,
		domain.AcceptanceRulePackageObject,
		domain.CustomerContractObject,
	}
	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", reversed...), nil)

	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED（reason=%q，前提未解析=%v）",
			closure.Outcome(), closure.Reason(), closure.PremiseUnresolvedBases())
	}
	basis, present := closure.AdoptedFor(domain.SettlementPolicyObject)
	if !present {
		t.Fatal("闭包没有采用结算依据")
	}
	if _, ok := basis.SettlementPolicy(); !ok {
		t.Fatal("闭包采用了结算依据却丢了政策")
	}
	// 同一请求换个声明次序必须得到同一个解析身份，否则续办引用与失效检测都会跟着次序摆动。
	inOrder := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...), nil)
	if closure.ResolutionID() != inOrder.ResolutionID() {
		t.Fatalf("声明次序改变了解析身份：%q vs %q", closure.ResolutionID(), inOrder.ResolutionID())
	}
}

// Covers: ADR-0080 —— 结算政策按**解出的**那一版合同选，不是按范围里碰巧存在的任何一份。
//
// 与上一例配对：只证「合同解不出时不问」，一个把合同维填成空串或任意值的实现也能通过，
// 而那样会采用一份约定给别的合同的结算方式。
func TestASettlementPolicyNamingAnotherContractIsNotAdopted(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", domain.CustomerContractObject, domain.AcceptanceRulePackageObject)
	registerSettlementPolicyUnder(
		t, registry, "scope-a", "policy-other-contract", "contract-9/v9", "charge-express",
		domain.TermsMethod)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", closureBases...), nil)

	if closure.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS——别的合同的结算约定被采用了",
			closure.Outcome())
	}
	unresolved := closure.UnresolvedBases()
	if len(unresolved) != 1 || unresolved[0] != domain.SettlementPolicyObject {
		t.Fatalf("unresolved = %v, want exactly the settlement policy", unresolved)
	}
	if premise := closure.PremiseUnresolvedBases(); len(premise) != 0 {
		t.Fatalf("premise-unresolved = %v, want empty——合同解出来了，这一项是真的问过", premise)
	}
}

// 只改政策、不动版本正文时解析身份仍须变：结算政策参与 ViewRevision 派生（ADR-0044）。
func TestRegisteringASettlementPolicyAdvancesTheViewRevision(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	tenant := commercialValue(t, domain.NewTenantID, "tenant-1")
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")

	version := effectiveIn(t, registry, domain.SettlementPolicyObject, "policy-1", "v1", "sha256:p1", "scope-a")
	before := registry.ViewRevision(tenant, scope)

	policy, err := domain.NewSettlementPolicy(version, domain.PrepaidMethod,
		applicability(t, "customer-1", "contract-1/v1", "charge-express", "SYN"))
	if err != nil {
		t.Fatalf("new settlement policy: %v", err)
	}
	registry.RegisterSettlementPolicy(policy)

	if registry.ViewRevision(tenant, scope) == before {
		t.Fatal("登记结算政策没有推进视图修订——先前解析的失效检测看不见它")
	}
}
