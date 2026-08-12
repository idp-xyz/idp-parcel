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
	version := effectiveIn(t, registry, domain.SettlementPolicyObject, objectID, "v1", "sha256:"+objectID, scope)
	policy, err := domain.NewSettlementPolicy(version, method,
		applicability(t, "customer-1", "contract-1/v1", chargeScope, "SYN"))
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
