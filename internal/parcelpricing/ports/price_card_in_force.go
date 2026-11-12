package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PriceCardInForceOutcome 是在用价卡解析的封闭结果（票 sa-cc/11 裁决 3）。三格按恢复动作分
// （ADR-0029 同一判据）：解析到了就形成评价；没有卡要去登记一张；多于一张要人裁——本口唯一的先后是
// ADR-0382 的指名优先（指名申报产品的卡先于不声明产品的卡，见 PriceCardInForceResolver），此外不种任何
// 「最新优先」「先登记优先」之类的选择规则，那是实例半边与 owner 语言题，替租户挑一张卡就是替发布
// 责任方作它没作的决定。
type PriceCardInForceOutcome uint8

const (
	PriceCardInForceOutcomeInvalid PriceCardInForceOutcome = iota
	// PriceCardVersionInForce：（租户、范围、方向、目的、时点）与申报产品取定的那一层里恰有一版适用，方案随答案交回。
	PriceCardVersionInForce
	// PriceCardNotConfigured：零命中。诚实停——缺的是价卡，不是评价算不出来。
	PriceCardNotConfigured
	// PriceCardApplicabilityConflict：多于一版同时适用（不同方案身份，或同一方案两版）。候选随答案
	// 交回供人裁，方案一格为空。
	PriceCardApplicabilityConflict
)

func (outcome PriceCardInForceOutcome) String() string {
	switch outcome {
	case PriceCardVersionInForce:
		return "IN_FORCE"
	case PriceCardNotConfigured:
		return "NOT_CONFIGURED"
	case PriceCardApplicabilityConflict:
		return "APPLICABILITY_CONFLICT"
	default:
		return ""
	}
}

// PriceCardInForceResolution 是一次解析的答案。Plan 只在 PriceCardVersionInForce 时有值；Candidates
// 只在 PriceCardApplicabilityConflict 时有值，列的是同时适用的每一版的引用，让裁的人知道在哪几张
// 之间裁。
type PriceCardInForceResolution struct {
	Outcome    PriceCardInForceOutcome
	Plan       domain.PricingPlanVersion
	Candidates []domain.VersionReference
}

// PriceCardInForceResolver 按（租户、范围、方向、目的、时点）与申报的服务产品在价卡登记册上解析在用版本。
//
// 与 PriceCardCatalog.LoadApplicable 分开立：那一口交回全部适用候选，是给「多份供应商价卡各形成
// 一份 BUY 评价、择优归 network-routing」那条路用的，多份适用在它那里是正当形态；这一口回答的是
// 「按一份评价请求该用哪一版」，多份适用在这里是要人裁的冲突。两问的答案形不同，合成一口就得
// 让调用方自己数候选，而那正是把裁决规则散进调用方的开头。
//
// 读 PricingPlanVersion 上的范围 / 方向 / 目的 / 有效期与适用服务产品。产品是最后一个入参，零值即委托没申报；选法是
// ADR-0382 决定二的指名优先：时点在用、指名这个产品的卡是一层，不声明产品的卡是另一层，前一层有卡就只看它，没有才看
// 后一层；指名了别的产品的卡哪一层都不进，零值只看后一层。取定的那一层里恰一版即它，多于一版答适用冲突，后一层一版都
// 没有答未配置。注意这是两层之间的指名优先，不是在同一层的几张卡之间挑一张——同一层多于一版照旧交人裁。方向与目的今天
// 一一配对，两者都收是让谓词与登记行的列面同形——配错的一对零命中，答未配置而不是猜。
type PriceCardInForceResolver interface {
	ResolveInForce(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.PricingScopeID,
		direction domain.PricingDirection,
		purpose domain.PricingPurpose,
		at time.Time,
		product domain.ServiceProductReference,
	) (PriceCardInForceResolution, error)
}
