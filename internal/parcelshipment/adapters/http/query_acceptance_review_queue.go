package shipmenthttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 复核队列查阅端点（票 acceptance-review-read-face/01）。列的是「当前停在等待人工
// 复核」的委托：等待态由 Decide 看过全部校验后写在任务文档上，读面照登记过滤，不在
// 读侧重推域判断。本端点只有读——复核完成的命令面（应用编排、授权判定、续办触发）
// 今天不存在，单列票 02 承载；查阅面收决定等于让读口长出第二种「处置」语义。
//
// Intake 复用 ShipmentRequestViewsIntake：复核队列是委托查阅面的子集视图，作用域
// 语义（CONTEXT「授权查询作用域」）与页大小裁决同一套，另造第二种准入形只会让隔离读
// 准入（ADR-0078）多一个要记得换值的变量。

// AcceptanceReviewQueueReader 是本端点消费的队列读口。详情方法与委托查阅读口同签名
// ——复核详情复用委托查阅详情，同一真库适配器同时满足两口。
type AcceptanceReviewQueueReader interface {
	ListAwaitingManualReview(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		limit int,
	) ([]ports.AcceptanceReviewQueueRecord, error)
	FindVisibleByID(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		requestID domain.ShipmentRequestID,
	) (ports.ShipmentRequestDetailRecord, bool, error)
}

// 编译期锁缝：读口形状与端口保持一致——本适配器不新造查询语义。
var _ AcceptanceReviewQueueReader = ports.AcceptanceReviewQueue(nil)

// RecordedJudgmentsReader 是详情分支消费的判断读口：复核角色审的正是「已记录的权威
// 判断都说了什么」，可达性、财务控制与采用解析三样照登记转写，与形成决定那一步读的
// 是同一批判断行。
type RecordedJudgmentsReader interface {
	LoadRecordedJudgments(
		ctx context.Context,
		tenant domain.TenantID,
		requestID domain.ShipmentRequestID,
	) (ports.RecordedJudgments, error)
}

var _ RecordedJudgmentsReader = ports.RecordedJudgmentReader(nil)

// 业务结果的封闭集合。列表空结果仍是 REVIEW_QUEUE_LISTED（空队列是答案不是错误）；
// 单份查不到不在这里——统一不可见走 404 + codeRequestNotVisible，与委托查阅同一枚。
const (
	outcomeReviewQueueListed = "REVIEW_QUEUE_LISTED"
	outcomeReviewCase        = "REVIEW_CASE"
)

// NewQueryAcceptanceReviewQueueEndpoint 交回复核队列的 HTTP 入口
// （GET /acceptance-review-queue）。列表与单份共用一个端点，按 `shipmentRequestId`
// 查询参数分派——分派规则、方法门与错误分流全部照 /shipment-request-views 的形。
func NewQueryAcceptanceReviewQueueEndpoint(
	intake ShipmentRequestViewsIntake,
	reader AcceptanceReviewQueueReader,
	judgments RecordedJudgmentsReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		if request.URL.Query().Has("shipmentRequestId") {
			serveReviewCase(response, request, intake, reader, judgments)
			return
		}
		serveReviewQueue(response, request, intake, reader)
	})
}

func serveReviewQueue(
	response http.ResponseWriter,
	request *http.Request,
	intake ShipmentRequestViewsIntake,
	reader AcceptanceReviewQueueReader,
) {
	query, err := intake.IntakeListQuery(request.Context(), request)
	if err != nil {
		writeIntakeProblem(response, err)
		return
	}

	records, err := reader.ListAwaitingManualReview(request.Context(), query.Scope, query.Limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}

	entries := make([]reviewQueueEntryBody, 0, len(records))
	for _, record := range records {
		entries = append(entries, reviewQueueEntryBodyOf(record))
	}
	writeJSON(response, http.StatusOK, reviewQueueListResponse{
		Outcome: outcomeReviewQueueListed,
		Entries: entries,
	})
}

func serveReviewCase(
	response http.ResponseWriter,
	request *http.Request,
	intake ShipmentRequestViewsIntake,
	reader AcceptanceReviewQueueReader,
	judgments RecordedJudgmentsReader,
) {
	query, err := intake.IntakeDetailQuery(request.Context(), request)
	if err != nil {
		writeIntakeProblem(response, err)
		return
	}

	record, found, err := reader.FindVisibleByID(request.Context(), query.Scope, query.RequestID)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if !found {
		writeProblem(response, http.StatusNotFound, codeRequestNotVisible)
		return
	}
	recorded, err := judgments.LoadRecordedJudgments(request.Context(), query.Scope.TenantID(), query.RequestID)
	if err != nil {
		// 判断读不回同样是答案未形成：详情缺了判断三组就不是复核要看的那份详情，
		// 砍半作答会让复核对着不完整的事实拍板。
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}

	body := detailBodyOf(record)
	writeJSON(response, http.StatusOK, reviewCaseResponse{
		Outcome:   outcomeReviewCase,
		Request:   &body,
		Review:    reviewStatusBodyOf(record.Task),
		Judgments: recordedJudgmentsBodyOf(recorded),
	})
}

type reviewQueueListResponse struct {
	Outcome string                 `json:"outcome"`
	Entries []reviewQueueEntryBody `json:"entries"`
}

type reviewCaseResponse struct {
	Outcome string             `json:"outcome"`
	Request *requestDetailBody `json:"request"`
	// review 与 recordedJudgments 不折进 request：前者是任务文档上的等待与留痕，
	// 后者是判断表上的另一批行——两样都是复核语境特有的面，委托查阅详情不背它们。
	Review    reviewStatusBody      `json:"review"`
	Judgments recordedJudgmentsBody `json:"recordedJudgments"`
}

type reviewQueueEntryBody struct {
	requestSummaryBody
	LastAttemptReason       string `json:"lastAttemptReason,omitempty"`
	LastAttemptContinuation string `json:"lastAttemptContinuation,omitempty"`
	LastAttemptedAt         string `json:"lastAttemptedAt,omitempty"`
	ReviewCompleted         bool   `json:"reviewCompleted"`
	ReviewAuthority         string `json:"reviewAuthority,omitempty"`
	ReviewReviewer          string `json:"reviewReviewer,omitempty"`
	ReviewEvidence          string `json:"reviewEvidence,omitempty"`
	ReviewCompletedAt       string `json:"reviewCompletedAt,omitempty"`
}

// reviewStatusBody 是任务文档上「停在哪、复核录了没」的传输形。waitingOn 取
// ResumePath 原词（MANUAL_REVIEW 等），缺席即决定已形成或任务已完成——详情分支
// 不按它过滤：操作员深链一份已续办的委托，如实呈现其当前状态好过答 404。
type reviewStatusBody struct {
	WaitingOn   string `json:"waitingOn,omitempty"`
	Completed   bool   `json:"completed"`
	Authority   string `json:"authority,omitempty"`
	Reviewer    string `json:"reviewer,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
}

type recordedJudgmentsBody struct {
	// 空数组而不是 null，判据同列表端点：调用方判「没有判断」不该先判「有没有字段」。
	Reachability []reviewReachabilityBody `json:"reachability"`
	// 财务控制零值表示尚未形成（ports.RecordedJudgments 的约定），整格缺席——
	// 不造一个「空结果」冒充判断过。采用解析同理。
	FinancialControl    *reviewFinancialControlBody `json:"financialControl,omitempty"`
	AdoptedResolutionID string                      `json:"adoptedResolutionId,omitempty"`
}

type reviewReachabilityBody struct {
	ParcelID string `json:"parcelId"`
	Value    string `json:"value"`
	// 判断标识与依据互补在场：`不适用`带依据说明这个问题为什么不该问，其余三值带
	// 判断标识（domain.ReachabilityJudgment 的构造约束），两者都照实透出不补造。
	JudgmentID    string `json:"judgmentId,omitempty"`
	Basis         string `json:"basis,omitempty"`
	AsOfAt        string `json:"asOfAt,omitempty"`
	AsOfSemantics string `json:"asOfSemantics,omitempty"`
	AsOfPolicy    string `json:"asOfPolicyVersion,omitempty"`
}

type reviewFinancialControlBody struct {
	Outcome       string `json:"outcome"`
	ResultID      string `json:"resultId,omitempty"`
	Basis         string `json:"basis,omitempty"`
	AsOfAt        string `json:"asOfAt,omitempty"`
	AsOfSemantics string `json:"asOfSemantics,omitempty"`
	AsOfPolicy    string `json:"asOfPolicyVersion,omitempty"`
}

func reviewQueueEntryBodyOf(record ports.AcceptanceReviewQueueRecord) reviewQueueEntryBody {
	entry := reviewQueueEntryBody{
		requestSummaryBody: summaryBodyOf(record.ShipmentRequestSummaryRecord),
		ReviewCompleted:    record.ReviewCompleted,
	}
	if record.HasAttempt {
		entry.LastAttemptReason = record.LastAttemptReason
		entry.LastAttemptContinuation = record.LastAttemptContinuation
		entry.LastAttemptedAt = record.LastAttemptedAt.UTC().Format(time.RFC3339Nano)
	}
	if record.ReviewCompleted {
		entry.ReviewAuthority = record.ReviewAuthority
		entry.ReviewReviewer = record.ReviewReviewer
		entry.ReviewEvidence = record.ReviewEvidence
		entry.ReviewCompletedAt = record.ReviewCompletedAt.UTC().Format(time.RFC3339Nano)
	}
	return entry
}

func reviewStatusBodyOf(task ports.AcceptanceTaskViewRecord) reviewStatusBody {
	body := reviewStatusBody{Completed: task.ReviewCompleted}
	if task.WaitingOn != domain.ResumePathInvalid {
		body.WaitingOn = task.WaitingOn.String()
	}
	if task.ReviewCompleted {
		body.Authority = task.ReviewAuthority
		body.Reviewer = task.ReviewReviewer
		body.Evidence = task.ReviewEvidence
		body.CompletedAt = task.ReviewCompletedAt.UTC().Format(time.RFC3339Nano)
	}
	return body
}

func recordedJudgmentsBodyOf(recorded ports.RecordedJudgments) recordedJudgmentsBody {
	body := recordedJudgmentsBody{
		Reachability: make([]reviewReachabilityBody, 0, len(recorded.Reachability)),
	}
	for _, judgment := range recorded.Reachability {
		row := reviewReachabilityBody{
			ParcelID:   judgment.DeclaredParcelID().String(),
			Value:      judgment.Value().String(),
			JudgmentID: judgment.JudgmentID().String(),
			Basis:      judgment.Basis().String(),
		}
		if asOf := judgment.AsOf(); !asOf.At().IsZero() {
			row.AsOfAt = asOf.At().UTC().Format(time.RFC3339Nano)
			row.AsOfSemantics = asOf.Semantics().String()
			row.AsOfPolicy = asOf.PolicyVersion().String()
		}
		body.Reachability = append(body.Reachability, row)
	}
	if recorded.FinancialControl.Outcome().String() != "" {
		control := &reviewFinancialControlBody{
			Outcome:  recorded.FinancialControl.Outcome().String(),
			ResultID: recorded.FinancialControl.ResultID().String(),
			Basis:    recorded.FinancialControl.Basis().String(),
		}
		if asOf := recorded.FinancialControl.AsOf(); !asOf.At().IsZero() {
			control.AsOfAt = asOf.At().UTC().Format(time.RFC3339Nano)
			control.AsOfSemantics = asOf.Semantics().String()
			control.AsOfPolicy = asOf.PolicyVersion().String()
		}
		body.FinancialControl = control
	}
	if recorded.AdoptedCommercialResolution.String() != "" {
		body.AdoptedResolutionID = recorded.AdoptedCommercialResolution.String()
	}
	return body
}
