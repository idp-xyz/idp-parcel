package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PriceCardRegistrationOutcome 是价卡登记的写入结果代数。冲突是业务答案不是错误
// （ADR-0031 的写入代数取向）：登记册拒绝静默替换，但事务保持可用，调用方拿答案
// 继续作治理处置。
type PriceCardRegistrationOutcome uint8

const (
	PriceCardRegistrationOutcomeInvalid PriceCardRegistrationOutcome = iota
	// PriceCardRegistered：新版本已入册。
	PriceCardRegistered
	// PriceCardAlreadyRegistered：同版本引用、同规范化版本、同内容摘要已在册——
	// 重复登记是幂等重放，不是错误。
	PriceCardAlreadyRegistered
	// PriceCardContentConflict：版本引用相同、规范化版本也相同而内容摘要不同。
	// CONTEXT 判为版本内容冲突，不得继续重放或静默替换；原行保持原样。
	PriceCardContentConflict
	// PriceCardCanonicalizationDiffers：同版本引用已按另一套规范化版本在册。摘要
	// 只在同一规范化版本内可比（ADR-0014），这不是版本内容冲突，也不是幂等重放，
	// 由治理责任方裁决续办；原行保持原样。
	PriceCardCanonicalizationDiffers
)

// PriceCardCatalog 是价卡版本仓储的存取口（票 07 的机制三件之①②）。它拥有已发布
// 定价方案版本的登记行与按（方向 + 适用范围 + 计价基准时点）的装载，不做评价、不选
// 商业口径——内容全属实例半边，本口只承载形状。
type PriceCardCatalog interface {
	// Register 登记一份已批准发布的价卡版本。行只增不改；同键重复按（规范化版本 +
	// 内容摘要)比对译成结果代数，原行永不被顶替。
	Register(ctx context.Context, registration domain.PriceCardRegistration) (PriceCardRegistrationOutcome, error)
	// LoadApplicable 取该租户在（方向 + 适用范围 + 计价基准时点）下的全部适用价卡
	// 版本，供评价用例消费。多份适用价卡是正当形态（每个候选形成自己的评价，择优
	// 归 network-routing）；同一方案身份两版同时适用则交回错误——挑任何一版都是替
	// 发布责任方作它没作的决定。
	LoadApplicable(ctx context.Context, tenant domain.TenantID, direction domain.PricingDirection, scope domain.PricingScopeID, asOf time.Time) ([]domain.PricingPlanVersion, error)
}
