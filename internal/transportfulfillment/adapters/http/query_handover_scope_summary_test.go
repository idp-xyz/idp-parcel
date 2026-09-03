package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证交接范围汇总读面（票 tf-unwired-seven/03 第四层）：端点只转写应用读用例的四格
// 答案，不自己数、不自己判；租户来自准入结果，范围来自请求；门次序照本包查阅面通例。
// 结果本体由真实的 SummarizeHandoverScopeHandler 配读口替身产出——结果类型不可在包外构造，
// 而照着结果形状手搓的替身会长成实现的镜像。

const summaryTarget = "/transport-fulfillment-handover-scope-summary?scope=scope-1"

// scopeViewStub 是 ports.HandoverScopeView 的替身，记下被问到的租户与范围。
type scopeViewStub struct {
	records     []ports.TransportHandoverRecord
	err         error
	askedTenant string
	askedScope  string
}

func (stub *scopeViewStub) ListByScope(
	_ context.Context, tenant domain.TenantID, scope domain.HandoverScopeReference,
) ([]ports.TransportHandoverRecord, error) {
	stub.askedTenant, stub.askedScope = tenant.String(), scope.String()
	return stub.records, stub.err
}

// unreachableSummarizer 断言读用例未被触到：传输形状的拒绝与未配置格都发生在它之前。
type unreachableSummarizer struct{ t *testing.T }

func (stub unreachableSummarizer) Summarize(
	context.Context, application.SummarizeHandoverScopeQuery,
) (application.SummarizeHandoverScopeResult, error) {
	stub.t.Helper()
	stub.t.Fatal("a refused request reached the summarizer")
	return application.SummarizeHandoverScopeResult{}, nil
}

type failingSummarizer struct{}

func (failingSummarizer) Summarize(
	context.Context, application.SummarizeHandoverScopeQuery,
) (application.SummarizeHandoverScopeResult, error) {
	return application.SummarizeHandoverScopeResult{}, errors.New("orchestration exploded")
}

// unnamedSummarizer 交回一个没有名字的结果——编程错误，不是业务答案。
type unnamedSummarizer struct{}

func (unnamedSummarizer) Summarize(
	context.Context, application.SummarizeHandoverScopeQuery,
) (application.SummarizeHandoverScopeResult, error) {
	return application.SummarizeHandoverScopeResult{}, nil
}

func summaryRecordFor(t *testing.T, tenant, scope, object string, verdict domain.HandoverVerdict) ports.TransportHandoverRecord {
	t.Helper()
	spec := domain.TransportHandoverSpec{
		TenantID:   mustDomain(t, domain.NewTenantID, tenant),
		Object:     mustDomain(t, domain.NewCarriedObjectReference, object),
		Scope:      mustDomain(t, domain.NewHandoverScopeReference, scope),
		ReleasedBy: mustDomain(t, domain.NewHandoverPartyReference, "node-1"),
		ReceivedBy: mustDomain(t, domain.NewHandoverPartyReference, "carrier-1"),
		Verdict:    verdict,
		Version:    mustDomain(t, domain.NewHandoverResultVersion, "v1"),
		JudgedAt:   catalogueBaseAt,
	}
	if verdict == domain.ObjectHandedOver {
		spec.ReleasingEvidence = mustDomain(t, domain.NewHandoverEvidenceReference, "seal-out")
		spec.ReceivingEvidence = mustDomain(t, domain.NewHandoverEvidenceReference, "seal-in")
		spec.Rule = mustDomain(t, domain.NewHandoverRuleReference, "rule/v1")
	} else {
		spec.Basis = mustDomain(t, domain.NewHandoverBasisReference, "basis-"+object)
	}
	handover, err := domain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("构造交接夹具 %s：%v", object, err)
	}
	return ports.TransportHandoverRecord{
		Key: ports.TransportHandoverKey{
			TenantID: handover.TenantID(), Object: handover.Object(),
			Scope: handover.Scope(), Version: handover.Version(),
		},
		Handover:   handover,
		RecordedAt: catalogueBaseAt,
	}
}

func mustDomain[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func summaryEndpoint(view *scopeViewStub) http.Handler {
	return tfhttp.NewQueryHandoverScopeSummaryEndpoint(
		grantedIntake(), application.NewSummarizeHandoverScopeHandler(view),
	)
}

func decodeSummary(t *testing.T, response *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return fields
}

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，读用例不被触到。
func TestHandoverScopeSummaryRefusesNonGetMethods(t *testing.T) {
	endpoint := tfhttp.NewQueryHandoverScopeSummaryEndpoint(grantedIntake(), unreachableSummarizer{t: t})
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, summaryTarget, nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: 请求形状 — 范围是这个读面的必备维，缺席或空白是坏请求（400），且判在渠道
// 准入之前：判它不需要先知道调用方是谁。
func TestHandoverScopeSummaryRefusesABlankScopeBeforeTheChannelCheck(t *testing.T) {
	endpoint := tfhttp.NewQueryHandoverScopeSummaryEndpoint(tfhttp.UnconfiguredIntake{}, unreachableSummarizer{t: t})
	for _, target := range []string{
		"/transport-fulfillment-handover-scope-summary",
		"/transport-fulfillment-handover-scope-summary?scope=",
		"/transport-fulfillment-handover-scope-summary?scope=%20%20",
	} {
		t.Run(target, func(t *testing.T) {
			response := serveGet(endpoint, target)
			if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			assertNoOutcome(t, response)
		})
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 一律 403 + ACCESS_CHANNEL_NOT_CONFIGURED，
// 读用例不被触到，不带业务结果格。
func TestUnconfiguredIntakeRefusesTheHandoverScopeSummary(t *testing.T) {
	endpoint := tfhttp.NewQueryHandoverScopeSummaryEndpoint(tfhttp.UnconfiguredIntake{}, unreachableSummarizer{t: t})
	response := serveGet(endpoint, summaryTarget)

	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

// Covers: ADR-0022 — Intake 的三格各自映射：坏请求 400、未配置 403（上面单独用例）、
// 其余是 500 INTAKE_FAILED。
func TestHandoverScopeSummaryIntakeFailuresKeepTheirGrades(t *testing.T) {
	malformed := tfhttp.NewQueryHandoverScopeSummaryEndpoint(
		failingCatalogueIntake{err: fmt.Errorf("%w: no scope", tfhttp.ErrMalformedRequest)},
		unreachableSummarizer{t: t},
	)
	response := serveGet(malformed, summaryTarget)
	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("malformed: %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)

	failing := tfhttp.NewQueryHandoverScopeSummaryEndpoint(
		failingCatalogueIntake{err: errors.New("identity capability is down")},
		unreachableSummarizer{t: t},
	)
	response = serveGet(failing, summaryTarget)
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "INTAKE_FAILED" {
		t.Fatalf("failing: %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

// Covers: ADR-0022 — 读用例没有形成答案（上抛了技术错误）：500 + NO_ANSWER_FORMED，
// 不带业务结果格；交回无名结果是编程错误：500 + UNNAMED_OUTCOME。
func TestHandoverScopeSummaryAnswers500WhenNoAnswerIsFormed(t *testing.T) {
	response := serveGet(tfhttp.NewQueryHandoverScopeSummaryEndpoint(grantedIntake(), failingSummarizer{}), summaryTarget)
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("failing: %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)

	response = serveGet(tfhttp.NewQueryHandoverScopeSummaryEndpoint(grantedIntake(), unnamedSummarizer{}), summaryTarget)
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "UNNAMED_OUTCOME" {
		t.Fatalf("unnamed: %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}

// Covers: 逐格转写 — 成立的汇总：三裁决计数分列不折并（CONTEXT 交接硬句），total 与
// allHandedOver 是领域的派生问答原样透出，读者不自己算；租户取自准入结果而不是 URL，
// 范围取自 URL。
func TestHandoverScopeSummaryTranscribesTheDomainSummary(t *testing.T) {
	view := &scopeViewStub{records: []ports.TransportHandoverRecord{
		summaryRecordFor(t, "TENANT-1", "scope-1", "parcel-1", domain.ObjectHandedOver),
		summaryRecordFor(t, "TENANT-1", "scope-1", "parcel-2", domain.ObjectHandedOver),
		summaryRecordFor(t, "TENANT-1", "scope-1", "parcel-3", domain.HandoverRefused),
		summaryRecordFor(t, "TENANT-1", "scope-1", "parcel-4", domain.HandoverPendingConfirmation),
	}}
	response := serveGet(summaryEndpoint(view),
		"/transport-fulfillment-handover-scope-summary?scope=scope-1&tenant=TENANT-EVIL")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if view.askedTenant != "TENANT-1" || view.askedScope != "scope-1" {
		t.Fatalf("读口收到的键走样：tenant=%q scope=%q", view.askedTenant, view.askedScope)
	}
	fields := decodeSummary(t, response)
	if string(fields["outcome"]) != `"SCOPE_SUMMARIZED"` {
		t.Fatalf("outcome = %s", fields["outcome"])
	}
	var summary map[string]json.RawMessage
	if err := json.Unmarshal(fields["summary"], &summary); err != nil {
		t.Fatalf("summary 键缺失或走样：%s", response.Body.String())
	}
	if string(summary["scope"]) != `"scope-1"` ||
		string(summary["handedOver"]) != `2` ||
		string(summary["refused"]) != `1` ||
		string(summary["unconfirmed"]) != `1` ||
		string(summary["total"]) != `4` ||
		string(summary["allHandedOver"]) != `false` {
		t.Fatalf("汇总转写走样：%s", response.Body.String())
	}
	for _, key := range []string{"reason", "continuationReference"} {
		if _, present := fields[key]; present {
			t.Fatalf("成立的汇总不该带 %q 键：%s", key, response.Body.String())
		}
	}
}

// Covers: 领域「空集不成立汇总」在传输层的样子 — 200 + SCOPE_NOT_SUMMARIZABLE，**没有**
// summary 键：三个零会被读者当成「这个范围全部为零」，而它说的是「这个范围还没有交接」。
func TestAnEmptyScopeAnswersNotSummarizableWithoutASummaryBody(t *testing.T) {
	response := serveGet(summaryEndpoint(&scopeViewStub{}), summaryTarget)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	fields := decodeSummary(t, response)
	if string(fields["outcome"]) != `"SCOPE_NOT_SUMMARIZABLE"` {
		t.Fatalf("outcome = %s", fields["outcome"])
	}
	if _, present := fields["summary"]; present {
		t.Fatalf("不成立汇总却带了 summary 键：%s", response.Body.String())
	}
}

// Covers: ADR-0022 — 未决是形成了的答案（200），带封闭原因与续办引用，不冒充空范围、
// 也不升成 5xx：读不回来的续办是重试同一次查询，与「范围里没有交接」的续办不同。
func TestAnUnreadableRegistryAnswersUndecidedWithAContinuation(t *testing.T) {
	response := serveGet(summaryEndpoint(&scopeViewStub{err: errors.New("registry down")}), summaryTarget)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	fields := decodeSummary(t, response)
	if string(fields["outcome"]) != `"SCOPE_UNDECIDED"` ||
		string(fields["reason"]) != `"HANDOVER_REGISTRY_UNAVAILABLE"` {
		t.Fatalf("未决转写走样：%s", response.Body.String())
	}
	if raw, present := fields["continuationReference"]; !present || string(raw) == `""` {
		t.Fatalf("未决没有续办引用：%s", response.Body.String())
	}
	if _, present := fields["summary"]; present {
		t.Fatalf("未决却带了 summary 键：%s", response.Body.String())
	}
}

// Covers: 准入交回立不住的作用域（租户为零值）时，读用例答`输入未受理`——那是形成了的
// 答案（200 + INPUT_NOT_ACCEPTED），不是 5xx；读口一次也不被触到。
func TestAScopeWithoutATenantIsNotAcceptedButStillAnswered(t *testing.T) {
	view := &scopeViewStub{}
	endpoint := tfhttp.NewQueryHandoverScopeSummaryEndpoint(
		zeroScopeIntake{}, application.NewSummarizeHandoverScopeHandler(view),
	)
	response := serveGet(endpoint, summaryTarget)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if fields := decodeSummary(t, response); string(fields["outcome"]) != `"INPUT_NOT_ACCEPTED"` {
		t.Fatalf("outcome = %s", fields["outcome"])
	}
	if view.askedTenant != "" || view.askedScope != "" {
		t.Fatalf("身份立不住却读了库：tenant=%q scope=%q", view.askedTenant, view.askedScope)
	}
}

// zeroScopeIntake 装出一个放行了却交回零值作用域的接入面——装配错误的样子。
type zeroScopeIntake struct{}

func (zeroScopeIntake) IntakeCatalogueQuery(context.Context, *http.Request) (tfhttp.CatalogueQuery, error) {
	return tfhttp.CatalogueQuery{Limit: 25}, nil
}
