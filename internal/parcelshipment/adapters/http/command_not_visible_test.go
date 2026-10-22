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

type manualReviewIntakeStub struct{}

func (manualReviewIntakeStub) IntakeManualReviewCompletion(context.Context, *http.Request) (application.CompleteManualReviewCommand, error) {
	return application.CompleteManualReviewCommand{}, nil
}

type manualReviewHandlerStub struct{ err error }

func (stub manualReviewHandlerStub) Handle(context.Context, application.CompleteManualReviewCommand) (application.CompleteManualReviewResult, error) {
	return application.CompleteManualReviewResult{}, stub.err
}

type activeRejectionIntakeStub struct{}

func (activeRejectionIntakeStub) IntakeActiveRejection(context.Context, *http.Request) (application.RejectShipmentRequestCommand, error) {
	return application.RejectShipmentRequestCommand{}, nil
}

type activeRejectionHandlerStub struct{ err error }

func (stub activeRejectionHandlerStub) Handle(context.Context, application.RejectShipmentRequestCommand) (application.RejectShipmentRequestResult, error) {
	return application.RejectShipmentRequestResult{}, stub.err
}

type authorizedDispositionIntakeStub struct{}

func (authorizedDispositionIntakeStub) IntakeAuthorizedDisposition(context.Context, *http.Request) (application.DisposeShipmentRequestCommand, error) {
	return application.DisposeShipmentRequestCommand{}, nil
}

type authorizedDispositionHandlerStub struct{ err error }

func (stub authorizedDispositionHandlerStub) Handle(context.Context, application.DisposeShipmentRequestCommand) (application.DisposeShipmentRequestResult, error) {
	return application.DisposeShipmentRequestResult{}, stub.err
}

var customerFaceCommands = map[string]bool{"撤回": true, "资料修订": true, "受控补充": true, "逐包裹取消": true}

// commandEndpointsFailingWith 交回指名一份委托的各命令口，编排一律交回 err。
func commandEndpointsFailingWith(err error) map[string]http.Handler {
	return map[string]http.Handler{
		"撤回":    shipmenthttp.NewWithdrawShipmentRequestEndpoint(&withdrawalIntakeDouble{}, &withdrawalHandlerDouble{err: err}),
		"资料修订":  shipmenthttp.NewAmendCustomerSourceDataEndpoint(&amendmentIntakeDouble{}, &amendmentHandlerDouble{err: err}),
		"受控补充":  shipmenthttp.NewFormNewSubmissionVersionEndpoint(&supplementIntakeDouble{}, &supplementHandlerDouble{err: err}),
		"逐包裹取消": shipmenthttp.NewCancelParcelEndpoint(&cancellationIntakeDouble{}, &cancellationHandlerDouble{err: err}),
		"复核完成":  shipmenthttp.NewCompleteManualReviewEndpoint(manualReviewIntakeStub{}, manualReviewHandlerStub{err: err}),
		"主动拒绝":  shipmenthttp.NewRejectShipmentRequestEndpoint(activeRejectionIntakeStub{}, activeRejectionHandlerStub{err: err}),
		"授权处置":  shipmenthttp.NewDisposeShipmentRequestEndpoint(authorizedDispositionIntakeStub{}, authorizedDispositionHandlerStub{err: err}),
	}
}

// Covers: ADR-0022 「`!found` 一律映射为对应上下文的`统一不可见结果`，用 `404`」与其 Consequences「接 HTTP 时由适配器
// 映射为 `404`，不得原样 500」——指名一份看不见的委托是调用方的错，5xx 会让离线客户端无限重投、堵住自己的队列。404 本身
// 就是「不存在」与「属于别人」的同答：码与委托查阅同一枚，不带 detail、不带 outcome。
func TestCommandsNamingAnInvisibleShipmentRequestAnswerNotFound(t *testing.T) {
	notVisible := fmt.Errorf("withdraw shipment request: %w", application.ErrShipmentRequestNotVisible)
	for name, endpoint := range commandEndpointsFailingWith(notVisible) {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))

			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404：查无的委托不是没形成答案", response.Code)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
				t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
			}
			if _, present := fields["outcome"]; present {
				t.Fatalf("404 带了 outcome：%s", response.Body.Bytes())
			}
			if _, present := fields["detail"]; present || problemCode(t, response) != "SHIPMENT_REQUEST_NOT_VISIBLE" {
				t.Fatalf("problem = %s, want 只带 SHIPMENT_REQUEST_NOT_VISIBLE", response.Body.Bytes())
			}
			// 客户面命令口（customerFaceCommands）写问题详情，操作者族各口照旧（ADR-0140 决定三；操作者族迁移另立后续票）。
			want := "application/json"
			if customerFaceCommands[name] {
				want = "application/problem+json"
			}
			if got := response.Header().Get("Content-Type"); got != want {
				t.Fatalf("Content-Type = %q, want %q", got, want)
			}
		})
	}
}

// Covers: ADR-0022 5xx 一格——查无之外没形成答案的失败照旧 5xx `NO_ANSWER_FORMED`：只有那一个具名错误换成 404，别的错误
// 里恰好带着「not found」字样也不算。
func TestOtherCommandFailuresStillAnswerNoAnswerFormed(t *testing.T) {
	for name, endpoint := range commandEndpointsFailingWith(errors.New("shipment request not found")) {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
			if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"NO_ANSWER_FORMED"`) {
				t.Fatalf("answer = %d %s, want 500 NO_ANSWER_FORMED", response.Code, response.Body.String())
			}
		})
	}
}

// Covers: ADR-0022 4xx 一格在资料修订口——受理阶段认出的调用方的错（意图未声明、内容与范围不配、委托未接受）重发不会改变
// 结果，不答 5xx：前两格改请求才会好，带只谈调用方自己载荷的 detail（ADR-0140 决定三）；未接受那一格要换一条路（受控补充），
// 不带 detail。
func TestAmendmentCallerErrorsAreNotNoAnswerFormed(t *testing.T) {
	cases := map[string]struct {
		err        error
		status     int
		code       string
		withDetail bool
	}{
		"意图未声明":   {err: application.ErrAmendmentIntentNotDeclared, status: http.StatusBadRequest, code: "MALFORMED_REQUEST", withDetail: true},
		"内容与范围不配": {err: application.ErrAmendmentContentUnfit, status: http.StatusBadRequest, code: "MALFORMED_REQUEST", withDetail: true},
		"委托未接受":   {err: application.ErrAmendedRequestNotAccepted, status: http.StatusBadRequest, code: "SHIPMENT_REQUEST_NOT_YET_ACCEPTED"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			endpoint := shipmenthttp.NewAmendCustomerSourceDataEndpoint(&amendmentIntakeDouble{},
				&amendmentHandlerDouble{err: fmt.Errorf("amend customer source data: %w", testCase.err)})
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))

			if response.Code != testCase.status {
				t.Fatalf("status = %d, want %d", response.Code, testCase.status)
			}
			if got := response.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Fatalf("Content-Type = %q, want application/problem+json", got)
			}
			var body struct {
				Outcome *string `json:"outcome"`
				Code    string  `json:"code"`
				Detail  string  `json:"detail"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
			}
			if body.Outcome != nil || body.Code != testCase.code || (body.Detail != "") != testCase.withDetail {
				t.Fatalf("problem = %s, want code %s detail %v", response.Body.Bytes(), testCase.code, testCase.withDetail)
			}
		})
	}
}
