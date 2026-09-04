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

type supplementIntakeDouble struct {
	err error
}

func (double *supplementIntakeDouble) IntakeSupplement(
	_ context.Context,
	_ *http.Request,
) (application.FormNewSubmissionVersionCommand, error) {
	if double.err != nil {
		return application.FormNewSubmissionVersionCommand{}, double.err
	}
	return application.FormNewSubmissionVersionCommand{}, nil
}

type supplementHandlerDouble struct {
	result application.FormNewSubmissionVersionResult
	err    error
}

func (double *supplementHandlerDouble) Handle(
	_ context.Context,
	_ application.FormNewSubmissionVersionCommand,
) (application.FormNewSubmissionVersionResult, error) {
	if double.err != nil {
		return application.FormNewSubmissionVersionResult{}, double.err
	}
	return double.result, nil
}

func postSupplement(
	t *testing.T,
	intake shipmenthttp.SupplementIntake,
	handler shipmenthttp.SupplementHandler,
	method string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := shipmenthttp.NewFormNewSubmissionVersionEndpoint(intake, handler)
	request := httptest.NewRequest(method, "/shipment-requests/supplements", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// Covers: ADR-0022 与传输层分流纪律在受控补充端点上——4xx 出队交给人（构造不出命令且重发不会
// 变）、受理依赖不可用是 5xx（误标 4xx 会让该重试的请求被丢掉）、没形成答案是 5xx 且错误体只带
// 稳定 code（统一不可见结果不拆出「未找到」码——FindBySourceIdentity 的否定结果不区分不存在与
// 别的租户）、非 POST 405、未命名结果是编程错误 5xx 不带 outcome 上线。
//
// `outcome` 的 201/200 映射与 submit / withdraw 端点同构（三行同款）：同构逻辑的 201 钉在 submit
// 端点自己的测试里；带业务结果的 FormNewSubmissionVersionResult 字段未导出是刻意的（传输层不该
// 能捏造业务结果），本端点的逐格由 cmd/parcel-api 的装配用例经真编排补。
func TestSupplementTransportFailuresSplitByRetryAction(t *testing.T) {
	malformed := postSupplement(t,
		&supplementIntakeDouble{err: fmt.Errorf("%w: bad envelope", shipmenthttp.ErrMalformedRequest)},
		&supplementHandlerDouble{},
		http.MethodPost,
	)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d", malformed.Code)
	}
	if code := problemCode(t, malformed); code != "MALFORMED_REQUEST" {
		t.Fatalf("error code = %q", code)
	}

	intakeDown := postSupplement(t,
		&supplementIntakeDouble{err: errors.New("authenticator unreachable")},
		&supplementHandlerDouble{},
		http.MethodPost,
	)
	if intakeDown.Code != http.StatusInternalServerError {
		t.Fatalf("intake failure status = %d; 依赖不可用误标 4xx 会让该重试的请求被丢掉", intakeDown.Code)
	}
	if code := problemCode(t, intakeDown); code != "INTAKE_FAILED" {
		t.Fatalf("error code = %q", code)
	}

	noAnswer := postSupplement(t,
		&supplementIntakeDouble{},
		&supplementHandlerDouble{err: errors.New("shipment request not found")},
		http.MethodPost,
	)
	if noAnswer.Code != http.StatusInternalServerError {
		t.Fatalf("no answer status = %d", noAnswer.Code)
	}
	if code := problemCode(t, noAnswer); code != "NO_ANSWER_FORMED" {
		t.Fatalf("error code = %q; 统一不可见结果不拆出未找到码", code)
	}

	unnamed := postSupplement(t,
		&supplementIntakeDouble{},
		&supplementHandlerDouble{result: application.FormNewSubmissionVersionResult{}},
		http.MethodPost,
	)
	if unnamed.Code != http.StatusInternalServerError {
		t.Fatalf("unnamed status = %d; 没有名字的结果不能带 outcome 上线", unnamed.Code)
	}
	if code := problemCode(t, unnamed); code != "UNNAMED_OUTCOME" {
		t.Fatalf("error code = %q", code)
	}
	assertNoOutcomeField(t, unnamed)

	wrongMethod := postSupplement(t, &supplementIntakeDouble{}, &supplementHandlerDouble{}, http.MethodGet)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d", wrongMethod.Code)
	}
	if allow := wrongMethod.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("allow = %q", allow)
	}
}
