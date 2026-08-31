package visibilityhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// TriageReviewReader 是异常分诊与处置协调页消费的读口：信号发作期册（连同分诊
// 结论）与处置请求册（管理台 exception-triage 页，票 admin-skeleton-closure-batch/06）。
type TriageReviewReader interface {
	ListSignalEpisodes(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.SignalEpisodeCatalogueRow, error)
	ListDispositionRequests(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.DispositionRequestCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ TriageReviewReader = ports.TriageReviewRead(nil)

const (
	outcomeSignalEpisodesListed      = "SIGNAL_EPISODES_LISTED"
	outcomeDispositionRequestsListed = "DISPOSITION_REQUESTS_LISTED"
)

const (
	registrySignalEpisode      = "signal-episode"
	registryDispositionRequest = "disposition-request"
)

// NewQueryExceptionTriageRecordsEndpoint 交回异常分诊与处置协调页两本册子的 HTTP
// 入口（GET /exception-triage-records，票 admin-skeleton-closure-batch/06）。
//
// 查阅不分诊、不建案、不发处置请求——查询端点只消费存储读面，不接应用编排
// （ADR-0077 Decision 一）。准入复用运营追踪查阅的 OperationsTrackingIntake，不新立
// 一路：案件侧查阅与投影查阅同属租户内运营读面，授权边界同为租户（判据同
// /visibility-catalogues 那句）。目录侧三本已接线的册子在 query_visibility_catalogues.go，
// 本端点不触碰。
//
// **不下推分诊决定参数**：页面的四结果决定集（关联既有案件、自动建立案件、进入人工
// 复核、不建案）是分诊命令面的事，查阅面收下决定参数就等于让目录读口长出第二种
// 「处置」语义。
func NewQueryExceptionTriageRecordsEndpoint(
	intake OperationsTrackingIntake,
	reader TriageReviewReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		registry := request.URL.Query().Get("registry")
		if registry != registrySignalEpisode && registry != registryDispositionRequest {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, err := intake.IntakeOperationsQuery(request.Context(), request)
		if err != nil {
			writeOperationsIntakeProblem(response, err)
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registrySignalEpisode:
			serveSignalEpisodes(response, request, reader, tenant, query.Limit)
		case registryDispositionRequest:
			serveDispositionRequests(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveSignalEpisodes(
	response http.ResponseWriter,
	request *http.Request,
	reader TriageReviewReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListSignalEpisodes(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]signalEpisodeBody, 0, len(rows))
	for _, row := range rows {
		body := signalEpisodeBody{
			EpisodeID:    row.EpisodeID,
			Parcel:       row.Parcel,
			Kind:         row.Kind,
			Rule:         row.Rule,
			Confidence:   row.Confidence,
			Hits:         row.Hits,
			StartedAt:    catalogueInstant(row.StartedAt),
			ReleaseBasis: row.ReleaseBasis,
			PriorEpisode: row.PriorEpisode,
			LastHitAt:    catalogueInstant(row.LastHitAt),
		}
		if row.EndedAt != nil {
			body.EndedAt = catalogueInstant(*row.EndedAt)
		}
		if row.TriagedAt != nil {
			body.Triage = &triageConclusionBody{
				Outcome:   row.Outcome,
				Rule:      row.OutcomeRule,
				TriagedAt: catalogueInstant(*row.TriagedAt),
			}
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, signalEpisodeListResponse{
		Outcome:  outcomeSignalEpisodesListed,
		Episodes: bodies,
	})
}

func serveDispositionRequests(
	response http.ResponseWriter,
	request *http.Request,
	reader TriageReviewReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListDispositionRequests(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]dispositionRequestBody, 0, len(rows))
	for _, row := range rows {
		body := dispositionRequestBody{
			RequestID:     row.RequestID,
			CaseID:        row.CaseID,
			TargetContext: row.TargetContext,
			Action:        row.Action,
			Scope:         row.Scope,
			Reason:        row.Reason,
			Evidence:      row.Evidence,
			IntentVersion: row.IntentVersion,
			SentAt:        catalogueInstant(row.SentAt),
			Judgment:      row.Judgment,
			Cancellation:  row.Cancellation,
			SupersededBy:  row.SupersededBy,
		}
		if row.AcceptanceWindow != nil {
			body.AcceptanceWindow = catalogueInstant(*row.AcceptanceWindow)
		}
		if row.JudgedAt != nil {
			body.JudgedAt = catalogueInstant(*row.JudgedAt)
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, dispositionRequestListResponse{
		Outcome:  outcomeDispositionRequestsListed,
		Requests: bodies,
	})
}

type signalEpisodeListResponse struct {
	Outcome  string              `json:"outcome"`
	Episodes []signalEpisodeBody `json:"episodes"`
}

// signalEpisodeBody 逐字段透出一段信号发作期连同它的分诊结论。
//
// triage 三件成对在场（0002 把发作期与结论钉成同笔提交），outcome 是分诊四结果的
// 封闭词，原样透出不折并——「进入人工复核」与「不建案」都是分诊结论，折成布尔会把
// 队列该停在哪一步这层真话抹掉。ended 两件缺席表示发作期仍活跃。页面详情区想要的
// 「事实依据」在发作期行上没有登记格，本体不带那一键（无处可登不代填，票 06
// Comments 记明）。
type signalEpisodeBody struct {
	EpisodeID    string                `json:"episodeId"`
	Parcel       string                `json:"parcel"`
	Kind         string                `json:"kind"`
	Rule         string                `json:"rule"`
	Confidence   string                `json:"confidence"`
	Hits         int64                 `json:"hits"`
	StartedAt    string                `json:"startedAt"`
	LastHitAt    string                `json:"lastHitAt"`
	ReleaseBasis string                `json:"releaseBasis,omitempty"`
	EndedAt      string                `json:"endedAt,omitempty"`
	PriorEpisode string                `json:"priorEpisode,omitempty"`
	Triage       *triageConclusionBody `json:"triage,omitempty"`
}

type triageConclusionBody struct {
	Outcome   string `json:"outcome"`
	Rule      string `json:"rule"`
	TriagedAt string `json:"triagedAt"`
}

type dispositionRequestListResponse struct {
	Outcome  string                   `json:"outcome"`
	Requests []dispositionRequestBody `json:"requests"`
}

// dispositionRequestBody 逐字段透出一份处置请求。
//
// judgment 两键成对缺席表示源上下文尚无答复（封闭四走向）；cancellation 只在已判断
// 后可能在场；supersededBy 在场即这行已被替代——替代不是删除（0005），原请求照列，
// 版本链在册面上完整可见。实际执行结果没有键：那是目标上下文按事实返回的东西，
// 行内没有它的字段，代填会把请求演成结果。
type dispositionRequestBody struct {
	RequestID        string `json:"requestId"`
	CaseID           string `json:"caseId"`
	TargetContext    string `json:"targetContext"`
	Action           string `json:"action"`
	Scope            string `json:"scope"`
	Reason           string `json:"reason"`
	Evidence         string `json:"evidence"`
	IntentVersion    int64  `json:"intentVersion"`
	SentAt           string `json:"sentAt"`
	AcceptanceWindow string `json:"acceptanceWindow,omitempty"`
	Judgment         string `json:"judgment,omitempty"`
	JudgedAt         string `json:"judgedAt,omitempty"`
	Cancellation     string `json:"cancellation,omitempty"`
	SupersededBy     string `json:"supersededBy,omitempty"`
}
