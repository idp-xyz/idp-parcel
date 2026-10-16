package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

// Covers: 票 group-legal-entities-uiux/11——操作者绑定查阅接在操作者查阅读上。发行方未配置答未配置；没有查阅读授予答未授予
// （同 ADR-0100 决定四的同答，不说它绑在哪）；有授予答册上的绑定租户，查询串里自报的租户不算数。
func TestOperatorBindingAnswersFromTheOperatorChannel(t *testing.T) {
	if identityhttp.OperatorBindingPath != "/operator-binding" {
		t.Fatalf("路径常量 = %s，端点表字面量对不上", identityhttp.OperatorBindingPath)
	}

	granted, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	ungranted, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	unconfigured, err := buildOperatorMinter(accessidentity.UnconfiguredOperatorCredentialVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		minter *accessidentity.OperatorMinter
		token  string
		status int
		code   string
	}{
		{"issuer unconfigured", unconfigured, "presented.operator.token", http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"no bearer token", granted, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"write grant is not a read grant", ungranted, "presented.operator.token", http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{"read grant answers the binding", granted, "presented.operator.token", http.StatusOK, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler, err := buildOperatorBinding(testCase.minter)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/operator-binding?tenant=SYN-TENANT-02", nil)
			if testCase.token != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != testCase.status {
				t.Fatalf("status %d body %s", response.Code, response.Body.String())
			}
			if testCase.code != "" {
				if problemCode(t, response) != testCase.code {
					t.Fatalf("code %s body %s", problemCode(t, response), response.Body.String())
				}
				return
			}
			var body struct {
				Outcome  string `json:"outcome"`
				TenantID string `json:"tenantId"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			// 夹具的操作者册把这个主体绑在 SYN-TENANT-01（operationsReaderRegister）。
			if body.Outcome != "OPERATOR_BINDING_RESOLVED" || body.TenantID != "SYN-TENANT-01" {
				t.Fatalf("body %s", response.Body.String())
			}
		})
	}

	if _, err := buildOperatorBinding(nil); err == nil {
		t.Fatal("nil minter was accepted")
	}
}
