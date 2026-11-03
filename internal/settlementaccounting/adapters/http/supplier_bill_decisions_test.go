package settlementhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
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

// Covers: AT-SA-226（Intake 半边）——审核口先把审核应付与争议决定两种授予都认一遍（ADR-0357 决定五），旧形载荷靠前一种进来，
// 命令只带四格与操作证据。
func TestTheSupplierBillAuditIntakeAuthenticatesItsOwnDecision(t *testing.T) {
	var asked []settlementhttp.OperatorDecision
	auditor := &capturingAuditor{}
	endpoint := settlementhttp.NewAuditSupplierBillEndpoint(operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: &asked}), auditor)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, decisionRequest(`{"claim":"SYN-BILL-1","version":"v1","line":"L1","payable":"SYN-PAYABLE-1"}`))
	if len(asked) != 2 || asked[0] != settlementhttp.OperatorDecisionSupplierBillAudit ||
		asked[1] != settlementhttp.OperatorDecisionSupplierBillDisputeDecision || len(auditor.calls) != 1 {
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

// Covers: AT-SA-233（Intake 半边）——接收口收可缺的 supplierBillNumber，逐字到主张上；不带这一格的旧载荷照收、主张上没有号；
// 写成空白答 400。
func TestTheSupplierBillReceptionIntakeCarriesTheSupplierBillNumber(t *testing.T) {
	receiver := &capturingReceiver{}
	endpoint := settlementhttp.NewReceiveSupplierBillEndpoint(operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: new([]settlementhttp.OperatorDecision)}), receiver)

	endpoint.ServeHTTP(httptest.NewRecorder(), decisionRequest(strings.Replace(receptionBody, `"supplier":"SYN-CARRIER-1",`, `"supplier":"SYN-CARRIER-1","supplierBillNumber":"INV-2610-001",`, 1)))
	endpoint.ServeHTTP(httptest.NewRecorder(), decisionRequest(receptionBody))
	if len(receiver.calls) != 2 {
		t.Fatalf("编排被调 %d 次", len(receiver.calls))
	}
	if receiver.calls[0].Claim.BillNumber.String() != "INV-2610-001" || receiver.calls[1].Claim.BillNumber.String() != "" {
		t.Fatalf("账单号 = %q / %q", receiver.calls[0].Claim.BillNumber, receiver.calls[1].Claim.BillNumber)
	}
	blank := httptest.NewRecorder()
	endpoint.ServeHTTP(blank, decisionRequest(strings.Replace(receptionBody, `"supplier":"SYN-CARRIER-1",`, `"supplier":"SYN-CARRIER-1","supplierBillNumber":"  ",`, 1)))
	if blank.Code != http.StatusBadRequest || len(receiver.calls) != 2 {
		t.Fatalf("空白账单号 = %d", blank.Code)
	}
}

// Covers: AT-SA-234、AT-SA-235（Intake 半边）——审核口的新形载荷逐字到命令：决定标识、结果、范围、依据与所解争议；旧形照收。两种形状
// 混给、结果写错、范围倒置或带符号、争议与拒绝不带依据、再审写成争议，都答 400、不进编排。
func TestTheSupplierBillAuditIntakeTranslatesRangedDecisions(t *testing.T) {
	auditor := &capturingAuditor{}
	endpoint := settlementhttp.NewAuditSupplierBillEndpoint(operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: new([]settlementhttp.OperatorDecision)}), auditor)
	const ranged = `{"claim":"SYN-BILL-1","version":"v1","line":"L1","decision":"AUDIT-1","verdict":"REJECTED","rangeFrom":"6000","rangeTo":"8500",` +
		`"basis":"SYN-REASON-1","resolvesDispute":"AUDIT-0"}`
	endpoint.ServeHTTP(httptest.NewRecorder(), decisionRequest(ranged))
	if len(auditor.calls) != 1 {
		t.Fatalf("编排被调 %d 次", len(auditor.calls))
	}
	command := auditor.calls[0]
	if command.Decision.String() != "AUDIT-1" || command.Verdict.String() != "REJECTED" || command.Range.From() != 6000 ||
		command.Range.To() != 8500 || command.Basis.String() != "SYN-REASON-1" || command.Resolves.String() != "AUDIT-0" ||
		command.Payable.String() != "" || command.OperationEvidence == "" {
		t.Fatalf("命令 = %+v", command)
	}

	for name, body := range map[string]string{
		"两种形状混给":   strings.Replace(ranged, `"decision":"AUDIT-1"`, `"payable":"SYN-PAYABLE-1","decision":"AUDIT-1"`, 1),
		"旧形带了结果":   `{"claim":"SYN-BILL-1","version":"v1","line":"L1","payable":"SYN-PAYABLE-1","verdict":"PASSED"}`,
		"结果写错":     strings.Replace(ranged, `"REJECTED"`, `"APPROVED"`, 1),
		"范围倒置":     strings.Replace(ranged, `"rangeFrom":"6000"`, `"rangeFrom":"9000"`, 1),
		"范围为空":     strings.Replace(ranged, `"rangeFrom":"6000"`, `"rangeFrom":"8500"`, 1),
		"起点带符号":    strings.Replace(ranged, `"rangeFrom":"6000"`, `"rangeFrom":"-1"`, 1),
		"拒绝不带依据":   strings.Replace(ranged, `"basis":"SYN-REASON-1",`, ``, 1),
		"再审写成争议":   strings.Replace(ranged, `"REJECTED"`, `"DISPUTED"`, 1),
		"没有决定标识":   strings.Replace(ranged, `"decision":"AUDIT-1",`, ``, 1),
		"没有范围终点":   strings.Replace(ranged, `,"rangeTo":"8500"`, ``, 1),
		"决定标识写成空白": strings.Replace(ranged, `"AUDIT-1"`, `" "`, 1),
	} {
		before := len(auditor.calls)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, decisionRequest(body))
		if response.Code != http.StatusBadRequest || len(auditor.calls) != before {
			t.Fatalf("%s = %d，进了编排 %v", name, response.Code, len(auditor.calls) != before)
		}
	}

	passed := `{"claim":"SYN-BILL-1","version":"v1","line":"L1","decision":"AUDIT-2","verdict":"PASSED","rangeFrom":"0","rangeTo":"6000"}`
	endpoint.ServeHTTP(httptest.NewRecorder(), decisionRequest(passed))
	if last := auditor.calls[len(auditor.calls)-1]; last.Decision.String() != "AUDIT-2" || last.Range.From() != 0 || last.Basis.String() != "" {
		t.Fatalf("直接通过不带依据也收：%+v", last)
	}
}

// grantingAuthenticator 只认 granted 里的那几种决定；refusals 给某一种指定拒因，没指定的答未授予；asked 依次记下问过哪几种。
type grantingAuthenticator struct {
	granted  map[settlementhttp.OperatorDecision]bool
	refusals map[settlementhttp.OperatorDecision]error
	asked    *[]settlementhttp.OperatorDecision
}

func (fake grantingAuthenticator) AuthenticateOperatorDecision(
	ctx context.Context,
	bearerToken string,
	decision settlementhttp.OperatorDecision,
) (settlementhttp.OperatorIdentity, error) {
	*fake.asked = append(*fake.asked, decision)
	if err := fake.refusals[decision]; err != nil {
		return settlementhttp.OperatorIdentity{}, err
	}
	if !fake.granted[decision] {
		return settlementhttp.OperatorIdentity{}, settlementhttp.ErrOperatorNotGranted
	}
	return decisionAuthenticator{}.AuthenticateOperatorDecision(ctx, bearerToken, decision)
}

// Covers: AT-SA-234、AT-SA-235（Intake 半边，ADR-0357 决定二、五）——审核口先认持两种授予之一，再判形状，再按结果核所需授予：只持
// 审核应付的人给争议、再审答 403，通过、拒绝、旧形进编排；只持争议决定的人给争议进编排，通过、拒绝、再审、旧形答 403；只持一种
// 授予的人送来拼错的结果、小写的结果或读不出的请求体答 400，不答 403；两种都不持答 403，请求体不读。
func TestTheSupplierBillAuditIntakeChecksTheGrantsTheVerdictNeeds(t *testing.T) {
	const prefix = `{"claim":"SYN-BILL-1","version":"v1","line":"L1",`
	bodies := map[string]string{
		"dispute": prefix + `"decision":"A1","verdict":"DISPUTED","rangeFrom":"0","rangeTo":"10","basis":"b"}`,
		"pass":    prefix + `"decision":"A2","verdict":"PASSED","rangeFrom":"0","rangeTo":"10"}`,
		"reject":  prefix + `"decision":"A3","verdict":"REJECTED","rangeFrom":"0","rangeTo":"10","basis":"b"}`,
		"reaudit": prefix + `"decision":"A4","verdict":"PASSED","rangeFrom":"0","rangeTo":"10","basis":"b","resolvesDispute":"A1"}`,
		"legacy":  prefix + `"payable":"P1"}`,
		"typo":    prefix + `"decision":"A5","verdict":"DISPUTE","rangeFrom":"0","rangeTo":"10","basis":"b"}`,
		"lower":   prefix + `"decision":"A6","verdict":"disputed","rangeFrom":"0","rangeTo":"10","basis":"b"}`,
		"broken":  `{"claim":`,
	}
	audit, dispute := settlementhttp.OperatorDecisionSupplierBillAudit, settlementhttp.OperatorDecisionSupplierBillDisputeDecision
	holders := map[string]map[settlementhttp.OperatorDecision]bool{
		"audit only":   {audit: true},
		"dispute only": {dispute: true},
		"both":         {audit: true, dispute: true},
		"neither":      {},
	}
	want := map[string]map[string]int{
		"audit only":   {"dispute": 403, "pass": 0, "reject": 0, "reaudit": 403, "legacy": 0, "typo": 400, "lower": 400, "broken": 400},
		"dispute only": {"dispute": 0, "pass": 403, "reject": 403, "reaudit": 403, "legacy": 403, "typo": 400, "lower": 400, "broken": 400},
		"both":         {"dispute": 0, "pass": 0, "reject": 0, "reaudit": 0, "legacy": 0, "typo": 400, "lower": 400, "broken": 400},
		"neither":      {"dispute": 403, "pass": 403, "reject": 403, "reaudit": 403, "legacy": 403, "typo": 403, "lower": 403, "broken": 403},
	}
	for holder, granted := range holders {
		for name, body := range bodies {
			t.Run(holder+"/"+name, func(t *testing.T) {
				asked := []settlementhttp.OperatorDecision{}
				auditor := &capturingAuditor{}
				endpoint := settlementhttp.NewAuditSupplierBillEndpoint(operatorDecisionIntakeForTest(t, grantingAuthenticator{granted: granted, asked: &asked}), auditor)
				response := httptest.NewRecorder()
				endpoint.ServeHTTP(response, decisionRequest(body))
				expected := want[holder][name]
				if expected == 0 {
					if len(auditor.calls) != 1 {
						t.Fatalf("要进编排，实得 %d %s（问过 %v）", response.Code, response.Body.String(), asked)
					}
					return
				}
				if response.Code != expected || len(auditor.calls) != 0 {
					t.Fatalf("status = %d，want %d；进了编排 %v（问过 %v）", response.Code, expected, len(auditor.calls) != 0, asked)
				}
			})
		}
	}
}

// Covers: AT-SA-234（Intake 半边，ADR-0357 决定五）——两种授予各自的拒因不被另一种吞掉：只持争议决定的人，争议那种的准入未登答
// 403 OUTSIDE_ADMISSION_SCOPE、身份依赖不可用答 503，不折成 403 未授予；两种都持而审核那种准入未登的人给争议照样进编排（争议只要
// 争议决定那一种），给通过答审核那种自己的拒因 403 OUTSIDE_ADMISSION_SCOPE。
func TestTheSupplierBillAuditIntakeKeepsEachGrantsOwnRefusal(t *testing.T) {
	const prefix = `{"claim":"SYN-BILL-1","version":"v1","line":"L1",`
	disputeBody := prefix + `"decision":"A1","verdict":"DISPUTED","rangeFrom":"0","rangeTo":"10","basis":"b"}`
	passBody := prefix + `"decision":"A2","verdict":"PASSED","rangeFrom":"0","rangeTo":"10"}`
	audit, dispute := settlementhttp.OperatorDecisionSupplierBillAudit, settlementhttp.OperatorDecisionSupplierBillDisputeDecision
	cases := []struct {
		name     string
		granted  map[settlementhttp.OperatorDecision]bool
		refusals map[settlementhttp.OperatorDecision]error
		body     string
		status   int
		code     string
	}{
		{"dispute holder, dispute outside admission", map[settlementhttp.OperatorDecision]bool{dispute: true},
			map[settlementhttp.OperatorDecision]error{dispute: settlementhttp.ErrOutsideAdmissionScope}, disputeBody, http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
		{"dispute holder, identity dependency down", map[settlementhttp.OperatorDecision]bool{dispute: true},
			map[settlementhttp.OperatorDecision]error{dispute: settlementhttp.ErrIdentityDependencyUnavailable}, disputeBody, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		{"both held, audit outside admission, dispute", map[settlementhttp.OperatorDecision]bool{audit: true, dispute: true},
			map[settlementhttp.OperatorDecision]error{audit: settlementhttp.ErrOutsideAdmissionScope}, disputeBody, 0, ""},
		{"both held, audit outside admission, pass", map[settlementhttp.OperatorDecision]bool{audit: true, dispute: true},
			map[settlementhttp.OperatorDecision]error{audit: settlementhttp.ErrOutsideAdmissionScope}, passBody, http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			asked := []settlementhttp.OperatorDecision{}
			auditor := &capturingAuditor{}
			intake := operatorDecisionIntakeForTest(t, grantingAuthenticator{granted: testCase.granted, refusals: testCase.refusals, asked: &asked})
			response := httptest.NewRecorder()
			settlementhttp.NewAuditSupplierBillEndpoint(intake, auditor).ServeHTTP(response, decisionRequest(testCase.body))
			if testCase.status == 0 {
				if len(auditor.calls) != 1 {
					t.Fatalf("要进编排，实得 %d %s", response.Code, response.Body.String())
				}
				return
			}
			if response.Code != testCase.status || problemCode(t, response) != testCase.code || len(auditor.calls) != 0 {
				t.Fatalf("答 %d %q，want %d %q；进了编排 %v", response.Code, problemCode(t, response), testCase.status, testCase.code, len(auditor.calls) != 0)
			}
		})
	}
}

// Covers: AT-SA-233（Intake 半边，ADR-0357 决定一）——账单号逐字收：首尾带空白或含控制字符答 400、不进编排；大小写照收不折算。
func TestTheSupplierBillReceptionIntakeTakesTheBillNumberVerbatim(t *testing.T) {
	receiver := &capturingReceiver{}
	endpoint := settlementhttp.NewReceiveSupplierBillEndpoint(operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: new([]settlementhttp.OperatorDecision)}), receiver)
	withNumber := func(number string) string {
		return strings.Replace(receptionBody, `"supplier":"SYN-CARRIER-1",`, `"supplier":"SYN-CARRIER-1","supplierBillNumber":`+number+`,`, 1)
	}
	for name, number := range map[string]string{
		"trailing space":  `"INV-9 "`,
		"leading space":   `" INV-9"`,
		"tab inside":      `"INV\t9"`,
		"newline at end":  `"INV-9\n"`,
		"control char":    `"INV-\u00079"`,
		"full-width tail": `"INV-9　"`,
		"zero width":      `"INV\u200b-9"`,
		"byte order mark": `"\ufeffINV-9"`,
		"bidi override":   `"INV-9\u202e"`,
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, decisionRequest(withNumber(number)))
		if response.Code != http.StatusBadRequest || len(receiver.calls) != 0 {
			t.Fatalf("%s = %d，进了编排 %v", name, response.Code, len(receiver.calls) != 0)
		}
	}
	endpoint.ServeHTTP(httptest.NewRecorder(), decisionRequest(withNumber(`"inv-9"`)))
	if len(receiver.calls) != 1 || receiver.calls[0].Claim.BillNumber.String() != "inv-9" {
		t.Fatalf("小写照收不折算：%+v", receiver.calls)
	}
}

// claimSupplierStore 是只走到「这份主张原先是哪家供应商的」那一步的接收库：其余方法没有实现，走到就是编排次序变了。
type claimSupplierStore struct {
	ports.BillReceptionStore
	supplier string
}

func (store claimSupplierStore) LockBillNumber(context.Context, domain.TenantID, domain.SupplierPartyReference, domain.SupplierBillNumber, domain.BillClaimID) error {
	return nil
}

func (store claimSupplierStore) FindClaimSupplier(context.Context, domain.TenantID, domain.BillClaimID) (domain.SupplierPartyReference, bool, error) {
	supplier, err := domain.NewSupplierPartyReference(store.supplier)
	return supplier, err == nil, err
}

// Covers: AT-SA-233（端点半边，ADR-0357 决定一）——同一份主张换了供应商的来件答 200 CLAIM_SUPPLIER_CONFLICT，答复体带出原先的供应商
// registeredSupplier，不带账单号冲突的两格。
func TestAClaimSupplierConflictAnswersTheOriginalSupplier(t *testing.T) {
	handler := application.NewReceiveSupplierBillHandler(application.ReceiveSupplierBillDeps{Receptions: claimSupplierStore{supplier: "SYN-CARRIER-0"}})
	endpoint := settlementhttp.NewReceiveSupplierBillEndpoint(operatorDecisionIntakeForTest(t, decisionAuthenticator{asked: new([]settlementhttp.OperatorDecision)}), handler)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, decisionRequest(receptionBody))
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || body["outcome"] != "CLAIM_SUPPLIER_CONFLICT" || body["registeredSupplier"] != "SYN-CARRIER-0" ||
		body["registeredClaim"] != nil || body["registeredSupplierBillNumber"] != nil {
		t.Fatalf("答复 = %d %s", response.Code, response.Body.String())
	}
}
