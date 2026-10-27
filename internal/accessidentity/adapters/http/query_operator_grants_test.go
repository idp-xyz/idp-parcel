package identityhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

type grantsAuthenticatorFake struct {
	held identityhttp.OperatorGrantsHeld
	err  error
}

func (fake grantsAuthenticatorFake) AuthenticateGrantsRead(context.Context, string) (identityhttp.OperatorGrantsHeld, error) {
	return fake.held, fake.err
}

// Covers: ADR-0315——答的是认证结果上的授予集，面与决定种类各一个数组；查询串里自报的主体或租户不进答复，授予只来自令牌所证明的那个主体。
func TestQueryOperatorGrantsAnswersTheEnvelopeGrants(t *testing.T) {
	handler := grantsHandler(t, grantsAuthenticatorFake{held: identityhttp.OperatorGrantsHeld{
		Faces:     []accessidentity.CapabilityFace{accessidentity.CapabilityMasterDataAndOperationsRead, accessidentity.CapabilityRegistryConfigurationWrite},
		Decisions: []accessidentity.DecisionKind{accessidentity.DecisionManualReviewCompletion},
	}})
	request := httptest.NewRequest(http.MethodGet, identityhttp.OperatorGrantsPath+"?subject=someone-else&tenant=SYN-TENANT-02", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d body %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome   string   `json:"outcome"`
		Grants    []string `json:"grants"`
		Decisions []string `json:"decisions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "OPERATOR_GRANTS_RESOLVED" ||
		len(body.Grants) != 2 || body.Grants[0] != "MASTER_DATA_AND_OPERATIONS_READ" || body.Grants[1] != "REGISTRY_CONFIGURATION_WRITE" ||
		len(body.Decisions) != 1 || body.Decisions[0] != "MANUAL_REVIEW_COMPLETION" {
		t.Fatalf("body %s", response.Body.String())
	}
}

// Covers: ADR-0315——没有运营决定授予时 decisions 是空数组而不是缺键或 null：管理台按数组判「没有」，缺键与 null 会被它读成「不知道」。
func TestQueryOperatorGrantsAnswersEmptyArraysNotNull(t *testing.T) {
	handler := grantsHandler(t, grantsAuthenticatorFake{held: identityhttp.OperatorGrantsHeld{
		Faces: []accessidentity.CapabilityFace{accessidentity.CapabilityMasterDataAndOperationsRead},
	}})
	response := serveGrants(handler, http.MethodGet)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["decisions"]) != "[]" {
		t.Fatalf("decisions = %s, want []", raw["decisions"])
	}
}

// Covers: ADR-0315——拒绝各态与其他操作者查阅口同口径（ADR-0100 决定四）：未配置、令牌不过、没有授予、依赖读不动各答各格；只收 GET。
func TestQueryOperatorGrantsAnswerGrades(t *testing.T) {
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
			response := serveGrants(grantsHandler(t, grantsAuthenticatorFake{err: testCase.err}), http.MethodGet)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("status %d code %s body %s", response.Code, problemCode(t, response), response.Body.String())
			}
		})
	}

	post := serveGrants(grantsHandler(t, grantsAuthenticatorFake{}), http.MethodPost)
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != http.MethodGet || problemCode(t, post) != "METHOD_NOT_ALLOWED" {
		t.Fatalf("POST = %d allow %s body %s", post.Code, post.Header().Get("Allow"), post.Body.String())
	}
}

// Covers: ADR-0315——没接铸造器时端点表上那一行答未配置，与发行方未设同一格；构造器不收 nil。
func TestOperatorGrantsUnwiredAnswersUnconfigured(t *testing.T) {
	response := serveGrants(identityhttp.OperatorGrantsHandler(nil), http.MethodGet)
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("status %d body %s", response.Code, response.Body.String())
	}
	if _, err := identityhttp.NewQueryOperatorGrantsEndpoint(nil); err == nil {
		t.Fatal("nil authenticator was accepted")
	}
	if _, err := identityhttp.NewOperatorGrantsAuthenticator(nil); err == nil {
		t.Fatal("nil minter was accepted")
	}
}

func grantsHandler(t *testing.T, authenticator identityhttp.OperatorGrantsAuthenticator) http.Handler {
	t.Helper()
	handler, err := identityhttp.NewQueryOperatorGrantsEndpoint(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func serveGrants(handler http.Handler, method string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, identityhttp.OperatorGrantsPath, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
