package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

const integrationClientUnconfiguredBody = `{"error":{"code":"ACCESS_CHANNEL_NOT_CONFIGURED"}}` + "\n"

var integrationClientFaces = []string{
	"/customs/external-results",
	"/customs-regulatory-credential-registrations",
	"/settlement-external-funds-fact-registrations",
	"/settlement-external-funds-fact-correction-registrations",
}

type absentClientRegister struct{}

func (absentClientRegister) FindIntegrationClient(context.Context, accessidentity.IntegrationClientSubject) (accessidentity.IntegrationClientStanding, bool, error) {
	return accessidentity.IntegrationClientStanding{}, false, nil
}

type acceptingClientVerifier struct{}

func (acceptingClientVerifier) VerifyIntegrationClientCredential(
	_ context.Context,
	credential accessidentity.IntegrationClientCredential,
) (accessidentity.VerifiedClientToken, error) {
	if credential.Token() == "" {
		return accessidentity.VerifiedClientToken{}, accessidentity.ErrCredentialRejected
	}
	subject, err := accessidentity.NewIntegrationClientSubject("https://id.syn.example/dex", "SYN-CLIENT-01")
	if err != nil {
		return accessidentity.VerifiedClientToken{}, err
	}
	return accessidentity.NewVerifiedClientToken(subject, false), nil
}

type clientFactRegister struct {
	facts []accessidentity.ExternalFactType
	start time.Time
	end   time.Time
}

func (register clientFactRegister) FindIntegrationClient(context.Context, accessidentity.IntegrationClientSubject) (accessidentity.IntegrationClientStanding, bool, error) {
	subject, err := accessidentity.NewIntegrationClientSubject("https://id.syn.example/dex", "SYN-CLIENT-01")
	if err != nil {
		return accessidentity.IntegrationClientStanding{}, false, err
	}
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-REF/broker")
	if err != nil {
		return accessidentity.IntegrationClientStanding{}, false, err
	}
	binding, err := accessidentity.NewIntegrationClientBinding(subject, "SYN-TENANT-01", "SYN-SOURCE/broker", reference, false, "SYN-BASIS")
	if err != nil {
		return accessidentity.IntegrationClientStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(register.start, register.end)
	if err != nil {
		return accessidentity.IntegrationClientStanding{}, false, err
	}
	var grants []accessidentity.RecordedClientGrant
	for _, fact := range register.facts {
		grant, err := accessidentity.NewIntegrationClientGrant("SYN-TENANT-01", "SYN-GRANT-"+fact.String(), subject, fact, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return accessidentity.IntegrationClientStanding{}, false, err
		}
		recorded, err := accessidentity.NewRecordedClientGrant(grant, nil)
		if err != nil {
			return accessidentity.IntegrationClientStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewIntegrationClientStanding(binding, grants)
	return standing, err == nil, err
}

func unconfiguredIntegrationClientIntakes() integrationClientIntakes {
	minter, err := buildIntegrationClientMinter(accessidentity.UnconfiguredIntegrationClientCredentialVerifier{}, absentClientRegister{})
	if err != nil {
		panic(err)
	}
	intakes, err := buildIntegrationClientIntakes(minter)
	if err != nil {
		panic(err)
	}
	return intakes
}

func integrationClientRouter(t *testing.T, verifier accessidentity.IntegrationClientCredentialVerifier, registry accessidentity.IntegrationClientRegistry) http.Handler {
	t.Helper()
	minter, err := buildIntegrationClientMinter(verifier, registry)
	if err != nil {
		t.Fatal(err)
	}
	intakes, err := buildIntegrationClientIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	return httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(
		unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), intakes, nil, nil, nil, nil, nil))
}

func TestIntegrationClientChannelWithoutIssuerParametersAnswersNotConfigured(t *testing.T) {
	verifier, configured, err := buildIntegrationClientCredentialVerifier(fakeGetenv(nil))
	if err != nil {
		t.Fatalf("unset parameters must not refuse to start: %v", err)
	}
	if configured {
		t.Fatal("unset parameters reported as a configured integration client channel")
	}
	_, err = verifier.VerifyIntegrationClientCredential(context.Background(), accessidentity.NewIntegrationClientCredential("any.presented.token", nil))
	if !errors.Is(err, accessidentity.ErrAccessChannelNotConfigured) {
		t.Fatalf("err = %v, want ErrAccessChannelNotConfigured", err)
	}
}

func TestIntegrationClientChannelWithIncompleteIssuerParametersRefusesToStart(t *testing.T) {
	cases := map[string]map[string]string{
		"issuer only":   {integrationClientIssuerEnv: "https://id.syn.example/dex"},
		"audience only": {integrationClientAudienceEnv: "idp-parcel-api"},
		"issuer without JWKS": {
			integrationClientIssuerEnv:   "https://id.syn.example/dex",
			integrationClientAudienceEnv: "idp-parcel-api",
		},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := buildIntegrationClientCredentialVerifier(fakeGetenv(values)); err == nil {
				t.Fatalf("parameters %v started; want refusal", values)
			}
		})
	}
}

func TestIntegrationClientFacesStayUnconfiguredWhenTheIssuerIsUnset(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnconfiguredBusinessEndpoints())
	for _, pattern := range integrationClientFaces {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(`{"any":"body"}`)))
		if response.Code != http.StatusForbidden || response.Body.String() != integrationClientUnconfiguredBody {
			t.Fatalf("%s: %d %q, want 403 %q", pattern, response.Code, response.Body.String(), integrationClientUnconfiguredBody)
		}
	}
}

func TestIntegrationClientFacesAnswerTheChannelGrades(t *testing.T) {
	now := time.Now()
	open := clientFactRegister{
		facts: []accessidentity.ExternalFactType{
			accessidentity.FactCustomsExternalResult,
			accessidentity.FactRegulatoryCredential,
			accessidentity.FactExternalFunds,
		},
		start: now.Add(-time.Hour),
		end:   now.Add(time.Hour),
	}
	expired := open
	expired.start = now.Add(-2 * time.Hour)
	expired.end = now.Add(-time.Hour)
	fundsOnly := clientFactRegister{
		facts: []accessidentity.ExternalFactType{accessidentity.FactExternalFunds},
		start: open.start,
		end:   open.end,
	}
	cases := map[string]struct {
		verifier accessidentity.IntegrationClientCredentialVerifier
		registry accessidentity.IntegrationClientRegistry
		token    string
		status   int
		code     string
		only     []string
	}{
		"no bearer token": {
			verifier: acceptingClientVerifier{}, registry: open, token: "",
			status: http.StatusUnauthorized, code: "INTEGRATION_CLIENT_CREDENTIAL_REJECTED",
		},
		"client not registered": {
			verifier: acceptingClientVerifier{}, registry: absentClientRegister{}, token: "presented.client.token",
			status: http.StatusForbidden, code: "INTEGRATION_CLIENT_NOT_GRANTED",
		},
		"grant interval has ended": {
			verifier: acceptingClientVerifier{}, registry: expired, token: "presented.client.token",
			status: http.StatusForbidden, code: "INTEGRATION_CLIENT_NOT_GRANTED",
		},
		"granted, admission not registered": {
			verifier: acceptingClientVerifier{}, registry: open, token: "presented.client.token",
			status: http.StatusForbidden, code: "OUTSIDE_ADMISSION_SCOPE",
		},
		"funds grant does not cover customs facts": {
			verifier: acceptingClientVerifier{}, registry: fundsOnly, token: "presented.client.token",
			status: http.StatusForbidden, code: "INTEGRATION_CLIENT_NOT_GRANTED",
			only: []string{"/customs/external-results", "/customs-regulatory-credential-registrations"},
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			router := integrationClientRouter(t, testCase.verifier, testCase.registry)
			faces := testCase.only
			if faces == nil {
				faces = integrationClientFaces
			}
			for _, pattern := range faces {
				request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(`{}`))
				if testCase.token != "" {
					request.Header.Set("Authorization", "Bearer "+testCase.token)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != testCase.status || problemCode(t, response) != testCase.code {
					t.Fatalf("%s: answer = %d %q, want %d %q", pattern, response.Code, problemCode(t, response), testCase.status, testCase.code)
				}
			}
		})
	}
}
