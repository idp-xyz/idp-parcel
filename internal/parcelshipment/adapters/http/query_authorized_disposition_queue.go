package shipmenthttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 授权处置队列查阅端点（票 sa-preacceptance-policy-view/04；ADR-0132 决定二）。列的是「当前停在等待授权
// 处置」的委托：等待态由 Decide 看过全部校验后写在任务文档上，读面照登记过滤，不在读侧重推域判断。逐行
// 透出受限项、正文登记的失败处置与责任引用、受限原因——处置角色选去向要看的东西；处置命令面另在
// /shipment-requests/authorized-dispositions，查阅面不收决定。
//
// 只有列表，没有单份分支：单份详情复用复核队列的 `?shipmentRequestId=` 分支——那里透出的判断三组正是
// 处置角色要审的（已记录的控制结果逐项带失败处置与责任引用），再开一份详情就是第二份同样的东西。
//
// Intake 复用 ShipmentRequestViewsIntake，理由同复核队列：它是委托查阅面的子集视图，作用域语义与页大小
// 裁决同一套。

// AuthorizedDispositionQueueReader 是本端点消费的队列读口。
type AuthorizedDispositionQueueReader interface {
	ListAwaitingAuthorizedDisposition(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		limit int,
	) ([]ports.AuthorizedDispositionQueueRecord, error)
}

// 编译期锁缝：读口形状与端口保持一致——本适配器不新造查询语义。
var _ AuthorizedDispositionQueueReader = ports.AuthorizedDispositionQueue(nil)

// 业务结果：列表空结果仍是 DISPOSITION_QUEUE_LISTED（空队列是答案不是错误）。
const outcomeDispositionQueueListed = "DISPOSITION_QUEUE_LISTED"

// NewQueryAuthorizedDispositionQueueEndpoint 交回授权处置队列的 HTTP 入口
// （GET /authorized-disposition-queue）。方法门与错误分流照 /acceptance-review-queue 的形。
func NewQueryAuthorizedDispositionQueueEndpoint(
	intake ShipmentRequestViewsIntake,
	reader AuthorizedDispositionQueueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeListQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		records, err := reader.ListAwaitingAuthorizedDisposition(request.Context(), query.Scope, query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		entries := make([]dispositionQueueEntryBody, 0, len(records))
		for _, record := range records {
			entries = append(entries, dispositionQueueEntryBodyOf(record))
		}
		writeJSON(response, http.StatusOK, dispositionQueueListResponse{
			Outcome: outcomeDispositionQueueListed,
			Entries: entries,
		})
	})
}

type dispositionQueueListResponse struct {
	Outcome string                      `json:"outcome"`
	Entries []dispositionQueueEntryBody `json:"entries"`
}

type dispositionQueueEntryBody struct {
	requestSummaryBody
	LastAttemptReason       string `json:"lastAttemptReason,omitempty"`
	LastAttemptContinuation string `json:"lastAttemptContinuation,omitempty"`
	LastAttemptedAt         string `json:"lastAttemptedAt,omitempty"`
	ControlResultID         string `json:"controlResultId,omitempty"`
	// restrictedItems 是空数组而不是 null（判据同其余列表端点）：一份停在等处置的委托不该没有受限项，
	// 读到空数组是坏数据的可观察征兆，不该被 null 盖住。
	RestrictedItems []restrictedControlItemBody `json:"restrictedItems"`
}

type restrictedControlItemBody struct {
	Kind               string `json:"kind"`
	Order              uint32 `json:"order"`
	Basis              string `json:"basis,omitempty"`
	FailureDisposition string `json:"failureDisposition,omitempty"`
	Responsibility     string `json:"responsibility,omitempty"`
}

func dispositionQueueEntryBodyOf(record ports.AuthorizedDispositionQueueRecord) dispositionQueueEntryBody {
	entry := dispositionQueueEntryBody{
		requestSummaryBody: summaryBodyOf(record.ShipmentRequestSummaryRecord),
		ControlResultID:    record.ControlResultID.String(),
		RestrictedItems:    make([]restrictedControlItemBody, 0, len(record.RestrictedItems)),
	}
	if record.HasAttempt {
		entry.LastAttemptReason = record.LastAttemptReason
		entry.LastAttemptContinuation = record.LastAttemptContinuation
		entry.LastAttemptedAt = record.LastAttemptedAt.UTC().Format(time.RFC3339Nano)
	}
	for _, item := range record.RestrictedItems {
		entry.RestrictedItems = append(entry.RestrictedItems, restrictedControlItemBody{
			Kind:               item.Kind.String(),
			Order:              item.Order,
			Basis:              item.Basis.String(),
			FailureDisposition: item.FailureDisposition.String(),
			Responsibility:     item.Responsibility.String(),
		})
	}
	return entry
}
