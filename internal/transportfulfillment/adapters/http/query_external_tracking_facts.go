package tfhttp

import (
	"context"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ExternalTrackingFactReviewReader 是本端点消费的读口：外部承运轨迹事实的当前版列表（管理台「有效时间
// 判断」页，label-channel/21）。
type ExternalTrackingFactReviewReader interface {
	ListCurrentExternalTrackingFacts(
		ctx context.Context,
		tenant domain.TenantID,
		source domain.TrackingSourceReference,
		filter ports.EffectiveTimeReviewFilter,
		limit int,
	) ([]ports.ExternalTrackingFactReviewRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ ExternalTrackingFactReviewReader = ports.ExternalTrackingFactReviewRead(nil)

const (
	outcomePendingEffectiveTimeFactsListed    = "PENDING_EFFECTIVE_TIME_FACTS_LISTED"
	outcomeCurrentExternalTrackingFactsListed = "CURRENT_EXTERNAL_TRACKING_FACTS_LISTED"
)

// 视图词是传输形状，与 ports.EffectiveTimeReviewFilter 一一对应；两格各有自己的 outcome 词，读的人不必
// 回看请求就知道拿到的是「该判哪几条」还是「这家源此刻的全部当前版」。
const (
	viewPending = "pending"
	viewCurrent = "current"
)

// NewQueryExternalTrackingFactsEndpoint 交回外部承运轨迹事实查阅的 HTTP 入口
// （GET /transport-fulfillment-external-tracking-facts?source=…&view=pending|current）。
//
// 它是判断面的读半边：判断人先看这家源有哪些待判断的当前版，再逐条到写口判。查阅零登记零编辑零披露，
// 消费本上下文自己的存储读面，因此走目录查阅那条准入（CatalogueQueryIntake，ADR-0077/0078）；写半边在
// judge_effective_time.go，挂命令面的准入，两边的 Intake 不可互换。
//
// 源是必备维而不是过滤器：规则按源登记、回填按源重判，判断人也按源看；缺席不猜「全部源」。视图同理
// 必备——「没传就当待判断」是隐含默认，与读口拒绝集外过滤词是同一条纪律。状态词原样透出不解释
// （ADR-0102 决定五）；**不给任何「推荐的有效时间」**——那是替所有者判断（票 21 红线）。
func NewQueryExternalTrackingFactsEndpoint(
	intake CatalogueQueryIntake,
	reader ExternalTrackingFactReviewReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		source, err := domain.NewTrackingSourceReference(request.URL.Query().Get("source"))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		var filter ports.EffectiveTimeReviewFilter
		var outcome string
		switch strings.TrimSpace(request.URL.Query().Get("view")) {
		case viewPending:
			filter, outcome = ports.PendingEffectiveTimeOnly, outcomePendingEffectiveTimeFactsListed
		case viewCurrent:
			filter, outcome = ports.EveryCurrentVersion, outcomeCurrentExternalTrackingFactsListed
		default:
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}

		rows, err := reader.ListCurrentExternalTrackingFacts(request.Context(), query.Scope.Tenant(), source, filter, query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]externalTrackingFactBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, externalTrackingFactBody{
				Fact:                 row.Fact,
				Version:              row.Version,
				Source:               row.Source,
				Credential:           row.Credential,
				Object:               row.Object,
				SourceEvent:          row.SourceEvent,
				Status:               row.Status,
				OccurredAt:           rfc3339(row.OccurredAt),
				ReceivedAt:           rfc3339(row.ReceivedAt),
				EffectiveBasis:       row.EffectiveBasis,
				EffectiveAt:          optionalInstant(row.EffectiveAt),
				EffectiveRule:        row.EffectiveRule,
				EffectiveRuleVersion: row.EffectiveRuleVersion,
				Supersedes:           row.Supersedes,
				Origin:               row.Origin,
				RecordedAt:           rfc3339(row.RecordedAt),
			})
		}
		writeJSON(response, http.StatusOK, externalTrackingFactListResponse{Outcome: outcome, Facts: bodies})
	})
}

type externalTrackingFactListResponse struct {
	Outcome string                     `json:"outcome"`
	Facts   []externalTrackingFactBody `json:"facts"`
}

// externalTrackingFactBody 逐字段透出一条当前版。
//
// 三个时间三键，归属不同（ADR-0102）：occurredAt 源给、receivedAt 本上下文铸、effectiveAt 只在判断过时在场
// ——待判断整键缺席，缺席就是「还没有人判」，不拿另两个时间顶上。effectiveRule 两键只随按规则判断在场；
// supersedes 首版缺席；origin 分素材到达（MATERIAL）与判断（JUDGMENT）形成的版本。sourceEvent 源未给即缺席
// （ADR-0102 决定四：不按内容补一个）。
type externalTrackingFactBody struct {
	Fact                 string `json:"fact"`
	Version              string `json:"version"`
	Source               string `json:"source"`
	Credential           string `json:"credential"`
	Object               string `json:"object"`
	SourceEvent          string `json:"sourceEvent,omitempty"`
	Status               string `json:"status"`
	OccurredAt           string `json:"occurredAt"`
	ReceivedAt           string `json:"receivedAt"`
	EffectiveBasis       string `json:"effectiveBasis"`
	EffectiveAt          string `json:"effectiveAt,omitempty"`
	EffectiveRule        string `json:"effectiveRule,omitempty"`
	EffectiveRuleVersion string `json:"effectiveRuleVersion,omitempty"`
	Supersedes           string `json:"supersedes,omitempty"`
	Origin               string `json:"origin"`
	RecordedAt           string `json:"recordedAt"`
}
