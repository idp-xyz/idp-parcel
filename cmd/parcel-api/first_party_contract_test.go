package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// customerFaceCommandPatterns 是 PS 客户面四个命令口。它们今天挂未配置即拒、还不属首方族（不带 `/v1`），问题体已经是首方契约
// 的形状：形状随处理器，前缀随装配族（ADR-0140 决定二、三）。
var customerFaceCommandPatterns = []string{
	"/shipment-requests/withdrawals",
	"/shipment-requests/source-data-amendments",
	"/shipment-requests/supplements",
	"/shipment-requests/parcel-cancellations",
}

// Covers: ADR-0140 决定二与 Consequences「装配测试对首方族端点断言前缀、对操作者族端点断言无前缀」——首方客户族的每一行带
// URL 主版本 `/v1`，其余各族一行都不带：它们的调用方与本产品同版本发布，前缀只会是一个永远不变的 `v1`。
func TestOnlyTheFirstPartyFamilyCarriesTheMajorVersion(t *testing.T) {
	firstParty := declaredPatterns(func(line endpointLine) bool { return line.face.family == faceCustomerClient })
	if len(firstParty) == 0 {
		t.Fatal("登记处没有首方客户族的行；本条断言会永远空过")
	}
	for _, pattern := range firstParty {
		if !strings.HasPrefix(pattern, "/v1/") {
			t.Errorf("首方客户族的 %s 不带 /v1", pattern)
		}
	}
	for _, pattern := range declaredPatterns(func(line endpointLine) bool { return line.face.family != faceCustomerClient }) {
		if strings.HasPrefix(pattern, "/v1/") {
			t.Errorf("非首方族的 %s 带了 /v1", pattern)
		}
	}
}

// Covers: ADR-0140 Consequences「首方族就位后，客户面提交口的路径变为 `/v1/shipment-requests`」——旧路径在首个兼容性基线之前
// 直接改名、不并挂（决定五：首版发布前没有基线；决定六：弃用窗口只对已发布端点），装配后的路由上旧路径答 404，不落进任何处理器。
func TestTheUnversionedFirstPartyPathsAreGone(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnconfiguredBusinessEndpoints())
	for _, old := range []struct{ method, target string }{
		{http.MethodPost, "/shipment-requests"},
		{http.MethodGet, "/customer-tracking-view?parcel=SYN-PCL-1"},
		{http.MethodPost, "/node-operations/delivery-pre-advices"},
		{http.MethodPost, "/node-operations/delivery-pre-advice-withdrawals"},
		{http.MethodPost, "/node-operations/delivery-handover-conclusions"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(old.method, old.target, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", old.method, old.target, response.Code)
		}
	}
}

// Covers: ADR-0140 决定三——首方族五口与 PS 客户面四个命令口的 4xx 是 `application/problem+json`：`type`、`title`、`status`、
// `code`、`requestId` 俱在，不带旧形状的 `error` 成员，也不带 `outcome`。在装配后的路由上探：requestId 只有经过路由中间件才有。
func TestCustomerFacingLinesAnswerProblemDetails(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnconfiguredBusinessEndpoints())
	probes := businessEndpointProbes()
	patterns := append(declaredPatterns(func(line endpointLine) bool { return line.face.family == faceCustomerClient }), customerFaceCommandPatterns...)
	for _, pattern := range patterns {
		probe, declared := probes[pattern]
		if !declared {
			t.Fatalf("%s 不在登记处", pattern)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(probe.method, probe.target, nil))

		if got := response.Header().Get("Content-Type"); got != httpapi.ProblemMediaType {
			t.Errorf("%s: Content-Type = %q, want %s", pattern, got, httpapi.ProblemMediaType)
			continue
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decode %s: %v", pattern, response.Body.Bytes(), err)
		}
		for _, member := range []string{"type", "title", "status", "code", "requestId"} {
			if _, present := body[member]; !present {
				t.Errorf("%s: 问题体缺 %s：%s", pattern, member, response.Body.Bytes())
			}
		}
		for _, member := range []string{"error", "outcome", "detail"} {
			if _, present := body[member]; present {
				t.Errorf("%s: 403 问题体带了 %s：%s", pattern, member, response.Body.Bytes())
			}
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Errorf("%s: code = %q", pattern, got)
		}
	}
}

// firstPartyUnconfigured 是首方族各口答「接入渠道未配置」的那一份问题详情，去掉逐请求生成的 requestId 之后逐字节比。
const firstPartyUnconfigured = `{"code":"ACCESS_CHANNEL_NOT_CONFIGURED","status":403,"title":"Access channel not configured",` +
	`"type":"urn:idp-parcel:problem:ACCESS_CHANNEL_NOT_CONFIGURED"}`

// answersFirstPartyUnconfigured：403、问题详情媒体类型，除 requestId 外与 firstPartyUnconfigured 逐字节相同。
func answersFirstPartyUnconfigured(t *testing.T, response *httptest.ResponseRecorder) bool {
	t.Helper()
	return response.Code == http.StatusForbidden && response.Header().Get("Content-Type") == httpapi.ProblemMediaType &&
		withoutRequestID(t, response) == firstPartyUnconfigured
}
