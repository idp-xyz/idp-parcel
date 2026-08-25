package customshttp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// unconfiguredResultEndpoint 装出外部结果接收端点的未配置形态。编排一律是「被调即
// 失败」的替身：未配置 Intake 的合同就是不构造命令，编排若被触到，说明有请求穿过了
// 未配置格——装配点因此才允许在真通道就位前不装配任何应用编排。规则库查阅端点的
// 未配置形态由其自己的测试文件覆盖。
func unconfiguredResultEndpoint(t *testing.T) http.Handler {
	t.Helper()
	return customshttp.NewReceiveExternalResultEndpoint(
		customshttp.UnconfiguredIntake{}, unreachableResultHandler{t: t},
	)
}

// Covers: ADR-0055 「未配置即拒、不读内容」 — 未配置 Intake 对每份报文答 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED，不读报文内容、不构造命令；按 ADR-0022，4xx 不带
// `outcome`。本面等的是 `PAR-INT-03`（外部结果通道），但错误码只命名状态不命名参数。
func TestUnconfiguredResultIntakeRefusesWithoutReadingTheBody(t *testing.T) {
	probe := &readProbe{}
	request := httptest.NewRequest(http.MethodPost, "/customs/external-results", probe)
	response := httptest.NewRecorder()

	unconfiguredResultEndpoint(t).ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", got)
	}
	assertNoOutcome(t, response)
	if probe.read {
		t.Fatal("an unconfigured intake read the external report")
	}
}

// Covers: ADR-0055 「答复对一切请求内容与自报身份一致，不因内容不同而变」 — 报文自称
// 的租户号是这一面最危险的自报身份（ADR-0003 隔离边界）；答复随它变化就是采信的第一个
// 征兆。
func TestUnconfiguredResultIntakeAnswersEveryRequestIdentically(t *testing.T) {
	endpoint := unconfiguredResultEndpoint(t)
	baseline := httptest.NewRecorder()
	endpoint.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/customs/external-results", nil))

	variants := map[string]*http.Request{
		"empty body":     httptest.NewRequest(http.MethodPost, "/customs/external-results", nil),
		"report body":    httptest.NewRequest(http.MethodPost, "/customs/external-results", strings.NewReader(`{"tenantId":"TENANT-9"}`)),
		"garbage body":   httptest.NewRequest(http.MethodPost, "/customs/external-results", strings.NewReader("!!not-json!!")),
		"query string":   httptest.NewRequest(http.MethodPost, "/customs/external-results?tenant=TENANT-9", nil),
		"another tenant": httptest.NewRequest(http.MethodPost, "/customs/external-results", strings.NewReader(`{"tenantId":"TENANT-8"}`)),
	}
	reported := httptest.NewRequest(http.MethodPost, "/customs/external-results", nil)
	reported.Header.Set("X-Reported-Tenant", "TENANT-9")
	variants["self-reported identity headers"] = reported

	for name, request := range variants {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		if response.Code != baseline.Code || response.Body.String() != baseline.Body.String() {
			t.Fatalf("%s: answer differs from baseline: %d %s vs %d %s",
				name, response.Code, response.Body.String(), baseline.Code, baseline.Body.String())
		}
	}
}

// readProbe 记录报文有没有被读过。用旗标而不是当场失败，是为了把「读了」报成一次
// 明确断言而不是一次来路不明的中断。
type readProbe struct{ read bool }

func (probe *readProbe) Read([]byte) (int, error) {
	probe.read = true
	return 0, io.EOF
}

func problemCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem response %s: %v", response.Body.Bytes(), err)
	}
	return body.Error.Code
}

func assertNoOutcome(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
	if _, present := fields["outcome"]; present {
		t.Fatalf("a response that formed no answer carried an outcome: %s", response.Body.Bytes())
	}
}

type unreachableResultHandler struct{ t *testing.T }

func (handler unreachableResultHandler) Handle(
	context.Context,
	application.ReceiveExternalResultCommand,
) (application.ReceiveExternalResultResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.ReceiveExternalResultResult{}, nil
}
