package tfhttp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// unconfiguredDeliveryEndpoints 遍历本包装着 UnconfiguredIntake 的两个端点。首登与
// 更正各走 DeliveryIntake 的一个方法，未配置格必须两边都堵——只堵一边就是给另一边
// 留了条无渠道也能进的路。编排一律是「被调即失败」的替身：未配置 Intake 的合同就是
// 不构造命令，编排若被触到，说明有请求穿过了未配置格。
func unconfiguredDeliveryEndpoints(t *testing.T) map[string]http.Handler {
	t.Helper()
	handler := unreachableDeliveryHandler{t: t}
	return map[string]http.Handler{
		"register": tfhttp.NewRegisterEffectiveDeliveryEndpoint(tfhttp.UnconfiguredIntake{}, handler),
		"correct":  tfhttp.NewCorrectDeliveryProofEndpoint(tfhttp.UnconfiguredIntake{}, handler),
	}
}

// Covers: ADR-0055 「未配置即拒、不读内容」 — 未配置 Intake 对每个请求答 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED，不读业务内容、不构造命令；按 ADR-0022，4xx 不带
// `outcome`。派送端离线补传靠 4xx/5xx 分流，这一格要的是出队交人去配置渠道。
func TestUnconfiguredDeliveryIntakeRefusesWithoutReadingTheBody(t *testing.T) {
	for name, endpoint := range unconfiguredDeliveryEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			probe := &readProbe{}
			request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries", probe)
			response := httptest.NewRecorder()

			endpoint.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
			if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", got)
			}
			assertNoOutcome(t, response)
			if probe.read {
				t.Fatal("an unconfigured intake read the business content")
			}
		})
	}
}

// Covers: ADR-0055 「答复对一切请求内容与自报身份一致，不因内容不同而变」 — POD 证据
// 引用与设备自报的租户号都在请求里；答复随它们变化就是采信的第一个征兆。
func TestUnconfiguredDeliveryIntakeAnswersEveryRequestIdentically(t *testing.T) {
	for name, endpoint := range unconfiguredDeliveryEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			baseline := httptest.NewRecorder()
			endpoint.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries", nil))

			variants := map[string]*http.Request{
				"empty body": httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries", nil),
				"delivery body": httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries",
					strings.NewReader(`{"object":"OBJ-9","proof":"POD-9"}`)),
				"garbage body": httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries", strings.NewReader("!!not-json!!")),
				"query string": httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries?tenant=TENANT-9", nil),
			}
			reported := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries", nil)
			reported.Header.Set("X-Reported-Tenant", "TENANT-9")
			variants["self-reported identity headers"] = reported

			for variant, request := range variants {
				response := httptest.NewRecorder()
				endpoint.ServeHTTP(response, request)
				if response.Code != baseline.Code || response.Body.String() != baseline.Body.String() {
					t.Fatalf("%s: answer differs from baseline: %d %s vs %d %s",
						variant, response.Code, response.Body.String(), baseline.Code, baseline.Body.String())
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

type unreachableDeliveryHandler struct{ t *testing.T }

func (handler unreachableDeliveryHandler) Register(
	context.Context,
	application.RegisterEffectiveDeliveryCommand,
) (application.RegisterEffectiveDeliveryResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RegisterEffectiveDeliveryResult{}, nil
}

func (handler unreachableDeliveryHandler) Correct(
	context.Context,
	application.CorrectDeliveryProofCommand,
) (application.RegisterEffectiveDeliveryResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RegisterEffectiveDeliveryResult{}, nil
}
