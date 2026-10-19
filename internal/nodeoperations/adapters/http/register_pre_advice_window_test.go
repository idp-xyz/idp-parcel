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

// 本文件证预报有效时窗的登记写面（ADR-0212 决定二末项；ADR-0085 决定一；ADR-0100 决定四）：挂操作者渠道的登记册配置写
// Intake，租户只从信封来；载荷封闭，提前量与宽限量必给、没有默认；答案三格各映一个状态码。夹具全为合成（S 级）。

// registryWriter 是登记写认证方的替身：交回设定的租户或格。
type registryWriter struct {
	tenant string
	err    error
}

func (writer registryWriter) AuthenticateRegistryWrite(_ context.Context, bearerToken string) (domain.TenantID, error) {
	if writer.err != nil {
		return domain.TenantID{}, writer.err
	}
	if bearerToken == "" {
		return domain.TenantID{}, nodeopshttp.ErrOperatorCredentialRejected
	}
	return domain.NewTenantID(writer.tenant)
}

// windowRegistrar 记下收到的命令，交回设定的答案。
type windowRegistrar struct {
	received []application.RegisterPreAdviceWindowCommand
	result   func(application.RegisterPreAdviceWindowCommand) (application.PreAdviceWindowRegistrationResult, error)
}

func (registrar *windowRegistrar) Handle(_ context.Context, command application.RegisterPreAdviceWindowCommand) (application.PreAdviceWindowRegistrationResult, error) {
	registrar.received = append(registrar.received, command)
	return registrar.result(command)
}

func windowEndpoint(t *testing.T, writer registryWriter, registrar *windowRegistrar) http.Handler {
	t.Helper()
	intake, err := nodeopshttp.NewOperatorRegistryIntake(writer)
	if err != nil {
		t.Fatal(err)
	}
	return nodeopshttp.NewRegisterPreAdviceWindowEndpoint(intake, registrar)
}

func postWindow(endpoint http.Handler, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/node-operations-pre-advice-window-registrations", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

const windowBody = `{"leadMinutes": 1440, "graceMinutes": 360, "effectiveFrom": "2026-01-01T00:00:00Z", "basis": "SYN-BASIS-PSXM08-WINDOW"}`

func answerWith(t *testing.T, outcome application.PreAdviceWindowRegistrationOutcome) func(application.RegisterPreAdviceWindowCommand) (application.PreAdviceWindowRegistrationResult, error) {
	return func(command application.RegisterPreAdviceWindowCommand) (application.PreAdviceWindowRegistrationResult, error) {
		window, err := domain.FormPreAdviceValidityWindow(domain.PreAdviceValidityWindowSpec{
			TenantID: command.TenantID, Lead: command.Lead, Grace: command.Grace, EffectiveFrom: command.EffectiveFrom,
			EffectiveUntil: command.EffectiveUntil, Basis: command.Basis, RegisteredAt: time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		return application.NewPreAdviceWindowRegistrationResult(outcome, window), nil
	}
}

// Covers: 写面把载荷逐格交给编排、租户取信封；首登答 201，已有结果与重叠都是形成了的答案、答 200 并回显那一行。
func TestTheWindowRegistrationFaceAnswersEachOutcome(t *testing.T) {
	for outcome, status := range map[application.PreAdviceWindowRegistrationOutcome]int{
		application.PreAdviceWindowRegistered:  http.StatusCreated,
		application.PreAdviceWindowExisting:    http.StatusOK,
		application.PreAdviceWindowOverlapping: http.StatusOK,
	} {
		registrar := &windowRegistrar{result: answerWith(t, outcome)}
		response := postWindow(windowEndpoint(t, registryWriter{tenant: "SYN-TENANT-01"}, registrar), "presented.operator.token", windowBody)
		if response.Code != status {
			t.Fatalf("%s: status = %d, want %d (%s)", outcome, response.Code, status, response.Body.String())
		}
		var answer struct {
			Outcome string `json:"outcome"`
			Window  struct {
				LeadMinutes    int64   `json:"leadMinutes"`
				GraceMinutes   int64   `json:"graceMinutes"`
				EffectiveFrom  string  `json:"effectiveFrom"`
				EffectiveUntil *string `json:"effectiveUntil"`
				Basis          string  `json:"basis"`
				RegisteredAt   string  `json:"registeredAt"`
			} `json:"window"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
		if answer.Outcome != outcome.String() || answer.Window.LeadMinutes != 1440 || answer.Window.GraceMinutes != 360 ||
			answer.Window.EffectiveFrom != "2026-01-01T00:00:00Z" || answer.Window.EffectiveUntil != nil || answer.Window.Basis != "SYN-BASIS-PSXM08-WINDOW" {
			t.Fatalf("%s: answer = %+v", outcome, answer)
		}
		command := registrar.received[0]
		if command.TenantID.String() != "SYN-TENANT-01" || command.Lead != 24*time.Hour || command.Grace != 6*time.Hour ||
			!command.EffectiveFrom.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || !command.EffectiveUntil.IsZero() {
			t.Fatalf("%s: command = %+v", outcome, command)
		}
	}
}

// Covers: 租户取值不写默认——提前量或宽限量缺席即坏载荷，不当零；批文自报租户、未知字段、尾随第二份文档都答坏载荷；编排拒形
// （宽限量为负一类）答坏载荷；编排读写不动答没形成答案。
func TestTheWindowRegistrationFaceRefusesWhatItCannotTranslate(t *testing.T) {
	for name, body := range map[string]string{
		"缺提前量":    `{"graceMinutes": 360, "effectiveFrom": "2026-01-01T00:00:00Z", "basis": "SYN-B"}`,
		"缺宽限量":    `{"leadMinutes": 1440, "effectiveFrom": "2026-01-01T00:00:00Z", "basis": "SYN-B"}`,
		"缺起点":     `{"leadMinutes": 1440, "graceMinutes": 360, "basis": "SYN-B"}`,
		"自报租户":    `{"tenantId": "SYN-TENANT-02", "leadMinutes": 1440, "graceMinutes": 360, "effectiveFrom": "2026-01-01T00:00:00Z", "basis": "SYN-B"}`,
		"未知字段":    `{"leadMinutes": 1440, "graceMinutes": 360, "effectiveFrom": "2026-01-01T00:00:00Z", "basis": "SYN-B", "node": "SYN-NODE"}`,
		"尾随第二份文档": windowBody + `{}`,
	} {
		registrar := &windowRegistrar{result: answerWith(t, application.PreAdviceWindowRegistered)}
		response := postWindow(windowEndpoint(t, registryWriter{tenant: "SYN-TENANT-01"}, registrar), "presented.operator.token", body)
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s: answer = %d %s", name, response.Code, response.Body.String())
		}
		if len(registrar.received) != 0 {
			t.Fatalf("%s: the registrar was reached", name)
		}
	}

	rejected := &windowRegistrar{result: func(application.RegisterPreAdviceWindowCommand) (application.PreAdviceWindowRegistrationResult, error) {
		return application.PreAdviceWindowRegistrationResult{}, application.ErrPreAdviceWindowRejected
	}}
	if response := postWindow(windowEndpoint(t, registryWriter{tenant: "SYN-TENANT-01"}, rejected), "presented.operator.token", windowBody); response.Code != http.StatusBadRequest ||
		problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("rejected by the registration: %d %s", response.Code, response.Body.String())
	}
	broken := &windowRegistrar{result: func(application.RegisterPreAdviceWindowCommand) (application.PreAdviceWindowRegistrationResult, error) {
		return application.PreAdviceWindowRegistrationResult{}, errors.New("register unreachable")
	}}
	if response := postWindow(windowEndpoint(t, registryWriter{tenant: "SYN-TENANT-01"}, broken), "presented.operator.token", windowBody); response.Code != http.StatusInternalServerError ||
		problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("register unreachable: %d %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0085 与 ADR-0100 决定四——未配置、未授予、令牌不过、身份依赖读不动各答其格，都在读批文之前；登记写面不判准入范围，
// 没有「不在准入范围」那一格。
func TestTheWindowRegistrationFaceAnswersEveryChannelGrade(t *testing.T) {
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
		registrar := &windowRegistrar{result: answerWith(t, application.PreAdviceWindowRegistered)}
		response := postWindow(windowEndpoint(t, registryWriter{tenant: "SYN-TENANT-01", err: testCase.err}, registrar), testCase.token, `{"tenantId": "SYN-TENANT-02"}`)
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s: answer = %d %s", name, response.Code, response.Body.String())
		}
		if len(registrar.received) != 0 {
			t.Fatalf("%s: the registrar was reached", name)
		}
	}
	if _, err := nodeopshttp.NewOperatorRegistryIntake(nil); err == nil {
		t.Fatal("缺认证方的 Intake 构造得出来")
	}

	get := httptest.NewRequest(http.MethodGet, "/node-operations-pre-advice-window-registrations", nil)
	response := httptest.NewRecorder()
	windowEndpoint(t, registryWriter{tenant: "SYN-TENANT-01"}, &windowRegistrar{}).ServeHTTP(response, get)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET: %d %q", response.Code, response.Header().Get("Allow"))
	}
}
