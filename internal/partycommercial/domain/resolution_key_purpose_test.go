package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func pricingKey(t *testing.T, scope string, direction domain.PriceDirection) domain.ResolutionKey {
	t.Helper()
	key := resolutionKey(t, scope, domain.PriceRuleObject)
	key.Purpose = domain.PricingPurpose
	key.PriceDirection = direction
	return key
}

// registerPricePolicy 把一份与方向同向的政策登记进权威视图，供计价解析采用。
func registerPricePolicy(
	t *testing.T,
	registry *domain.CommercialRegistry,
	objectID string,
	direction domain.PriceDirection,
	scope, plan string,
) domain.CommercialPricePolicy {
	t.Helper()
	version := effectiveIn(t, registry, domain.PriceRuleObject, objectID, "v1", "sha256:"+objectID, scope)
	policy, err := domain.NewCommercialPricePolicy(
		version,
		direction,
		commercialValue(t, domain.NewPricingPlanReference, plan),
		direction,
		domain.PlanBindingConversionNone,
		commercialValue(t, domain.NewCommercialScopeReference, scope),
		mustInterval(t),
	)
	if err != nil {
		t.Fatalf("new price policy: %v", err)
	}
	registry.RegisterPricePolicy(policy)
	return policy
}

// Covers: UC-PC-002 一致性「请求计价依据时，解析键还必须包含计算目的和价格方向；同一范围
// 的 BUY、SELL 与 INTERNAL 解析和缓存不能互相复用」。
func TestPricingDirectionsNeverShareAResolutionIdentity(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerPricePolicy(t, registry, "policy-buy", domain.BuyDirection, "scope-a", "plan-buy")
	registerPricePolicy(t, registry, "policy-sell", domain.SellDirection, "scope-a", "plan-sell")
	registerPricePolicy(t, registry, "policy-internal", domain.InternalDirection, "scope-a", "plan-internal")

	identities := map[domain.PriceDirection]domain.ResolutionID{}
	for direction, plan := range map[domain.PriceDirection]string{
		domain.BuyDirection:      "plan-buy",
		domain.SellDirection:     "plan-sell",
		domain.InternalDirection: "plan-internal",
	} {
		result := domain.ResolveCommercialBasis(registry, pricingKey(t, "scope-a", direction), allPlansAdoptable)
		if result.Outcome() != domain.UniquelyResolved {
			t.Fatalf("direction %q outcome = %q, want UNIQUELY_RESOLVED", direction, result.Outcome())
		}
		policy, ok := result.AdoptedPricePolicy()
		if !ok || policy.PricingPlan().String() != plan || policy.Direction() != direction {
			t.Fatalf("direction %q adopted policy = %+v, want plan %q", direction, policy, plan)
		}
		identities[direction] = result.ResolutionID()
	}

	seen := map[domain.ResolutionID]domain.PriceDirection{}
	for direction, id := range identities {
		if other, clash := seen[id]; clash {
			t.Fatalf("%q and %q share resolution identity %q; one direction can answer the other", direction, other, id)
		}
		seen[id] = direction
	}
}

// Covers: UC-PC-002 — 计算目的是解析键的必需维度，缺失即输入未受理。
func TestPurposeIsRequiredOnEveryResolutionKey(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
	key.Purpose = domain.ResolutionPurposeInvalid

	result := domain.ResolveCommercialBasis(registry, key, nil)
	if result.Outcome() != domain.InputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
	}
}

// Covers: UC-PC-002 — 价格方向仅对计价目的有意义：计价缺方向不受理，非计价带方向同样
// 不受理，否则两个只在无意义字段上不同的键会解析出不同身份。
func TestPriceDirectionIsRequiredForPricingAndForbiddenOtherwise(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.PriceRuleObject, "price-1", "v1", "sha256:p1", "scope-a")
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	t.Run("pricing without a direction is not accepted", func(t *testing.T) {
		key := pricingKey(t, "scope-a", domain.PriceDirectionInvalid)
		if got := domain.ResolveCommercialBasis(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})

	t.Run("acceptance control carrying a direction is not accepted", func(t *testing.T) {
		key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
		key.PriceDirection = domain.BuyDirection
		if got := domain.ResolveCommercialBasis(registry, key, nil).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})
}

// Covers: UC-PC-002 — 目的不同的解析互不复用，即便范围与对象类型相同。
func TestDifferentPurposesResolveUnderDifferentIdentities(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	registerPricePolicy(t, registry, "policy-sell", domain.SellDirection, "scope-a", "plan-sell")

	pricing := domain.ResolveCommercialBasis(registry, pricingKey(t, "scope-a", domain.SellDirection), allPlansAdoptable)

	control := resolutionKey(t, "scope-a", domain.PriceRuleObject)
	control.Purpose = domain.AcceptanceControlPurpose
	controlResult := domain.ResolveCommercialBasis(registry, control, nil)

	if pricing.Outcome() != domain.UniquelyResolved || controlResult.Outcome() != domain.UniquelyResolved {
		t.Fatalf("fixture outcomes: pricing %q control %q", pricing.Outcome(), controlResult.Outcome())
	}
	if pricing.ResolutionID() == controlResult.ResolutionID() {
		t.Fatal("two purposes shared one resolution identity")
	}
}

// Covers: party-commercial CONTEXT — 价格方向是封闭集合，且没有第四个取值可以让一个
// 未声明方向的请求混进计价解析。
func TestPriceDirectionSetIsExactlyThreeValued(t *testing.T) {
	labels := map[string]struct{}{}
	for _, direction := range []domain.PriceDirection{domain.BuyDirection, domain.SellDirection, domain.InternalDirection} {
		label := direction.String()
		if label == "" {
			t.Fatalf("direction %d has no label", direction)
		}
		labels[label] = struct{}{}
	}
	if len(labels) != 3 {
		t.Fatalf("price direction collapsed into %d labels", len(labels))
	}
	if domain.PriceDirection(len(labels)+1).String() != "" {
		t.Fatal("a fourth price direction carries a label")
	}
}
