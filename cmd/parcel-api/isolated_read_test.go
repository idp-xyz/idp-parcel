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

// Covers: ADR-0078 Decision 三 — 合成租户合规时各上下文 Intake 全部就位；漏一个字段
// 会让对应端点在启用态仍答 403，与放行面枚举失配。
func TestBuildIsolatedReadIntakesGrantsAllContexts(t *testing.T) {
	intakes, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{
		isolatedReadTenantEnv: "SYN-TENANT-01",
	}))
	if err != nil {
		t.Fatalf("synthetic tenant refused: %v", err)
	}
	if intakes == nil {
		t.Fatal("synthetic tenant yielded nil intakes")
	}
	if intakes.shipmentRequestViews == nil || intakes.labelTransactions == nil || intakes.channelSelectionDecisions == nil ||
		intakes.trackingProjections == nil ||
		intakes.pricingCatalogue == nil || intakes.networkCatalog == nil ||
		intakes.complianceRules == nil || intakes.commercialCatalogue == nil ||
		intakes.collectionCatalogue == nil || intakes.settlementCatalogue == nil ||
		intakes.governanceRegisters == nil || intakes.nodeOperationsCatalogue == nil ||
		intakes.transportCatalogue == nil {
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
	"/shipment-request-views": true,
	// 复核队列是委托查阅面的子集视图，在装配上与它共用同一个 Intake 变量，因此启用态
	// 必然随它一起放行——这一行不是可选项，漏了它就等于断言「同一个 Intake 会给出两种
	// 答案」，而那是装不出来的形状（票 admin-skeleton-closure-batch/09）。
	"/acceptance-review-queue": true,
	// 授权处置队列同样是委托查阅面的子集视图、共用同一个 Intake 变量，启用态必然随它一起放行（票
	// sa-preacceptance-policy-view/04）。三条判据逐条满足：消费本上下文自己的存储读面、零持久化、
	// 作用域来自运营侧授权结果——队列只列不处置，处置在另一条写行上，那一行挂字面量 UnconfiguredIntake{}。
	"/authorized-disposition-queue": true,
	// 面单交易查阅同样共用委托查阅的 Intake 变量，因此启用态必然随它一起放行（票
	// admin-skeleton-closure-batch/08）。它照样满足那三条判据：消费本上下文自己的存储
	// 读面、零持久化、作用域来自运营侧授权结果——查阅一笔面单交易不触发任何判断、派生
	// 或披露，渠道墙未降前它读到的还是空册。
	"/label-transactions":                  true,
	"/node-operations-records":             true,
	"/transport-fulfillment-records":       true,
	"/tracking-projections":                true,
	"/pricing-price-cards":                 true,
	"/pricing-reference-series":            true,
	"/pricing-reference-series-coverage":   true,
	"/pricing-pending-series-evaluations":  true,
	"/pricing-evaluations":                 true,
	"/network-catalog":                     true,
	"/route-plans":                         true,
	"/governance-registers":                true,
	"/customs-compliance-rules":            true,
	"/customs-case-registers":              true,
	"/customs-gate-conditions":             true,
	"/customs-ports-paths":                 true,
	"/customs-credentials":                 true,
	"/customs-duty-collaborations":         true,
	"/customs-duty-verifications":          true,
	"/commercial-business-parties":         true,
	"/commercial-service-products":         true,
	"/commercial-policies":                 true,
	"/commercial-customer-contracts":       true,
	"/commercial-supplier-agreements":      true,
	"/commercial-group-legal-entities":     true,
	"/commercial-party-relationships":      true,
	"/commercial-customer-accounts":        true,
	"/commercial-product-channel-mappings": true,
	"/visibility-catalogues":               true,
	"/exception-triage-records":            true,
	"/exception-case-records":              true,
	"/claims-recovery-records":             true,
	"/collection-subledgers":               true,
	"/settlement-charges":                  true,
	"/settlement-statements":               true,
	"/settlement-funds-applications":       true,
	"/settlement-operating-results":        true,
	// 交接范围汇总（票 admin-web-audit-followups/06）与运输履约查阅共用同一个 Intake 变量，
	// 因此启用态必然随它一起放行——漏了这一行等于断言「同一个 Intake 会给出两种答案」，
	// 那是装不出来的形状。它照样满足那三条判据：消费本上下文自己的存储读面、零持久化、
	// 作用域来自运营侧授权结果。第二参是应用读用例而不是读口，判据不因此改口径——那个
	// 用例零登记零编辑零披露，只把成员交给领域派生。
	//
	// 单独列在表尾而不挨着 /transport-fulfillment-records：这个键比表里最长的还长，插进
	// 那一段会让 gofmt 把整段的对齐列一起改掉，而这是一份几个会话都在往里加行的共享文件。
	"/transport-fulfillment-handover-scope-summary": true,
	// 外部承运轨迹事实当前版查阅（票 label-channel/21 读半边）同样共用运输履约查阅的 Intake 变量，启用态
	// 随它一起放行。三条判据逐条满足：消费本上下文自己的存储读面、零持久化、作用域来自运营侧授权结果——
	// 上列「该判哪几条」不触发判断，判断在另一条写行上，而写行挂的是字面量 UnconfiguredIntake{}，本
	// 用例下面那半会证它仍答 403。单列在表尾的理由同上一行。
	"/transport-fulfillment-external-tracking-facts": true,
	// 实际承运商首次有效收寄链查阅（票 label-channel/31 读半边）同样共用运输履约查阅的 Intake 变量，启用态随它一起
	// 放行；判据同上一行：上列一条链不触发判断，判断在另一条写行上，那一行挂的是字面量 UnconfiguredIntake{}。
	"/transport-fulfillment-carrier-first-effective-pickups": true,
	// 渠道择优决定查阅（票 label-channel/23）随委托查阅的注入值一起放行（第三半接口，只交出租户维）。三条判据逐条
	// 满足：消费本上下文自己的存储读面、零持久化、作用域来自运营侧授权结果——列并列冲突不触发任何判断，人工裁决
	// 不在这一口。单列在表尾的理由同上两行。
	"/channel-selection-decisions": true,
	// 责任法人修订历史（票 admin-web-group-legal-entities/03）与商业目录查阅共用同一个 Intake 变量，启用态必然随
	// 它一起放行。三条判据逐条满足：消费本上下文自己的存储读面（同一张 legal_entity_registration 表，只是不取
	// DISTINCT ON）、零持久化、作用域来自运营侧授权结果——法人标识在路径上、租户仍只从注入作用域取。本用例经真
	// 路由打到它并期待 500 而不是 400，顺带钉住 chi 把 {legalEntityId} 填进了 PathValue。单列在表尾的理由同上。
	"/commercial-group-legal-entities/{legalEntityId}/revisions": true,
	// 业务参与方修订历史（票 admin-web-group-legal-entities/12）与商业目录查阅共用同一个 Intake 变量，启用态必然随
	// 它一起放行。三条判据逐条满足：消费本上下文自己的存储读面（同一张 business_party_registration 表，只是不取
	// DISTINCT ON）、零持久化、作用域来自运营侧授权结果——参与方标识在路径上、租户仍只从注入作用域取。本用例经真
	// 路由打到它并期待 500 而不是 400，顺带钉住 chi 把 {partyId} 填进了 PathValue。单列在表尾的理由同上。
	"/commercial-business-parties/{partyId}/revisions": true,
	// 注册号类型目录（票 legal-entity-profile/01）与商业目录查阅共用同一个 Intake 变量，启用态必然随它一起放行。
	// 三条判据逐条满足：消费本上下文自己的存储读面、零持久化、作用域来自运营侧授权结果——上列不判号，判号走
	// ports.RegistrationNumberTypeLookup；登记与停用两条写行挂的是字面量 UnconfiguredIntake{}。单列在表尾的理由同上。
	"/commercial-registration-number-types": true,
	// 法人资料修订历史（票 legal-entity-profile/03）与商业目录查阅共用同一个 Intake 变量，启用态必然随它一起放行。
	// 三条判据逐条满足：消费本上下文自己的存储读面（0036 的资料修订表）、零持久化、作用域来自运营侧授权结果——法人标识
	// 在路径上、租户仍只从注入作用域取；只列修订事实，不做按时点解析。登记写行挂的是字面量 UnconfiguredIntake{}。
	// 经真路由期待 500 而不是 400，同样钉住 chi 把 {legalEntityId} 填进了 PathValue。单列在表尾的理由同上。
	"/commercial-group-legal-entities/{legalEntityId}/profile-revisions": true,
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
