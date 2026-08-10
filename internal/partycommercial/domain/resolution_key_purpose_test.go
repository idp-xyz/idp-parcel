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

// Covers: UC-PC-002 一致性「请求计价依据时，解析键还必须包含计算目的和价格方向；同一范围
// 的 BUY、SELL 与 INTERNAL 解析和缓存不能互相复用」。
func TestPricingDirectionsNeverShareAResolutionIdentity(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.PriceRuleObject, "price-1", "v1", "sha256:p1", "scope-a")

	identities := map[domain.PriceDirection]domain.ResolutionID{}
	for _, direction := range []domain.PriceDirection{domain.BuyDirection, domain.SellDirection, domain.InternalDirection} {
		result := domain.ResolveCommercialBasis(registry, pricingKey(t, "scope-a", direction))
		if result.Outcome() != domain.UniquelyResolved {
			t.Fatalf("direction %q outcome = %q, want UNIQUELY_RESOLVED", direction, result.Outcome())
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

	result := domain.ResolveCommercialBasis(registry, key)
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
		if got := domain.ResolveCommercialBasis(registry, key).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})

	t.Run("acceptance control carrying a direction is not accepted", func(t *testing.T) {
		key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
		key.PriceDirection = domain.BuyDirection
		if got := domain.ResolveCommercialBasis(registry, key).Outcome(); got != domain.InputNotAccepted {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
		}
	})
}

// Covers: UC-PC-002 — 目的不同的解析互不复用，即便范围与对象类型相同。
func TestDifferentPurposesResolveUnderDifferentIdentities(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.PriceRuleObject, "price-1", "v1", "sha256:p1", "scope-a")

	pricing := domain.ResolveCommercialBasis(registry, pricingKey(t, "scope-a", domain.SellDirection))

	control := resolutionKey(t, "scope-a", domain.PriceRuleObject)
	control.Purpose = domain.AcceptanceControlPurpose
	controlResult := domain.ResolveCommercialBasis(registry, control)

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
