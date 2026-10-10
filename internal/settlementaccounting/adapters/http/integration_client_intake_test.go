package settlementhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

const clientTenant = "SYN-TENANT-01"

type fundsAuthenticator struct {
	err error
}

func (fake fundsAuthenticator) AuthenticateExternalFunds(
	_ context.Context,
	credential settlementhttp.PresentedClientCredential,
) (domain.TenantID, error) {
	if credential.Token == "" && fake.err == nil {
		return domain.TenantID{}, settlementhttp.ErrIntegrationClientCredentialRejected
	}
	if fake.err != nil {
		return domain.TenantID{}, fake.err
	}
	return domain.NewTenantID(clientTenant)
}

func fundsIntakeForTest(t *testing.T, authenticator settlementhttp.IntegrationClientAuthenticator) *settlementhttp.IntegrationClientIntake {
	t.Helper()
	if authenticator == nil {
		authenticator = fundsAuthenticator{}
	}
	intake, err := settlementhttp.NewIntegrationClientIntake(authenticator)
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

const externalFundsFactBody = `{"factRef":"SYN-FUNDS-FACT-0801","sourceRef":"SYN-FUNDS-SOURCE/bank-08",` +
	`"payerRef":"SYN-PAYER/consignee-08","kind":"RECEIPT_CONFIRMED","currency":"SGD","amountMinor":12345,"version":"v1",` +
	`"occurredAt":"2026-09-24T15:00:00+08:00"}`

const externalFundsCorrectionBody = `{"factRef":"SYN-FUNDS-FACT-0801","corrects":"v1","version":"v2","amountMinor":100,"correctedAt":"2026-09-25T15:00:00+08:00"}`

func TestIntegrationClientIntakeTranslatesExternalFundsFactUnderTheAuthenticatedTenant(t *testing.T) {
	command, err := fundsIntakeForTest(t, nil).IntakeExternalFundsFactRegistration(context.Background(),
		commandRequest(externalFundsFactBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != clientTenant {
		t.Fatalf("TenantID = %q, want %q", got, clientTenant)
	}
	if command.Fact != "SYN-FUNDS-FACT-0801" || command.Source != "SYN-FUNDS-SOURCE/bank-08" || command.Payer != "SYN-PAYER/consignee-08" ||
		command.Kind.String() != "RECEIPT_CONFIRMED" || command.Currency != "SGD" || command.AmountMinor != 12345 || command.Version != "v1" {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
	if want := time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("OccurredAt = %s, want %s", command.OccurredAt, want)
	}
}

func TestIntegrationClientIntakeTranslatesExternalFundsCorrectionUnderTheSameGrant(t *testing.T) {
	command, err := fundsIntakeForTest(t, nil).IntakeExternalFundsFactCorrectionRegistration(context.Background(),
		commandRequest(externalFundsCorrectionBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.TenantID.String() != clientTenant || command.Fact != "SYN-FUNDS-FACT-0801" || command.Corrects != "v1" ||
		command.Version != "v2" || command.AmountMinor != 100 {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
}

func TestIntegrationClientIntakeRefusesMalformedExternalFundsFacts(t *testing.T) {
	intake := fundsIntakeForTest(t, nil)
	rest := strings.TrimPrefix(externalFundsFactBody, "{")
	for name, testCase := range map[string]struct{ body, keyword string }{
		"自报租户":   {`{"tenantId":"` + clientTenant + `",` + rest, "tenantId"},
		"空租户":    {`{"tenantId":null,` + rest, "tenantId"},
		"null":   {`null`, ""},
		"数组":     {`[` + externalFundsFactBody + `]`, ""},
		"尾随内容":   {externalFundsFactBody + ` {"x":1}`, "trailing"},
		"未知键":    {`{"bankRef":"x",` + rest, ""},
		"词表外的种类": {strings.Replace(externalFundsFactBody, "RECEIPT_CONFIRMED", "MAYBE_PAID", 1), ""},
		"空载荷":    {``, ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeExternalFundsFactRegistration(context.Background(), commandRequest(testCase.body))
			if !errors.Is(err, settlementhttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
			if testCase.keyword != "" && !strings.Contains(err.Error(), testCase.keyword) {
				t.Fatalf("拒绝理由没点名 %s：%v", testCase.keyword, err)
			}
		})
	}
}

type fundsRegistrarMustNotRun struct{ t *testing.T }

func (registrar fundsRegistrarMustNotRun) AdoptFact(context.Context, application.AdoptFundsFactCommand) (application.FundsResult, error) {
	registrar.t.Fatal("adoption reached the handler although the intake refused")
	return application.FundsResult{}, nil
}

func (registrar fundsRegistrarMustNotRun) CorrectFact(context.Context, application.CorrectFundsFactCommand) (application.FundsResult, error) {
	registrar.t.Fatal("correction reached the handler although the intake refused")
	return application.FundsResult{}, nil
}

func TestSettlementIntegrationClientAnswerGrades(t *testing.T) {
	cases := map[string]struct {
		err    error
		token  string
		status int
		code   string
	}{
		"no bearer token":          {token: "", status: http.StatusUnauthorized, code: "INTEGRATION_CLIENT_CREDENTIAL_REJECTED"},
		"issuer parameters unset":  {err: settlementhttp.ErrAccessChannelNotConfigured, token: "t", status: http.StatusForbidden, code: "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"token rejected":           {err: settlementhttp.ErrIntegrationClientCredentialRejected, token: "t", status: http.StatusUnauthorized, code: "INTEGRATION_CLIENT_CREDENTIAL_REJECTED"},
		"client not granted":       {err: settlementhttp.ErrIntegrationClientNotGranted, token: "t", status: http.StatusForbidden, code: "INTEGRATION_CLIENT_NOT_GRANTED"},
		"outside admission scope":  {err: settlementhttp.ErrOutsideAdmissionScope, token: "t", status: http.StatusForbidden, code: "OUTSIDE_ADMISSION_SCOPE"},
		"identity dependency down": {err: settlementhttp.ErrIdentityDependencyUnavailable, token: "t", status: http.StatusServiceUnavailable, code: "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	registrar := fundsRegistrarMustNotRun{t}
	endpoints := map[string]func(*settlementhttp.IntegrationClientIntake) http.Handler{
		"/settlement-external-funds-fact-registrations": func(intake *settlementhttp.IntegrationClientIntake) http.Handler {
			return settlementhttp.NewRegisterExternalFundsFactEndpoint(intake, registrar)
		},
		"/settlement-external-funds-fact-correction-registrations": func(intake *settlementhttp.IntegrationClientIntake) http.Handler {
			return settlementhttp.NewRegisterExternalFundsFactCorrectionEndpoint(intake, registrar)
		},
	}
	for pattern, build := range endpoints {
		for name, testCase := range cases {
			t.Run(pattern+"/"+name, func(t *testing.T) {
				endpoint := build(fundsIntakeForTest(t, fundsAuthenticator{err: testCase.err}))
				request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(externalFundsFactBody))
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

func TestNewIntegrationClientIntakeRejectsAMissingAuthenticator(t *testing.T) {
	if _, err := settlementhttp.NewIntegrationClientIntake(nil); err == nil {
		t.Fatal("缺认证方被接受")
	}
}
