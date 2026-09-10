package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// stubDutyCollaborations 是读口替身：记录收到的键，交回预置的册子或故障。
type stubDutyCollaborations struct {
	entries []domain.DutyPaymentCollaboration
	err     error

	gotTenant string
	gotLimit  int
}

func (stub *stubDutyCollaborations) ListDutyCollaborations(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]domain.DutyPaymentCollaboration, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.entries, stub.err
}

// unreachableDutyCollaborations 断言读口未被触到。
type unreachableDutyCollaborations struct{ t *testing.T }

func (stub unreachableDutyCollaborations) ListDutyCollaborations(
	context.Context, domain.TenantID, int,
) ([]domain.DutyPaymentCollaboration, error) {
	stub.t.Fatal("a refused request reached the duty collaboration register")
	return nil, nil
}

func collaborationRequest(method string) *http.Request {
	return httptest.NewRequest(method, "/customs-duty-collaborations", nil)
}

func collaborationOf(t *testing.T, kind domain.DutyObligationKind, scope string) domain.DutyPaymentCollaboration {
	t.Helper()
	spec := domain.DutyCollaborationSpec{
		Kind:        kind,
		Scope:       endpointValue(t, domain.NewDecisionScopeReference, scope),
		Obligor:     endpointValue(t, domain.NewLegalObligorReference, "SYN-OBLIGOR-01"),
		Requirement: endpointValue(t, domain.NewPaymentRequirementSource, "SYN-ASSESSMENT-01"),
		Target:      endpointValue(t, domain.NewResponsibilityTargetReference, "SYN-DUTY-DESK"),
		FormedAt:    catalogueBaseAt,
	}
	switch kind {
	case domain.ObligationFromAssessedDuty:
		spec.Duty = endpointValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1")
	case domain.ObligationExplicitlyNotRequired:
		spec.NoPayBasis = "SYN-PROGRAM-01: no duty on this scope"
	}
	collaboration, err := domain.FormDutyCollaboration(spec)
	if err != nil {
		t.Fatalf("构造协作事项：%v", err)
	}
	return collaboration
}

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，读口不被触到。
func TestDutyCollaborationQueryRefusesNonGetMethods(t *testing.T) {
	endpoint := customshttp.NewQueryDutyCollaborationsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 50},
		unreachableDutyCollaborations{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, collaborationRequest(http.MethodPost))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 答 403，读口不被触到。
func TestUnconfiguredDutyCollaborationIntakeRefusesTheQuery(t *testing.T) {
	endpoint := customshttp.NewQueryDutyCollaborationsEndpoint(
		customshttp.UnconfiguredIntake{}, unreachableDutyCollaborations{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, collaborationRequest(http.MethodGet))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", got)
	}
	assertNoOutcome(t, response)
}

// Covers: ADR-0077 Decision 四 / 票 sa-cc/10 完成判据 1 — 空册以空数组在场；租户与页大小
// 从作用域来，不采信请求自报。
func TestAnEmptyDutyCollaborationRegisterAnswersAnEmptyArray(t *testing.T) {
	register := &stubDutyCollaborations{}
	endpoint := customshttp.NewQueryDutyCollaborationsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, register,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs-duty-collaborations?tenant=TENANT-9&limit=9999", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome        string          `json:"outcome"`
		Collaborations json.RawMessage `json:"collaborations"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "DUTY_COLLABORATIONS_LISTED" || string(body.Collaborations) != "[]" {
		t.Fatalf("空册没有以空数组在场：%s", response.Body.String())
	}
}

// Covers: CONTEXT「税费付款协作事项」/ 0016 自注 — 义务依据两格逐字段转写、互不串格：核定
// 税费格带 duty 不带 noPayBasis，明确无需付款格反之；kind 封闭二值原词。
func TestDutyCollaborationListTranscribesBothKindsWithoutCrossingFields(t *testing.T) {
	register := &stubDutyCollaborations{
		entries: []domain.DutyPaymentCollaboration{
			collaborationOf(t, domain.ObligationFromAssessedDuty, "SYN-UNIT-01"),
			collaborationOf(t, domain.ObligationExplicitlyNotRequired, "SYN-UNIT-02"),
		},
	}
	endpoint := customshttp.NewQueryDutyCollaborationsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, register,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, collaborationRequest(http.MethodGet))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome        string `json:"outcome"`
		Collaborations []struct {
			Scope       string  `json:"scope"`
			Kind        string  `json:"kind"`
			Duty        *string `json:"duty"`
			NoPayBasis  *string `json:"noPayBasis"`
			Obligor     string  `json:"obligor"`
			Requirement string  `json:"requirement"`
			Target      string  `json:"target"`
			FormedAt    string  `json:"formedAt"`
		} `json:"collaborations"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "DUTY_COLLABORATIONS_LISTED" || len(body.Collaborations) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	assessed, notRequired := body.Collaborations[0], body.Collaborations[1]
	if assessed.Scope != "SYN-UNIT-01" || assessed.Kind != "ASSESSED_DUTY" ||
		assessed.Duty == nil || *assessed.Duty != "SYN-DUTY-01/v1" || assessed.NoPayBasis != nil ||
		assessed.Obligor != "SYN-OBLIGOR-01" || assessed.Requirement != "SYN-ASSESSMENT-01" ||
		assessed.Target != "SYN-DUTY-DESK" || assessed.FormedAt != catalogueBaseAt.Format(time.RFC3339Nano) {
		t.Fatalf("核定税费格转写走样（要 duty 在场、noPayBasis 缺席）：%+v", assessed)
	}
	if notRequired.Scope != "SYN-UNIT-02" || notRequired.Kind != "EXPLICITLY_NOT_REQUIRED" ||
		notRequired.Duty != nil || notRequired.NoPayBasis == nil ||
		*notRequired.NoPayBasis != "SYN-PROGRAM-01: no duty on this scope" {
		t.Fatalf("明确无需付款格转写走样（要 noPayBasis 在场、duty 缺席）：%+v", notRequired)
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成（5xx），不伪装成空册。
func TestAFailingDutyCollaborationReadIsNoAnswerRatherThanAnEmptyRegister(t *testing.T) {
	endpoint := customshttp.NewQueryDutyCollaborationsEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25},
		&stubDutyCollaborations{err: errors.New("connection refused")},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, collaborationRequest(http.MethodGet))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
	}
	assertNoOutcome(t, response)
}
