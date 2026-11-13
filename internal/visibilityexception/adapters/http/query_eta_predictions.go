package visibilityhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

const outcomeETAPredictionsListed = "ETA_PREDICTIONS_LISTED"

// ETAPredictionReviewReader 是 ETA 预测查阅端点消费的读口。
type ETAPredictionReviewReader interface {
	ListCurrentETAPredictions(
		ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
	) (cataloguepage.Result[ports.ETAPredictionReviewRow], error)
}

var _ ETAPredictionReviewReader = ports.ETAPredictionReviewRead(nil)

type etaPredictionListResponse struct {
	Outcome     string              `json:"outcome"`
	Predictions []etaPredictionBody `json:"predictions"`
	Page        cataloguepage.Page  `json:"page"`
}

// etaPredictionBody 逐字段透出一条预测线的当前版。priorVersion 首版缺席；manualEntry 只在这一版是人工录入的时在场，
// 整键缺席即预测来源送来的。这一行只说登记了什么预测，不说它对客户可不可见——那是可见性判断的答复。
type etaPredictionBody struct {
	Parcel       string              `json:"parcel"`
	Milestone    string              `json:"milestone"`
	Source       string              `json:"source"`
	Version      string              `json:"version"`
	Inputs       string              `json:"inputs"`
	Model        string              `json:"model"`
	RangeFrom    string              `json:"rangeFrom"`
	RangeTo      string              `json:"rangeTo"`
	Confidence   string              `json:"confidence"`
	PredictedAt  string              `json:"predictedAt"`
	PriorVersion string              `json:"priorVersion,omitempty"`
	ManualEntry  *etaManualEntryBody `json:"manualEntry,omitempty"`
}

type etaManualEntryBody struct {
	EnteredBy string `json:"enteredBy"`
	Original  string `json:"original"`
	EnteredAt string `json:"enteredAt"`
}

// NewQueryETAPredictionsEndpoint 交回 ETA 预测查阅的 HTTP 入口（GET /eta-prediction-records；收哪些键见 ports.ETAPredictionPages）。
// 租户只取自认证；空册答空列表。
func NewQueryETAPredictionsEndpoint(intake OperationsTrackingIntake, reader ETAPredictionReviewReader) http.Handler {
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
		values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		pageQuery, err := ports.ETAPredictionPages.Decode(values)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		page, err := reader.ListCurrentETAPredictions(request.Context(), query.Scope.Tenant(), query.Limit, pageQuery)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]etaPredictionBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			body := etaPredictionBody{
				Parcel: row.Parcel, Milestone: row.Milestone, Source: row.Source, Version: row.Version,
				Inputs: row.Inputs, Model: row.Model,
				RangeFrom: catalogueInstant(row.RangeFrom), RangeTo: catalogueInstant(row.RangeTo),
				Confidence: row.Confidence, PredictedAt: catalogueInstant(row.PredictedAt), PriorVersion: row.PriorVersion,
			}
			if manual := row.ManualEntry; manual != nil {
				body.ManualEntry = &etaManualEntryBody{
					EnteredBy: manual.EnteredBy, Original: manual.Original, EnteredAt: catalogueInstant(manual.EnteredAt),
				}
			}
			bodies = append(bodies, body)
		}
		writeJSON(response, http.StatusOK, etaPredictionListResponse{
			Outcome: outcomeETAPredictionsListed, Predictions: bodies, Page: page.Page(query.Limit),
		})
	})
}
