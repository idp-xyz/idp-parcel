package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// 本文件是路由判断两册的伴生列表读端口（票 admin-skeleton-closure-batch/03，读面
// 形状照 ADR-0077 通例）：管理台 route-plans 页的供数面。不拓宽既有判断口
// （InitialRouteStore / RouteReassessmentStore）——那两口按完整判断键取单行，服务
// 编排的写前查，与目录上列是两种消费；伴生读端口另立，理由与 catalog_read 同句。
//
// 上列的是**检索列面**的照实转写：计划本体（jsonb）、无路可走判断（jsonb）、改路
// 决定与建议（jsonb）是判断的权威内容，读回要走各判断口的装载纪律，本口不从 jsonb
// 里抠字段冒充列。计划适用性表没有租户列（计划版本标识全局唯一由签发方保证，见
// 0005_plan_applicability.sql 的键选维）——本口的适用性列经**本租户的判断行**联查
// 得出，租户作用域由判断行的租户条件承担，不存在跨租可见路径（ADR-0003）。
//
// 租户在方法签名上（ADR-0077 Decision 五）；limit 非正拒；空册如实答空列表
// （Decision 四：两册的写入方都是渠道墙后的编排，册空是墙拦不是缺陷）。

// InitialRouteCatalogueRow 是初始路由判断册上列的一行：一次已落册判断的检索列面。
// 结论封闭两格（ROUTE_FORMED / NO_CURRENT_ROUTE，迁移 CHECK 钉住）——「无当前有效
// 路由」是明确判断不是空行，照登透出不折叠。计划版本仅 ROUTE_FORMED 在场；适用性
// 三列在计划版本已有适用性登记时成组在场（plan_applicability 联查，Save 换态一行）。
type InitialRouteCatalogueRow struct {
	CustomerAccountID  string
	ShipmentRequestID  string
	AcceptanceBaseline string
	DeclaredParcelID   string
	ServicePurpose     string
	Conclusion         string
	PlanVersion        string
	HasPlanVersion     bool

	// 适用性组：State 是封闭四态原词（CURRENTLY_EFFECTIVE/SUPERSEDED/LAPSED/
	// CONCLUDED）；Basis 与 Successor 的在场矩阵由库面 CHECK 钉住（当前有效两缺、
	// 已被替代两在、失效/结束有据无接班），照登转写不补半边。
	HasApplicability       bool
	ApplicabilityState     string
	ApplicabilityBasis     string
	HasApplicabilityBasis  bool
	ApplicabilitySuccessor string
	HasSuccessor           bool
	ApplicabilityChangedAt time.Time

	RecordedAt time.Time
}

// RouteReassessmentCatalogueRow 是路由复核册上列的一行。结论封闭四走向
// （STILL_APPLICABLE/PLAN_LAPSED/FIRST_PLAN_FORMED/REROUTED）；候选评估与改路判定
// 是可缺席的封闭词（缺席=本走向不评估/未评估），逐结论在场件矩阵由库面 CHECK 钉住。
// 新计划本体在 jsonb 内不上列——改路成的新计划版本会以新的适用性登记出现在初始
// 路由册的联查列上，两册对照即见替代关系。
type RouteReassessmentCatalogueRow struct {
	CorrelationID      string
	CustomerAccountID  string
	ShipmentRequestID  string
	AcceptanceBaseline string
	DeclaredParcelID   string
	ServicePurpose     string
	Conclusion         string
	ReviewedPlan       string
	HasReviewedPlan    bool
	LapseBasis         string
	HasLapseBasis      bool
	CandidateState     string
	HasCandidateState  bool
	RerouteState       string
	HasRerouteState    bool
	ReassessedAt       time.Time
	RecordedAt         time.Time
}

// RoutePlanCatalogueRead 是路由判断两册的伴生列表读端口。
type RoutePlanCatalogueRead interface {
	ListInitialRoutes(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]InitialRouteCatalogueRow, error)
	ListRouteReassessments(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]RouteReassessmentCatalogueRow, error)
}
