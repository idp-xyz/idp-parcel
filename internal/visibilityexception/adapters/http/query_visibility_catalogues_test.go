package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// catalogueReaderDouble 按注入行作答，并记录收到的租户与页大小——端点必须把 Intake
// 裁决的作用域与 limit 原样递给读口，不采信请求里的任何自报。
type catalogueReaderDouble struct {
	tenant domain.TenantID
	limit  int

	mappings       []ports.MilestoneMappingCatalogueRow
	triage         []ports.TriageRuleCatalogueRow
	notifications  []ports.NotificationPolicyCatalogueRow
	eligibilities  []ports.ClaimEligibilityCatalogueRow
	authorizations []ports.ClaimAuthorizationCatalogueRow
	disclosures    []ports.DisclosurePolicyCatalogueRow
	disclosureRule []ports.ExceptionDisclosureRuleCatalogueRow
	conflictRules  []ports.ConflictSignalRuleCatalogueRow
	err            error
}

func (double *catalogueReaderDouble) record(tenant domain.TenantID, limit int) error {
	double.tenant = tenant
	double.limit = limit
	return double.err
}

func (double *catalogueReaderDouble) ListMilestoneMappings(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.MilestoneMappingCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.mappings, nil
}

func (double *catalogueReaderDouble) ListTriageRules(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.TriageRuleCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.triage, nil
}

func (double *catalogueReaderDouble) ListNotificationPolicies(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.NotificationPolicyCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.notifications, nil
}

func (double *catalogueReaderDouble) ListClaimEligibilities(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.ClaimEligibilityCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.eligibilities, nil
}

func (double *catalogueReaderDouble) ListClaimAuthorizations(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.ClaimAuthorizationCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.authorizations, nil
}

func (double *catalogueReaderDouble) ListDisclosurePolicies(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.DisclosurePolicyCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.disclosures, nil
}

func (double *catalogueReaderDouble) ListExceptionDisclosureRules(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.ExceptionDisclosureRuleCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.disclosureRule, nil
}

func (double *catalogueReaderDouble) ListConflictSignalRules(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.ConflictSignalRuleCatalogueRow, error) {
	if err := double.record(tenant, limit); err != nil {
		return nil, err
	}
	return double.conflictRules, nil
}

// unreachableCatalogueReader 是「被调即失败」的替身：未配置 Intake 的合同就是不构造
// 查询，读口若被触到，说明有请求穿过了未配置格。
type unreachableCatalogueReader struct{ t *testing.T }

func (reader unreachableCatalogueReader) fail() {
	reader.t.Error("a request passed the unconfigured intake and reached the catalogue reader")
}

func (reader unreachableCatalogueReader) ListMilestoneMappings(
	context.Context, domain.TenantID, int,
) ([]ports.MilestoneMappingCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

func (reader unreachableCatalogueReader) ListTriageRules(
	context.Context, domain.TenantID, int,
) ([]ports.TriageRuleCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

func (reader unreachableCatalogueReader) ListNotificationPolicies(
	context.Context, domain.TenantID, int,
) ([]ports.NotificationPolicyCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

func (reader unreachableCatalogueReader) ListClaimEligibilities(
	context.Context, domain.TenantID, int,
) ([]ports.ClaimEligibilityCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

func (reader unreachableCatalogueReader) ListClaimAuthorizations(
	context.Context, domain.TenantID, int,
) ([]ports.ClaimAuthorizationCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

func (reader unreachableCatalogueReader) ListDisclosurePolicies(
	context.Context, domain.TenantID, int,
) ([]ports.DisclosurePolicyCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

func (reader unreachableCatalogueReader) ListExceptionDisclosureRules(
	context.Context, domain.TenantID, int,
) ([]ports.ExceptionDisclosureRuleCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

func (reader unreachableCatalogueReader) ListConflictSignalRules(
	context.Context, domain.TenantID, int,
) ([]ports.ConflictSignalRuleCatalogueRow, error) {
	reader.fail()
	return nil, nil
}

var visibilityCatalogueKinds = []string{
	"MILESTONE_MAPPING", "TRIAGE_RULE", "NOTIFICATION_POLICY",
	"CLAIM_ELIGIBILITY", "CLAIM_AUTHORIZATION", "DISCLOSURE_POLICY",
	"EXCEPTION_DISCLOSURE_RULE", "CONFLICT_SIGNAL_RULE",
}

func vcServe(
	t *testing.T,
	intake visibilityhttp.OperationsTrackingIntake,
	reader visibilityhttp.VisibilityCatalogueReader,
	method, target string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := visibilityhttp.NewQueryVisibilityCataloguesEndpoint(intake, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

// Covers: kind 是传输形状——缺席或集外按坏请求拒（六种册子行形状互不相同，替调用方
// 选就是猜）；方法检查同级先行。
func TestVisibilityCataloguesRefuseWrongMethodAndForeignKind(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}

	wrongMethod := vcServe(t, intake, &catalogueReaderDouble{},
		http.MethodPost, "/visibility-catalogues?kind=TRIAGE_RULE")
	if wrongMethod.Code != http.StatusMethodNotAllowed ||
		wrongMethod.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d allow = %q, want 405/GET",
			wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}

	for name, target := range map[string]string{
		"kind 缺席": "/visibility-catalogues",
		"kind 集外": "/visibility-catalogues?kind=EXCEPTION_CASE",
	} {
		response := vcServe(t, intake, &catalogueReaderDouble{}, http.MethodGet, target)
		if response.Code != http.StatusBadRequest ||
			problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s：status = %d body = %s，want 400 MALFORMED_REQUEST",
				name, response.Code, response.Body.String())
		}
	}
}

// Covers: 完成标准「未启用隔离读准入时答 403，与既有查阅端点同签名」——未配置 Intake
// 对全部六种 kind 同答 403 + ACCESS_CHANNEL_NOT_CONFIGURED，读口不被触到，4xx 不带
// outcome（ADR-0022）。
func TestVisibilityCataloguesUnconfiguredIntakeRefusesEveryKind(t *testing.T) {
	for _, kind := range visibilityCatalogueKinds {
		response := vcServe(t, visibilityhttp.UnconfiguredIntake{},
			unreachableCatalogueReader{t: t},
			http.MethodGet, "/visibility-catalogues?kind="+kind)
		if response.Code != http.StatusForbidden ||
			problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("kind=%s：status = %d body = %s，want 403 ACCESS_CHANNEL_NOT_CONFIGURED",
				kind, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	}
}

// Covers: 六种 kind 各自 200 + kind 回显 + 行体逐字段透出；作用域与页大小来自 Intake
// 裁决而不是请求自报；空册交回空数组而不是 null（ADR-0077 Decision 四）。
func TestVisibilityCataloguesListEachKindVerbatim(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	closedAt := operationsBaseAt.Add(48 * time.Hour)
	reader := &catalogueReaderDouble{
		mappings: []ports.MilestoneMappingCatalogueRow{{
			Version:        "map/v1",
			EffectiveFrom:  operationsBaseAt,
			EffectiveTo:    closedAt,
			HasEffectiveTo: true,
			ApprovedBy:     "tracking-ops",
			Entries: []ports.MilestoneMappingEntryRow{{
				Source: "NODE_OPERATIONS", FactKind: "node-intake", Milestone: "PICKED_UP",
			}},
		}},
		triage: []ports.TriageRuleCatalogueRow{{
			Version:       "triage/v1",
			EffectiveFrom: operationsBaseAt,
			ApprovedBy:    "tracking-ops",
			Entries: []ports.TriageRuleEntryRow{{
				SignalKind: "DELIVERY_FAILED", Confidence: "CARRIER_CONFIRMED", Outcome: "AUTO_ESTABLISH",
			}},
		}},
		notifications: []ports.NotificationPolicyCatalogueRow{{
			Policy: "disclose/v1", Channel: "EMAIL", DeadlineAfter: "72:00:00",
			Obligation: "ACK_REQUIRED", ApprovedBy: "tracking-ops",
		}},
		eligibilities: []ports.ClaimEligibilityCatalogueRow{{
			Contract: "CONTRACT-01", Version: "claims/v1", ApprovedBy: "claims-ops",
			CoveredKinds: []string{"DAMAGE", "LOSS"},
		}},
		authorizations: []ports.ClaimAuthorizationCatalogueRow{{
			Customer: "CUST-02", Version: "auth/v1", ApprovedBy: "claims-ops",
			Applicants: []string{},
		}},
		disclosures: []ports.DisclosurePolicyCatalogueRow{{
			Version:       "disclose/v1",
			EffectiveFrom: operationsBaseAt,
			ApprovedBy:    "tracking-ops",
			Entries: []ports.DisclosurePolicyEntryRow{{
				Customer:   "CUST-01",
				Milestones: ports.DisclosureDimensionCell{State: "SHOWN", Content: "public-milestones/v1"},
				ETA:        ports.DisclosureDimensionCell{State: "PENDING_CONFIRMATION"},
				Final:      ports.DisclosureDimensionCell{State: "NOT_DISCLOSED"},
				Note:       ports.DisclosureDimensionCell{State: "SHOWN", Content: "note/plain"},
			}},
		}},
	}

	type envelope struct {
		Outcome    string            `json:"outcome"`
		Kind       string            `json:"kind"`
		Catalogues []json.RawMessage `json:"catalogues"`
	}
	decode := func(t *testing.T, kind string) envelope {
		t.Helper()
		response := vcServe(t, intake, reader,
			http.MethodGet, "/visibility-catalogues?kind="+kind)
		if response.Code != http.StatusOK {
			t.Fatalf("kind=%s：status = %d body = %s", kind, response.Code, response.Body.String())
		}
		var body envelope
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("kind=%s decode: %v", kind, err)
		}
		if body.Outcome != "VISIBILITY_CATALOGUES_LISTED" || body.Kind != kind {
			t.Fatalf("kind=%s：outcome = %q kind = %q", kind, body.Outcome, body.Kind)
		}
		if reader.tenant.String() != "tenant-1" || reader.limit != 50 {
			t.Fatalf("kind=%s：读口收到 tenant=%q limit=%d，应来自 Intake 裁决（tenant-1/50）",
				kind, reader.tenant, reader.limit)
		}
		return body
	}

	mapping := decode(t, "MILESTONE_MAPPING")
	var mappingBody struct {
		Version       string `json:"version"`
		EffectiveFrom string `json:"effectiveFrom"`
		EffectiveTo   string `json:"effectiveTo"`
		ApprovedBy    string `json:"approvedBy"`
		Entries       []struct {
			Source    string `json:"source"`
			FactKind  string `json:"factKind"`
			Milestone string `json:"milestone"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(mapping.Catalogues[0], &mappingBody); err != nil {
		t.Fatalf("decode mapping: %v", err)
	}
	if mappingBody.Version != "map/v1" ||
		mappingBody.EffectiveFrom != operationsBaseAt.Format(time.RFC3339Nano) ||
		mappingBody.EffectiveTo != closedAt.Format(time.RFC3339Nano) ||
		mappingBody.ApprovedBy != "tracking-ops" ||
		len(mappingBody.Entries) != 1 || mappingBody.Entries[0].FactKind != "node-intake" ||
		mappingBody.Entries[0].Source != "NODE_OPERATIONS" ||
		mappingBody.Entries[0].Milestone != "PICKED_UP" {
		t.Fatalf("映射行体走样：%+v", mappingBody)
	}

	triage := decode(t, "TRIAGE_RULE")
	var triageBody struct {
		Version     string `json:"version"`
		EffectiveTo any    `json:"effectiveTo"`
		Entries     []struct {
			SignalKind string `json:"signalKind"`
			Confidence string `json:"confidence"`
			Outcome    string `json:"outcome"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(triage.Catalogues[0], &triageBody); err != nil {
		t.Fatalf("decode triage: %v", err)
	}
	if triageBody.Version != "triage/v1" || triageBody.EffectiveTo != nil ||
		len(triageBody.Entries) != 1 || triageBody.Entries[0].Outcome != "AUTO_ESTABLISH" {
		t.Fatalf("分诊行体走样（当前版不得带 effectiveTo）：%+v", triageBody)
	}

	notification := decode(t, "NOTIFICATION_POLICY")
	var notificationBody struct {
		Policy        string `json:"policy"`
		Channel       string `json:"channel"`
		DeadlineAfter string `json:"deadlineAfter"`
		Obligation    string `json:"obligation"`
	}
	if err := json.Unmarshal(notification.Catalogues[0], &notificationBody); err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	if notificationBody.Policy != "disclose/v1" || notificationBody.DeadlineAfter != "72:00:00" ||
		notificationBody.Channel != "EMAIL" || notificationBody.Obligation != "ACK_REQUIRED" {
		t.Fatalf("通知行体走样：%+v", notificationBody)
	}

	eligibility := decode(t, "CLAIM_ELIGIBILITY")
	var eligibilityBody struct {
		Contract     string   `json:"contract"`
		CoveredKinds []string `json:"coveredKinds"`
	}
	if err := json.Unmarshal(eligibility.Catalogues[0], &eligibilityBody); err != nil {
		t.Fatalf("decode eligibility: %v", err)
	}
	if eligibilityBody.Contract != "CONTRACT-01" || len(eligibilityBody.CoveredKinds) != 2 {
		t.Fatalf("资格行体走样：%+v", eligibilityBody)
	}

	authorization := decode(t, "CLAIM_AUTHORIZATION")
	var authorizationBody struct {
		Customer   string   `json:"customer"`
		Applicants []string `json:"applicants"`
	}
	if err := json.Unmarshal(authorization.Catalogues[0], &authorizationBody); err != nil {
		t.Fatalf("decode authorization: %v", err)
	}
	// 空名单必须是显式空数组：目录在场而不授权任何人是一次已作出的决定（0018）。
	if authorizationBody.Customer != "CUST-02" || authorizationBody.Applicants == nil ||
		len(authorizationBody.Applicants) != 0 {
		t.Fatalf("授权行体走样（空名单应为 [] 而非 null）：%s", authorization.Catalogues[0])
	}

	disclosure := decode(t, "DISCLOSURE_POLICY")
	var disclosureBody struct {
		Version string `json:"version"`
		Entries []struct {
			Customer   string          `json:"customer"`
			Milestones json.RawMessage `json:"milestones"`
			ETA        json.RawMessage `json:"eta"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(disclosure.Catalogues[0], &disclosureBody); err != nil {
		t.Fatalf("decode disclosure: %v", err)
	}
	if disclosureBody.Version != "disclose/v1" || len(disclosureBody.Entries) != 1 {
		t.Fatalf("披露行体走样：%+v", disclosureBody)
	}
	if string(disclosureBody.Entries[0].Milestones) !=
		`{"state":"SHOWN","content":"public-milestones/v1"}` {
		t.Fatalf("展示维应带内容：%s", disclosureBody.Entries[0].Milestones)
	}
	// 非展示维不带 content 字段——缺席即「这一维不带引用」，与空引用分得开。
	if string(disclosureBody.Entries[0].ETA) != `{"state":"PENDING_CONFIRMATION"}` {
		t.Fatalf("待确认维不得带内容字段：%s", disclosureBody.Entries[0].ETA)
	}

	empty := vcServe(t, intake, &catalogueReaderDouble{},
		http.MethodGet, "/visibility-catalogues?kind=MILESTONE_MAPPING")
	var emptyBody envelope
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if empty.Code != http.StatusOK || emptyBody.Catalogues == nil {
		t.Fatalf("空册 = %d %s；want 200 + 空数组", empty.Code, empty.Body.String())
	}
}

// Covers: 异常披露规则（0023）与冲突信号规则（0025）两册各自 200 + kind 回显 + 行体逐键透出
// （票 ve-disclosure-policy-view/03）；两册的空册同答 LISTED + 空数组而不是 null（ADR-0077
// Decision 四）。规则条目的 content 只在 disclosable 时在场——缺席即「这一条不带内容来处」，
// 与空引用分得开（0023 成对约束的传输镜像）。
func TestVisibilityCataloguesListRuleCataloguesVerbatim(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	registeredAt := operationsBaseAt.Add(time.Hour)
	reader := &catalogueReaderDouble{
		disclosureRule: []ports.ExceptionDisclosureRuleCatalogueRow{{
			Version:       "disclose-rule/v1",
			EffectiveFrom: operationsBaseAt,
			ApprovedBy:    "tracking-ops",
			Entries: []ports.ExceptionDisclosureRuleEntryRow{
				{
					Customer: "CUST-01", SignalKind: "ADDRESS_UNKNOWN", Confidence: "HEURISTIC",
					Disclosable: false, AutoRelease: false,
				},
				{
					Customer: "CUST-02", SignalKind: "DELIVERY_FAILED", Confidence: "CARRIER_CONFIRMED",
					Disclosable: true, AutoRelease: true, Content: "delivery-failed/plain",
				},
			},
		}},
		conflictRules: []ports.ConflictSignalRuleCatalogueRow{{
			SignalKind: "FACT_CONFLICT_PENDING", Version: "conflict/v1",
			Confidence: "ALTERNATIVE_CHAIN_FORK", ApprovedBy: "tracking-ops", RegisteredAt: registeredAt,
		}},
	}

	type envelope struct {
		Outcome    string            `json:"outcome"`
		Kind       string            `json:"kind"`
		Catalogues []json.RawMessage `json:"catalogues"`
	}
	decode := func(t *testing.T, reader visibilityhttp.VisibilityCatalogueReader, kind string) envelope {
		t.Helper()
		response := vcServe(t, intake, reader, http.MethodGet, "/visibility-catalogues?kind="+kind)
		if response.Code != http.StatusOK {
			t.Fatalf("kind=%s：status = %d body = %s", kind, response.Code, response.Body.String())
		}
		var body envelope
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("kind=%s decode: %v", kind, err)
		}
		if body.Outcome != "VISIBILITY_CATALOGUES_LISTED" || body.Kind != kind {
			t.Fatalf("kind=%s：outcome = %q kind = %q", kind, body.Outcome, body.Kind)
		}
		return body
	}

	rule := decode(t, reader, "EXCEPTION_DISCLOSURE_RULE")
	if reader.tenant.String() != "tenant-1" || reader.limit != 50 {
		t.Fatalf("读口收到 tenant=%q limit=%d，应来自 Intake 裁决（tenant-1/50）", reader.tenant, reader.limit)
	}
	var ruleBody struct {
		Version       string            `json:"version"`
		EffectiveFrom string            `json:"effectiveFrom"`
		EffectiveTo   any               `json:"effectiveTo"`
		ApprovedBy    string            `json:"approvedBy"`
		Entries       []json.RawMessage `json:"entries"`
	}
	if len(rule.Catalogues) != 1 {
		t.Fatalf("规则册应一版，实得 %d", len(rule.Catalogues))
	}
	if err := json.Unmarshal(rule.Catalogues[0], &ruleBody); err != nil {
		t.Fatalf("decode rule: %v", err)
	}
	if ruleBody.Version != "disclose-rule/v1" || ruleBody.EffectiveTo != nil ||
		ruleBody.EffectiveFrom != operationsBaseAt.Format(time.RFC3339Nano) ||
		ruleBody.ApprovedBy != "tracking-ops" || len(ruleBody.Entries) != 2 {
		t.Fatalf("异常披露规则行体走样（当前版不得带 effectiveTo）：%+v", ruleBody)
	}
	if string(ruleBody.Entries[0]) !=
		`{"customer":"CUST-01","signalKind":"ADDRESS_UNKNOWN","confidence":"HEURISTIC","disclosable":false,"autoRelease":false}` {
		t.Fatalf("不披露条目不得带 content 字段：%s", ruleBody.Entries[0])
	}
	if string(ruleBody.Entries[1]) !=
		`{"customer":"CUST-02","signalKind":"DELIVERY_FAILED","confidence":"CARRIER_CONFIRMED","disclosable":true,"autoRelease":true,"content":"delivery-failed/plain"}` {
		t.Fatalf("披露条目应逐键带内容来处：%s", ruleBody.Entries[1])
	}

	conflict := decode(t, reader, "CONFLICT_SIGNAL_RULE")
	if len(conflict.Catalogues) != 1 {
		t.Fatalf("冲突信号规则册应一行，实得 %d", len(conflict.Catalogues))
	}
	if string(conflict.Catalogues[0]) !=
		`{"signalKind":"FACT_CONFLICT_PENDING","version":"conflict/v1","confidence":"ALTERNATIVE_CHAIN_FORK","approvedBy":"tracking-ops","registeredAt":"`+
			registeredAt.Format(time.RFC3339Nano)+`"}` {
		t.Fatalf("冲突信号规则行体应逐键透出：%s", conflict.Catalogues[0])
	}

	for _, kind := range []string{"EXCEPTION_DISCLOSURE_RULE", "CONFLICT_SIGNAL_RULE"} {
		empty := decode(t, &catalogueReaderDouble{}, kind)
		if empty.Catalogues == nil || len(empty.Catalogues) != 0 {
			t.Fatalf("kind=%s 空册应为 LISTED + 空数组，实得 %v", kind, empty.Catalogues)
		}
	}
}

// Covers: 读不回是答案未形成（500 + NO_ANSWER_FORMED），不伪装成空册——那会让一次
// 该重试的故障变成一份看起来如实的空登记册。
func TestVisibilityCataloguesReaderFailureFormsNoAnswer(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	for _, kind := range visibilityCatalogueKinds {
		response := vcServe(t, intake,
			&catalogueReaderDouble{err: context.DeadlineExceeded},
			http.MethodGet, "/visibility-catalogues?kind="+kind)
		if response.Code != http.StatusInternalServerError ||
			problemCode(t, response) != "NO_ANSWER_FORMED" {
			t.Fatalf("kind=%s：status = %d body = %s，want 500 NO_ANSWER_FORMED",
				kind, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	}
}
