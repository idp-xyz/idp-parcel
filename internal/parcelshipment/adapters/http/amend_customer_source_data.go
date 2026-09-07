package shipmenthttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// SourceDataAmendmentIntake 把一次已认证的接入请求翻译成一条资料修订命令（UC-PS-002 的编排半边，
// 票 ps-port-remainder/04 把入口接进生产）。它是接口而非解析代码，理由与 SubmissionIntake 完全相同
// 且多两条：修订请求自己的来源身份与产生委托的那次提交必须分开成两个信封（合用会被判成原提交的
// 重放）；请求方与实际决定方的采信整组属 `BD-PS-009` / `PAR-INT-01`（实例半边）——从请求体里读一个
// 「我是谁」正是 UC-PS-002「登录操作人不能替代实际决定方」那句禁止的事。未决期间本包不带任何实现，
// 包括「开发用」的采信头部版本。
type SourceDataAmendmentIntake interface {
	IntakeSourceDataAmendment(ctx context.Context, request *http.Request) (application.AmendCustomerSourceDataCommand, error)
}

// AmendmentHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。真实装配
// 接 cmd/parcel-api 的资料修订边界壳（来源保全一层、版本落库一层，交接意图由编排自己在其后交出）。
type AmendmentHandler interface {
	Handle(
		ctx context.Context,
		command application.AmendCustomerSourceDataCommand,
	) (application.AmendCustomerSourceDataResult, error)
}

// NewAmendCustomerSourceDataEndpoint 交回资料修订的 HTTP 入口：客户或其授权代表在`已接受`委托上形成
// 一份客户原始资料新版本（`AT-PS-014`..`AT-PS-032`）。
//
// 委托查不到时应用层上抛技术错误，本端点把它与其余没形成答案的失败一并回 5xx `NO_ANSWER_FORMED`
// ——统一不可见结果不拆开，理由同撤回端点。委托未接受、修订意图未声明同样以错误上抛，同一格：
// 那是调用方对世界的判断错了，不是一个可续办的业务答案。
func NewAmendCustomerSourceDataEndpoint(intake SourceDataAmendmentIntake, handler AmendmentHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeSourceDataAmendment(request.Context(), request)
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
		writeAmendmentOutcome(response, result)
	})
}

// amendmentResponse 是本端点的封闭响应形状。`outcome` 取应用结果枚举的原名；版本已形成（或重放读回
// 原版本）时带版本号与该范围此刻的采用判断——下游此刻该消费哪一份与刚形成的那一份在分叉时并不是
// 同一个（UC-PS-002 步骤 8），两样都交给客户；未决时带原因与续办引用。`已记录并采用`只表示本上下文
// 的资料版本判断完成，不表示下游已经采用。
type amendmentResponse struct {
	Outcome               string `json:"outcome"`
	SourceDataVersionID   string `json:"sourceDataVersionId,omitempty"`
	AdoptionOutcome       string `json:"adoptionOutcome,omitempty"`
	AdoptedVersionID      string `json:"adoptedVersionId,omitempty"`
	PendingReason         string `json:"pendingReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
}

func writeAmendmentOutcome(response http.ResponseWriter, result application.AmendCustomerSourceDataResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := amendmentResponse{Outcome: outcome}
	if version, present := result.Version(); present {
		body.SourceDataVersionID = version.VersionID().String()
	}
	if adoption, present := result.Adoption(); present {
		body.AdoptionOutcome = adoption.Outcome().String()
		if adopted, adoptedPresent := adoption.AdoptedVersion(); adoptedPresent {
			body.AdoptedVersionID = adopted.String()
		}
	}
	if reason := result.PendingReason().String(); reason != "" {
		body.PendingReason = reason
	}
	if continuation := result.ContinuationReference().String(); continuation != "" {
		body.ContinuationReference = continuation
	}

	status := http.StatusOK
	if result.Outcome() == application.AmendmentRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
