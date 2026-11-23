package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// DirectionAuthorizationAssessment 是消费侧读口对「这张卡引用的授权，在登记时刻授权的方向
// 与卡的方向是不是同一个」的答复。读不回不是这里的一格，那是 error。
type DirectionAuthorizationAssessment uint8

const (
	DirectionAuthorizationAssessmentInvalid DirectionAuthorizationAssessment = iota
	// DirectionAuthorizationMatches：引用的是一版已发布、登记时刻仍在效力内、没被收尾的授权规则，
	// 它指名的商业价格政策的方向与价卡方向相同。
	DirectionAuthorizationMatches
	// DirectionAuthorizationNotPublished：授权不在册，或还没发布。
	DirectionAuthorizationNotPublished
	// DirectionAuthorizationNotInForce：已到期、已收尾，或登记时刻不在有效区间内。
	DirectionAuthorizationNotInForce
	// DirectionAuthorizationDirectionMismatch：读得到规则，但它授权的方向与价卡方向不同，
	// 包括它没有指名一份此刻能读出方向的商业价格政策。
	DirectionAuthorizationDirectionMismatch
)

func (assessment DirectionAuthorizationAssessment) String() string {
	switch assessment {
	case DirectionAuthorizationMatches:
		return "MATCHES"
	case DirectionAuthorizationNotPublished:
		return "NOT_PUBLISHED"
	case DirectionAuthorizationNotInForce:
		return "NOT_IN_FORCE"
	case DirectionAuthorizationDirectionMismatch:
		return "DIRECTION_MISMATCH"
	default:
		return ""
	}
}

// DirectionAuthorizationQuery 是一次核对：租户、价卡引用的授权、价卡自己的方向、登记时刻。
type DirectionAuthorizationQuery struct {
	Tenant        domain.TenantID
	Authorization domain.VersionReference
	Direction     domain.PricingDirection
	At            time.Time
}

// DirectionAuthorizationReader 是计价对参与方与商业的消费侧口：只在价卡入册前（以及预览）
// 核对卡上那条价格方向授权。评价不走这只口。方向从对方指名的商业价格政策读回来，本上下文不另存一份。
type DirectionAuthorizationReader interface {
	ReadDirectionAuthorization(ctx context.Context, query DirectionAuthorizationQuery) (DirectionAuthorizationAssessment, error)
}
