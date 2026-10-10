package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

const clientTenant = "SYN-TENANT-01"

var clientReceivedAt = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type clientAuthenticator struct {
	err    error
	tenant string
}

func (fake clientAuthenticator) Authenticate(
	_ context.Context,
	credential customshttp.PresentedClientCredential,
	_ customshttp.IntegrationClientFact,
) (domain.TenantID, error) {
	if credential.Token == "" && fake.err == nil {
		return domain.TenantID{}, customshttp.ErrIntegrationClientCredentialRejected
	}
	if fake.err != nil {
		return domain.TenantID{}, fake.err
	}
	tenant := fake.tenant
	if tenant == "" {
		tenant = clientTenant
	}
	return domain.NewTenantID(tenant)
}

func clientIntakeForTest(t *testing.T, authenticator customshttp.IntegrationClientAuthenticator) *customshttp.IntegrationClientIntake {
	t.Helper()
	if authenticator == nil {
		authenticator = clientAuthenticator{}
	}
	intake, err := customshttp.NewIntegrationClientIntake(authenticator, fixedClock{at: clientReceivedAt})
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func commandRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.client.token")
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	request.URL.RawQuery = "tenant=TENANT-9"
	return request
}

const externalResultBody = `{"sourceId":"SYN-CUSTOMS-RESP/0001","layer":"RELEASE_RESULT","role":"CUSTOMS_AUTHORITY",` +
	`"rawSemantics":"SYN 放行回执 R01","claimedVersion":"SYN-DECL-V-0001","attempt":1,"scope":"SYN-DECL-UNIT-0001",` +
	`"occurredAt":"2026-09-24T10:30:00+08:00","release":{"kind":"CONDITIONAL","authority":"SYN-AUTHORITY/customs-sg",` +
	`"condition":"SYN 待补提单副本"}}`

func TestIntegrationClientIntakeTranslatesExternalResultUnderTheAuthenticatedTenant(t *testing.T) {
	command, err := clientIntakeForTest(t, nil).IntakeResult(context.Background(), commandRequest(externalResultBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != clientTenant {
		t.Fatalf("TenantID = %q, want %q", got, clientTenant)
	}
	if command.SourceID != "SYN-CUSTOMS-RESP/0001" || command.Layer.String() != "RELEASE_RESULT" || command.Role != "CUSTOMS_AUTHORITY" ||
		command.RawSemantics != "SYN 放行回执 R01" || command.ClaimedVersion != "SYN-DECL-V-0001" || command.Attempt != 1 ||
		command.Scope != "SYN-DECL-UNIT-0001" {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
	if want := time.Date(2026, 9, 24, 2, 30, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("OccurredAt = %s, want %s", command.OccurredAt, want)
	}
	if !command.ReceivedAt.Equal(clientReceivedAt) {
		t.Fatalf("ReceivedAt = %s, want %s", command.ReceivedAt, clientReceivedAt)
	}
	if command.Release == nil || command.Release.Kind.String() != "CONDITIONAL" ||
		command.Release.Authority.String() != "SYN-AUTHORITY/customs-sg" || command.Release.Condition != "SYN 待补提单副本" {
		t.Fatalf("Release = %+v，与载荷不符", command.Release)
	}
}

func TestIntegrationClientIntakeLeavesAnAbsentReleaseAbsent(t *testing.T) {
	command, err := clientIntakeForTest(t, nil).IntakeResult(context.Background(), commandRequest(
		`{"sourceId":"s","layer":"REGULATORY_RECEIPT","role":"r","rawSemantics":"x","claimedVersion":"v","scope":"u","occurredAt":"2026-09-24T10:30:00+08:00"}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.Release != nil {
		t.Fatalf("Release = %+v, want nil", command.Release)
	}
}

func TestIntegrationClientIntakeRefusesMalformedExternalResults(t *testing.T) {
	intake := clientIntakeForTest(t, nil)
	rest := strings.TrimPrefix(externalResultBody, "{")
	for name, body := range map[string]string{
		"自报租户":    `{"tenantId":"` + clientTenant + `",` + rest,
		"自报接收时间":  `{"receivedAt":"2026-09-24T09:00:00Z",` + rest,
		"尾随内容":    externalResultBody + ` {"x":1}`,
		"不是 JSON": "!!not-json!!",
		"空载荷":     "",
		"词表外的层":   `{"sourceId":"s","layer":"FINAL_ANSWER","rawSemantics":"x","claimedVersion":"v"}`,
		"词表外的放行":  `{"sourceId":"s","layer":"RELEASE_RESULT","rawSemantics":"x","claimedVersion":"v","release":{"kind":"MOSTLY"}}`,
		"时刻不是时刻":  `{"sourceId":"s","layer":"REGULATORY_RECEIPT","rawSemantics":"x","claimedVersion":"v","occurredAt":"昨天"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeResult(context.Background(), commandRequest(body))
			if !errors.Is(err, customshttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

const regulatoryCredentialBody = `{"credentialId":"SYN-CRED-0801","issuerRef":"SYN-AUTHORITY/customs-sg",` +
	`"holderRef":"SYN-HOLDER/broker-08","procedureRef":"SYN-PROCEDURE/import-general","validFrom":"2026-09-01T00:00:00Z",` +
	`"validTo":"2027-08-31T00:00:00Z","uses":12}`

func TestIntegrationClientIntakeTranslatesRegulatoryCredentialUnderTheAuthenticatedTenant(t *testing.T) {
	command, err := clientIntakeForTest(t, nil).IntakeRegulatoryCredentialRegistration(context.Background(),
		commandRequest(regulatoryCredentialBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != clientTenant {
		t.Fatalf("TenantID = %q, want %q", got, clientTenant)
	}
	if command.ID.String() != "SYN-CRED-0801" || command.Issuer.String() != "SYN-AUTHORITY/customs-sg" ||
		command.Holder.String() != "SYN-HOLDER/broker-08" || command.Procedure.String() != "SYN-PROCEDURE/import-general" ||
		command.Uses != 12 {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
	if !command.ValidFrom.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) || !command.ValidTo.Equal(time.Date(2027, 8, 31, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("有效期 = %s..%s，与载荷不符", command.ValidFrom, command.ValidTo)
	}
}

func TestIntegrationClientIntakeRefusesMalformedRegulatoryCredentials(t *testing.T) {
	intake := clientIntakeForTest(t, nil)
	rest := strings.TrimPrefix(regulatoryCredentialBody, "{")
	for name, testCase := range map[string]struct{ body, keyword string }{
		"自报租户": {`{"tenantId":"` + clientTenant + `",` + rest, "tenantId"},
		"空租户":  {`{"tenantId":null,` + rest, "tenantId"},
		"null": {`null`, ""},
		"数组":   {`[` + regulatoryCredentialBody + `]`, ""},
		"尾随内容": {regulatoryCredentialBody + ` {"x":1}`, "trailing"},
		"未知键":  {`{"issuedBy":"x",` + rest, ""},
		"空载荷":  {``, ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeRegulatoryCredentialRegistration(context.Background(), commandRequest(testCase.body))
			if !errors.Is(err, customshttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
			if testCase.keyword != "" && !strings.Contains(err.Error(), testCase.keyword) {
				t.Fatalf("拒绝理由没点名 %s：%v", testCase.keyword, err)
			}
		})
	}
}

type resultHandlerMustNotRun struct{ t *testing.T }

func (handler resultHandlerMustNotRun) Handle(context.Context, application.ReceiveExternalResultCommand) (application.ReceiveExternalResultResult, error) {
	handler.t.Fatal("external result reached the handler although the intake refused")
	return application.ReceiveExternalResultResult{}, nil
}

type credentialRegistrarMustNotRun struct{ t *testing.T }

func (registrar credentialRegistrarMustNotRun) Handle(context.Context, application.RegisterCredentialCommand) (application.CaseConfigurationOutcome, error) {
	registrar.t.Fatal("credential registration reached the handler although the intake refused")
	return 0, nil
}

func TestCustomsIntegrationClientAnswerGrades(t *testing.T) {
	cases := map[string]struct {
		err    error
		token  string
		status int
		code   string
	}{
		"no bearer token":          {token: "", status: http.StatusUnauthorized, code: "INTEGRATION_CLIENT_CREDENTIAL_REJECTED"},
		"issuer parameters unset":  {err: customshttp.ErrAccessChannelNotConfigured, token: "t", status: http.StatusForbidden, code: "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"token rejected":           {err: customshttp.ErrIntegrationClientCredentialRejected, token: "t", status: http.StatusUnauthorized, code: "INTEGRATION_CLIENT_CREDENTIAL_REJECTED"},
		"client not granted":       {err: customshttp.ErrIntegrationClientNotGranted, token: "t", status: http.StatusForbidden, code: "INTEGRATION_CLIENT_NOT_GRANTED"},
		"outside admission scope":  {err: customshttp.ErrOutsideAdmissionScope, token: "t", status: http.StatusForbidden, code: "OUTSIDE_ADMISSION_SCOPE"},
		"identity dependency down": {err: customshttp.ErrIdentityDependencyUnavailable, token: "t", status: http.StatusServiceUnavailable, code: "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	endpoints := map[string]func(customshttp.IntegrationClientAuthenticator) http.Handler{
		"/customs/external-results": func(authenticator customshttp.IntegrationClientAuthenticator) http.Handler {
			return customshttp.NewReceiveExternalResultEndpoint(clientIntakeForTest(t, authenticator), resultHandlerMustNotRun{t})
		},
		"/customs-regulatory-credential-registrations": func(authenticator customshttp.IntegrationClientAuthenticator) http.Handler {
			return customshttp.NewRegisterRegulatoryCredentialEndpoint(clientIntakeForTest(t, authenticator), credentialRegistrarMustNotRun{t})
		},
	}
	for pattern, build := range endpoints {
		for name, testCase := range cases {
			t.Run(pattern+"/"+name, func(t *testing.T) {
				endpoint := build(clientAuthenticator{err: testCase.err})
				request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(externalResultBody))
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
				if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
					t.Fatal(err)
				}
				if recorder.Code != testCase.status || problem.Error.Code != testCase.code {
					t.Fatalf("answer = %d %q, want %d %q", recorder.Code, problem.Error.Code, testCase.status, testCase.code)
				}
			})
		}
	}
}

func TestNewIntegrationClientIntakeRejectsMissingDependencies(t *testing.T) {
	if _, err := customshttp.NewIntegrationClientIntake(nil, fixedClock{}); err == nil {
		t.Fatal("缺认证方被接受")
	}
	if _, err := customshttp.NewIntegrationClientIntake(clientAuthenticator{}, nil); err == nil {
		t.Fatal("缺时钟被接受")
	}
}
