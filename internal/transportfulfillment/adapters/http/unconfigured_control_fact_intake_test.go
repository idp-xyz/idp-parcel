package tfhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// unconfiguredControlFactEndpoints 遍历控制事实入口里装着 UnconfiguredIntake 的端点（票 04）。
// 未配置是渠道这一层的状态，不是某个端点的状态：交接首登、交接更正各走 HandoverIntake 的
// 一个方法，少堵一个就是给那一个留了条无渠道也能进的路。编排一律是「被调即失败」的替身。
func unconfiguredControlFactEndpoints(t *testing.T) map[string]http.Handler {
	t.Helper()
	handover := unreachableHandoverHandler{t: t}
	return map[string]http.Handler{
		"/transport-fulfillment/handovers":               tfhttp.NewRegisterTransportHandoverEndpoint(tfhttp.UnconfiguredIntake{}, handover),
		"/transport-fulfillment/handover-corrections":    tfhttp.NewCorrectTransportHandoverEndpoint(tfhttp.UnconfiguredIntake{}, handover),
		"/transport-fulfillment/offsite-pickups":         tfhttp.NewRegisterOffsitePickupEndpoint(tfhttp.UnconfiguredIntake{}, unreachablePickupRegistrationHandler{t: t}),
		"/transport-fulfillment/offsite-pickup-attempts": tfhttp.NewPerformOffsitePickupEndpoint(tfhttp.UnconfiguredIntake{}, unreachablePickupAttemptHandler{t: t}),
	}
}

// Covers: ADR-0055「未配置即拒、不读内容」在控制事实入口上——403 + ACCESS_CHANNEL_NOT_CONFIGURED，
// 不读业务内容、不构造命令；4xx 不带 outcome（ADR-0022）。**段引用也在请求体里**：未配置格连
// 它都不读，进段那道门在渠道就位前不可能被任何请求触到。
func TestUnconfiguredControlFactIntakeRefusesWithoutReadingTheBody(t *testing.T) {
	for path, endpoint := range unconfiguredControlFactEndpoints(t) {
		t.Run(path, func(t *testing.T) {
			probe := &readProbe{}
			request := httptest.NewRequest(http.MethodPost, path, probe)
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

// Covers: ADR-0055「答复对一切请求内容与自报身份一致」——带段引用的体、自报租户头、垃圾体
// 都得到与空体逐字节相同的答复。
func TestUnconfiguredControlFactIntakeAnswersEveryRequestIdentically(t *testing.T) {
	for path, endpoint := range unconfiguredControlFactEndpoints(t) {
		t.Run(path, func(t *testing.T) {
			baseline := httptest.NewRecorder()
			endpoint.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, path, nil))

			variants := map[string]*http.Request{
				"empty body":   httptest.NewRequest(http.MethodPost, path, nil),
				"segment body": httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"object":"OBJ-9","segment":"SEG-9"}`)),
				"garbage body": httptest.NewRequest(http.MethodPost, path, strings.NewReader("!!not-json!!")),
				"query string": httptest.NewRequest(http.MethodPost, path+"?tenant=TENANT-9", nil),
			}
			reported := httptest.NewRequest(http.MethodPost, path, nil)
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

type unreachableHandoverHandler struct{ t *testing.T }

func (handler unreachableHandoverHandler) Register(
	context.Context,
	application.RegisterTransportHandoverCommand,
) (application.RegisterTransportHandoverResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RegisterTransportHandoverResult{}, nil
}

func (handler unreachableHandoverHandler) Correct(
	context.Context,
	application.CorrectTransportHandoverCommand,
) (application.RegisterTransportHandoverResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RegisterTransportHandoverResult{}, nil
}

type unreachablePickupAttemptHandler struct{ t *testing.T }

func (handler unreachablePickupAttemptHandler) Handle(
	context.Context,
	application.PerformOffsitePickupCommand,
) (application.PerformOffsitePickupResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.PerformOffsitePickupResult{}, nil
}

type unreachablePickupRegistrationHandler struct{ t *testing.T }

func (handler unreachablePickupRegistrationHandler) Register(
	context.Context,
	application.RegisterOffsitePickupCommand,
) (application.RegisterOffsitePickupResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RegisterOffsitePickupResult{}, nil
}
