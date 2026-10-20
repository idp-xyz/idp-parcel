package nodeopshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证预报有效时窗的收口写面（票 ps-external-mark-relations/14 裁定）：与登记写面同一能力面、同一 Intake，租户只从信封来；载荷
// 封闭，指名要收口那一行的起点、收口时刻与依据都必给；答案两格各映一个状态码并回显带着收口的那一行，收不了口答坏载荷。夹具全为
// 合成（S 级）。

// windowCloser 记下收到的命令，交回设定的答案。
type windowCloser struct {
	received []application.ClosePreAdviceWindowCommand
	result   func(application.ClosePreAdviceWindowCommand) (application.PreAdviceWindowClosureResult, error)
}

func (closer *windowCloser) Handle(_ context.Context, command application.ClosePreAdviceWindowCommand) (application.PreAdviceWindowClosureResult, error) {
	closer.received = append(closer.received, command)
	return closer.result(command)
}

func closureEndpoint(t *testing.T, writer registryWriter, closer *windowCloser) http.Handler {
	t.Helper()
	intake, err := nodeopshttp.NewOperatorRegistryIntake(writer)
	if err != nil {
		t.Fatal(err)
	}
	return nodeopshttp.NewClosePreAdviceWindowEndpoint(intake, closer)
}

func postClosure(endpoint http.Handler, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/node-operations-pre-advice-window-closures", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

const closureBody = `{"windowEffectiveFrom": "2026-01-01T00:00:00Z", "closedAt": "2026-07-01T00:00:00Z", "basis": "SYN-BASIS-PSXM14-CLOSURE"}`

func closeWith(t *testing.T, outcome application.PreAdviceWindowClosureOutcome) func(application.ClosePreAdviceWindowCommand) (application.PreAdviceWindowClosureResult, error) {
	return func(command application.ClosePreAdviceWindowCommand) (application.PreAdviceWindowClosureResult, error) {
		registeredAt := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
		window, err := domain.FormPreAdviceValidityWindow(domain.PreAdviceValidityWindowSpec{
			TenantID: command.TenantID, Lead: 24 * time.Hour, Grace: 6 * time.Hour, EffectiveFrom: command.WindowFrom,
			Basis: "SYN-BASIS-PSXM08-WINDOW", RegisteredAt: registeredAt,
		})
		if err != nil {
			t.Fatalf("form window: %v", err)
		}
		closure, err := domain.FormPreAdviceWindowClosure(domain.PreAdviceWindowClosureSpec{
			TenantID: command.TenantID, WindowFrom: command.WindowFrom, ClosedAt: command.ClosedAt, Basis: command.Basis, RegisteredAt: registeredAt,
		})
		if err != nil {
			t.Fatalf("form closure: %v", err)
		}
		closed, err := window.Close(closure)
		if err != nil {
			t.Fatalf("close: %v", err)
		}
		return application.NewPreAdviceWindowClosureResult(outcome, closed), nil
	}
}

// Covers: 写面把载荷逐格交给编排、租户取信封；收口落下答 201，原样重复收口是形成了的答案、答 200；都回显那一行与它的收口，登记的
// 终点仍是没有。
func TestTheWindowClosureFaceAnswersEachOutcome(t *testing.T) {
	for outcome, status := range map[application.PreAdviceWindowClosureOutcome]int{
		application.PreAdviceWindowClosed:          http.StatusCreated,
		application.PreAdviceWindowClosureExisting: http.StatusOK,
	} {
		closer := &windowCloser{result: closeWith(t, outcome)}
		response := postClosure(closureEndpoint(t, registryWriter{tenant: "SYN-TENANT-01"}, closer), "presented.operator.token", closureBody)
		if response.Code != status {
			t.Fatalf("%s: status = %d, want %d (%s)", outcome, response.Code, status, response.Body.String())
		}
		var answer struct {
			Outcome string `json:"outcome"`
			Window  struct {
				EffectiveFrom  string  `json:"effectiveFrom"`
				EffectiveUntil *string `json:"effectiveUntil"`
				Closure        *struct {
					ClosedAt     string `json:"closedAt"`
					Basis        string `json:"basis"`
					RegisteredAt string `json:"registeredAt"`
				} `json:"closure"`
			} `json:"window"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
		if answer.Outcome != outcome.String() || answer.Window.EffectiveFrom != "2026-01-01T00:00:00Z" || answer.Window.EffectiveUntil != nil ||
			answer.Window.Closure == nil || answer.Window.Closure.ClosedAt != "2026-07-01T00:00:00Z" ||
			answer.Window.Closure.Basis != "SYN-BASIS-PSXM14-CLOSURE" || answer.Window.Closure.RegisteredAt != "2026-10-19T09:00:00Z" {
			t.Fatalf("%s: answer = %s", outcome, response.Body.String())
		}
		command := closer.received[0]
		if command.TenantID.String() != "SYN-TENANT-01" || !command.WindowFrom.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) ||
			!command.ClosedAt.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) || command.Basis != "SYN-BASIS-PSXM14-CLOSURE" {
			t.Fatalf("%s: command = %+v", outcome, command)
		}
	}
}

// Covers: 指名那一行与收口时刻都没有默认——缺起点或缺收口时刻即坏载荷；批文自报租户、未知字段、尾随第二份文档、依据带 NUL 都答
// 坏载荷，编排不被问到；编排答收不了口（裁定的坏输入各格）答坏载荷；编排读写不动答没形成答案。渠道各格在读批文之前，与登记写面
// 同一 Intake、同一组答法。
func TestTheWindowClosureFaceRefusesWhatItCannotTranslate(t *testing.T) {
	for name, body := range map[string]string{
		"缺那一行的起点": `{"closedAt": "2026-07-01T00:00:00Z", "basis": "SYN-B"}`,
		"缺收口时刻":   `{"windowEffectiveFrom": "2026-01-01T00:00:00Z", "basis": "SYN-B"}`,
		"自报租户":    `{"tenantId": "SYN-TENANT-02", "windowEffectiveFrom": "2026-01-01T00:00:00Z", "closedAt": "2026-07-01T00:00:00Z", "basis": "SYN-B"}`,
		"未知字段":    `{"windowEffectiveFrom": "2026-01-01T00:00:00Z", "closedAt": "2026-07-01T00:00:00Z", "basis": "SYN-B", "leadMinutes": 60}`,
		"尾随第二份文档": closureBody + `{}`,
		"依据带 NUL": `{"windowEffectiveFrom": "2026-01-01T00:00:00Z", "closedAt": "2026-07-01T00:00:00Z", "basis": "SYN-B\u0000"}`,
	} {
		closer := &windowCloser{result: closeWith(t, application.PreAdviceWindowClosed)}
		response := postClosure(closureEndpoint(t, registryWriter{tenant: "SYN-TENANT-01"}, closer), "presented.operator.token", body)
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s: answer = %d %s", name, response.Code, response.Body.String())
		}
		if len(closer.received) != 0 {
			t.Fatalf("%s: the closer was reached", name)
		}
	}

	rejected := &windowCloser{result: func(application.ClosePreAdviceWindowCommand) (application.PreAdviceWindowClosureResult, error) {
		return application.PreAdviceWindowClosureResult{}, application.ErrPreAdviceWindowClosureRejected
	}}
	if response := postClosure(closureEndpoint(t, registryWriter{tenant: "SYN-TENANT-01"}, rejected), "presented.operator.token", closureBody); response.Code != http.StatusBadRequest ||
		problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("rejected by the closure: %d %s", response.Code, response.Body.String())
	}
	broken := &windowCloser{result: func(application.ClosePreAdviceWindowCommand) (application.PreAdviceWindowClosureResult, error) {
		return application.PreAdviceWindowClosureResult{}, errors.New("register unreachable")
	}}
	if response := postClosure(closureEndpoint(t, registryWriter{tenant: "SYN-TENANT-01"}, broken), "presented.operator.token", closureBody); response.Code != http.StatusInternalServerError ||
		problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("register unreachable: %d %s", response.Code, response.Body.String())
	}

	for name, testCase := range map[string]struct {
		err    error
		token  string
		status int
		code   string
	}{
		"发行方未配置":  {nodeopshttp.ErrAccessChannelNotConfigured, "presented.operator.token", http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		"没有登记写授予": {nodeopshttp.ErrOperatorNotGranted, "presented.operator.token", http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"令牌不过":    {nil, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"操作者册读不动": {nodeopshttp.ErrIdentityDependencyUnavailable, "presented.operator.token", http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	} {
		closer := &windowCloser{result: closeWith(t, application.PreAdviceWindowClosed)}
		response := postClosure(closureEndpoint(t, registryWriter{tenant: "SYN-TENANT-01", err: testCase.err}, closer), testCase.token, `{"tenantId": "SYN-TENANT-02"}`)
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s: answer = %d %s", name, response.Code, response.Body.String())
		}
		if len(closer.received) != 0 {
			t.Fatalf("%s: the closer was reached", name)
		}
	}

	get := httptest.NewRequest(http.MethodGet, "/node-operations-pre-advice-window-closures", nil)
	response := httptest.NewRecorder()
	closureEndpoint(t, registryWriter{tenant: "SYN-TENANT-01"}, &windowCloser{}).ServeHTTP(response, get)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET: %d %q", response.Code, response.Header().Get("Allow"))
	}
}
