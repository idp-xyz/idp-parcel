package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// 本文件钉关闭 / 重开两个端点的传输层分流（票 label-channel/30 做法 4；ADR-0022 纪律，与撤回端点同款）。
// 带业务结果的响应体逐格钉由 cmd/parcel-api 真库装配用例做——结果字段未导出是刻意的（传输层不该能捏造业务结果），
// 这里只用零值结果证「未命名结果是编程错误」。

type continuedAttemptIntakeDouble struct {
	err error
}

func (double *continuedAttemptIntakeDouble) IntakeControlledClosure(context.Context, *http.Request) (application.FormControlledClosureCommand, error) {
	return application.FormControlledClosureCommand{}, double.err
}

func (double *continuedAttemptIntakeDouble) IntakeReopening(context.Context, *http.Request) (application.FormReopeningCommand, error) {
	return application.FormReopeningCommand{}, double.err
}

type continuedAttemptHandlerDouble struct {
	result   application.ContinuedAttemptDecisionResult
	err      error
	closures int
	reopens  int
}

func (double *continuedAttemptHandlerDouble) FormControlledClosure(context.Context, application.FormControlledClosureCommand) (application.ContinuedAttemptDecisionResult, error) {
	double.closures++
	return double.result, double.err
}

func (double *continuedAttemptHandlerDouble) FormReopening(context.Context, application.FormReopeningCommand) (application.ContinuedAttemptDecisionResult, error) {
	double.reopens++
	return double.result, double.err
}

type continuedAttemptErrorBody struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

func postContinuedAttemptDecision(
	t *testing.T,
	endpoint http.Handler,
	method string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "/parcels/continued-attempt-decisions", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

func decodeContinuedAttemptError(t *testing.T, response *httptest.ResponseRecorder) continuedAttemptErrorBody {
	t.Helper()
	var body continuedAttemptErrorBody
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

// Covers: 两个端点各自分流——接入未配置 403（人不来配就永远不会好，不留队重发）、构造不出命令 400、接入依赖不可用 5xx、
// 编排没形成答案 5xx 且不拆「未找到」码、未命名结果 5xx、非 POST 405；关闭端点只调关闭、重开端点只调重开。
func TestContinuedAttemptDecisionEndpointsSplitTransportFailuresByRetryAction(t *testing.T) {
	endpoints := map[string]func(shipmenthttp.ContinuedAttemptDecisionIntake, shipmenthttp.ContinuedAttemptDecisionHandler) http.Handler{
		"controlled closure": shipmenthttp.NewFormControlledClosureEndpoint,
		"reopening":          shipmenthttp.NewFormReopeningEndpoint,
	}
	for name, build := range endpoints {
		t.Run(name, func(t *testing.T) {
			unconfigured := postContinuedAttemptDecision(t, build(shipmenthttp.UnconfiguredIntake{}, &continuedAttemptHandlerDouble{}), http.MethodPost)
			if unconfigured.Code != http.StatusForbidden {
				t.Fatalf("unconfigured status = %d", unconfigured.Code)
			}
			if body := decodeContinuedAttemptError(t, unconfigured); body.Error.Code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("error code = %q", body.Error.Code)
			}

			malformed := postContinuedAttemptDecision(t, build(
				&continuedAttemptIntakeDouble{err: fmt.Errorf("%w: bad envelope", shipmenthttp.ErrMalformedRequest)},
				&continuedAttemptHandlerDouble{},
			), http.MethodPost)
			if malformed.Code != http.StatusBadRequest {
				t.Fatalf("malformed status = %d", malformed.Code)
			}
			if body := decodeContinuedAttemptError(t, malformed); body.Error.Code != "MALFORMED_REQUEST" {
				t.Fatalf("error code = %q", body.Error.Code)
			}

			intakeDown := postContinuedAttemptDecision(t, build(
				&continuedAttemptIntakeDouble{err: errors.New("authenticator unreachable")},
				&continuedAttemptHandlerDouble{},
			), http.MethodPost)
			if intakeDown.Code != http.StatusInternalServerError {
				t.Fatalf("intake failure status = %d; 依赖不可用误标 4xx 会让该重试的请求被丢掉", intakeDown.Code)
			}
			if body := decodeContinuedAttemptError(t, intakeDown); body.Error.Code != "INTAKE_FAILED" {
				t.Fatalf("error code = %q", body.Error.Code)
			}

			noAnswer := postContinuedAttemptDecision(t, build(
				&continuedAttemptIntakeDouble{},
				&continuedAttemptHandlerDouble{err: errors.New("outbox unavailable")},
			), http.MethodPost)
			if noAnswer.Code != http.StatusInternalServerError {
				t.Fatalf("no answer status = %d", noAnswer.Code)
			}
			if body := decodeContinuedAttemptError(t, noAnswer); body.Error.Code != "NO_ANSWER_FORMED" {
				t.Fatalf("error code = %q", body.Error.Code)
			}

			unnamed := postContinuedAttemptDecision(t, build(&continuedAttemptIntakeDouble{}, &continuedAttemptHandlerDouble{}), http.MethodPost)
			if unnamed.Code != http.StatusInternalServerError {
				t.Fatalf("unnamed status = %d; 没有名字的结果不能带 outcome 上线", unnamed.Code)
			}
			if body := decodeContinuedAttemptError(t, unnamed); body.Error.Code != "UNNAMED_OUTCOME" {
				t.Fatalf("error code = %q", body.Error.Code)
			}

			wrongMethod := postContinuedAttemptDecision(t, build(&continuedAttemptIntakeDouble{}, &continuedAttemptHandlerDouble{}), http.MethodGet)
			if wrongMethod.Code != http.StatusMethodNotAllowed {
				t.Fatalf("method status = %d", wrongMethod.Code)
			}
			if allow := wrongMethod.Header().Get("Allow"); allow != http.MethodPost {
				t.Fatalf("allow = %q", allow)
			}
		})
	}
}

// Covers: 关闭端点只调关闭、重开端点只调重开——一个 handler 两条命令，端点不得串线。
func TestEachContinuedAttemptDecisionEndpointCallsItsOwnCommand(t *testing.T) {
	handler := &continuedAttemptHandlerDouble{}
	postContinuedAttemptDecision(t, shipmenthttp.NewFormControlledClosureEndpoint(&continuedAttemptIntakeDouble{}, handler), http.MethodPost)
	if handler.closures != 1 || handler.reopens != 0 {
		t.Fatalf("closures / reopens = %d / %d after the closure endpoint", handler.closures, handler.reopens)
	}
	postContinuedAttemptDecision(t, shipmenthttp.NewFormReopeningEndpoint(&continuedAttemptIntakeDouble{}, handler), http.MethodPost)
	if handler.closures != 1 || handler.reopens != 1 {
		t.Fatalf("closures / reopens = %d / %d after the reopening endpoint", handler.closures, handler.reopens)
	}
}
