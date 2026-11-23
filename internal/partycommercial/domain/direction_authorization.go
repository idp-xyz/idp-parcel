package domain

import "time"

// 价卡登记来核它引用的那一版授权规则时，这一版在登记时刻的位置。方向不在授权规则自己身上：
// 规则指名一份商业价格政策，方向是那份政策的方向。这里只回答「这一版此刻算不算一份能读出
// 唯一方向的授权」，不替计价领域保存第二份方向。

type DirectionAuthorizationStanding uint8

const (
	DirectionAuthorizationStandingInvalid DirectionAuthorizationStanding = iota
	// DirectionAuthorizationNotPublished：不在册，或还是草稿。草稿不入版本册，与「没有这一版」同一格。
	DirectionAuthorizationNotPublished
	// DirectionAuthorizationNotInForce：已发布，但登记时刻不在选用区间内，或已经收尾（到期、退役、替代）。
	// 选用区间有更正时用更正后的那一段，没有则用版本自己声明的有效区间。
	DirectionAuthorizationNotInForce
	// DirectionAuthorizationDirectionUnreadable：规则仍在效力内，但没有指名一份此刻仍在效力内、
	// 且各版方向一致的商业价格政策。调用方把它译成方向不符——对不上价卡的方向，包括对不上任何方向。
	DirectionAuthorizationDirectionUnreadable
	// DirectionAuthorizationAuthorizes：已发布、登记时刻落在选用区间内、没被收尾，指名的价格政策给出唯一方向。
	DirectionAuthorizationAuthorizes
)

func (standing DirectionAuthorizationStanding) String() string {
	switch standing {
	case DirectionAuthorizationNotPublished:
		return "NOT_PUBLISHED"
	case DirectionAuthorizationNotInForce:
		return "NOT_IN_FORCE"
	case DirectionAuthorizationDirectionUnreadable:
		return "DIRECTION_UNREADABLE"
	case DirectionAuthorizationAuthorizes:
		return "AUTHORIZES"
	default:
		return ""
	}
}

// DirectionAuthorizationFact 是 JudgeCitedDirectionAuthorization 的答复。Direction 只在
// Standing 为 DirectionAuthorizationAuthorizes 时有意义。
type DirectionAuthorizationFact struct {
	standing  DirectionAuthorizationStanding
	direction PriceDirection
}

func (fact DirectionAuthorizationFact) Standing() DirectionAuthorizationStanding {
	return fact.standing
}

func (fact DirectionAuthorizationFact) Direction() PriceDirection { return fact.direction }

// CitedAuthorizationInForce 回答这一版在 at 是否仍可作为新登记的授权：已发布或已生效，
// 选用区间含 at，且不是三种收尾态。已发布但尚未取效的版本，只要选用区间已经含 at，也算在内——
// 发布不等于生效，但价卡核的是「已发布且时刻落在区间内、没被收尾」。
func CitedAuthorizationInForce(version CommercialVersion, selection EffectiveInterval, at time.Time) bool {
	switch version.Status() {
	case CommercialVersionPublished, CommercialVersionEffective:
		return selection.Contains(at)
	default:
		return false
	}
}

// JudgeCitedDirectionAuthorization 核价卡引用的那一版授权规则。found 为假即不在册。
// selection 是选用区间（有区间更正时用更正后的那段）。policyDirections 是它指名的价格政策
// 在同一时刻仍在效力内的各版方向；没指名、或没有这样的版本时为空。at 为零是调用方的错。
func JudgeCitedDirectionAuthorization(
	found bool,
	version CommercialVersion,
	selection EffectiveInterval,
	at time.Time,
	policyDirections []PriceDirection,
) (DirectionAuthorizationFact, error) {
	if at.IsZero() {
		return DirectionAuthorizationFact{}, ErrInvalidCommercialVersion
	}
	if !found {
		return DirectionAuthorizationFact{standing: DirectionAuthorizationNotPublished}, nil
	}
	if !CitedAuthorizationInForce(version, selection, at) {
		switch version.Status() {
		case CommercialVersionPublished, CommercialVersionEffective,
			CommercialVersionExpired, CommercialVersionRetired, CommercialVersionSuperseded:
			return DirectionAuthorizationFact{standing: DirectionAuthorizationNotInForce}, nil
		default:
			return DirectionAuthorizationFact{standing: DirectionAuthorizationNotPublished}, nil
		}
	}
	seen := map[PriceDirection]struct{}{}
	var only PriceDirection
	for _, direction := range policyDirections {
		if !direction.valid() {
			continue
		}
		seen[direction] = struct{}{}
		only = direction
	}
	if len(seen) != 1 {
		return DirectionAuthorizationFact{standing: DirectionAuthorizationDirectionUnreadable}, nil
	}
	return DirectionAuthorizationFact{standing: DirectionAuthorizationAuthorizes, direction: only}, nil
}
