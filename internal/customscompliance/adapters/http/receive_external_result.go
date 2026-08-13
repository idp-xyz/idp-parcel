// Package customshttp 是 customs-compliance 的 HTTP 入站适配器，按 ADR-0022 把应用
// 结果映射成响应：状态码只回答服务端有没有形成答案，业务判别一律进响应体的 `outcome`。
//
// 包名与目录名不一致与 shipmenthttp 同理：目录按 ADR-0018 叫 `adapters/http`，包名
// 叫 `http` 会遮住标准库。
package customshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// ErrMalformedRequest 表示这次请求构造不出命令，且重发同样的内容不会改变结果。
// 4xx/5xx 的分法决定通道侧的动作——4xx 出队交给人，5xx 留在队里重发。
var ErrMalformedRequest = errors.New("customs compliance http: malformed request")

// ResultIntake 把一次已认证的通道请求翻译成一条外部结果接收命令。它是接口而非解析
// 代码：租户归属只能来自通道认证结果（真实监管回执通道的认证方式属 PAR-INT-01 待
// 提供，采信报文自称的租户号会穿透 ADR-0003 的隔离边界）；来源标识、层、原文语义与
// 发生时间照 ADR-0023 的同一条纪律从报文体收——服务端不代铸外部事实的身份与时间。
// 未决期间本包不带任何实现，包括「开发用」的采信头部版本。
type ResultIntake interface {
	IntakeResult(ctx context.Context, request *http.Request) (application.ReceiveExternalResultCommand, error)
}

// ResultHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type ResultHandler interface {
	Handle(
		ctx context.Context,
		command application.ReceiveExternalResultCommand,
	) (application.ReceiveExternalResultResult, error)
}

const (
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeMalformedRequest = "MALFORMED_REQUEST"
	codeIntakeFailed     = "INTAKE_FAILED"
	codeNoAnswerFormed   = "NO_ANSWER_FORMED"
	codeUnnamedOutcome   = "UNNAMED_OUTCOME"
)

// NewReceiveExternalResultEndpoint 交回 `UC-CC-006` 外部结果接收编排的 HTTP 入口。
func NewReceiveExternalResultEndpoint(intake ResultIntake, handler ResultHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeResult(request.Context(), request)
		if err != nil {
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
		writeResultOutcome(response, result)
	})
}

// resultResponse 是本端点的封闭响应形状。`outcome` 取应用结果枚举的原名；归属不上
// 与同层冲突都是已入册的答案（留存不猜/留存双方），通道方要知道的是记录成立与否和
// 续办引用，不是被翻译过的监管语义。
type resultResponse struct {
	Outcome               string `json:"outcome"`
	UndecidedReason       string `json:"undecidedReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
	HandoffReference      string `json:"handoffReference,omitempty"`
}

func writeResultOutcome(response http.ResponseWriter, result application.ReceiveExternalResultResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := resultResponse{Outcome: outcome}
	if reason := result.UndecidedReason().String(); reason != "" {
		body.UndecidedReason = reason
	}
	if continuation := result.ContinuationReference(); continuation != "" {
		body.ContinuationReference = continuation
	}
	if handoff := result.ResultHandoffReference(); handoff != "" {
		body.HandoffReference = handoff
	}

	status := http.StatusOK
	if result.Outcome() == application.ResultRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
