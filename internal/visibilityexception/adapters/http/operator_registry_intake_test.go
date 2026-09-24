package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

type registryAuthenticator struct{ err error }

func (fake registryAuthenticator) AuthenticateRegistryWrite(_ context.Context, token string) (domain.TenantID, error) {
	if token == "" {
		return domain.TenantID{}, visibilityhttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return domain.TenantID{}, fake.err
	}
	return domain.NewTenantID("SYN-TENANT-01")
}

func registryIntake(t *testing.T, authenticator visibilityhttp.OperatorRegistryAuthenticator) *visibilityhttp.OperatorRegistryIntake {
	t.Helper()
	intake, err := visibilityhttp.NewOperatorRegistryIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func registryRequest(token, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/visibility-catalogue-milestone-mapping-registrations", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

const milestoneMappingBody = `{
	"version": "SYN-MAP-V1",
	"approvedBy": "SYN-approver-1",
	"effectiveFrom": "2026-09-01T00:00:00Z",
	"entries": [{"source": "PARCEL_SHIPMENT", "factKind": "SYN_KIND_DELIVERED", "milestone": "SYN-MILESTONE-DELIVERED"}]
}`

func TestOperatorRegistryIntakeTranslatesUnderTheAuthenticatedTenant(t *testing.T) {
	intake := registryIntake(t, registryAuthenticator{})
	command, err := intake.IntakeMilestoneMappingRegistration(context.Background(), registryRequest("t", milestoneMappingBody))
	if err != nil {
		t.Fatalf("milestone mapping: %v", err)
	}
	if command.TenantID.String() != "SYN-TENANT-01" || command.Header.Version != "SYN-MAP-V1" {
		t.Fatalf("command = tenant %q, header %+v", command.TenantID.String(), command.Header)
	}
	for name, body := range map[string]string{
		"tenant smuggled":   strings.Replace(milestoneMappingBody, `"version"`, `"tenantId": "SYN-TENANT-02", "version"`, 1),
		"tenant null":       strings.Replace(milestoneMappingBody, `"version"`, `"tenantId": null, "version"`, 1),
		"unknown field":     strings.Replace(milestoneMappingBody, `"version"`, `"registeredBy": "someone", "version"`, 1),
		"not a JSON object": `[]`,
	} {
		if _, err := intake.IntakeMilestoneMappingRegistration(context.Background(), registryRequest("t", body)); !errors.Is(err, visibilityhttp.ErrMalformedRegistration) {
			t.Fatalf("%s: err = %v, want ErrMalformedRegistration", name, err)
		}
	}
}

type milestoneRegistrarMustNotRun struct{ t *testing.T }

func (registrar milestoneRegistrarMustNotRun) Handle(context.Context, application.RegisterMilestoneMappingCommand) (application.RegisterCatalogResult, error) {
	registrar.t.Fatal("registration reached although the intake refused")
	return application.RegisterCatalogResult{}, nil
}

func TestOperatorRegistryAnswerGrades(t *testing.T) {
	cases := map[string]struct {
		err    error
		token  string
		body   string
		status int
		code   string
	}{
		"no bearer token":          {token: "", body: milestoneMappingBody, status: http.StatusUnauthorized, code: "OPERATOR_CREDENTIAL_REJECTED"},
		"issuer parameters unset":  {err: visibilityhttp.ErrAccessChannelNotConfigured, token: "t", body: milestoneMappingBody, status: http.StatusForbidden, code: "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"no registry grant":        {err: visibilityhttp.ErrOperatorNotGranted, token: "t", body: milestoneMappingBody, status: http.StatusForbidden, code: "OPERATOR_NOT_GRANTED"},
		"identity dependency down": {err: visibilityhttp.ErrIdentityDependencyUnavailable, token: "t", body: milestoneMappingBody, status: http.StatusServiceUnavailable, code: "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"self-reported tenant":     {token: "t", body: `{"tenantId": "SYN-TENANT-02"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			endpoint := visibilityhttp.NewRegisterMilestoneMappingEndpoint(registryIntake(t, registryAuthenticator{err: testCase.err}), milestoneRegistrarMustNotRun{t})
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, registryRequest(testCase.token, testCase.body))
			var problem struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			_ = json.Unmarshal(recorder.Body.Bytes(), &problem)
			if recorder.Code != testCase.status || problem.Error.Code != testCase.code {
				t.Fatalf("answer = %d %q, want %d %q", recorder.Code, problem.Error.Code, testCase.status, testCase.code)
			}
		})
	}
}
