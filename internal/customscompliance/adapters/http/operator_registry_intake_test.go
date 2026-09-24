package customshttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

type registryAuthenticator struct{ err error }

func (fake registryAuthenticator) AuthenticateRegistryWrite(_ context.Context, token string) (domain.TenantID, error) {
	if token == "" {
		return domain.TenantID{}, customshttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return domain.TenantID{}, fake.err
	}
	return domain.NewTenantID("SYN-TENANT-01")
}

type gateRegistrarMustNotRun struct{ t *testing.T }

func (registrar gateRegistrarMustNotRun) Handle(context.Context, application.RegisterGateCatalogCommand) (application.CaseConfigurationOutcome, error) {
	registrar.t.Fatal("registration reached although the intake refused")
	return 0, nil
}

const gateCatalogBody = `{"scopeRef": "scope-1", "action": "FINAL_DELIVERY", "boundaryRef": "boundary-1", "registeredAt": "2026-09-25T01:00:00Z"}`

func TestOperatorRegistryIntakeTranslatesCustomsRegistrationsUnderTheAuthenticatedTenant(t *testing.T) {
	intake, err := customshttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/customs-gate-catalog-registrations", strings.NewReader(gateCatalogBody))
	request.Header.Set("Authorization", "Bearer t")
	command, err := intake.IntakeGateCatalogRegistration(context.Background(), request)
	if err != nil || command.TenantID.String() != "SYN-TENANT-01" {
		t.Fatalf("command tenant %q, err %v", command.TenantID.String(), err)
	}
}

func TestCustomsOperatorRegistryAnswerGrades(t *testing.T) {
	cases := map[string]struct {
		err    error
		token  string
		body   string
		status int
		code   string
	}{
		"no bearer token":          {token: "", body: gateCatalogBody, status: http.StatusUnauthorized, code: "OPERATOR_CREDENTIAL_REJECTED"},
		"issuer parameters unset":  {err: customshttp.ErrAccessChannelNotConfigured, token: "t", body: gateCatalogBody, status: http.StatusForbidden, code: "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"no registry grant":        {err: customshttp.ErrOperatorNotGranted, token: "t", body: gateCatalogBody, status: http.StatusForbidden, code: "OPERATOR_NOT_GRANTED"},
		"identity dependency down": {err: customshttp.ErrIdentityDependencyUnavailable, token: "t", body: gateCatalogBody, status: http.StatusServiceUnavailable, code: "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"self-reported tenant":     {token: "t", body: `{"tenantId": "SYN-TENANT-02"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			intake, err := customshttp.NewOperatorRegistryIntake(registryAuthenticator{err: testCase.err})
			if err != nil {
				t.Fatal(err)
			}
			endpoint := customshttp.NewRegisterGateCatalogEndpoint(intake, gateRegistrarMustNotRun{t})
			request := httptest.NewRequest(http.MethodPost, "/customs-gate-catalog-registrations", strings.NewReader(testCase.body))
			if testCase.token != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.token)
			}
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, request)
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
