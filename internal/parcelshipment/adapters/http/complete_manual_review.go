package shipmenthttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// ManualReviewCompletionIntake 把一次已认证的接入请求翻译成复核完成命令。它是接口而非
// 解析代码，理由与 WithdrawalIntake 相同：操作者认证与「渠道原始载荷 → 复核人/授权/证据
// 引用」的翻译属渠道接入契约（PAR-INT-01 待提供），采信自报的复核人等于让任何调用方替
// 任何角色签复核。未决期间本包不带任何实现，包括「开发用」的采信头部版本。
type ManualReviewCompletionIntake interface {
	IntakeManualReviewCompletion(
		ctx context.Context,
		request *http.Request,
	) (application.CompleteManualReviewCommand, error)
}

// ManualReviewCompletionHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，
// 只转交与映射。真实装配接 cmd/parcel-api 的复核完成边界壳（完成落库与「复核已完成」
// 信封同一事务，ADR-0086）。
type ManualReviewCompletionHandler interface {
	Handle(
		ctx context.Context,
		command application.CompleteManualReviewCommand,
	) (application.CompleteManualReviewResult, error)
}

// NewCompleteManualReviewEndpoint 交回复核完成的 HTTP 入口（票 09；ADR-0081 的命令面
// 保留条款）。
//
// 委托查不到时应用层上抛技术错误，本端点把它与其余没形成答案的失败一并回 5xx
// `NO_ANSWER_FORMED`——统一不可见结果不拆开，理由同撤回端点。
func NewCompleteManualReviewEndpoint(
	intake ManualReviewCompletionIntake,
	handler ManualReviewCompletionHandler,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeManualReviewCompletion(request.Context(), request)
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
		writeManualReviewCompletionOutcome(response, result)
	})
}

// manualReviewCompletionResponse 是本端点的封闭响应形状。`已有完成`带先到那份的留痕
// （操作员要知道签的是谁）；`任务已完结`带决定视图（复核无处可签，结果已定）；`版本
// 已换代`带当前版本（调用方据以重读队列）。
type manualReviewCompletionResponse struct {
	Outcome        string `json:"outcome"`
	RequestState   string `json:"requestState,omitempty"`
	Reviewer       string `json:"reviewer,omitempty"`
	Authority      string `json:"authority,omitempty"`
	Evidence       string `json:"evidence,omitempty"`
	CompletedAt    string `json:"completedAt,omitempty"`
	DecisionKind   string `json:"decisionKind,omitempty"`
	CurrentVersion string `json:"currentVersion,omitempty"`
}

func writeManualReviewCompletionOutcome(
	response http.ResponseWriter,
	result application.CompleteManualReviewResult,
) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := manualReviewCompletionResponse{Outcome: outcome}
	if state := result.State().String(); state != "" {
		body.RequestState = state
	}
	if completion, present := result.Completion(); present {
		body.Reviewer = completion.Reviewer().String()
		body.Authority = completion.Authority().String()
		body.Evidence = completion.Evidence().String()
		body.CompletedAt = completion.CompletedAt().UTC().Format(time.RFC3339)
	}
	if decision, present := result.AcceptanceDecision(); present {
		if decision.Accepted() {
			body.DecisionKind = "ACCEPTED"
		} else {
			body.DecisionKind = "REJECTED"
		}
	}
	if version := result.CurrentVersion().String(); version != "" {
		body.CurrentVersion = version
	}

	status := http.StatusOK
	if result.Outcome() == application.ManualReviewCompletionRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
