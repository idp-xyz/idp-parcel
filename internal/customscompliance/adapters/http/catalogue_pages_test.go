package customshttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// pageRecorder 是本上下文各册读口的同一个替身：记下端点交来的租户、页大小与查询对象（外部结果另记版本选择器），交回预置的
// next、total 与积压件数，不交行——各册行的转写另有用例。
type pageRecorder struct {
	next  string
	total int64
	count int

	called  bool
	tenant  string
	limit   int
	query   cataloguepage.Query
	version string
}

var (
	_ customshttp.CustomsCaseRecordReader          = (*pageRecorder)(nil)
	_ customshttp.SubmissionRecordReader           = (*pageRecorder)(nil)
	_ customshttp.ExternalResultViewReader         = (*pageRecorder)(nil)
	_ customshttp.ActionAdmissionJudgementReader   = (*pageRecorder)(nil)
	_ customshttp.UnadmittedObjectReader           = (*pageRecorder)(nil)
	_ customshttp.RuleCatalogueReader              = (*pageRecorder)(nil)
	_ customshttp.CaseRegisterCatalogueReader      = (*pageRecorder)(nil)
	_ customshttp.GateConditionCatalogueReader     = (*pageRecorder)(nil)
	_ customshttp.PortsPathsCatalogueReader        = (*pageRecorder)(nil)
	_ customshttp.CredentialCatalogueReader        = (*pageRecorder)(nil)
	_ customshttp.DutyCollaborationCatalogueReader = (*pageRecorder)(nil)
	_ customshttp.DutyVerificationCatalogueReader  = (*pageRecorder)(nil)
)

func recorded[Row any](
	recorder *pageRecorder, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[Row], error) {
	recorder.called, recorder.tenant, recorder.limit, recorder.query = true, tenant.String(), limit, query
	return cataloguepage.Result[Row]{Next: recorder.next, Total: recorder.total}, nil
}

func (recorder *pageRecorder) ListCustomsCases(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomsCaseRecord], error) {
	return recorded[ports.CustomsCaseRecord](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListSubmissionVersions(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.SubmissionVersionRecord], error) {
	return recorded[ports.SubmissionVersionRecord](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListExternalResults(
	_ context.Context, tenant domain.TenantID, version string, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.ExternalResultViewRecord], error) {
	recorder.version = version
	return recorded[ports.ExternalResultViewRecord](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListActionAdmissionJudgements(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.ActionAdmissionJudgementRecord], error) {
	return recorded[ports.ActionAdmissionJudgementRecord](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListUnadmittedObjects(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (ports.UnadmittedObjects, error) {
	result, err := recorded[ports.ActionAdmissionJudgementRecord](recorder, tenant, limit, query)
	return ports.UnadmittedObjects{Result: result, Count: recorder.count}, err
}

func (recorder *pageRecorder) ListCaseRequirementRules(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.CaseRequirementRuleEntry], error) {
	return recorded[ports.CaseRequirementRuleEntry](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListInterpretationRules(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.InterpretationRuleEntry], error) {
	return recorded[ports.InterpretationRuleEntry](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListReadinessJudgments(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[domain.ReadinessJudgment], error) {
	return recorded[domain.ReadinessJudgment](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListSubmissionAuthorities(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[domain.SubmissionAuthorization], error) {
	return recorded[domain.SubmissionAuthorization](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListClosureObligations(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.ClosureObligationCatalogueEntry], error) {
	return recorded[ports.ClosureObligationCatalogueEntry](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListGateConditions(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.GateConditionCatalogueEntry], error) {
	return recorded[ports.GateConditionCatalogueEntry](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListCandidatePorts(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.CandidatePortEntry], error) {
	return recorded[ports.CandidatePortEntry](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListDeclarationPaths(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.DeclarationPathEntry], error) {
	return recorded[ports.DeclarationPathEntry](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListCredentials(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.CredentialCatalogueEntry], error) {
	return recorded[ports.CredentialCatalogueEntry](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListDutyCollaborations(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[domain.DutyPaymentCollaboration], error) {
	return recorded[domain.DutyPaymentCollaboration](recorder, tenant, limit, query)
}

func (recorder *pageRecorder) ListDutyVerifications(
	_ context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.DutyVerificationRecord], error) {
	return recorded[ports.DutyVerificationRecord](recorder, tenant, limit, query)
}

// serving 把一个端点构造函数接到记录替身上；替身实现了全部读口（上面的锁缝）。
func serving[Reader any](
	build func(customshttp.CatalogueQueryIntake, Reader) http.Handler,
) func(customshttp.CatalogueQueryIntake, *pageRecorder) http.Handler {
	return func(intake customshttp.CatalogueQueryIntake, recorder *pageRecorder) http.Handler {
		return build(intake, any(recorder).(Reader))
	}
}

// pagedCatalogue 是一册在端点上的样子。target 是路由连同选择器；sort 是一个集内、非缺省的排序，at 是按它排时合法的一个
// 末行位置，用来编真游标；filters 是一组合法筛选，窄口参数在其中；narrow 是本册按对象收窄的那一维（没有则空）；
// outOfVocabulary 是一个词表外的筛选值，没有封闭词表维的册为空。
type pagedCatalogue struct {
	name            string
	target          string
	catalogue       *cataloguepage.Catalogue
	endpoint        func(customshttp.CatalogueQueryIntake, *pageRecorder) http.Handler
	sort            string
	at              cataloguepage.Position
	filters         url.Values
	narrow          string
	outOfVocabulary url.Values
	list            string
}

const pageInstant = "2026-08-14T09:00:00Z"

func pagedCatalogues() []pagedCatalogue {
	direction := cataloguepage.Words[domain.ManifestDirection]()[0]
	layer := cataloguepage.Words[domain.ResultLayer]()[0]
	answer := cataloguepage.Words[domain.AdmissionAnswer]()[0]
	action := cataloguepage.Words[domain.GuardedAction]()[0]
	blocked, admitted := admissionAnswerWords()
	outside := func(dimension string) url.Values { return url.Values{dimension: {"SYN-NOT-A-WORD"}} }
	at := func(value string, identity ...string) cataloguepage.Position {
		return cataloguepage.Position{Value: value, Identity: identity}
	}
	return []pagedCatalogue{
		{
			name: "案件", target: "/customs-case-records", catalogue: ports.CustomsCasePages,
			endpoint: serving(customshttp.NewQueryCustomsCasesEndpoint),
			sort:     "-establishedAt", at: at(pageInstant, "SYN-CASE-01"),
			filters: url.Values{"case": {"SYN-CASE-02", "SYN-CASE-01"}, "direction": {direction}}, narrow: "case",
			outOfVocabulary: outside("direction"), list: "cases",
		},
		{
			name: "提交版本", target: "/customs-submission-records", catalogue: ports.SubmissionVersionPages,
			endpoint: serving(customshttp.NewQuerySubmissionVersionsEndpoint),
			sort:     "-fixedAt", at: at(pageInstant, "SYN-UNIT-01", "SYN-PROCEDURE-01", pageInstant, "SYN-SUBV-01"),
			filters: url.Values{"version": {"SYN-SUBV-01"}, "isCurrent": {"true"}}, narrow: "version",
			outOfVocabulary: outside("isCurrent"), list: "submissions",
		},
		{
			name: "外部结果", target: "/customs-external-result-records?version=SYN-SUBV-01", catalogue: ports.ExternalResultPages,
			endpoint: serving(customshttp.NewQueryExternalResultsEndpoint),
			sort:     "-recordedAt", at: at(pageInstant, "SYN-RESULT-01"),
			filters: url.Values{"layer": {layer}}, outOfVocabulary: outside("layer"), list: "results",
		},
		{
			name: "判断册", target: "/customs-action-admission-judgements", catalogue: ports.AdmissionJudgementPages,
			endpoint: serving(customshttp.NewQueryActionAdmissionJudgementsEndpoint),
			sort:     "judgedAt", at: at(pageInstant, "SYN-JUDGEMENT-01"),
			filters: url.Values{"object": {"SYN-PARCEL-01"}, "answer": {answer}}, narrow: "object",
			outOfVocabulary: outside("answer"), list: "judgements",
		},
		{
			name: "卡关对象", target: "/customs-unadmitted-objects", catalogue: ports.UnadmittedObjectPages,
			endpoint: serving(customshttp.NewQueryUnadmittedObjectsEndpoint),
			sort:     "-actionAt", at: at(pageInstant, "SYN-PARCEL-01"),
			// 过门的答复进不了这一册，也不在它的词表里。
			filters: url.Values{"answer": {blocked}}, outOfVocabulary: url.Values{"answer": {admitted}}, list: "judgements",
		},
		{
			name: "建案要求规则", target: "/customs-compliance-rules?registry=case-requirement",
			catalogue: ports.CaseRequirementRulePages, endpoint: serving(customshttp.NewQueryComplianceRulesEndpoint),
			sort: "-jurisdiction", at: at("SYN-JURISDICTION-01", "SYN-JURISDICTION-01", direction, "SYN-PROCEDURE-01"),
			filters: url.Values{"direction": {direction}}, outOfVocabulary: outside("direction"), list: "rules",
		},
		{
			name: "解释规则", target: "/customs-compliance-rules?registry=interpretation",
			catalogue: ports.InterpretationRulePages, endpoint: serving(customshttp.NewQueryComplianceRulesEndpoint),
			sort: "-appliesFrom", at: at(pageInstant, layer, "SYN-JURISDICTION-01", pageInstant),
			filters: url.Values{"layer": {layer}}, outOfVocabulary: outside("layer"), list: "rules",
		},
		{
			name: "就绪判断", target: "/customs-case-registers?registry=readiness",
			catalogue: ports.ReadinessJudgmentPages, endpoint: serving(customshttp.NewQueryCaseRegistersEndpoint),
			sort: "-judgedAt", at: at(pageInstant, "SYN-UNIT-01"), list: "judgments",
		},
		{
			name: "提交授权", target: "/customs-case-registers?registry=submission-authority",
			catalogue: ports.SubmissionAuthorityPages, endpoint: serving(customshttp.NewQueryCaseRegistersEndpoint),
			sort: "-grantedAt", at: at(pageInstant, "SYN-UNIT-01"), list: "authorities",
		},
		{
			name: "关闭义务目录", target: "/customs-case-registers?registry=closure-obligation",
			catalogue: ports.ClosureObligationPages, endpoint: serving(customshttp.NewQueryCaseRegistersEndpoint),
			sort: "-registeredAt", at: at(pageInstant, "SYN-CASE-01"), list: "catalogues",
		},
		{
			name: "门禁条件", target: "/customs-gate-conditions", catalogue: ports.GateConditionPages,
			endpoint: serving(customshttp.NewQueryGateConditionsEndpoint),
			sort:     "-registeredAt", at: at(pageInstant, "SYN-SCOPE-01", action, "SYN-BOUNDARY-01"),
			filters: url.Values{"action": {action}}, outOfVocabulary: outside("action"), list: "gates",
		},
		{
			name: "口岸", target: "/customs-ports-paths?registry=candidate-port",
			catalogue: ports.CandidatePortPages, endpoint: serving(customshttp.NewQueryPortsPathsEndpoint),
			sort: "-appliesFrom", at: at(pageInstant, "SYN-PORT-01", pageInstant), list: "ports",
		},
		{
			name: "路径", target: "/customs-ports-paths?registry=declaration-path",
			catalogue: ports.DeclarationPathPages, endpoint: serving(customshttp.NewQueryPortsPathsEndpoint),
			sort: "-appliesFrom", at: at(pageInstant, "SYN-PATH-01", pageInstant),
			filters: url.Values{"direction": {direction}}, outOfVocabulary: outside("direction"), list: "paths",
		},
		{
			name: "凭证", target: "/customs-credentials", catalogue: ports.CredentialPages,
			endpoint: serving(customshttp.NewQueryCredentialsEndpoint),
			sort:     "-registeredAt", at: at(pageInstant, "SYN-CREDENTIAL-01"), list: "credentials",
		},
		{
			name: "协作事项", target: "/customs-duty-collaborations", catalogue: ports.DutyCollaborationPages,
			endpoint: serving(customshttp.NewQueryDutyCollaborationsEndpoint),
			sort:     "-formedAt", at: at(pageInstant, "SYN-SCOPE-01", "SYN-DUTY-01"),
			filters:         url.Values{"kind": {cataloguepage.Words[domain.DutyObligationKind]()[0]}},
			outOfVocabulary: outside("kind"), list: "collaborations",
		},
		{
			name: "核对", target: "/customs-duty-verifications", catalogue: ports.DutyVerificationPages,
			endpoint: serving(customshttp.NewQueryDutyVerificationsEndpoint),
			sort:     "-verifiedAt", at: at(pageInstant, "SYN-DUTY-01", "SYN-FUNDS-01", "SYN-SCOPE-01", pageInstant, "SYN-DIGEST-01"),
			filters: url.Values{
				"coverage": {cataloguepage.Words[domain.DutyCoverage]()[0]},
				"delta":    {cataloguepage.Words[domain.DutyDelta]()[0]},
				"validity": {cataloguepage.Words[domain.DutyFactValidity]()[0]},
			},
			outOfVocabulary: outside("validity"), list: "verifications",
		},
	}
}

// admissionAnswerWords 从领域给的全集里各挑一格不过门的与过门的答复。
func admissionAnswerWords() (blocked, admitted string) {
	for _, answer := range domain.AdmissionAnswers() {
		if answer.Admits() && admitted == "" {
			admitted = answer.String()
		}
		if !answer.Admits() && blocked == "" {
			blocked = answer.String()
		}
	}
	return blocked, admitted
}

func merged(groups ...url.Values) url.Values {
	values := url.Values{}
	for _, group := range groups {
		for key, given := range group {
			values[key] = append(values[key], given...)
		}
	}
	return values
}

// values 是本册路由上的选择器加上 extra 那几格；同一组值既拿去请求，也拿去编游标，选择器因此进同一份摘要。
func (paged pagedCatalogue) values(t *testing.T, extra url.Values) url.Values {
	t.Helper()
	parsed, err := url.Parse(paged.target)
	if err != nil {
		t.Fatal(err)
	}
	return merged(parsed.Query(), extra)
}

func (paged pagedCatalogue) serve(
	t *testing.T, intake customshttp.CatalogueQueryIntake, recorder *pageRecorder, values url.Values,
) *httptest.ResponseRecorder {
	t.Helper()
	path, _, _ := strings.Cut(paged.target, "?")
	response := httptest.NewRecorder()
	paged.endpoint(intake, recorder).ServeHTTP(response, httptest.NewRequest(http.MethodGet, path+"?"+values.Encode(), nil))
	return response
}

// cursor 在 values 那组条件下编一枚指向 at 的真游标，与读面经 cataloguepage.Finish 编出的同一种。
func (paged pagedCatalogue) cursor(t *testing.T, values url.Values, at cataloguepage.Position) string {
	t.Helper()
	query, err := paged.catalogue.Decode(values)
	if err != nil {
		t.Fatalf("解出编游标用的查询：%v", err)
	}
	cursor, err := query.CursorAfter(at)
	if err != nil {
		t.Fatalf("编游标：%v", err)
	}
	return cursor
}

func pagedCatalogueNamed(t *testing.T, name string) pagedCatalogue {
	t.Helper()
	for _, paged := range pagedCatalogues() {
		if paged.name == name {
			return paged
		}
	}
	t.Fatalf("没有叫 %s 的册", name)
	return pagedCatalogue{}
}

var grantedPageIntake = grantedCatalogueIntake{tenant: "SYN-TENANT-01", limit: 25}

// Covers: ADR-0144 决定一、四、五、六——各册各自：查询参数按本册声明解成查询对象原样交给读口（排序、维内去重升序的筛选、
// q、游标位置），租户与页大小只取自接入面；答复带 page（size 取接入面给的页大小，next 与 total 原样带出），列表那一格是数组。
// 窄口参数（案件的 case、提交版本的 version、判断册的 object）是筛选维，带值照答。
func TestCustomsCataloguesHandTheDecodedQueryToTheReader(t *testing.T) {
	for _, paged := range pagedCatalogues() {
		t.Run(paged.name, func(t *testing.T) {
			conditions := paged.values(t, merged(url.Values{"sort": {paged.sort}, "q": {"SYN"}}, paged.filters))
			recorder := &pageRecorder{next: "SYN-NEXT", total: 7}
			response := paged.serve(t, grantedPageIntake, recorder,
				merged(conditions, url.Values{"after": {paged.cursor(t, conditions, paged.at)}}))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d；body = %s", response.Code, response.Body)
			}
			if recorder.tenant != "SYN-TENANT-01" || recorder.limit != 25 {
				t.Fatalf("租户或页大小不是接入面给的：%q %d", recorder.tenant, recorder.limit)
			}
			field, descending := strings.CutPrefix(paged.sort, "-")
			if recorder.query.Sort != (cataloguepage.Sort{Field: field, Descending: descending}) || recorder.query.Keyword != "SYN" {
				t.Fatalf("排序或 q 不对：%+v", recorder.query)
			}
			want := map[string][]string{}
			for dimension, given := range paged.filters {
				canonical := append([]string(nil), given...)
				sort.Strings(canonical)
				want[dimension] = canonical
			}
			if !reflect.DeepEqual(recorder.query.Filters, want) {
				t.Fatalf("筛选得 %v，应为 %v", recorder.query.Filters, want)
			}
			if recorder.query.After == nil || !reflect.DeepEqual(*recorder.query.After, paged.at) {
				t.Fatalf("游标位置没有原样交给读口：%+v", recorder.query.After)
			}

			page := pageOf(t, response)
			if string(page["size"]) != "25" || string(page["next"]) != `"SYN-NEXT"` || string(page["total"]) != "7" {
				t.Fatalf("page 不对：%s", response.Body.Bytes())
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if string(body[paged.list]) != "[]" {
				t.Fatalf("列表那一格 %s 应是空数组：%s", paged.list, response.Body.Bytes())
			}
		})
	}
}

// Covers: ADR-0144 决定一、三、四——各册各自：请求自报的 tenant、limit 是集外键（租户与页大小只从接入面来），集外排序维、
// 词表外筛选值、解不开的游标、换了排序或 q 的旧游标，一律答 400 带理由散文，读口不被调用。
func TestCustomsCataloguesRefuseQueriesOutsideTheirDeclaration(t *testing.T) {
	for _, paged := range pagedCatalogues() {
		t.Run(paged.name, func(t *testing.T) {
			sorted := paged.values(t, url.Values{"sort": {paged.sort}})
			refusals := map[string]url.Values{
				"自报租户":     paged.values(t, url.Values{"tenant": {"SYN-TENANT-02"}}),
				"自报页大小":    paged.values(t, url.Values{"limit": {"9999"}}),
				"集外排序维":    paged.values(t, url.Values{"sort": {"SYN-NOT-A-FIELD"}}),
				"解不开的游标":   paged.values(t, url.Values{"after": {"SYN-not-a-cursor"}}),
				"换了排序的旧游标": paged.values(t, url.Values{"after": {paged.cursor(t, sorted, paged.at)}}),
				"换了 q 的旧游标": paged.values(t, url.Values{
					"sort": {paged.sort}, "q": {"SYN-B"},
					"after": {paged.cursor(t, paged.values(t, url.Values{"sort": {paged.sort}, "q": {"SYN-A"}}), paged.at)},
				}),
			}
			if paged.outOfVocabulary != nil {
				refusals["词表外筛选值"] = paged.values(t, paged.outOfVocabulary)
			}
			for name, values := range refusals {
				t.Run(name, func(t *testing.T) {
					recorder := &pageRecorder{}
					assertPageQueryRefused(t, paged.serve(t, grantedPageIntake, recorder, values))
					if recorder.called {
						t.Fatal("坏请求不该走到读口")
					}
				})
			}
		})
	}
}

// Covers: ADR-0055 决定三——端点先 Intake、后解码：渠道未配置时各册对一切请求同答 403，参数写对写错（集外排序维、集外键、
// 解不开的游标、词表外筛选值，按 registry 分派的路由上缺 registry 或给集外取值）答复逐字相同，没有凭据的调用方探不出
// 有哪几册、各册开放了哪些查询维度；读口不被调用。
func TestUnconfiguredChannelsAnswerEveryCatalogueQueryIdentically(t *testing.T) {
	for _, paged := range pagedCatalogues() {
		t.Run(paged.name, func(t *testing.T) {
			queries := map[string]url.Values{
				"写对":     paged.values(t, merged(url.Values{"sort": {paged.sort}}, paged.filters)),
				"集外排序维":  paged.values(t, url.Values{"sort": {"SYN-NOT-A-FIELD"}}),
				"集外键":    paged.values(t, url.Values{"SYN-NOT-A-KEY": {"SYN-VALUE"}}),
				"解不开的游标": paged.values(t, url.Values{"after": {"SYN-not-a-cursor"}}),
			}
			if paged.outOfVocabulary != nil {
				queries["词表外筛选值"] = paged.values(t, paged.outOfVocabulary)
			}
			if paged.values(t, nil).Has("registry") {
				queries["缺 registry"] = url.Values{}
				queries["集外 registry"] = url.Values{"registry": {"SYN-NOT-A-REGISTRY"}}
			}
			answers := map[string]string{}
			for name, values := range queries {
				recorder := &pageRecorder{}
				response := paged.serve(t, customshttp.UnconfiguredIntake{}, recorder, values)
				if response.Code != http.StatusForbidden || recorder.called {
					t.Fatalf("%s：status = %d（want 403），读口被调用 = %v", name, response.Code, recorder.called)
				}
				answers[response.Body.String()] = name
			}
			if len(answers) != 1 {
				t.Fatalf("未配置的答复随请求内容变了：%v", answers)
			}
		})
	}
}

// Covers: 迁移前就在用的窄口参数（案件的 case、提交版本的 version、判断册的 object）空值即不收窄、落到整册，迁移后照旧——
// 空值在解码前去掉，不答「筛选维的取值为空」；与有值的同给时只按有值的那几个收窄。
func TestBlankNarrowingStillListsTheWholeCatalogue(t *testing.T) {
	for _, paged := range pagedCatalogues() {
		if paged.narrow == "" {
			continue
		}
		t.Run(paged.name, func(t *testing.T) {
			for name, test := range map[string]struct {
				given []string
				want  []string
			}{
				"只给空值":    {given: []string{""}, want: nil},
				"空值与有值同给": {given: []string{"", "SYN-KEY-01"}, want: []string{"SYN-KEY-01"}},
			} {
				t.Run(name, func(t *testing.T) {
					recorder := &pageRecorder{}
					response := paged.serve(t, grantedPageIntake, recorder, paged.values(t, url.Values{paged.narrow: test.given}))
					if response.Code != http.StatusOK || !recorder.called {
						t.Fatalf("status = %d；body = %s", response.Code, response.Body)
					}
					if got := recorder.query.Filters[paged.narrow]; !reflect.DeepEqual(got, test.want) {
						t.Fatalf("%s 收窄得 %v，应为 %v", paged.narrow, got, test.want)
					}
				})
			}
		})
	}
}

// Covers: ADR-0144 决定一——按 registry 分派的路由每种各是一册：一册的游标换个 registry 取值去翻答 400，回到原册照样接着翻。
// 就绪判断与提交授权的排序维与标识同形，拒绝只可能出自选择器进了摘要。
func TestARegistryCursorDoesNotCarryOverToAnotherRegistry(t *testing.T) {
	for _, pair := range [][2]string{{"建案要求规则", "解释规则"}, {"就绪判断", "提交授权"}, {"口岸", "路径"}} {
		from, to := pagedCatalogueNamed(t, pair[0]), pagedCatalogueNamed(t, pair[1])
		t.Run(from.name+"→"+to.name, func(t *testing.T) {
			// 缺省序在这几册都是文本维，末行位置的标识照本册声明的段数与形状给。
			cursor := from.cursor(t, from.values(t, nil), cataloguepage.Position{Value: "SYN-KEY-01", Identity: from.at.Identity})
			after := url.Values{"after": {cursor}}

			assertPageQueryRefused(t, to.serve(t, grantedPageIntake, &pageRecorder{}, to.values(t, after)))
			recorder := &pageRecorder{}
			if resumed := from.serve(t, grantedPageIntake, recorder, from.values(t, after)); resumed.Code != http.StatusOK ||
				recorder.query.After == nil {
				t.Fatalf("回到原册应接着翻：status = %d after = %+v", resumed.Code, recorder.query.After)
			}
		})
	}
}

// Covers: 外部结果的 version 是选择器——原样交给读口、空即不收窄（迁移前的原义），不进筛选；不带它也照答。
func TestTheExternalResultVersionStaysASelector(t *testing.T) {
	paged := pagedCatalogueNamed(t, "外部结果")
	for name, test := range map[string]struct {
		values url.Values
		want   string
	}{
		"带版本":  {values: url.Values{"version": {"SYN-SUBV-01"}}, want: "SYN-SUBV-01"},
		"版本为空": {values: url.Values{"version": {""}}, want: ""},
		"不带版本": {values: url.Values{}, want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := &pageRecorder{}
			response := httptest.NewRecorder()
			paged.endpoint(grantedPageIntake, recorder).ServeHTTP(response,
				httptest.NewRequest(http.MethodGet, "/customs-external-result-records?"+test.values.Encode(), nil))
			if response.Code != http.StatusOK || recorder.version != test.want || len(recorder.query.Filters) != 0 {
				t.Fatalf("status = %d version = %q filters = %v", response.Code, recorder.version, recorder.query.Filters)
			}
		})
	}
}

// Covers: 带计数与 truncated 的待办队列保留这两格——卡关对象的 unadmittedCount 是整个队列的积压数，不随筛选与 q 变；page.total 是这一组条件下的
// 件数；truncated 即 page.next 不为 null。
func TestTheUnadmittedCountIsTheWholeBacklogWhilePageTotalFollowsTheConditions(t *testing.T) {
	blocked, _ := admissionAnswerWords()
	for name, test := range map[string]struct {
		next      string
		nextJSON  string
		truncated bool
	}{
		"还有下一页": {next: "SYN-NEXT", nextJSON: `"SYN-NEXT"`, truncated: true},
		"已到末页":  {next: "", nextJSON: "null", truncated: false},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := &pageRecorder{next: test.next, total: 2, count: 9}
			response := httptest.NewRecorder()
			customshttp.NewQueryUnadmittedObjectsEndpoint(grantedPageIntake, recorder).ServeHTTP(response,
				httptest.NewRequest(http.MethodGet, "/customs-unadmitted-objects?q=SYN&answer="+blocked, nil))
			var body struct {
				UnadmittedCount int  `json:"unadmittedCount"`
				Truncated       bool `json:"truncated"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			page := pageOf(t, response)
			if response.Code != http.StatusOK || body.UnadmittedCount != 9 || string(page["total"]) != "2" ||
				body.Truncated != test.truncated || string(page["next"]) != test.nextJSON {
				t.Fatalf("积压件数、total 或截断走样：%s", response.Body.Bytes())
			}
		})
	}
}
