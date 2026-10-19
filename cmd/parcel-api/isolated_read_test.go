package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

func fakeGetenv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

// Covers: ADR-0078 Decision 三 — 未设即全拦（nil 输入），装配缺省朝拦。
func TestBuildIsolatedReadIntakesUnsetMeansDisabled(t *testing.T) {
	intakes, err := buildIsolatedReadIntakes(fakeGetenv(nil))
	if err != nil {
		t.Fatalf("unset env: err = %v, want nil", err)
	}
	if intakes != nil {
		t.Fatalf("unset env: intakes = %+v, want nil", intakes)
	}
}

// Covers: ADR-0078 Decision 三 — 非合成前缀启动即拒，不静默回落：静默回落会让配置
// 错误与「刻意拦着」两态可观察签名相同。
func TestBuildIsolatedReadIntakesRefusesRealLookingTenant(t *testing.T) {
	intakes, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{
		isolatedReadTenantEnv: "TENANT-PROD-1",
	}))
	if err == nil {
		t.Fatalf("non-synthetic tenant accepted: intakes = %+v", intakes)
	}
	if !strings.Contains(err.Error(), syntheticIdentifierPrefix+"-") {
		t.Fatalf("refusal does not name the required prefix: %v", err)
	}
}

// Covers: ADR-0078 Decision 三 — 合成租户合规时委托查阅那一只 Intake 就位；漏了它，委托查阅三口在启用态仍答 403。
// 运营查阅其余各族已换操作者查阅读（票 operator-channel/05、35），开关不再产出它们的 Intake。
func TestBuildIsolatedReadIntakesGrantsTheShipmentViews(t *testing.T) {
	intakes, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{
		isolatedReadTenantEnv: "SYN-TENANT-01",
	}))
	if err != nil {
		t.Fatalf("synthetic tenant refused: %v", err)
	}
	if intakes == nil || intakes.shipmentRequestViews == nil {
		t.Fatalf("shipment views intake missing: %+v", intakes)
	}
}

// isolatedReadAdmittedPatterns 是 ADR-0078 Decision 一的放行面今天还剩的那几口：委托查阅与它的两个子集视图，三口
// 共用一只 Intake 变量。它与下面的两态测试互为对照：这里多列一个端点，启用态断言会在该端点上撞到 403 而失败；少列
// 一个，会在「其余端点仍拒」的断言上失败——枚举漂移在两个方向上都有测试信号。运营查阅其余各族已换操作者查阅读、
// 放行同笔撤下（ADR-0150 决定三），那几口的答复格由 TestSwappedCatalogueReadFacesAnswerFromTheOperatorChannel 钉。
var isolatedReadAdmittedPatterns = map[string]bool{
	"/shipment-request-views": true,
	// 复核队列与授权处置队列是委托查阅面的子集视图，在装配上共用同一个 Intake 变量，启用态必然随它一起放行——漏了
	// 就等于断言「同一个 Intake 会给出两种答案」，那是装不出来的形状。
	"/acceptance-review-queue":      true,
	"/authorized-disposition-queue": true,
}

// Covers: ADR-0078 Decision 一、二 — 启用态只放运营查阅行。unwired* 读口交回稳定
// 错误，于是「越过了 Intake」在传输层可观察为 NO_ANSWER_FORMED（5xx）而不再是
// ACCESS_CHANNEL_NOT_CONFIGURED（403）：两态在同一探针下签名不同，恰是本测试要钉的
// 分界。命令面与客户查阅面照旧 403——放行装不进它们由编译期保证，这里再从进程真正
// 对外的路由上证一次。
func TestIsolatedReadAdmissionSwitchesOnlyOperationsReadLines(t *testing.T) {
	intakes, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{
		isolatedReadTenantEnv: "SYN-TENANT-01",
	}))
	if err != nil {
		t.Fatalf("build isolated read intakes: %v", err)
	}
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpoints(intakes))

	for pattern, probe := range businessEndpointProbes {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(probe.method, probe.target, nil))

		if isolatedReadAdmittedPatterns[pattern] {
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("%s: status = %d, want %d (granted intake must reach the unwired reader)",
					pattern, response.Code, http.StatusInternalServerError)
			}
			if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
				t.Fatalf("%s: code = %q, want NO_ANSWER_FORMED", pattern, got)
			}
			continue
		}

		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want %d (non-admitted endpoints must stay unconfigured)",
				pattern, response.Code, http.StatusForbidden)
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", pattern, got)
		}
	}
}

// Covers: ADR-0078 Decision 二 — 放行答复与自报身份无关：作用域来自注入，带不带
// 冒充头答案一致（与未配置面的同名测试同款，证的是启用态）。
func TestIsolatedReadAdmissionIgnoresSelfReportedIdentity(t *testing.T) {
	intakes, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{
		isolatedReadTenantEnv: "SYN-TENANT-01",
	}))
	if err != nil {
		t.Fatalf("build isolated read intakes: %v", err)
	}
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpoints(intakes))

	baseline := httptest.NewRecorder()
	router.ServeHTTP(baseline, httptest.NewRequest(http.MethodGet, "/tracking-projections", nil))

	reported := httptest.NewRequest(http.MethodGet, "/tracking-projections", nil)
	reported.Header.Set("X-Reported-Tenant", "TENANT-9")
	reported.Header.Set("Authorization", "Bearer whatever")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, reported)

	if response.Code != baseline.Code || response.Body.String() != baseline.Body.String() {
		t.Fatalf("answer differs from baseline: %d %s vs %d %s",
			response.Code, response.Body.String(), baseline.Code, baseline.Body.String())
	}
}
