package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// businessEndpointMethods 是本进程应当服务的全部业务端点及各自的方法。它与装配点
// 互为对照：表里多一项说明装配漏了一个端点（那一项会退回路由层的 404，正是 ADR-0055
// 要治的折叠），装配点里多一项说明上线了一个没人钉过形状的面。方法必须逐个写对——
// 拿错方法测出来的 405 会盖过 403，断言就什么也没守住。
var businessEndpointMethods = map[string]string{
	"/shipment-requests":                                http.MethodPost,
	"/shipment-requests/withdrawals":                    http.MethodPost,
	"/node-operations/receptions":                       http.MethodPost,
	"/transport-fulfillment/deliveries":                 http.MethodPost,
	"/transport-fulfillment/delivery-proof-corrections": http.MethodPost,
	"/customer-tracking-view":                           http.MethodGet,
	"/claims":                                           http.MethodPost,
	"/customs/external-results":                         http.MethodPost,
}

// Covers: ADR-0055 第一、二、三条 — 端点已装配、未配置自成一格、状态码取 403。
//
// 这是唯一证明「装配确实发生了」的地方：各上下文的传输层测试拿自己构造的处理器跑，
// 装不装配它们都绿。这里走的是 cmd/parcel-api 真正交给 http.Server 的那个路由。
func TestEveryAssembledEndpointAnswersUnconfigured(t *testing.T) {
	// 传 unwiredSubmission 而非真编排：本测试钉的是未配置面（403 在编排之前），
	// 真编排的装配与行为由 assemble_submission_test.go 对真库另证。
	endpoints := assembleBusinessEndpoints(unwiredSubmission{})
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints)

	mounted := make(map[string]bool, len(endpoints))
	for _, endpoint := range endpoints {
		method, listed := businessEndpointMethods[endpoint.Pattern]
		if !listed {
			t.Fatalf("装配了未登记的端点 %s：形状没有任何测试钉住", endpoint.Pattern)
		}
		if mounted[endpoint.Pattern] {
			t.Fatalf("端点 %s 装配了两次：后一个会静默盖掉前一个", endpoint.Pattern)
		}
		mounted[endpoint.Pattern] = true

		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, endpoint.Pattern, nil))

		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s: status = %d, want %d", method, endpoint.Pattern, response.Code, http.StatusForbidden)
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", endpoint.Pattern, got)
		}
		assertNoOutcome(t, response, endpoint.Pattern)
	}

	for pattern := range businessEndpointMethods {
		if !mounted[pattern] {
			t.Fatalf("端点 %s 没进装配：它会答 404，与「产品没有这个能力」不可分辨", pattern)
		}
	}
}

// Covers: ADR-0055 「未配置格住在 Intake 缝里，不在路由层另设闸」 — 未配置不改变方法
// 约束：方法不对仍由处理器自己答 405，403 不越过它抢答。两处各有权威就会各改一次。
func TestUnconfiguredDoesNotSwallowTheMethodGate(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleBusinessEndpoints(unwiredSubmission{}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/shipment-requests", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", allow, http.MethodPost)
	}
}

// Covers: ADR-0055 「答复对一切请求内容与自报身份一致」 — 在装配后的路由上再钉一次：
// 各包的替身证的是自己那个处理器，这里证的是进程真正对外的那一个。
func TestAssembledEndpointsIgnoreSelfReportedIdentity(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleBusinessEndpoints(unwiredSubmission{}))

	baseline := httptest.NewRecorder()
	router.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/shipment-requests", nil))

	reported := httptest.NewRequest(http.MethodPost, "/shipment-requests", nil)
	reported.Header.Set("X-Reported-Tenant", "TENANT-9")
	reported.Header.Set("Authorization", "Bearer whatever")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, reported)

	if response.Code != baseline.Code || response.Body.String() != baseline.Body.String() {
		t.Fatalf("answer differs from baseline: %d %s vs %d %s",
			response.Code, response.Body.String(), baseline.Code, baseline.Body.String())
	}
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

func assertNoOutcome(t *testing.T, response *httptest.ResponseRecorder, pattern string) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
	if _, present := fields["outcome"]; present {
		t.Fatalf("%s: 一个没形成答案的响应带了 outcome：%s", pattern, response.Body.Bytes())
	}
}
