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

// workFactOperatorFace 是换上作业事实登记能力面的一口：首登与三个更正口同一份认证与授予（票 operator-channel/10 第 4 条）。
type workFactOperatorFace struct {
	pattern   string
	scopeKind string
	scopeRef  string
	device    string
	body      string
}

var workFactOperatorFaces = []workFactOperatorFace{
	{
		pattern: "/node-operations/receptions", scopeKind: "NODE", scopeRef: "SYN-NODE-SHA-HUB", device: "SYN-DEVICE-NODE",
		body: `{"sourceId":"SYN-DEVICE-08/000001","deliveredBy":"SYN-COURIER-08","handlingUnit":"SYN-HU-08-0001",` +
			`"claim":"REFUSED","refusalReason":"SYN 外包装破损拒收","occurredAt":"2026-09-24T09:00:00+08:00"}`,
	},
	{
		pattern: "/transport-fulfillment/offsite-pickups", scopeKind: "OFFSITE", scopeRef: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE",
		body: `{"object":"SYN-PARCEL-08-01","task":"SYN-TASK-08-01","attempt":"SYN-ATTEMPT-08-01","place":"SYN-PLACE/shipper-dock-01",` +
			`"control":"SYN-CONTROL/signed-pickup-01","executedBy":"SYN-COURIER-08","occurredAt":"2026-09-24T08:30:00+08:00"}`,
	},
	{
		pattern: "/transport-fulfillment/offsite-pickup-attempts", scopeKind: "OFFSITE", scopeRef: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE",
		body: `{"sourceId":"SYN-DEVICE-08/attempt-01","task":"SYN-TASK-08-02","attempt":"SYN-ATTEMPT-08-02","executedBy":"SYN-COURIER-08",` +
			`"place":"SYN-PLACE/shipper-dock-02","plannedFrom":"2026-09-24T08:00:00+08:00","plannedTo":"2026-09-24T10:00:00+08:00",` +
			`"arrivedAt":"2026-09-24T08:40:00+08:00","evidence":"SYN-EVIDENCE/visit-02","objects":[{"object":"SYN-PARCEL-08-02",` +
			`"outcome":"PICKED_UP","control":"SYN-CONTROL/signed-02","occurredAt":"2026-09-24T08:45:00+08:00"}]}`,
	},
	{
		pattern: "/transport-fulfillment/offsite-pickup-corrections", scopeKind: "OFFSITE", scopeRef: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE",
		body: `{"object":"parcel-1","attempt":"attempt-1","predecessorVersion":"pickup-result/v1","place":"door-2",` +
			`"control":"control-2","executedBy":"courier-2","occurredAt":"2026-09-05T08:00:00Z","correctedAt":"2026-09-06T09:00:00Z"}`,
	},
	{
		pattern: "/transport-fulfillment/handovers", scopeKind: "OFFSITE", scopeRef: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE",
		body: `{"object":"SYN-PARCEL-08-05","scope":"SYN-SCOPE/hub-dock-05","releasedBy":"SYN-PARTY/hub-08","receivedBy":"SYN-PARTY/linehaul-08",` +
			`"verdict":"HANDED_OVER","releasingEvidence":"SYN-EVIDENCE/release-05","receivingEvidence":"SYN-EVIDENCE/receive-05",` +
			`"rule":"SYN-RULE/handover@v1","version":"v1","judgedAt":"2026-09-24T13:00:00+08:00"}`,
	},
	{
		pattern: "/transport-fulfillment/handover-corrections", scopeKind: "OFFSITE", scopeRef: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE",
		body: `{"object":"SYN-PARCEL-08-05","scope":"SYN-SCOPE/hub-dock-05","predecessorVersion":"v1","verdict":"HANDED_OVER",` +
			`"releasingEvidence":"SYN-EVIDENCE/release-05","receivingEvidence":"SYN-EVIDENCE/receive-05",` +
			`"rule":"SYN-RULE/handover@v1","newVersion":"v2","correctedAt":"2026-09-24T14:00:00+08:00"}`,
	},
	{
		pattern: "/transport-fulfillment/movement-facts", scopeKind: "OFFSITE", scopeRef: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE",
		body: `{"fact":"SYN-MOVE-08-06","schedule":"SYN-SCHEDULE/linehaul-06","kind":"ARRIVAL","location":"SYN-NODE-SIN-HUB",` +
			`"version":"v1","occurredAt":"2026-09-24T18:00:00+08:00"}`,
	},
	{
		pattern: "/transport-fulfillment-delivery-dispatch-triggers", scopeKind: "OFFSITE", scopeRef: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE",
		body: `{"segment":"SYN-SEGMENT-08-09","object":"SYN-PARCEL-08-09","occurredAt":"2026-09-25T07:00:00+08:00"}`,
	},
	{
		pattern: "/transport-fulfillment/deliveries", scopeKind: "OFFSITE", scopeRef: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE",
		body: `{"attempt":"SYN-ATTEMPT-08-10","object":"SYN-PARCEL-08-10","method":"HANDED_TO_RECIPIENT",` +
			`"recipient":"SYN-RECIPIENT/consignee-10","proof":"SYN-POD/signature-10"}`,
	},
	{
		pattern: "/transport-fulfillment/delivery-proof-corrections", scopeKind: "OFFSITE", scopeRef: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE",
		body: `{"attempt":"SYN-ATTEMPT-08-10","object":"SYN-PARCEL-08-10","predecessorVersion":"SYN-DRV-08-10-1",` +
			`"newProof":"SYN-POD/signature-10b","correctedAt":"2026-09-06T08:00:00Z"}`,
	},
	{
		pattern: "/transport-fulfillment/delivery-attempts", scopeKind: "OFFSITE", scopeRef: "SYN-OFFSITE-01", device: "SYN-DEVICE-OFFSITE",
		body: `{"task":"SYN-DISPATCH-08-13","attempt":"SYN-DELIVERY-ATTEMPT-08-13","executedBy":"SYN-COURIER-08",` +
			`"place":"SYN-PLACE/consignee-13","plannedFrom":"2026-09-25T09:00:00+08:00","plannedTo":"2026-09-25T12:00:00+08:00",` +
			`"arrivedAt":"2026-09-25T10:00:00+08:00","evidence":"SYN-EVIDENCE/arrival-13",` +
			`"objects":[{"object":"SYN-PARCEL-08-13","outcome":"DELIVERED","occurredAt":"2026-09-25T10:05:00+08:00"}]}`,
	},
}

// 这四口归运营决定（票 15），作业事实渠道就位之后仍答未配置。
var operationDecisionFacesLeftOnIsolatedWrite = []string{
	"/transport-fulfillment-carrier-first-effective-pickup-judgments",
	"/transport-fulfillment-dispatch-task-registrations",
	"/transport-fulfillment-segment-closures",
	"/transport-fulfillment-effective-time-judgments",
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

// Covers: 票 operator-channel/10 做什么第 4 条与完成判据第二条——作业事实各口（含三个更正口）在装配好的路由上答操作者渠道的格，
// 发行方未配置、无令牌、授予不对、设备不在用、设备册读不动各是一格；授予齐备才走到未接线的编排（500）。运营决定四口不在这一面。
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
		for _, face := range workFactOperatorFaces {
			response := postWorkFactFace(testCase.router, face, testCase.token, testCase.device, testCase.headers)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("%s, %s: answer = %d %q, want %d %q; body %s",
					face.pattern, testCase.name, response.Code, problemCode(t, response), testCase.status, testCase.code, response.Body.String())
			}
		}
	}

	// 节点上在用的设备、种类 NODE：认证能过，但运输履约不收这一格。漏了种类门会答 500，而不是 400。
	granted := workFactOperatorRouter(t, acceptingVerifier{}, workFactOperators{}, workFactDevices{})
	for _, face := range workFactOperatorFaces {
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
	for _, pattern := range operationDecisionFacesLeftOnIsolatedWrite {
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
