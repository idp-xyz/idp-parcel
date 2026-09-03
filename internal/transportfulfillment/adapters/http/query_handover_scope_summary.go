package tfhttp

import (
	"context"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// HandoverScopeSummarizer 是本端点转交的应用读用例（票 tf-unwired-seven/03）。
//
// 这一口接的是读用例而不是存储读面，与本包目录查阅（ADR-0077 Decision 一「不接编排」）
// 不同属：目录上列不形成任何判断，而汇总是派生量——「按裁决各有多少」只能由
// domain.SummarizeHandovers 派生，跨范围/跨租户校验与版本链折叠都在那一处。让端点直读
// 存储再自己数，就是为同一形状立第二个口径。读用例零登记零编辑零披露，查阅的性质不变。
type HandoverScopeSummarizer interface {
	Summarize(
		ctx context.Context,
		query application.SummarizeHandoverScopeQuery,
	) (application.SummarizeHandoverScopeResult, error)
}

var _ HandoverScopeSummarizer = (*application.SummarizeHandoverScopeHandler)(nil)

const queryParameterScope = "scope"

// NewQueryHandoverScopeSummaryEndpoint 交回交接范围汇总的 HTTP 入口
// （GET /transport-fulfillment-handover-scope-summary?scope=…）。
//
// 门次序照本包查阅面通例：方法 → 请求形状 → 准入。范围是这个读面的必备维，缺席是坏请求，
// 判它不需要先知道调用方是谁；租户只来自准入结果（ADR-0077 Decision 二/三），URL 里任何
// 自报租户都不读。准入交回的页大小在这里用不上——汇总是一个答案不是一页列表。
func NewQueryHandoverScopeSummaryEndpoint(
	intake CatalogueQueryIntake,
	summarizer HandoverScopeSummarizer,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		scope := strings.TrimSpace(request.URL.Query().Get(queryParameterScope))
		if scope == "" {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}

		result, err := summarizer.Summarize(request.Context(), application.SummarizeHandoverScopeQuery{
			TenantID: query.Scope.Tenant(),
			Scope:    scope,
		})
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeScopeSummary(response, result)
	})
}

// handoverScopeSummaryResponse 是封闭的响应形状。`outcome` 取应用结果枚举的原名，四格
// 各带各的伴随键：成立带 summary，未决带 reason 与 continuationReference，另两格只有
// outcome。`不成立汇总`刻意**没有** summary 键——三个零会被读者当成「这个范围全部为零」，
// 而它说的是「这个范围还没有交接」。
type handoverScopeSummaryResponse struct {
	Outcome               string                    `json:"outcome"`
	Reason                string                    `json:"reason,omitempty"`
	ContinuationReference string                    `json:"continuationReference,omitempty"`
	Summary               *handoverScopeSummaryBody `json:"summary,omitempty"`
}

// handoverScopeSummaryBody 逐格透出领域的汇总。
//
// total 与 allHandedOver 是领域的派生问答，原样透出而不留给读者自己算：CONTEXT 交接硬句
// 「整批/整车/整袋结论只能由对象级结果派生」，`AllHandedOver` 就是那个派生的唯一出处，
// 前端自己拿三格相加再比对等于在页面上立第二个口径。
//
// 三格计数直投 JSON number 而不像容量四量那样转成串：那条转写纪律针对 int64 量在 2^53
// 之上失真，一个交接范围里的对象数不在那个量级；同一份答复里 total 与三格同型，读者不必
// 猜哪个要 parse。
type handoverScopeSummaryBody struct {
	Scope         string `json:"scope"`
	HandedOver    int    `json:"handedOver"`
	Refused       int    `json:"refused"`
	Unconfirmed   int    `json:"unconfirmed"`
	Total         int    `json:"total"`
	AllHandedOver bool   `json:"allHandedOver"`
}

func writeScopeSummary(response http.ResponseWriter, result application.SummarizeHandoverScopeResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := handoverScopeSummaryResponse{Outcome: outcome}
	if summary, present := result.Summary(); present {
		body.Summary = &handoverScopeSummaryBody{
			Scope:         summary.Scope().String(),
			HandedOver:    summary.HandedOver(),
			Refused:       summary.Refused(),
			Unconfirmed:   summary.Unconfirmed(),
			Total:         summary.Total(),
			AllHandedOver: summary.AllHandedOver(),
		}
	}
	if reason := result.Reason().String(); reason != "" {
		body.Reason = reason
	}
	if continuation := result.Continuation(); continuation != "" {
		body.ContinuationReference = continuation
	}
	// ADR-0022：四格都是形成了的答案，一律 200；状态码不替 outcome 说话。
	writeJSON(response, http.StatusOK, body)
}
