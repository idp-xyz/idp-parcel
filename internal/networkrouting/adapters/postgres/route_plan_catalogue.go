package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// RoutePlanCatalogue 实现路由判断两册的列表读端口（票 admin-skeleton-closure-batch/03，
// 形状照 ADR-0077）。与 NetworkCatalog 分立成型：那边上列的是主数据目录（七族定义版
// 本），这边上列的是判断库（业务事实）——写入方是渠道墙后的路由编排，没有登记 CLI，
// 业务事实不造种子，读面各自成器不互相搭车（先例：parcelpricing.EvaluationCatalogue
// 与 OperationsCatalogue 同一分法）。
//
// 只读：上列不重建判断、不选版不折叠。计划本体/无路可走/改路决定住在 jsonb 内，
// 权威读法归各判断口，本口只转写检索列。租户条件进每条语句（ADR-0003）；
// plan_applicability 无租户列，其行只经本租户判断行的计划版本联查得出——联查键是
// 全局唯一的计划版本标识，不构成跨租读路（见 ports.RoutePlanCatalogueRead 口面注）。
type RoutePlanCatalogue struct {
	db *bentopg.DB
}

func NewRoutePlanCatalogue(db *bentopg.DB) (*RoutePlanCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &RoutePlanCatalogue{db: db}, nil
}

var _ ports.RoutePlanCatalogueRead = (*RoutePlanCatalogue)(nil)

// routePlanListLimit 判据与 NetworkCatalog.listQuerier 同款：limit 非正是调用方编程
// 错误，静默答一页会把「忘了传」变成一个没人决定过的页大小（ADR-0077 Decision 五）。
func routePlanListLimit(operation string, limit int) error {
	if limit < 1 {
		return fmt.Errorf("%s: limit must be positive, got %d", operation, limit)
	}
	return nil
}

// ListInitialRoutes 上列初始路由判断册的检索列面，适用性经计划版本 LEFT JOIN——
// 无路可走行与尚未登记适用性的计划行，适用性组如实缺席。排序以落册时间倒序、同刻
// 按完整判断键正序收尾，保证分页可重复。
func (catalogue *RoutePlanCatalogue) ListInitialRoutes(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.InitialRouteCatalogueRow, error) {
	if err := routePlanListLimit("list initial routes", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list initial routes: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT r.customer_account_id, r.shipment_request_id, r.acceptance_baseline,
		        r.declared_parcel_id, r.service_purpose, r.conclusion, r.plan_version,
		        a.state, a.basis, a.successor, a.transitioned_at, r.recorded_at
		   FROM network_routing.initial_route r
		   LEFT JOIN network_routing.plan_applicability a ON a.plan_id = r.plan_version
		  WHERE r.tenant_id = $1
		  ORDER BY r.recorded_at DESC, r.customer_account_id, r.shipment_request_id,
		           r.acceptance_baseline, r.declared_parcel_id, r.service_purpose
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list initial routes: %w", err)
	}
	defer rows.Close()

	judgments := make([]ports.InitialRouteCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.InitialRouteCatalogueRow
		var planVersion, state, basis, successor *string
		var transitionedAt *time.Time
		if err := rows.Scan(
			&row.CustomerAccountID, &row.ShipmentRequestID, &row.AcceptanceBaseline,
			&row.DeclaredParcelID, &row.ServicePurpose, &row.Conclusion, &planVersion,
			&state, &basis, &successor, &transitionedAt, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list initial routes: %w", err)
		}
		if planVersion != nil {
			row.PlanVersion, row.HasPlanVersion = *planVersion, true
		}
		// 适用性组按在场与否成组翻译：state 与 transitioned_at 同 NOT NULL 由表面
		// 保证；basis/successor 的逐态矩阵由 CHECK 钉住，照实转写不补半边。
		if state != nil && transitionedAt != nil {
			row.HasApplicability = true
			row.ApplicabilityState = *state
			row.ApplicabilityChangedAt = *transitionedAt
			if basis != nil {
				row.ApplicabilityBasis, row.HasApplicabilityBasis = *basis, true
			}
			if successor != nil {
				row.ApplicabilitySuccessor, row.HasSuccessor = *successor, true
			}
		}
		judgments = append(judgments, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list initial routes: %w", err)
	}
	return judgments, nil
}

// ListRouteReassessments 上列路由复核册的检索列面。排序以落册时间倒序、同刻按
// 触发关联正序收尾。
func (catalogue *RoutePlanCatalogue) ListRouteReassessments(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.RouteReassessmentCatalogueRow, error) {
	if err := routePlanListLimit("list route reassessments", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list route reassessments: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT correlation_id, customer_account_id, shipment_request_id,
		        acceptance_baseline, declared_parcel_id, service_purpose, conclusion,
		        reviewed_plan, lapse_basis, candidate_state, reroute_state,
		        reassessed_at, recorded_at
		   FROM network_routing.route_reassessment
		  WHERE tenant_id = $1
		  ORDER BY recorded_at DESC, correlation_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list route reassessments: %w", err)
	}
	defer rows.Close()

	reassessments := make([]ports.RouteReassessmentCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.RouteReassessmentCatalogueRow
		var reviewedPlan, lapseBasis, candidateState, rerouteState *string
		if err := rows.Scan(
			&row.CorrelationID, &row.CustomerAccountID, &row.ShipmentRequestID,
			&row.AcceptanceBaseline, &row.DeclaredParcelID, &row.ServicePurpose,
			&row.Conclusion, &reviewedPlan, &lapseBasis, &candidateState, &rerouteState,
			&row.ReassessedAt, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list route reassessments: %w", err)
		}
		if reviewedPlan != nil {
			row.ReviewedPlan, row.HasReviewedPlan = *reviewedPlan, true
		}
		if lapseBasis != nil {
			row.LapseBasis, row.HasLapseBasis = *lapseBasis, true
		}
		if candidateState != nil {
			row.CandidateState, row.HasCandidateState = *candidateState, true
		}
		if rerouteState != nil {
			row.RerouteState, row.HasRerouteState = *rerouteState, true
		}
		reassessments = append(reassessments, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list route reassessments: %w", err)
	}
	return reassessments, nil
}
