package settlementhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证各列表端点上的翻页契约（ADR-0144；票 catalogue-read-pagination-rollout/08）：答复带 page、解码失败答 400 带理由、
// 渠道未配置时参数写对写错都答 403。逐格转写仍在各端点自己的用例里。

// assertPageQueryRefused 断言答复是 400 MALFORMED_REQUEST 且带理由散文，交回那段散文。
func assertPageQueryRefused(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400；body = %s", response.Code, response.Body)
	}
	var body struct {
		Error struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem %s: %v", response.Body.Bytes(), err)
	}
	if body.Error.Code != "MALFORMED_REQUEST" || body.Error.Detail == "" {
		t.Fatalf("应答 MALFORMED_REQUEST 且带理由散文，得 %s", response.Body.Bytes())
	}
	return body.Error.Detail
}

// pageOf 读出答复里的 page 一格；size、next、total 都必须在场。
func pageOf(t *testing.T, response *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var body struct {
		Page map[string]json.RawMessage `json:"page"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	for _, key := range []string{"size", "next", "total"} {
		if _, present := body.Page[key]; !present {
			t.Fatalf("page 缺 %s：%s", key, response.Body.Bytes())
		}
	}
	return body.Page
}

const (
	pagingNext  = "SYN-CURSOR-NEXT"
	pagingTotal = 41
)

// pagingCatalogues 是各册共用的读口替身：不交行，只交游标与总数，并记下端点交给它的页大小与查询。各册一个类型，理由同
// unreachableCatalogues。
type pagingCatalogues struct {
	gotLimit int
	gotQuery *cataloguepage.Query
}

func (stub *pagingCatalogues) record(limit int, query cataloguepage.Query) {
	stub.gotLimit, stub.gotQuery = limit, &query
}

func pagingResult[Row any]() cataloguepage.Result[Row] {
	return cataloguepage.Result[Row]{Rows: []Row{}, Next: pagingNext, Total: pagingTotal}
}

func (stub *pagingCatalogues) ListCustomerCharges(
	_ context.Context, _ domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerChargeCatalogueRow], error) {
	stub.record(limit, query)
	return pagingResult[ports.CustomerChargeCatalogueRow](), nil
}

func (stub *pagingCatalogues) ListSupplierExpectedCosts(
	_ context.Context, _ domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierExpectedCostCatalogueRow], error) {
	stub.record(limit, query)
	return pagingResult[ports.SupplierExpectedCostCatalogueRow](), nil
}

func (stub *pagingCatalogues) ListCustomerStatements(
	_ context.Context, _ domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerStatementCatalogueRow], error) {
	stub.record(limit, query)
	return pagingResult[ports.CustomerStatementCatalogueRow](), nil
}

func (stub *pagingCatalogues) ListSupplierBillReceptions(
	_ context.Context, _ domain.TenantID, _ ports.SupplierBillReceptionFilter, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierBillReceptionCatalogueRow], error) {
	stub.record(limit, query)
	return pagingResult[ports.SupplierBillReceptionCatalogueRow](), nil
}

func (stub *pagingCatalogues) ListDisputedCustomerStatements(
	_ context.Context, _ domain.TenantID, limit int, query cataloguepage.Query,
) (ports.DisputedCustomerStatements, error) {
	stub.record(limit, query)
	return ports.DisputedCustomerStatements{Count: pagingTotal, Next: pagingNext, Total: pagingTotal}, nil
}

func (stub *pagingCatalogues) ListAuditedPayables(
	_ context.Context, _ domain.TenantID, _ ports.AuditedPayableFilter, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.AuditedPayableCatalogueRow], error) {
	stub.record(limit, query)
	return pagingResult[ports.AuditedPayableCatalogueRow](), nil
}

func (stub *pagingCatalogues) ListExternalFundsFacts(
	_ context.Context, _ domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.ExternalFundsFactCatalogueRow], error) {
	stub.record(limit, query)
	return pagingResult[ports.ExternalFundsFactCatalogueRow](), nil
}

func (stub *pagingCatalogues) ListOperatingResults(
	_ context.Context, _ domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.OperatingResultCatalogueRow], error) {
	stub.record(limit, query)
	return pagingResult[ports.OperatingResultCatalogueRow](), nil
}

func (stub *pagingCatalogues) ListCostAllocations(
	_ context.Context, _ domain.TenantID, limit int, query cataloguepage.Query,
) (cataloguepage.Result[ports.CostAllocationCatalogueRow], error) {
	stub.record(limit, query)
	return pagingResult[ports.CostAllocationCatalogueRow](), nil
}

// settlementCatalogueReaders 是各册端点读口的并集：unreachableCatalogues 与 pagingCatalogues 都实现它，同一张册表接哪只替身
// 由用例定。
type settlementCatalogueReaders interface {
	settlementhttp.ChargeCatalogueReader
	settlementhttp.StatementCatalogueReader
	settlementhttp.DisputedStatementReader
	settlementhttp.AuditedPayableCatalogueReader
	settlementhttp.FundsApplicationCatalogueReader
	settlementhttp.OperatingCatalogueReader
}

// pagedCatalogue 是一本按 ADR-0144 翻页的册子：端点、取到它的目标（不带翻页参数）、一个合法的非缺省排序，有筛选维时再带一维
// 与它词表里的一个词；铸游标用的声明与缺省序下一个合乎声明的末行位置。
type pagedCatalogue struct {
	name    string
	target  string
	sort    string
	filter  string
	word    string
	pages   *cataloguepage.Catalogue
	last    cataloguepage.Position
	handler func(settlementhttp.CatalogueQueryIntake, settlementCatalogueReaders) http.Handler
}

// pagedCatalogues 交回本上下文按 ADR-0144 翻页的全部册子。
func pagedCatalogues() []pagedCatalogue {
	charges := func(intake settlementhttp.CatalogueQueryIntake, reader settlementCatalogueReaders) http.Handler {
		return settlementhttp.NewQuerySettlementChargesEndpoint(intake, reader)
	}
	statements := func(intake settlementhttp.CatalogueQueryIntake, reader settlementCatalogueReaders) http.Handler {
		return settlementhttp.NewQuerySettlementStatementsEndpoint(intake, reader)
	}
	disputed := func(intake settlementhttp.CatalogueQueryIntake, reader settlementCatalogueReaders) http.Handler {
		return settlementhttp.NewQueryDisputedCustomerStatementsEndpoint(intake, reader)
	}
	payables := func(intake settlementhttp.CatalogueQueryIntake, reader settlementCatalogueReaders) http.Handler {
		return settlementhttp.NewQueryAuditedPayablesEndpoint(intake, reader)
	}
	funds := func(intake settlementhttp.CatalogueQueryIntake, reader settlementCatalogueReaders) http.Handler {
		return settlementhttp.NewQuerySettlementFundsApplicationsEndpoint(intake, reader)
	}
	operating := func(intake settlementhttp.CatalogueQueryIntake, reader settlementCatalogueReaders) http.Handler {
		return settlementhttp.NewQuerySettlementOperatingResultsEndpoint(intake, reader)
	}
	statement := cataloguepage.Position{Value: "SYN-STMT-01", Identity: []string{"SYN-STMT-01"}}
	return []pagedCatalogue{
		{name: "customer-charges", target: "/settlement-charges?registry=customer-charge", sort: "-formedAt",
			filter: "stage", word: "CONFIRMED", pages: ports.CustomerChargePages,
			last: cataloguepage.Position{Value: "SYN-CHG-01", Identity: []string{"SYN-CHG-01"}}, handler: charges},
		{name: "supplier-expected-costs", target: "/settlement-charges?registry=supplier-expected-cost", sort: "-recordedAt",
			pages: ports.SupplierExpectedCostPages,
			last:  cataloguepage.Position{Value: "SYN-OCC-01", Identity: []string{"SYN-FEE-01", "SYN-COST-01"}}, handler: charges},
		{name: "customer-statements", target: "/settlement-statements?registry=customer-statement", sort: "-publishedAt",
			pages: ports.CustomerStatementPages, last: statement, handler: statements},
		{name: "supplier-bill-receptions", target: "/settlement-statements?registry=supplier-bill-reception", sort: "period",
			pages: ports.SupplierBillReceptionPages,
			last:  cataloguepage.Position{Value: "2026-08-28T09:00:00Z", Identity: []string{"SYN-SUP-01/SYN-INV-01", "1"}}, handler: statements},
		{name: "disputed-customer-statements", target: "/settlement-disputed-statements?", sort: "-publishedAt",
			pages: ports.DisputedCustomerStatementPages, last: statement, handler: disputed},
		{name: "audited-payables", target: "/settlement-audited-payables?", sort: "auditedAt",
			pages: ports.AuditedPayablePages,
			last:  cataloguepage.Position{Value: "2026-08-28T09:00:00Z", Identity: []string{"SYN-PAYABLE-01"}}, handler: payables},
		{name: "external-funds-facts", target: "/settlement-funds-applications?", sort: "-occurredAt",
			filter: "kind", word: "PAYMENT_FAILED", pages: ports.ExternalFundsFactPages,
			last: cataloguepage.Position{Value: "SYN-FACT-01", Identity: []string{"SYN-FACT-01", "SYN-V1"}}, handler: funds},
		{name: "operating-results", target: "/settlement-operating-results?registry=operating-result", sort: "-recordedAt",
			filter: "basis", word: "SETTLED", pages: ports.OperatingResultPages,
			last: cataloguepage.Position{Value: "SYN-SCOPE-01", Identity: []string{"SYN-SCOPE-01", "SYN-PER-01", "CONFIRMED"}}, handler: operating},
		{name: "cost-allocations", target: "/settlement-operating-results?registry=cost-allocation", sort: "-recordedAt",
			pages: ports.CostAllocationPages,
			last:  cataloguepage.Position{Value: "SYN-ALC-01", Identity: []string{"SYN-ALC-01"}}, handler: operating},
	}
}

// withParameter 在目标后接一段查询串；目标已以「?」收尾时不再加「&」。
func withParameter(target, parameter string) string {
	if parameter == "" || strings.HasSuffix(target, "?") {
		return target + parameter
	}
	return target + "&" + parameter
}

// Covers: ADR-0144 决定五 — 各册的答复都带 page 的 size、next、total：size 是 Intake 给的页大小（不是本页行数），next 与 total 是读口答的；读口
// 收到的页大小就是 Intake 的，查询按声明解出（排序与筛选原样到读口）。
func TestEveryPagedSettlementCatalogueAnswersThePageTriple(t *testing.T) {
	for _, catalogue := range pagedCatalogues() {
		t.Run(catalogue.name, func(t *testing.T) {
			reader := &pagingCatalogues{}
			parameters := "sort=" + catalogue.sort
			if catalogue.filter != "" {
				parameters += "&" + catalogue.filter + "=" + catalogue.word
			}
			response := serveGet(catalogue.handler(grantedIntake(), reader), withParameter(catalogue.target, parameters))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			page := pageOf(t, response)
			if string(page["size"]) != "25" || string(page["next"]) != `"`+pagingNext+`"` || string(page["total"]) != "41" {
				t.Fatalf("page 走样：%s", response.Body.String())
			}
			if reader.gotLimit != 25 || reader.gotQuery == nil {
				t.Fatalf("读口收到的页大小 = %d（要 Intake 给的 25）", reader.gotLimit)
			}
			if got := reader.gotQuery.Sort.String(); got != catalogue.sort {
				t.Fatalf("读口收到的排序 = %q，要 %q", got, catalogue.sort)
			}
			if catalogue.filter != "" {
				if got := reader.gotQuery.Filters[catalogue.filter]; len(got) != 1 || got[0] != catalogue.word {
					t.Fatalf("读口收到的筛选 = %v", reader.gotQuery.Filters)
				}
			}
		})
	}
}

// malformedPageQueries 是一本册子上越出声明的各种问法：集外键、自报租户与页大小、集外排序维、给了两次的单值键、坏游标、超长 q，
// 有筛选维时再加词表外与为空的筛选值。
func malformedPageQueries(catalogue pagedCatalogue) map[string]string {
	malformed := map[string]string{
		"unknown key":        "unknown=1",
		"self-reported":      "tenant=SYN-TENANT-9&limit=9999",
		"unknown sort":       "sort=bogus",
		"unknown descending": "sort=-bogus",
		"sort twice":         "sort=" + catalogue.sort + "&sort=" + catalogue.sort,
		"bad cursor":         "after=not-a-cursor",
		"keyword too long":   "q=" + url.QueryEscape(strings.Repeat("字", cataloguepage.MaxKeywordRunes+1)),
	}
	if catalogue.filter != "" {
		malformed["word outside vocabulary"] = catalogue.filter + "=NOT_A_WORD"
		malformed["empty filter value"] = catalogue.filter + "="
	}
	return malformed
}

// Covers: ADR-0144 决定四 — malformedPageQueries 里的每一种问法都答 400 MALFORMED_REQUEST 带理由散文，读口不被触到，不带业务
// 结果格。
func TestPagedSettlementCataloguesRefuseMalformedPageQueries(t *testing.T) {
	for _, catalogue := range pagedCatalogues() {
		for label, parameters := range malformedPageQueries(catalogue) {
			t.Run(catalogue.name+"/"+label, func(t *testing.T) {
				response := serveGet(catalogue.handler(grantedIntake(), unreachableCatalogues{t: t}), withParameter(catalogue.target, parameters))
				assertPageQueryRefused(t, response)
				assertNoOutcome(t, response)
			})
		}
	}
}

// Covers: spec「端点先 Intake、后解码查询参数」、ADR-0055 决定三 — 渠道未配置时各册对一切请求同答 403：不带翻页参数、合规的一问
// 与 malformedPageQueries 里的每一种问法，答复逐字相同、读口不被触到，未配置的调用方从答复里看不出自己的参数对不对。
func TestUnconfiguredChannelRefusesEveryPagedQueryWhateverItsParameters(t *testing.T) {
	for _, catalogue := range pagedCatalogues() {
		t.Run(catalogue.name, func(t *testing.T) {
			endpoint := catalogue.handler(settlementhttp.UnconfiguredIntake{}, unreachableCatalogues{t: t})
			bare := serveGet(endpoint, catalogue.target)
			if bare.Code != http.StatusForbidden || problemCode(t, bare) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("status = %d body = %s", bare.Code, bare.Body.String())
			}
			assertNoOutcome(t, bare)

			queries := malformedPageQueries(catalogue)
			queries["declared sort"] = "sort=" + catalogue.sort
			for label, parameters := range queries {
				response := serveGet(endpoint, withParameter(catalogue.target, parameters))
				if response.Code != http.StatusForbidden || response.Body.String() != bare.Body.String() {
					t.Fatalf("%s：未配置的渠道答复随查询串变了：%d %s，不带翻页参数答 %s", label, response.Code, response.Body, bare.Body)
				}
			}
		})
	}
}

// Covers: ADR-0144 决定一与越权风险点 6 — 各册的游标只在铸出它的那一组条件下有效：同条件接得上，读口收到的就是游标里的位置；
// 换了 q、换了排序再拿旧游标都答 400，理由是决定一点名的那句，读口不被触到。
func TestEveryPagedSettlementCatalogueRefusesItsCursorUnderOtherConditions(t *testing.T) {
	for _, catalogue := range pagedCatalogues() {
		t.Run(catalogue.name, func(t *testing.T) {
			target, err := url.Parse(catalogue.target)
			if err != nil {
				t.Fatal(err)
			}
			minted := target.Query()
			minted.Set("q", "SYN")
			query, err := catalogue.pages.Decode(minted)
			if err != nil {
				t.Fatal(err)
			}
			cursor, err := query.CursorAfter(catalogue.last)
			if err != nil {
				t.Fatal(err)
			}
			after := "&after=" + url.QueryEscape(cursor)

			reader := &pagingCatalogues{}
			continued := serveGet(catalogue.handler(grantedIntake(), reader), withParameter(catalogue.target, "q=SYN"+after))
			if continued.Code != http.StatusOK || reader.gotQuery == nil || reader.gotQuery.After == nil ||
				!reflect.DeepEqual(*reader.gotQuery.After, catalogue.last) {
				t.Fatalf("同条件的游标该接得上：%d %s", continued.Code, continued.Body.String())
			}
			for label, parameters := range map[string]string{
				"changed keyword": "q=OTHER" + after,
				"changed sort":    "q=SYN&sort=" + catalogue.sort + after,
			} {
				t.Run(label, func(t *testing.T) {
					response := serveGet(catalogue.handler(grantedIntake(), unreachableCatalogues{t: t}),
						withParameter(catalogue.target, parameters))
					if detail := assertPageQueryRefused(t, response); detail != "游标与本次的排序或筛选不符，请从第一页重取" {
						t.Fatalf("理由散文：得 %q", detail)
					}
					assertNoOutcome(t, response)
				})
			}
		})
	}
}

// Covers: 游标只在铸出它的那一组条件下有效 — 同册同条件接得上；换了 q、换了检索选择器的取值、或拿到另一册（对账单册与争议
// 清单行形相同、册名不同）上都答 400。
func TestASettlementCursorOnlyContinuesTheListItWasMintedFor(t *testing.T) {
	mint := func(t *testing.T, catalogue *cataloguepage.Catalogue, values url.Values, last cataloguepage.Position) string {
		t.Helper()
		query, err := catalogue.Decode(values)
		if err != nil {
			t.Fatal(err)
		}
		cursor, err := query.CursorAfter(last)
		if err != nil {
			t.Fatal(err)
		}
		return url.QueryEscape(cursor)
	}
	statement := cataloguepage.Position{Value: "SYN-STMT-01", Identity: []string{"SYN-STMT-01"}}
	statementCursor := mint(t, ports.CustomerStatementPages, url.Values{"registry": {"customer-statement"}, "q": {"SYN"}}, statement)

	reader := &pagingCatalogues{}
	continued := serveGet(settlementhttp.NewQuerySettlementStatementsEndpoint(grantedIntake(), reader),
		"/settlement-statements?registry=customer-statement&q=SYN&after="+statementCursor)
	if continued.Code != http.StatusOK || reader.gotQuery == nil || reader.gotQuery.After == nil ||
		reader.gotQuery.After.Value != "SYN-STMT-01" {
		t.Fatalf("同条件的游标该接得上：%d %s", continued.Code, continued.Body.String())
	}

	unreachable := unreachableCatalogues{t: t}
	assertPageQueryRefused(t, serveGet(settlementhttp.NewQuerySettlementStatementsEndpoint(grantedIntake(), unreachable),
		"/settlement-statements?registry=customer-statement&q=OTHER&after="+statementCursor))
	assertPageQueryRefused(t, serveGet(settlementhttp.NewQueryDisputedCustomerStatementsEndpoint(grantedIntake(), unreachable),
		"/settlement-disputed-statements?q=SYN&after="+statementCursor))

	reception := cataloguepage.Position{Value: "2026-08-28T09:00:00Z", Identity: []string{"SYN-SUP-01/INV-1", "1"}}
	receptionCursor := mint(t, ports.SupplierBillReceptionPages,
		url.Values{"registry": {"supplier-bill-reception"}, "supplier": {"SYN-SUP-01"}}, reception)
	assertPageQueryRefused(t, serveGet(settlementhttp.NewQuerySettlementStatementsEndpoint(grantedIntake(), unreachable),
		"/settlement-statements?registry=supplier-bill-reception&supplier=SYN-SUP-02&after="+receptionCursor))
}
