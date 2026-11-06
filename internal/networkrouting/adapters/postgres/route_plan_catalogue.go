package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// RoutePlanCatalogue 实现路由判断两册的列表读端口（票 admin-skeleton-closure-batch/03，
// 形状照 ADR-0077）。与 NetworkCatalog 分立成型：那边上列的是主数据目录（七族定义版
// 本），这边上列的是判断库（业务事实）——写入方是渠道墙后的路由编排，没有登记 CLI，
// 业务事实不造种子，读面各自成器不互相搭车（先例：parcelpricing.EvaluationCatalogue
// 与 OperationsCatalogue 同一分法）。
//
// 只读：上列不选版不折叠。计划本体/无路可走/改路决定住在 jsonb 内，权威读法归各
// 判断口，本口只转写检索列；成计划行的段链例外（ADR-0217），照判断口的装载纪律重建
// 整份计划后逐段转写，不从 jsonb 里抠字段。租户条件进每条语句（ADR-0003）；
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

// initialRoutePages 把初始路由册的声明落到判断行 r 的列上。总数那一句只数判断行，所以各维与 q 不能落到适用性表 a 上。
var initialRoutePages = ports.InitialRoutePages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{"recordedAt": "r.recorded_at"},
	Identity: []string{
		"r.customer_account_id", "r.shipment_request_id", "r.acceptance_baseline", "r.declared_parcel_id", "r.service_purpose",
	},
	Filters: map[string]string{"conclusion": "r.conclusion"},
	Keyword: []string{
		"r.customer_account_id", "r.shipment_request_id", "r.acceptance_baseline", "r.declared_parcel_id", "r.plan_version",
	},
})

// scannedInitialRoute 是取页读回的一行。计划 jsonb 先留着：重建段链只对本页的行做，多取来判下一页的那一行若是坏计划，不该让
// 本页答不出。
type scannedInitialRoute struct {
	row  ports.InitialRouteCatalogueRow
	plan []byte
}

// ListInitialRoutes 上列初始路由判断册的检索列面，适用性经计划版本 LEFT JOIN——
// 无路可走行与尚未登记适用性的计划行，适用性组如实缺席。按查询的筛选、q 与排序取游标
// 之后的一页；缺省是落册时刻倒序、同刻按判断键各段倒序。
func (catalogue *RoutePlanCatalogue) ListInitialRoutes(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.InitialRouteCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.InitialRouteCatalogueRow], error) {
		return cataloguepage.Result[ports.InitialRouteCatalogueRow]{}, fmt.Errorf("list initial routes: %w", err)
	}
	if err := routePlanListLimit("list initial routes", limit); err != nil {
		return cataloguepage.Result[ports.InitialRouteCatalogueRow]{}, err
	}
	plan, err := initialRoutePages.Plan(query, "r.tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	// 总数不联适用性表：LEFT JOIN 按计划版本主键至多连上一行，不改行数。
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL("network_routing.initial_route r"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`r.customer_account_id, r.shipment_request_id, r.acceptance_baseline,
		 r.declared_parcel_id, r.service_purpose, r.conclusion, r.plan_version,
		 a.state, a.basis, a.successor, a.transitioned_at, r.recorded_at, r.plan`,
		`network_routing.initial_route r
		 LEFT JOIN network_routing.plan_applicability a ON a.plan_id = r.plan_version`,
	), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var fetched []scannedInitialRoute
	for rows.Next() {
		var scanned scannedInitialRoute
		row := &scanned.row
		var planVersion, state, basis, successor *string
		var transitionedAt *time.Time
		if err := rows.Scan(
			&row.CustomerAccountID, &row.ShipmentRequestID, &row.AcceptanceBaseline,
			&row.DeclaredParcelID, &row.ServicePurpose, &row.Conclusion, &planVersion,
			&state, &basis, &successor, &transitionedAt, &row.RecordedAt, &scanned.plan,
		); err != nil {
			return fail(err)
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
		fetched = append(fetched, scanned)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	rows.Close()

	page, err := cataloguepage.Finish(query, fetched, limit, total, initialRoutePosition)
	if err != nil {
		return fail(err)
	}
	judgments := make([]ports.InitialRouteCatalogueRow, 0, len(page.Rows))
	for _, scanned := range page.Rows {
		row := scanned.row
		if scanned.plan != nil {
			legs, err := catalogueLegsOf(tenant, row, scanned.plan)
			if err != nil {
				return fail(err)
			}
			row.Legs = legs
		}
		judgments = append(judgments, row)
	}
	return cataloguepage.Result[ports.InitialRouteCatalogueRow]{Rows: judgments, Next: page.Next, Total: page.Total}, nil
}

// initialRoutePosition 取一行在缺省序上的位置。本册的可排维只放出 recordedAt；声明另放可排维时，这里要按 sortField 取那一维的列值。
func initialRoutePosition(scanned scannedInitialRoute, _ string) cataloguepage.Position {
	row := scanned.row
	return cataloguepage.Position{
		Value: cataloguepage.FormatInstant(row.RecordedAt),
		Identity: []string{
			row.CustomerAccountID, row.ShipmentRequestID, row.AcceptanceBaseline, row.DeclaredParcelID, row.ServicePurpose,
		},
	}
}

// catalogueLegsOf 按判断口的装载纪律重建整份计划，再逐段转写：重建失败是坏数据，整页答不出，不跳过这一行
// ——跳过会让一份坏计划在查阅页上看起来像一行没有段的计划。
func catalogueLegsOf(
	tenant domain.TenantID,
	row ports.InitialRouteCatalogueRow,
	planJSON []byte,
) ([]ports.RoutePlanLegCatalogueRow, error) {
	key, err := rebuildRouteKey(tenant, routeKeyColumns{
		customerAccount:    row.CustomerAccountID,
		shipmentRequest:    row.ShipmentRequestID,
		acceptanceBaseline: row.AcceptanceBaseline,
		declaredParcel:     row.DeclaredParcelID,
		servicePurpose:     row.ServicePurpose,
	})
	if err != nil {
		return nil, err
	}
	plan, err := rebuildPlan(key, planJSON)
	if err != nil {
		return nil, err
	}
	legs := plan.Legs()
	rows := make([]ports.RoutePlanLegCatalogueRow, 0, len(legs))
	for index, leg := range legs {
		reference, err := domain.NewPlannedLegReference(plan.Version(), index+1)
		if err != nil {
			return nil, err
		}
		window := leg.Window()
		legRow := ports.RoutePlanLegCatalogueRow{
			Reference:   reference.String(),
			Kind:        leg.Kind().String(),
			From:        leg.From().String(),
			To:          leg.To().String(),
			Responsible: leg.Responsible().String(),
			Earliest:    window.Earliest(),
			Latest:      window.Latest(),
			WindowBasis: window.Basis().String(),
		}
		if scope, delivery := leg.DeliveryScope(); delivery {
			legRow.To = scope.String()
		}
		rows = append(rows, legRow)
	}
	return rows, nil
}

var routeReassessmentPages = ports.RouteReassessmentPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "recorded_at"},
	Identity: []string{"correlation_id"},
	Filters:  map[string]string{"conclusion": "conclusion"},
	Keyword: []string{
		"correlation_id", "customer_account_id", "shipment_request_id", "acceptance_baseline", "declared_parcel_id",
		"reviewed_plan",
	},
})

// ListRouteReassessments 上列路由复核册的检索列面。按查询的筛选、q 与排序取游标之后的
// 一页；缺省是落册时刻倒序、同刻按触发关联倒序。
func (catalogue *RoutePlanCatalogue) ListRouteReassessments(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.RouteReassessmentCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.RouteReassessmentCatalogueRow], error) {
		return cataloguepage.Result[ports.RouteReassessmentCatalogueRow]{}, fmt.Errorf("list route reassessments: %w", err)
	}
	if err := routePlanListLimit("list route reassessments", limit); err != nil {
		return cataloguepage.Result[ports.RouteReassessmentCatalogueRow]{}, err
	}
	plan, err := routeReassessmentPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "network_routing.route_reassessment"
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`correlation_id, customer_account_id, shipment_request_id,
		 acceptance_baseline, declared_parcel_id, service_purpose, conclusion,
		 reviewed_plan, lapse_basis, candidate_state, reroute_state,
		 reassessed_at, recorded_at`,
		source,
	), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var reassessments []ports.RouteReassessmentCatalogueRow
	for rows.Next() {
		var row ports.RouteReassessmentCatalogueRow
		var reviewedPlan, lapseBasis, candidateState, rerouteState *string
		if err := rows.Scan(
			&row.CorrelationID, &row.CustomerAccountID, &row.ShipmentRequestID,
			&row.AcceptanceBaseline, &row.DeclaredParcelID, &row.ServicePurpose,
			&row.Conclusion, &reviewedPlan, &lapseBasis, &candidateState, &rerouteState,
			&row.ReassessedAt, &row.RecordedAt,
		); err != nil {
			return fail(err)
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
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, reassessments, limit, total, routeReassessmentPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// routeReassessmentPosition 取一行在缺省序上的位置。RecordedAt 是扫描回来的列值，不是副本。本册的可排维只放出 recordedAt；声明
// 另放可排维时，这里要按 sortField 取那一维的列值。
func routeReassessmentPosition(row ports.RouteReassessmentCatalogueRow, _ string) cataloguepage.Position {
	return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.RecordedAt), Identity: []string{row.CorrelationID}}
}
