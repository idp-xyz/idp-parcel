package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidPricePolicy      = errors.New("party commercial: invalid commercial price policy")
	ErrInvalidPricePolicyQuery = errors.New("party commercial: invalid price policy query")
	ErrNoApplicablePricePolicy = errors.New("party commercial: no applicable commercial price policy")
	ErrPricePolicyConflict     = errors.New("party commercial: one direction and scope is covered by several price policies")
)

// PricingPlanReference 指向 parcel-pricing 拥有的可执行定价方案版本。
// 它只是一个引用并且始终只是引用：价卡、费率表和费用依赖计算都属于那个上下文，
// 本上下文内不执行任何计价。
type PricingPlanReference struct{ requiredValue }

func NewPricingPlanReference(value string) (PricingPlanReference, error) {
	required, err := newRequiredValue("pricing plan reference", value)
	return PricingPlanReference{required}, err
}

// CommercialPricePolicy 是一个价格规则版本的正文：它授权哪个价格方向、绑定哪个
// 可执行定价方案，以及适用的计价范围与有效区间。
//
// 光有价格规则版本无法计价，因为价格方向与定价方案绑定放在这里而不在版本上。
// 这是有意的：只返回版本的通用解析并没有产出可用的定价依据，价格方向也就跳不过去。
type CommercialPricePolicy struct {
	version   CommercialVersion
	direction PriceDirection
	plan      PricingPlanReference
	scope     CommercialScopeReference
	effective EffectiveInterval
}

func NewCommercialPricePolicy(
	version CommercialVersion,
	direction PriceDirection,
	plan PricingPlanReference,
	scope CommercialScopeReference,
	effective EffectiveInterval,
) (CommercialPricePolicy, error) {
	if version.kind != PriceRuleObject ||
		version.status != CommercialVersionEffective ||
		!direction.valid() || !plan.valid() || !scope.valid() || !effective.valid() {
		return CommercialPricePolicy{}, ErrInvalidPricePolicy
	}
	return CommercialPricePolicy{
		version:   version,
		direction: direction,
		plan:      plan,
		scope:     scope,
		effective: effective,
	}, nil
}

func (policy CommercialPricePolicy) Version() CommercialVersion {
	return policy.version
}

func (policy CommercialPricePolicy) Direction() PriceDirection {
	return policy.direction
}

func (policy CommercialPricePolicy) PricingPlan() PricingPlanReference {
	return policy.plan
}

func (policy CommercialPricePolicy) Scope() CommercialScopeReference {
	return policy.scope
}

func (policy CommercialPricePolicy) covers(query PricePolicyQuery) bool {
	return policy.direction == query.direction &&
		policy.scope == query.scope &&
		policy.effective.Contains(query.at)
}

type PricePolicyQuery struct {
	direction PriceDirection
	scope     CommercialScopeReference
	at        time.Time
}

func NewPricePolicyQuery(
	direction PriceDirection,
	scope CommercialScopeReference,
	at time.Time,
) (PricePolicyQuery, error) {
	if !direction.valid() || !scope.valid() || at.IsZero() {
		return PricePolicyQuery{}, ErrInvalidPricePolicyQuery
	}
	return PricePolicyQuery{direction: direction, scope: scope, at: at.UTC()}, nil
}

// ResolveCommercialPricePolicy 在一个价格方向与计价范围内选出唯一适用的政策。
// 价格方向参与匹配，因此 BUY 政策永远不会回答 SELL 的请求，某个方向没有自己的
// 政策就是没有。
//
// 零候选与多候选都如实报出而不就地裁决：本上下文禁止回落到默认价，而在两条重叠
// 政策里挑一条，正是换个名义做同一件事。
func ResolveCommercialPricePolicy(
	policies []CommercialPricePolicy,
	query PricePolicyQuery,
) (CommercialPricePolicy, error) {
	matches := make([]CommercialPricePolicy, 0, 2)
	for _, policy := range policies {
		if policy.covers(query) {
			matches = append(matches, policy)
		}
	}

	switch len(matches) {
	case 0:
		return CommercialPricePolicy{}, ErrNoApplicablePricePolicy
	case 1:
		return matches[0], nil
	default:
		return CommercialPricePolicy{}, ErrPricePolicyConflict
	}
}
