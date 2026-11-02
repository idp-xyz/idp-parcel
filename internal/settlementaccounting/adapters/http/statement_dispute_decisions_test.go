package settlementhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 钉登记争议口与处理争议口的 Intake 那一层（ADR-0353 决定一、二、四）：各按自己那一种决定认证，租户取认证结果，提交操作者记成命令上的
// 操作证据；载荷形状不对答 400、不进编排。编排各格的答复由 cmd/parcel-api 的真库用例钉。

type capturingDisputes struct {
	opened   []application.OpenDisputeCommand
	resolved []application.ResolveDisputeCommand
}

func (double *capturingDisputes) OpenDispute(_ context.Context, command application.OpenDisputeCommand) (application.StatementResult, error) {
	double.opened = append(double.opened, command)
	return application.StatementResult{}, nil
}

func (double *capturingDisputes) ResolveDispute(_ context.Context, command application.ResolveDisputeCommand) (application.StatementResult, error) {
	double.resolved = append(double.resolved, command)
	return application.StatementResult{}, nil
}

const disputeBody = `{"dispute":"SYN-DSP-1","statement":"SYN-STMT-1","charge":"SYN-CHG-1","disputedAmount":"1600",` +
	`"reason":"客户来电：重量按 1.2 kg 计，单上是 1.5 kg","openedAt":"2026-11-01T09:00:00Z"}`

// Covers: AT-SA-228（Intake 半边）——登记口按 STATEMENT_DISPUTE_REGISTRATION 认证；单、费用、金额（最小币单位整数串）、理由与客户提出的
// 时刻逐字到命令，操作证据是 `OPERATOR/发行方#sub`。金额为零或带符号、理由空白或过长、时刻写坏、自报租户，都答 400、不进编排。
func TestTheDisputeRegistrationIntakeTranslatesTheDisputeAndRecordsTheClerk(t *testing.T) {
	var asked []settlementhttp.OperatorDecision
	disputes := &capturingDisputes{}
	endpoint := settlementhttp.NewRegisterStatementDisputeEndpoint(operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: &asked}), disputes)

	endpoint.ServeHTTP(httptest.NewRecorder(), decisionRequest(disputeBody))
	if len(asked) != 1 || asked[0] != settlementhttp.OperatorDecisionStatementDisputeRegistration || len(disputes.opened) != 1 {
		t.Fatalf("认证 %v、编排被调 %d 次", asked, len(disputes.opened))
	}
	command := disputes.opened[0]
	if command.TenantID.String() != clientTenant || command.Dispute != "SYN-DSP-1" || command.Number != "SYN-STMT-1" ||
		command.ChargeID != "SYN-CHG-1" || command.DisputedMinor != 1600 || !strings.HasPrefix(command.Reason, "客户来电") ||
		command.OpenedAt.Format("2006-01-02T15:04:05Z") != "2026-11-01T09:00:00Z" ||
		command.OperationEvidence != "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-01" {
		t.Fatalf("命令 = %+v", command)
	}

	for name, body := range map[string]string{
		"金额为零":  strings.Replace(disputeBody, `"1600"`, `"0"`, 1),
		"金额带符号": strings.Replace(disputeBody, `"1600"`, `"+1600"`, 1),
		"理由空白":  strings.Replace(disputeBody, `"客户来电：重量按 1.2 kg 计，单上是 1.5 kg"`, `"  "`, 1),
		"理由过长":  strings.Replace(disputeBody, `"客户来电：重量按 1.2 kg 计，单上是 1.5 kg"`, `"`+strings.Repeat("长", 1001)+`"`, 1),
		"时刻写坏":  strings.Replace(disputeBody, `"2026-11-01T09:00:00Z"`, `"昨天"`, 1),
		"自报租户":  strings.Replace(disputeBody, `{"dispute"`, `{"tenantId":"SYN-TENANT-02","dispute"`, 1),
		"缺费用":   strings.Replace(disputeBody, `"charge":"SYN-CHG-1",`, ``, 1),
	} {
		before := len(disputes.opened)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, decisionRequest(body))
		if response.Code != http.StatusBadRequest || len(disputes.opened) != before {
			t.Fatalf("%s = %d，进了编排 %v", name, response.Code, len(disputes.opened) != before)
		}
	}
}

// Covers: AT-SA-229（Intake 半边）——处理口按 STATEMENT_DISPUTE_RESOLUTION 认证，与登记分开；四种走向按原词认，依据与时刻到命令、
// 带操作证据。走向写错或带金额，答 400、不进编排。
func TestTheDisputeResolutionIntakeAuthenticatesItsOwnDecision(t *testing.T) {
	var asked []settlementhttp.OperatorDecision
	disputes := &capturingDisputes{}
	endpoint := settlementhttp.NewResolveStatementDisputeEndpoint(operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: &asked}), disputes)
	body := `{"dispute":"SYN-DSP-1","resolution":"PENDING_REVIEW","basis":"已调过磅单，等仓库回话","resolvedAt":"2026-11-01T10:00:00Z"}`

	endpoint.ServeHTTP(httptest.NewRecorder(), decisionRequest(body))
	if len(asked) != 1 || asked[0] != settlementhttp.OperatorDecisionStatementDisputeResolution || len(disputes.resolved) != 1 {
		t.Fatalf("认证 %v、编排被调 %d 次", asked, len(disputes.resolved))
	}
	command := disputes.resolved[0]
	if command.Dispute != "SYN-DSP-1" || command.Kind != domain.DisputePendingReview || command.Basis == "" ||
		command.TenantID.String() != clientTenant || command.OperationEvidence == "" {
		t.Fatalf("命令 = %+v", command)
	}
	for name, raw := range map[string]string{
		"走向写错": strings.Replace(body, `"PENDING_REVIEW"`, `"APPROVED"`, 1),
		"带金额":  strings.Replace(body, `{"dispute"`, `{"amount":"100","dispute"`, 1),
	} {
		before := len(disputes.resolved)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, decisionRequest(raw))
		if response.Code != http.StatusBadRequest || len(disputes.resolved) != before {
			t.Fatalf("%s = %d", name, response.Code)
		}
	}
}
