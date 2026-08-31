package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// EvaluationCatalogueReader 是评价登记册端点消费的读口（票 admin-skeleton-closure-batch/03）。
// 上列的是登记册检索列面：评价的语义细节（对象、方向、金额）住在快照内，属详情读法，
// 本端点不透出——见 ports.EvaluationCatalogueRead 的口面纪律。
type EvaluationCatalogueReader interface {
	ListEvaluations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.EvaluationCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本适配器不新造查询语义。
var _ EvaluationCatalogueReader = ports.EvaluationCatalogueRead(nil)

// outcomeEvaluationsListed 是本端点唯一的业务成格：空册也是这一格（ADR-0077
// Decision 四）。评价的写入方是渠道墙后的评价编排，册空是墙拦不是缺陷。
const outcomeEvaluationsListed = "EVALUATIONS_LISTED"

// NewQueryEvaluationsEndpoint 交回评价登记册查阅的 HTTP 入口
// （GET /pricing-evaluations；最终路径归装配票，本批为 closure-batch/07）。
// 复用本包目录查阅的 Intake：评价查阅同属运营查阅，授权边界同是租户
// （ADR-0077 Decision 五），不为业务事实册另立第二种准入形。
func NewQueryEvaluationsEndpoint(
	intake PricingCatalogueIntake,
	reader EvaluationCatalogueReader,
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

		rows, err := reader.ListEvaluations(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			// 读不回是答案未形成，不是「空册」——伪装成后者会让一次该重试的故障
			// 变成一份看起来如实的空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		// 空册交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
		bodies := make([]evaluationBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, evaluationBodyOf(row))
		}
		writeJSON(response, http.StatusOK, evaluationListResponse{
			Outcome:     outcomeEvaluationsListed,
			Evaluations: bodies,
		})
	})
}

type evaluationListResponse struct {
	Outcome     string           `json:"outcome"`
	Evaluations []evaluationBody `json:"evaluations"`
}

// evaluationBody 逐字段透出检索列面。状态是迁移钉住的封闭五格原词
// （COMPLETED/PENDING/CONFLICT/FAILED/UNRATABLE），四种非完成结果不得互相冒充，
// 转写不折叠；双摘要与规范化版本照登透出（ADR-0014 的比对列）。
type evaluationBody struct {
	EvaluationID      string `json:"evaluationId"`
	Status            string `json:"status"`
	SemanticDigest    string `json:"semanticDigest"`
	PlanContentDigest string `json:"planContentDigest"`
	Canonicalization  string `json:"canonicalization"`
	RecordedAt        string `json:"recordedAt"`
}

func evaluationBodyOf(row ports.EvaluationCatalogueRow) evaluationBody {
	return evaluationBody{
		EvaluationID:      row.EvaluationID,
		Status:            row.Status,
		SemanticDigest:    row.SemanticDigest,
		PlanContentDigest: row.PlanContentDigest,
		Canonicalization:  row.Canonicalization,
		RecordedAt:        rfc3339(row.RecordedAt),
	}
}
