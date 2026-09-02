package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// Covers: ADR-0091 决定四 — 未设即禁用，装配缺省朝拦；与隔离读面同款三态的第一态。
func TestBuildIsolatedWriteAdmissionUnsetMeansDisabled(t *testing.T) {
	admission, err := buildIsolatedWriteAdmission(fakeGetenv(nil))
	if err != nil {
		t.Fatalf("unset env: err = %v, want nil", err)
	}
	if admission != nil {
		t.Fatalf("unset env: admission = %+v, want nil", admission)
	}
}

// Covers: ADR-0091 决定四 — 非合成前缀启动即拒，不静默回落。写路径会落行，静默回落
// 的代价比读面重一档：落下的行事后靠租户维分辨，而那一维正是这道门在把关。
func TestBuildIsolatedWriteAdmissionRefusesRealLookingTenant(t *testing.T) {
	admission, err := buildIsolatedWriteAdmission(fakeGetenv(map[string]string{
		isolatedWriteTenantEnv: "TENANT-PROD-1",
	}))
	if err == nil {
		t.Fatalf("non-synthetic tenant accepted: admission = %+v", admission)
	}
	if !strings.Contains(err.Error(), syntheticIdentifierPrefix+"-") {
		t.Fatalf("refusal does not name the required prefix: %v", err)
	}
}

// Covers: ADR-0091 决定二与决定五 — 合成租户合规时两格都就位：目录与自身权威串。
// 缺任一格，`ProductionOwnershipAdapter.governanceScope` 都会先于读登记册早退，
// 表现是启用了却仍答`权威未确定`。
func TestBuildIsolatedWriteAdmissionGrantsBothGrids(t *testing.T) {
	admission, err := buildIsolatedWriteAdmission(fakeGetenv(map[string]string{
		isolatedWriteTenantEnv: "SYN-TENANT-01",
	}))
	if err != nil {
		t.Fatalf("synthetic tenant refused: %v", err)
	}
	if admission == nil {
		t.Fatal("synthetic tenant yielded nil admission")
	}
	if admission.governanceDirectory == nil {
		t.Fatal("范围目录为空——归属会先于读登记册早退")
	}
	if admission.selfAuthority == "" {
		t.Fatal("自身权威串为空——无从判断命中的区间是不是自己的")
	}
	if !strings.HasPrefix(admission.selfAuthority, syntheticIdentifierPrefix+"-") {
		t.Fatalf("自身权威串 = %q，缺 %q 前缀——它会写进归属决定的修订串",
			admission.selfAuthority, syntheticIdentifierPrefix+"-")
	}
}

// Covers: ADR-0091 决定四 — 两开关都设时取值必须相同。取值不同时写下的委托挂在一个
// 读面不过滤的租户上，页面看不见它，而那个症状看起来像缺陷不像配置错。
func TestBuildIsolatedWriteAdmissionRefusesTenantMismatch(t *testing.T) {
	admission, err := buildIsolatedWriteAdmission(fakeGetenv(map[string]string{
		isolatedReadTenantEnv:  "SYN-TENANT-01",
		isolatedWriteTenantEnv: "SYN-TENANT-02",
	}))
	if err == nil {
		t.Fatalf("两开关取值不同也装得起来: admission = %+v", admission)
	}
	if !strings.Contains(err.Error(), isolatedReadTenantEnv) ||
		!strings.Contains(err.Error(), isolatedWriteTenantEnv) {
		t.Fatalf("拒绝理由没同时点名两个开关，配错的人不知道去改哪个：%v", err)
	}
}

// Covers: ADR-0091 决定二 — 写面放行只及 `/shipment-requests` 一行，其余命令面照旧
// `403` + `ACCESS_CHANNEL_NOT_CONFIGURED`。
//
// 放行后的提交口签名取 `400` + `MALFORMED_REQUEST`：探针请求没有报文体，而隔离 Intake
// 真的去读了它。这一格比 `500` 更能说明问题——`403` 意味着「压根没读」，`400` 意味着
// 「读了，读不出命令」，两者的可观察签名不同，正是本用例要钉的分界。
func TestIsolatedWriteAdmissionSwitchesOnlyTheSubmissionLine(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{},
		assembleUnwiredBusinessEndpointsWith(nil, isolatedSubmissionIntakeForTest(t)))

	for pattern, probe := range businessEndpointProbes {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(probe.method, probe.target, nil))

		if pattern == "/shipment-requests" {
			if response.Code != http.StatusBadRequest {
				t.Fatalf("%s: status = %d, want %d——放行后 Intake 该真的读请求",
					pattern, response.Code, http.StatusBadRequest)
			}
			if got := problemCode(t, response); got != "MALFORMED_REQUEST" {
				t.Fatalf("%s: code = %q, want MALFORMED_REQUEST", pattern, got)
			}
			continue
		}

		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want %d——写面放行只及提交一行",
				pattern, response.Code, http.StatusForbidden)
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", pattern, got)
		}
	}
}

// Covers: ADR-0091 决定四 — **读开关换不了写行**。只给隔离读入参时，提交口仍答 403；
// 这一条与上面那个用例互为对照，两个开关分设的全部意义就在这一格上。
func TestIsolatedReadAdmissionCannotOpenTheSubmissionLine(t *testing.T) {
	intakes, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{
		isolatedReadTenantEnv: "SYN-TENANT-01",
	}))
	if err != nil {
		t.Fatalf("build isolated read intakes: %v", err)
	}
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpoints(intakes))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/shipment-requests", nil))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d——读开关开了写行，两个开关就白分了",
			response.Code, http.StatusForbidden)
	}
}

// isolatedSubmissionIntakeForTest 用装配点自己的合成常量构造提交 Intake，归属那一侧换
// 放行替身（真库不在本用例范围内，本用例问的是路由与准入形状）。
func isolatedSubmissionIntakeForTest(t *testing.T) shipmenthttp.SubmissionIntake {
	t.Helper()
	intake, err := shipmenthttp.NewIsolatedSubmissionIntake(shipmenthttp.IsolatedSubmissionIntakeDeps{
		Tenant:          "SYN-TENANT-01",
		CustomerAccount: isolatedSubmissionCustomerAcct,
		Source:          isolatedSubmissionSource,
		ScopeReference:  isolatedSubmissionScopeRef,
		ScopeDigest:     isolatedSubmissionScopeDigest,
		Ownership:       permittingOwnership{anchor: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)},
		Clock:           fixedClock{at: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("构造隔离提交 Intake：%v", err)
	}
	return intake
}

// Covers: ADR-0091 决定四 — 两者取值相同时放行；写开关也可单设。读开关不是写开关的
// 前件：无头的接口验证不必先开读面，强求耦合是在这道门上多加一条没有理由的规则。
func TestBuildIsolatedWriteAdmissionAcceptsMatchingAndSoloTenants(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"两开关同值": {
			isolatedReadTenantEnv:  "SYN-TENANT-01",
			isolatedWriteTenantEnv: "SYN-TENANT-01",
		},
		"只设写开关": {
			isolatedWriteTenantEnv: "SYN-TENANT-01",
		},
	} {
		t.Run(name, func(t *testing.T) {
			admission, err := buildIsolatedWriteAdmission(fakeGetenv(env))
			if err != nil {
				t.Fatalf("被拒：%v", err)
			}
			if admission == nil {
				t.Fatal("交回 nil——该启用却没启用")
			}
		})
	}
}
