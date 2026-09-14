package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

var _ ports.PriceCardInForceResolver = (*PriceCards)(nil)

// ResolveInForce 按（租户、范围、方向、目的、时点）解析在用价卡版本（ports.PriceCardInForceResolver，
// 票 sa-cc/11 裁决 3）。谓词与 LoadApplicable 同源再多一格目的；选版同按 [effective_from, effective_to)
// 含 at。三格在 SQL 之外分：零行答未配置，一行经 priceCardRow.plan 整图重验后交回，多于一行只把每一行
// 的方案引用列成候选——冲突这一格不重建任何一版方案，因为没有任何一版会被采用，重建只会让一张坏
// 快照把「有两张卡」这个事实报成读回失败。
//
// 同一方案身份两版同时适用在 LoadApplicable 是 ErrAmbiguousPriceCard，在这里是候选里的两条：那一口
// 交回的是候选列表，多候选正当、同身份两版才是异常；这一口只要一版，多于一版无论身份同不同都是
// 「要人裁」，按封闭结果答而不是按错误答，调用方才能把它落成显式未决。
func (catalog *PriceCards) ResolveInForce(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.PricingScopeID,
	direction domain.PricingDirection,
	purpose domain.PricingPurpose,
	at time.Time,
) (ports.PriceCardInForceResolution, error) {
	none := ports.PriceCardInForceResolution{}
	if tenant.String() == "" || scope.String() == "" || direction.String() == "" || purpose.String() == "" || at.IsZero() {
		return none, fmt.Errorf("resolve in-force price card: tenant, scope, direction, purpose and at are required")
	}
	querier, err := catalog.db.ReadExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("resolve in-force price card: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT `+priceCardRowColumns+`
		   FROM parcel_pricing.price_card_version
		  WHERE tenant_id = $1 AND scope = $2 AND direction = $3 AND purpose = $4
		    AND effective_from <= $5
		    AND (effective_to IS NULL OR effective_to > $5)
		  ORDER BY plan_id, plan_version`,
		tenant.String(), scope.String(), direction.String(), purpose.String(), at.UTC(),
	)
	if err != nil {
		return none, fmt.Errorf("resolve in-force price card: %w", err)
	}
	defer rows.Close()

	var applicable []priceCardRow
	for rows.Next() {
		row, err := scanPriceCardRow(rows)
		if err != nil {
			return none, fmt.Errorf("resolve in-force price card: %w", err)
		}
		applicable = append(applicable, row)
	}
	if err := rows.Err(); err != nil {
		return none, fmt.Errorf("resolve in-force price card: %w", err)
	}

	switch len(applicable) {
	case 0:
		return ports.PriceCardInForceResolution{Outcome: ports.PriceCardNotConfigured}, nil
	case 1:
		plan, err := applicable[0].plan(tenant)
		if err != nil {
			return none, fmt.Errorf("resolve in-force price card: %w", err)
		}
		return ports.PriceCardInForceResolution{Outcome: ports.PriceCardVersionInForce, Plan: plan}, nil
	default:
		candidates := make([]domain.VersionReference, 0, len(applicable))
		for _, row := range applicable {
			reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, row.planID, row.planVersion)
			if err != nil {
				return none, fmt.Errorf("resolve in-force price card: candidate %s/%s: %w", row.planID, row.planVersion, err)
			}
			candidates = append(candidates, reference)
		}
		return ports.PriceCardInForceResolution{Outcome: ports.PriceCardApplicabilityConflict, Candidates: candidates}, nil
	}
}
