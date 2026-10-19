package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// 节点关务区域登记口的传输面：未配置 403 且不碰编排、答案逐名转写、未决不冒充治理答案。端点体与另几类同一份
// （newConfigurationRegistrationEndpoint），这里只钉本口自己接得对。

type nodeZoneIntakeDouble struct {
	command application.RegisterNodeCustomsZoneCommand
	err     error
}

func (double nodeZoneIntakeDouble) IntakeNodeCustomsZoneRegistration(
	context.Context, *http.Request,
) (application.RegisterNodeCustomsZoneCommand, error) {
	return double.command, double.err
}

func TestNodeCustomsZoneRegistrationEndpointAnswersByName(t *testing.T) {
	command, err := registrationjson.NodeCustomsZoneFromJSON([]byte(`{
		"tenantId": "SYN-TENANT-01", "nodeRef": "SYN-NODE-SHA-HUB", "zoneRef": "SYN-ZONE-CN",
		"appliesFrom": "2026-01-01T00:00:00Z"
	}`))
	if err != nil {
		t.Fatalf("译装：%v", err)
	}
	for _, test := range []struct {
		outcome application.CaseConfigurationOutcome
		status  int
		body    string
	}{
		{application.ConfigurationRegistered, http.StatusCreated, "REGISTERED"},
		{application.ConfigurationExisting, http.StatusOK, "EXISTING"},
		{application.ConfigurationContentConflict, http.StatusOK, "CONTENT_CONFLICT"},
	} {
		registrar := &configurationRegistrarDouble[application.RegisterNodeCustomsZoneCommand]{outcome: test.outcome}
		endpoint := customshttp.NewRegisterNodeCustomsZoneEndpoint(nodeZoneIntakeDouble{command: command}, registrar)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/customs-node-zone-registrations", strings.NewReader("{}")))
		var body struct {
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("解答复：%v", err)
		}
		if response.Code != test.status || body.Outcome != test.body {
			t.Fatalf("%s：%d %s", test.outcome, response.Code, response.Body.Bytes())
		}
	}

	undecided := &configurationRegistrarDouble[application.RegisterNodeCustomsZoneCommand]{outcome: application.ConfigurationUndecided}
	response := httptest.NewRecorder()
	customshttp.NewRegisterNodeCustomsZoneEndpoint(nodeZoneIntakeDouble{command: command}, undecided).
		ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/customs-node-zone-registrations", strings.NewReader("{}")))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("未决不是治理答案：%d", response.Code)
	}
}

func TestNodeCustomsZoneRegistrationEndpointRefusesBeforeTheOrchestrationWhenUnconfigured(t *testing.T) {
	endpoint := customshttp.NewRegisterNodeCustomsZoneEndpoint(customshttp.UnconfiguredIntake{},
		unreachableConfigurationRegistrar[application.RegisterNodeCustomsZoneCommand]{t: t})
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/customs-node-zone-registrations", strings.NewReader(`{"tenantId": "SYN-TENANT-01"}`)))
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "ACCESS_CHANNEL_NOT_CONFIGURED") {
		t.Fatalf("未配置：%d %s", response.Code, response.Body.Bytes())
	}

	malformed := customshttp.NewRegisterNodeCustomsZoneEndpoint(
		nodeZoneIntakeDouble{err: errors.Join(customshttp.ErrMalformedRequest, errors.New("bad"))},
		unreachableConfigurationRegistrar[application.RegisterNodeCustomsZoneCommand]{t: t})
	response = httptest.NewRecorder()
	malformed.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/customs-node-zone-registrations", strings.NewReader("{}")))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("坏报文：%d", response.Code)
	}
}
