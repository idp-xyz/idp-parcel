package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// workFactOperatorFace 是换上作业事实登记能力面的一口：首登与更正口同一份认证与授予（票 operator-channel/10 做什么
// 「主链事实的更正口随原事实归本能力面」）。
type workFactOperatorFace struct {
	pattern   string
	scopeKind string
	scopeRef  string
	device    string
	body      string
}

// workScopeFixture 是替身操作者册与设备册认得的那一组作业范围：范围引用与在用设备，按作业范围种类取。
type workScopeFixture struct {
	reference string
	device    string
}

var workScopeFixtures = map[accessidentity.WorkScopeKind]workScopeFixture{
	accessidentity.WorkScopeNode:    {reference: "SYN-NODE-SHA-HUB", device: "SYN-DEVICE-NODE"},
	accessidentity.WorkScopeOffsite: {reference: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE"},
}

// workFactProbeBodies 是作业事实各口能越过载荷译装、走到编排的那一份探测载荷，由各特性的 endpoints_<特性>_test.go 登记。
// 载荷是测试数据，不进生产文件；登记处里有作业事实行而这里没有载荷，workFactOperatorFaces 当场停。
var workFactProbeBodies = map[string]string{}

func registerWorkFactProbe(pattern, body string) {
	if _, registered := workFactProbeBodies[pattern]; registered {
		panic("parcel-api: work fact probe registered twice: " + pattern)
	}
	workFactProbeBodies[pattern] = body
}

// workFactOperatorFaces 由登记处导出：经作业事实渠道（不是特性自建 Intake）的各行，范围头取它声明的作业范围种类。
func workFactOperatorFaces() []workFactOperatorFace {
	lines := declaredEndpointLines()
	var faces []workFactOperatorFace
	for _, pattern := range declaredPatterns(func(line endpointLine) bool { return line.face.family == faceWorkFact && !line.ownIntake }) {
		body, registered := workFactProbeBodies[pattern]
		if !registered {
			panic("parcel-api: work fact line has no probe body; register one in its endpoints_<feature>_test.go: " + pattern)
		}
		scope := lines[pattern].face.workScope
		faces = append(faces, workFactOperatorFace{
			pattern: pattern, scopeKind: string(scope), scopeRef: workScopeFixtures[scope].reference, device: workScopeFixtures[scope].device, body: body,
		})
	}
	return faces
}

// operationDecisionPatterns 是归「运营决定」能力面的各口（ADR-0151 决定一），由登记处导出：作业事实渠道装不进它们，
// 路由只装作业事实渠道时它们答未配置。
func operationDecisionPatterns() []string {
	return declaredPatterns(func(line endpointLine) bool { return line.face.family == faceOperationDecision })
}

type unavailableWorkFactDevices struct{}

func (unavailableWorkFactDevices) FindDevice(context.Context, string, string) (accessidentity.RecordedDevice, bool, error) {
	return accessidentity.RecordedDevice{}, false, errors.New("device register unreachable")
}

func workFactOperatorRouter(
	t *testing.T,
	verifier accessidentity.OperatorCredentialVerifier,
	operators accessidentity.OperatorRegistry,
	devices accessidentity.DeviceRegistry,
) http.Handler {
	t.Helper()
	reception, transport, err := buildWorkFactIntakes(verifier, operators, devices, accessidentity.UnconfiguredAdmissionScope{})
	if err != nil {
		t.Fatal(err)
	}
	return httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(
		unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(),
		nil, nil, reception, transport))
}

func postWorkFactFace(router http.Handler, face workFactOperatorFace, token, device string, withHeaders bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, face.pattern, strings.NewReader(face.body))
	if withHeaders {
		request.Header.Set("Parcel-Work-Scope-Kind", face.scopeKind)
		request.Header.Set("Parcel-Work-Scope-Reference", face.scopeRef)
		if device == "" {
			device = face.device
		}
		request.Header.Set("Parcel-Device-Id", device)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// Covers: 票 operator-channel/10 做什么「主链事实的更正口随原事实归本能力面」与完成判据「隔离放行用例改写为真渠道答复格用例」——
// 作业事实各口（含更正口）在装配好的路由上答操作者渠道的格，发行方未配置、无令牌、授予不对、设备不在用、设备册读不动各是
// 一格；授予齐备才走到未接线的编排（500）。运营决定各口（operationDecisionPatterns）不在这一面。
// 测试输入是隔离合成，只记 `S`。
func TestWorkFactFacesAnswerOnTheOperatorChannel(t *testing.T) {
	const token = "presented.operator.token"
	cases := []struct {
		name    string
		router  http.Handler
		token   string
		device  string
		headers bool
		status  int
		code    string
	}{
		{
			name:   "issuer not configured",
			router: workFactOperatorRouter(t, accessidentity.UnconfiguredOperatorCredentialVerifier{}, workFactOperators{}, workFactDevices{}),
			token:  token, headers: true, status: http.StatusForbidden, code: "ACCESS_CHANNEL_NOT_CONFIGURED",
		},
		{
			name:    "no bearer token",
			router:  workFactOperatorRouter(t, acceptingVerifier{}, workFactOperators{}, workFactDevices{}),
			headers: true, status: http.StatusUnauthorized, code: "OPERATOR_CREDENTIAL_REJECTED",
		},
		{
			name:   "read grant is not a work fact grant",
			router: workFactOperatorRouter(t, acceptingVerifier{}, readerRegister{}, workFactDevices{}),
			token:  token, headers: true, status: http.StatusForbidden, code: "OPERATOR_NOT_GRANTED",
		},
		{
			name:   "device is not in use",
			router: workFactOperatorRouter(t, acceptingVerifier{}, workFactOperators{}, workFactDevices{}),
			token:  token, device: "SYN-DEVICE-ABSENT", headers: true, status: http.StatusForbidden, code: "DEVICE_NOT_IN_USE",
		},
		{
			name:   "device register is down",
			router: workFactOperatorRouter(t, acceptingVerifier{}, workFactOperators{}, unavailableWorkFactDevices{}),
			token:  token, headers: true, status: http.StatusServiceUnavailable, code: "IDENTITY_DEPENDENCY_UNAVAILABLE",
		},
		{
			name:   "headers missing",
			router: workFactOperatorRouter(t, acceptingVerifier{}, workFactOperators{}, workFactDevices{}),
			token:  token, status: http.StatusBadRequest, code: "MALFORMED_REQUEST",
		},
		{
			name:   "granted operator reaches the unwired handler",
			router: workFactOperatorRouter(t, acceptingVerifier{}, workFactOperators{}, workFactDevices{}),
			token:  token, headers: true, status: http.StatusInternalServerError, code: "NO_ANSWER_FORMED",
		},
	}
	for _, testCase := range cases {
		for _, face := range workFactOperatorFaces() {
			response := postWorkFactFace(testCase.router, face, testCase.token, testCase.device, testCase.headers)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("%s, %s: answer = %d %q, want %d %q; body %s",
					face.pattern, testCase.name, response.Code, problemCode(t, response), testCase.status, testCase.code, response.Body.String())
			}
		}
	}

	// 节点上在用的设备、种类 NODE：认证能过，但运输履约不收这一格。漏了种类门会答 500，而不是 400。
	granted := workFactOperatorRouter(t, acceptingVerifier{}, workFactOperators{}, workFactDevices{})
	for _, face := range workFactOperatorFaces() {
		if face.scopeKind != "OFFSITE" {
			continue
		}
		face.scopeKind = "NODE"
		face.scopeRef = "SYN-NODE-SHA-HUB"
		face.device = "SYN-DEVICE-NODE"
		response := postWorkFactFace(granted, face, token, "", true)
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s, transport fulfillment rejects NODE: answer = %d %q, want 400 MALFORMED_REQUEST; body %s",
				face.pattern, response.Code, problemCode(t, response), response.Body.String())
		}
	}
}

func TestWorkFactChannelLeavesOperationDecisionFacesUnconfigured(t *testing.T) {
	router := workFactOperatorRouter(t, acceptingVerifier{}, workFactOperators{}, workFactDevices{})
	for _, pattern := range operationDecisionPatterns() {
		request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: answer = %d %q, want 403 ACCESS_CHANNEL_NOT_CONFIGURED", pattern, response.Code, problemCode(t, response))
		}
	}
}
