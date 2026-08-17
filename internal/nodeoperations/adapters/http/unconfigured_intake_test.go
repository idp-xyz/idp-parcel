package nodeopshttp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

// unconfiguredReception 装出本包唯一那个端点的未配置形态。编排一律是「被调即失败」的
// 替身：未配置 Intake 的合同就是不构造命令，编排若被触到，说明有请求穿过了未配置格
// ——装配点因此才允许在真渠道就位前不装配任何应用编排。
func unconfiguredReception(t *testing.T) http.Handler {
	t.Helper()
	return nodeopshttp.NewReceiveDeliveredUnitEndpoint(
		nodeopshttp.UnconfiguredIntake{}, unreachableReceptionHandler{t: t},
	)
}

// Covers: ADR-0055 「未配置即拒、不读内容」 — 未配置 Intake 对每个请求答 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED，不读业务内容、不构造命令；按 ADR-0022，4xx 不带
// `outcome`。设备侧尤其要紧：4xx 出队交人，正是「去配置接入渠道」这个恢复动作。
func TestUnconfiguredReceptionRefusesWithoutReadingTheBody(t *testing.T) {
	probe := &readProbe{}
	request := httptest.NewRequest(http.MethodPost, "/node-operations/receptions", probe)
	response := httptest.NewRecorder()

	unconfiguredReception(t).ServeHTTP(response, request)

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
}

// Covers: ADR-0055 「答复对一切请求内容与自报身份一致，不因内容不同而变」 — 采信自报
// 身份的第一个征兆就是答复随它变化；这里钉住不同载荷、自报租户/节点头与查询串得到
// 逐字节相同的答复。
func TestUnconfiguredReceptionAnswersEveryRequestIdentically(t *testing.T) {
	endpoint := unconfiguredReception(t)
	baseline := httptest.NewRecorder()
	endpoint.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/node-operations/receptions", nil))

	variants := map[string]*http.Request{
		"empty body":   httptest.NewRequest(http.MethodPost, "/node-operations/receptions", nil),
		"device body":  httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader(`{"sourceId":"SCAN-9"}`)),
		"garbage body": httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader("!!not-json!!")),
		"query string": httptest.NewRequest(http.MethodPost, "/node-operations/receptions?tenant=TENANT-9", nil),
	}
	reported := httptest.NewRequest(http.MethodPost, "/node-operations/receptions", nil)
	reported.Header.Set("X-Reported-Tenant", "TENANT-9")
	reported.Header.Set("X-Reported-Node", "NODE-9")
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

type unreachableReceptionHandler struct{ t *testing.T }

func (handler unreachableReceptionHandler) Handle(
	context.Context,
	application.ReceiveDeliveredUnitCommand,
) (application.ReceiveDeliveredUnitResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.ReceiveDeliveredUnitResult{}, nil
}
