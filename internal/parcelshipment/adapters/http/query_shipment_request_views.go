package shipmenthttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.ShipmentRequestSummaryRecord], error)
	FindVisibleByID(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		requestID domain.ShipmentRequestID,
	) (ports.ShipmentRequestDetailRecord, bool, error)
	ListVisibleByDocumentNumber(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		number domain.ShipmentDocumentNumber,
	) ([]ports.ShipmentRequestSummaryRecord, error)
	ListVisibleByIDs(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		requestIDs []domain.ShipmentRequestID,
	) ([]ports.ShipmentRequestSummaryRecord, error)
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
//
// 列表分支另收 `documentNumber`（运单号，ADR-0355 决定十一「可按单据号检索」）：带了就只答单据号逐字等于它的那几行，
// 仍是 LISTED——没有是空数组，不是 404：管理台拿运单号解析委托标识，要分得出「没有这个运单号」与「读不动」。作用域与
// 不带参数时同一份（Intake 的列表查询），值在 Intake 之后才读，未配置的渠道照旧先答 403。只有值的编码在本处理器之前判：
// 解码后含 NUL 或不是合法 UTF-8，装配处挂的 httpapi.RefuseUnstorableReadTarget 先答 400，未配置的渠道也是（传输形状）。
//
// 列表分支还收 `shipmentRequestIds`（逗号分隔的一组委托标识，至多 maxShipmentRequestIDsPerLookup 个）：别的上下文的页面
// 上出现委托标识时，管理台按页上那几份一次取回运单号（ADR-0355 决定十一），引用方的读模型不复制单据号。作用域仍是列表
// 查询那一份，与 `shipmentRequestId` 单份查阅同一套；作用域外的与查无的都不出现，仍答 LISTED。两种筛选不并用。
//
// 不带这两个参数的列表按 ADR-0144 翻页：查询参数按 ports.ShipmentRequestViewPages 解码，在 Intake 之后（未配置的渠道对一切
// 请求同答，ADR-0055 决定三），答复带 page。按运单号与按一组标识的两支是有界查找，不解码、不带 page。
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

		// 分派先于 Intake，只能照 URL.Query() 宽松地看键在不在，理由在 cataloguepage.ParseQuery 头注。
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
	values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		writePageQueryProblem(response, err)
		return
	}
	var pageQuery cataloguepage.Query
	if !values.Has("documentNumber") && !values.Has("shipmentRequestIds") {
		if pageQuery, err = ports.ShipmentRequestViewPages.Decode(values); err != nil {
			writePageQueryProblem(response, err)
			return
		}
	}

	var records []ports.ShipmentRequestSummaryRecord
	var page *cataloguepage.Page
	switch {
	case values.Has("documentNumber") && values.Has("shipmentRequestIds"):
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	case values.Has("documentNumber"):
		number, parseErr := domain.NewShipmentDocumentNumber(values.Get("documentNumber"))
		if parseErr != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		records, err = reader.ListVisibleByDocumentNumber(request.Context(), query.Scope, number)
	case values.Has("shipmentRequestIds"):
		requestIDs, parsed := shipmentRequestIDsOf(values["shipmentRequestIds"])
		if !parsed {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		records, err = reader.ListVisibleByIDs(request.Context(), query.Scope, requestIDs)
	default:
		var listed cataloguepage.Result[ports.ShipmentRequestSummaryRecord]
		if listed, err = reader.ListVisible(request.Context(), query.Scope, query.Limit, pageQuery); err == nil {
			records = listed.Rows
			spelled := listed.Page(query.Limit)
			page = &spelled
		}
	}
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
		Page:     page,
	})
}

// maxShipmentRequestIDsPerLookup 是一次按标识集合查阅最多带几个委托标识。上限按请求行定：委托标识三十来个字符，一百个连同
// 分隔符约三千字节，留在常见代理对请求行的限度之内；调用方一页的行多时按这个数分批问。超出是坏请求、不截断——截断会让
// 调用方把没问到的那几份当成看不见。
const maxShipmentRequestIDsPerLookup = 100

// shipmentRequestIDsOf 解开 `shipmentRequestIds`：只认一格、逗号分隔、每段都是委托标识，重复的去重；空段、多给一格或
// 超出上限都答不成立。
func shipmentRequestIDsOf(raw []string) ([]domain.ShipmentRequestID, bool) {
	if len(raw) != 1 {
		return nil, false
	}
	parts := strings.Split(raw[0], ",")
	if len(parts) > maxShipmentRequestIDsPerLookup {
		return nil, false
	}
	seen := make(map[string]bool, len(parts))
	requestIDs := make([]domain.ShipmentRequestID, 0, len(parts))
	for _, part := range parts {
		requestID, err := domain.NewShipmentRequestID(part)
		if err != nil {
			return nil, false
		}
		if seen[requestID.String()] {
			continue
		}
		seen[requestID.String()] = true
		requestIDs = append(requestIDs, requestID)
	}
	return requestIDs, true
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

// writeIntakeProblem 是本包 Intake 失败的唯一映射：各口不再各抄一份。操作者渠道与首方客户族的各格与未配置并列，
// 各对一种恢复动作（ADR-0029）；操作者渠道与首方客户族的令牌格与授予格各用各的错误码，不合并。
func writeIntakeProblem(response http.ResponseWriter, err error) {
	answerIntakeFailure(legacyProblems(response), err)
}

// answerIntakeFailure 是 writeIntakeProblem 的映射本身，客户面处理器以问题详情的写法调它。
func answerIntakeFailure(problem problemWriter, err error) {
	switch {
	case errors.Is(err, ErrAccessChannelNotConfigured):
		problem(http.StatusForbidden, codeAccessChannelNotConfigured, "")
	case errors.Is(err, ErrOperatorCredentialRejected):
		problem(http.StatusUnauthorized, codeOperatorCredentialRejected, "")
	case errors.Is(err, ErrOperatorNotGranted):
		problem(http.StatusForbidden, codeOperatorNotGranted, "")
	case errors.Is(err, ErrCustomerClientCredentialRejected):
		problem(http.StatusUnauthorized, codeCustomerClientCredentialRejected, "")
	case errors.Is(err, ErrCustomerClientNotGranted):
		problem(http.StatusForbidden, codeCustomerClientNotGranted, "")
	case errors.Is(err, ErrOutsideAdmissionScope):
		problem(http.StatusForbidden, codeOutsideAdmissionScope, "")
	case errors.Is(err, ErrIdentityDependencyUnavailable):
		problem(http.StatusServiceUnavailable, codeIdentityDependencyUnavailable, "")
	case errors.Is(err, ErrMalformedRequest):
		detail := ""
		if malformed := (*malformedFieldError)(nil); errors.As(err, &malformed) {
			detail = malformed.detail()
		}
		problem(http.StatusBadRequest, codeMalformedRequest, detail)
	default:
		problem(http.StatusInternalServerError, codeIntakeFailed, "")
	}
}

// viewsListResponse 的 page 只在翻页的那一支在场：按运单号与按一组标识的查找不是一页目录。
type viewsListResponse struct {
	Outcome  string               `json:"outcome"`
	Requests []requestSummaryBody `json:"requests"`
	Page     *cataloguepage.Page  `json:"page,omitempty"`
}

type viewDetailResponse struct {
	Outcome string             `json:"outcome"`
	Request *requestDetailBody `json:"request"`
}

// requestSummaryBody 的 `documentNumber` 是委托的单据号（管理台叫运单号，ADR-0355 决定十一），没有号时整键不出现、不拿委托标识
// 顶；`documentNumberAbsence` 只在没有号时在场，说是哪一种（ISSUED_BEFORE_NUMBERING / NUMBERING_TIME_ZONE_NOT_CONFIGURED），
// 管理台按它在技术详情里注明，不一律写成上线前开立。
type requestSummaryBody struct {
	ShipmentRequestID     string `json:"shipmentRequestId"`
	DocumentNumber        string `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string `json:"documentNumberAbsence,omitempty"`
	CustomerAccountID     string `json:"customerAccountId"`
	Source                string `json:"source"`
	SourceRequestKey      string `json:"sourceRequestKey"`
	State                 string `json:"state"`
	SubmissionVersionID   string `json:"submissionVersionId"`
	DeclaredParcelCount   int    `json:"declaredParcelCount"`
	SubmittedAt           string `json:"submittedAt"`
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
	// Goods 是此刻在用的货物明细（有已采用的资料修订就是那一版），PlatformShipment 是委托上的平台货件（ADR-0373、ADR-0374），
	// 没报即整键缺席。
	Goods            *goodsViewBody            `json:"goods,omitempty"`
	PlatformShipment *platformShipmentViewBody `json:"platformShipment,omitempty"`
	// GoodsStanding 说 goods 出自哪里：ON_SUBMISSION（提交时报的）、ON_ADOPTED_VERSION（资料修订后的那一版）或
	// UNDETERMINED（两条修订分叉待复核，不交 goods）。查阅的人要分得清「没报」与「说不出该按哪一版」。
	GoodsStanding string `json:"goodsStanding,omitempty"`
	// FinancialControl 是受理判断记下的接受前财务控制结论（ADR-0379 决定三），形状与复核详情的 recordedJudgments.financialControl
	// 同一份（financialControlBodyOf）；没判过即整键缺席。不带金额：估价与占用由页面另向结算的读口取。
	FinancialControl *financialControlBody `json:"financialControl,omitempty"`
}

type declaredParcelBody struct {
	ParcelID            string                `json:"parcelId"`
	DeclaredWeightValue string                `json:"declaredWeightValue,omitempty"`
	DeclaredWeightUnit  string                `json:"declaredWeightUnit,omitempty"`
	Dimensions          *parcelDimensionsBody `json:"dimensions,omitempty"`
	// 这一件的客户件号、平台箱标号与装箱分配，形状与提交口的包裹草案对应；跟踪号是渠道签发的，不在提交口。没有即整键缺席。
	CustomerParcelNumber string            `json:"customerParcelNumber,omitempty"`
	PlatformBoxLabel     string            `json:"platformBoxLabel,omitempty"`
	TrackingNumbers      []string          `json:"trackingNumbers,omitempty"`
	Packing              []packingViewBody `json:"packing,omitempty"`
}

type parcelDimensionsBody struct {
	Length string `json:"length"`
	Width  string `json:"width"`
	Height string `json:"height"`
	Unit   string `json:"unit"`
}

// acceptanceTaskBody 的「上次处理」几格读自处理尝试实际落库的那一处（ADR-0380 决定四）。lastAttemptResumePath 交续办方原词
// （INTERNAL_RETRY、OPERATOR_REGISTRATION……，与 waitingOn 同一套）：谁来动手照记下的那一格读，不从原因名或 CONT- 引用推。
// lastAttemptSettlementReason 是结算的未形成原因码原样（不冠 SA-），lastAttemptServiceProduct 是这一版申报的服务产品，
// 两格都只在那一轮有时在场（ADR-0380 决定三）。字段名与管理台票 declared-goods-lines/24 钉死。
// lastAttemptEstimateGapCodes 是申报不全那一轮计价点名的缺项码原样、去重（ADR-0381 决定四），只在申报不全分出的两路
// 等待上在场；字段名钉死给管理台票 declared-goods-lines/27。
type acceptanceTaskBody struct {
	State                       string   `json:"state"`
	LastAttemptReason           string   `json:"lastAttemptReason,omitempty"`
	LastAttemptResumePath       string   `json:"lastAttemptResumePath,omitempty"`
	LastAttemptContinuation     string   `json:"lastAttemptContinuation,omitempty"`
	LastAttemptedAt             string   `json:"lastAttemptedAt,omitempty"`
	LastAttemptSettlementReason string   `json:"lastAttemptSettlementReason,omitempty"`
	LastAttemptServiceProduct   string   `json:"lastAttemptServiceProduct,omitempty"`
	LastAttemptEstimateGapCodes []string `json:"lastAttemptEstimateGapCodes,omitempty"`
	// SupplementGaps 是停在等待受控补充时缺的各项（ADR-0376 决定一）：查阅的人要知道缺的是哪一条、哪几箱，而不只是
	// 「在等补充」。
	SupplementGaps []supplementGapBody `json:"supplementGaps,omitempty"`
}

type supplementGapBody struct {
	Group    string `json:"group"`
	ParcelID string `json:"parcelId,omitempty"`
	Reason   string `json:"reason"`
}

// decisionBody 的 kind 词汇与撤回端点的 decisionKind 同一套（ACCEPTED/REJECTED），
// 不另造第二组决定词。
type decisionBody struct {
	DecisionID string `json:"decisionId"`
	Kind       string `json:"kind"`
	DecidedAt  string `json:"decidedAt"`
}

func summaryBodyOf(record ports.ShipmentRequestSummaryRecord) requestSummaryBody {
	body := requestSummaryBody{
		ShipmentRequestID:   record.ShipmentRequestID.String(),
		CustomerAccountID:   record.CustomerAccountID.String(),
		Source:              record.Source.String(),
		SourceRequestKey:    record.SourceRequestKey.String(),
		State:               record.State.String(),
		SubmissionVersionID: record.SubmissionVersionID.String(),
		DeclaredParcelCount: record.DeclaredParcelCount,
		SubmittedAt:         record.SubmittedAt.UTC().Format(time.RFC3339Nano),
	}
	if number, numbered := record.DocumentNumbering.Number(); numbered {
		body.DocumentNumber = number.String()
	}
	if absence, absent := record.DocumentNumbering.Absence(); absent {
		body.DocumentNumberAbsence = absence.String()
	}
	return body
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
	for _, gap := range record.Task.SupplementGaps {
		body.AcceptanceTask.SupplementGaps = append(body.AcceptanceTask.SupplementGaps,
			supplementGapBody{Group: gap.Group.String(), ParcelID: gap.Parcel, Reason: gap.Reason})
	}
	packing := packingViewBodiesByParcel(record.Goods)
	for _, parcel := range record.DeclaredParcels {
		parcelBody := declaredParcelBody{
			ParcelID:             parcel.Parcel.String(),
			DeclaredWeightValue:  parcel.WeightValue,
			DeclaredWeightUnit:   parcel.WeightUnit,
			CustomerParcelNumber: parcel.CustomerParcelNumber,
			PlatformBoxLabel:     parcel.PlatformBoxLabel,
			TrackingNumbers:      parcel.TrackingNumbers,
			Packing:              packing[parcel.Parcel],
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
	body.Goods = goodsViewBodyOf(record.Goods)
	// 提交时没报、也没修订过的委托不带这一格：那是绝大多数委托，交一个「出自提交」却没有 goods 只是噪声。
	if record.Goods.Declared() || record.GoodsStanding != domain.GoodsDetailsOnSubmission {
		body.GoodsStanding = record.GoodsStanding.String()
	}
	body.PlatformShipment = platformShipmentViewBodyOf(record.PlatformShipment)
	body.FinancialControl = financialControlBodyOf(record.FinancialControl)
	if record.Task.HasAttempt {
		body.AcceptanceTask.LastAttemptReason = record.Task.LastAttemptReason
		body.AcceptanceTask.LastAttemptResumePath = record.Task.LastAttemptResumePath.String()
		body.AcceptanceTask.LastAttemptContinuation = record.Task.LastAttemptContinuation
		body.AcceptanceTask.LastAttemptedAt = record.Task.LastAttemptedAt.UTC().Format(time.RFC3339Nano)
		body.AcceptanceTask.LastAttemptSettlementReason = record.Task.LastAttemptSettlementReason
		body.AcceptanceTask.LastAttemptServiceProduct = record.Task.LastAttemptServiceProduct
		body.AcceptanceTask.LastAttemptEstimateGapCodes = record.Task.LastAttemptEstimateGapCodes
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
