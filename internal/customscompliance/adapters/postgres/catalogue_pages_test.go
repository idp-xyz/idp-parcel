package postgres_test

import (
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本上下文各册在真库上的翻页用例（ADR-0144；该票 spec「完成判据」的真库那一条）：翻页期间插入新行不重出、不漏行，total 随筛选与
// q 变，空册答空页且 total 为 0。各册怎么播一行、怎么取一页写在 pagedRegisters 里，每条用例对每册各过一遍。

const (
	pageTenant  = "SYN-TENANT-01"
	otherTenant = "SYN-TENANT-02"
	emptyTenant = "SYN-TENANT-EMPTY"
)

// pagedRegister 是一册在真库上的样子。add 往 tenant 下播第 n 行：各册的键都按 n 编，第 n 行在缺省序里就排在 n 的位置；
// n 大于 2 的行落在 filter 那一组筛选里，other 是同一维上另一个取值（没有筛选维的册两者皆空）。key 是第 n 行的键，本册的 q
// 覆盖键所在的列，键的小写只命中那一行。list 取一页，交回各行的键。
type pagedRegister struct {
	name      string
	catalogue *cataloguepage.Catalogue
	key       func(n int) string
	add       func(t *testing.T, fixture *viewFixture, tenant string, n int)
	list      func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage
	filter    url.Values
	other     url.Values
}

func (register pagedRegister) keys(ns ...int) []string {
	keys := make([]string, 0, len(ns))
	for _, n := range ns {
		keys = append(keys, register.key(n))
	}
	return keys
}

type keyedPage struct {
	keys  []string
	next  string
	total int64
}

func keyed[Row any](t *testing.T, page cataloguepage.Result[Row], err error, key func(Row) string) keyedPage {
	t.Helper()
	if err != nil {
		t.Fatalf("取页：%v", err)
	}
	keys := make([]string, 0, len(page.Rows))
	for _, row := range page.Rows {
		keys = append(keys, key(row))
	}
	return keyedPage{keys: keys, next: page.Next, total: page.Total}
}

// built 接住读口构造函数的两个返回值，交回的函数在构造失败时让用例失败。
func built[Read any](read Read, err error) func(t *testing.T) Read {
	return func(t *testing.T) Read {
		t.Helper()
		if err != nil {
			t.Fatalf("构造读口：%v", err)
		}
		return read
	}
}

func assertKeyedPage(t *testing.T, label string, page keyedPage, keys []string, total int64, more bool) {
	t.Helper()
	if !reflect.DeepEqual(page.keys, keys) || page.total != total || (page.next != "") != more {
		t.Fatalf("%s：得 %v / total %d / next %q，应为 %v / total %d / 还有下一页 %v",
			label, page.keys, page.total, page.next, keys, total, more)
	}
}

func synKey(tag string) func(n int) string {
	return func(n int) string { return fmt.Sprintf("SYN-%s-%02d", tag, n) }
}

// pick 在 n 大于 2 时取 matching，否则取 other——落在 filter 里的就是这几行。
func pick(n int, matching, other string) string {
	if n > 2 {
		return matching
	}
	return other
}

// twoWords 从一份封闭词表里取两个词，升序：落在筛选里的取后一个，解释规则按层排时这几行因此排在后面，与 n 的次序一致。
func twoWords(words []string) (other, matching string) {
	sorted := append([]string(nil), words...)
	sort.Strings(sorted)
	return sorted[0], sorted[1]
}

func pagedRegisters() []pagedRegister {
	tenantOf := func(t *testing.T, tenant string) domain.TenantID { return viewValue(t, domain.NewTenantID, tenant) }
	minutes := func(n int) time.Time { return viewBaseAt.Add(time.Duration(n) * time.Minute) }
	nonLayer, layer := twoWords(cataloguepage.Words[domain.ResultLayer]())

	operations := func(t *testing.T, fixture *viewFixture) *adapter.OperationsRecordViews {
		return built(adapter.NewOperationsRecordViews(fixture.db))(t)
	}
	rules := func(t *testing.T, fixture *viewFixture) *adapter.RuleCatalogue {
		return built(adapter.NewRuleCatalogue(fixture.db))(t)
	}
	caseRegisters := func(t *testing.T, fixture *viewFixture) *adapter.CaseRegisterCatalogue {
		return built(adapter.NewCaseRegisterCatalogue(fixture.db))(t)
	}
	portsPaths := func(t *testing.T, fixture *viewFixture) *adapter.PortsPathsCatalogue {
		return built(adapter.NewPortsPathsCatalogue(fixture.db))(t)
	}
	duty := func(t *testing.T, fixture *viewFixture) *adapter.DutyReconciliationCatalogue {
		return built(adapter.NewDutyReconciliationCatalogue(fixture.db))(t)
	}
	// judgement 播一份出发的动作准入判断，动作时点与判断时刻同为 at。
	judgement := func(t *testing.T, fixture *viewFixture, tenant string, n int, at time.Time, answer string) {
		fixture.seed(t,
			`INSERT INTO customs_compliance.action_admission_judgement
				(tenant_id, judgement_id, object_ref, intent_kind, node_ref, onward_node_ref, action_at,
				 cross_zone, answer, zones, findings, judged_at)
			 VALUES ($1, $2, $3, 'DEPARTURE', 'SYN-NODE-01', 'SYN-NODE-02', $4, 'CROSSING', $5, '[]',
			         jsonb_build_array(jsonb_build_object('action', 'LOADING_DEPARTURE', 'answer', $5::text, 'bases', '[]'::jsonb)),
			         $4)`,
			tenant, synKey("JUDGEMENT")(n), synKey("PARCEL")(n), at, answer)
	}

	return []pagedRegister{
		{
			name: "案件", catalogue: ports.CustomsCasePages, key: synKey("CASE"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.customs_case
						(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at)
					 VALUES ($1, 'SYN-JURISDICTION-01', $2, 'SYN-PROCEDURE-01', $3, $4,
					         jsonb_build_array(jsonb_build_object('parcel', $5::text, 'customer', 'SYN-CUSTOMER-01', 'sourceRef', 'SYN-SOURCE-01')),
					         '[]', $6)`,
					tenant, pick(n, "EXPORT", "IMPORT"), synKey("OBLIGATION")(n), synKey("CASE")(n), synKey("PARCEL")(n), viewBaseAt)
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := operations(t, fixture).ListCustomsCases(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.CustomsCaseRecord) string { return row.Case })
			},
			filter: url.Values{"direction": {"EXPORT"}}, other: url.Values{"direction": {"IMPORT"}},
		},
		{
			name: "提交版本", catalogue: ports.SubmissionVersionPages, key: synKey("UNIT"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.declaration_submission
						(tenant_id, unit_id, procedure_ref, version_id, content_digest, members, dossier_ref, roles_ref,
						 readiness_basis, authority_ref, fixed_at, recorded_at, is_current)
					 VALUES ($1, $2, 'SYN-PROCEDURE-01', $3, $4, jsonb_build_array($5::text), 'SYN-DOSSIER-01', 'SYN-ROLES-01',
					         'SYN-BASIS-01', 'SYN-AUTHORITY-01', $6, $6, $7)`,
					tenant, synKey("UNIT")(n), synKey("VERSION")(n), synKey("DIGEST")(n), synKey("PARCEL")(n), viewBaseAt, n > 2)
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := operations(t, fixture).ListSubmissionVersions(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.SubmissionVersionRecord) string { return row.Unit })
			},
			filter: url.Values{"isCurrent": {"true"}}, other: url.Values{"isCurrent": {"false"}},
		},
		{
			name: "外部结果", catalogue: ports.ExternalResultPages, key: synKey("RESULT"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.external_result
						(tenant_id, source_id, content_digest, unattributable, layer_conflict, result, submission_version, recorded_at)
					 VALUES ($1, $2, $3, false, false,
					         jsonb_build_object('layer', $4::text, 'sourceId', $2::text, 'role', 'CUSTOMS', 'rawSemantics', 'SYN-SEMANTICS',
					                            'rule', 'SYN-RULE/v1', 'version', 'SYN-VERSION-01', 'attempt', 1, 'scope', 'SYN-SCOPE-01',
					                            'occurredAt', '2026-08-14T09:30:00Z', 'receivedAt', '2026-08-14T09:31:00Z'),
					         'SYN-VERSION-01', $5)`,
					tenant, synKey("RESULT")(n), synKey("DIGEST")(n), pick(n, layer, nonLayer), minutes(n))
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := operations(t, fixture).ListExternalResults(t.Context(), tenantOf(t, tenant), "", limit, query)
				return keyed(t, page, err, func(row ports.ExternalResultViewRecord) string { return row.Source })
			},
			filter: url.Values{"layer": {layer}}, other: url.Values{"layer": {nonLayer}},
		},
		{
			// 缺省序是判断时刻倒序：第 n 行的判断时刻随 n 往前推，n 小的在前。
			name: "判断册", catalogue: ports.AdmissionJudgementPages, key: synKey("JUDGEMENT"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				judgement(t, fixture, tenant, n, minutes(100-n), pick(n, "ADMITTED", "BLOCKED"))
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := operations(t, fixture).ListActionAdmissionJudgements(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.ActionAdmissionJudgementRecord) string { return row.Judgement })
			},
			filter: url.Values{"answer": {"ADMITTED"}}, other: url.Values{"answer": {"BLOCKED"}},
		},
		{
			// 每个对象只有一份判断，答复都不过门，于是每个对象都卡关；缺省序是动作时点升序。
			name: "卡关对象", catalogue: ports.UnadmittedObjectPages, key: synKey("PARCEL"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				judgement(t, fixture, tenant, n, minutes(n), pick(n, "UNDECIDED", "BLOCKED"))
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				listed, err := operations(t, fixture).ListUnadmittedObjects(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, listed.Result, err, func(row ports.ActionAdmissionJudgementRecord) string { return row.Object })
			},
			filter: url.Values{"answer": {"UNDECIDED"}}, other: url.Values{"answer": {"BLOCKED"}},
		},
		{
			name: "建案要求规则", catalogue: ports.CaseRequirementRulePages, key: synKey("JURISDICTION"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.case_requirement_rule
						(tenant_id, jurisdiction_ref, direction, procedure_ref, required, basis)
					 VALUES ($1, $2, $3, 'SYN-PROCEDURE-01', true, 'SYN-BASIS-01')`,
					tenant, synKey("JURISDICTION")(n), pick(n, "EXPORT", "IMPORT"))
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := rules(t, fixture).ListCaseRequirementRules(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.CaseRequirementRuleEntry) string { return row.Jurisdiction.String() })
			},
			filter: url.Values{"direction": {"EXPORT"}}, other: url.Values{"direction": {"IMPORT"}},
		},
		{
			name: "解释规则", catalogue: ports.InterpretationRulePages, key: synKey("JURISDICTION"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.interpretation_rule
						(tenant_id, result_layer, jurisdiction_ref, applies_from, applies_until, rule_ref)
					 VALUES ($1, $2, $3, $4, NULL, $5)`,
					tenant, pick(n, layer, nonLayer), synKey("JURISDICTION")(n), viewBaseAt, synKey("RULE")(n))
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := rules(t, fixture).ListInterpretationRules(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.InterpretationRuleEntry) string { return row.Jurisdiction.String() })
			},
			filter: url.Values{"layer": {layer}}, other: url.Values{"layer": {nonLayer}},
		},
		{
			name: "就绪判断", catalogue: ports.ReadinessJudgmentPages, key: synKey("UNIT"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.readiness_judgment (tenant_id, unit_id, basis_ref, judged_at)
					 VALUES ($1, $2, 'SYN-BASIS-01', $3)`,
					tenant, synKey("UNIT")(n), viewBaseAt)
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := caseRegisters(t, fixture).ListReadinessJudgments(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row domain.ReadinessJudgment) string { return row.Unit().String() })
			},
		},
		{
			name: "提交授权", catalogue: ports.SubmissionAuthorityPages, key: synKey("UNIT"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.submission_authority (tenant_id, unit_id, authority_ref, granted_at)
					 VALUES ($1, $2, 'SYN-AUTHORITY-01', $3)`,
					tenant, synKey("UNIT")(n), viewBaseAt)
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := caseRegisters(t, fixture).ListSubmissionAuthorities(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row domain.SubmissionAuthorization) string { return row.Unit().String() })
			},
		},
		{
			name: "关闭义务目录", catalogue: ports.ClosureObligationPages, key: synKey("CASE"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.closure_obligation_catalog (tenant_id, case_ref, registered_at)
					 VALUES ($1, $2, $3)`,
					tenant, synKey("CASE")(n), viewBaseAt)
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := caseRegisters(t, fixture).ListClosureObligations(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.ClosureObligationCatalogueEntry) string { return row.Case.String() })
			},
		},
		{
			name: "门禁条件", catalogue: ports.GateConditionPages, key: synKey("SCOPE"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.gate_condition_catalog (tenant_id, scope_ref, action, boundary_ref, registered_at)
					 VALUES ($1, $2, $3, 'SYN-BOUNDARY-01', $4)`,
					tenant, synKey("SCOPE")(n), pick(n, "OUTBOUND_RELEASE", "FINAL_DELIVERY"), viewBaseAt)
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				catalogue := built(adapter.NewGateConditionCatalogue(fixture.db))(t)
				page, err := catalogue.ListGateConditions(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.GateConditionCatalogueEntry) string { return row.Scope.String() })
			},
			filter: url.Values{"action": {"OUTBOUND_RELEASE"}}, other: url.Values{"action": {"FINAL_DELIVERY"}},
		},
		{
			name: "口岸", catalogue: ports.CandidatePortPages, key: synKey("PORT"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.candidate_port (tenant_id, port_ref, applies_from) VALUES ($1, $2, $3)`,
					tenant, synKey("PORT")(n), viewBaseAt)
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := portsPaths(t, fixture).ListCandidatePorts(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.CandidatePortEntry) string { return row.Port.String() })
			},
		},
		{
			name: "路径", catalogue: ports.DeclarationPathPages, key: synKey("PATH"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.declaration_path
						(tenant_id, path_ref, applies_from, applies_until, port_ref, direction, declaration_mode)
					 VALUES ($1, $2, $3, NULL, 'SYN-PORT-01', $4, 'SYN-MODE-GENERAL')`,
					tenant, synKey("PATH")(n), viewBaseAt, pick(n, "EXPORT", "IMPORT"))
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := portsPaths(t, fixture).ListDeclarationPaths(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.DeclarationPathEntry) string { return row.Path.String() })
			},
			filter: url.Values{"direction": {"EXPORT"}}, other: url.Values{"direction": {"IMPORT"}},
		},
		{
			name: "凭证", catalogue: ports.CredentialPages, key: synKey("CREDENTIAL"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				fixture.seed(t,
					`INSERT INTO customs_compliance.regulatory_credential
						(tenant_id, credential_id, issuer_ref, holder_ref, procedure_ref, valid_from, valid_to, uses, registered_at)
					 VALUES ($1, $2, 'SYN-AUTHORITY-01', 'SYN-HOLDER-01', 'SYN-PROCEDURE-01', $3, $4, 1, $3)`,
					tenant, synKey("CREDENTIAL")(n), viewBaseAt, viewBaseAt.Add(30*24*time.Hour))
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				catalogue := built(adapter.NewCredentialCatalogue(fixture.db))(t)
				page, err := catalogue.ListCredentials(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.CredentialCatalogueEntry) string { return row.Credential.ID().String() })
			},
		},
		{
			name: "协作事项", catalogue: ports.DutyCollaborationPages, key: synKey("SCOPE"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				dutyRef, noPayBasis := synKey("DUTY")(n), ""
				if n <= 2 {
					dutyRef, noPayBasis = "", "SYN-PROGRAM-01"
				}
				fixture.seed(t,
					`INSERT INTO customs_compliance.duty_payment_collaboration
						(tenant_id, scope_ref, duty_ref, kind, no_pay_basis, obligor_ref, requirement_ref, target_ref, formed_at)
					 VALUES ($1, $2, $3, $4, $5, 'SYN-OBLIGOR-01', 'SYN-REQUIREMENT-01', 'SYN-DUTY-DESK', $6)`,
					tenant, synKey("SCOPE")(n), dutyRef, pick(n, "ASSESSED_DUTY", "EXPLICITLY_NOT_REQUIRED"), noPayBasis, viewBaseAt)
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := duty(t, fixture).ListDutyCollaborations(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row domain.DutyPaymentCollaboration) string { return row.Scope().String() })
			},
			filter: url.Values{"kind": {"ASSESSED_DUTY"}}, other: url.Values{"kind": {"EXPLICITLY_NOT_REQUIRED"}},
		},
		{
			// 核对行的外键钉资金事实及其版本：每行各播一条自己的资金事实。
			name: "核对", catalogue: ports.DutyVerificationPages, key: synKey("DUTY"),
			add: func(t *testing.T, fixture *viewFixture, tenant string, n int) {
				funds := synKey("FUNDS")(n)
				seedFundsFact(t, fixture, tenant, funds)
				fixture.seed(t,
					`INSERT INTO customs_compliance.duty_payment_verification
						(tenant_id, duty_ref, funds_ref, scope_ref, version_digest, funds_version, procedure_ref,
						 coverage, delta, validity, basis, verified_at)
					 VALUES ($1, $2, $3, 'SYN-SCOPE-01', $4, $5, 'SYN-PROCEDURE-01', $6, $7, $8, 'SYN-RULE-01', $9)`,
					tenant, synKey("DUTY")(n), funds, synKey("DIGEST")(n), funds+"/v1",
					pick(n, "COVERED", "NONE"), pick(n, "NO_DELTA", "PENDING"), pick(n, "VALID", "PENDING"), viewBaseAt)
			},
			list: func(t *testing.T, fixture *viewFixture, tenant string, limit int, query cataloguepage.Query) keyedPage {
				page, err := duty(t, fixture).ListDutyVerifications(t.Context(), tenantOf(t, tenant), limit, query)
				return keyed(t, page, err, func(row ports.DutyVerificationRecord) string { return row.Key.Duty.String() })
			},
			filter: url.Values{"coverage": {"COVERED"}}, other: url.Values{"coverage": {"NONE"}},
		},
	}
}

func joined(groups ...url.Values) url.Values {
	values := url.Values{}
	for _, group := range groups {
		for key, given := range group {
			values[key] = append(values[key], given...)
		}
	}
	return values
}

// Covers: ADR-0144 决定一——各册各自：按「排序维值 + 标识」的游标翻页。翻完第一页后插入两行，一行落在已翻过的那头、一行落在
// 未翻的那头：第二页不重出第一页的行（偏移分页会在第二页开头重出第一页的末行），新落在未翻那头的那一行照样翻到；total 随之由
// 4 变 6，末页 next 为空。他租户同形的行不进来。
func TestCustomsCataloguesPageByCursorWithoutRepeatsOrGapsWhileRowsArrive(t *testing.T) {
	for _, register := range pagedRegisters() {
		t.Run(register.name, func(t *testing.T) {
			fixture := newViewFixture(t)
			for _, n := range []int{2, 4, 6, 8} {
				register.add(t, fixture, pageTenant, n)
			}
			register.add(t, fixture, otherTenant, 3)

			first := register.list(t, fixture, pageTenant, 2, firstPage(t, register.catalogue))
			assertKeyedPage(t, "第一页", first, register.keys(2, 4), 4, true)

			register.add(t, fixture, pageTenant, 1)
			register.add(t, fixture, pageTenant, 5)
			second := register.list(t, fixture, pageTenant, 2, nextPage(t, register.catalogue, url.Values{}, first.next))
			assertKeyedPage(t, "第二页", second, register.keys(5, 6), 6, true)

			last := register.list(t, fixture, pageTenant, 2, nextPage(t, register.catalogue, url.Values{}, second.next))
			assertKeyedPage(t, "末页", last, register.keys(8), 6, false)
		})
	}
}

// Covers: ADR-0144 决定四、五——各册各自：筛选维内为或、维与 q 为与；q 在本册声明的列上做不分大小写的字面包含（% 按字面、
// 零命中）；total 与本页出自同一组条件。空册答空页且 total 为 0（ADR-0077 Decision 四）。
func TestCustomsCatalogueTotalsFollowTheFiltersAndTheKeyword(t *testing.T) {
	for _, register := range pagedRegisters() {
		t.Run(register.name, func(t *testing.T) {
			fixture := newViewFixture(t)
			for n := 1; n <= 4; n++ {
				register.add(t, fixture, pageTenant, n)
			}
			type pageCase struct {
				values url.Values
				want   []string
			}
			cases := map[string]pageCase{
				"不带条件":      {values: url.Values{}, want: register.keys(1, 2, 3, 4)},
				"q 只命中一行":   {values: url.Values{"q": {strings.ToLower(register.key(3))}}, want: register.keys(3)},
				"q 的 % 按字面": {values: url.Values{"q": {"%"}}, want: []string{}},
			}
			if register.filter != nil {
				cases["一个取值"] = pageCase{values: register.filter, want: register.keys(3, 4)}
				cases["维内为或"] = pageCase{values: joined(register.filter, register.other), want: register.keys(1, 2, 3, 4)}
				cases["维与 q 为与"] = pageCase{values: joined(register.filter, url.Values{"q": {register.key(1)}}), want: []string{}}
			}
			for name, test := range cases {
				t.Run(name, func(t *testing.T) {
					page := register.list(t, fixture, pageTenant, 10, pageQuery(t, register.catalogue, test.values))
					assertKeyedPage(t, name, page, test.want, int64(len(test.want)), false)
				})
			}

			empty := register.list(t, fixture, emptyTenant, 10, firstPage(t, register.catalogue))
			assertKeyedPage(t, "空册", empty, []string{}, 0, false)
		})
	}
}

// Covers: 带计数与 truncated 的待办队列保留这两格——卡关对象的积压件数是整个队列的，不随筛选与 q 变；total 是这一组条件下的件数，本页一件也没有时
// 两个数照样答出。
func TestTheUnadmittedBacklogStaysWholeUnderFiltersAndTheKeyword(t *testing.T) {
	fixture := newViewFixture(t)
	var register pagedRegister
	for _, candidate := range pagedRegisters() {
		if candidate.catalogue == ports.UnadmittedObjectPages {
			register = candidate
		}
	}
	for n := 1; n <= 4; n++ {
		register.add(t, fixture, pageTenant, n)
	}
	views := built(adapter.NewOperationsRecordViews(fixture.db))(t)
	for name, test := range map[string]struct {
		values url.Values
		total  int64
	}{
		"不带条件": {values: url.Values{}, total: 4},
		"按答复筛": {values: register.filter, total: 2},
		"q":    {values: url.Values{"q": {strings.ToLower(register.key(3))}}, total: 1},
		"零命中":  {values: url.Values{"q": {"%"}}, total: 0},
	} {
		t.Run(name, func(t *testing.T) {
			listed, err := views.ListUnadmittedObjects(t.Context(), viewValue(t, domain.NewTenantID, pageTenant), 1,
				pageQuery(t, ports.UnadmittedObjectPages, test.values))
			if err != nil {
				t.Fatal(err)
			}
			if listed.Count != 4 || listed.Total != test.total || (listed.Next != "") != (test.total > 1) {
				t.Fatalf("积压 %d / total %d / next %q，应为积压 4 / total %d", listed.Count, listed.Total, listed.Next, test.total)
			}
		})
	}
}

// Covers: q 落在包裹号数组上只比元素——数组 JSON 文本里的引号、逗号与括号不被命中，两个包裹号首尾相接拼出的串也不命中；
// 命中第二个包裹号的照样找到。案件的 parcels 与提交版本的 members 各过一遍。
func TestTheKeywordOnParcelArraysMatchesElementsOnly(t *testing.T) {
	fixture := newViewFixture(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.customs_case
			(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at)
		 VALUES ($1, 'SYN-JURISDICTION-01', 'EXPORT', 'SYN-PROCEDURE-01', 'SYN-OBLIGATION-01', 'SYN-CASE-01',
		         jsonb_build_array(
		             jsonb_build_object('parcel', 'SYN-PARCEL-01', 'customer', 'SYN-CUSTOMER-01', 'sourceRef', 'SYN-SOURCE-01'),
		             jsonb_build_object('parcel', 'SYN-PARCEL-02', 'customer', 'SYN-CUSTOMER-01', 'sourceRef', 'SYN-SOURCE-02')),
		         '[]', $2)`,
		pageTenant, viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_submission
			(tenant_id, unit_id, procedure_ref, version_id, content_digest, members, dossier_ref, roles_ref,
			 readiness_basis, authority_ref, fixed_at, recorded_at, is_current)
		 VALUES ($1, 'SYN-UNIT-01', 'SYN-PROCEDURE-01', 'SYN-VERSION-01', 'SYN-DIGEST-01',
		         jsonb_build_array('SYN-PARCEL-01', 'SYN-PARCEL-02'), 'SYN-DOSSIER-01', 'SYN-ROLES-01',
		         'SYN-BASIS-01', 'SYN-AUTHORITY-01', $2, $2, true)`,
		pageTenant, viewBaseAt)
	views := built(adapter.NewOperationsRecordViews(fixture.db))(t)
	tenant := viewValue(t, domain.NewTenantID, pageTenant)
	for keyword, want := range map[string]int64{`"`: 0, ",": 0, "[": 0, "01SYN": 0, "parcel-02": 1} {
		t.Run(keyword, func(t *testing.T) {
			values := url.Values{"q": {keyword}}
			cases, err := views.ListCustomsCases(t.Context(), tenant, 10, pageQuery(t, ports.CustomsCasePages, values))
			if err != nil {
				t.Fatal(err)
			}
			submissions, err := views.ListSubmissionVersions(t.Context(), tenant, 10,
				pageQuery(t, ports.SubmissionVersionPages, values))
			if err != nil {
				t.Fatal(err)
			}
			if cases.Total != want || submissions.Total != want {
				t.Fatalf("q=%q：案件 total %d、提交版本 total %d，应各为 %d", keyword, cases.Total, submissions.Total, want)
			}
		})
	}
}
