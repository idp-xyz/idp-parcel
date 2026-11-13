package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

type caseDecisionAuthenticator struct {
	asked *[]visibilityhttp.OperatorDecision
	err   error
}

func (fake caseDecisionAuthenticator) AuthenticateOperatorDecision(_ context.Context, token string, decision visibilityhttp.OperatorDecision) (visibilityhttp.OperatorIdentity, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, decision)
	}
	if token == "" {
		return visibilityhttp.OperatorIdentity{}, visibilityhttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return visibilityhttp.OperatorIdentity{}, fake.err
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	return visibilityhttp.OperatorIdentity{Tenant: tenant, Operator: "https://issuer#alice"}, err
}

// 端点测试驱动真实编排，替身只在端口层：传输层不该能捏造业务结果。
type caseStoreStub struct {
	byID      map[string]*domain.ExceptionCase
	operators map[string]string
	err       error
}

func (stub *caseStoreStub) FindCase(_ context.Context, tenant domain.TenantID, id domain.CaseID) (*domain.ExceptionCase, bool, error) {
	if stub.err != nil {
		return nil, false, stub.err
	}
	stored, found := stub.byID[tenant.String()+"|"+id.String()]
	if !found {
		return nil, false, nil
	}
	copied, err := domain.RehydrateCase(stored.Snapshot())
	return copied, err == nil, err
}

func (stub *caseStoreStub) AdvanceCase(_ context.Context, advance ports.CaseAdvance) (ports.CaseAdvanceOutcome, error) {
	key := advance.Tenant.String() + "|" + advance.Case.ID().String()
	if stored, found := stub.byID[key]; !found || stored.Phase() != advance.From {
		return ports.CasePhaseMoved, nil
	}
	stub.byID[key] = advance.Case
	stub.operators[key] = advance.Operator
	return ports.CaseAdvanced, nil
}

type caseClock struct{}

func (caseClock) Now() time.Time { return time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC) }

type caseEndpoints struct {
	takeUp, closure, merge http.Handler
	store                  *caseStoreStub
	asked                  *[]visibilityhttp.OperatorDecision
}

func caseEndpointsForTest(t *testing.T, authErr error, cases ...string) caseEndpoints {
	t.Helper()
	var asked []visibilityhttp.OperatorDecision
	intake, err := visibilityhttp.NewOperatorDecisionIntake(caseDecisionAuthenticator{asked: &asked, err: authErr})
	if err != nil {
		t.Fatal(err)
	}
	store := &caseStoreStub{byID: map[string]*domain.ExceptionCase{}, operators: map[string]string{}}
	for _, id := range cases {
		caseID, err := domain.NewCaseID(id)
		if err != nil {
			t.Fatal(err)
		}
		number, err := domain.NewDocumentNumber("SYN-EXC-" + id)
		if err != nil {
			t.Fatal(err)
		}
		root, _ := domain.NewTrackedParcelReference("SYN-PARCEL-0001")
		scope, _ := domain.NewImpactScopeReference("SYN-SCOPE/v1")
		team, _ := domain.NewResponsibleTeamReference("SYN-TEAM-01")
		established, err := domain.EstablishCase(caseID, number, root, scope, team, time.Date(2026, 11, 12, 6, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("establish %s: %v", id, err)
		}
		store.byID["SYN-TENANT-01|"+id] = established
	}
	manager, err := application.NewManageCaseHandler(application.ManageCaseDeps{Cases: store, Clock: caseClock{}})
	if err != nil {
		t.Fatal(err)
	}
	return caseEndpoints{
		takeUp:  visibilityhttp.NewTakeUpExceptionCaseEndpoint(intake, manager),
		closure: visibilityhttp.NewCloseExceptionCaseEndpoint(intake, manager),
		merge:   visibilityhttp.NewMergeExceptionCaseEndpoint(intake, manager),
		store:   store,
		asked:   &asked,
	}
}

type caseAnswer struct {
	Outcome         string `json:"outcome"`
	CaseID          string `json:"caseId"`
	Phase           string `json:"phase"`
	FirstResponseAt string `json:"firstResponseAt"`
	MergedInto      string `json:"mergedInto"`
	Error           struct {
		Code string `json:"code"`
	} `json:"error"`
}

func serveCase(t *testing.T, handler http.Handler, token, body string) (int, caseAnswer) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var answer caseAnswer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, recorder.Body.String())
	}
	return recorder.Code, answer
}

// Covers: 异常案件的三个运营决定口（UC-VE-005）——各按自己那一种决定认证；接单形成首次响应时间并记下认证出来的操作者
// （201），再接一次答已在处理中（200）；重复案件并进主案件（201）并指向它；以结论关闭主案件（201）。
func TestCaseFacesTakeUpMergeAndCloseUnderTheirOwnDecisionKinds(t *testing.T) {
	endpoints := caseEndpointsForTest(t, nil, "SYN-CASE-MAIN", "SYN-CASE-DUP")

	status, answer := serveCase(t, endpoints.takeUp, "t", `{"caseId":"SYN-CASE-MAIN"}`)
	if status != http.StatusCreated || answer.Outcome != "TAKEN_UP" || answer.Phase != "IN_PROGRESS" || answer.FirstResponseAt != "2026-11-13T02:00:00Z" {
		t.Fatalf("接单：status %d answer %+v", status, answer)
	}
	if operator := endpoints.store.operators["SYN-TENANT-01|SYN-CASE-MAIN"]; operator != "https://issuer#alice" {
		t.Fatalf("接单人 = %q——该是认证出来的操作者", operator)
	}
	if status, answer = serveCase(t, endpoints.takeUp, "t", `{"caseId":"SYN-CASE-MAIN"}`); status != http.StatusOK || answer.Outcome != "ALREADY_IN_PROGRESS" {
		t.Fatalf("再接单：status %d answer %+v", status, answer)
	}

	status, answer = serveCase(t, endpoints.merge, "t", `{"caseId":"SYN-CASE-DUP","mainCaseId":"SYN-CASE-MAIN"}`)
	if status != http.StatusCreated || answer.Outcome != "MERGED" || answer.Phase != "CLOSED" || answer.MergedInto != "SYN-CASE-MAIN" {
		t.Fatalf("归并：status %d answer %+v", status, answer)
	}

	status, answer = serveCase(t, endpoints.closure, "t", `{"caseId":"SYN-CASE-MAIN","conclusion":"已恢复：改派后妥投"}`)
	if status != http.StatusCreated || answer.Outcome != "CLOSED" || answer.Phase != "CLOSED" {
		t.Fatalf("关闭：status %d answer %+v", status, answer)
	}

	asked := *endpoints.asked
	if len(asked) != 4 || asked[0] != "EXCEPTION_CASE_TAKE_UP" || asked[2] != "EXCEPTION_CASE_MERGE" || asked[3] != "EXCEPTION_CASE_CLOSURE" {
		t.Fatalf("asked = %v——三口要各按自己那一种决定认证", asked)
	}
}

// Covers: 三口的拒绝各格——载荷是封闭的，不收租户、操作者与时刻（400）；令牌缺失 401；没有这一种决定的授予、不在准入
// 范围 403；身份依赖读不动 503；不在册的案件、主案件不合格、没有结论是业务答复（200），案件册不动；案件册读不动是
// 没形成答案（500）。
func TestCaseFacesGradeTheirRefusals(t *testing.T) {
	for name, test := range map[string]struct {
		face     string
		token    string
		body     string
		authErr  error
		storeErr bool
		status   int
		outcome  string
		code     string
	}{
		"接单带租户":   {face: "takeUp", token: "t", body: `{"tenantId":"T","caseId":"SYN-CASE-1"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"接单带操作者":  {face: "takeUp", token: "t", body: `{"operator":"bob","caseId":"SYN-CASE-1"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"关闭带时刻":   {face: "closure", token: "t", body: `{"caseId":"SYN-CASE-1","conclusion":"x","closedAt":"2026-01-01T00:00:00Z"}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"缺案件标识":   {face: "takeUp", token: "t", body: `{}`, status: http.StatusBadRequest, code: "MALFORMED_REQUEST"},
		"令牌缺失":    {face: "takeUp", body: `{"caseId":"SYN-CASE-1"}`, status: http.StatusUnauthorized, code: "OPERATOR_CREDENTIAL_REJECTED"},
		"没有这一种授予": {face: "closure", token: "t", body: `{"caseId":"SYN-CASE-1","conclusion":"x"}`, authErr: visibilityhttp.ErrOperatorNotGranted, status: http.StatusForbidden, code: "OPERATOR_NOT_GRANTED"},
		"不在准入范围":  {face: "takeUp", token: "t", body: `{"caseId":"SYN-CASE-1"}`, authErr: visibilityhttp.ErrOutsideAdmissionScope, status: http.StatusForbidden, code: "OUTSIDE_ADMISSION_SCOPE"},
		"身份依赖读不动": {face: "takeUp", token: "t", body: `{"caseId":"SYN-CASE-1"}`, authErr: visibilityhttp.ErrIdentityDependencyUnavailable, status: http.StatusServiceUnavailable, code: "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"案件不在册":   {face: "takeUp", token: "t", body: `{"caseId":"SYN-CASE-9"}`, status: http.StatusOK, outcome: "NOT_FOUND"},
		"主案件不在册":  {face: "merge", token: "t", body: `{"caseId":"SYN-CASE-1","mainCaseId":"SYN-CASE-9"}`, status: http.StatusOK, outcome: "MAIN_CASE_NOT_ELIGIBLE"},
		"关闭没有结论":  {face: "closure", token: "t", body: `{"caseId":"SYN-CASE-1","conclusion":" "}`, status: http.StatusOK, outcome: "NOT_ACCEPTED"},
		"案件册读不动":  {face: "takeUp", token: "t", body: `{"caseId":"SYN-CASE-1"}`, storeErr: true, status: http.StatusInternalServerError, code: "NO_ANSWER_FORMED"},
	} {
		t.Run(name, func(t *testing.T) {
			endpoints := caseEndpointsForTest(t, test.authErr, "SYN-CASE-1")
			if test.storeErr {
				endpoints.store.err = errors.New("connection refused")
			}
			handler := map[string]http.Handler{"takeUp": endpoints.takeUp, "closure": endpoints.closure, "merge": endpoints.merge}[test.face]
			status, answer := serveCase(t, handler, test.token, test.body)
			if status != test.status || answer.Outcome != test.outcome || answer.Error.Code != test.code {
				t.Fatalf("status %d outcome %q code %q, want %d %q %q", status, answer.Outcome, answer.Error.Code, test.status, test.outcome, test.code)
			}
			if endpoints.store.byID["SYN-TENANT-01|SYN-CASE-1"].Phase() != domain.CaseAwaitingResponse {
				t.Fatal("被拒的请求动了案件")
			}
		})
	}
}
