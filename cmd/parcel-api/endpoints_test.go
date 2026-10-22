package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 本文件是端点登记处的测试侧（ADR-0222）：装配测试展开的是生产那一张登记表、那一个 mount，只把各特性的依赖换成各自
// endpoints_<特性>_test.go 登记的未接线占位。各张换口清单由登记处的行声明导出，不再手列。

// unwiredWirings 按依赖结构的类型（(*D)(nil)）存各特性的未接线占位。
var unwiredWirings = map[any]any{}

// registerUnwired 由各特性的 endpoints_<特性>_test.go 在 init 里调用。占位交回稳定错误：走到它们的请求如实答
// NO_ANSWER_FORMED（分辨见 unwired_orchestration.go 的文件注释），而不是 nil 接口被调后由 Recoverer 兜成一个没有 code 的 500。
func registerUnwired[D any](wiring D) {
	key := any((*D)(nil))
	if _, registered := unwiredWirings[key]; registered {
		panic(fmt.Sprintf("parcel-api: unwired wiring %T registered twice", key))
	}
	unwiredWirings[key] = wiring
}

// assembleUnwiredEndpoints 按登记处逐特性挂上未接线占位。少登一份占位就当场停：那个特性的行会从装配测试里整片消失。
func assembleUnwiredEndpoints(channels endpointChannels) []httpapi.BusinessEndpoint {
	var endpoints []httpapi.BusinessEndpoint
	for _, feature := range endpointRegistry {
		wiring, registered := unwiredWirings[feature.wiringKey]
		if !registered {
			panic(fmt.Sprintf("parcel-api: no unwired wiring for %T; register one in that feature's endpoints_<feature>_test.go", feature.wiringKey))
		}
		endpoints = append(endpoints, feature.mountWith(channels, wiring)...)
	}
	return endpoints
}

// declaredEndpointLines 收登记处各特性声明的行。同一路径在两个特性里各声明一次是编程错误。
func declaredEndpointLines() map[string]endpointLine {
	lines := map[string]endpointLine{}
	for _, feature := range endpointRegistry {
		for pattern, line := range feature.lines {
			if _, declared := lines[pattern]; declared {
				panic(fmt.Sprintf("parcel-api: endpoint %s declared by two features", pattern))
			}
			lines[pattern] = line
		}
	}
	return lines
}

// declaredPatterns 交回满足 keep 的行的路径，按路径排序：报错稳定可读。
func declaredPatterns(keep func(endpointLine) bool) []string {
	var patterns []string
	for pattern, line := range declaredEndpointLines() {
		if keep(line) {
			patterns = append(patterns, pattern)
		}
	}
	sort.Strings(patterns)
	return patterns
}

// businessEndpointProbe 是一行端点的有效探测请求。
//
// 方法与必填分派参数必须逐项写对：拿错方法测出来的 405、漏掉 family/registry/kind
// 测出来的 400 都会盖过未配置 Intake 的 403，断言就什么也没守住。它们取自登记处的行声明。
type businessEndpointProbe struct {
	method string
	target string
}

// businessEndpointProbes 是本进程应当服务的全部业务端点及各自的有效探测请求，由登记处的行声明导出。它与各特性的 mount
// 互为对照（TestEveryRegisteredFeatureMountsExactlyItsDeclaredLines）：声明里多一行说明装配漏了一个端点（那一行会退回
// 路由层的 404，正是 ADR-0055 要治的折叠），mount 里多一行说明上线了一个没人钉过形状的面。
func businessEndpointProbes() map[string]businessEndpointProbe {
	probes := map[string]businessEndpointProbe{}
	for pattern, line := range declaredEndpointLines() {
		target := line.probe
		if target == "" {
			target = pattern
		}
		probes[pattern] = businessEndpointProbe{method: line.method, target: target}
	}
	return probes
}

// Covers: ADR-0222 决定三 — 每个特性 mount 出的路径恰是它声明的那几行，两个方向都不许多。
func TestEveryRegisteredFeatureMountsExactlyItsDeclaredLines(t *testing.T) {
	channels := unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
	for _, feature := range endpointRegistry {
		wiring, registered := unwiredWirings[feature.wiringKey]
		if !registered {
			t.Fatalf("特性 %T 没有登记未接线占位", feature.wiringKey)
		}
		mounted := map[string]bool{}
		for _, endpoint := range feature.mountWith(channels, wiring) {
			if _, declared := feature.lines[endpoint.Pattern]; !declared {
				t.Fatalf("特性 %T 挂了未声明的端点 %s：形状没有任何测试钉住", feature.wiringKey, endpoint.Pattern)
			}
			if mounted[endpoint.Pattern] {
				t.Fatalf("特性 %T 把 %s 挂了两次：后一个会静默盖掉前一个", feature.wiringKey, endpoint.Pattern)
			}
			mounted[endpoint.Pattern] = true
		}
		for pattern := range feature.lines {
			if !mounted[pattern] {
				t.Fatalf("特性 %T 声明了 %s 却没挂：它会答 404，与「产品没有这个能力」不可分辨", feature.wiringKey, pattern)
			}
		}
	}
	if len(unwiredWirings) != len(endpointRegistry) {
		t.Fatalf("登记了 %d 份未接线占位，登记处只有 %d 个特性：有占位对不上任何特性", len(unwiredWirings), len(endpointRegistry))
	}
}

// Covers: ADR-0055 第一、二、三条 — 端点已装配、未配置自成一格、状态码取 403。
//
// 这是唯一证明「装配确实发生了」的地方：各上下文的传输层测试拿自己构造的处理器跑，
// 装不装配它们都绿。这里走的是 cmd/parcel-api 真正交给 http.Server 的那个路由。
func TestEveryAssembledEndpointAnswersUnconfigured(t *testing.T) {
	// 传 unwired* 占位而非真编排与真读口：本测试钉的是未配置面（403 在编排之前），
	// 真编排的装配与行为由各 assemble_*_test.go 对真库另证。
	endpoints := assembleUnconfiguredBusinessEndpoints()
	assertEveryDeclaredLineAnswersUnconfigured(t, endpoints, nil)
}

// assertEveryDeclaredLineAnswersUnconfigured 逐行探一遍，prepare 非 nil 时先给请求补头。
func assertEveryDeclaredLineAnswersUnconfigured(t *testing.T, endpoints []httpapi.BusinessEndpoint, prepare func(*http.Request, endpointLine)) {
	t.Helper()
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints)
	probes := businessEndpointProbes()
	lines := declaredEndpointLines()

	mounted := make(map[string]bool, len(endpoints))
	for _, endpoint := range endpoints {
		probe, listed := probes[endpoint.Pattern]
		if !listed {
			t.Fatalf("装配了未登记的端点 %s：形状没有任何测试钉住", endpoint.Pattern)
		}
		if mounted[endpoint.Pattern] {
			t.Fatalf("端点 %s 装配了两次：后一个会静默盖掉前一个", endpoint.Pattern)
		}
		mounted[endpoint.Pattern] = true

		request := httptest.NewRequest(probe.method, probe.target, nil)
		if prepare != nil {
			prepare(request, lines[endpoint.Pattern])
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s: status = %d, want %d", probe.method, probe.target, response.Code, http.StatusForbidden)
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", endpoint.Pattern, got)
		}
		assertNoOutcome(t, response, endpoint.Pattern)
	}

	for pattern := range probes {
		if !mounted[pattern] {
			t.Fatalf("端点 %s 没进装配：它会答 404，与「产品没有这个能力」不可分辨", pattern)
		}
	}
}

// Covers: ADR-0222 决定三 — 行声明的接入面与 mount 挂的 Intake 是同一件事：逐个渠道单独打开，答复离开「接入渠道未配置」
// 的恰是声明了那一面、且 Intake 经共享渠道来的行。Intake 由特性自建的行在装配测试里恒为未配置，各自的特性测试另证。
//
// 只看「离没离开未配置」，不看离开后答哪一格：各面的答复格由各自那条换口用例钉。
func TestEachLineOpensOnlyOnItsDeclaredAccessFace(t *testing.T) {
	isolated, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{isolatedReadTenantEnv: "SYN-TENANT-01"}))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	openCustomer := customerStandingRegister{start: now.Add(-time.Hour), end: now.Add(time.Hour)}
	operatorMinter, err := buildOperatorMinter(acceptingVerifier{}, readerRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	operatorDecisions, err := buildOperatorDecisionIntakes(operatorMinter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	operatorRegistries, err := buildOperatorRegistryIntakes(operatorMinter)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		router  http.Handler
		opens   map[faceFamily]bool
		prepare func(*http.Request, endpointLine)
	}{
		{
			name:   "isolated read",
			router: httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpoints(isolated)),
			opens:  map[faceFamily]bool{faceIsolatedRead: true},
		},
		{
			name: "first-party customer",
			router: httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWith(nil,
				customerSubmissionWith(t, fixedCustomerVerifier{}, openCustomer))),
			opens: map[faceFamily]bool{faceCustomerClient: true},
		},
		{
			name: "operator",
			router: httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(
				operatorDecisions, operatorRegistries, unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)),
			opens: map[faceFamily]bool{faceOperationsRead: true, faceRegistryWrite: true, faceOperationDecision: true},
		},
		{
			name:   "integration client",
			router: integrationClientRouter(t, acceptingClientVerifier{}, absentClientRegister{}),
			opens:  map[faceFamily]bool{faceIntegrationClient: true},
		},
		{
			name:    "work fact",
			router:  workFactOperatorRouter(t, acceptingVerifier{}, workFactOperators{}, workFactDevices{}),
			opens:   map[faceFamily]bool{faceWorkFact: true},
			prepare: stampWorkScope,
		},
	}
	lines := declaredEndpointLines()
	for _, testCase := range cases {
		for pattern, probe := range businessEndpointProbes() {
			line := lines[pattern]
			request := httptest.NewRequest(probe.method, probe.target, strings.NewReader(`{}`))
			if testCase.prepare != nil {
				testCase.prepare(request, line)
			}
			request.Header.Set("Authorization", "Bearer presented.token")
			response := httptest.NewRecorder()
			testCase.router.ServeHTTP(response, request)
			stillUnconfigured := response.Code == http.StatusForbidden && problemCode(t, response) == "ACCESS_CHANNEL_NOT_CONFIGURED"
			wantOpen := testCase.opens[line.face.family] && !line.ownIntake
			if wantOpen == stillUnconfigured {
				t.Fatalf("%s 打开时 %s（声明 %s，自建 Intake=%t）答 %d %s",
					testCase.name, pattern, line.face, line.ownIntake, response.Code, response.Body.String())
			}
		}
	}
}

// stampWorkScope 给请求带上令牌与作业范围三个头：作业事实各口要带齐才走得到认证；不是作业事实的行按运输履约那一组带，
// 它们不读这三个头。
func stampWorkScope(request *http.Request, line endpointLine) {
	kind := line.face.workScope
	if kind == "" {
		kind = accessidentity.WorkScopeOffsite
	}
	stampWorkFact(request, string(kind), workScopeFixtures[kind].reference, workScopeFixtures[kind].device)
}

// Covers: ADR-0222 Consequences「生产装配第一次有了真库用例」 — 生产装配对真库整张接得起来：未配置的部署环境下每一行
// 都在、都答未配置。改前生产装配只在进程启动时走得到。
//
// 请求带齐令牌与作业范围头：生产上作业事实渠道的 Intake 恒在（发行方未配置时核验方答未配置），缺头的请求先在译头那一步答
// 400，到不了核验方。装配测试里那一格是字面量未配置，不读头。
func TestProductionAssemblyMountsEveryDeclaredLine(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	unset := fakeGetenv(nil)
	operatorVerifier, _, err := buildOperatorCredentialVerifier(unset)
	if err != nil {
		t.Fatal(err)
	}
	integrationClientVerifier, _, err := buildIntegrationClientCredentialVerifier(unset)
	if err != nil {
		t.Fatal(err)
	}
	customerClientVerifier, _, err := buildCustomerClientCredentialVerifier(unset)
	if err != nil {
		t.Fatal(err)
	}
	endpoints, err := assembleProductionEndpoints(db, startupChannels{
		operatorVerifier:          operatorVerifier,
		integrationClientVerifier: integrationClientVerifier,
		customerClientVerifier:    customerClientVerifier,
	})
	if err != nil {
		t.Fatalf("生产装配：%v", err)
	}
	assertEveryDeclaredLineAnswersUnconfigured(t, endpoints, stampWorkScope)
}

// Covers: ADR-0222 决定六 — 同一路径登两次，装配交回错误而不是让后一个静默盖掉前一个。
func TestRepeatedPatternsAreRefused(t *testing.T) {
	endpoints := []httpapi.BusinessEndpoint{{Pattern: "/claims"}, {Pattern: "/claims"}}
	if err := refuseRepeatedPatterns(endpoints); err == nil {
		t.Fatal("same pattern twice accepted")
	}
}

// Covers: ADR-0055 「未配置格住在 Intake 缝里，不在路由层另设闸」 — 未配置不改变方法
// 约束：方法不对仍由处理器自己答 405，403 不越过它抢答。两处各有权威就会各改一次。
func TestUnconfiguredDoesNotSwallowTheMethodGate(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnconfiguredBusinessEndpoints())

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/v1/shipment-requests", nil))

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
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnconfiguredBusinessEndpoints())

	baseline := httptest.NewRecorder()
	router.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/v1/shipment-requests", nil))

	reported := httptest.NewRequest(http.MethodPost, "/v1/shipment-requests", nil)
	reported.Header.Set("X-Reported-Tenant", "TENANT-9")
	reported.Header.Set("Authorization", "Bearer whatever")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, reported)

	// requestId 是路由中间件逐请求生成的对账句柄（ADR-0140 决定三），两次本来就不同；除它之外须逐字相同。
	if response.Code != baseline.Code || withoutRequestID(t, response) != withoutRequestID(t, baseline) {
		t.Fatalf("answer differs from baseline: %d %s vs %d %s",
			response.Code, response.Body.String(), baseline.Code, baseline.Body.String())
	}
}

func withoutRequestID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
	delete(fields, "requestId")
	canonical, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("re-encode response: %v", err)
	}
	return string(canonical)
}

func assembleUnconfiguredBusinessEndpoints() []httpapi.BusinessEndpoint {
	return assembleUnwiredBusinessEndpoints(nil)
}

// assembleUnwiredBusinessEndpoints 以 unwired* 占位读口与编排装配全部端点，隔离读面
// 输入由调用方给：nil 钉「未配置面」，非 nil 钉「放行只及运营查阅行」（isolated_read_test.go）。
//
// 提交口的首方 Intake 恒为 nil：**读开关换不了写行**是 ADR-0091 决定四的形状，而这个
// 辅助函数正是隔离读那组用例的入口——它若把读与首方入参并成一个，那组用例就再也证不了这件事。
// 首方族在提交口的各格另有 assemble_customer_client_test.go。
func assembleUnwiredBusinessEndpoints(isolatedRead *isolatedReadIntakes) []httpapi.BusinessEndpoint {
	return assembleUnwiredBusinessEndpointsWith(isolatedRead, nil)
}

func assembleUnwiredBusinessEndpointsWith(
	isolatedRead *isolatedReadIntakes,
	customerSubmission shipmenthttp.SubmissionIntake,
) []httpapi.BusinessEndpoint {
	return assembleUnwiredBusinessEndpointsWithOperatorIntakes(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(),
		unconfiguredIntegrationClientIntakes(),
		isolatedRead, customerSubmission, nil, nil)
}

// assembleUnwiredBusinessEndpointsWithOperatorIntakes 同上，另收操作者渠道各口的 Intake：换口那几行的答复格由它钉。
func assembleUnwiredBusinessEndpointsWithOperatorIntakes(
	operatorDecisions operatorDecisionIntakes,
	operatorRegistries operatorRegistryIntakes,
	integrationClients integrationClientIntakes,
	isolatedRead *isolatedReadIntakes,
	customerSubmission shipmenthttp.SubmissionIntake,
	workFactReception nodeopshttp.ReceptionIntake,
	workFactTransport *tfhttp.WorkFactCommandIntake,
) []httpapi.BusinessEndpoint {
	return assembleUnwiredEndpoints(unwiredChannels(operatorDecisions, operatorRegistries, integrationClients,
		isolatedRead, customerSubmission, workFactReception, workFactTransport))
}

func unwiredChannels(
	operatorDecisions operatorDecisionIntakes,
	operatorRegistries operatorRegistryIntakes,
	integrationClients integrationClientIntakes,
	isolatedRead *isolatedReadIntakes,
	customerSubmission shipmenthttp.SubmissionIntake,
	workFactReception nodeopshttp.ReceptionIntake,
	workFactTransport *tfhttp.WorkFactCommandIntake,
) endpointChannels {
	return endpointChannels{
		isolatedRead:       isolatedRead,
		customerSubmission: customerSubmission,
		operatorDecisions:  operatorDecisions,
		operatorRegistries: operatorRegistries,
		integrationClients: integrationClients,
		workFactReception:  workFactReception,
		workFactTransport:  workFactTransport,
	}
}

// problemCode 两种问题体都认：首方族与 PS 客户面命令口是 RFC 9457 问题详情、`code` 在顶层（ADR-0140 决定三），其余各族仍是
// `{"error":{"code"}}`，迁到问题详情之前两种并存。
func problemCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code  string `json:"code"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem response %s: %v", response.Body.Bytes(), err)
	}
	if body.Code != "" {
		return body.Code
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
