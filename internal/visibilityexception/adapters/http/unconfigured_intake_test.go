package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本包各端点的方法并不相同：视图查询是 GET，索赔提交与配置登记是 POST。未配置格要在各自
// 的方法上成立——拿错方法测出来的 405 会盖过 403，测试就什么也没钉住。
type unconfiguredCase struct {
	endpoint http.Handler
	method   string
	path     string
}

// unconfiguredVisibilityEndpoints 遍历本包装着 UnconfiguredIntake 的全部端点。下游一律
// 是「被调即失败」的替身：未配置 Intake 的合同就是不构造查询键也不构造命令，下游若被
// 触到，说明有请求穿过了未配置格。
//
// 六个配置登记写面（ADR-0085，票 admin-write-faces/02 切片 02d）一并进这张表，判据与读面
// 同一条：写面的未配置格也住在 Intake 缝里，且它更要紧——穿过去的不是一次读，是一次写。
func unconfiguredVisibilityEndpoints(t *testing.T) map[string]unconfiguredCase {
	t.Helper()
	unconfigured := visibilityhttp.UnconfiguredIntake{}
	return map[string]unconfiguredCase{
		"tracking view": {
			endpoint: visibilityhttp.NewQueryCustomerTrackingViewEndpoint(
				unconfigured, unreachableViewReader{t: t},
			),
			method: http.MethodGet,
			path:   "/customer-tracking-view",
		},
		"claim": {
			endpoint: visibilityhttp.NewReceiveClaimEndpoint(
				unconfigured, unreachableClaimReceiver{t: t},
			),
			method: http.MethodPost,
			path:   "/claims",
		},
		"milestone mapping registration": {
			endpoint: visibilityhttp.NewRegisterMilestoneMappingEndpoint(
				unconfigured, unreachableRegistrar[application.RegisterMilestoneMappingCommand]{t: t},
			),
			method: http.MethodPost,
			path:   "/visibility-milestone-mapping-registrations",
		},
		"triage rules registration": {
			endpoint: visibilityhttp.NewRegisterTriageRulesEndpoint(
				unconfigured, unreachableRegistrar[application.RegisterTriageRulesCommand]{t: t},
			),
			method: http.MethodPost,
			path:   "/visibility-triage-rule-registrations",
		},
		"notification policy registration": {
			endpoint: visibilityhttp.NewRegisterNotificationPolicyEndpoint(
				unconfigured, unreachableRegistrar[application.RegisterNotificationPolicyCommand]{t: t},
			),
			method: http.MethodPost,
			path:   "/visibility-notification-policy-registrations",
		},
		"claim eligibility registration": {
			endpoint: visibilityhttp.NewRegisterClaimEligibilityEndpoint(
				unconfigured, unreachableRegistrar[application.RegisterClaimEligibilityCommand]{t: t},
			),
			method: http.MethodPost,
			path:   "/visibility-claim-eligibility-registrations",
		},
		"claim authorization registration": {
			endpoint: visibilityhttp.NewRegisterClaimAuthorizationEndpoint(
				unconfigured, unreachableRegistrar[application.RegisterClaimAuthorizationCommand]{t: t},
			),
			method: http.MethodPost,
			path:   "/visibility-claim-authorization-registrations",
		},
		"disclosure policy registration": {
			endpoint: visibilityhttp.NewRegisterDisclosurePolicyEndpoint(
				unconfigured, unreachableRegistrar[application.RegisterDisclosurePolicyCommand]{t: t},
			),
			method: http.MethodPost,
			path:   "/visibility-disclosure-policy-registrations",
		},
	}
}

// Covers: ADR-0055 「未配置即拒、不读内容」 — 未配置 Intake 对每个请求答 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED，不读业务内容、不构造查询键或命令；按 ADR-0022，
// 4xx 不带 `outcome`。
//
// 与 ADR-0029 的探针纪律不冲突：这里披露的是「本产品有此端点、渠道未配置」，属产品
// 表面；对象是否存在、是否属于别的账户仍然一律同答，那条约束由 VIEW_NOT_FOUND 守，
// 真渠道 Intake 就位后照常适用（ADR-0055 第四条）。
func TestUnconfiguredVisibilityIntakeRefusesWithoutReadingTheBody(t *testing.T) {
	for name, unconfigured := range unconfiguredVisibilityEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			probe := &readProbe{}
			request := httptest.NewRequest(unconfigured.method, unconfigured.path, probe)
			response := httptest.NewRecorder()

			unconfigured.endpoint.ServeHTTP(response, request)

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

// Covers: ADR-0055 「答复对一切请求内容与自报身份一致，不因内容不同而变」 — 客户自报的
// 账户号是这一面最危险的自报身份（采信它就穿透账户隔离）；答复随它变化就是采信的第一个
// 征兆。查询面的自报身份多半在查询串里，因此变体覆盖到它。
func TestUnconfiguredVisibilityIntakeAnswersEveryRequestIdentically(t *testing.T) {
	for name, unconfigured := range unconfiguredVisibilityEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			baseline := httptest.NewRecorder()
			unconfigured.endpoint.ServeHTTP(baseline, httptest.NewRequest(unconfigured.method, unconfigured.path, nil))

			variants := map[string]*http.Request{
				"empty body":   httptest.NewRequest(unconfigured.method, unconfigured.path, nil),
				"json body":    httptest.NewRequest(unconfigured.method, unconfigured.path, strings.NewReader(`{"customerAccount":"CUST-9"}`)),
				"garbage body": httptest.NewRequest(unconfigured.method, unconfigured.path, strings.NewReader("!!not-json!!")),
				"query string": httptest.NewRequest(unconfigured.method, unconfigured.path+"?customerAccount=CUST-9&parcel=PARCEL-9", nil),
			}
			reported := httptest.NewRequest(unconfigured.method, unconfigured.path, nil)
			reported.Header.Set("X-Reported-Tenant", "TENANT-9")
			reported.Header.Set("X-Reported-Customer-Account", "CUST-9")
			variants["self-reported identity headers"] = reported

			for variant, request := range variants {
				response := httptest.NewRecorder()
				unconfigured.endpoint.ServeHTTP(response, request)
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

type unreachableViewReader struct{ t *testing.T }

func (reader unreachableViewReader) FindCurrent(
	context.Context,
	domain.TenantID,
	domain.CustomerAccountReference,
	domain.TrackedParcelReference,
) (domain.CustomerTrackingView, bool, error) {
	reader.t.Fatal("a request passed the unconfigured intake and reached the view store")
	return domain.CustomerTrackingView{}, false, nil
}

type unreachableClaimReceiver struct{ t *testing.T }

func (receiver unreachableClaimReceiver) ReceiveClaim(
	context.Context,
	application.ReceiveClaimCommand,
) (application.HandleClaimResult, error) {
	receiver.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.HandleClaimResult{}, nil
}
