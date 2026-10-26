package networkhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉并列裁决口与并列读面的传输形状（ADR-0309 决定二、五、六；ADR-0022）：与授权改路决定口同一种授予、同一套认证与接入各格，
// 载荷封闭，租户与决定方取认证结果。

type tieDeciderDouble struct {
	result   application.DecideInitialRouteResult
	err      error
	commands []application.DecideInitialRouteCommand
}

func (double *tieDeciderDouble) Decide(_ context.Context, command application.DecideInitialRouteCommand) (application.DecideInitialRouteResult, error) {
	double.commands = append(double.commands, command)
	return double.result, double.err
}

const tieDecisionBody = `{"declaredParcelId": "SYN-PARCEL-1", "candidate": "SYN-LINE-B@1", "basis": "SYN-APPROVAL-97"}`

func postTie(t *testing.T, handler http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/route-initial-route-decisions", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// Covers: AT-NR-103（ADR-0309 决定二、五）——口按授权改路那一种授予认证；命令的租户与决定方取认证结果，决定方记成 OPERATOR/<发行方>#<sub>；
// 载荷里自报租户或决定方、缺格、空白、不是 JSON 都答 400 坏报文，不进编排。
func TestTheTieDecisionIntakeTakesTheTenantAndTheDeciderFromTheSession(t *testing.T) {
	authenticator := &decisionAuthenticator{}
	decider := &tieDeciderDouble{}
	handler := networkhttp.NewDecideInitialRouteEndpoint(decisionIntake(t, authenticator), decider)

	postTie(t, handler, "presented.operator.token", tieDecisionBody)
	if len(authenticator.asked) != 1 || authenticator.asked[0] != networkhttp.OperatorDecisionAuthorizedReroute {
		t.Fatalf("认证所问的决定种类 = %v", authenticator.asked)
	}
	if len(decider.commands) != 1 {
		t.Fatalf("编排被调 %d 次", len(decider.commands))
	}
	command := decider.commands[0]
	if command.Tenant.String() != "SYN-TENANT-01" || command.DecidedBy.String() != "OPERATOR/https://issuer.example#ops-7" ||
		command.Parcel.String() != "SYN-PARCEL-1" || command.Candidate.String() != "SYN-LINE-B@1" ||
		command.Basis.String() != "SYN-APPROVAL-97" {
		t.Fatalf("command = %+v", command)
	}

	for name, body := range map[string]string{
		"self-reported tenant":  `{"tenantId": "SYN-TENANT-02", "declaredParcelId": "p", "candidate": "c", "basis": "b"}`,
		"self-reported decider": `{"decidedBy": "OPERATOR/x#y", "declaredParcelId": "p", "candidate": "c", "basis": "b"}`,
		"basis missing":         `{"declaredParcelId": "p", "candidate": "c"}`,
		"blank candidate":       `{"declaredParcelId": "p", "candidate": "  ", "basis": "b"}`,
		"not json":              `declare`,
	} {
		rejected := &tieDeciderDouble{}
		response := postTie(t, networkhttp.NewDecideInitialRouteEndpoint(decisionIntake(t, &decisionAuthenticator{}), rejected),
			"presented.operator.token", body)
		if response.Code != http.StatusBadRequest || errorCodeOf(t, response) != "MALFORMED_REQUEST" {
			t.Errorf("%s: %d %s", name, response.Code, response.Body.String())
		}
		if len(rejected.commands) != 0 {
			t.Errorf("%s: 坏报文进了编排", name)
		}
	}
}

// Covers: AT-NR-103（ADR-0309 决定五）——接入各格按恢复动作分码，没授予 403、准入范围外 403、没令牌 401、身份依赖读不动 503，都不进编排；
// 编排报错是没形成答案（500 NO_ANSWER_FORMED，不带 outcome），交回没有名字的结果同样 500。
func TestTheTieDecisionEndpointAnswersEachAccessGradeAndDoesNotDisguiseAFailure(t *testing.T) {
	cases := map[string]struct {
		token  string
		err    error
		status int
		code   string
	}{
		"no token":            {"", nil, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"not granted":         {"t", networkhttp.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"outside admission":   {"t", networkhttp.ErrOutsideAdmissionScope, http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
		"identity dependency": {"t", networkhttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	for name, testCase := range cases {
		decider := &tieDeciderDouble{}
		response := postTie(t, networkhttp.NewDecideInitialRouteEndpoint(decisionIntake(t, &decisionAuthenticator{err: testCase.err}), decider),
			testCase.token, tieDecisionBody)
		if response.Code != testCase.status || errorCodeOf(t, response) != testCase.code {
			t.Errorf("%s: %d %s, want %d %s", name, response.Code, response.Body.String(), testCase.status, testCase.code)
		}
		if len(decider.commands) != 0 {
			t.Errorf("%s: 没过接入就进了编排", name)
		}
	}
	failing := &tieDeciderDouble{err: errors.New("boom")}
	response := postTie(t, networkhttp.NewDecideInitialRouteEndpoint(decisionIntake(t, &decisionAuthenticator{}), failing), "t", tieDecisionBody)
	if response.Code != http.StatusInternalServerError || errorCodeOf(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("编排报错：%d %s", response.Code, response.Body.String())
	}
	unnamed := &tieDeciderDouble{}
	response = postTie(t, networkhttp.NewDecideInitialRouteEndpoint(decisionIntake(t, &decisionAuthenticator{}), unnamed), "t", tieDecisionBody)
	if response.Code != http.StatusInternalServerError || errorCodeOf(t, response) != "UNNAMED_OUTCOME" {
		t.Fatalf("没有名字的结果：%d %s", response.Code, response.Body.String())
	}
}

type tieReader struct {
	rows   []ports.InitialRouteTieCatalogueRow
	err    error
	tenant domain.TenantID
	limit  int
}

func (reader *tieReader) ListInitialRouteTies(_ context.Context, tenant domain.TenantID, limit int) ([]ports.InitialRouteTieCatalogueRow, error) {
	reader.tenant, reader.limit = tenant, limit
	return reader.rows, reader.err
}

// Covers: AT-NR-103（ADR-0309 决定六）——并列读面逐行透出判断键、全部候选、并列的那几家与打平的金额、状态，已决定的带决定；空册答空数组；
// 读不回答 500，不伪装成空册。
func TestTheTieReadFaceListsTiesWithTheirState(t *testing.T) {
	decidedAt := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	reader := &tieReader{rows: []ports.InitialRouteTieCatalogueRow{{
		CustomerAccountID: "SYN-CUST-1", ShipmentRequestID: "SYN-REQ-1", AcceptanceBaseline: "SYN-REQ-1/v1",
		DeclaredParcelID: "SYN-PARCEL-1", ServicePurpose: "NETWORK_SERVICE",
		Candidates: []ports.RerouteSuggestionCandidateRow{
			{ID: "SYN-LINE-A@1", Outcome: "QUALIFIED"}, {ID: "SYN-LINE-B@1", Outcome: "QUALIFIED"},
		},
		Tied: []ports.InitialRouteTieCostRow{
			{Candidate: "SYN-LINE-A@1", AmountMinor: 4200, Currency: "CNY"}, {Candidate: "SYN-LINE-B@1", AmountMinor: 4200, Currency: "CNY"},
		},
		AcceptedAt: decidedAt.Add(-3 * time.Hour), SettledAt: decidedAt.Add(-2 * time.Hour),
		State: ports.InitialRouteTieDecided, HasDecision: true,
		Decision: ports.InitialRouteTieDecisionRow{Candidate: "SYN-LINE-B@1", PlanVersion: "SYN-RPV-2",
			DecidedBy: "OPERATOR/https://issuer.example#ops-7", Basis: "SYN-APPROVAL-97", DecidedAt: decidedAt},
	}}}
	handler := networkhttp.NewQueryInitialRouteTiesEndpoint(readIntake{}, reader)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/route-initial-route-ties", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d %s", response.Code, response.Body.String())
	}
	text := response.Body.String()
	for _, want := range []string{
		`"outcome":"INITIAL_ROUTE_TIES_LISTED"`, `"declaredParcelId":"SYN-PARCEL-1"`, `"state":"DECIDED"`,
		`"candidate":"SYN-LINE-A@1","amountMinor":4200,"currency":"CNY"`, `"planVersion":"SYN-RPV-2"`,
		`"decidedAt":"2026-10-26T09:00:00Z"`, `"settledAt":"2026-10-26T07:00:00Z"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("响应缺 %s：%s", want, text)
		}
	}
	if reader.tenant.String() != "SYN-TENANT-01" || reader.limit != 200 {
		t.Fatalf("读口收到 tenant=%q limit=%d", reader.tenant, reader.limit)
	}

	empty := httptest.NewRecorder()
	networkhttp.NewQueryInitialRouteTiesEndpoint(readIntake{}, &tieReader{}).
		ServeHTTP(empty, httptest.NewRequest(http.MethodGet, "/route-initial-route-ties", nil))
	if !strings.Contains(empty.Body.String(), `"ties":[]`) {
		t.Fatalf("空册 = %s", empty.Body.String())
	}
	failing := httptest.NewRecorder()
	networkhttp.NewQueryInitialRouteTiesEndpoint(readIntake{}, &tieReader{err: errors.New("boom")}).
		ServeHTTP(failing, httptest.NewRequest(http.MethodGet, "/route-initial-route-ties", nil))
	if failing.Code != http.StatusInternalServerError || errorCodeOf(t, failing) != "NO_ANSWER_FORMED" {
		t.Fatalf("读不回：%d %s", failing.Code, failing.Body.String())
	}
}
