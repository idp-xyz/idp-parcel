package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// unconfiguredEndpoints 遍历本包装着 UnconfiguredIntake 的全部端点。编排一律是「被调
// 即失败」的替身：未配置 Intake 的合同就是不构造命令，编排若被触到，说明有请求穿过了
// 未配置格——装配点因此才允许在真渠道就位前不装配任何应用编排。
func unconfiguredEndpoints(t *testing.T) map[string]http.Handler {
	t.Helper()
	return map[string]http.Handler{
		"submit": shipmenthttp.NewSubmitShipmentRequestEndpoint(
			shipmenthttp.UnconfiguredIntake{}, unreachableSubmissionHandler{t: t},
		),
		"withdraw": shipmenthttp.NewWithdrawShipmentRequestEndpoint(
			shipmenthttp.UnconfiguredIntake{}, unreachableWithdrawalHandler{t: t},
		),
	}
}

// Covers: ADR-0055 「未配置即拒、不读内容」 — 未配置 Intake 对每个请求答 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED，不读业务内容、不构造命令；按 ADR-0022，4xx 不带
// `outcome`。
func TestUnconfiguredIntakeRefusesWithoutReadingTheBody(t *testing.T) {
	for name, endpoint := range unconfiguredEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			probe := &readProbe{}
			request := httptest.NewRequest(http.MethodPost, "/", probe)
			response := httptest.NewRecorder()

			endpoint.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
			if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", got)
			}
			assertNoOutcomeField(t, response)
			if probe.read {
				t.Fatal("an unconfigured intake read the business content")
			}
		})
	}
}

// Covers: ADR-0055 「答复对一切请求内容与自报身份一致，不因内容不同而变」 — 采信自报
// 身份的第一个征兆就是答复随它变化；这里钉住不同载荷、自报租户头与查询串得到逐字节
// 相同的答复。
func TestUnconfiguredIntakeAnswersEveryRequestIdentically(t *testing.T) {
	variants := map[string]func() *http.Request{
		"empty body": func() *http.Request {
			return httptest.NewRequest(http.MethodPost, "/", nil)
		},
		"json body": func() *http.Request {
			return httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"tenantId":"TENANT-9"}`))
		},
		"garbage body": func() *http.Request {
			return httptest.NewRequest(http.MethodPost, "/", strings.NewReader("!!not-json!!"))
		},
		"self-reported identity headers": func() *http.Request {
			request := httptest.NewRequest(http.MethodPost, "/", nil)
			request.Header.Set("X-Reported-Tenant", "TENANT-9")
			request.Header.Set("X-Reported-Customer-Account", "CUST-9")
			return request
		},
		"query string": func() *http.Request {
			return httptest.NewRequest(http.MethodPost, "/?tenant=TENANT-9", nil)
		},
	}

	for name, endpoint := range unconfiguredEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			baseline := httptest.NewRecorder()
			endpoint.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/", nil))

			for variantName, newRequest := range variants {
				response := httptest.NewRecorder()
				endpoint.ServeHTTP(response, newRequest())
				if response.Code != baseline.Code || response.Body.String() != baseline.Body.String() {
					t.Fatalf("%s: answer differs from baseline: %d %s vs %d %s",
						variantName, response.Code, response.Body.String(), baseline.Code, baseline.Body.String())
				}
			}
		})
	}
}

// readProbe 记录 body 有没有被读过。用旗标而不是当场失败，是为了把「读了」报成一次
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

type unreachableSubmissionHandler struct{ t *testing.T }

func (handler unreachableSubmissionHandler) Handle(
	context.Context,
	application.SubmitShipmentRequestCommand,
) (application.SubmitShipmentRequestResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.SubmitShipmentRequestResult{}, nil
}

type unreachableWithdrawalHandler struct{ t *testing.T }

func (handler unreachableWithdrawalHandler) Handle(
	context.Context,
	application.WithdrawShipmentRequestCommand,
) (application.WithdrawShipmentRequestResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.WithdrawShipmentRequestResult{}, nil
}
