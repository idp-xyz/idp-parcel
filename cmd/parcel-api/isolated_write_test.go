package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// Covers: ADR-0180 决定三 — 隔离写开关开着、本产品的权威串也设了而两值不等，启动即拒：一个进程里不许有两个「本产品」。
// 两值相等照常启用；写开关没开时不判，权威串单独设着是生产形态的正常配置。
func TestBuildIsolatedWriteAdmissionRefusesASecondSelfAuthority(t *testing.T) {
	admission, err := buildIsolatedWriteAdmission(fakeGetenv(map[string]string{
		isolatedWriteTenantEnv:     "SYN-TENANT-01",
		governanceSelfAuthorityEnv: "SYN-AUTH/another-product",
	}))
	if err == nil {
		t.Fatalf("两个权威串并存：admission = %+v, want 启动即拒", admission)
	}
	if !strings.Contains(err.Error(), governanceSelfAuthorityEnv) {
		t.Fatalf("拒因没点名参数：%v", err)
	}
	if admission, err := buildIsolatedWriteAdmission(fakeGetenv(map[string]string{
		isolatedWriteTenantEnv:     "SYN-TENANT-01",
		governanceSelfAuthorityEnv: isolatedSelfAuthority,
	})); err != nil || admission == nil {
		t.Fatalf("两值相等：admission = %+v, err = %v", admission, err)
	}
	if admission, err := buildIsolatedWriteAdmission(fakeGetenv(map[string]string{
		governanceSelfAuthorityEnv: "SYN-AUTH/another-product",
	})); err != nil || admission != nil {
		t.Fatalf("写开关没开：admission = %+v, err = %v, want 未启用且不拒", admission, err)
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

// Covers: ADR-0150 决定三与票 first-party-api/00「隔离提交放行已撤」——写开关开着、首方客户族没配时，`/shipment-requests`
// 答的是首方族的未配置，与生产形态逐字节相同：写开关不再进业务端点的装配，它换不了任何命令面。读开关同样换不了这一行
// （ADR-0091 决定四）。
func TestNeitherIsolatedSwitchOpensTheSubmissionLine(t *testing.T) {
	if admission, err := buildIsolatedWriteAdmission(fakeGetenv(map[string]string{
		isolatedWriteTenantEnv: "SYN-TENANT-01",
	})); err != nil || admission == nil {
		t.Fatalf("写开关：admission = %+v, err = %v", admission, err)
	}
	intakes, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{
		isolatedReadTenantEnv: "SYN-TENANT-01",
	}))
	if err != nil {
		t.Fatalf("build isolated read intakes: %v", err)
	}
	const unconfigured = `{"error":{"code":"ACCESS_CHANNEL_NOT_CONFIGURED"}}` + "\n"
	router := httpapi.NewWithEndpoints(buildinfo.Info{},
		assembleUnwiredBusinessEndpointsWith(intakes, unconfiguredCustomerSubmission(t)))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/shipment-requests", strings.NewReader(`{"any":"body"}`)))
	if response.Code != http.StatusForbidden || response.Body.String() != unconfigured {
		t.Fatalf("/shipment-requests: %d %q, want 403 %q——两个隔离开关都开不了提交口", response.Code, response.Body.String(), unconfigured)
	}
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
