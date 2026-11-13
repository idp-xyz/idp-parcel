package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// disclosureDeciderStub 交回预置的答复并记下收到的命令：这里验的是线格式与转写，编排自己的行为在 application 的测试里。
type disclosureDeciderStub struct {
	result         application.DecideDisclosureResult
	err            error
	decided        []application.DecideDisclosureCommand
	authorizations []application.AuthorizeDisclosureCommand
}

func (stub *disclosureDeciderStub) Handle(_ context.Context, command application.DecideDisclosureCommand) (application.DecideDisclosureResult, error) {
	stub.decided = append(stub.decided, command)
	return stub.result, stub.err
}

func (stub *disclosureDeciderStub) Authorize(_ context.Context, command application.AuthorizeDisclosureCommand) (application.DecideDisclosureResult, error) {
	stub.authorizations = append(stub.authorizations, command)
	return stub.result, stub.err
}

type disclosureAnswer struct {
	Outcome, Reason, Conclusion, Customer, Policy, Content, DecidedAt string
	Error                                                             struct{ Code string }
}

func disclosureEndpointsForTest(t *testing.T, authErr error) (http.Handler, http.Handler, *disclosureDeciderStub, *[]visibilityhttp.OperatorDecision) {
	t.Helper()
	asked := []visibilityhttp.OperatorDecision{}
	intake, err := visibilityhttp.NewOperatorDecisionIntake(caseDecisionAuthenticator{asked: &asked, err: authErr})
	if err != nil {
		t.Fatal(err)
	}
	stub := &disclosureDeciderStub{}
	return visibilityhttp.NewDecideCustomerDisclosureEndpoint(intake, stub), visibilityhttp.NewAuthorizeCustomerDisclosureEndpoint(intake, stub), stub, &asked
}

func serveDisclosure(t *testing.T, handler http.Handler, token, body string) (int, disclosureAnswer) {
	t.Helper()
	status, raw := serveRaw(t, handler, token, body)
	var answer disclosureAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, raw)
	}
	return status, answer
}

// Covers: 披露的两个运营决定口各按自己那一种决定认证，租户与操作者只取认证结果，载荷封闭且只指名包裹与信号类型。
func TestDisclosureFacesAuthenticateUnderTheirOwnDecisionKindsAndTakeIdentityFromAuthentication(t *testing.T) {
	decide, authorize, stub, asked := disclosureEndpointsForTest(t, nil)
	stub.err = errors.New("编排不在本用例的范围")

	body := `{"parcel":"SYN-PARCEL-1","signalKind":"ETA_BREACH_RISK"}`
	if status, _ := serveRaw(t, decide, "t", body); status != http.StatusInternalServerError {
		t.Fatalf("编排报错该答 500，实得 %d", status)
	}
	if status, _ := serveRaw(t, authorize, "t", body); status != http.StatusInternalServerError {
		t.Fatalf("编排报错该答 500，实得 %d", status)
	}
	if len(*asked) != 2 || (*asked)[0] != visibilityhttp.OperatorDecisionCustomerDisclosureDecision ||
		(*asked)[1] != visibilityhttp.OperatorDecisionCustomerDisclosureAuthorization {
		t.Fatalf("认证问的决定种类 = %v", *asked)
	}
	if len(stub.decided) != 1 || len(stub.authorizations) != 1 {
		t.Fatalf("编排收到 %d / %d 次", len(stub.decided), len(stub.authorizations))
	}
	decided, authorized := stub.decided[0], stub.authorizations[0]
	if decided.TenantID.String() != "SYN-TENANT-01" || decided.Operator != "https://issuer#alice" ||
		decided.Parcel.String() != "SYN-PARCEL-1" || decided.Kind.String() != "ETA_BREACH_RISK" {
		t.Fatalf("披露判断命令 = %+v", decided)
	}
	if authorized.TenantID.String() != "SYN-TENANT-01" || authorized.Operator != "https://issuer#alice" || authorized.Parcel.String() != "SYN-PARCEL-1" {
		t.Fatalf("披露授权命令 = %+v", authorized)
	}

	for name, malformed := range map[string]string{
		"带了结论":   `{"parcel":"SYN-PARCEL-1","signalKind":"ETA_BREACH_RISK","conclusion":"DISCLOSE"}`,
		"带了租户":   `{"parcel":"SYN-PARCEL-1","signalKind":"ETA_BREACH_RISK","tenantId":"other"}`,
		"带了内容":   `{"parcel":"SYN-PARCEL-1","signalKind":"ETA_BREACH_RISK","content":"x"}`,
		"没有包裹":   `{"signalKind":"ETA_BREACH_RISK"}`,
		"没有类型":   `{"parcel":"SYN-PARCEL-1"}`,
		"不是JSON": `parcel=1`,
	} {
		for _, endpoint := range []http.Handler{decide, authorize} {
			if status, _ := serveRaw(t, endpoint, "t", malformed); status != http.StatusBadRequest {
				t.Fatalf("%s 该答 400，实得 %d", name, status)
			}
		}
	}
	if len(stub.decided) != 1 || len(stub.authorizations) != 1 {
		t.Fatal("坏载荷到了编排")
	}
	if status, _ := serveRaw(t, decide, "", body); status != http.StatusUnauthorized {
		t.Fatalf("没有凭据该答 401，实得 %d", status)
	}
	_, refused, refusedStub, _ := disclosureEndpointsForTest(t, visibilityhttp.ErrOutsideAdmissionScope)
	if status, _ := serveRaw(t, refused, "t", body); status != http.StatusForbidden || len(refusedStub.authorizations) != 0 {
		t.Fatalf("不在准入范围该答 403 且不到编排，实得 %d", status)
	}
}

// Covers: 答复的转写（ADR-0022）——决定真落了 201 并带结论、客户、规则引用、内容与决定时间；已有结果与业务答案 200；
// 规则未配置与客户归属不可确定是业务答案（200 带原因）；依赖读写不动的未决是没形成答案（500）。
func TestDisclosureAnswersAreTranscribedByOutcome(t *testing.T) {
	fixture := newDisclosureOrchestration(t)
	decide, authorize := fixture.decide, fixture.authorize
	body := `{"parcel":"parcel-1","signalKind":"ETA_BREACH_RISK"}`

	fixture.rules.configured = false
	if status, answer := serveDisclosure(t, decide, "t", body); status != http.StatusOK || answer.Outcome != "UNDECIDED" || answer.Reason != "DISCLOSURE_RULE_NOT_CONFIGURED" {
		t.Fatalf("规则未配置：status %d answer %+v", status, answer)
	}
	fixture.rules.configured = true
	fixture.rules.err = errors.New("rules down")
	if status, answer := serveDisclosure(t, decide, "t", body); status != http.StatusInternalServerError || answer.Error.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("规则读不动：status %d answer %+v", status, answer)
	}
	fixture.rules.err = nil

	if status, answer := serveDisclosure(t, authorize, "t", body); status != http.StatusOK || answer.Outcome != "NOT_AWAITING_AUTHORIZATION" {
		t.Fatalf("没判过就确认：status %d answer %+v", status, answer)
	}
	status, answer := serveDisclosure(t, decide, "t", body)
	if status != http.StatusCreated || answer.Outcome != "DECIDED" || answer.Conclusion != "AWAITING_AUTHORIZATION" ||
		answer.Customer != "customer-1" || answer.Policy != "exception-disclosure/v1" || answer.Content != "" || answer.DecidedAt != "2026-11-13T02:00:00Z" {
		t.Fatalf("判出待授权：status %d answer %+v", status, answer)
	}
	if status, answer = serveDisclosure(t, decide, "t", body); status != http.StatusOK || answer.Outcome != "EXISTING_RESULT" || answer.Conclusion != "AWAITING_AUTHORIZATION" {
		t.Fatalf("重判：status %d answer %+v", status, answer)
	}
	fixture.clock.at = fixture.clock.at.Add(time.Minute)
	status, answer = serveDisclosure(t, authorize, "t", body)
	if status != http.StatusCreated || answer.Outcome != "AUTHORIZED" || answer.Conclusion != "DISCLOSE" ||
		answer.Content != "disclosure-content/eta-breach/v1" || answer.DecidedAt != "2026-11-13T02:01:00Z" {
		t.Fatalf("确认发布：status %d answer %+v", status, answer)
	}
	if status, answer = serveDisclosure(t, authorize, "t", body); status != http.StatusOK || answer.Outcome != "EXISTING_RESULT" || answer.Conclusion != "DISCLOSE" {
		t.Fatalf("再确认：status %d answer %+v", status, answer)
	}
	if status, answer = serveDisclosure(t, decide, "t", `{"parcel":"parcel-none","signalKind":"ETA_BREACH_RISK"}`); status != http.StatusOK || answer.Outcome != "NOT_ACCEPTED" {
		t.Fatalf("没有信号的包裹：status %d answer %+v", status, answer)
	}
	fixture.customers.found = false
	if status, answer = serveDisclosure(t, decide, "t", body); status != http.StatusOK || answer.Reason != "DISCLOSURE_CUSTOMER_ACCOUNT_UNDETERMINABLE" {
		t.Fatalf("客户归属不可确定：status %d answer %+v", status, answer)
	}
}
