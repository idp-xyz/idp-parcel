package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func pricePolicy(t *testing.T, objectID string, direction domain.PriceDirection, scope, plan string) domain.CommercialPricePolicy {
	t.Helper()
	live, err := registerable(t, domain.PriceRuleObject, objectID, "v1", "sha256:"+objectID).
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	policy, err := domain.NewCommercialPricePolicy(
		live,
		direction,
		commercialValue(t, domain.NewPricingPlanReference, plan),
		commercialValue(t, domain.NewCommercialScopeReference, scope),
		mustInterval(t),
	)
	if err != nil {
		t.Fatalf("new commercial price policy: %v", err)
	}
	return policy
}

// allPlansAdoptable 是「parcel-pricing 说每一份方案都还在」的替身，供那些不以方案存续为
// 主题的用例使用。它显式给出而不是留空：留空的含义是没问到，那会让每一条用例都停在未决上。
func allPlansAdoptable(domain.PricingPlanReference) domain.PricingPlanStanding {
	return domain.PricingPlanAdoptable
}

func priceQuery(t *testing.T, direction domain.PriceDirection, scope string) domain.PricePolicyQuery {
	t.Helper()
	query, err := domain.NewPricePolicyQuery(
		direction,
		commercialValue(t, domain.NewCommercialScopeReference, scope),
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new price policy query: %v", err)
	}
	return query
}

// Covers: CONTEXT「BUY、SELL 和 INTERNAL 的商业授权与适用范围分别表达」— 同一范围的
// 三个方向各自解析，一个方向的政策绝不回答另一个方向。
func TestEachDirectionResolvesItsOwnPolicy(t *testing.T) {
	policies := []domain.CommercialPricePolicy{
		pricePolicy(t, "policy-buy", domain.BuyDirection, "scope-a", "plan-buy"),
		pricePolicy(t, "policy-sell", domain.SellDirection, "scope-a", "plan-sell"),
	}

	for direction, plan := range map[domain.PriceDirection]string{
		domain.BuyDirection:  "plan-buy",
		domain.SellDirection: "plan-sell",
	} {
		t.Run(direction.String(), func(t *testing.T) {
			resolved, err := domain.ResolveCommercialPricePolicy(policies, priceQuery(t, direction, "scope-a"), allPlansAdoptable)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if resolved.PricingPlan().String() != plan {
				t.Fatalf("plan = %q, want %q", resolved.PricingPlan(), plan)
			}
			if resolved.Direction() != direction {
				t.Fatalf("direction = %q, want %q", resolved.Direction(), direction)
			}
		})
	}

	t.Run("an undeclared direction has no policy", func(t *testing.T) {
		if _, err := domain.ResolveCommercialPricePolicy(policies, priceQuery(t, domain.InternalDirection, "scope-a"), allPlansAdoptable); !errors.Is(err, domain.ErrNoApplicablePricePolicy) {
			t.Fatalf("error = %v; a direction without a policy borrowed another's", err)
		}
	})
}

// Covers: CONTEXT「绑定缺失、过期、区间重叠或引用未决时，返回无适用依据、适用冲突或解析
// 未决，不使用默认价」。
func TestMissingOrOverlappingBindingsNeverFallBackToADefault(t *testing.T) {
	t.Run("no policy is no applicable basis", func(t *testing.T) {
		if _, err := domain.ResolveCommercialPricePolicy(nil, priceQuery(t, domain.SellDirection, "scope-a"), allPlansAdoptable); !errors.Is(err, domain.ErrNoApplicablePricePolicy) {
			t.Fatalf("error = %v, want ErrNoApplicablePricePolicy", err)
		}
	})

	t.Run("overlapping policies conflict rather than pick one", func(t *testing.T) {
		policies := []domain.CommercialPricePolicy{
			pricePolicy(t, "policy-sell-1", domain.SellDirection, "scope-a", "plan-1"),
			pricePolicy(t, "policy-sell-2", domain.SellDirection, "scope-a", "plan-2"),
		}
		resolved, err := domain.ResolveCommercialPricePolicy(policies, priceQuery(t, domain.SellDirection, "scope-a"), allPlansAdoptable)
		if !errors.Is(err, domain.ErrPricePolicyConflict) {
			t.Fatalf("error = %v, want ErrPricePolicyConflict", err)
		}
		if resolved.PricingPlan().String() != "" {
			t.Fatal("a conflict still named a pricing plan")
		}
	})

	t.Run("expired applicability does not answer", func(t *testing.T) {
		policies := []domain.CommercialPricePolicy{pricePolicy(t, "policy-sell", domain.SellDirection, "scope-a", "plan-sell")}
		late, err := domain.NewPricePolicyQuery(
			domain.SellDirection,
			commercialValue(t, domain.NewCommercialScopeReference, "scope-a"),
			time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatalf("new query: %v", err)
		}
		if _, err := domain.ResolveCommercialPricePolicy(policies, late, allPlansAdoptable); !errors.Is(err, domain.ErrNoApplicablePricePolicy) {
			t.Fatalf("error = %v; an expired policy still answered", err)
		}
	})
}

// Covers: `AT-PC-036`「合同和价格政策唯一，但绑定价卡版本已退役且无替代 → 返回解析未决或
// 无适用依据，不选择最新价卡兜底」，以及 CONTEXT「绑定缺失、过期、区间重叠或**引用未决**时
// …不使用默认价」里此前没有任何用例的那一支。
//
// 政策仍在有效区间内，证明不了它绑的那份方案还在：方案由 parcel-pricing 拥有，它退役时本
// 上下文的政策一个字节都没变，于是按方向加范围解析照样唯一命中。
func TestAPolicyBoundToAnUnusablePlanDoesNotResolve(t *testing.T) {
	policies := []domain.CommercialPricePolicy{
		pricePolicy(t, "policy-sell", domain.SellDirection, "scope-a", "plan-retired"),
	}

	t.Run("a withdrawn plan is not adopted", func(t *testing.T) {
		resolved, err := domain.ResolveCommercialPricePolicy(policies, priceQuery(t, domain.SellDirection, "scope-a"),
			func(domain.PricingPlanReference) domain.PricingPlanStanding { return domain.PricingPlanWithdrawn })
		if !errors.Is(err, domain.ErrPricingPlanWithdrawn) {
			t.Fatalf("error = %v, want ErrPricingPlanWithdrawn", err)
		}
		if resolved.PricingPlan().String() != "" {
			t.Fatalf("绑着已退役方案的政策仍被采用：%q", resolved.PricingPlan())
		}
	})

	t.Run("an unanswered standing is not a pass", func(t *testing.T) {
		if _, err := domain.ResolveCommercialPricePolicy(policies, priceQuery(t, domain.SellDirection, "scope-a"), nil); !errors.Is(err, domain.ErrPricingPlanNotConfirmed) {
			t.Fatalf("error = %v, want ErrPricingPlanNotConfirmed", err)
		}
	})

	// 问的必须是这份政策绑的那一份方案。问成别的，答复再准也证明不了被采用的这一份还在。
	t.Run("the plan asked about is the bound one", func(t *testing.T) {
		var asked []string
		if _, err := domain.ResolveCommercialPricePolicy(policies, priceQuery(t, domain.SellDirection, "scope-a"),
			func(plan domain.PricingPlanReference) domain.PricingPlanStanding {
				asked = append(asked, plan.String())
				return domain.PricingPlanAdoptable
			}); err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(asked) != 1 || asked[0] != "plan-retired" {
			t.Fatalf("asked %v, want exactly the bound plan", asked)
		}
	})

	// 两种不可用必须分得开：一个要再问一次 parcel-pricing，一个要商业责任方改挂。压成一个
	// 哨兵，调用方就会对着一份永远不会回来的方案重试到底。
	t.Run("the two unusable answers stay distinguishable", func(t *testing.T) {
		if errors.Is(domain.ErrPricingPlanWithdrawn, domain.ErrPricingPlanNotConfirmed) ||
			errors.Is(domain.ErrPricingPlanNotConfirmed, domain.ErrPricingPlanWithdrawn) {
			t.Fatal("两个哨兵互相 Is，调用方分不出该重试还是该转商业责任方")
		}
		for _, unusable := range []error{domain.ErrPricingPlanWithdrawn, domain.ErrPricingPlanNotConfirmed} {
			if errors.Is(unusable, domain.ErrNoApplicablePricePolicy) {
				t.Fatalf("%v 被读成了「这个范围没有价格政策」，而权威并没有这么说", unusable)
			}
		}
	})
}

// Covers: CONTEXT「必须显式绑定价格方向和可执行定价方案版本」— 两者缺任一都构造不出政策。
func TestPricePolicyMustBindBothDirectionAndPlan(t *testing.T) {
	live, err := registerable(t, domain.PriceRuleObject, "policy-x", "v1", "sha256:x").
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")

	t.Run("no direction", func(t *testing.T) {
		if _, err := domain.NewCommercialPricePolicy(live, domain.PriceDirectionInvalid, commercialValue(t, domain.NewPricingPlanReference, "plan-1"), scope, mustInterval(t)); !errors.Is(err, domain.ErrInvalidPricePolicy) {
			t.Fatalf("error = %v, want ErrInvalidPricePolicy", err)
		}
	})

	t.Run("no pricing plan", func(t *testing.T) {
		if _, err := domain.NewCommercialPricePolicy(live, domain.SellDirection, domain.PricingPlanReference{}, scope, mustInterval(t)); !errors.Is(err, domain.ErrInvalidPricePolicy) {
			t.Fatalf("error = %v, want ErrInvalidPricePolicy", err)
		}
	})
}

// Covers: CONTEXT「`party-commercial` 不在自身上下文执行价卡、费率表或费用依赖计算」—
// 政策只持有对可执行定价方案的引用，结构上无处放费率或计算结果。
func TestPricePolicyHoldsAReferenceAndPerformsNoCalculation(t *testing.T) {
	policyType := reflect.TypeOf(domain.CommercialPricePolicy{})
	forbidden := []string{"rate", "tariff", "table", "amount", "price", "calculated", "total", "surcharge"}
	for index := 0; index < policyType.NumField(); index++ {
		name := strings.ToLower(policyType.Field(index).Name)
		for _, word := range forbidden {
			if strings.Contains(name, word) {
				t.Fatalf("CommercialPricePolicy carries %s, which pulls rate-card execution into this context",
					policyType.Field(index).Name)
			}
		}
	}
}

// Covers: CONTEXT 商业版本共同不变量 — 政策内容挂在一个当前可用的价格规则版本上。
func TestPricePolicyNeedsAUsablePriceRuleVersion(t *testing.T) {
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")
	plan := commercialValue(t, domain.NewPricingPlanReference, "plan-1")

	t.Run("refuses a draft", func(t *testing.T) {
		draft := commercialDraft(t, domain.PriceRuleObject, "policy-y", "v1", "sha256:y")
		if _, err := domain.NewCommercialPricePolicy(draft, domain.SellDirection, plan, scope, mustInterval(t)); !errors.Is(err, domain.ErrInvalidPricePolicy) {
			t.Fatalf("error = %v, want ErrInvalidPricePolicy", err)
		}
	})

	t.Run("refuses another object kind", func(t *testing.T) {
		contract := contractVersion(t, "contract-7")
		if _, err := domain.NewCommercialPricePolicy(contract, domain.SellDirection, plan, scope, mustInterval(t)); !errors.Is(err, domain.ErrInvalidPricePolicy) {
			t.Fatalf("error = %v, want ErrInvalidPricePolicy", err)
		}
	})
}
