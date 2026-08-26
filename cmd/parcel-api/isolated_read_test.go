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

// Covers: ADR-0078 Decision 三 — 合成租户合规时六上下文 Intake 全部就位；漏一个字段
// 会让对应端点在启用态仍答 403，与放行面枚举失配。
func TestBuildIsolatedReadIntakesGrantsAllSixContexts(t *testing.T) {
	intakes, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{
		isolatedReadTenantEnv: "SYN-TENANT-01",
	}))
	if err != nil {
		t.Fatalf("synthetic tenant refused: %v", err)
	}
	if intakes == nil {
		t.Fatal("synthetic tenant yielded nil intakes")
	}
	if intakes.shipmentRequestViews == nil || intakes.trackingProjections == nil ||
		intakes.pricingCatalogue == nil || intakes.networkCatalog == nil ||
		intakes.complianceRules == nil || intakes.commercialCatalogue == nil {
		t.Fatalf("some context intake is nil: %+v", intakes)
	}
}

// isolatedReadAdmittedPatterns 是 ADR-0078 Decision 一的放行面。它与下面的两态
// 测试互为对照：这里多列一个端点，启用态断言会在该端点上撞到 403 而失败；少列一个，
// 会在「其余端点仍拒」的断言上失败——枚举漂移在两个方向上都有测试信号。
//
// 入格靠的是那三条判据（消费所属上下文的存储读面且不触发判断、派生或披露；零持久化；
// 作用域是运营侧的授权结果），不是 ADR 正文里那份名单——正文自己把名单限定在「当前
// 装配点上」，新的目录查阅端点满足同三条即入格，不另开 ADR。合同与协议两行就是照这
// 条进来的（票 admin-web-page-wiring-frontier/01）。
var isolatedReadAdmittedPatterns = map[string]bool{
	"/shipment-request-views":         true,
	"/tracking-projections":           true,
	"/pricing-price-cards":            true,
	"/pricing-reference-series":       true,
	"/network-catalog":                true,
	"/customs-compliance-rules":       true,
	"/commercial-service-products":    true,
	"/commercial-policies":            true,
	"/commercial-customer-contracts":  true,
	"/commercial-supplier-agreements": true,
	"/visibility-catalogues":          true,
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
	router.ServeHTTP(baseline, httptest.NewRequest(http.MethodGet, "/pricing-price-cards", nil))

	reported := httptest.NewRequest(http.MethodGet, "/pricing-price-cards", nil)
	reported.Header.Set("X-Reported-Tenant", "TENANT-9")
	reported.Header.Set("Authorization", "Bearer whatever")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, reported)

	if response.Code != baseline.Code || response.Body.String() != baseline.Body.String() {
		t.Fatalf("answer differs from baseline: %d %s vs %d %s",
			response.Code, response.Body.String(), baseline.Code, baseline.Body.String())
	}
}
