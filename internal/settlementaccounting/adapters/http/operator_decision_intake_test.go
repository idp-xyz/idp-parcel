package settlementhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// decisionAuthenticator 是运营决定认证方的替身：记下被问的决定种类，令牌为空答令牌不过，否则交回演示租户与固定的操作者。
type decisionAuthenticator struct {
	asked *[]settlementhttp.OperatorDecision
	err   error
}

func (fake decisionAuthenticator) AuthenticateOperatorDecision(
	_ context.Context,
	bearerToken string,
	decision settlementhttp.OperatorDecision,
) (settlementhttp.OperatorIdentity, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, decision)
	}
	if fake.err != nil {
		return settlementhttp.OperatorIdentity{}, fake.err
	}
	if bearerToken == "" {
		return settlementhttp.OperatorIdentity{}, settlementhttp.ErrOperatorCredentialRejected
	}
	tenant, err := domain.NewTenantID(clientTenant)
	if err != nil {
		return settlementhttp.OperatorIdentity{}, err
	}
	return settlementhttp.OperatorIdentity{Tenant: tenant, Operator: "https://id.syn.example/dex#SYN-FINANCE-01"}, nil
}

func operatorDecisionIntakeForTest(t *testing.T, authenticator settlementhttp.OperatorDecisionAuthenticator) *settlementhttp.OperatorDecisionIntake {
	t.Helper()
	intake, err := settlementhttp.NewOperatorDecisionIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

// Covers: AT-SA-200（Intake 半边）——采用与更正都按「外部资金事实采用」这一种决定认证；租户取认证结果，载荷与集成客户端族同一份
// 形状；提交操作者记成命令上的操作证据 `OPERATOR/发行方#sub`，不从载荷收。
func TestTheOperatorDecisionIntakeAdoptsUnderTheAuthenticatedTenantAndRecordsTheOperator(t *testing.T) {
	var asked []settlementhttp.OperatorDecision
	intake := operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: &asked})

	adopted, err := intake.IntakeExternalFundsFactRegistration(context.Background(), commandRequest(externalFundsFactBody))
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	if adopted.TenantID.String() != clientTenant || adopted.Fact != "SYN-FUNDS-FACT-0801" || adopted.AmountMinor != 12345 ||
		adopted.OperationEvidence != "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-01" {
		t.Fatalf("采用命令 = %+v", adopted)
	}
	corrected, err := intake.IntakeExternalFundsFactCorrectionRegistration(context.Background(), commandRequest(externalFundsCorrectionBody))
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	if corrected.TenantID.String() != clientTenant || corrected.Corrects != "v1" || corrected.Version != "v2" ||
		corrected.OperationEvidence != "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-01" {
		t.Fatalf("更正命令 = %+v", corrected)
	}
	if len(asked) != 2 || asked[0] != settlementhttp.OperatorDecisionExternalFundsFactAdoption || asked[1] != asked[0] {
		t.Fatalf("两口问的决定种类 = %v，都要是外部资金事实采用", asked)
	}
}

// Covers: AT-SA-201（Intake 半边）——认证方的格原样交回、不读载荷：未授予答 OPERATOR_NOT_GRANTED，不在准入范围答
// OUTSIDE_ADMISSION_SCOPE；载荷自报租户或带操作证据一类身份格是坏报文。
func TestTheOperatorDecisionIntakeAnswersTheChannelGradesAndRefusesSelfReportedIdentity(t *testing.T) {
	for _, grade := range []error{settlementhttp.ErrOperatorNotGranted, settlementhttp.ErrOutsideAdmissionScope, settlementhttp.ErrAccessChannelNotConfigured} {
		intake := operatorDecisionIntakeForTest(t, decisionAuthenticator{err: grade})
		if _, err := intake.IntakeExternalFundsFactRegistration(context.Background(), commandRequest(`not json`)); !errors.Is(err, grade) {
			t.Fatalf("采用口答 %v, want %v", err, grade)
		}
		if _, err := intake.IntakeExternalFundsFactCorrectionRegistration(context.Background(), commandRequest(`not json`)); !errors.Is(err, grade) {
			t.Fatalf("更正口答 %v, want %v", err, grade)
		}
	}

	intake := operatorDecisionIntakeForTest(t, decisionAuthenticator{})
	for name, body := range map[string]string{
		"tenant":    strings.Replace(externalFundsFactBody, `{`, `{"tenantId":"TENANT-9",`, 1),
		"evidence":  strings.Replace(externalFundsFactBody, `{`, `{"operationEvidence":"OPERATOR/someone-else",`, 1),
		"malformed": `{"factRef":`,
	} {
		if _, err := intake.IntakeExternalFundsFactRegistration(context.Background(), commandRequest(body)); !errors.Is(err, settlementhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want 坏报文", name, err)
		}
	}
}

// Covers: AT-SA-201（端点半边）——经采用口的端点体：未授予答 403 OPERATOR_NOT_GRANTED，编排一次也不被调到。
func TestTheAdoptionEndpointAnswersNotGrantedBeforeTheOrchestration(t *testing.T) {
	intake := operatorDecisionIntakeForTest(t, decisionAuthenticator{err: settlementhttp.ErrOperatorNotGranted})
	registrar := &fundsRegistrarDouble{}
	endpoint := settlementhttp.NewRegisterExternalFundsFactEndpoint(intake, registrar)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(externalFundsFactBody)))
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "OPERATOR_NOT_GRANTED") {
		t.Fatalf("answer = %d %s", response.Code, response.Body.String())
	}
	if registrar.called {
		t.Fatal("未授予的请求走到了采用编排")
	}
}
