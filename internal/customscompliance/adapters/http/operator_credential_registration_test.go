package customshttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

type capturingCredentialRegistrar struct {
	command application.RegisterCredentialCommand
	calls   int
}

func (fake *capturingCredentialRegistrar) Handle(_ context.Context, command application.RegisterCredentialCommand) (application.CaseConfigurationOutcome, error) {
	fake.command = command
	fake.calls++
	return application.ConfigurationRegistered, nil
}

// Covers: AT-CC-446（Intake 半边）——凭证登记口按「监管凭证登记」这一种决定认证；租户取认证结果，载荷与集成客户端那一口同一份
// 形状；登记人记成命令上的操作证据 `OPERATOR/发行方#sub`，不从载荷收。
func TestTheCredentialFaceAuthenticatesItsOwnKindAndRecordsTheRegistrant(t *testing.T) {
	var asked []customshttp.OperatorDecision
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{asked: &asked})
	if err != nil {
		t.Fatal(err)
	}
	registrar := &capturingCredentialRegistrar{}
	response := httptest.NewRecorder()
	customshttp.NewRegisterRegulatoryCredentialEndpoint(intake, registrar).ServeHTTP(response, post("/probe", regulatoryCredentialBody))

	if response.Code != http.StatusCreated || registrar.calls != 1 {
		t.Fatalf("answer = %d %s, calls = %d", response.Code, response.Body.String(), registrar.calls)
	}
	if len(asked) != 1 || asked[0] != "REGULATORY_CREDENTIAL_REGISTRATION" {
		t.Fatalf("asked = %v", asked)
	}
	command := registrar.command
	if command.TenantID.String() != "SYN-TENANT-01" || command.ID.String() != "SYN-CRED-0801" || command.Uses != 12 ||
		command.OperationEvidence != "OPERATOR/https://issuer#alice" {
		t.Fatalf("command = %+v", command)
	}
}

// Covers: AT-CC-447（Intake 半边）——认证方的格先于载荷：未授予答 403 OPERATOR_NOT_GRANTED，编排一次也不被调到；载荷自报租户或带
// 操作证据一类身份格是坏报文。
func TestTheCredentialFaceAnswersNotGrantedAndRefusesSelfReportedIdentity(t *testing.T) {
	denied, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{err: customshttp.ErrOperatorNotGranted})
	if err != nil {
		t.Fatal(err)
	}
	registrar := &capturingCredentialRegistrar{}
	response := httptest.NewRecorder()
	customshttp.NewRegisterRegulatoryCredentialEndpoint(denied, registrar).ServeHTTP(response, post("/probe", `not json`))
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "OPERATOR_NOT_GRANTED") || registrar.calls != 0 {
		t.Fatalf("未授予：%d %s calls=%d", response.Code, response.Body.String(), registrar.calls)
	}

	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	rest := strings.TrimPrefix(regulatoryCredentialBody, "{")
	for name, body := range map[string]string{
		"tenant":   `{"tenantId":"SYN-TENANT-09",` + rest,
		"evidence": `{"operationEvidence":"OPERATOR/someone-else",` + rest,
	} {
		if _, err := intake.IntakeRegulatoryCredentialRegistration(context.Background(), post("/probe", body)); !errors.Is(err, customshttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want 坏报文", name, err)
		}
	}
}

// Covers: AT-CC-447（Intake 半边：读取上限）——凭证手工口的请求体超过登记批文的读取上限即答坏报文；填充放在对象里面，不设上限时
// 这一份能译出命令，证得出是上限在拒。
func TestTheCredentialFaceRefusesAnOversizedBody(t *testing.T) {
	intake, err := customshttp.NewOperatorDecisionIntake(decisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	padded := strings.TrimSuffix(regulatoryCredentialBody, "}") + strings.Repeat(" ", 1<<20) + "}"
	if _, err := intake.IntakeRegulatoryCredentialRegistration(context.Background(), post("/probe", padded)); !errors.Is(err, customshttp.ErrMalformedRequest) {
		t.Fatalf("超限：err = %v, want 坏报文", err)
	}
	if _, err := intake.IntakeRegulatoryCredentialRegistration(context.Background(), post("/probe", regulatoryCredentialBody)); err != nil {
		t.Fatalf("上限之内的同一份被拒：%v", err)
	}
}
