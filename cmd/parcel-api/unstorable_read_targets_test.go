package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

// 本文件证票 document-numbering/15：读口的路径参数或查询值含 NUL、不是合法 UTF-8，在装配处挂的检查里答 400，不进处理器、不读库；
// 合法多字节（中文、全角、emoji）照收。测试输入是隔离合成，只记 `S`。

// legacyMalformedRequest 是检查答的旧形状坏请求的字节；不带 detail 的处理器自己答的那一份也是这一串。
const legacyMalformedRequest = `{"error":{"code":"MALFORMED_REQUEST"}}` + "\n"

// unstorableValues 是库存不下的几种值，百分号转义后放进请求目标：NUL、孤立高位字节、超长编码、代理区码点。
var unstorableValues = map[string]string{
	"NUL":   "SYN%00X",
	"孤立 FF": "SYN%FFX",
	"超长编码":  "SYN%C0%AFX",
	"代理区":   "SYN%ED%A0%80X",
}

// storableMultibyte 是一份合法的多字节值：中文、全角、emoji 各占一段。
const storableMultibyte = "包裹－ＳＹＮ－１📦"

func assertLegacyMalformedRequest(t *testing.T, name string, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusBadRequest || response.Body.String() != legacyMalformedRequest ||
		response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%s：答 %d %q（%s），want 400 %q（application/json）", name, response.Code, response.Body.String(),
			response.Header().Get("Content-Type"), legacyMalformedRequest)
	}
}

// withQueryValue 在请求目标上加一格查询参数，取值原样拼进去（调用方已转义）。
func withQueryValue(target, key, escapedValue string) string {
	separator := "?"
	if strings.Contains(target, "?") {
		separator = "&"
	}
	return target + separator + key + "=" + escapedValue
}

// spiedTrackingIntake 是运营追踪查阅的 Intake 替身：记下被问了几次，答渠道未配置。
type spiedTrackingIntake struct{ asked *int }

func (intake spiedTrackingIntake) IntakeOperationsQuery(context.Context, *http.Request) (visibilityhttp.OperationsTrackingQuery, error) {
	*intake.asked++
	return visibilityhttp.OperationsTrackingQuery{}, visibilityhttp.ErrAccessChannelNotConfigured
}

// spiedViewsIntake 是委托查阅的 Intake 替身，同上。
type spiedViewsIntake struct{ asked *int }

func (intake spiedViewsIntake) IntakeListQuery(context.Context, *http.Request) (shipmenthttp.ShipmentRequestViewsQuery, error) {
	*intake.asked++
	return shipmenthttp.ShipmentRequestViewsQuery{}, shipmenthttp.ErrAccessChannelNotConfigured
}

func (intake spiedViewsIntake) IntakeDetailQuery(context.Context, *http.Request) (shipmenthttp.ShipmentRequestViewQuery, error) {
	*intake.asked++
	return shipmenthttp.ShipmentRequestViewQuery{}, shipmenthttp.ErrAccessChannelNotConfigured
}

// Covers: 票 document-numbering/15 完成判据「每个改过的口补 HTTP 用例」——操作者族取 /tracking-projections（复评点名的
// serveCurrentProjection），隔离读族取 /shipment-request-views：查询值含 NUL 或非法 UTF-8 答 400 旧形状，Intake 一次也没被问，
// 读口（nil）也就够不着；合法多字节照常进 Intake。拒法取登记处为这两行算好的那一份，与生产装配同源。首方客户族读口的同一格由
// visibilityhttp 的 TestUnstorableQueryValuesAreBadRequestsOnBothLines 钉。
func TestUnstorableReadValuesNeverReachTheIntakeOrTheReader(t *testing.T) {
	refusals := unstorableTargetRefusals(declaredEndpointLines())
	var trackingAsked, viewsAsked int
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, guardReadTargets(refusals, []httpapi.BusinessEndpoint{
		{Pattern: "/tracking-projections", Handler: visibilityhttp.NewQueryTrackingProjectionsEndpoint(spiedTrackingIntake{&trackingAsked}, nil)},
		{Pattern: "/shipment-request-views", Handler: shipmenthttp.NewQueryShipmentRequestViewsEndpoint(spiedViewsIntake{&viewsAsked}, nil)},
	}))
	get := func(target string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		return response
	}

	for name, value := range unstorableValues {
		for _, target := range []string{
			"/tracking-projections?parcel=" + value,
			"/tracking-projections?version=" + value,
			"/shipment-request-views?shipmentRequestId=" + value,
			"/shipment-request-views?documentNumber=" + value,
		} {
			assertLegacyMalformedRequest(t, name+" "+target, get(target))
		}
	}
	if trackingAsked != 0 || viewsAsked != 0 {
		t.Fatalf("坏值问到了 Intake：运营追踪 %d 次、委托查阅 %d 次", trackingAsked, viewsAsked)
	}

	multibyte := url.QueryEscape(storableMultibyte)
	for _, target := range []string{"/tracking-projections?parcel=" + multibyte, "/shipment-request-views?documentNumber=" + multibyte} {
		if response := get(target); response.Code != http.StatusForbidden {
			t.Fatalf("合法多字节 %s：答 %d %s，want 进 Intake 答 403", target, response.Code, response.Body.String())
		}
	}
	if trackingAsked != 1 || viewsAsked != 1 {
		t.Fatalf("合法多字节没进 Intake：运营追踪 %d 次、委托查阅 %d 次", trackingAsked, viewsAsked)
	}
}

// Covers: 票 document-numbering/15——检查挂在登记处、不靠逐口记得：登记处的每一行 GET（首方客户族除外）在装配之后对坏目标答 400
// 旧形状，路径参数段也算；同一格换成合法多字节，答复与换成 ASCII 一字不差，检查不误拒。首方客户族的 GET 行与全部命令行不挂：
// 同样的坏目标照旧答渠道未配置——前者先认证、后读查询串，后者不读查询串。
func TestEveryRegisteredReadLineCarriesTheTargetCheck(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnconfiguredBusinessEndpoints())
	serve := func(method, target string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, target, nil))
		return response
	}
	lines := declaredEndpointLines()
	probes := businessEndpointProbes()
	patterns := make([]string, 0, len(lines))
	for pattern := range lines {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)

	guardedReads := 0
	for _, pattern := range patterns {
		line, probe := lines[pattern], probes[pattern]
		guarded := line.method == http.MethodGet && line.face.family != faceCustomerClient
		for name, value := range unstorableValues {
			targets := []string{withQueryValue(probe.target, "probeValue", value)}
			if strings.Contains(pattern, "{") {
				targets = append(targets, withBadPathParameter(pattern, probe.target, value))
			}
			for _, target := range targets {
				response := serve(probe.method, target)
				if guarded {
					assertLegacyMalformedRequest(t, pattern+" "+name+" "+target, response)
					continue
				}
				if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
					t.Fatalf("%s %s %s：答 %d %s，want 不挂检查、照旧 403 渠道未配置", probe.method, name, target, response.Code, response.Body.String())
				}
			}
		}
		if !guarded {
			continue
		}
		guardedReads++
		plain := serve(probe.method, withQueryValue(probe.target, "probeValue", "SYN-PLAIN"))
		multibyte := serve(probe.method, withQueryValue(probe.target, "probeValue", url.QueryEscape(storableMultibyte)))
		if multibyte.Code != plain.Code || multibyte.Body.String() != plain.Body.String() {
			t.Fatalf("%s：合法多字节答 %d %s，ASCII 答 %d %s——检查误拒了", pattern, multibyte.Code, multibyte.Body.String(), plain.Code, plain.Body.String())
		}
	}
	if guardedReads == 0 {
		t.Fatal("一行挂检查的 GET 都没探到")
	}
}

// withBadPathParameter 把探针目标里第一个路径参数段换成坏值。
func withBadPathParameter(pattern, target, value string) string {
	path, query, _ := strings.Cut(target, "?")
	patternSegments, pathSegments := strings.Split(pattern, "/"), strings.Split(path, "/")
	for index, segment := range patternSegments {
		if strings.HasPrefix(segment, "{") {
			pathSegments[index] = value
			break
		}
	}
	rebuilt := strings.Join(pathSegments, "/")
	if query != "" {
		rebuilt += "?" + query
	}
	return rebuilt
}

// unstorableReadCase 是一个口上原样进 SQL 参数的一格：target 里的 {v} 换成取值；取值进查询串还是路径段由 escape 定。
type unstorableReadCase struct {
	pattern string
	target  string
	escape  func(string) string
}

// productionReadRouter 按登记处装上 cases 点名的那几个特性，读口接真库、渠道用授予齐备的合成操作者与隔离读入参。
func productionReadRouter(t *testing.T, cases []unstorableReadCase) http.Handler {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	isolated, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{isolatedReadTenantEnv: "SYN-TENANT-01"}))
	if err != nil {
		t.Fatal(err)
	}
	devices, err := aipg.NewDeviceRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	sources := endpointSources{db: db, operatorMinter: minter, deviceRegistry: devices,
		documentNumbers: documentNumberingComponents(t, db, docnumber.SecretTable{})}
	channels := endpointChannels{isolatedRead: isolated, operatorRegistries: registries}

	wanted := map[string]bool{}
	for _, readCase := range cases {
		wanted[readCase.pattern] = true
	}
	var endpoints []httpapi.BusinessEndpoint
	for _, feature := range endpointRegistry {
		owns := false
		for pattern := range feature.lines {
			owns = owns || wanted[pattern]
		}
		if !owns {
			continue
		}
		mounted, err := feature.production(sources, channels)
		if err != nil {
			t.Fatalf("装配 %T：%v", feature.wiringKey, err)
		}
		endpoints = append(endpoints, mounted...)
	}
	return httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints)
}

// Covers: 票 document-numbering/15 完成判据「cmd 真库测试」——清点里修前答 500 NO_ANSWER_FORMED 的格，每个上下文取一口，经生产
// 装配接真库：NUL 与三种非法 UTF-8 都答 400 旧形状；同一格换成合法多字节，答复与 ASCII 同状态、不是 400 也不是 500。外加一条
// 同形核对：/tracking-projections 处理器自己答的坏请求（两个定位参数同在）与检查答的逐字节相同。
func TestUnstorableReadValuesAreBadRequestsAgainstARealDatabase(t *testing.T) {
	query, path := url.QueryEscape, url.PathEscape
	cases := []unstorableReadCase{
		{"/tracking-projections", "/tracking-projections?parcel={v}", query},
		{"/shipment-request-views", "/shipment-request-views?documentNumber={v}", query},
		{"/commercial-business-parties/{partyId}/revisions", "/commercial-business-parties/{v}/revisions", path},
		{"/commercial-customer-accounts", "/commercial-customer-accounts?q={v}", query},
		{"/customs-case-records", "/customs-case-records?case={v}", query},
		{"/network-catalog", "/network-catalog?family=node&q={v}", query},
		{"/node-operations-delivery-pre-advices/{preAdviceId}", "/node-operations-delivery-pre-advices/{v}", path},
		{"/pricing-price-cards/content", "/pricing-price-cards/content?id={v}&version=v1", query},
		{"/settlement-statement-lines", "/settlement-statement-lines?statement={v}", query},
		{"/transport-fulfillment-external-tracking-facts", "/transport-fulfillment-external-tracking-facts?source={v}&view=pending", query},
	}
	router := productionReadRouter(t, cases)
	get := func(target string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	for _, readCase := range cases {
		for name, value := range unstorableValues {
			target := strings.Replace(readCase.target, "{v}", value, 1)
			assertLegacyMalformedRequest(t, name+" "+target, get(target))
		}
		plain := get(strings.Replace(readCase.target, "{v}", readCase.escape("SYN-PROBE-01"), 1))
		multibyte := get(strings.Replace(readCase.target, "{v}", readCase.escape(storableMultibyte), 1))
		if plain.Code == http.StatusBadRequest || plain.Code >= http.StatusInternalServerError || multibyte.Code != plain.Code {
			t.Fatalf("%s：ASCII 答 %d %s，合法多字节答 %d %s；want 同状态、读得到库", readCase.pattern,
				plain.Code, plain.Body.String(), multibyte.Code, multibyte.Body.String())
		}
	}

	own := get("/tracking-projections?parcel=SYN-PROBE-01&version=SYN-PROBE-01")
	assertLegacyMalformedRequest(t, "处理器自己的坏请求", own)
}
