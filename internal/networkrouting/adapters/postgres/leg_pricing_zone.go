package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// RegisterLegPricingZones 追加段计价区域（迁移 0019，ADR-0189 决定二）。写之前先核每一段都挂了成本依据：外键会拦，
// 但撞外键会把调用方的整个事务打进中止态，登记方只拿到一段约束名；先读一遍就能答一个说得清补什么的错误。
func (catalog *NetworkCatalog) RegisterLegPricingZones(
	ctx context.Context,
	tenant domain.TenantID,
	lineCode string,
	version int32,
	zones []ports.LegPricingZone,
) error {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("register leg pricing zones: %w", err)
	}
	bases, err := catalog.LoadLineCostBases(ctx, tenant, lineCode, version)
	if err != nil {
		return fmt.Errorf("register leg pricing zones: %w", err)
	}
	based := make(map[int]bool, len(bases))
	for _, basis := range bases {
		based[basis.SegmentIndex] = true
	}
	for _, zone := range zones {
		if !based[zone.SegmentIndex] {
			return fmt.Errorf("register leg pricing zones: segment %d: %w", zone.SegmentIndex, ports.ErrLegPricingZoneWithoutCostBasis)
		}
	}
	for _, zone := range zones {
		if _, err := executor.Exec(ctx,
			`INSERT INTO network_routing.line_leg_pricing_zone
				(tenant_id, line_code, version, segment_index, zone, basis_ref)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tenant.String(), lineCode, version, zone.SegmentIndex, zone.Zone, zone.Basis,
		); err != nil {
			return fmt.Errorf("register leg pricing zones: %w", err)
		}
	}
	return nil
}

// LoadLegPricingZones 读回一条线路版本已登的段计价区域，按段号升序。没登即空列表不是错误：那几段计价答待判断。
func (catalog *NetworkCatalog) LoadLegPricingZones(
	ctx context.Context,
	tenant domain.TenantID,
	lineCode string,
	version int32,
) ([]ports.LegPricingZone, error) {
	querier, err := catalog.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load leg pricing zones: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT segment_index, zone, basis_ref
		   FROM network_routing.line_leg_pricing_zone
		  WHERE tenant_id = $1 AND line_code = $2 AND version = $3
		  ORDER BY segment_index`,
		tenant.String(), lineCode, version,
	)
	if err != nil {
		return nil, fmt.Errorf("load leg pricing zones: %w", err)
	}
	defer rows.Close()
	zones := make([]ports.LegPricingZone, 0, 3)
	for rows.Next() {
		var zone ports.LegPricingZone
		if err := rows.Scan(&zone.SegmentIndex, &zone.Zone, &zone.Basis); err != nil {
			return nil, fmt.Errorf("load leg pricing zones: %w", err)
		}
		zones = append(zones, zone)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load leg pricing zones: %w", err)
	}
	return zones, nil
}
