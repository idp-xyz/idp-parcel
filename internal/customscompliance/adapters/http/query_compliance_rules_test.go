package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var catalogueBaseAt = time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)

// grantedCatalogueIntake 装出「认证已就位」的接入面：作用域与页大小都来自它，不读
// 请求内容。真渠道（操作者渠道，ADR-0100）未就位，生产装配点不会有这样的实现——它只在测试里
// 存在，为的是隔离验证端点的分派与转写。
type grantedCatalogueIntake struct {
	tenant string
	limit  int
}

func (intake grantedCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (customshttp.CatalogueQuery, error) {
	reference, err := domain.NewOperationsScopeReference("OPS-SCOPE-1")
	if err != nil {
		return customshttp.CatalogueQuery{}, err
	}
	tenant, err := domain.NewTenantID(intake.tenant)
	if err != nil {
		return customshttp.CatalogueQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		return customshttp.CatalogueQuery{}, err
	}
	return customshttp.CatalogueQuery{Scope: scope, Limit: intake.limit}, nil
}

type failingCatalogueIntake struct{ err error }

func (intake failingCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (customshttp.CatalogueQuery, error) {
	return customshttp.CatalogueQuery{}, intake.err
}

// stubRuleCatalogue 是读口替身：记录收到的键，交回预置的条目或故障。
type stubRuleCatalogue struct {
	requirementEntries    []ports.CaseRequirementRuleEntry
	interpretationEntries []ports.InterpretationRuleEntry
	err                   error

	gotTenant string
	gotLimit  int
}

func (stub *stubRuleCatalogue) ListCaseRequirementRules(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.CaseRequirementRuleEntry, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.requirementEntries, stub.err
}

func (stub *stubRuleCatalogue) ListInterpretationRules(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.InterpretationRuleEntry, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.interpretationEntries, stub.err
}

// unreachableRuleCatalogue 断言读口未被触到：传输形状的拒绝与未配置格都发生在读库
// 之前。
type unreachableRuleCatalogue struct{ t *testing.T }

func (stub unreachableRuleCatalogue) ListCaseRequirementRules(
	context.Context, domain.TenantID, int,
) ([]ports.CaseRequirementRuleEntry, error) {
	stub.t.Fatal("a refused request reached the rule catalogue")
	return nil, nil
}

func (stub unreachableRuleCatalogue) ListInterpretationRules(
	context.Context, domain.TenantID, int,
) ([]ports.InterpretationRuleEntry, error) {
	stub.t.Fatal("a refused request reached the rule catalogue")
	return nil, nil
}

func catalogueRequest(t *testing.T, query string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/customs-compliance-rules"+query, nil)
}

func requirementEntry(t *testing.T, jurisdiction string, direction domain.ManifestDirection, procedure string, required bool, basis string) ports.CaseRequirementRuleEntry {
	t.Helper()
	return ports.CaseRequirementRuleEntry{
		Jurisdiction: endpointValue(t, domain.NewRegulatoryJurisdictionReference, jurisdiction),
		Direction:    direction,
		Procedure:    endpointValue(t, domain.NewCustomsProcedureReference, procedure),
		Judgment:     ports.CaseRequirementJudgment{Required: required, Basis: basis},
	}
}

func endpointValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，读口不被触到。
func TestComplianceRuleQueryRefusesNonGetMethods(t *testing.T) {
	endpoint := customshttp.NewQueryComplianceRulesEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 50},
		unreachableRuleCatalogue{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/customs-compliance-rules?registry=interpretation", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: registry 封闭集 — 缺席与未知值都是坏请求（替调用方默认一本册子就是替它猜），
// 拒在 Intake 之前，属传输形状。
func TestComplianceRuleQueryRejectsAnAbsentOrUnknownRegistry(t *testing.T) {
	endpoint := customshttp.NewQueryComplianceRulesEndpoint(
		failingCatalogueIntake{err: errors.New("intake must not be consulted")},
		unreachableRuleCatalogue{t: t},
	)
	for name, query := range map[string]string{
		"absent":  "",
		"blank":   "?registry=",
		"unknown": "?registry=readiness",
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, catalogueRequest(t, query))
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
func TestUnconfiguredCatalogueIntakeRefusesBothRegistriesIdentically(t *testing.T) {
	endpoint := customshttp.NewQueryComplianceRulesEndpoint(
		customshttp.UnconfiguredIntake{}, unreachableRuleCatalogue{t: t},
	)
	var bodies []string
	for _, query := range []string{"?registry=case-requirement", "?registry=interpretation"} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, catalogueRequest(t, query))
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want %d", query, response.Code, http.StatusForbidden)
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", query, got)
		}
		assertNoOutcome(t, response)
		bodies = append(bodies, response.Body.String())
	}
	if bodies[0] != bodies[1] {
		t.Fatalf("两个分支的未配置答复不一致：%s vs %s", bodies[0], bodies[1])
	}
}

// Covers: ADR-0022 — Intake 的三格各自映射：坏请求 400、未配置 403（上一用例）、
// 其余是 500 INTAKE_FAILED。
func TestCatalogueIntakeFailuresKeepTheirGrades(t *testing.T) {
	malformed := customshttp.NewQueryComplianceRulesEndpoint(
		failingCatalogueIntake{err: fmt.Errorf("%w: no scope", customshttp.ErrMalformedRequest)},
		unreachableRuleCatalogue{t: t},
	)
	response := httptest.NewRecorder()
	malformed.ServeHTTP(response, catalogueRequest(t, "?registry=interpretation"))
	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("malformed: %d %s", response.Code, response.Body.String())
	}

	failing := customshttp.NewQueryComplianceRulesEndpoint(
		failingCatalogueIntake{err: errors.New("identity capability is down")},
		unreachableRuleCatalogue{t: t},
	)
	response = httptest.NewRecorder()
	failing.ServeHTTP(response, catalogueRequest(t, "?registry=interpretation"))
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "INTAKE_FAILED" {
		t.Fatalf("intake failure: %d %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0077 Decision 一/五 — 建案要求规则逐字段转写；租户与页大小从作用域来，
// 不采信请求自报。
func TestCaseRequirementRulesAreListedVerbatim(t *testing.T) {
	catalogue := &stubRuleCatalogue{
		requirementEntries: []ports.CaseRequirementRuleEntry{
			requirementEntry(t, "US-CBP", domain.ImportManifest, "IMPORT/GENERAL", true, "PRODUCT/DDP-REQUIRES-CASE"),
			requirementEntry(t, "EU-DE", domain.ExportManifest, "EXPORT/SIMPLE", false, "PRODUCT/CARRIER-DECLARES"),
		},
	}
	endpoint := customshttp.NewQueryComplianceRulesEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, catalogue,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogueRequest(t, "?registry=case-requirement&tenant=TENANT-9&limit=9999"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if catalogue.gotTenant != "TENANT-1" || catalogue.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d（自报查询参数被采信了）",
			catalogue.gotTenant, catalogue.gotLimit)
	}

	var body struct {
		Outcome string `json:"outcome"`
		Rules   []struct {
			Jurisdiction string `json:"jurisdiction"`
			Direction    string `json:"direction"`
			Procedure    string `json:"procedure"`
			Required     bool   `json:"required"`
			Basis        string `json:"basis"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "CASE_REQUIREMENT_RULES_LISTED" || len(body.Rules) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	first, second := body.Rules[0], body.Rules[1]
	if first.Jurisdiction != "US-CBP" || first.Direction != "IMPORT" ||
		first.Procedure != "IMPORT/GENERAL" || !first.Required || first.Basis != "PRODUCT/DDP-REQUIRES-CASE" {
		t.Fatalf("首条转写走样：%+v", first)
	}
	if second.Required || second.Basis != "PRODUCT/CARRIER-DECLARES" {
		t.Fatalf("「不要求」那条的依据没透出：%+v", second)
	}
}

// Covers: ADR-0070 问一甲 — 解释规则版本区间逐字段转写；开放版 appliesUntil 缺席，
// 不为区间完整补一个编造的时刻。
func TestInterpretationRulesCarryTheirIntervalsVerbatim(t *testing.T) {
	catalogue := &stubRuleCatalogue{
		interpretationEntries: []ports.InterpretationRuleEntry{
			{
				Layer:        domain.ReleaseResultLayer,
				Jurisdiction: endpointValue(t, domain.NewRegulatoryJurisdictionReference, "US-CBP"),
				Rule:         endpointValue(t, domain.NewInterpretationRuleReference, "interpret/release/v2"),
				AppliesFrom:  catalogueBaseAt.Add(48 * time.Hour),
			},
			{
				Layer:        domain.ReleaseResultLayer,
				Jurisdiction: endpointValue(t, domain.NewRegulatoryJurisdictionReference, "US-CBP"),
				Rule:         endpointValue(t, domain.NewInterpretationRuleReference, "interpret/release/v1"),
				AppliesFrom:  catalogueBaseAt,
				AppliesUntil: catalogueBaseAt.Add(48 * time.Hour),
			},
		},
	}
	endpoint := customshttp.NewQueryComplianceRulesEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, catalogue,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogueRequest(t, "?registry=interpretation"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string `json:"outcome"`
		Rules   []struct {
			Layer        string `json:"layer"`
			Jurisdiction string `json:"jurisdiction"`
			Rule         string `json:"rule"`
			AppliesFrom  string `json:"appliesFrom"`
			AppliesUntil string `json:"appliesUntil"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "INTERPRETATION_RULES_LISTED" || len(body.Rules) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	open, closed := body.Rules[0], body.Rules[1]
	if open.Layer != "RELEASE_RESULT" || open.Rule != "interpret/release/v2" || open.AppliesUntil != "" {
		t.Fatalf("开放版转写走样（终点该缺席）：%+v", open)
	}
	if closed.AppliesFrom != catalogueBaseAt.Format(time.RFC3339Nano) ||
		closed.AppliesUntil != catalogueBaseAt.Add(48*time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("已闭合版区间走样：%+v", closed)
	}
}

// Covers: ADR-0077 Decision 四 — 空册是 2xx 成格 + 空数组（不是 null）：空目录的续办
// 是去登记口登记，不是配置渠道，两格不得合并。
func TestAnEmptyRegisterAnswersAnEmptyArray(t *testing.T) {
	endpoint := customshttp.NewQueryComplianceRulesEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, &stubRuleCatalogue{},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogueRequest(t, "?registry=case-requirement"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if string(fields["rules"]) != "[]" {
		t.Fatalf("空册没有交回空数组：%s", response.Body.String())
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成（5xx），不伪装成空册：前者该重试，
// 后者是终局答案。
func TestAFailingCatalogueReadIsNoAnswerRatherThanAnEmptyRegister(t *testing.T) {
	endpoint := customshttp.NewQueryComplianceRulesEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25},
		&stubRuleCatalogue{err: errors.New("connection refused")},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, catalogueRequest(t, "?registry=interpretation"))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
	}
	assertNoOutcome(t, response)
}
