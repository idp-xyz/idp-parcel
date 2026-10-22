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

// customerFaceCommandPatterns 是 PS 的客户面命令口：撤回、资料修订、受控补充、逐包裹取消。它们今天挂未配置即拒、还不属首方族（不带 `/v1`），问题体已经是首方契约
// 的形状：形状随处理器，前缀随装配族（ADR-0140 决定二、三）。
var customerFaceCommandPatterns = []string{
	"/shipment-requests/withdrawals",
	"/shipment-requests/source-data-amendments",
	"/shipment-requests/supplements",
	"/shipment-requests/parcel-cancellations",
}

// sameReleaseFamilies 是与本产品客户端同版本发布的各族：操作者渠道的各能力面（主数据与运营查阅读、登记册配置写、运营决定）
// 与作业事实登记（ADR-0021）。
var sameReleaseFamilies = map[faceFamily]bool{
	faceOperationsRead: true, faceRegistryWrite: true, faceOperationDecision: true, faceWorkFact: true,
}

// Covers: ADR-0140 决定二与 Consequences「装配测试对首方族端点断言前缀、对操作者族端点断言无前缀」——首方客户族的每一行带
// URL 主版本 `/v1`；操作者族与作业事实族一行都不带：它们的调用方与本产品同版本发布，前缀只会是一个永远不变的 `v1`。集成客户端族
// 暂不断言：ADR-0140 决定二要求它也带 `/v1`，待 code/82；未配置即拒的行等各自那一族的 Intake 换上时随族定。
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
	sameRelease := declaredPatterns(func(line endpointLine) bool { return sameReleaseFamilies[line.face.family] })
	if len(sameRelease) == 0 {
		t.Fatal("登记处没有操作者族或作业事实族的行；本条断言会永远空过")
	}
	for _, pattern := range sameRelease {
		if strings.HasPrefix(pattern, "/v1/") {
			t.Errorf("与产品同版本发布的 %s 带了 /v1", pattern)
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

// Covers: ADR-0140 决定三——首方客户族各口与 PS 客户面命令口（customerFaceCommandPatterns）的 4xx（403 未配置与 405 方法不对两种探针） 是 `application/problem+json`：`type`、`title`、`status`、
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

		// 方法不对由处理器自答 405（httpapi.BusinessEndpoint 刻意不带方法），形状同样是问题详情，不是 chi 的纯文本。
		wrong := http.MethodGet
		if probe.method == http.MethodGet {
			wrong = http.MethodPost
		}
		refused := httptest.NewRecorder()
		router.ServeHTTP(refused, httptest.NewRequest(wrong, probe.target, nil))
		if refused.Code != http.StatusMethodNotAllowed || refused.Header().Get("Allow") != probe.method ||
			refused.Header().Get("Content-Type") != httpapi.ProblemMediaType || problemCode(t, refused) != "METHOD_NOT_ALLOWED" {
			t.Errorf("%s %s: %d Allow=%q %q %s, want 405 问题详情", wrong, pattern, refused.Code, refused.Header().Get("Allow"),
				refused.Header().Get("Content-Type"), refused.Body.Bytes())
		}
	}
}

// firstPartyUnconfigured 是首方族各口答「接入渠道未配置」的那一份问题详情，去掉逐请求生成的 requestId 之后逐字节比。
const firstPartyUnconfigured = `{"code":"ACCESS_CHANNEL_NOT_CONFIGURED","status":403,"title":"Access channel not configured",` +
	`"type":"tag:idp.xyz,2026:problem:ACCESS_CHANNEL_NOT_CONFIGURED"}`

// answersFirstPartyUnconfigured：403、问题详情媒体类型，除 requestId 外与 firstPartyUnconfigured 逐字节相同。
func answersFirstPartyUnconfigured(t *testing.T, response *httptest.ResponseRecorder) bool {
	t.Helper()
	return response.Code == http.StatusForbidden && response.Header().Get("Content-Type") == httpapi.ProblemMediaType &&
		withoutRequestID(t, response) == firstPartyUnconfigured
}
