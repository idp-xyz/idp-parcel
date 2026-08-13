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

type withdrawalIntakeDouble struct {
	err error
}

func (double *withdrawalIntakeDouble) IntakeWithdrawal(
	_ context.Context,
	_ *http.Request,
) (application.WithdrawShipmentRequestCommand, error) {
	if double.err != nil {
		return application.WithdrawShipmentRequestCommand{}, double.err
	}
	return application.WithdrawShipmentRequestCommand{}, nil
}

type withdrawalHandlerDouble struct {
	result application.WithdrawShipmentRequestResult
	err    error
}

func (double *withdrawalHandlerDouble) Handle(
	_ context.Context,
	_ application.WithdrawShipmentRequestCommand,
) (application.WithdrawShipmentRequestResult, error) {
	if double.err != nil {
		return application.WithdrawShipmentRequestResult{}, double.err
	}
	return double.result, nil
}

type withdrawalErrorBody struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

func postWithdrawal(
	t *testing.T,
	intake shipmenthttp.WithdrawalIntake,
	handler shipmenthttp.WithdrawalHandler,
	method string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := shipmenthttp.NewWithdrawShipmentRequestEndpoint(intake, handler)
	request := httptest.NewRequest(method, "/shipment-requests/withdrawals", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// Covers: ADR-0022 与传输层分流纪律在 UC-PS-005 端点上——4xx 出队交给人（构造不出
// 命令且重发不会变）、受理依赖不可用是 5xx（误标 4xx 会让该重试的请求被丢掉）、没形成
// 答案是 5xx 且错误体只带稳定 code（统一不可见结果不拆出「未找到」码——FindBySourceIdentity
// 的否定结果不区分不存在与别的租户）、非 POST 405、未命名结果是编程错误 5xx 不带
// outcome 上线。
//
// `outcome` 的 201/200 映射与 submit 端点同构（writeOutcome 三行同款）：同构逻辑的
// 201 钉在 submit 端点自己的测试里，本端点自身的逐格钉由接线后端到端补——不在本文件
// 为构造带业务结果的 WithdrawShipmentRequestResult 重建全套聚合替身，因为结果字段
// 未导出是刻意的（传输层不该能捏造业务结果），这条设计约束比逐格断言更值钱。
func TestWithdrawalTransportFailuresSplitByRetryAction(t *testing.T) {
	malformed := postWithdrawal(t,
		&withdrawalIntakeDouble{err: fmt.Errorf("%w: bad envelope", shipmenthttp.ErrMalformedRequest)},
		&withdrawalHandlerDouble{},
		http.MethodPost,
	)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d", malformed.Code)
	}
	if body := decodeWithdrawalError(t, malformed); body.Error.Code != "MALFORMED_REQUEST" {
		t.Fatalf("error code = %q", body.Error.Code)
	}

	intakeDown := postWithdrawal(t,
		&withdrawalIntakeDouble{err: errors.New("authenticator unreachable")},
		&withdrawalHandlerDouble{},
		http.MethodPost,
	)
	if intakeDown.Code != http.StatusInternalServerError {
		t.Fatalf("intake failure status = %d; 依赖不可用误标 4xx 会让该重试的请求被丢掉", intakeDown.Code)
	}
	if body := decodeWithdrawalError(t, intakeDown); body.Error.Code != "INTAKE_FAILED" {
		t.Fatalf("error code = %q", body.Error.Code)
	}

	noAnswer := postWithdrawal(t,
		&withdrawalIntakeDouble{},
		&withdrawalHandlerDouble{err: errors.New("shipment request not found")},
		http.MethodPost,
	)
	if noAnswer.Code != http.StatusInternalServerError {
		t.Fatalf("no answer status = %d", noAnswer.Code)
	}
	if body := decodeWithdrawalError(t, noAnswer); body.Error.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("error code = %q; 统一不可见结果不拆出未找到码", body.Error.Code)
	}

	unnamed := postWithdrawal(t,
		&withdrawalIntakeDouble{},
		&withdrawalHandlerDouble{result: application.WithdrawShipmentRequestResult{}},
		http.MethodPost,
	)
	if unnamed.Code != http.StatusInternalServerError {
		t.Fatalf("unnamed status = %d; 没有名字的结果不能带 outcome 上线", unnamed.Code)
	}
	if body := decodeWithdrawalError(t, unnamed); body.Error.Code != "UNNAMED_OUTCOME" {
		t.Fatalf("error code = %q", body.Error.Code)
	}

	wrongMethod := postWithdrawal(t, &withdrawalIntakeDouble{}, &withdrawalHandlerDouble{}, http.MethodGet)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d", wrongMethod.Code)
	}
	if allow := wrongMethod.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("allow = %q", allow)
	}
}

func decodeWithdrawalError(t *testing.T, response *httptest.ResponseRecorder) withdrawalErrorBody {
	t.Helper()
	var body withdrawalErrorBody
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}
