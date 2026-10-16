package identityhttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

// Covers: 票 group-legal-entities-uiux/11——答的是认证结果上的绑定租户；查询串里自报的租户不进答复，身份只来自操作者册。
func TestQueryOperatorBindingAnswersTheEnvelopeTenant(t *testing.T) {
	handler := bindingHandler(t, authenticatorFake{tenant: catalogueTenant})
	request := httptest.NewRequest(http.MethodGet, identityhttp.OperatorBindingPath+"?tenant=SYN-TENANT-02", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d body %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome  string `json:"outcome"`
		TenantID string `json:"tenantId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "OPERATOR_BINDING_RESOLVED" || body.TenantID != catalogueTenant {
		t.Fatalf("body %s", response.Body.String())
	}
}

// Covers: 拒绝各态与其他操作者查阅口同口径（ADR-0100 决定四）：未配置、令牌不过、没有授予、依赖读不动各答各格；
// 只收 GET。
func TestQueryOperatorBindingAnswerGrades(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"issuer unconfigured", accessidentity.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"credential rejected", accessidentity.ErrCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"not granted", accessidentity.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{"verifier unavailable", accessidentity.ErrCredentialVerifierUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		{"registry unavailable", accessidentity.ErrOperatorRegistryUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := serveBinding(bindingHandler(t, authenticatorFake{err: testCase.err}), http.MethodGet)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("status %d code %s body %s", response.Code, problemCode(t, response), response.Body.String())
			}
		})
	}

	post := serveBinding(bindingHandler(t, authenticatorFake{tenant: catalogueTenant}), http.MethodPost)
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != http.MethodGet || problemCode(t, post) != "METHOD_NOT_ALLOWED" {
		t.Fatalf("POST = %d allow %s body %s", post.Code, post.Header().Get("Allow"), post.Body.String())
	}
}

// Covers: 没接铸造器时端点表上那一行答未配置，与发行方未设同一格；构造器不收 nil。
func TestOperatorBindingUnwiredAnswersUnconfigured(t *testing.T) {
	response := serveBinding(identityhttp.OperatorBindingHandler(nil), http.MethodGet)
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("status %d body %s", response.Code, response.Body.String())
	}
	if _, err := identityhttp.NewQueryOperatorBindingEndpoint(nil); err == nil {
		t.Fatal("nil authenticator was accepted")
	}
}

func bindingHandler(t *testing.T, authenticator identityhttp.OperatorCatalogueAuthenticator) http.Handler {
	t.Helper()
	handler, err := identityhttp.NewQueryOperatorBindingEndpoint(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func serveBinding(handler http.Handler, method string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, identityhttp.OperatorBindingPath, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
