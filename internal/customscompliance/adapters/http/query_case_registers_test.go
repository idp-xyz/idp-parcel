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
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// stubCaseRegisters 是读口替身：记录收到的键，交回预置的册子或故障。
type stubCaseRegisters struct {
	judgments      []domain.ReadinessJudgment
	authorizations []domain.SubmissionAuthorization
	obligations    []ports.ClosureObligationCatalogueEntry
	err            error

	gotTenant string
	gotLimit  int
}

func (stub *stubCaseRegisters) ListReadinessJudgments(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]domain.ReadinessJudgment, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.judgments, stub.err
}

func (stub *stubCaseRegisters) ListSubmissionAuthorities(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]domain.SubmissionAuthorization, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.authorizations, stub.err
}

func (stub *stubCaseRegisters) ListClosureObligations(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.ClosureObligationCatalogueEntry, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.obligations, stub.err
}

// unreachableCaseRegisters 断言读口未被触到：传输形状的拒绝与未配置格都发生在读库
// 之前。
type unreachableCaseRegisters struct{ t *testing.T }

func (stub unreachableCaseRegisters) ListReadinessJudgments(
	context.Context, domain.TenantID, int,
) ([]domain.ReadinessJudgment, error) {
	stub.t.Fatal("a refused request reached the case registers")
	return nil, nil
}

func (stub unreachableCaseRegisters) ListSubmissionAuthorities(
	context.Context, domain.TenantID, int,
) ([]domain.SubmissionAuthorization, error) {
	stub.t.Fatal("a refused request reached the case registers")
	return nil, nil
}

func (stub unreachableCaseRegisters) ListClosureObligations(
	context.Context, domain.TenantID, int,
) ([]ports.ClosureObligationCatalogueEntry, error) {
	stub.t.Fatal("a refused request reached the case registers")
	return nil, nil
}

func registerRequest(t *testing.T, query string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/customs-case-registers"+query, nil)
}

func readinessOf(t *testing.T, unit, basis string, judgedAt time.Time) domain.ReadinessJudgment {
	t.Helper()
	judgment, err := domain.JudgeReady(
		endpointValue(t, domain.NewDeclarationUnitID, unit),
		endpointValue(t, domain.NewReadinessBasisReference, basis),
		judgedAt,
	)
	if err != nil {
		t.Fatalf("构造就绪判断：%v", err)
	}
	return judgment
}

func authorityOf(t *testing.T, unit, authority string, grantedAt time.Time) domain.SubmissionAuthorization {
	t.Helper()
	authorization, err := domain.GrantSubmissionAuthority(
		endpointValue(t, domain.NewDeclarationUnitID, unit),
		endpointValue(t, domain.NewSubmissionAuthorityReference, authority),
		grantedAt,
	)
	if err != nil {
		t.Fatalf("构造提交授权：%v", err)
	}
	return authorization
}

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，读口不被触到。
func TestCaseRegisterQueryRefusesNonGetMethods(t *testing.T) {
	endpoint := customshttp.NewQueryCaseRegistersEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 50},
		unreachableCaseRegisters{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/customs-case-registers?registry=readiness", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: registry 封闭集 — 缺席与集外值都是坏请求，拒在 Intake 之前；规则库那两个
// registry 值在本端点也是集外（两端点各自的封闭集不互相收留）。
func TestCaseRegisterQueryRejectsAnAbsentOrUnknownRegistry(t *testing.T) {
	endpoint := customshttp.NewQueryCaseRegistersEndpoint(
		failingCatalogueIntake{err: errors.New("intake must not be consulted")},
		unreachableCaseRegisters{t: t},
	)
	for name, query := range map[string]string{
		"absent":        "",
		"blank":         "?registry=",
		"rule-registry": "?registry=case-requirement",
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, registerRequest(t, query))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want %d", name, response.Code, http.StatusBadRequest)
		}
		if got := problemCode(t, response); got != "MALFORMED_REQUEST" {
			t.Fatalf("%s: code = %q, want MALFORMED_REQUEST", name, got)
		}
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 对全部分派分支同答 403，
// 分支选择不泄露任何东西；读口不被触到。
func TestUnconfiguredCaseRegisterIntakeRefusesAllRegistriesIdentically(t *testing.T) {
	endpoint := customshttp.NewQueryCaseRegistersEndpoint(
		customshttp.UnconfiguredIntake{}, unreachableCaseRegisters{t: t},
	)
	var bodies []string
	for _, query := range []string{
		"?registry=readiness", "?registry=submission-authority", "?registry=closure-obligation",
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, registerRequest(t, query))
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want %d", query, response.Code, http.StatusForbidden)
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", query, got)
		}
		assertNoOutcome(t, response)
		bodies = append(bodies, response.Body.String())
	}
	if bodies[0] != bodies[1] || bodies[1] != bodies[2] {
		t.Fatalf("三个分支的未配置答复不一致：%v", bodies)
	}
}

// Covers: CONTEXT「提交授权与就绪判断分别形成和失效」 / 票 05 形状约束一二 — 就绪与授权分册分格转写：有效行撤销两列
// 缺席，失效行原判断与失效两列同场；租户与页大小从作用域来，不采信请求自报。
func TestReadinessAndAuthorityListsTranscribeRevocationVerbatim(t *testing.T) {
	revoked, err := readinessOf(t, "unit-2", "basis-2", catalogueBaseAt).
		Revoke("RULES-CHANGED", catalogueBaseAt.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("撤销就绪判断：%v", err)
	}
	registers := &stubCaseRegisters{
		judgments: []domain.ReadinessJudgment{
			readinessOf(t, "unit-1", "basis-1", catalogueBaseAt),
			revoked,
		},
	}
	endpoint := customshttp.NewQueryCaseRegistersEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, registers,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, registerRequest(t, "?registry=readiness&tenant=TENANT-9&limit=9999"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if registers.gotTenant != "TENANT-1" || registers.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d（自报查询参数被采信了）",
			registers.gotTenant, registers.gotLimit)
	}
	var body struct {
		Outcome   string `json:"outcome"`
		Judgments []struct {
			Unit      string `json:"unit"`
			Basis     string `json:"basis"`
			JudgedAt  string `json:"judgedAt"`
			RevokedBy string `json:"revokedBy"`
			RevokedAt string `json:"revokedAt"`
		} `json:"judgments"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "READINESS_JUDGMENTS_LISTED" || len(body.Judgments) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	active, lapsed := body.Judgments[0], body.Judgments[1]
	if active.Unit != "unit-1" || active.RevokedBy != "" || active.RevokedAt != "" {
		t.Fatalf("有效行不该带失效两列：%+v", active)
	}
	if lapsed.Unit != "unit-2" || lapsed.Basis != "basis-2" ||
		lapsed.RevokedBy != "RULES-CHANGED" ||
		lapsed.RevokedAt != catalogueBaseAt.Add(24*time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("失效行转写走样（原判断与失效两列都要在场）：%+v", lapsed)
	}

	// 授权那条轨独立转写：同一路端点、不同 registry、各自的结果格——两册在传输层
	// 就分列，不合成「可提交」。
	revokedAuthority, err := authorityOf(t, "unit-1", "authority-1", catalogueBaseAt).
		Revoke("MANDATE-WITHDRAWN", catalogueBaseAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("撤销提交授权：%v", err)
	}
	registers.authorizations = []domain.SubmissionAuthorization{revokedAuthority}
	response = httptest.NewRecorder()
	endpoint.ServeHTTP(response, registerRequest(t, "?registry=submission-authority"))
	var authorityBody struct {
		Outcome     string `json:"outcome"`
		Authorities []struct {
			Unit      string `json:"unit"`
			Authority string `json:"authority"`
			RevokedBy string `json:"revokedBy"`
		} `json:"authorities"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &authorityBody); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if authorityBody.Outcome != "SUBMISSION_AUTHORITIES_LISTED" ||
		len(authorityBody.Authorities) != 1 ||
		authorityBody.Authorities[0].Authority != "authority-1" ||
		authorityBody.Authorities[0].RevokedBy != "MANDATE-WITHDRAWN" {
		t.Fatalf("授权册转写走样：%s", response.Body.String())
	}
}

// Covers: 0008 自注 / 票 05 形状约束三 — 义务目录逐份转写：登记了零义务项的目录以空
// items 数组在场（不是 null 也不是缺行）；承接项带承接对象、开放区间终点缺席。
func TestClosureObligationCataloguesTranscribeTheEmptyRegisterHonestly(t *testing.T) {
	handed := domain.ClosureObligationItem{
		Obligation: "obl-b-handed",
		Scope:      "scope-1",
		State:      domain.ObligationHandedOver,
		Basis:      "basis-b",
		HandedTo:   "broker-7",
	}
	registers := &stubCaseRegisters{
		obligations: []ports.ClosureObligationCatalogueEntry{
			{
				Case:         endpointValue(t, domain.NewCustomsCaseID, "case-empty"),
				RegisteredAt: catalogueBaseAt,
			},
			{
				Case:         endpointValue(t, domain.NewCustomsCaseID, "case-full"),
				RegisteredAt: catalogueBaseAt,
				Items: []ports.ObligationRegistration{
					{Item: handed, AppliesFrom: catalogueBaseAt},
				},
			},
		},
	}
	endpoint := customshttp.NewQueryCaseRegistersEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, registers,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, registerRequest(t, "?registry=closure-obligation"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var fields struct {
		Outcome    string `json:"outcome"`
		Catalogues []struct {
			Case  string          `json:"case"`
			Items json.RawMessage `json:"items"`
		} `json:"catalogues"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if fields.Outcome != "CLOSURE_OBLIGATIONS_LISTED" || len(fields.Catalogues) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	if fields.Catalogues[0].Case != "case-empty" || string(fields.Catalogues[0].Items) != "[]" {
		t.Fatalf("空清单目录没有以空数组在场：%s", response.Body.String())
	}
	var items []struct {
		Obligation   string `json:"obligation"`
		State        string `json:"state"`
		HandedTo     string `json:"handedTo"`
		AppliesFrom  string `json:"appliesFrom"`
		AppliesUntil string `json:"appliesUntil"`
	}
	if err := json.Unmarshal(fields.Catalogues[1].Items, &items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	if len(items) != 1 || items[0].State != "HANDED_OVER" || items[0].HandedTo != "broker-7" {
		t.Fatalf("承接项转写走样：%+v", items)
	}
	if items[0].AppliesFrom != catalogueBaseAt.Format(time.RFC3339Nano) || items[0].AppliesUntil != "" {
		t.Fatalf("适用区间转写走样（开放区间终点该缺席）：%+v", items[0])
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成（5xx），不伪装成空册。
func TestAFailingCaseRegisterReadIsNoAnswerRatherThanAnEmptyRegister(t *testing.T) {
	endpoint := customshttp.NewQueryCaseRegistersEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25},
		&stubCaseRegisters{err: errors.New("connection refused")},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, registerRequest(t, "?registry=closure-obligation"))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
	}
	assertNoOutcome(t, response)
}
