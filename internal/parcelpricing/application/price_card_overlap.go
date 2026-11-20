package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 预览、存草稿、批准与发布共用这一段：重叠只进答复，不改那一格结果。
// 把重叠收成拒绝，后继就登不进去——旧版只经指名后继的显式收尾离开新的选择（ADR-0234）。

// overlapsOnPlan 问这张卡折完收尾之后还会和哪些在册版本同时适用。
// 查找口没装时交回 nil，答复里这一格缺席。读册失败是依赖故障，不是「没有重叠」。
func overlapsOnPlan(
	ctx context.Context,
	finder ports.PriceCardOverlapFinder,
	tenant domain.TenantID,
	plan domain.PricingPlanVersion,
) ([]domain.PriceCardOverlap, error) {
	if finder == nil {
		return nil, nil
	}
	return finder.FindOverlappingInForce(
		ctx, tenant, plan.Reference(), plan.Scope(), plan.Direction(), plan.Purpose(),
		plan.EffectivePeriod(), plan.ServiceProducts(),
	)
}

// attachOverlaps 只在已校验时去问。带问题或整份不收还没有一张立得住的卡，问了也对不上随后的选卡。
func attachOverlaps(ctx context.Context, finder ports.PriceCardOverlapFinder, tenant domain.TenantID, preview *PriceCardImportPreview) error {
	if finder == nil || preview.Outcome != PriceCardImportValidated || preview.Content == nil {
		return nil
	}
	found, err := overlapsOnPlan(ctx, finder, tenant, preview.Content.Plan)
	if err != nil {
		return err
	}
	preview.Overlaps = found
	return nil
}
