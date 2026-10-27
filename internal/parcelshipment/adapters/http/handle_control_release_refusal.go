package shipmenthttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// ControlReleaseRefusalHandlingIntake 把一次已认证的接入请求翻译成释放拒收处理命令。它是接口而非解析代码，理由同本包其余运营决定口：
// 处理人与租户只能来自认证结果，采信自报就是让任何调用方替任何人销账。
type ControlReleaseRefusalHandlingIntake interface {
	IntakeControlReleaseRefusalHandling(
		ctx context.Context,
		request *http.Request,
	) (application.HandleControlReleaseRefusalCommand, error)
}

// ControlReleaseRefusalHandlingHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。真实装配接
// HandleControlReleaseRefusalHandler，外面包一笔事务：处理留痕、续办信封与结算侧的释放同生共死。
type ControlReleaseRefusalHandlingHandler interface {
	Handle(
		ctx context.Context,
		command application.HandleControlReleaseRefusalCommand,
	) (application.HandleControlReleaseRefusalResult, error)
}

var _ ControlReleaseRefusalHandlingIntake = UnconfiguredIntake{}

// IntakeControlReleaseRefusalHandling 不读请求：渠道未配置就无从铸造处理人。
func (UnconfiguredIntake) IntakeControlReleaseRefusalHandling(context.Context, *http.Request) (application.HandleControlReleaseRefusalCommand, error) {
	return application.HandleControlReleaseRefusalCommand{}, ErrAccessChannelNotConfigured
}

// NewHandleControlReleaseRefusalEndpoint 交回释放拒收处理的 HTTP 入口（POST /control-release-refusal-handlings；ADR-0293 决定二、三）。
func NewHandleControlReleaseRefusalEndpoint(
	intake ControlReleaseRefusalHandlingIntake,
	handler ControlReleaseRefusalHandlingHandler,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeControlReleaseRefusalHandling(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := handler.Handle(request.Context(), command)
		if err != nil {
			writeCommandProblem(response, err)
			return
		}
		writeControlReleaseRefusalHandlingOutcome(response, result)
	})
}

// controlReleaseRefusalHandlingResponse 是本端点的封闭响应形状。`已处理`与`已被处理`带做法与处理时刻（后者是先到那一份的）；`释放再被拒收`
// 带原因；回执里不带处理人：谁处理的只在留痕里（与别的运营决定口同一条纪律）。
type controlReleaseRefusalHandlingResponse struct {
	Outcome      string `json:"outcome"`
	HandlingKind string `json:"handlingKind,omitempty"`
	HandledAt    string `json:"handledAt,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

func writeControlReleaseRefusalHandlingOutcome(
	response http.ResponseWriter,
	result application.HandleControlReleaseRefusalResult,
) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := controlReleaseRefusalHandlingResponse{Outcome: outcome}
	if handling, present := result.Handling(); present {
		body.HandlingKind = handling.Kind.String()
		body.HandledAt = handling.HandledAt.UTC().Format(time.RFC3339Nano)
	}
	if reason := result.Reason().String(); reason != "" {
		body.Reason = reason
	}
	status := http.StatusOK
	if result.Outcome() == application.ControlReleaseRefusalHandled {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
