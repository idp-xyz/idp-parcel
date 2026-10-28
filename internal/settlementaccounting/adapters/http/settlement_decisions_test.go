package settlementhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// 钉运营确认口与截单发布口的 Intake 与时点门那一层（ADR-0339 决定一、二）：各按自己那一种决定认证，租户取认证结果，载荷形状不对
// 答 400；时点没登答 200 带时点门的原名、不调编排之外的东西；时点册读不动与编排交回 error 都是没形成答案。编排各格的答复由
// cmd/parcel-api 的真库用例钉。

type momentOnlyConfirmer struct {
	moment application.SettlementMomentOutcome
	err    error
	calls  []application.ConfirmChargeCommand
}

func (double *momentOnlyConfirmer) Confirm(
	_ context.Context, command application.ConfirmChargeCommand,
) (application.SettlementMomentOutcome, application.ConfirmChargeResult, error) {
	double.calls = append(double.calls, command)
	return double.moment, application.ConfirmChargeResult{}, double.err
}

type momentOnlyPublisher struct {
	moment application.SettlementMomentOutcome
	err    error
	calls  []application.PublishStatementCommand
}

func (double *momentOnlyPublisher) CutOff(
	_ context.Context, command application.PublishStatementCommand,
) (application.SettlementMomentOutcome, application.StatementResult, error) {
	double.calls = append(double.calls, command)
	return double.moment, application.StatementResult{}, double.err
}

func decisionRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer t")
	return request
}

const publicationBody = `{"number":"SYN-STMT-1","account":"SYN-ACCT-1","period":"SYN-PERIOD-1","version":"v1","currency":"CNY",` +
	`"charges":["SYN-CHARGE-1"],"lastAttributionDate":"2026-10-31","declaredTotal":"9600","cutOffAt":"2026-10-31T16:00:00Z"}`

// Covers: AT-SA-221（Intake 半边）——确认口按 CHARGE_CONFIRMATION 认证，命令只带认证出的租户与点名的费用；时点没登答 200 原名。
func TestTheChargeConfirmationFaceAuthenticatesItsOwnDecisionAndAnswersTheMomentGate(t *testing.T) {
	var asked []settlementhttp.OperatorDecision
	intake := operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: &asked})
	confirmer := &momentOnlyConfirmer{moment: application.SettlementMomentUnconfigured}
	endpoint := settlementhttp.NewConfirmChargeEndpoint(intake, confirmer)

	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, decisionRequest(`{"charge":"SYN-CHARGE-1"}`))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"SETTLEMENT_MOMENT_UNCONFIGURED"`) {
		t.Fatalf("时点没登 = %d %s", response.Code, response.Body)
	}
	if len(asked) != 1 || asked[0] != settlementhttp.OperatorDecisionChargeConfirmation {
		t.Fatalf("认证的决定种类 = %v", asked)
	}
	if len(confirmer.calls) != 1 || confirmer.calls[0].ChargeID != "SYN-CHARGE-1" || confirmer.calls[0].TenantID.String() != clientTenant {
		t.Fatalf("命令 = %+v", confirmer.calls)
	}

	for name, body := range map[string]string{
		"自报租户": `{"charge":"SYN-CHARGE-1","tenantId":"SYN-TENANT-02"}`,
		"缺费用":  `{}`,
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, decisionRequest(body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d，want 400", name, response.Code)
		}
	}

	for name, double := range map[string]*momentOnlyConfirmer{
		"时点册读不动": {moment: application.SettlementMomentViewUnavailable},
		"编排出错":   {err: errors.New("synthetic")},
	} {
		response := httptest.NewRecorder()
		settlementhttp.NewConfirmChargeEndpoint(intake, double).ServeHTTP(response, decisionRequest(`{"charge":"SYN-CHARGE-1"}`))
		if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), `"outcome"`) {
			t.Fatalf("%s = %d %s，want 500 不带 outcome", name, response.Code, response.Body)
		}
	}

	get := httptest.NewRecorder()
	endpoint.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/x", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", get.Code)
	}
}

// Covers: AT-SA-222（Intake 半边）——发布口按 STATEMENT_PUBLICATION 认证；载荷逐格到命令，合计按最小币单位的整数串收，发布时刻取端点
// 时钟、不从载荷收；合计写成数字、不列费用、自报租户都答 400。
func TestTheStatementPublicationFaceTakesThePublishedAtFromItsClock(t *testing.T) {
	var asked []settlementhttp.OperatorDecision
	intake := operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: &asked})
	publisher := &momentOnlyPublisher{moment: application.SettlementMomentUnconfigured}
	at := time.Date(2026, 10, 31, 17, 0, 0, 0, time.UTC)
	endpoint := settlementhttp.NewPublishStatementEndpoint(intake, publisher, func() time.Time { return at })

	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, decisionRequest(publicationBody))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"SETTLEMENT_MOMENT_UNCONFIGURED"`) {
		t.Fatalf("时点没登 = %d %s", response.Code, response.Body)
	}
	if len(asked) != 1 || asked[0] != settlementhttp.OperatorDecisionStatementPublication {
		t.Fatalf("认证的决定种类 = %v", asked)
	}
	command := publisher.calls[0]
	if command.Number != "SYN-STMT-1" || command.Account != "SYN-ACCT-1" || command.DeclaredTotalMinor != 9600 ||
		len(command.ChargeIDs) != 1 || command.LastAttributionDate != "2026-10-31" || !command.PublishedAt.Equal(at) ||
		!command.CutOffAt.Equal(time.Date(2026, 10, 31, 16, 0, 0, 0, time.UTC)) || command.TenantID.String() != clientTenant {
		t.Fatalf("命令 = %+v", command)
	}

	for name, body := range map[string]string{
		"合计写成数字": strings.Replace(publicationBody, `"9600"`, `9600`, 1),
		"不列费用":   strings.Replace(publicationBody, `["SYN-CHARGE-1"]`, `[]`, 1),
		"自报租户":   strings.Replace(publicationBody, `{"number"`, `{"tenantId":"SYN-TENANT-02","number"`, 1),
		"带发布时刻":  strings.Replace(publicationBody, `{"number"`, `{"publishedAt":"2026-10-01T00:00:00Z","number"`, 1),
		"合计带正号":  strings.Replace(publicationBody, `"9600"`, `"+9600"`, 1),
		"合计带前导零": strings.Replace(publicationBody, `"9600"`, `"09600"`, 1),
		"单号空白":   strings.Replace(publicationBody, `"SYN-STMT-1"`, `" "`, 1),
		"归属日写坏":  strings.Replace(publicationBody, `"2026-10-31"`, `"2026-10-32"`, 1),
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, decisionRequest(body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d，want 400", name, response.Code)
		}
	}
}
