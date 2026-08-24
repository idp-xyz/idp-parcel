package shipmenthttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// CancellationIntake 把一次已认证的接入请求翻译成一条取消命令。它是接口而非解析代码，
// 理由随 WithdrawalIntake：来源信封与请求方身份只能来自认证结果（接入契约属
// PAR-INT-03/07 待登记），业务发生时间由渠道契约声明。批量请求的逐包裹分发也归接入面
// ——UC-PS-006 步骤 1：批量只归组、不拥有共同状态，本端点一次受理一件包裹的取消请求，
// 部分成功由逐件请求自然表达。未决期间本包不带任何实现，包括「开发用」的采信头部版本。
type CancellationIntake interface {
	IntakeCancellation(ctx context.Context, request *http.Request) (application.CancelParcelCommand, error)
}

// CancellationHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type CancellationHandler interface {
	Handle(
		ctx context.Context,
		command application.CancelParcelCommand,
	) (application.CancelParcelResult, error)
}

// NewCancelParcelEndpoint 交回 `UC-PS-006` 接受后取消编排的 HTTP 入口。
//
// 与撤回端点的一处分流差别：委托查无、编号不符、成员出界与跨租户由取消编排折成 2xx 的
// REQUEST_NOT_ACCEPTED（统一不可见在编排内作答，AT-PS-090），不经由本端点的 5xx——
// 这里的 5xx 只剩真正没形成答案的失败。
func NewCancelParcelEndpoint(intake CancellationIntake, handler CancellationHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeCancellation(request.Context(), request)
		if err != nil {
			if errors.Is(err, ErrAccessChannelNotConfigured) {
				writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
				return
			}
			if errors.Is(err, ErrMalformedRequest) {
				writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
				return
			}
			writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
			return
		}

		result, err := handler.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeCancellationOutcome(response, result)
	})
}

// cancellationResponse 是本端点的封闭响应形状。`outcome` 取应用结果枚举的原名；三种
// 已提交走向各带自己的凭据——取消成立带取消决定标识、待处置带越过的收寄版本（调用方
// 要知道输给了哪次收寄）、拒绝带规则依据；未决带原因与续办引用；取消成立而发布意图
// 未交出时带重发引用（补偿与判断续办分开，理由随撤回端点）。
type cancellationResponse struct {
	Outcome               string `json:"outcome"`
	CancellationID        string `json:"cancellationId,omitempty"`
	IntakeVersion         string `json:"intakeVersion,omitempty"`
	RefusalBasis          string `json:"refusalBasis,omitempty"`
	DecidedAt             string `json:"decidedAt,omitempty"`
	PendingReason         string `json:"pendingReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
	HandoffReference      string `json:"handoffReference,omitempty"`
}

func writeCancellationOutcome(response http.ResponseWriter, result application.CancelParcelResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := cancellationResponse{Outcome: outcome}
	if record, present := result.Record(); present {
		body.DecidedAt = record.DecidedAt.UTC().Format(time.RFC3339Nano)
		switch record.Kind {
		case ports.RecordParcelCancelled:
			body.CancellationID = record.Cancellation.ID().String()
		case ports.RecordDispositionPending:
			body.IntakeVersion = record.IntakeVersion.String()
		case ports.RecordCancellationRefused:
			body.RefusalBasis = record.RefusalBasis.String()
		}
	}
	if reason := result.UndecidedReason().String(); reason != "" {
		body.PendingReason = reason
	}
	if continuation := result.ContinuationReference().String(); continuation != "" {
		body.ContinuationReference = continuation
	}
	if handoff := result.CancellationHandoffReference().String(); handoff != "" {
		body.HandoffReference = handoff
	}

	status := http.StatusOK
	if result.Outcome() == application.ParcelCancelled {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
