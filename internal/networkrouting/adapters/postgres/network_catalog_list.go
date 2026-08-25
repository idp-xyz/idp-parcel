package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件是 NetworkCatalog 的运营查阅上列半边（ports.OperationsCatalogRead，ADR-0077）：
// 上列读的就是这份目录库，不是第二份数据（先例：visibilityexception 的 Projections
// 同一适配器一并实现运营读面）。
//
// 与 LoadDefinitionsAt 的分工：那边按 asOf 选版、拒歧义、带修订，是判断依据的取数口；
// 这边按族列版本行**原文**——含已闭区间的历史版、尚未生效的未来版与整条调整历史链，
// 不选版不折叠，两版区间重叠在这里不是错误（上列如实透出，修目录的人正需要看见它们）。
// 修订行不参与：「登记过与否」的分辨器属证据端口语义（ADR-0052），目录上列这一格
// 空表本身就是内容（ADR-0077 Decision 四），空族如实答空列表。
//
// 排序一律身份升序、同一身份内版本倒序（最新版在前），保证分页可重复；limit 非正是
// 调用方编程错误——静默答一页会把「忘了传」变成一个没人决定过的页大小。
var _ ports.OperationsCatalogRead = (*NetworkCatalog)(nil)

func (catalog *NetworkCatalog) ListNodeVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.NodeDefinitionVersion, error) {
	querier, err := catalog.listQuerier(ctx, "list node versions", limit)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT node_code, version, business_timezone, effective_from, effective_to
		   FROM network_routing.logistics_node_version
		  WHERE tenant_id = $1
		  ORDER BY node_code, version DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list node versions: %w", err)
	}
	defer rows.Close()

	versions := make([]ports.NodeDefinitionVersion, 0, limit)
	for rows.Next() {
		var row ports.NodeDefinitionVersion
		var effectiveTo *time.Time
		if err := rows.Scan(&row.Code, &row.Version, &row.BusinessTimezone,
			&row.EffectiveFrom, &effectiveTo); err != nil {
			return nil, fmt.Errorf("list node versions: %w", err)
		}
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		versions = append(versions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list node versions: %w", err)
	}
	return versions, nil
}

func (catalog *NetworkCatalog) ListConnectionVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ConnectionDefinitionVersion, error) {
	querier, err := catalog.listQuerier(ctx, "list connection versions", limit)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT connection_code, version, from_node_code, to_node_code,
		        business_timezone, effective_from, effective_to
		   FROM network_routing.network_connection_version
		  WHERE tenant_id = $1
		  ORDER BY connection_code, version DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list connection versions: %w", err)
	}
	defer rows.Close()

	versions := make([]ports.ConnectionDefinitionVersion, 0, limit)
	for rows.Next() {
		var row ports.ConnectionDefinitionVersion
		var effectiveTo *time.Time
		if err := rows.Scan(&row.Code, &row.Version, &row.FromNode, &row.ToNode,
			&row.BusinessTimezone, &row.EffectiveFrom, &effectiveTo); err != nil {
			return nil, fmt.Errorf("list connection versions: %w", err)
		}
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		versions = append(versions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list connection versions: %w", err)
	}
	return versions, nil
}

func (catalog *NetworkCatalog) ListLineVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.LineDefinitionVersion, error) {
	querier, err := catalog.listQuerier(ctx, "list line versions", limit)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT line_code, version, segments, business_timezone, applicable_scope,
		        effective_from, effective_to
		   FROM network_routing.line_version
		  WHERE tenant_id = $1
		  ORDER BY line_code, version DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list line versions: %w", err)
	}
	defer rows.Close()

	versions := make([]ports.LineDefinitionVersion, 0, limit)
	for rows.Next() {
		var row ports.LineDefinitionVersion
		var segmentsRaw []byte
		var effectiveTo *time.Time
		if err := rows.Scan(&row.Code, &row.Version, &segmentsRaw, &row.BusinessTimezone,
			&row.ApplicableScope, &row.EffectiveFrom, &effectiveTo); err != nil {
			return nil, fmt.Errorf("list line versions: %w", err)
		}
		if err := json.Unmarshal(segmentsRaw, &row.Segments); err != nil {
			return nil, fmt.Errorf("list line versions: 译回段链：%w", err)
		}
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		versions = append(versions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list line versions: %w", err)
	}
	return versions, nil
}

func (catalog *NetworkCatalog) ListServiceAreaVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ServiceAreaDefinitionVersion, error) {
	querier, err := catalog.listQuerier(ctx, "list service area versions", limit)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT area_code, version, effective_from, effective_to
		   FROM network_routing.service_area_version
		  WHERE tenant_id = $1
		  ORDER BY area_code, version DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list service area versions: %w", err)
	}
	defer rows.Close()

	versions := make([]ports.ServiceAreaDefinitionVersion, 0, limit)
	for rows.Next() {
		var row ports.ServiceAreaDefinitionVersion
		var effectiveTo *time.Time
		if err := rows.Scan(&row.Code, &row.Version, &row.EffectiveFrom, &effectiveTo); err != nil {
			return nil, fmt.Errorf("list service area versions: %w", err)
		}
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		versions = append(versions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list service area versions: %w", err)
	}
	return versions, nil
}

func (catalog *NetworkCatalog) ListServiceCalendarVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ServiceCalendarDefinitionVersion, error) {
	querier, err := catalog.listQuerier(ctx, "list service calendar versions", limit)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT target_kind, target_code, version, effective_from, effective_to
		   FROM network_routing.service_calendar_version
		  WHERE tenant_id = $1
		  ORDER BY target_kind, target_code, version DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list service calendar versions: %w", err)
	}
	defer rows.Close()

	versions := make([]ports.ServiceCalendarDefinitionVersion, 0, limit)
	for rows.Next() {
		var row ports.ServiceCalendarDefinitionVersion
		var kindRaw string
		var effectiveTo *time.Time
		if err := rows.Scan(&kindRaw, &row.TargetCode, &row.Version,
			&row.EffectiveFrom, &effectiveTo); err != nil {
			return nil, fmt.Errorf("list service calendar versions: %w", err)
		}
		kind, err := ports.CatalogTargetKindFrom(kindRaw)
		if err != nil {
			return nil, fmt.Errorf("list service calendar versions: %w", err)
		}
		row.TargetKind = kind
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		versions = append(versions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list service calendar versions: %w", err)
	}
	return versions, nil
}

func (catalog *NetworkCatalog) ListAvailabilityAdjustments(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.AvailabilityAdjustmentStatement, error) {
	querier, err := catalog.listQuerier(ctx, "list availability adjustments", limit)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT adjustment_code, version, target_kind, target_code,
		        adjustment_kind, source, effective_at, lifted_at
		   FROM network_routing.availability_adjustment
		  WHERE tenant_id = $1
		  ORDER BY adjustment_code, version DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list availability adjustments: %w", err)
	}
	defer rows.Close()

	statements := make([]ports.AvailabilityAdjustmentStatement, 0, limit)
	for rows.Next() {
		var row ports.AvailabilityAdjustmentStatement
		var targetKindRaw, adjustmentKindRaw string
		var liftedAt *time.Time
		if err := rows.Scan(&row.Code, &row.Version, &targetKindRaw, &row.TargetCode,
			&adjustmentKindRaw, &row.Source, &row.EffectiveAt, &liftedAt); err != nil {
			return nil, fmt.Errorf("list availability adjustments: %w", err)
		}
		targetKind, err := ports.CatalogTargetKindFrom(targetKindRaw)
		if err != nil {
			return nil, fmt.Errorf("list availability adjustments: %w", err)
		}
		adjustmentKind, err := ports.AvailabilityAdjustmentKindFrom(adjustmentKindRaw)
		if err != nil {
			return nil, fmt.Errorf("list availability adjustments: %w", err)
		}
		row.TargetKind, row.Kind = targetKind, adjustmentKind
		row.LiftedAt, row.HasLiftedAt = timeOf(liftedAt), liftedAt != nil
		statements = append(statements, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list availability adjustments: %w", err)
	}
	return statements, nil
}

func (catalog *NetworkCatalog) ListRouteStrategyVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.RouteStrategyDefinitionVersion, error) {
	querier, err := catalog.listQuerier(ctx, "list route strategy versions", limit)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT strategy_code, version, applicable_scope, effective_from, effective_to
		   FROM network_routing.route_strategy_version
		  WHERE tenant_id = $1
		  ORDER BY strategy_code, version DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list route strategy versions: %w", err)
	}
	defer rows.Close()

	versions := make([]ports.RouteStrategyDefinitionVersion, 0, limit)
	for rows.Next() {
		var row ports.RouteStrategyDefinitionVersion
		var effectiveTo *time.Time
		if err := rows.Scan(&row.Code, &row.Version, &row.ApplicableScope,
			&row.EffectiveFrom, &effectiveTo); err != nil {
			return nil, fmt.Errorf("list route strategy versions: %w", err)
		}
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		versions = append(versions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list route strategy versions: %w", err)
	}
	return versions, nil
}

// listQuerier 是七个上列方法共用的入口检查：limit 门禁加读执行器。
func (catalog *NetworkCatalog) listQuerier(
	ctx context.Context,
	operation string,
	limit int,
) (bentopg.Querier, error) {
	if limit < 1 {
		return nil, fmt.Errorf("%s: limit must be positive, got %d", operation, limit)
	}
	querier, err := catalog.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return querier, nil
}
