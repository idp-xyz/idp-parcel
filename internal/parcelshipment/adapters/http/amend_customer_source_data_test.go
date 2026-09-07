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

type amendmentIntakeDouble struct {
	err error
}

func (double *amendmentIntakeDouble) IntakeSourceDataAmendment(
	_ context.Context,
	_ *http.Request,
) (application.AmendCustomerSourceDataCommand, error) {
	if double.err != nil {
		return application.AmendCustomerSourceDataCommand{}, double.err
	}
	return application.AmendCustomerSourceDataCommand{}, nil
}

type amendmentHandlerDouble struct {
	result application.AmendCustomerSourceDataResult
	err    error
}

func (double *amendmentHandlerDouble) Handle(
	_ context.Context,
	_ application.AmendCustomerSourceDataCommand,
) (application.AmendCustomerSourceDataResult, error) {
	if double.err != nil {
		return application.AmendCustomerSourceDataResult{}, double.err
	}
	return double.result, nil
}

func postAmendment(
	t *testing.T,
	intake shipmenthttp.SourceDataAmendmentIntake,
	handler shipmenthttp.AmendmentHandler,
	method string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := shipmenthttp.NewAmendCustomerSourceDataEndpoint(intake, handler)
	request := httptest.NewRequest(method, "/shipment-requests/source-data-amendments", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// Covers: ADR-0022 与传输层分流纪律在资料修订端点上——未配置渠道 403（人来配才会好，不是重试）、
// 构造不出命令 4xx、受理依赖不可用 5xx、没形成答案 5xx 且只带稳定 code（统一不可见结果不拆出
// 「未找到」——FindBySourceIdentity 的否定结果不区分不存在与别的租户）、非 POST 405、未命名结果是
// 编程错误 5xx 不带 outcome 上线。
//
// 带业务结果的 AmendCustomerSourceDataResult 字段未导出是刻意的（传输层不该能捏造业务结果），
// 逐格映射由 cmd/parcel-api 的装配用例经真编排补。
func TestAmendmentTransportFailuresSplitByRetryAction(t *testing.T) {
	unconfigured := postAmendment(t, shipmenthttp.UnconfiguredIntake{}, &amendmentHandlerDouble{}, http.MethodPost)
	if unconfigured.Code != http.StatusForbidden {
		t.Fatalf("unconfigured status = %d, want 403", unconfigured.Code)
	}
	if code := problemCode(t, unconfigured); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("error code = %q", code)
	}

	malformed := postAmendment(t,
		&amendmentIntakeDouble{err: fmt.Errorf("%w: bad envelope", shipmenthttp.ErrMalformedRequest)},
		&amendmentHandlerDouble{},
		http.MethodPost,
	)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d", malformed.Code)
	}
	if code := problemCode(t, malformed); code != "MALFORMED_REQUEST" {
		t.Fatalf("error code = %q", code)
	}

	intakeDown := postAmendment(t,
		&amendmentIntakeDouble{err: errors.New("authenticator unreachable")},
		&amendmentHandlerDouble{},
		http.MethodPost,
	)
	if intakeDown.Code != http.StatusInternalServerError {
		t.Fatalf("intake failure status = %d; 依赖不可用误标 4xx 会让该重试的请求被丢掉", intakeDown.Code)
	}
	if code := problemCode(t, intakeDown); code != "INTAKE_FAILED" {
		t.Fatalf("error code = %q", code)
	}

	noAnswer := postAmendment(t,
		&amendmentIntakeDouble{},
		&amendmentHandlerDouble{err: errors.New("shipment request not found")},
		http.MethodPost,
	)
	if noAnswer.Code != http.StatusInternalServerError {
		t.Fatalf("no answer status = %d", noAnswer.Code)
	}
	if code := problemCode(t, noAnswer); code != "NO_ANSWER_FORMED" {
		t.Fatalf("error code = %q; 统一不可见结果不拆出未找到码", code)
	}

	unnamed := postAmendment(t,
		&amendmentIntakeDouble{},
		&amendmentHandlerDouble{result: application.AmendCustomerSourceDataResult{}},
		http.MethodPost,
	)
	if unnamed.Code != http.StatusInternalServerError {
		t.Fatalf("unnamed status = %d; 没有名字的结果不能带 outcome 上线", unnamed.Code)
	}
	if code := problemCode(t, unnamed); code != "UNNAMED_OUTCOME" {
		t.Fatalf("error code = %q", code)
	}
	assertNoOutcomeField(t, unnamed)

	wrongMethod := postAmendment(t, &amendmentIntakeDouble{}, &amendmentHandlerDouble{}, http.MethodGet)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d", wrongMethod.Code)
	}
	if allow := wrongMethod.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("allow = %q", allow)
	}
}
