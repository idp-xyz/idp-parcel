package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证路由判断两册读面（票 admin-skeleton-closure-batch/03，
// 形状照 ADR-0077）：检索列面照登转写、jsonb 判断本体不透出、适用性联查按在场成组、
// 跨租户不可见、空租户答空列表、limit 生效且非正拒。
//
// 夹具以测试内 SQL 插行铺设（S 级合成行，SYN- 前缀）：两册是业务事实，写入方是渠道
// 墙后的路由编排，没有登记 CLI 可借用；读面机制验证只需要「册上有行」这个事实本身，
// 插行经过迁移钉住的全部 CHECK 矩阵。直用池 Exec 是 visibilityexception 与
// parcelpricing 读面测试的既有先例。
func newRoutePlanCatalogue(t *testing.T) (*adapter.RoutePlanCatalogue, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := adapter.NewRoutePlanCatalogue(db)
	if err != nil {
		t.Fatalf("构造路由计划册读面：%v", err)
	}
	return catalogue, pool
}

var routePlanBaseAt = time.Date(2026, 8, 2, 8, 0, 0, 0, time.UTC)

// insertInitialRouteRow 直插一行合规初始路由判断。formedPlanVersion 空串表示
// NO_CURRENT_ROUTE（计划与无路可走二居其一，XOR CHECK）。
func insertInitialRouteRow(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	tenantID, parcelID, formedPlanVersion string,
	recordedAt time.Time,
) {
	t.Helper()
	conclusion, planVersion, plan, noRoute := "NO_CURRENT_ROUTE", (*string)(nil), []byte(nil), []byte(`{"synthetic":true,"basis":"SYN-NO-ROUTE"}`)
	if formedPlanVersion != "" {
		conclusion, planVersion, plan, noRoute = "ROUTE_FORMED", &formedPlanVersion, []byte(`{"synthetic":true}`), nil
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.initial_route
			(tenant_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, conclusion, plan_version, plan,
			 no_route, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		tenantID, "SYN-ACC-1", "SYN-REQ-"+parcelID, "SYN-BASELINE-1",
		parcelID, "DELIVERY", conclusion, planVersion, plan, noRoute, recordedAt,
	); err != nil {
		t.Fatalf("插初始路由行 %s：%v", parcelID, err)
	}
}

// insertApplicabilityRow 直插一行合规计划适用性（逐态在场矩阵见
// 0005_plan_applicability.sql 的 CHECK）。
func insertApplicabilityRow(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	planID, state, basis, successor string,
	transitionedAt time.Time,
) {
	t.Helper()
	var basisValue, successorValue *string
	if basis != "" {
		basisValue = &basis
	}
	if successor != "" {
		successorValue = &successor
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.plan_applicability
			(plan_id, state, transitioned_at, basis, successor)
		 VALUES ($1, $2, $3, $4, $5)`,
		planID, state, transitionedAt, basisValue, successorValue,
	); err != nil {
		t.Fatalf("插适用性行 %s：%v", planID, err)
	}
}

// Covers: 初始路由册照列转写——两种结论并见不折叠（「无当前有效路由」是明确判断
// 不是空行）；适用性按计划版本联查成组透出（当前有效两缺、已被替代两在、未登记
// 如实缺席）；判断本体 jsonb 不上列；他租的判断不进本租户列表——含他租计划的
// 适用性行也不经联查泄出；空租户答空。
func TestInitialRouteCatalogueTranscribesJudgmentsWithApplicability(t *testing.T) {
	catalogue, pool := newRoutePlanCatalogue(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-a")

	insertInitialRouteRow(t, pool, ctx, "tenant-a", "SYN-PARCEL-1", "SYN-PLAN-1#v1", routePlanBaseAt)
	insertInitialRouteRow(t, pool, ctx, "tenant-a", "SYN-PARCEL-2", "SYN-PLAN-2#v1", routePlanBaseAt.Add(time.Hour))
	insertInitialRouteRow(t, pool, ctx, "tenant-a", "SYN-PARCEL-3", "", routePlanBaseAt.Add(2*time.Hour))
	insertInitialRouteRow(t, pool, ctx, "tenant-a", "SYN-PARCEL-4", "SYN-PLAN-4#v1", routePlanBaseAt.Add(3*time.Hour))
	insertInitialRouteRow(t, pool, ctx, "tenant-b", "SYN-PARCEL-THEIRS", "SYN-PLAN-THEIRS#v1", routePlanBaseAt.Add(4*time.Hour))

	insertApplicabilityRow(t, pool, ctx, "SYN-PLAN-1#v1", "CURRENTLY_EFFECTIVE", "", "", routePlanBaseAt)
	insertApplicabilityRow(t, pool, ctx, "SYN-PLAN-2#v1", "SUPERSEDED",
		"SYN-REROUTE/network-lapse", "SYN-PLAN-2#v2", routePlanBaseAt.Add(90*time.Minute))
	insertApplicabilityRow(t, pool, ctx, "SYN-PLAN-THEIRS#v1", "CURRENTLY_EFFECTIVE", "", "", routePlanBaseAt)
	// SYN-PLAN-4#v1 不登适用性：联查列如实缺席。

	rows, err := catalogue.ListInitialRoutes(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列初始路由：%v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("上列 %d 行，want 4（他租的判断不得进本租户列表）", len(rows))
	}
	byParcel := map[string]int{}
	for index, row := range rows {
		byParcel[row.DeclaredParcelID] = index
	}
	if _, bled := byParcel["SYN-PARCEL-THEIRS"]; bled {
		t.Fatal("他租的判断进了本租户的册")
	}
	// 落册时间倒序：4 → 3 → 2 → 1。
	if byParcel["SYN-PARCEL-4"] != 0 || byParcel["SYN-PARCEL-1"] != 3 {
		t.Fatalf("排序变形：%v", byParcel)
	}

	effective := rows[byParcel["SYN-PARCEL-1"]]
	if effective.Conclusion != "ROUTE_FORMED" || !effective.HasPlanVersion ||
		effective.PlanVersion != "SYN-PLAN-1#v1" {
		t.Fatalf("成计划行变形：%+v", effective)
	}
	if !effective.HasApplicability || effective.ApplicabilityState != "CURRENTLY_EFFECTIVE" ||
		!effective.ApplicabilityChangedAt.Equal(routePlanBaseAt) {
		t.Fatalf("当前有效适用性变形：%+v", effective)
	}
	if effective.HasApplicabilityBasis || effective.HasSuccessor {
		t.Fatalf("当前有效长出了依据或接班：%+v", effective)
	}
	if effective.CustomerAccountID != "SYN-ACC-1" || effective.ServicePurpose != "DELIVERY" ||
		effective.AcceptanceBaseline != "SYN-BASELINE-1" || effective.ShipmentRequestID != "SYN-REQ-SYN-PARCEL-1" {
		t.Fatalf("判断键列变形：%+v", effective)
	}

	superseded := rows[byParcel["SYN-PARCEL-2"]]
	if !superseded.HasApplicability || superseded.ApplicabilityState != "SUPERSEDED" ||
		!superseded.HasApplicabilityBasis || superseded.ApplicabilityBasis != "SYN-REROUTE/network-lapse" ||
		!superseded.HasSuccessor || superseded.ApplicabilitySuccessor != "SYN-PLAN-2#v2" {
		t.Fatalf("已被替代适用性没成组透出：%+v", superseded)
	}

	noRoute := rows[byParcel["SYN-PARCEL-3"]]
	if noRoute.Conclusion != "NO_CURRENT_ROUTE" || noRoute.HasPlanVersion || noRoute.HasApplicability {
		t.Fatalf("无路可走行变形：%+v", noRoute)
	}

	unregistered := rows[byParcel["SYN-PARCEL-4"]]
	if !unregistered.HasPlanVersion || unregistered.HasApplicability {
		t.Fatalf("未登适用性的计划行变形：%+v", unregistered)
	}

	empty, err := catalogue.ListInitialRoutes(ctx, scalar(t, domain.NewTenantID, "tenant-empty"), 10)
	if err != nil {
		t.Fatalf("空租户上列：%v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("空租户答了 %d 行，want 0", len(empty))
	}
}

// insertReassessmentRow 直插一行合规路由复核（逐结论在场件矩阵见
// 0002_route_judgments.sql 的 CHECK）。
func insertReassessmentRow(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	tenantID, correlationID, conclusion string,
	reviewedPlan, lapseBasis, candidateState string,
	recordedAt time.Time,
) {
	t.Helper()
	var reviewedValue, lapseValue, candidateValue *string
	if reviewedPlan != "" {
		reviewedValue = &reviewedPlan
	}
	if lapseBasis != "" {
		lapseValue = &lapseBasis
	}
	if candidateState != "" {
		candidateValue = &candidateState
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose, conclusion,
			 reviewed_plan, lapse_basis, candidate_state, reassessed_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		tenantID, correlationID, "SYN-ACC-1", "SYN-REQ-1", "SYN-BASELINE-1",
		"SYN-PARCEL-1", "DELIVERY", conclusion,
		reviewedValue, lapseValue, candidateValue, recordedAt.Add(-time.Minute), recordedAt,
	); err != nil {
		t.Fatalf("插复核行 %s：%v", correlationID, err)
	}
}

// Covers: 复核册照列转写——走向原词与可缺席封闭词按在场翻译（仍适用不评估候选、
// 已失效带评估结果），改路决定 jsonb 不上列；他租不可见；limit 生效且非正拒，
// 两口同判据。
func TestRouteReassessmentCatalogueTranscribesTheColumnFace(t *testing.T) {
	catalogue, pool := newRoutePlanCatalogue(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-a")

	insertReassessmentRow(t, pool, ctx, "tenant-a", "SYN-CORR-1", "STILL_APPLICABLE",
		"SYN-PLAN-1#v1", "", "", routePlanBaseAt)
	insertReassessmentRow(t, pool, ctx, "tenant-a", "SYN-CORR-2", "PLAN_LAPSED",
		"SYN-PLAN-2#v1", "SYN-LAPSE/line-withdrawn", "NO_QUALIFIED_CANDIDATES",
		routePlanBaseAt.Add(time.Hour))
	insertReassessmentRow(t, pool, ctx, "tenant-b", "SYN-CORR-THEIRS", "STILL_APPLICABLE",
		"SYN-PLAN-THEIRS#v1", "", "", routePlanBaseAt.Add(2*time.Hour))

	rows, err := catalogue.ListRouteReassessments(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列复核：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("上列 %d 行，want 2（他租的复核不得进本租户列表）", len(rows))
	}
	// 落册时间倒序：已失效在前。
	lapsed, still := rows[0], rows[1]
	if lapsed.CorrelationID != "SYN-CORR-2" || still.CorrelationID != "SYN-CORR-1" {
		t.Fatalf("排序变形：%q, %q", lapsed.CorrelationID, still.CorrelationID)
	}
	if lapsed.Conclusion != "PLAN_LAPSED" || !lapsed.HasReviewedPlan ||
		lapsed.ReviewedPlan != "SYN-PLAN-2#v1" || !lapsed.HasLapseBasis ||
		lapsed.LapseBasis != "SYN-LAPSE/line-withdrawn" || !lapsed.HasCandidateState ||
		lapsed.CandidateState != "NO_QUALIFIED_CANDIDATES" || lapsed.HasRerouteState {
		t.Fatalf("已失效行变形：%+v", lapsed)
	}
	if still.Conclusion != "STILL_APPLICABLE" || !still.HasReviewedPlan ||
		still.HasLapseBasis || still.HasCandidateState || still.HasRerouteState {
		t.Fatalf("仍适用行变形：%+v", still)
	}
	if still.ReassessedAt.IsZero() || still.RecordedAt.IsZero() {
		t.Fatalf("两个时刻没透出：%+v", still)
	}

	if _, err := catalogue.ListRouteReassessments(ctx, tenant, 0); err == nil {
		t.Fatal("复核册 limit 0 未被拒")
	}
	limited, err := catalogue.ListInitialRoutes(ctx, tenant, 1)
	if err != nil {
		t.Fatalf("带 limit 上列：%v", err)
	}
	if len(limited) > 1 {
		t.Fatalf("limit 1 交回 %d 行", len(limited))
	}
	if _, err := catalogue.ListInitialRoutes(ctx, tenant, -1); err == nil {
		t.Fatal("初始路由册 limit -1 未被拒")
	}
}
