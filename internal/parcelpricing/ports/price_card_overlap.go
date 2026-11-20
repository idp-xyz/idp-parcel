package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PriceCardOverlapFinder 列出与这张卡适用期有交、折完收尾后仍可选、并且会和它落在同一产品层的在册版本。
// 空切片是没有这样的版本，不是故障。调用方只拿它提示；重叠不拦登记，后继必须先入册才能收尾旧版（ADR-0234）。
type PriceCardOverlapFinder interface {
	FindOverlappingInForce(
		ctx context.Context,
		tenant domain.TenantID,
		plan domain.VersionReference,
		scope domain.PricingScopeID,
		direction domain.PricingDirection,
		purpose domain.PricingPurpose,
		period domain.EffectivePeriod,
		products []domain.ServiceProductReference,
	) ([]domain.PriceCardOverlap, error)
}
