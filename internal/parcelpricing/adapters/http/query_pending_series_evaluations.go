package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// PendingSeriesEvaluationReader 是被挂起评价联动端点消费的读口。
type PendingSeriesEvaluationReader interface {
	CountPendingSeriesEvaluations(ctx context.Context, tenant domain.TenantID) ([]ports.PendingSeriesEvaluationCount, error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ PendingSeriesEvaluationReader = ports.PendingSeriesEvaluationRead(nil)

// outcomePendingSeriesEvaluationsCounted 是本端点唯一的业务成格。
const outcomePendingSeriesEvaluationsCounted = "PENDING_SERIES_EVALUATIONS_COUNTED"

// NewQueryPendingSeriesEvaluationsEndpoint 交回「因序列未解析而待判断的评价数，按序列种类分组」的 HTTP 入口
// （GET /pricing-pending-series-evaluations；ADR-0105 Decision 五；票 pricing-reference-series-operations/05
// 第 2 项）。读面通例照 ADR-0077：租户级、未配置即拒、空册如实答空。
//
// asOf 与覆盖端点同一时刻源回显——这一格的数进摘要条与覆盖地平线并排，两个数说的是不是同一刻要说出来。
// 不收 limit：分组键是序列种类的封闭集，行数有上界。
func NewQueryPendingSeriesEvaluationsEndpoint(
	intake PricingCatalogueIntake,
	reader PendingSeriesEvaluationReader,
	clock CoverageClock,
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
		at := clock.Now().UTC()
		counts, err := reader.CountPendingSeriesEvaluations(request.Context(), query.Scope.Tenant())
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]pendingSeriesEvaluationBody, 0, len(counts))
		for _, count := range counts {
			bodies = append(bodies, pendingSeriesEvaluationBody{Kind: count.Kind, EvaluationCount: count.EvaluationCount})
		}
		writeJSON(response, http.StatusOK, pendingSeriesEvaluationsResponse{
			Outcome: outcomePendingSeriesEvaluationsCounted,
			AsOf:    rfc3339(at),
			Counts:  bodies,
		})
	})
}

// pendingSeriesEvaluationsResponse 顶层带 asOf（判据同覆盖端点）。counts 为空数组即租户内没有因序列未解析而
// 待判断的评价——那是正常答案；计数覆盖的是问题项子表有行的评价（ADR-0105 Decision 四不回填）。
type pendingSeriesEvaluationsResponse struct {
	Outcome string                        `json:"outcome"`
	AsOf    string                        `json:"asOf"`
	Counts  []pendingSeriesEvaluationBody `json:"counts"`
}

type pendingSeriesEvaluationBody struct {
	Kind            string `json:"kind"`
	EvaluationCount int    `json:"evaluationCount"`
}
