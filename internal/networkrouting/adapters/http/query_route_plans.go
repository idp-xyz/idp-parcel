package networkhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// RoutePlanCatalogueReader 是路由判断两册端点消费的读口（票 admin-skeleton-closure-batch/03）。
// 上列的是检索列面：计划本体、无路可走判断与改路决定住在 jsonb 内，权威读法归各判断
// 口，本端点不透出——见 ports.RoutePlanCatalogueRead 的口面纪律；成计划行的段链是那条纪律
// 写明的例外（ADR-0217）。
type RoutePlanCatalogueReader interface {
	ListInitialRoutes(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.InitialRouteCatalogueRow], error)
	ListRouteReassessments(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.RouteReassessmentCatalogueRow], error)
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

// registerCatalogues 是两册的查询声明（ADR-0144）：register 选中哪一册，就按哪一册的声明解码其余参数。
var registerCatalogues = map[string]*cataloguepage.Catalogue{
	registerInitialRoute: ports.InitialRoutePages,
	registerReassessment: ports.RouteReassessmentPages,
}

// NewQueryRoutePlansEndpoint 交回路由判断两册查阅的 HTTP 入口
// （GET /route-plans，ADR-0077；最终路径归装配票，本批为 closure-batch/07）。
//
// 两册共用一个端点，按 `register` 查询参数分派（先例：本包网络目录端点按 `family` 分派各族）。
// 缺席或取值不在封闭集内按坏请求拒：替调用方默认一册就是替它猜。
// 复用本包目录查阅的 Intake：判断册查阅同属运营查阅，授权边界同是租户，不为业务
// 事实册另立第二种准入形（先例：parcelpricing 评价端点同一分法）。
//
// register 与其余查询参数（after、sort、筛选维与 q，由本端点按所选册的声明解码，
// ADR-0144 决定六）都在 Intake 之后才看：未配置的渠道对一切请求同答 403，答复不随
// 查询串变（ADR-0055 决定三），没有凭据的调用方探不出有哪几册、各册开放了哪些查询
// 维度。配置了的渠道上不成立即 400。
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

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		values := request.URL.Query()
		register := values.Get("register")
		catalogue, known := registerCatalogues[register]
		if !known {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		pageQuery, err := catalogue.Decode(values)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		ctx := request.Context()
		tenant := query.Scope.Tenant()
		limit := query.Limit

		switch register {
		case registerInitialRoute:
			page, err := reader.ListInitialRoutes(ctx, tenant, limit, pageQuery)
			if err != nil {
				// 读不回是答案未形成（5xx），不伪装成空册——前者该重试，后者是终局答案。
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			// 空册交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
			bodies := make([]initialRouteBody, 0, len(page.Rows))
			for _, row := range page.Rows {
				bodies = append(bodies, initialRouteBodyOf(row))
			}
			writeJSON(response, http.StatusOK, initialRouteListResponse{
				Outcome:   outcomeInitialRoutesListed,
				Judgments: bodies,
				Page:      page.Page(limit),
			})
		case registerReassessment:
			page, err := reader.ListRouteReassessments(ctx, tenant, limit, pageQuery)
			if err != nil {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			bodies := make([]reassessmentBody, 0, len(page.Rows))
			for _, row := range page.Rows {
				bodies = append(bodies, reassessmentBodyOf(row))
			}
			writeJSON(response, http.StatusOK, reassessmentListResponse{
				Outcome:       outcomeRouteReassessmentsListed,
				Reassessments: bodies,
				Page:          page.Page(limit),
			})
		}
	})
}

type initialRouteListResponse struct {
	Outcome   string             `json:"outcome"`
	Judgments []initialRouteBody `json:"judgments"`
	Page      cataloguepage.Page `json:"page"`
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
	Legs               []plannedLegBody   `json:"legs,omitempty"`
	RecordedAt         string             `json:"recordedAt"`
}

// plannedLegBody 是成计划行上的一段（ADR-0217）。reference 是登记方声明段引用时照抄的拼写；kind 是
// domain.PlannedLegKind 的原词；交付段的 to 是交付范围。各格都是字符串：管理台契约夹具的值类型今天只认字符串。
type plannedLegBody struct {
	Reference   string `json:"reference"`
	Kind        string `json:"kind"`
	From        string `json:"from"`
	To          string `json:"to"`
	Responsible string `json:"responsible"`
	Earliest    string `json:"earliest"`
	Latest      string `json:"latest"`
	WindowBasis string `json:"windowBasis"`
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
	for _, leg := range row.Legs {
		body.Legs = append(body.Legs, plannedLegBody{
			Reference:   leg.Reference,
			Kind:        leg.Kind,
			From:        leg.From,
			To:          leg.To,
			Responsible: leg.Responsible,
			Earliest:    utcText(leg.Earliest),
			Latest:      utcText(leg.Latest),
			WindowBasis: leg.WindowBasis,
		})
	}
	return body
}

type reassessmentListResponse struct {
	Outcome       string             `json:"outcome"`
	Reassessments []reassessmentBody `json:"reassessments"`
	Page          cataloguepage.Page `json:"page"`
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
