package shipmenthttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ShipmentRequestViewsQuery 是一次已授权的委托查阅列表查询。作用域来自认证与授权结果
// （CONTEXT「授权查询作用域」），页大小由接入面按渠道契约裁决——两样都不采信客户自报。
type ShipmentRequestViewsQuery struct {
	Scope domain.AuthorizedQueryScope
	Limit int
}

// ShipmentRequestViewQuery 是一次已授权的单份委托查阅。标识只定位候选对象，不单独证明
// 查询权限——权限在作用域上。
type ShipmentRequestViewQuery struct {
	Scope     domain.AuthorizedQueryScope
	RequestID domain.ShipmentRequestID
}

// ShipmentRequestViewsIntake 把一次已认证的查阅请求翻译成查询。
//
// 它是接口而非解析代码，理由与 SubmissionIntake 相同：作用域整组只能来自认证与授权
// 结果（运营查阅归操作者渠道 ADR-0100，真 Intake 未就位），采信客户自报的租户或账户会穿透 ADR-0003 的隔离边界。
// 未决期间本包不带任何实现，包括「开发用」的采信头部版本。
type ShipmentRequestViewsIntake interface {
	IntakeListQuery(ctx context.Context, request *http.Request) (ShipmentRequestViewsQuery, error)
	IntakeDetailQuery(ctx context.Context, request *http.Request) (ShipmentRequestViewQuery, error)
}

// ShipmentRequestViewsReader 是本端点消费的读口。查阅不触发判断、决定或披露——所以这里
// 接存储读面，不接应用编排（与 visibility-exception 的视图查询端点同一条分界）。
type ShipmentRequestViewsReader interface {
	ListVisible(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		limit int,
	) ([]ports.ShipmentRequestSummaryRecord, error)
	FindVisibleByID(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		requestID domain.ShipmentRequestID,
	) (ports.ShipmentRequestDetailRecord, bool, error)
}

// 编译期锁缝：读口形状与端口保持一致——本适配器不新造查询语义。
var _ ShipmentRequestViewsReader = ports.ShipmentRequestViews(nil)

// 业务结果的封闭集合。列表的空结果仍是 LISTED（谁也没被指名，空列表不泄露任何存在性）；
// 单份查阅查不到不在这里——按 ADR-0022 它是 404，`统一不可见结果`不区分「不存在」与
// 「属别的租户或客户账户」，也因此 4xx 不携带 outcome。
const (
	outcomeListed      = "LISTED"
	outcomeRequestView = "REQUEST_VIEW"
)

// codeRequestNotVisible 是`统一不可见结果`的传输层表达：单一 code、无自由文本，对象
// 存在与否、属谁，从这格答复里读不出来（ADR-0029 的探针纪律）。
const codeRequestNotVisible = "SHIPMENT_REQUEST_NOT_VISIBLE"

// NewQueryShipmentRequestViewsEndpoint 交回委托查阅的 HTTP 入口（GET /shipment-request-views）。
//
// 列表与单份共用一个端点，按 `shipmentRequestId` 查询参数分派：参数是否在场属传输形状
// （与方法检查同级，先于 Intake），读它不构成读业务内容——未配置 Intake 对两条分支同答
// 403，分支选择不泄露任何东西。
func NewQueryShipmentRequestViewsEndpoint(
	intake ShipmentRequestViewsIntake,
	reader ShipmentRequestViewsReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		if request.URL.Query().Has("shipmentRequestId") {
			serveDetail(response, request, intake, reader)
			return
		}
		serveList(response, request, intake, reader)
	})
}

func serveList(
	response http.ResponseWriter,
	request *http.Request,
	intake ShipmentRequestViewsIntake,
	reader ShipmentRequestViewsReader,
) {
	query, err := intake.IntakeListQuery(request.Context(), request)
	if err != nil {
		writeIntakeProblem(response, err)
		return
	}

	records, err := reader.ListVisible(request.Context(), query.Scope, query.Limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}

	// 空列表交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
	summaries := make([]requestSummaryBody, 0, len(records))
	for _, record := range records {
		summaries = append(summaries, summaryBodyOf(record))
	}
	writeJSON(response, http.StatusOK, viewsListResponse{
		Outcome:  outcomeListed,
		Requests: summaries,
	})
}

func serveDetail(
	response http.ResponseWriter,
	request *http.Request,
	intake ShipmentRequestViewsIntake,
	reader ShipmentRequestViewsReader,
) {
	query, err := intake.IntakeDetailQuery(request.Context(), request)
	if err != nil {
		writeIntakeProblem(response, err)
		return
	}

	record, found, err := reader.FindVisibleByID(request.Context(), query.Scope, query.RequestID)
	if err != nil {
		// 读不回是答案未形成，不是「不可见」——伪装成后者会让一次该重试的故障变成终局。
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if !found {
		writeProblem(response, http.StatusNotFound, codeRequestNotVisible)
		return
	}
	body := detailBodyOf(record)
	writeJSON(response, http.StatusOK, viewDetailResponse{
		Outcome: outcomeRequestView,
		Request: &body,
	})
}

// writeIntakeProblem 是本包 Intake 失败的唯一映射：各口不再各抄一份。操作者渠道的四格与未配置并列，各对一种
// 恢复动作（ADR-0029）。
func writeIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	if errors.Is(err, ErrOperatorCredentialRejected) {
		writeProblem(response, http.StatusUnauthorized, codeOperatorCredentialRejected)
		return
	}
	if errors.Is(err, ErrOperatorNotGranted) {
		writeProblem(response, http.StatusForbidden, codeOperatorNotGranted)
		return
	}
	if errors.Is(err, ErrOutsideAdmissionScope) {
		writeProblem(response, http.StatusForbidden, codeOutsideAdmissionScope)
		return
	}
	if errors.Is(err, ErrIdentityDependencyUnavailable) {
		writeProblem(response, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
		return
	}
	if errors.Is(err, ErrMalformedRequest) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

type viewsListResponse struct {
	Outcome  string               `json:"outcome"`
	Requests []requestSummaryBody `json:"requests"`
}

type viewDetailResponse struct {
	Outcome string             `json:"outcome"`
	Request *requestDetailBody `json:"request"`
}

type requestSummaryBody struct {
	ShipmentRequestID   string `json:"shipmentRequestId"`
	CustomerAccountID   string `json:"customerAccountId"`
	Source              string `json:"source"`
	SourceRequestKey    string `json:"sourceRequestKey"`
	State               string `json:"state"`
	SubmissionVersionID string `json:"submissionVersionId"`
	DeclaredParcelCount int    `json:"declaredParcelCount"`
	SubmittedAt         string `json:"submittedAt"`
}

type requestDetailBody struct {
	requestSummaryBody
	BatchID           string               `json:"batchId"`
	OccurredAt        string               `json:"occurredAt"`
	ReceivedAt        string               `json:"receivedAt"`
	DeclaredParcels   []declaredParcelBody `json:"declaredParcels"`
	PriorVersionCount int                  `json:"priorVersionCount"`
	AcceptanceTask    acceptanceTaskBody   `json:"acceptanceTask"`
	Decision          *decisionBody        `json:"decision,omitempty"`
}

type declaredParcelBody struct {
	ParcelID            string                `json:"parcelId"`
	DeclaredWeightValue string                `json:"declaredWeightValue,omitempty"`
	DeclaredWeightUnit  string                `json:"declaredWeightUnit,omitempty"`
	Dimensions          *parcelDimensionsBody `json:"dimensions,omitempty"`
}

type parcelDimensionsBody struct {
	Length string `json:"length"`
	Width  string `json:"width"`
	Height string `json:"height"`
	Unit   string `json:"unit"`
}

type acceptanceTaskBody struct {
	State                   string `json:"state"`
	LastAttemptReason       string `json:"lastAttemptReason,omitempty"`
	LastAttemptContinuation string `json:"lastAttemptContinuation,omitempty"`
	LastAttemptedAt         string `json:"lastAttemptedAt,omitempty"`
}

// decisionBody 的 kind 词汇与撤回端点的 decisionKind 同一套（ACCEPTED/REJECTED），
// 不另造第二组决定词。
type decisionBody struct {
	DecisionID string `json:"decisionId"`
	Kind       string `json:"kind"`
	DecidedAt  string `json:"decidedAt"`
}

func summaryBodyOf(record ports.ShipmentRequestSummaryRecord) requestSummaryBody {
	return requestSummaryBody{
		ShipmentRequestID:   record.ShipmentRequestID.String(),
		CustomerAccountID:   record.CustomerAccountID.String(),
		Source:              record.Source.String(),
		SourceRequestKey:    record.SourceRequestKey.String(),
		State:               record.State.String(),
		SubmissionVersionID: record.SubmissionVersionID.String(),
		DeclaredParcelCount: record.DeclaredParcelCount,
		SubmittedAt:         record.SubmittedAt.UTC().Format(time.RFC3339Nano),
	}
}

func detailBodyOf(record ports.ShipmentRequestDetailRecord) requestDetailBody {
	body := requestDetailBody{
		requestSummaryBody: summaryBodyOf(record.ShipmentRequestSummaryRecord),
		BatchID:            record.BatchID.String(),
		OccurredAt:         record.OccurredAt.UTC().Format(time.RFC3339Nano),
		ReceivedAt:         record.ReceivedAt.UTC().Format(time.RFC3339Nano),
		DeclaredParcels:    make([]declaredParcelBody, 0, len(record.DeclaredParcels)),
		PriorVersionCount:  record.PriorVersionCount,
		AcceptanceTask:     acceptanceTaskBody{State: record.Task.State.String()},
	}
	for _, parcel := range record.DeclaredParcels {
		parcelBody := declaredParcelBody{
			ParcelID:            parcel.Parcel.String(),
			DeclaredWeightValue: parcel.WeightValue,
			DeclaredWeightUnit:  parcel.WeightUnit,
		}
		if parcel.HasDimensions {
			parcelBody.Dimensions = &parcelDimensionsBody{
				Length: parcel.Length,
				Width:  parcel.Width,
				Height: parcel.Height,
				Unit:   parcel.DimensionsUnit,
			}
		}
		body.DeclaredParcels = append(body.DeclaredParcels, parcelBody)
	}
	if record.Task.HasAttempt {
		body.AcceptanceTask.LastAttemptReason = record.Task.LastAttemptReason
		body.AcceptanceTask.LastAttemptContinuation = record.Task.LastAttemptContinuation
		body.AcceptanceTask.LastAttemptedAt = record.Task.LastAttemptedAt.UTC().Format(time.RFC3339Nano)
	}
	if record.HasDecision {
		kind := "REJECTED"
		if record.Decision.Accepted {
			kind = "ACCEPTED"
		}
		body.Decision = &decisionBody{
			DecisionID: record.Decision.DecisionID.String(),
			Kind:       kind,
			DecidedAt:  record.Decision.DecidedAt.UTC().Format(time.RFC3339Nano),
		}
	}
	return body
}
