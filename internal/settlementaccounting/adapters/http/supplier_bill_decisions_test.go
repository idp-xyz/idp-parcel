package settlementhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// 钉供应商账单接收口与审核口的 Intake 那一层（ADR-0340 决定一至三）：各按自己那一种决定认证，租户取认证结果，提交操作者记成命令上的
// 操作证据；载荷形状不对答 400、不进编排。编排各格的答复由 cmd/parcel-api 的真库用例钉。

type capturingReceiver struct {
	calls []application.ReceiveSupplierBillCommand
}

func (double *capturingReceiver) Handle(_ context.Context, command application.ReceiveSupplierBillCommand) (application.ReceiveSupplierBillResult, error) {
	double.calls = append(double.calls, command)
	return application.ReceiveSupplierBillResult{}, nil
}

type capturingAuditor struct {
	calls []application.AuditBillLineCommand
}

func (double *capturingAuditor) Audit(_ context.Context, command application.AuditBillLineCommand) (application.AuditResult, error) {
	double.calls = append(double.calls, command)
	return application.AuditResult{}, nil
}

const receptionBody = `{"claim":"SYN-BILL-1","version":"v1","supplier":"SYN-CARRIER-1","legalEntity":"SYN-LE-1","period":"SYN-PERIOD-1",` +
	`"currency":"CNY","lines":[{"line":"L1","feeItem":"SYN-FEE-1","claimed":"8500"},{"line":"L2","feeItem":"SYN-FEE-1","claimed":"300"}],` +
	`"receivedAt":"2026-10-28T02:00:00Z","directives":[{"line":"L1","classification":"MATCHED","expectedVersion":"SYN-COST-1"},` +
	`{"line":"L2","classification":"NO_MATCHING_OCCURRENCE","basis":"SYN-BASIS-1"}]}`

// Covers: AT-SA-225（Intake 半边）——接收口按 SUPPLIER_BILL_RECEPTION 认证；账单各格与逐行裁决逐字到命令，金额按最小币单位整数串收，
// 完全匹配不带依据也收；操作证据是 `OPERATOR/发行方#sub`。分类写错、金额带符号、自报租户、裁决形状不合匹配门、每行不是恰好一条
// 裁决，都答 400、不进编排。
func TestTheSupplierBillReceptionIntakeTranslatesTheBillAndRecordsTheClerk(t *testing.T) {
	var asked []settlementhttp.OperatorDecision
	receiver := &capturingReceiver{}
	endpoint := settlementhttp.NewReceiveSupplierBillEndpoint(operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: &asked}), receiver)

	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, decisionRequest(receptionBody))
	if len(asked) != 1 || asked[0] != settlementhttp.OperatorDecisionSupplierBillReception || len(receiver.calls) != 1 {
		t.Fatalf("认证 %v、编排被调 %d 次", asked, len(receiver.calls))
	}
	command := receiver.calls[0]
	if command.TenantID.String() != clientTenant || command.Claim.Claim.String() != "SYN-BILL-1" || len(command.Claim.Lines) != 2 ||
		command.Claim.Lines[0].ClaimedMinor != 8500 || len(command.Directives) != 2 ||
		command.Directives[0].Classification.String() != "MATCHED" || command.Directives[0].Basis.String() != "" ||
		command.Directives[1].ExpectedVersion.String() != "" || command.Directives[1].Basis.String() != "SYN-BASIS-1" ||
		command.OperationEvidence != "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-01" {
		t.Fatalf("命令 = %+v", command)
	}

	for name, body := range map[string]string{
		"分类写错":         strings.Replace(receptionBody, `"MATCHED"`, `"OK"`, 1),
		"金额带符号":        strings.Replace(receptionBody, `"8500"`, `"+8500"`, 1),
		"自报租户":         strings.Replace(receptionBody, `{"claim"`, `{"tenantId":"SYN-TENANT-02","claim"`, 1),
		"同一行两条裁决":      strings.Replace(receptionBody, `{"line":"L2","classification"`, `{"line":"L1","classification"`, 1),
		"漏一行裁决":        strings.Replace(receptionBody, `,{"line":"L2","classification":"NO_MATCHING_OCCURRENCE","basis":"SYN-BASIS-1"}`, ``, 1),
		"裁决指账单外的行":     strings.Replace(receptionBody, `{"line":"L2","classification"`, `{"line":"L9","classification"`, 1),
		"完全匹配带依据":      strings.Replace(receptionBody, `"expectedVersion":"SYN-COST-1"}`, `"expectedVersion":"SYN-COST-1","basis":"SYN-BASIS-9"}`, 1),
		"无对应发生项点名预期成本": strings.Replace(receptionBody, `"classification":"NO_MATCHING_OCCURRENCE",`, `"classification":"NO_MATCHING_OCCURRENCE","expectedVersion":"SYN-COST-1",`, 1),
		"没有行":          strings.Replace(receptionBody, `"lines":[{"line":"L1","feeItem":"SYN-FEE-1","claimed":"8500"},{"line":"L2","feeItem":"SYN-FEE-1","claimed":"300"}]`, `"lines":[]`, 1),
	} {
		before := len(receiver.calls)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, decisionRequest(body))
		if response.Code != http.StatusBadRequest || len(receiver.calls) != before {
			t.Fatalf("%s = %d，进了编排 %v", name, response.Code, len(receiver.calls) != before)
		}
	}
}

// Covers: AT-SA-226（Intake 半边）——审核口按 SUPPLIER_BILL_AUDIT 认证，命令只带四格与操作证据。
func TestTheSupplierBillAuditIntakeAuthenticatesItsOwnDecision(t *testing.T) {
	var asked []settlementhttp.OperatorDecision
	auditor := &capturingAuditor{}
	endpoint := settlementhttp.NewAuditSupplierBillEndpoint(operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: &asked}), auditor)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, decisionRequest(`{"claim":"SYN-BILL-1","version":"v1","line":"L1","payable":"SYN-PAYABLE-1"}`))
	if len(asked) != 1 || asked[0] != settlementhttp.OperatorDecisionSupplierBillAudit || len(auditor.calls) != 1 {
		t.Fatalf("认证 %v、编排被调 %d 次", asked, len(auditor.calls))
	}
	command := auditor.calls[0]
	if command.Payable.String() != "SYN-PAYABLE-1" || command.Line.String() != "L1" || command.TenantID.String() != clientTenant ||
		command.OperationEvidence == "" {
		t.Fatalf("命令 = %+v", command)
	}
	bad := httptest.NewRecorder()
	endpoint.ServeHTTP(bad, decisionRequest(`{"claim":"SYN-BILL-1","version":"v1","line":"L1","payable":"SYN-PAYABLE-1","amount":"1"}`))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("带金额 = %d，want 400", bad.Code)
	}
}
