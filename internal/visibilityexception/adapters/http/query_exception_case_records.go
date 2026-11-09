package visibilityhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// CaseReviewReader 是异常案件管理页消费的读口（管理台 exception-cases 页，
// 票 admin-skeleton-closure-batch/06）。
type CaseReviewReader interface {
	ListExceptionCases(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.ExceptionCaseCatalogueRow], error)
}

var _ CaseReviewReader = ports.CaseReviewRead(nil)

const outcomeExceptionCasesListed = "EXCEPTION_CASES_LISTED"

// NewQueryExceptionCaseRecordsEndpoint 交回异常案件册的 HTTP 入口
// （GET /exception-case-records，票 admin-skeleton-closure-batch/06）。
//
// 单册端点，不设 registry 参数：页面只有一张案件列表，多余的选择轴是给未来语义
// 占坑。查阅不推进案件阶段、不合并、不关闭——那些是案件命令面的判断（CONTEXT
// 「异常案件」），本端点只消费存储读面（ADR-0077 Decision 一）。准入复用运营追踪
// 查阅的 OperationsTrackingIntake，判据同 /exception-triage-records。
//
// 翻页、排序与筛选照 ADR-0144：查询参数在 Intake 之后按 ports.ExceptionCasePages 解码（writePageQueryProblem 头注），
// 坏参数答 400 带理由散文，答复加 page。
func NewQueryExceptionCaseRecordsEndpoint(
	intake OperationsTrackingIntake,
	reader CaseReviewReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeOperationsQuery(request.Context(), request)
		if err != nil {
			writeOperationsIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.ExceptionCasePages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		page, err := reader.ListExceptionCases(request.Context(), query.Scope.Tenant(), query.Limit, pageQuery)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]exceptionCaseBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			body := exceptionCaseBody{
				CaseID:                row.CaseID,
				DocumentNumber:        row.DocumentNumber,
				DocumentNumberAbsence: row.DocumentNumberAbsence,
				RootParcel:            row.RootParcel,
				ImpactScope:           row.ImpactScope,
				ResponsibleTeam:       row.ResponsibleTeam,
				Phase:                 row.Phase,
				EstablishedAt:         catalogueInstant(row.EstablishedAt),
				Conclusion:            row.Conclusion,
				MergedInto:            row.MergedInto,
			}
			if row.FirstResponse != nil {
				body.FirstResponse = catalogueInstant(*row.FirstResponse)
			}
			if row.ClosedAt != nil {
				body.ClosedAt = catalogueInstant(*row.ClosedAt)
			}
			bodies = append(bodies, body)
		}
		writeJSON(response, http.StatusOK, exceptionCaseListResponse{
			Outcome: outcomeExceptionCasesListed,
			Cases:   bodies,
			Page:    page.Page(query.Limit),
		})
	})
}

type exceptionCaseListResponse struct {
	Outcome string              `json:"outcome"`
	Cases   []exceptionCaseBody `json:"cases"`
	Page    cataloguepage.Page  `json:"page"`
}

// exceptionCaseBody 逐字段透出一件异常案件。
//
// phase 原样透出（建立/处置中/已关闭三态，0008 CHECK 钉死）；mergedInto 在场即这件
// 已并入他案——被并入的案件不从册面消失，指针留痕（案件合并是登记事实不是删除）。
// 页面模板的「严重程度」「优先级」「工作状况」在案件行上没有登记格：严重程度与
// 优先级是分诊/处置侧的判断词，案件表不复述；工作状况若从 phase 推导就是替调用方
// 下结论。三键结构上不存在，缺席即答案（票 06 Comments 记明）。conclusion 只在
// 关闭时在场（0008 成对约束）。
//
// documentNumber 是案件的单据号（ADR-0355），没有号时缺席——不答空串，也不拿 caseId 顶；那时 documentNumberAbsence 在场，
// 说是上线前开立（ISSUED_BEFORE_NUMBERING）还是开立时单据编号时区未配置（NUMBERING_TIME_ZONE_NOT_CONFIGURED）。
type exceptionCaseBody struct {
	CaseID                string `json:"caseId"`
	DocumentNumber        string `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string `json:"documentNumberAbsence,omitempty"`
	RootParcel            string `json:"rootParcel"`
	ImpactScope           string `json:"impactScope"`
	ResponsibleTeam       string `json:"responsibleTeam"`
	Phase                 string `json:"phase"`
	EstablishedAt         string `json:"establishedAt"`
	FirstResponse         string `json:"firstResponse,omitempty"`
	ClosedAt              string `json:"closedAt,omitempty"`
	Conclusion            string `json:"conclusion,omitempty"`
	MergedInto            string `json:"mergedInto,omitempty"`
}
