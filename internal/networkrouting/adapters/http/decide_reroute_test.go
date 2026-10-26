package networkhttp_test

import (
	"context"
	"encoding/json"
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

// 本文件钉授权改路决定口与改路建议读面的传输形状（ADR-0284 决定一、六；ADR-0022）：租户与决定方取认证结果，载荷封闭；接入各格
// 按恢复动作分码；决定成立 201，其余业务答案 200，编排报错 500 不带 outcome。

// decisionAuthenticator 替操作者认证：不带令牌答凭证不过；带了令牌答 err，err 为空即认证到 SYN-TENANT-01 的 ops-7。记下被问的
// 决定种类。
type decisionAuthenticator struct {
	err   error
	asked []networkhttp.OperatorDecision
}

func (fake *decisionAuthenticator) AuthenticateOperatorDecision(
	_ context.Context, token string, decision networkhttp.OperatorDecision,
) (networkhttp.OperatorIdentity, error) {
	fake.asked = append(fake.asked, decision)
	if token == "" {
		return networkhttp.OperatorIdentity{}, networkhttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return networkhttp.OperatorIdentity{}, fake.err
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	return networkhttp.OperatorIdentity{Tenant: tenant, Operator: "https://issuer.example#ops-7"}, err
}

type deciderDouble struct {
	result   application.DecideRerouteResult
	err      error
	commands []application.DecideRerouteCommand
}

func (double *deciderDouble) Decide(_ context.Context, command application.DecideRerouteCommand) (application.DecideRerouteResult, error) {
	double.commands = append(double.commands, command)
	return double.result, double.err
}

const rerouteDecisionBody = `{"declaredParcelId": "SYN-PARCEL-1", "suggestion": "handover/1:a", "candidate": "SYN-LINE-B@1", "basis": "SYN-APPROVAL-7"}`

func postDecision(t *testing.T, handler http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/route-reroute-decisions", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decisionIntake(t *testing.T, authenticator networkhttp.OperatorDecisionAuthenticator) *networkhttp.OperatorDecisionIntake {
	t.Helper()
	intake, err := networkhttp.NewOperatorDecisionIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func errorCodeOf(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Outcome string `json:"outcome"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v %s", err, response.Body.String())
	}
	if body.Outcome != "" {
		t.Fatalf("问题响应带了 outcome %q（ADR-0022）", body.Outcome)
	}
	return body.Error.Code
}

// Covers: AT-NR-098（ADR-0284 决定一）——口按「授权改路」那一种认证；命令的租户与决定方取认证结果，决定方记成 OPERATOR/<发行方>#<sub>；其余
// 各格取载荷。
func TestTheRerouteDecisionIntakeTakesTheTenantAndTheDeciderFromTheSession(t *testing.T) {
	authenticator := &decisionAuthenticator{}
	decider := &deciderDouble{result: application.DecideRerouteResult{}}
	handler := networkhttp.NewDecideRerouteEndpoint(decisionIntake(t, authenticator), decider)

	postDecision(t, handler, "presented.operator.token", rerouteDecisionBody)
	if len(authenticator.asked) != 1 || authenticator.asked[0] != networkhttp.OperatorDecisionAuthorizedReroute {
		t.Fatalf("认证所问的决定种类 = %v", authenticator.asked)
	}
	if len(decider.commands) != 1 {
		t.Fatalf("编排被调 %d 次", len(decider.commands))
	}
	command := decider.commands[0]
	if command.Tenant.String() != "SYN-TENANT-01" || command.DecidedBy.String() != "OPERATOR/https://issuer.example#ops-7" ||
		command.Parcel.String() != "SYN-PARCEL-1" || command.Suggestion.String() != "handover/1:a" ||
		command.Candidate.String() != "SYN-LINE-B@1" || command.Basis.String() != "SYN-APPROVAL-7" {
		t.Fatalf("command = %+v", command)
	}
}

// Covers: AT-NR-098（ADR-0284 决定一）——载荷封闭：自报租户或决定方、缺格、空白、不是 JSON 都答 400 坏报文，不进编排。
func TestTheRerouteDecisionPayloadIsClosed(t *testing.T) {
	for name, body := range map[string]string{
		"self-reported tenant":  `{"tenantId": "SYN-TENANT-02", "declaredParcelId": "SYN-PARCEL-1", "suggestion": "s", "candidate": "c", "basis": "b"}`,
		"self-reported decider": `{"decidedBy": "OPERATOR/x#y", "declaredParcelId": "SYN-PARCEL-1", "suggestion": "s", "candidate": "c", "basis": "b"}`,
		"basis missing":         `{"declaredParcelId": "SYN-PARCEL-1", "suggestion": "s", "candidate": "c"}`,
		"blank candidate":       `{"declaredParcelId": "SYN-PARCEL-1", "suggestion": "s", "candidate": "  ", "basis": "b"}`,
		"not json":              `declare`,
	} {
		decider := &deciderDouble{}
		response := postDecision(t, networkhttp.NewDecideRerouteEndpoint(decisionIntake(t, &decisionAuthenticator{}), decider),
			"presented.operator.token", body)
		if response.Code != http.StatusBadRequest || errorCodeOf(t, response) != "MALFORMED_REQUEST" {
			t.Errorf("%s: %d %s", name, response.Code, response.Body.String())
		}
		if len(decider.commands) != 0 {
			t.Errorf("%s: 坏报文进了编排", name)
		}
	}
}

// Covers: AT-NR-098（ADR-0284 决定一）——接入各格按恢复动作分码：没令牌 401、没授予 403、准入范围外 403、身份依赖读不动 503，都不进编排。
func TestTheRerouteDecisionIntakeAnswersEachAccessGrade(t *testing.T) {
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
		"channel unconfigured": {"t", networkhttp.ErrAccessChannelNotConfigured, http.StatusForbidden,
			"ACCESS_CHANNEL_NOT_CONFIGURED"},
	}
	for name, testCase := range cases {
		decider := &deciderDouble{}
		handler := networkhttp.NewDecideRerouteEndpoint(decisionIntake(t, &decisionAuthenticator{err: testCase.err}), decider)
		response := postDecision(t, handler, testCase.token, rerouteDecisionBody)
		if response.Code != testCase.status || errorCodeOf(t, response) != testCase.code {
			t.Errorf("%s: %d %s, want %d %s", name, response.Code, response.Body.String(), testCase.status, testCase.code)
		}
		if len(decider.commands) != 0 {
			t.Errorf("%s: 没过接入就进了编排", name)
		}
	}
}

// Covers: ADR-0022——编排报错是没形成答案，答 500 NO_ANSWER_FORMED 不带 outcome；编排交回没有名字的结果是实现坏了，同样 500。
func TestTheRerouteDecisionEndpointDoesNotDisguiseAFailure(t *testing.T) {
	failing := &deciderDouble{err: errors.New("boom")}
	response := postDecision(t, networkhttp.NewDecideRerouteEndpoint(decisionIntake(t, &decisionAuthenticator{}), failing),
		"t", rerouteDecisionBody)
	if response.Code != http.StatusInternalServerError || errorCodeOf(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("编排报错：%d %s", response.Code, response.Body.String())
	}
	unnamed := &deciderDouble{}
	response = postDecision(t, networkhttp.NewDecideRerouteEndpoint(decisionIntake(t, &decisionAuthenticator{}), unnamed),
		"t", rerouteDecisionBody)
	if response.Code != http.StatusInternalServerError || errorCodeOf(t, response) != "UNNAMED_OUTCOME" {
		t.Fatalf("没有名字的结果：%d %s", response.Code, response.Body.String())
	}
}

type suggestionReader struct {
	rows   []ports.RerouteSuggestionCatalogueRow
	err    error
	tenant domain.TenantID
	limit  int
}

func (reader *suggestionReader) ListRerouteSuggestions(_ context.Context, tenant domain.TenantID, limit int) ([]ports.RerouteSuggestionCatalogueRow, error) {
	reader.tenant, reader.limit = tenant, limit
	return reader.rows, reader.err
}

type readIntake struct{}

func (readIntake) IntakeCatalogueQuery(context.Context, *http.Request) (networkhttp.NetworkCatalogQuery, error) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		return networkhttp.NetworkCatalogQuery{}, err
	}
	reference, err := domain.NewOperationsScopeReference("MASTER_DATA_AND_OPERATIONS_READ")
	if err != nil {
		return networkhttp.NetworkCatalogQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	return networkhttp.NetworkCatalogQuery{Scope: scope, Limit: 200}, err
}

// Covers: AT-NR-099（ADR-0284 决定六）——建议读面逐行透出判断键、触发关联、被复核计划、阻塞、候选与状态，已决定的带决定；空册答空数组；读
// 不回答 500，不伪装成空册。
func TestTheRerouteSuggestionReadFaceListsSuggestionsWithTheirState(t *testing.T) {
	decidedAt := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	reader := &suggestionReader{rows: []ports.RerouteSuggestionCatalogueRow{{
		CorrelationID: "handover/1:a", CustomerAccountID: "SYN-CUST-1", ShipmentRequestID: "SYN-REQ-1",
		AcceptanceBaseline: "SYN-REQ-1/v1", DeclaredParcelID: "SYN-PARCEL-1", ServicePurpose: "NETWORK_SERVICE",
		ReviewedPlan: "SYN-RPV-1", LapseBasis: "HARD_CONSTRAINT_RESTRICTION/X", Blockers: []string{"NOT_AT_CONTROLLED_NODE"},
		Candidates: []ports.RerouteSuggestionCandidateRow{
			{ID: "SYN-LINE-A@1", Outcome: "ELIMINATED", Reason: "HARD_CONSTRAINT_RESTRICTION/X"},
			{ID: "SYN-LINE-B@1", Outcome: "QUALIFIED"},
		},
		SuggestedAt: decidedAt.Add(-time.Hour), TriggerOccurredAt: decidedAt.Add(-2 * time.Hour),
		State: ports.RerouteSuggestionDecided, HasDecision: true,
		Decision: ports.RerouteSuggestionDecisionRow{Candidate: "SYN-LINE-B@1", NewPlanVersion: "SYN-RPV-2",
			DecidedBy: "OPERATOR/https://issuer.example#ops-7", Basis: "SYN-APPROVAL-7", DecidedAt: decidedAt},
	}}}
	handler := networkhttp.NewQueryRerouteSuggestionsEndpoint(readIntake{}, reader)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/route-reroute-suggestions", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome     string `json:"outcome"`
		Suggestions []struct {
			CorrelationID string `json:"correlationId"`
			State         string `json:"state"`
			Candidates    []struct {
				ID      string `json:"id"`
				Outcome string `json:"outcome"`
				Reason  string `json:"reason"`
			} `json:"candidates"`
			Blockers []string `json:"blockers"`
			Decision *struct {
				Candidate      string `json:"candidate"`
				NewPlanVersion string `json:"newPlanVersion"`
				DecidedBy      string `json:"decidedBy"`
				DecidedAt      string `json:"decidedAt"`
			} `json:"decision"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "REROUTE_SUGGESTIONS_LISTED" || len(body.Suggestions) != 1 {
		t.Fatalf("body = %s", response.Body.String())
	}
	row := body.Suggestions[0]
	if row.CorrelationID != "handover/1:a" || row.State != "DECIDED" || len(row.Candidates) != 2 ||
		row.Candidates[0].Reason != "HARD_CONSTRAINT_RESTRICTION/X" || len(row.Blockers) != 1 ||
		row.Decision == nil || row.Decision.NewPlanVersion != "SYN-RPV-2" || row.Decision.DecidedAt != "2026-10-26T09:00:00Z" {
		t.Fatalf("row = %+v", row)
	}
	if reader.tenant.String() != "SYN-TENANT-01" || reader.limit != 200 {
		t.Fatalf("读口收到 tenant=%q limit=%d", reader.tenant, reader.limit)
	}

	empty := httptest.NewRecorder()
	networkhttp.NewQueryRerouteSuggestionsEndpoint(readIntake{}, &suggestionReader{}).
		ServeHTTP(empty, httptest.NewRequest(http.MethodGet, "/route-reroute-suggestions", nil))
	if !strings.Contains(empty.Body.String(), `"suggestions":[]`) {
		t.Fatalf("空册 = %s", empty.Body.String())
	}
	failing := httptest.NewRecorder()
	networkhttp.NewQueryRerouteSuggestionsEndpoint(readIntake{}, &suggestionReader{err: errors.New("boom")}).
		ServeHTTP(failing, httptest.NewRequest(http.MethodGet, "/route-reroute-suggestions", nil))
	if failing.Code != http.StatusInternalServerError || errorCodeOf(t, failing) != "NO_ANSWER_FORMED" {
		t.Fatalf("读不回：%d %s", failing.Code, failing.Body.String())
	}
}
