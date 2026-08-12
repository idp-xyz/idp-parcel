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
	// ErrPriceDirectionBindingConflict 是发布冲突：政策方向与价卡方向不一致，又没有声明转换。
	// 它不是 ErrInvalidPricePolicy——构造参数形状可以都合法，错的是跨向绑定本身（AT-PC-033）。
	ErrPriceDirectionBindingConflict = errors.New("party commercial: price policy direction conflicts with the bound plan without a declared conversion")
	// ErrPricingPlanNotConfirmed 与 ErrPricingPlanWithdrawn 刻意分成两个哨兵：前者要再问一次
	// parcel-pricing，后者要商业责任方改挂一份仍在的方案。压成一个，调用方就只能靠猜该重试
	// 还是该转人工（ADR-0029 同一条道理）。
	//
	// 两者都不是 ErrNoApplicablePricePolicy：这个范围有政策，权威没有说过它没有。
	ErrPricingPlanNotConfirmed = errors.New("party commercial: the bound pricing plan version could not be confirmed adoptable")
	ErrPricingPlanWithdrawn    = errors.New("party commercial: the bound pricing plan version is withdrawn with no replacement")
)

// PlanBindingConversion 声明政策方向与价卡方向不一致时的显式转换。零值 = 未声明：方向必须一致。
//
// CONTEXT：「销售政策可以显式允许引用一次已冻结的采购评价，但不得把当前成本…隐式当作可执行价格。」
type PlanBindingConversion uint8

const (
	PlanBindingConversionNone PlanBindingConversion = iota
	PlanBindingFrozenBuyEvaluation
)

func (conversion PlanBindingConversion) String() string {
	switch conversion {
	case PlanBindingConversionNone:
		return "NONE"
	case PlanBindingFrozenBuyEvaluation:
		return "FROZEN_BUY_EVALUATION"
	default:
		return ""
	}
}

func (conversion PlanBindingConversion) valid() bool {
	return conversion == PlanBindingConversionNone || conversion == PlanBindingFrozenBuyEvaluation
}

// PricingPlanStanding 是 parcel-pricing 对一份定价方案版本此刻还能不能被采用的答复。
// 它只能由那个上下文给：价卡属于它，本上下文只持引用，就地推断等于替它回答。
type PricingPlanStanding uint8

const (
	// PricingPlanStandingInvalid 是零值，意思是没有人回答过，而不是「没问题」。让忘了作答的
	// 调用点落在这里并因此停下，比让它默认通过安全。
	PricingPlanStandingInvalid PricingPlanStanding = iota
	PricingPlanAdoptable
	PricingPlanWithdrawn
)

func (standing PricingPlanStanding) String() string {
	switch standing {
	case PricingPlanAdoptable:
		return "ADOPTABLE"
	case PricingPlanWithdrawn:
		return "WITHDRAWN"
	default:
		return ""
	}
}

// PricingPlanStandingLookup 是问 parcel-pricing 要那份答复的方式。它作为入参出现而不是本包
// 内的一次查询，因为本上下文没有资格自己查价卡。
type PricingPlanStandingLookup func(PricingPlanReference) PricingPlanStanding

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

// NewCommercialPricePolicy 构造一份已生效的商业价格政策。
//
// planDirection 是 parcel-pricing 对这份定价方案版本自身方向的答复，不是本上下文推断的。
// 缺它就无法分辨「SELL 政策绑了 BUY 价卡」——而那正是 AT-PC-033 要挡的隐式跨向。
// conversion 只在方向不一致时有意义：销售政策可显式声明引用一次已冻结的采购评价；
// 未声明则发布冲突。
func NewCommercialPricePolicy(
	version CommercialVersion,
	direction PriceDirection,
	plan PricingPlanReference,
	planDirection PriceDirection,
	conversion PlanBindingConversion,
	scope CommercialScopeReference,
	effective EffectiveInterval,
) (CommercialPricePolicy, error) {
	if version.kind != PriceRuleObject ||
		version.status != CommercialVersionEffective ||
		!direction.valid() || !plan.valid() || !planDirection.valid() ||
		!conversion.valid() || !scope.valid() || !effective.valid() {
		return CommercialPricePolicy{}, ErrInvalidPricePolicy
	}
	if err := checkPlanBinding(direction, planDirection, conversion); err != nil {
		return CommercialPricePolicy{}, err
	}
	return CommercialPricePolicy{
		version:   version,
		direction: direction,
		plan:      plan,
		scope:     scope,
		effective: effective,
	}, nil
}

// checkPlanBinding 拒绝未声明转换的跨向绑定。同向时再声明转换没有意义，也拒——否则
// 「声明了转换」会变成永远为真的装饰字段。
func checkPlanBinding(
	policyDirection PriceDirection,
	planDirection PriceDirection,
	conversion PlanBindingConversion,
) error {
	if policyDirection == planDirection {
		if conversion != PlanBindingConversionNone {
			return ErrInvalidPricePolicy
		}
		return nil
	}
	if policyDirection == SellDirection &&
		planDirection == BuyDirection &&
		conversion == PlanBindingFrozenBuyEvaluation {
		return nil
	}
	return ErrPriceDirectionBindingConflict
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
	standingOf PricingPlanStandingLookup,
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
	default:
		return CommercialPricePolicy{}, ErrPricePolicyConflict
	}

	// 采用之前先问绑定的定价方案还在不在。政策自己仍在有效区间内证明不了这一点：方案由
	// parcel-pricing 拥有，它退役时这份政策一个字节都没变，按方向加范围照样唯一命中。
	adopted := matches[0]
	standing := PricingPlanStandingInvalid
	if standingOf != nil {
		standing = standingOf(adopted.plan)
	}
	switch standing {
	case PricingPlanAdoptable:
		return adopted, nil
	case PricingPlanWithdrawn:
		return CommercialPricePolicy{}, ErrPricingPlanWithdrawn
	default:
		// 零值落在这里：没问到就是没确认，绝不当作可用。缺一个答复与拿到一个「还在」的
		// 答复之间的差别，正是本上下文不许自己替 parcel-pricing 补上的那一个。
		return CommercialPricePolicy{}, ErrPricingPlanNotConfirmed
	}
}
