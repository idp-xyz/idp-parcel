package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	warehousedomain "go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	warehouseports "go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本特性的未接线占位是开关打开的那一份：占位登记的是装配测试里「别的登记写行、查阅行怎么答，仓配的行就怎么答」的基线，既有的
// 各条换口、请求体检查与目标检查用例因此不改一条断言就把仓配的行也核到。开关关着的那一格（缺省，生产装配的基线）由本文件下面的
// 用例与 TestProductionAssemblyMountsEveryDeclaredLine 钉。

type unwiredWarehouseRegistration struct{}

func (unwiredWarehouseRegistration) Register(
	context.Context,
	warehouseapp.RegisterWarehouseCommand,
) (warehouseapp.RegisterWarehouseResult, error) {
	return warehouseapp.RegisterWarehouseResult{}, errOrchestrationNotWired
}

type unwiredWarehouseCatalogue struct{}

func (unwiredWarehouseCatalogue) ListWarehouses(
	context.Context,
	warehousedomain.TenantID,
	int,
	cataloguepage.Query,
) (cataloguepage.Result[warehouseports.WarehouseRow], error) {
	return cataloguepage.Result[warehouseports.WarehouseRow]{}, errOrchestrationNotWired
}

func (unwiredWarehouseCatalogue) WarehouseRevisions(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.WarehouseCode,
) ([]warehouseports.WarehouseRow, error) {
	return nil, errOrchestrationNotWired
}

type unwiredZoneRegistration struct{}

func (unwiredZoneRegistration) Register(
	context.Context,
	warehouseapp.RegisterZoneCommand,
) (warehouseapp.RegisterZoneResult, error) {
	return warehouseapp.RegisterZoneResult{}, errOrchestrationNotWired
}

type unwiredZoneCatalogue struct{}

func (unwiredZoneCatalogue) ListZones(
	context.Context,
	warehousedomain.TenantID,
	int,
	cataloguepage.Query,
) (cataloguepage.Result[warehouseports.ZoneRow], error) {
	return cataloguepage.Result[warehouseports.ZoneRow]{}, errOrchestrationNotWired
}

func (unwiredZoneCatalogue) ZoneRevisions(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.WarehouseCode,
	warehousedomain.ZoneCode,
) ([]warehouseports.ZoneRow, error) {
	return nil, errOrchestrationNotWired
}

func unwiredWarehouseFulfillment(enabled bool) warehouseFulfillmentWiring {
	return warehouseFulfillmentWiring{
		enabled:          enabled,
		registration:     unwiredWarehouseRegistration{},
		catalogue:        unwiredWarehouseCatalogue{},
		revisions:        unwiredWarehouseCatalogue{},
		zoneRegistration: unwiredZoneRegistration{},
		zoneCatalogue:    unwiredZoneCatalogue{},
		zoneRevisions:    unwiredZoneCatalogue{},
	}
}

func init() {
	registerUnwired(unwiredWarehouseFulfillment(true))
}

const (
	warehouseRegistrationsLine = "/warehouse-fulfillment-warehouse-registrations"
	warehouseListLine          = "/warehouse-fulfillment-warehouses"
	warehouseRevisionsLine     = "/warehouse-fulfillment-warehouse-revisions"
	zoneRegistrationsLine      = "/warehouse-fulfillment-zone-registrations"
	zoneListLine               = "/warehouse-fulfillment-zones"
	zoneRevisionsLine          = "/warehouse-fulfillment-zone-revisions"
)

// warehouseFulfillmentLines 是登记处里声明了仓配开关的各行，按路径排序。
func warehouseFulfillmentLines() []string {
	return declaredPatterns(func(line endpointLine) bool { return line.warehouseFulfillment })
}

// warehouseFulfillmentRouter 只挂仓配特性的各行（经登记处的 mountWith，请求体与目标检查照挂），接入渠道与开关由调用方给。
func warehouseFulfillmentRouter(t *testing.T, channels endpointChannels, wiring warehouseFulfillmentWiring) http.Handler {
	t.Helper()
	key := any((*warehouseFulfillmentWiring)(nil))
	for _, feature := range endpointRegistry {
		if feature.wiringKey == key {
			return httpapi.NewWithEndpoints(buildinfo.Info{}, feature.mountWith(channels, wiring))
		}
	}
	t.Fatal("登记处没有仓配履约这个特性")
	return nil
}

// probeWarehouseFulfillment 打仓配的一行：令牌可缺，请求体取 body（GET 行不带）。
func probeWarehouseFulfillment(router http.Handler, pattern, token, body string) *httptest.ResponseRecorder {
	probe := businessEndpointProbes()[pattern]
	var request *http.Request
	if probe.method == http.MethodGet {
		request = httptest.NewRequest(probe.method, probe.target, nil)
	} else {
		request = httptest.NewRequest(probe.method, probe.target, strings.NewReader(body))
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func leftUnconfigured(t *testing.T, response *httptest.ResponseRecorder) bool {
	t.Helper()
	return response.Code == http.StatusForbidden && problemCode(t, response) == "ACCESS_CHANNEL_NOT_CONFIGURED"
}

// Covers: AT-WF-010、AT-WF-012——登记处里仓配的行恰是声明了开关的那几行，接入面是登记册配置写（登记口）与主数据与运营查阅读
// （各读口）：声明错了，换口与闭合用例会红，这一条先点名。
func TestTheWarehouseFulfillmentLinesDeclareTheirFacesAndTheSwitch(t *testing.T) {
	want := map[string]struct {
		method string
		face   faceFamily
	}{
		warehouseRegistrationsLine: {http.MethodPost, faceRegistryWrite},
		warehouseListLine:          {http.MethodGet, faceOperationsRead},
		warehouseRevisionsLine:     {http.MethodGet, faceOperationsRead},
		zoneRegistrationsLine:      {http.MethodPost, faceRegistryWrite},
		zoneListLine:               {http.MethodGet, faceOperationsRead},
		zoneRevisionsLine:          {http.MethodGet, faceOperationsRead},
	}
	lines := declaredEndpointLines()
	if got := warehouseFulfillmentLines(); len(got) != len(want) {
		t.Fatalf("声明了仓配开关的行 = %v，want 恰是 %d 行：%v", got, len(want), want)
	}
	for pattern, expected := range want {
		line, declared := lines[pattern]
		if !declared || !line.warehouseFulfillment || line.method != expected.method || line.face.family != expected.face || line.ownIntake {
			t.Fatalf("%s 的声明 = %+v，want %s %s、声明仓配开关、经共享渠道", pattern, line, expected.method, expected.face)
		}
	}
}

// Covers: AT-WF-012（端点那一半）、ADR-0222 决定三——开关开着时，闭合用例对仓配的行也核到：渠道都不配，仓配各行答「接入渠道未配置」；
// 只打开操作者渠道，各行的答复离开「接入渠道未配置」；打开别的渠道（隔离读、首方客户、集成客户端），各行仍是「接入渠道未配置」——
// 仓配的行不消费它们。只看离没离开未配置，离开后答哪一格由 AT-WF-010 那一条钉。
func TestWarehouseFulfillmentLinesOpenOnlyOnTheOperatorChannelWhenSwitchedOn(t *testing.T) {
	isolated, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{isolatedReadTenantEnv: "SYN-TENANT-01"}))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	customer := customerSubmissionWith(t, fixedCustomerVerifier{}, customerStandingRegister{start: now.Add(-time.Hour), end: now.Add(time.Hour)})
	clientMinter, err := buildIntegrationClientMinter(acceptingClientVerifier{}, absentClientRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	clients, err := buildIntegrationClientIntakes(clientMinter)
	if err != nil {
		t.Fatal(err)
	}
	operatorOpen := registriesFor(t, everyOperatorGrant{})

	nothing := unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
	cases := []struct {
		name     string
		channels endpointChannels
		opens    bool
	}{
		{"nothing configured", nothing, false},
		{"isolated read", unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), isolated, nil, nil, nil), false},
		{"first-party customer", unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), nil, customer, nil, nil), false},
		{"integration client", unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), clients, nil, nil, nil, nil), false},
		{"operator", unwiredChannels(unconfiguredOperatorDecisions(), operatorOpen, unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil), true},
	}
	for _, testCase := range cases {
		router := warehouseFulfillmentRouter(t, testCase.channels, unwiredWarehouseFulfillment(true))
		for _, pattern := range warehouseFulfillmentLines() {
			response := probeWarehouseFulfillment(router, pattern, presentedOperatorToken, `{}`)
			if got := leftUnconfigured(t, response); got == testCase.opens {
				t.Fatalf("%s 打开、开关开着：%s 答 %d %s（离开了「接入渠道未配置」=%t，want %t）",
					testCase.name, pattern, response.Code, response.Body.String(), !got, testCase.opens)
			}
			assertNoOutcome(t, response, pattern)
		}
	}
}

// Covers: AT-WF-010（仓的部分）——操作者渠道打开后，仓配各行按各自声明的面认授予：只持登记册配置写的操作者，登记口越过授予那一格
// （答坏载荷，而不是未授予），各查阅口答 OPERATOR_NOT_GRANTED；只持查阅读的反过来：登记口答未授予，查阅口越过授予走到读口
// （这里接的是未接线读口，答 NO_ANSWER_FORMED）。无令牌答凭据被拒。
func TestWarehouseFulfillmentLinesAskForTheGrantOfTheirOwnFace(t *testing.T) {
	type answer struct {
		status int
		code   string
	}
	same := func(status int, code string) map[string]answer {
		return map[string]answer{
			warehouseRegistrationsLine: {status, code}, warehouseListLine: {status, code}, warehouseRevisionsLine: {status, code},
			zoneRegistrationsLine: {status, code}, zoneListLine: {status, code}, zoneRevisionsLine: {status, code},
		}
	}
	cases := []struct {
		name     string
		register accessidentity.OperatorRegistry
		token    string
		want     map[string]answer
	}{
		{"write grant only", registryWriterRegister{}, presentedOperatorToken, map[string]answer{
			warehouseRegistrationsLine: {http.StatusBadRequest, "MALFORMED_REQUEST"},
			warehouseListLine:          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			warehouseRevisionsLine:     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			zoneRegistrationsLine:      {http.StatusBadRequest, "MALFORMED_REQUEST"},
			zoneListLine:               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			zoneRevisionsLine:          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		}},
		{"read grant only", operationsReaderRegister{}, presentedOperatorToken, map[string]answer{
			warehouseRegistrationsLine: {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			warehouseListLine:          {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			warehouseRevisionsLine:     {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			zoneRegistrationsLine:      {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			zoneListLine:               {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			zoneRevisionsLine:          {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
		}},
		{"no bearer token", everyOperatorGrant{}, "", same(http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED")},
		{"register is down", failingRegister{}, presentedOperatorToken, same(http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE")},
	}
	for _, testCase := range cases {
		channels := unwiredChannels(unconfiguredOperatorDecisions(), registriesFor(t, testCase.register), unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
		router := warehouseFulfillmentRouter(t, channels, unwiredWarehouseFulfillment(true))
		for pattern, want := range testCase.want {
			response := probeWarehouseFulfillment(router, pattern, testCase.token, `{}`)
			if response.Code != want.status || problemCode(t, response) != want.code {
				t.Fatalf("%s, %s：答 %d %s，want %d %s", testCase.name, pattern, response.Code, response.Body.String(), want.status, want.code)
			}
			assertNoOutcome(t, response, pattern)
		}
	}
}

// Covers: AT-WF-012（端点那一半）——开关关着时仓配各行都答 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED，不是 404，也不是「接入渠道未配置」，
// 不带 outcome；与渠道配没配无关（全不配、操作者渠道授予齐备且带令牌，答复逐字相同，requestId 除外）；方法不对仍答 405 并带本行
// 声明的方法（与开关开着时同一份）。
func TestSwitchedOffWarehouseFulfillmentLinesAnswerNotEnabledWhateverTheChannels(t *testing.T) {
	nothing := unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
	everything := unwiredChannels(unconfiguredOperatorDecisions(), registriesFor(t, everyOperatorGrant{}), unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
	closed, open := warehouseFulfillmentRouter(t, nothing, unwiredWarehouseFulfillment(false)), warehouseFulfillmentRouter(t, everything, unwiredWarehouseFulfillment(false))

	lines := declaredEndpointLines()
	for _, pattern := range warehouseFulfillmentLines() {
		bare := probeWarehouseFulfillment(closed, pattern, "", "")
		granted := probeWarehouseFulfillment(open, pattern, presentedOperatorToken, `{"tenantId":"SYN-TENANT-02"}`)
		for name, response := range map[string]*httptest.ResponseRecorder{"nothing configured": bare, "operator channel fully open": granted} {
			if response.Code != http.StatusForbidden || problemCode(t, response) != "WAREHOUSE_FULFILLMENT_NOT_ENABLED" {
				t.Fatalf("%s，%s：答 %d %s，want 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED", name, pattern, response.Code, response.Body.String())
			}
			if response.Header().Get("Content-Type") != httpapi.ProblemMediaType {
				t.Fatalf("%s，%s：Content-Type = %q，want %s", name, pattern, response.Header().Get("Content-Type"), httpapi.ProblemMediaType)
			}
			assertNoOutcome(t, response, pattern)
		}
		if withoutRequestID(t, bare) != withoutRequestID(t, granted) {
			t.Fatalf("%s：开关关着的答复随渠道变了：%s 与 %s", pattern, bare.Body.String(), granted.Body.String())
		}

		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
			response := httptest.NewRecorder()
			closed.ServeHTTP(response, httptest.NewRequest(method, pattern, nil))
			switch {
			case response.Code == http.StatusNotFound:
				t.Fatalf("%s %s：开关关着不得答 404", method, pattern)
			case method == lines[pattern].method:
			case response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != lines[pattern].method:
				t.Fatalf("%s %s：答 %d（Allow=%q），want 405 并带 %s", method, pattern, response.Code, response.Header().Get("Allow"), lines[pattern].method)
			}
		}
	}
}

// Covers: AT-WF-012（端点那一半）——关着时同一装配里别的行的答复与开关无关：本特性以外的行，在仓配开关关着与开着的两种装配下答复
// 逐字相同（拿一行不属于仓配的登记写行与一行查阅行比）。
func TestOtherLinesAnswerTheSameWhetherOrNotWarehouseFulfillmentIsSwitchedOn(t *testing.T) {
	channels := unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
	for _, pattern := range []string{"/commercial-registration-number-type-registrations", "/commercial-registration-number-types"} {
		line := declaredEndpointLines()[pattern]
		if line.warehouseFulfillment {
			t.Fatalf("%s 不该声明仓配开关", pattern)
		}
		var answers []string
		for _, enabled := range []bool{false, true} {
			endpoints := assembleUnwiredEndpointsReplacing(channels, unwiredWarehouseFulfillment(enabled))
			response := httptest.NewRecorder()
			request := httptest.NewRequest(line.method, businessEndpointProbes()[pattern].target, strings.NewReader(`{}`))
			httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints).ServeHTTP(response, request)
			answers = append(answers, withoutRequestID(t, response))
			if !leftUnconfigured(t, response) {
				t.Fatalf("%s：答 %d %s，want 起步的「接入渠道未配置」", pattern, response.Code, response.Body.String())
			}
		}
		if answers[0] != answers[1] {
			t.Fatalf("%s：开关关着与开着答复不同：%s 与 %s", pattern, answers[0], answers[1])
		}
	}
}

// assembleUnwiredEndpointsReplacing 同 assembleUnwiredEndpoints，只把仓配特性的占位换成给定的一份。
func assembleUnwiredEndpointsReplacing(channels endpointChannels, wiring warehouseFulfillmentWiring) []httpapi.BusinessEndpoint {
	var endpoints []httpapi.BusinessEndpoint
	replaced := any((*warehouseFulfillmentWiring)(nil))
	for _, feature := range endpointRegistry {
		placeholder := unwiredWirings[feature.wiringKey]
		if feature.wiringKey == replaced {
			placeholder = wiring
		}
		endpoints = append(endpoints, feature.mountWith(channels, placeholder)...)
	}
	return endpoints
}

// Covers: ADR-0375 决定四「关着时不构造任何仓配的适配器与编排」——开关关着时特性的 build 不碰库：db 传 nil 也装得起来，装出的
// 依赖没有任何编排；开着时 build 要构造仓配的适配器，db 为空装不起来（证明上面「装得起来」不是因为 build 什么都不做）。
func TestTheSwitchedOffFeatureBuildsNoAdaptersAndTheSwitchedOnOneNeedsTheDatabase(t *testing.T) {
	var feature registeredEndpoints
	for _, registered := range endpointRegistry {
		if registered.wiringKey == any((*warehouseFulfillmentWiring)(nil)) {
			feature = registered
		}
	}
	if feature.production == nil {
		t.Fatal("登记处没有仓配履约这个特性")
	}

	endpoints, err := feature.production(endpointSources{warehouseFulfillment: false}, endpointChannels{})
	if err != nil {
		t.Fatalf("开关关着、db 为空：%v，want 装得起来", err)
	}
	if len(endpoints) != len(warehouseFulfillmentLines()) {
		t.Fatalf("开关关着挂出 %d 个端点，want 与声明的行数一致", len(endpoints))
	}
	if _, err := feature.production(endpointSources{warehouseFulfillment: true}, endpointChannels{}); err == nil {
		t.Fatal("开关开着、db 为空装起来了：build 没有构造仓配的适配器")
	}
}
