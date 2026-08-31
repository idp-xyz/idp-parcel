package networkhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// RoutePlanCatalogueReader 是路由判断两册端点消费的读口（票 admin-skeleton-closure-batch/03）。
// 上列的是检索列面：计划本体、无路可走判断与改路决定住在 jsonb 内，权威读法归各判断
// 口，本端点不透出——见 ports.RoutePlanCatalogueRead 的口面纪律。
type RoutePlanCatalogueReader interface {
	ListInitialRoutes(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.InitialRouteCatalogueRow, error)
	ListRouteReassessments(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.RouteReassessmentCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ RoutePlanCatalogueReader = ports.RoutePlanCatalogueRead(nil)

// 业务结果的封闭集合：两册各占一格。空册如实答空列表走 2xx 成格，不折成未配置
// （ADR-0077 Decision 四：两册的写入方是渠道墙后的路由编排，册空是墙拦不是缺陷）。
const (
	outcomeInitialRoutesListed      = "INITIAL_ROUTES_LISTED"
	outcomeRouteReassessmentsListed = "ROUTE_REASSESSMENTS_LISTED"
)

// register 查询参数的封闭两册。词取判断库两张表的表名转写——查阅口与库面对同一册
// 用同一个词，页面不必维护第二套对照表。
const (
	registerInitialRoute = "initial-route"
	registerReassessment = "reassessment"
)

// NewQueryRoutePlansEndpoint 交回路由判断两册查阅的 HTTP 入口
// （GET /route-plans，ADR-0077；最终路径归装配票，本批为 closure-batch/07）。
//
// 两册共用一个端点，按 `register` 查询参数分派（先例：本包网络目录端点一口七分派）：
// 参数在场与否、取值在不在封闭集内属传输形状，先于 Intake；未配置 Intake 对两个分支
// 同答 403，分支选择不泄露任何东西。缺席按坏请求拒：替调用方默认一册就是替它猜。
// 复用本包目录查阅的 Intake：判断册查阅同属运营查阅，授权边界同是租户，不为业务
// 事实册另立第二种准入形（先例：parcelpricing 评价端点同一分法）。
func NewQueryRoutePlansEndpoint(
	intake CatalogueQueryIntake,
	reader RoutePlanCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		register := request.URL.Query().Get("register")
		if register != registerInitialRoute && register != registerReassessment {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		ctx := request.Context()
		tenant := query.Scope.Tenant()
		limit := query.Limit

		switch register {
		case registerInitialRoute:
			rows, err := reader.ListInitialRoutes(ctx, tenant, limit)
			if err != nil {
				// 读不回是答案未形成（5xx），不伪装成空册——前者该重试，后者是终局答案。
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			// 空册交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
			bodies := make([]initialRouteBody, 0, len(rows))
			for _, row := range rows {
				bodies = append(bodies, initialRouteBodyOf(row))
			}
			writeJSON(response, http.StatusOK, initialRouteListResponse{
				Outcome:   outcomeInitialRoutesListed,
				Judgments: bodies,
			})
		case registerReassessment:
			rows, err := reader.ListRouteReassessments(ctx, tenant, limit)
			if err != nil {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			bodies := make([]reassessmentBody, 0, len(rows))
			for _, row := range rows {
				bodies = append(bodies, reassessmentBodyOf(row))
			}
			writeJSON(response, http.StatusOK, reassessmentListResponse{
				Outcome:       outcomeRouteReassessmentsListed,
				Reassessments: bodies,
			})
		}
	})
}

type initialRouteListResponse struct {
	Outcome   string             `json:"outcome"`
	Judgments []initialRouteBody `json:"judgments"`
}

// initialRouteBody 逐字段透出检索列面。conclusion 是封闭两格原词——「无当前有效
// 路由」是明确判断，照登透出不折成空行；planVersion 仅成计划行在场；applicability
// 组仅计划版本已有适用性登记时在场，其内的 basis/successor 逐态矩阵照登转写。
type initialRouteBody struct {
	CustomerAccountID  string             `json:"customerAccountId"`
	ShipmentRequestID  string             `json:"shipmentRequestId"`
	AcceptanceBaseline string             `json:"acceptanceBaseline"`
	DeclaredParcelID   string             `json:"declaredParcelId"`
	ServicePurpose     string             `json:"servicePurpose"`
	Conclusion         string             `json:"conclusion"`
	PlanVersion        string             `json:"planVersion,omitempty"`
	Applicability      *applicabilityBody `json:"applicability,omitempty"`
	RecordedAt         string             `json:"recordedAt"`
}

type applicabilityBody struct {
	State          string `json:"state"`
	TransitionedAt string `json:"transitionedAt"`
	Basis          string `json:"basis,omitempty"`
	Successor      string `json:"successor,omitempty"`
}

func initialRouteBodyOf(row ports.InitialRouteCatalogueRow) initialRouteBody {
	body := initialRouteBody{
		CustomerAccountID:  row.CustomerAccountID,
		ShipmentRequestID:  row.ShipmentRequestID,
		AcceptanceBaseline: row.AcceptanceBaseline,
		DeclaredParcelID:   row.DeclaredParcelID,
		ServicePurpose:     row.ServicePurpose,
		Conclusion:         row.Conclusion,
		RecordedAt:         utcText(row.RecordedAt),
	}
	if row.HasPlanVersion {
		body.PlanVersion = row.PlanVersion
	}
	if row.HasApplicability {
		applicability := applicabilityBody{
			State:          row.ApplicabilityState,
			TransitionedAt: utcText(row.ApplicabilityChangedAt),
		}
		if row.HasApplicabilityBasis {
			applicability.Basis = row.ApplicabilityBasis
		}
		if row.HasSuccessor {
			applicability.Successor = row.ApplicabilitySuccessor
		}
		body.Applicability = &applicability
	}
	return body
}

type reassessmentListResponse struct {
	Outcome       string             `json:"outcome"`
	Reassessments []reassessmentBody `json:"reassessments"`
}

// reassessmentBody 逐字段透出检索列面。conclusion 封闭四走向原词；候选评估与改路
// 判定是可缺席封闭词，缺席即「本走向不评估/未评估」——缺席是真话不是缺陷。
type reassessmentBody struct {
	CorrelationID      string `json:"correlationId"`
	CustomerAccountID  string `json:"customerAccountId"`
	ShipmentRequestID  string `json:"shipmentRequestId"`
	AcceptanceBaseline string `json:"acceptanceBaseline"`
	DeclaredParcelID   string `json:"declaredParcelId"`
	ServicePurpose     string `json:"servicePurpose"`
	Conclusion         string `json:"conclusion"`
	ReviewedPlan       string `json:"reviewedPlan,omitempty"`
	LapseBasis         string `json:"lapseBasis,omitempty"`
	CandidateState     string `json:"candidateState,omitempty"`
	RerouteState       string `json:"rerouteState,omitempty"`
	ReassessedAt       string `json:"reassessedAt"`
	RecordedAt         string `json:"recordedAt"`
}

func reassessmentBodyOf(row ports.RouteReassessmentCatalogueRow) reassessmentBody {
	body := reassessmentBody{
		CorrelationID:      row.CorrelationID,
		CustomerAccountID:  row.CustomerAccountID,
		ShipmentRequestID:  row.ShipmentRequestID,
		AcceptanceBaseline: row.AcceptanceBaseline,
		DeclaredParcelID:   row.DeclaredParcelID,
		ServicePurpose:     row.ServicePurpose,
		Conclusion:         row.Conclusion,
		ReassessedAt:       utcText(row.ReassessedAt),
		RecordedAt:         utcText(row.RecordedAt),
	}
	if row.HasReviewedPlan {
		body.ReviewedPlan = row.ReviewedPlan
	}
	if row.HasLapseBasis {
		body.LapseBasis = row.LapseBasis
	}
	if row.HasCandidateState {
		body.CandidateState = row.CandidateState
	}
	if row.HasRerouteState {
		body.RerouteState = row.RerouteState
	}
	return body
}
