package shipmenthttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// WithdrawalIntake 把一次已认证的接入请求翻译成一条撤回命令。它是接口而非解析代码，
// 理由与 SubmissionIntake 完全相同且缺一不可：来源信封只能来自认证结果（PAR-INT-01
// 待提供）、载荷摘要等规范化形状、撤回请求自己的来源身份与产生委托的那次提交必须
// 分开成两个信封——合用一个会让撤回被判成原提交的重放。三项未决期间本包不带任何
// 实现，包括「开发用」的采信头部版本。
type WithdrawalIntake interface {
	IntakeWithdrawal(ctx context.Context, request *http.Request) (application.WithdrawShipmentRequestCommand, error)
}

// WithdrawalHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type WithdrawalHandler interface {
	Handle(
		ctx context.Context,
		command application.WithdrawShipmentRequestCommand,
	) (application.WithdrawShipmentRequestResult, error)
}

// NewWithdrawShipmentRequestEndpoint 交回 `UC-PS-005` 决定前撤回编排的 HTTP 入口。
//
// 委托查不到时应用层上抛技术错误（指名一份查不到的委托是调用方的错），本端点把它与
// 其余没形成答案的失败一并回 5xx `NO_ANSWER_FORMED`——不细分，因为 `FindBySourceIdentity`
// 的否定结果不区分「不存在」与「属于别的租户」，细分出一个「未找到」码就把统一不可见
// 结果拆开了（UC-PS-001 语义，撤回照办）。
func NewWithdrawShipmentRequestEndpoint(intake WithdrawalIntake, handler WithdrawalHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeWithdrawal(request.Context(), request)
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
		writeWithdrawalOutcome(response, result)
	})
}

// withdrawalResponse 是本端点的封闭响应形状。`outcome` 取应用结果枚举的原名；撞上
// 既有决定时带决定视图（调用方要知道输给了接受还是拒绝）；撤回成立而随附释放未确定
// 完成时带补偿续办引用——补偿与判断续办分开，合成一个会让调用方分不清该重判还是该
// 续补偿。
type withdrawalResponse struct {
	Outcome               string `json:"outcome"`
	RequestState          string `json:"requestState,omitempty"`
	WithdrawalID          string `json:"withdrawalId,omitempty"`
	DecisionKind          string `json:"decisionKind,omitempty"`
	PendingReason         string `json:"pendingReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
	CompensationReference string `json:"compensationReference,omitempty"`
}

func writeWithdrawalOutcome(response http.ResponseWriter, result application.WithdrawShipmentRequestResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := withdrawalResponse{Outcome: outcome}
	if state := result.State().String(); state != "" {
		body.RequestState = state
	}
	if withdrawal, present := result.Withdrawal(); present {
		body.WithdrawalID = withdrawal.DecisionID().String()
	}
	if decision, present := result.AcceptanceDecision(); present {
		if decision.Accepted() {
			body.DecisionKind = "ACCEPTED"
		} else {
			body.DecisionKind = "REJECTED"
		}
	}
	if reason := result.PendingReason().String(); reason != "" {
		body.PendingReason = reason
	}
	if continuation := result.ContinuationReference().String(); continuation != "" {
		body.ContinuationReference = continuation
	}
	if compensation := result.CompensationReference().String(); compensation != "" {
		body.CompensationReference = compensation
	}

	status := http.StatusOK
	if result.Outcome() == application.WithdrawalFormed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
