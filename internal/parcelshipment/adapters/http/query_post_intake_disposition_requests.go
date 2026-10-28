package shipmenthttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 等收寄后服务处置决定回应的停止请求的运营查阅端点（ADR-0332）。只有读：形成决定是「运营决定」能力面上另一个端点，查阅面里没有动作。

// PostIntakeDispositionRequestQuery 是一次已授权的查阅：作用域只有租户，页大小由接入面按渠道契约裁决，两样都不采信调用方自报。
type PostIntakeDispositionRequestQuery struct {
	Tenant domain.TenantID
	Limit  int
}

// PostIntakeDispositionRequestQueryIntake 把一次已认证的运营查阅请求翻译成查询；理由同冲突册查阅：作用域整组只能来自认证与授权结果。
type PostIntakeDispositionRequestQueryIntake interface {
	IntakePostIntakeDispositionRequestQuery(ctx context.Context, request *http.Request) (PostIntakeDispositionRequestQuery, error)
}

// PostIntakeDispositionRequestReader 是本端点消费的读口：查阅不触发决定，所以接存储读面。
type PostIntakeDispositionRequestReader interface {
	ListUnansweredPostIntakeDispositionRequests(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) (ports.UnansweredPostIntakeDispositionRequests, error)
}

var (
	_ PostIntakeDispositionRequestReader      = ports.PostIntakeDispositionRequests(nil)
	_ PostIntakeDispositionRequestQueryIntake = UnconfiguredIntake{}
)

// IntakePostIntakeDispositionRequestQuery 不读请求：渠道未配置就无从铸造作用域。
func (UnconfiguredIntake) IntakePostIntakeDispositionRequestQuery(context.Context, *http.Request) (PostIntakeDispositionRequestQuery, error) {
	return PostIntakeDispositionRequestQuery{}, ErrAccessChannelNotConfigured
}

// 空清单仍是 LISTED：「此刻没有等回应的请求」是如实答案，与「接入渠道未配置」（403）分得开。
const outcomePostIntakeDispositionRequestsListed = "POST_INTAKE_DISPOSITION_REQUESTS_LISTED"

// NewQueryPostIntakeDispositionRequestsEndpoint 交回等回应请求查阅的 HTTP 入口（GET /post-intake-disposition-requests）。
func NewQueryPostIntakeDispositionRequestsEndpoint(
	intake PostIntakeDispositionRequestQueryIntake,
	reader PostIntakeDispositionRequestReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakePostIntakeDispositionRequestQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		listed, err := reader.ListUnansweredPostIntakeDispositionRequests(request.Context(), query.Tenant, query.Limit)
		if err != nil {
			// 读不回是答案未形成，不伪装成空清单：把一次该重试的故障读成「没有等回应的请求」，卡住的包裹就更没人看见了。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]postIntakeDispositionRequestBody, 0, len(listed.Requests))
		for _, entry := range listed.Requests {
			bodies = append(bodies, postIntakeDispositionRequestBodyOf(entry))
		}
		writeJSON(response, http.StatusOK, postIntakeDispositionRequestsResponse{
			Outcome:      outcomePostIntakeDispositionRequestsListed,
			PendingCount: listed.Count,
			Truncated:    listed.Count > len(listed.Requests),
			Requests:     bodies,
		})
	})
}

type postIntakeDispositionRequestsResponse struct {
	Outcome      string                             `json:"outcome"`
	PendingCount int                                `json:"pendingCount"`
	Truncated    bool                               `json:"truncated"`
	Requests     []postIntakeDispositionRequestBody `json:"requests"`
}

// postIntakeDispositionRequestBody 逐字段透出一份等回应的请求：决定命令要的几段（请求键、包裹、当前决定版本）原样给出；请求方、授权、原因与请求的业务时间
// 在先于保全落下的待处置记录上为空，空串表示「当时没有保全」，不表示没有请求方。
type postIntakeDispositionRequestBody struct {
	RequestKey     string `json:"requestKey"`
	ParcelID       string `json:"parcelId"`
	Origin         string `json:"origin"`
	IntakeBefore   string `json:"intakeBefore,omitempty"`
	Cancellation   string `json:"cancellation,omitempty"`
	Requester      string `json:"requester,omitempty"`
	Authority      string `json:"authority,omitempty"`
	Reason         string `json:"reason,omitempty"`
	RequestedAt    string `json:"requestedAt,omitempty"`
	RecordedAt     string `json:"recordedAt"`
	CurrentVersion int    `json:"currentVersion"`
}

func postIntakeDispositionRequestBodyOf(entry ports.PostIntakeDispositionRequest) postIntakeDispositionRequestBody {
	body := postIntakeDispositionRequestBody{
		RequestKey:     entry.Key.RequestKey.String(),
		ParcelID:       entry.Key.Parcel.String(),
		Origin:         entry.Origin.String(),
		IntakeBefore:   entry.IntakeBefore.String(),
		Cancellation:   entry.Cancellation.String(),
		Requester:      entry.Requester.String(),
		Authority:      entry.Authority.String(),
		Reason:         entry.Reason.String(),
		RecordedAt:     entry.RecordedAt.UTC().Format(time.RFC3339Nano),
		CurrentVersion: entry.CurrentVersion,
	}
	if !entry.RequestedAt.IsZero() {
		body.RequestedAt = entry.RequestedAt.UTC().Format(time.RFC3339Nano)
	}
	return body
}
