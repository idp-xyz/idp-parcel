package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

var _ ports.PriceCardOverlapFinder = (*PriceCards)(nil)

// FindOverlappingInForce 列出与候选卡同时适用的在册版本（ports.PriceCardOverlapFinder）。
// 发布适用期只做交集的必要条件预筛；截止与退役折进选用区间只经 selectablePriceCardRow.selection，与 ResolveInForce 同一套列，
// 不另写第二套。不重建方案：冲突提示要的是「还有哪些版」，一张坏快照不该把这个事实报成读回失败。
func (catalog *PriceCards) FindOverlappingInForce(
	ctx context.Context,
	tenant domain.TenantID,
	plan domain.VersionReference,
	scope domain.PricingScopeID,
	direction domain.PricingDirection,
	purpose domain.PricingPurpose,
	period domain.EffectivePeriod,
	products []domain.ServiceProductReference,
) ([]domain.PriceCardOverlap, error) {
	if tenant.String() == "" || plan.ID() == "" || plan.Version() == "" || scope.String() == "" ||
		direction.String() == "" || purpose.String() == "" || period.StartsAt().IsZero() {
		return nil, fmt.Errorf("find overlapping price cards: tenant, plan, scope, direction, purpose and period are required")
	}
	if end := period.EndsAt(); !end.IsZero() && !end.After(period.StartsAt()) {
		return nil, fmt.Errorf("find overlapping price cards: period does not hold")
	}
	querier, err := catalog.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find overlapping price cards: %w", err)
	}
	query := `SELECT ` + priceCardRowColumns + `, ` + priceCardClosureColumns + `
		   FROM parcel_pricing.price_card_version v
		  WHERE tenant_id = $1 AND scope = $2 AND direction = $3 AND purpose = $4
		    AND (effective_to IS NULL OR effective_to > $5)`
	args := []any{tenant.String(), scope.String(), direction.String(), purpose.String(), period.StartsAt().UTC()}
	if end := period.EndsAt(); !end.IsZero() {
		query += `
		    AND effective_from < $6`
		args = append(args, end.UTC())
	}
	query += `
		  ORDER BY plan_id, plan_version`
	rows, err := querier.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find overlapping price cards: %w", err)
	}
	defer rows.Close()

	overlaps := make([]domain.PriceCardOverlap, 0)
	for rows.Next() {
		selectable, err := scanSelectablePriceCardRow(rows)
		if err != nil {
			return nil, fmt.Errorf("find overlapping price cards: %w", err)
		}
		selection, selects, err := selectable.selection()
		if err != nil {
			return nil, fmt.Errorf("find overlapping price cards: %w", err)
		}
		if !selects {
			continue
		}
		registered, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, selectable.planID, selectable.planVersion)
		if err != nil {
			return nil, fmt.Errorf("find overlapping price cards: %s/%s: %w", selectable.planID, selectable.planVersion, err)
		}
		registeredProducts, err := serviceProductReferences(selectable.serviceProducts)
		if err != nil {
			return nil, fmt.Errorf("find overlapping price cards: %s/%s: %w", selectable.planID, selectable.planVersion, err)
		}
		overlap, ok := domain.ClassifyPriceCardOverlap(plan, period, products, registered, selection, registeredProducts)
		if ok {
			overlaps = append(overlaps, overlap)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find overlapping price cards: %w", err)
	}
	return overlaps, nil
}

// serviceProductReferences 只把列面上的产品标识译成引用，好跟候选卡比是不是同一层。
// 不走 priceCardRow.plan：那一道整图重验会让无关的坏快照挡住冲突提示。
func serviceProductReferences(values []string) ([]domain.ServiceProductReference, error) {
	if len(values) == 0 {
		return nil, nil
	}
	products := make([]domain.ServiceProductReference, 0, len(values))
	for _, value := range values {
		product, err := domain.NewServiceProductReference(value)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, nil
}
