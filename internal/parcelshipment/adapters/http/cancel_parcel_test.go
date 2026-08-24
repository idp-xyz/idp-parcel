package shipmenthttp_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

type cancellationIntakeDouble struct {
	err error
}

func (double *cancellationIntakeDouble) IntakeCancellation(
	_ context.Context,
	_ *http.Request,
) (application.CancelParcelCommand, error) {
	if double.err != nil {
		return application.CancelParcelCommand{}, double.err
	}
	return application.CancelParcelCommand{}, nil
}

type cancellationHandlerDouble struct {
	result application.CancelParcelResult
	err    error
}

func (double *cancellationHandlerDouble) Handle(
	_ context.Context,
	_ application.CancelParcelCommand,
) (application.CancelParcelResult, error) {
	if double.err != nil {
		return application.CancelParcelResult{}, double.err
	}
	return double.result, nil
}

func postCancellation(
	t *testing.T,
	intake shipmenthttp.CancellationIntake,
	handler shipmenthttp.CancellationHandler,
	method string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := shipmenthttp.NewCancelParcelEndpoint(intake, handler)
	request := httptest.NewRequest(method, "/shipment-requests/parcel-cancellations", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// Covers: ADR-0022 与传输层分流纪律在 UC-PS-006 端点上——4xx 出队交给人（构造不出
// 命令且重发不会变）、受理依赖不可用是 5xx、编排上抛是 5xx 且错误体只带稳定 code、
// 非 POST 405、未命名结果是编程错误 5xx 不带 outcome 上线。
//
// 与撤回端点的一处分流差别记在这里：委托查无、编号不符、成员出界与跨租户在取消编排里
// 折成 2xx 的 REQUEST_NOT_ACCEPTED（统一不可见在编排内作答），不走 5xx——所以本端点的
// 5xx 只剩真正的依赖与编程失败。`outcome` 逐格钉由装配测试对真库补，理由随撤回端点：
// 结果字段未导出是刻意的，传输层不该能捏造业务结果。
func TestCancellationTransportFailuresSplitByRetryAction(t *testing.T) {
	malformed := postCancellation(t,
		&cancellationIntakeDouble{err: fmt.Errorf("%w: bad envelope", shipmenthttp.ErrMalformedRequest)},
		&cancellationHandlerDouble{},
		http.MethodPost,
	)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d", malformed.Code)
	}
	if body := decodeWithdrawalError(t, malformed); body.Error.Code != "MALFORMED_REQUEST" {
		t.Fatalf("error code = %q", body.Error.Code)
	}

	intakeDown := postCancellation(t,
		&cancellationIntakeDouble{err: errors.New("authenticator unreachable")},
		&cancellationHandlerDouble{},
		http.MethodPost,
	)
	if intakeDown.Code != http.StatusInternalServerError {
		t.Fatalf("intake failure status = %d; 依赖不可用误标 4xx 会让该重试的请求被丢掉", intakeDown.Code)
	}
	if body := decodeWithdrawalError(t, intakeDown); body.Error.Code != "INTAKE_FAILED" {
		t.Fatalf("error code = %q", body.Error.Code)
	}

	noAnswer := postCancellation(t,
		&cancellationIntakeDouble{},
		&cancellationHandlerDouble{err: errors.New("decide parcel cancellation: broken")},
		http.MethodPost,
	)
	if noAnswer.Code != http.StatusInternalServerError {
		t.Fatalf("no answer status = %d", noAnswer.Code)
	}
	if body := decodeWithdrawalError(t, noAnswer); body.Error.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("error code = %q", body.Error.Code)
	}

	unnamed := postCancellation(t,
		&cancellationIntakeDouble{},
		&cancellationHandlerDouble{result: application.CancelParcelResult{}},
		http.MethodPost,
	)
	if unnamed.Code != http.StatusInternalServerError {
		t.Fatalf("unnamed status = %d; 没有名字的结果不能带 outcome 上线", unnamed.Code)
	}
	if body := decodeWithdrawalError(t, unnamed); body.Error.Code != "UNNAMED_OUTCOME" {
		t.Fatalf("error code = %q", body.Error.Code)
	}

	wrongMethod := postCancellation(t, &cancellationIntakeDouble{}, &cancellationHandlerDouble{}, http.MethodGet)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d", wrongMethod.Code)
	}
	if allow := wrongMethod.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("allow = %q", allow)
	}
}
