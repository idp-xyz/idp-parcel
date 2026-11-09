package postgres_test

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 证本上下文各列表读口迁到 ADR-0144 之后的取页（票 catalogue-read-pagination-rollout/08）：每册按小页走完全部页，
// 拼起来恰是全集且次序即所点的序；翻页期间新登的行不让后面的页重出或漏行；筛选维与 q 改变 total，q 的 % 按字面；待对账队列的
// 张数不随 q 变。键集机制本身由共用件证过，这里证的是各册落列：落错一列，次序就乱、页与页之间就重出或漏行。空册答空页、total
// 为 0 由 TestEmptySettlementRegistersAnswerEmptyLists 证。

// catalogueWalk 用同一组条件走完全部页（页大小由 list 定），交回每页的标识与最后一页的 total。
func catalogueWalk[Row any](
	t *testing.T,
	catalogue *cataloguepage.Catalogue,
	values url.Values,
	list func(cataloguepage.Query) (cataloguepage.Result[Row], error),
	id func(Row) string,
) ([][]string, int64) {
	t.Helper()
	var pages [][]string
	query := pageQuery(t, catalogue, values)
	for {
		page, err := list(query)
		if err != nil {
			t.Fatalf("取页 %v：%v", values, err)
		}
		pages = append(pages, catalogueIDs(page.Rows, id))
		if page.Next == "" {
			return pages, page.Total
		}
		if len(pages) > 20 {
			t.Fatal("翻了二十页还没到末页：游标没有前进")
		}
		query = nextPage(t, catalogue, values, page.Next)
	}
}

func catalogueIDs[Row any](rows []Row, id func(Row) string) []string {
	ids := make([]string, len(rows))
	for index, row := range rows {
		ids[index] = id(row)
	}
	return ids
}

// catalogueWalkCase 是一组条件与它该走出的页。
type catalogueWalkCase struct {
	values url.Values
	want   [][]string
	total  int64
}

func catalogueWalkCases[Row any](
	t *testing.T,
	catalogue *cataloguepage.Catalogue,
	cases map[string]catalogueWalkCase,
	list func(cataloguepage.Query) (cataloguepage.Result[Row], error),
	id func(Row) string,
) {
	t.Helper()
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if got, total := catalogueWalk(t, catalogue, test.values, list, id); !reflect.DeepEqual(got, test.want) || total != test.total {
				t.Fatalf("得 %v / total %d，应为 %v / %d", got, total, test.want, test.total)
			}
		})
	}
}

// Covers: ADR-0144 决定一、三——客户费用册按费用标识正序翻页。翻完第一页后登进两笔：一笔排在已翻过的那头、一笔排在未翻的那头。
// 第二页不重出第一页的行（偏移分页会在第二页开头重出 SYN-CHG-3），排在后头的那笔在末页出现，total 跟着变。formedAt 落在形成
// 时刻列上，倒序时同刻按费用标识同向倒序，同刻的两笔分在两页也接得上。
func TestCustomerChargesPageWithoutRepeatsOrGapsWhileChargesArrive(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	charge := func(id string, formedAfter time.Duration) {
		fixture.seedCustomerCharge(t, catalogueTenant, id, "SYN-FEE-01", "ESTIMATED", "SYN-CUR-01", 100, "SYN-CUR-01", 100, "", "")
		fixture.seed(t, `UPDATE settlement_accounting.customer_charge SET formed_at = $3 WHERE tenant_id = $1 AND charge_id = $2`,
			catalogueTenant, id, catalogueBaseAt.Add(formedAfter))
	}
	charge("SYN-CHG-2", time.Hour)
	charge("SYN-CHG-3", 3*time.Hour)
	charge("SYN-CHG-4", 2*time.Hour)
	charge("SYN-CHG-5", 3*time.Hour)
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.CustomerChargeCatalogueRow], error) {
		return fixture.charges.ListCustomerCharges(t.Context(), tenant, 2, query)
	}
	id := func(row ports.CustomerChargeCatalogueRow) string { return row.Charge }

	first, err := list(firstPage(t, ports.CustomerChargePages))
	if got := catalogueIDs(first.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-CHG-2", "SYN-CHG-3"}) || first.Total != 4 {
		t.Fatalf("第一页：得 %v / total %d（%v）", got, first.Total, err)
	}
	charge("SYN-CHG-1", 0)
	charge("SYN-CHG-6", 4*time.Hour)
	second, err := list(nextPage(t, ports.CustomerChargePages, url.Values{}, first.Next))
	if got := catalogueIDs(second.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-CHG-4", "SYN-CHG-5"}) || second.Total != 6 {
		t.Fatalf("第二页：得 %v / total %d（%v）", got, second.Total, err)
	}
	last, err := list(nextPage(t, ports.CustomerChargePages, url.Values{}, second.Next))
	if got := catalogueIDs(last.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-CHG-6"}) || last.Next != "" {
		t.Fatalf("末页：得 %v / next %q（%v）", got, last.Next, err)
	}

	byFormed, total := catalogueWalk(t, ports.CustomerChargePages, url.Values{"sort": {"-formedAt"}}, list, id)
	if want := [][]string{{"SYN-CHG-6", "SYN-CHG-5"}, {"SYN-CHG-3", "SYN-CHG-4"}, {"SYN-CHG-2", "SYN-CHG-1"}}; !reflect.DeepEqual(byFormed, want) || total != 6 {
		t.Fatalf("形成时刻倒序：得 %v / total %d，应为 %v / 6", byFormed, total, want)
	}
}

// Covers: ADR-0144 决定四、五——stage 落在阶段列上，维内为或、维与 q 为与；q 在带包裹号的来源事实引用上做不分大小写的字面包含，
// % 按字面、零命中；total 与本页出自同一组条件，别的租户同一引用不算进来。
func TestCustomerChargesTotalFollowsTheStageFilterAndTheKeyword(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	fixture.seedCustomerCharge(t, catalogueTenant, "SYN-CHG-1", "SYN-FEE-01", "ESTIMATED", "SYN-CUR-01", 100, "SYN-CUR-01", 100, "", "")
	fixture.seedCustomerCharge(t, catalogueTenant, "SYN-CHG-2", "SYN-FEE-01", "PROVISIONAL", "SYN-CUR-01", 100, "SYN-CUR-01", 100, "", "")
	fixture.seedCustomerCharge(t, catalogueTenant, "SYN-CHG-3", "SYN-FEE-01", "CONFIRMED", "SYN-CUR-01", 100, "SYN-CUR-01", 100, "", "SYN-CBASIS-01")
	fixture.seedCustomerCharge(t, "SYN-TENANT-B", "SYN-CHG-9", "SYN-FEE-01", "CONFIRMED", "SYN-CUR-01", 100, "SYN-CUR-01", 100, "", "SYN-CBASIS-01")

	catalogueWalkCases(t, ports.CustomerChargePages, map[string]catalogueWalkCase{
		"一个阶段":      {values: url.Values{"stage": {"CONFIRMED"}}, want: [][]string{{"SYN-CHG-3"}}, total: 1},
		"维内为或":      {values: url.Values{"stage": {"ESTIMATED", "PROVISIONAL"}}, want: [][]string{{"SYN-CHG-1", "SYN-CHG-2"}}, total: 2},
		"q 命中来源事实":  {values: url.Values{"q": {"source-fact/syn"}}, want: [][]string{{"SYN-CHG-3"}}, total: 1},
		"维与 q 为与":   {values: url.Values{"stage": {"ESTIMATED"}, "q": {"SYN-CHG-3"}}, want: [][]string{{}}, total: 0},
		"q 的 % 按字面": {values: url.Values{"q": {"%"}}, want: [][]string{{}}, total: 0},
	}, func(query cataloguepage.Query) (cataloguepage.Result[ports.CustomerChargeCatalogueRow], error) {
		return fixture.charges.ListCustomerCharges(t.Context(), tenant, 10, query)
	}, func(row ports.CustomerChargeCatalogueRow) string { return row.Charge })
}

// Covers: spec「缺省序保今天的次序」「行标识取登记表主键去掉租户那几段」——供应商预期成本版本册按发生项正序、同一发生项按（费用项目、版本）翻页：翻页期间新登的发生项不让后面的页
// 重出或漏行；页大小 1 逐行走，游标落在同一条版本链中间、同一发生项的两条链之间、两个发生项之间都接得上；版本号最小的 SYN-COST-1
// 属另一条链，排在 SYN-FEE-A 那条链之后。recordedAt 落在登记时刻列上；q 命中费用项目，% 按字面。
func TestSupplierExpectedCostsPageAlongTheVersionChains(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	cost := func(version, occurrence, feeItem, prior string, recordedAfter time.Duration) {
		var priorRef, reason any
		if prior != "" {
			priorRef, reason = prior, "PRICING_CORRECTION"
		}
		fixture.seed(t,
			`INSERT INTO settlement_accounting.supplier_expected_cost
				(tenant_id, version, occurrence_id, occurrence_reason, occurrence_version,
				 occurred_at, fee_item, purchase_rule_version, agreement_ref, evaluation_ref,
				 original_currency, original_minor, settlement_currency, settlement_minor,
				 conversion_ref, prior_version, correction_reason, recorded_at)
			 VALUES ($1, $2, $3, 'BOOKING', 'V1', $4, $5, 'SYN-RULE-01', 'SYN-AGR-01', 'SYN-BUY-01',
			         'SYN-CUR-01', 1000, 'SYN-CUR-01', 1000, NULL, $6, $7, $8)`,
			catalogueTenant, version, occurrence, catalogueBaseAt, feeItem, priorRef, reason, catalogueBaseAt.Add(recordedAfter))
	}
	cost("SYN-COST-1", "SYN-OCC-1", "SYN-FEE-B", "", 0)
	cost("SYN-COST-2", "SYN-OCC-1", "SYN-FEE-A", "", time.Hour)
	cost("SYN-COST-3", "SYN-OCC-1", "SYN-FEE-A", "SYN-COST-2", 3*time.Hour)
	cost("SYN-COST-4", "SYN-OCC-2", "SYN-FEE-A", "", 2*time.Hour)
	listBy := func(size int) func(cataloguepage.Query) (cataloguepage.Result[ports.SupplierExpectedCostCatalogueRow], error) {
		return func(query cataloguepage.Query) (cataloguepage.Result[ports.SupplierExpectedCostCatalogueRow], error) {
			return fixture.charges.ListSupplierExpectedCosts(t.Context(), tenant, size, query)
		}
	}
	id := func(row ports.SupplierExpectedCostCatalogueRow) string { return row.Version }

	list := listBy(2)
	first, err := list(firstPage(t, ports.SupplierExpectedCostPages))
	if got := catalogueIDs(first.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-COST-2", "SYN-COST-3"}) || first.Total != 4 {
		t.Fatalf("第一页：得 %v / total %d（%v）", got, first.Total, err)
	}
	cost("SYN-COST-5", "SYN-OCC-0", "SYN-FEE-A", "", 4*time.Hour)
	cost("SYN-COST-6", "SYN-OCC-3", "SYN-FEE-A", "", -time.Hour)
	second, err := list(nextPage(t, ports.SupplierExpectedCostPages, url.Values{}, first.Next))
	if got := catalogueIDs(second.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-COST-1", "SYN-COST-4"}) || second.Total != 6 {
		t.Fatalf("第二页：得 %v / total %d（%v）", got, second.Total, err)
	}
	last, err := list(nextPage(t, ports.SupplierExpectedCostPages, url.Values{}, second.Next))
	if got := catalogueIDs(last.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-COST-6"}) || last.Next != "" {
		t.Fatalf("末页：得 %v / next %q（%v）", got, last.Next, err)
	}

	catalogueWalkCases(t, ports.SupplierExpectedCostPages, map[string]catalogueWalkCase{
		"缺省序": {
			values: url.Values{},
			want:   [][]string{{"SYN-COST-5"}, {"SYN-COST-2"}, {"SYN-COST-3"}, {"SYN-COST-1"}, {"SYN-COST-4"}, {"SYN-COST-6"}},
			total:  6,
		},
		"登记时刻倒序": {
			values: url.Values{"sort": {"-recordedAt"}},
			want:   [][]string{{"SYN-COST-5"}, {"SYN-COST-3"}, {"SYN-COST-4"}, {"SYN-COST-2"}, {"SYN-COST-1"}, {"SYN-COST-6"}},
			total:  6,
		},
		"q 命中费用项目":  {values: url.Values{"q": {"syn-fee-b"}}, want: [][]string{{"SYN-COST-1"}}, total: 1},
		"q 的 % 按字面": {values: url.Values{"q": {"%"}}, want: [][]string{{}}, total: 0},
	}, listBy(1), id)
}

// Covers: ADR-0144 决定一、四——已发布对账单册按单号正序翻页，翻页期间新发布的单不让后面的页重出或漏行；publishedAt 落在发布时刻
// 列上；q 命中单上争议的争议号（客户报来的常是它），% 按字面。
func TestCustomerStatementsPageWhileStatementsArriveAndFindTheDisputeNumber(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	publish := func(number string, publishedAfter time.Duration) {
		fixture.seedStatement(t, catalogueTenant, number, false)
		fixture.seed(t, `UPDATE settlement_accounting.customer_statement SET published_at = $3 WHERE tenant_id = $1 AND statement_number = $2`,
			catalogueTenant, number, catalogueBaseAt.Add(publishedAfter))
	}
	publish("SYN-STMT-2", time.Hour)
	publish("SYN-STMT-3", 3*time.Hour)
	publish("SYN-STMT-4", 2*time.Hour)
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.CustomerStatementCatalogueRow], error) {
		return fixture.statement.ListCustomerStatements(t.Context(), tenant, 2, query)
	}
	id := func(row ports.CustomerStatementCatalogueRow) string { return row.StatementNumber }

	first, err := list(firstPage(t, ports.CustomerStatementPages))
	if got := catalogueIDs(first.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-STMT-2", "SYN-STMT-3"}) || first.Total != 3 {
		t.Fatalf("第一页：得 %v / total %d（%v）", got, first.Total, err)
	}
	publish("SYN-STMT-1", 0)
	publish("SYN-STMT-5", 4*time.Hour)
	second, err := list(nextPage(t, ports.CustomerStatementPages, url.Values{}, first.Next))
	if got := catalogueIDs(second.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-STMT-4", "SYN-STMT-5"}) ||
		second.Total != 5 || second.Next != "" {
		t.Fatalf("第二页：得 %v / total %d / next %q（%v）", got, second.Total, second.Next, err)
	}

	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-3", "SYN-DSP-3", "")
	fixture.seed(t, `UPDATE settlement_accounting.statement_dispute SET document_number = 'SYN-DSP-202608-00007'
		WHERE tenant_id = $1 AND dispute_id = 'SYN-DSP-3'`, catalogueTenant)
	catalogueWalkCases(t, ports.CustomerStatementPages, map[string]catalogueWalkCase{
		"发布时刻倒序": {
			values: url.Values{"sort": {"-publishedAt"}},
			want:   [][]string{{"SYN-STMT-5", "SYN-STMT-3"}, {"SYN-STMT-4", "SYN-STMT-2"}, {"SYN-STMT-1"}},
			total:  5,
		},
		"q 命中争议号":   {values: url.Values{"q": {"syn-dsp-202608-00007"}}, want: [][]string{{"SYN-STMT-3"}}, total: 1},
		"q 的 % 按字面": {values: url.Values{"q": {"%"}}, want: [][]string{{}}, total: 0},
	}, list, id)
}

// Covers: spec「已带计数与 truncated 的待办队列保留这两格」——待对账队列的张数是整个队列的积压，不随 q 变；page.total 与本页随 q 变。队列按单号正序翻页，翻页期间新挂上
// 争议的单不让后面的页重出或漏行、张数随之涨，末页没有下一页；q 命中一张争议已终局的单，它也不进队列。
func TestDisputedStatementCountStaysTheBacklogWhileTheKeywordNarrowsThePage(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	for _, number := range []string{"SYN-STMT-A", "SYN-STMT-B", "SYN-STMT-C", "SYN-STMT-D"} {
		fixture.seedStatement(t, catalogueTenant, number, false)
	}
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-A", "SYN-DSP-A", "")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-B", "SYN-DSP-B", "PENDING_REVIEW")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-C", "SYN-DSP-C", "")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-D", "SYN-DSP-D", "ACCEPTED")
	fixture.seed(t, `UPDATE settlement_accounting.statement_dispute SET document_number = 'SYN-DSP-202608-00002'
		WHERE tenant_id = $1 AND dispute_id = 'SYN-DSP-B'`, catalogueTenant)
	list := func(t *testing.T, query cataloguepage.Query) ports.DisputedCustomerStatements {
		t.Helper()
		listed, err := fixture.statement.ListDisputedCustomerStatements(t.Context(), tenant, 2, query)
		if err != nil {
			t.Fatal(err)
		}
		return listed
	}
	numbers := func(listed ports.DisputedCustomerStatements) []string {
		return catalogueIDs(listed.Statements, func(row ports.CustomerStatementCatalogueRow) string { return row.StatementNumber })
	}

	first := list(t, firstPage(t, ports.DisputedCustomerStatementPages))
	if !reflect.DeepEqual(numbers(first), []string{"SYN-STMT-A", "SYN-STMT-B"}) || first.Count != 3 || first.Total != 3 || first.Next == "" {
		t.Fatalf("第一页：得 %v / count %d / total %d / next %q", numbers(first), first.Count, first.Total, first.Next)
	}
	for _, number := range []string{"SYN-STMT-0", "SYN-STMT-E"} {
		fixture.seedStatement(t, catalogueTenant, number, false)
		fixture.seedDispute(t, catalogueTenant, number, "SYN-DSP-"+number, "")
	}
	second := list(t, nextPage(t, ports.DisputedCustomerStatementPages, url.Values{}, first.Next))
	if !reflect.DeepEqual(numbers(second), []string{"SYN-STMT-C", "SYN-STMT-E"}) || second.Count != 5 || second.Total != 5 || second.Next != "" {
		t.Fatalf("末页：得 %v / count %d / total %d / next %q", numbers(second), second.Count, second.Total, second.Next)
	}
	for name, test := range map[string]struct {
		q    string
		want []string
	}{
		"q 命中争议号":     {q: "syn-dsp-202608-00002", want: []string{"SYN-STMT-B"}},
		"q 命中争议已终局的单": {q: "SYN-STMT-D", want: []string{}},
		"q 的 % 按字面":   {q: "%", want: []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			listed := list(t, pageQuery(t, ports.DisputedCustomerStatementPages, url.Values{"q": {test.q}}))
			if !reflect.DeepEqual(numbers(listed), test.want) || listed.Total != int64(len(test.want)) || listed.Count != 5 || listed.Next != "" {
				t.Fatalf("得 %v / total %d / count %d / next %q，应为 %v / count 仍是 5", numbers(listed), listed.Total, listed.Count, listed.Next, test.want)
			}
		})
	}
}

// Covers: ADR-0144 决定四「有状态的册至少放出状态一维」——对账单册与待对账队列按 voided 筛出的单，读回的作废时刻在场与否与所给的
// 词一致，total 随这一维变、与 q 为与；待对账队列的张数仍是整个队列的积压，不随这一维变（spec「已带计数与 truncated 的待办队列
// 保留这两格」），作废之前已开的争议照旧让那张已作废的单留在队列里。
func TestCustomerStatementVoidedDimensionAgreesWithTheVoidTrace(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	for number, voided := range map[string]bool{
		"SYN-STMT-V-1": false, "SYN-STMT-V-2": true, "SYN-STMT-V-3": false, "SYN-STMT-V-4": true,
	} {
		fixture.seedStatement(t, catalogueTenant, number, voided)
	}
	fixture.seedStatement(t, "tenant-b", "SYN-STMT-V-1", true)
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-V-2", "SYN-DSP-V-2", "")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-V-3", "SYN-DSP-V-3", "")
	numbers := func(rows []ports.CustomerStatementCatalogueRow) []string {
		return catalogueIDs(rows, func(row ports.CustomerStatementCatalogueRow) string { return row.StatementNumber })
	}
	// agree 核一页里每张单的作废时刻在场与否与 voided 那一维给的唯一取值一致；没给或给了两个时不按单个词核。
	agree := func(t *testing.T, values url.Values, rows []ports.CustomerStatementCatalogueRow) {
		t.Helper()
		words := values["voided"]
		if len(words) != 1 {
			return
		}
		for _, row := range rows {
			if (row.VoidedAt != nil) != (words[0] == "true") {
				t.Fatalf("按 voided=%s 筛出的 %s 读回作废时刻 %v", words[0], row.StatementNumber, row.VoidedAt)
			}
		}
	}

	for name, test := range map[string]struct {
		values url.Values
		want   []string
	}{
		"不带条件":      {values: url.Values{}, want: []string{"SYN-STMT-V-1", "SYN-STMT-V-2", "SYN-STMT-V-3", "SYN-STMT-V-4"}},
		"已作废":       {values: url.Values{"voided": {"true"}}, want: []string{"SYN-STMT-V-2", "SYN-STMT-V-4"}},
		"未作废":       {values: url.Values{"voided": {"false"}}, want: []string{"SYN-STMT-V-1", "SYN-STMT-V-3"}},
		"维内为或":      {values: url.Values{"voided": {"false", "true"}}, want: []string{"SYN-STMT-V-1", "SYN-STMT-V-2", "SYN-STMT-V-3", "SYN-STMT-V-4"}},
		"已作废且 q 命中": {values: url.Values{"voided": {"true"}, "q": {"syn-stmt-v-4"}}, want: []string{"SYN-STMT-V-4"}},
		"未作废且 q 不中": {values: url.Values{"voided": {"false"}, "q": {"SYN-STMT-V-2"}}, want: []string{}},
	} {
		t.Run("对账单册/"+name, func(t *testing.T) {
			page, err := fixture.statement.ListCustomerStatements(t.Context(), tenant, 10, pageQuery(t, ports.CustomerStatementPages, test.values))
			if err != nil {
				t.Fatal(err)
			}
			agree(t, test.values, page.Rows)
			if got := numbers(page.Rows); !reflect.DeepEqual(got, test.want) || page.Total != int64(len(test.want)) {
				t.Fatalf("得 %v / total %d，应为 %v / total %d", got, page.Total, test.want, len(test.want))
			}
		})
	}

	for name, test := range map[string]struct {
		values url.Values
		want   []string
	}{
		"不带条件": {values: url.Values{}, want: []string{"SYN-STMT-V-2", "SYN-STMT-V-3"}},
		"已作废":  {values: url.Values{"voided": {"true"}}, want: []string{"SYN-STMT-V-2"}},
		"未作废":  {values: url.Values{"voided": {"false"}}, want: []string{"SYN-STMT-V-3"}},
	} {
		t.Run("待对账队列/"+name, func(t *testing.T) {
			listed, err := fixture.statement.ListDisputedCustomerStatements(t.Context(), tenant, 10,
				pageQuery(t, ports.DisputedCustomerStatementPages, test.values))
			if err != nil {
				t.Fatal(err)
			}
			agree(t, test.values, listed.Statements)
			if got := numbers(listed.Statements); !reflect.DeepEqual(got, test.want) || listed.Total != int64(len(test.want)) || listed.Count != 2 {
				t.Fatalf("得 %v / total %d / count %d，应为 %v / total %d / count 仍是 2",
					got, listed.Total, listed.Count, test.want, len(test.want))
			}
		})
	}
}

// Covers: spec「行标识取登记表主键去掉租户那几段」「可排维与行标识的列必须非空」——账单接收册按收到时刻倒序翻页，同刻按（主张身份、版本）同向倒序：页大小 2 时第二、三页的分界落在同刻同
// 主张的两个版本之间，照样接得上；翻页期间新收到的账单（排在已翻过那头）不让后面的页重出，更早收到的那份在末页出现。period 落在
// 账期列上，同账期按主张身份与版本正序；q 命中供应商，% 按字面，与检索选择器为与。
func TestSupplierBillReceptionsPageByTheReceivedInstantWithTiesBrokenTheSameWay(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	seedReceptionReceivedAt(t, fixture, "SYN-SUP-01/R-4", "SYN-SUP-01", catalogueBaseAt, true)
	seedReceptionReceivedAt(t, fixture, "SYN-SUP-01/R-1", "SYN-SUP-01", catalogueBaseAt.Add(-3*time.Hour), true)
	// seedSupplierBill 的主张文档写死了收到时刻，这三行同刻收到。
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-SUP-02/R-2", "1", "SYN-SUP-02", "", catalogueBaseAt)
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-SUP-02/R-2", "2", "SYN-SUP-02", "", catalogueBaseAt)
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-SUP-02/R-3", "1", "SYN-SUP-02", "", catalogueBaseAt)
	reader := func(filter ports.SupplierBillReceptionFilter) func(cataloguepage.Query) (cataloguepage.Result[ports.SupplierBillReceptionCatalogueRow], error) {
		return func(query cataloguepage.Query) (cataloguepage.Result[ports.SupplierBillReceptionCatalogueRow], error) {
			return fixture.statement.ListSupplierBillReceptions(t.Context(), tenant, filter, 2, query)
		}
	}
	every := reader(ports.SupplierBillReceptionFilter{})
	id := func(row ports.SupplierBillReceptionCatalogueRow) string { return row.Claim + "@" + row.ClaimVersion }

	first, err := every(firstPage(t, ports.SupplierBillReceptionPages))
	if got := catalogueIDs(first.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-SUP-01/R-4@1", "SYN-SUP-01/R-1@1"}) || first.Total != 5 {
		t.Fatalf("第一页：得 %v / total %d（%v）", got, first.Total, err)
	}
	seedReceptionReceivedAt(t, fixture, "SYN-SUP-01/R-5", "SYN-SUP-01", catalogueBaseAt.Add(time.Hour), true)
	seedReceptionReceivedAt(t, fixture, "SYN-SUP-01/R-0", "SYN-SUP-01", catalogueBaseAt.Add(-24*time.Hour), true)
	second, err := every(nextPage(t, ports.SupplierBillReceptionPages, url.Values{}, first.Next))
	if got := catalogueIDs(second.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-SUP-02/R-3@1", "SYN-SUP-02/R-2@2"}) || second.Total != 7 {
		t.Fatalf("第二页：得 %v / total %d（%v）", got, second.Total, err)
	}
	last, err := every(nextPage(t, ports.SupplierBillReceptionPages, url.Values{}, second.Next))
	if got := catalogueIDs(last.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-SUP-02/R-2@1", "SYN-SUP-01/R-0@1"}) || last.Next != "" {
		t.Fatalf("末页：得 %v / next %q（%v）", got, last.Next, err)
	}

	fixture.seed(t, `UPDATE settlement_accounting.supplier_bill_reception SET period_ref = 'SYN-PER-2607'
		WHERE tenant_id = $1 AND claim_id = 'SYN-SUP-01/R-1'`, catalogueTenant)
	catalogueWalkCases(t, ports.SupplierBillReceptionPages, map[string]catalogueWalkCase{
		"按账期": {
			values: url.Values{"sort": {"period"}},
			want: [][]string{
				{"SYN-SUP-01/R-1@1", "SYN-SUP-01/R-0@1"}, {"SYN-SUP-01/R-4@1", "SYN-SUP-01/R-5@1"},
				{"SYN-SUP-02/R-2@1", "SYN-SUP-02/R-2@2"}, {"SYN-SUP-02/R-3@1"},
			},
			total: 7,
		},
		"q 命中供应商": {
			values: url.Values{"q": {"sup-02"}},
			want:   [][]string{{"SYN-SUP-02/R-3@1", "SYN-SUP-02/R-2@2"}, {"SYN-SUP-02/R-2@1"}},
			total:  3,
		},
		"q 的 % 按字面": {values: url.Values{"q": {"%"}}, want: [][]string{{}}, total: 0},
	}, every, id)
	narrowed, total := catalogueWalk(t, ports.SupplierBillReceptionPages, url.Values{"supplier": {"SYN-SUP-01"}, "q": {"r-4"}},
		reader(ports.SupplierBillReceptionFilter{Supplier: "SYN-SUP-01"}), id)
	if want := [][]string{{"SYN-SUP-01/R-4@1"}}; !reflect.DeepEqual(narrowed, want) || total != 1 {
		t.Fatalf("选择器与 q 为与：得 %v / total %d，应为 %v / 1", narrowed, total, want)
	}
}

// Covers: spec「可排维与行标识的列必须非空」——收到时刻导出成排序维的前提由迁移 0056 的约束在库上担保：主张文档缺收到时刻、写成 null、或写成不带时区或日在
// 前的形状，接收行落不了库（23514）；排序维因此不会是 NULL，也不随会话的时区与日期风格变。形状对而值越界的，取值函数在建索引项时
// 就解析失败，同样落不了库。
func TestASupplierBillReceptionWithoutAnInstantReceivedCannotBeRecorded(t *testing.T) {
	fixture := newCatalogueFixture(t)
	insert := func(t *testing.T, receivedAt string) error {
		t.Helper()
		_, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO settlement_accounting.supplier_bill_reception
				(tenant_id, claim_id, claim_version, supplier_ref, legal_entity, period_ref,
				 currency, content_digest, claim, matches, audit_authority_configured, recorded_at)
			 VALUES ($1, 'SYN-SUP-01/BAD', '1', 'SYN-SUP-01', 'SYN-LE-01', 'SYN-PER-2608', 'CNY', 'SYN-DIGEST',
			         jsonb_build_object('lines', '[{"line":"L1","fee_item":"SYN-FEE-01","claimed_minor":1000}]'::jsonb) || $2::jsonb,
			         '[]'::jsonb, true, $3)`,
			catalogueTenant, receivedAt, catalogueBaseAt)
		return err
	}
	for name, receivedAt := range map[string]string{
		"缺收到时刻":      `{}`,
		"收到时刻为 null": `{"received_at":null}`,
		"不带时区":       `{"received_at":"2026-08-30 09:00:00"}`,
		"日在前":        `{"received_at":"30/08/2026 09:00"}`,
		"不是时刻":       `{"received_at":"yesterday"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var pgErr *pgconn.PgError
			if err := insert(t, receivedAt); !errors.As(err, &pgErr) || pgErr.Code != "23514" ||
				pgErr.ConstraintName != "supplier_bill_reception_received_at_is_an_instant" {
				t.Fatalf("要被收到时刻那道约束拒（23514）：%v", err)
			}
		})
	}
	t.Run("形状对而值越界", func(t *testing.T) {
		if err := insert(t, `{"received_at":"2026-13-30T09:00:00Z"}`); err == nil {
			t.Fatal("十三月的收到时刻落了库")
		}
	})
	if err := insert(t, `{"received_at":"2026-08-30T09:00:00.123456+08:00"}`); err != nil {
		t.Fatalf("写口序列化的形状（带小数秒与时区偏移）要落得了库：%v", err)
	}
}

// Covers: spec「每迁一册补一条与缺省序同形的索引」——账单接收册的缺省序能走迁移 0056 的表达式索引：读面按取值函数排序，函数不被内联展开，规划器认得出同形索引。
// 照读面的来源形状（接收行左连账单号册）问规划器，关掉顺序扫描与显式排序，看它拿得出不排序的计划。
func TestSupplierBillReceptionDefaultOrderWalksTheReceivedAtIndex(t *testing.T) {
	fixture := newCatalogueFixture(t)
	ctx := t.Context()
	tx, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, setting := range []string{"SET LOCAL enable_seqscan = off", "SET LOCAL enable_sort = off"} {
		if _, err := tx.Exec(ctx, setting); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := tx.Query(ctx, `EXPLAIN
		WITH r AS (
			SELECT b.tenant_id, b.claim_id, b.claim_version,
			       settlement_accounting.supplier_bill_received_at(b.claim) AS received_at,
			       n.bill_number
			  FROM settlement_accounting.supplier_bill_reception AS b
			  LEFT JOIN settlement_accounting.supplier_bill_number AS n
			    ON n.tenant_id = b.tenant_id AND n.claim_id = b.claim_id
		)
		SELECT r.claim_id, r.bill_number
		  FROM r
		 WHERE r.tenant_id = 'tenant-a'
		 ORDER BY r.received_at DESC, r.claim_id DESC, r.claim_version DESC
		 LIMIT 3`)
	if err != nil {
		t.Fatal(err)
	}
	var plan []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if text := strings.Join(plan, "\n"); !strings.Contains(text, "supplier_bill_reception_by_received_at") || strings.Contains(text, "Sort") {
		t.Fatalf("缺省序没有顺着收到时刻的表达式索引走：\n%s", text)
	}
}

// Covers: spec「行标识取登记表主键去掉租户那几段」——审核应付册按审核时刻倒序翻页，同刻按应付标识同向倒序：页大小 2 时第一、二页的分界落在同刻的两笔之间，照样
// 接得上；翻页期间新审出的一笔（排在已翻过那头）不让后面的页重出，更早审的那笔在末页出现。q 命中应付单号，% 按字面，与检索选择器
// 为与。
func TestAuditedPayablesPageNewestFirstWithTiesBrokenTheSameWay(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-SUP-01/INV-1", "1", "SYN-SUP-01", "", catalogueBaseAt)
	payable := func(id string, number any, auditedAfter time.Duration) {
		seedAuditedPayable(t, fixture, catalogueTenant, id, "SYN-SUP-01/INV-1", "L1", 100, number, nil, catalogueBaseAt.Add(auditedAfter))
	}
	payable("SYN-PAYABLE-1", nil, 0)
	payable("SYN-PAYABLE-2", "SYN-AP-202608-00002", time.Hour)
	payable("SYN-PAYABLE-3", nil, time.Hour)
	payable("SYN-PAYABLE-4", nil, 2*time.Hour)
	reader := func(filter ports.AuditedPayableFilter) func(cataloguepage.Query) (cataloguepage.Result[ports.AuditedPayableCatalogueRow], error) {
		return func(query cataloguepage.Query) (cataloguepage.Result[ports.AuditedPayableCatalogueRow], error) {
			return fixture.statement.ListAuditedPayables(t.Context(), tenant, filter, 2, query)
		}
	}
	every := reader(ports.AuditedPayableFilter{})
	id := func(row ports.AuditedPayableCatalogueRow) string { return row.Payable }

	first, err := every(firstPage(t, ports.AuditedPayablePages))
	if got := catalogueIDs(first.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-PAYABLE-4", "SYN-PAYABLE-3"}) || first.Total != 4 {
		t.Fatalf("第一页：得 %v / total %d（%v）", got, first.Total, err)
	}
	payable("SYN-PAYABLE-5", nil, 3*time.Hour)
	payable("SYN-PAYABLE-0", nil, -time.Hour)
	second, err := every(nextPage(t, ports.AuditedPayablePages, url.Values{}, first.Next))
	if got := catalogueIDs(second.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-PAYABLE-2", "SYN-PAYABLE-1"}) || second.Total != 6 {
		t.Fatalf("第二页：得 %v / total %d（%v）", got, second.Total, err)
	}
	last, err := every(nextPage(t, ports.AuditedPayablePages, url.Values{}, second.Next))
	if got := catalogueIDs(last.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-PAYABLE-0"}) || last.Next != "" {
		t.Fatalf("末页：得 %v / next %q（%v）", got, last.Next, err)
	}

	catalogueWalkCases(t, ports.AuditedPayablePages, map[string]catalogueWalkCase{
		"q 命中应付单号":  {values: url.Values{"q": {"syn-ap-202608-00002"}}, want: [][]string{{"SYN-PAYABLE-2"}}, total: 1},
		"q 的 % 按字面": {values: url.Values{"q": {"%"}}, want: [][]string{{}}, total: 0},
	}, every, id)
	narrowed, total := catalogueWalk(t, ports.AuditedPayablePages, url.Values{"claim": {"SYN-SUP-01/INV-1"}, "q": {"payable-5"}},
		reader(ports.AuditedPayableFilter{Claim: "SYN-SUP-01/INV-1"}), id)
	if want := [][]string{{"SYN-PAYABLE-5"}}; !reflect.DeepEqual(narrowed, want) || total != 1 {
		t.Fatalf("选择器与 q 为与：得 %v / total %d，应为 %v / 1", narrowed, total, want)
	}
}

// Covers: ADR-0144 决定三、四——外部资金事实册按事实标识正序翻页，一笔事实只列链头那一版，total 数的是事实不是版本；翻页期间新采用
// 的事实不让后面的页重出或漏行；kind 落在事实类型列上，维内为或；q 命中事实标识，% 按字面；occurredAt 落在发生时刻列上。
func TestExternalFundsFactsPageOneChainHeadPerFact(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	fact := func(id, kind string, occurredAfter time.Duration) {
		seedExternalFundsFactVersion(t, fixture, id, kind, 1000, "V1", "", catalogueBaseAt)
		fixture.seed(t, `UPDATE settlement_accounting.external_funds_fact_version SET occurred_at = $3 WHERE tenant_id = $1 AND fact_id = $2`,
			catalogueTenant, id, catalogueBaseAt.Add(occurredAfter))
	}
	seedExternalFundsFactVersion(t, fixture, "SYN-FACT-1", "RECEIPT_CONFIRMED", 1000, "V1", "", catalogueBaseAt)
	seedExternalFundsFactVersion(t, fixture, "SYN-FACT-1", "RECEIPT_CONFIRMED", 900, "V2", "V1", catalogueBaseAt.Add(24*time.Hour))
	fact("SYN-FACT-2", "FUNDS_RETURNED", 2*time.Hour)
	fact("SYN-FACT-3", "RECEIPT_CONFIRMED", -time.Hour)
	fact("SYN-FACT-4", "PAYMENT_FAILED", time.Hour)
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.ExternalFundsFactCatalogueRow], error) {
		return fixture.funds.ListExternalFundsFacts(t.Context(), tenant, 2, query)
	}
	id := func(row ports.ExternalFundsFactCatalogueRow) string { return row.Fact + "@" + row.Version }

	first, err := list(firstPage(t, ports.ExternalFundsFactPages))
	if got := catalogueIDs(first.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-FACT-1@V2", "SYN-FACT-2@V1"}) || first.Total != 4 {
		t.Fatalf("第一页：得 %v / total %d（%v）", got, first.Total, err)
	}
	fact("SYN-FACT-0", "PAYMENT_FAILED", -2*time.Hour)
	fact("SYN-FACT-5", "FUNDS_RETURNED", 3*time.Hour)
	second, err := list(nextPage(t, ports.ExternalFundsFactPages, url.Values{}, first.Next))
	if got := catalogueIDs(second.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-FACT-3@V1", "SYN-FACT-4@V1"}) || second.Total != 6 {
		t.Fatalf("第二页：得 %v / total %d（%v）", got, second.Total, err)
	}
	last, err := list(nextPage(t, ports.ExternalFundsFactPages, url.Values{}, second.Next))
	if got := catalogueIDs(last.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-FACT-5@V1"}) || last.Next != "" {
		t.Fatalf("末页：得 %v / next %q（%v）", got, last.Next, err)
	}

	catalogueWalkCases(t, ports.ExternalFundsFactPages, map[string]catalogueWalkCase{
		"发生时刻": {
			values: url.Values{"sort": {"occurredAt"}},
			want:   [][]string{{"SYN-FACT-0@V1", "SYN-FACT-3@V1"}, {"SYN-FACT-1@V2", "SYN-FACT-4@V1"}, {"SYN-FACT-2@V1", "SYN-FACT-5@V1"}},
			total:  6,
		},
		"一个类型": {values: url.Values{"kind": {"RECEIPT_CONFIRMED"}}, want: [][]string{{"SYN-FACT-1@V2", "SYN-FACT-3@V1"}}, total: 2},
		"维内为或": {
			values: url.Values{"kind": {"FUNDS_RETURNED", "PAYMENT_FAILED"}},
			want:   [][]string{{"SYN-FACT-0@V1", "SYN-FACT-2@V1"}, {"SYN-FACT-4@V1", "SYN-FACT-5@V1"}},
			total:  4,
		},
		"q 命中事实标识":  {values: url.Values{"q": {"syn-fact-3"}}, want: [][]string{{"SYN-FACT-3@V1"}}, total: 1},
		"q 的 % 按字面": {values: url.Values{"q": {"%"}}, want: [][]string{{}}, total: 0},
	}, list, id)
}

// Covers: spec「行标识取登记表主键去掉租户那几段」——经营结果册按分析范围正序、同一范围按（账期、基准）翻页：翻页期间新登的快照不让后面的页重出或漏行；页大小 1
// 逐行走，只差基准的两行之间也接得上；basis 落在基准列上，维内为或；q 命中账期，% 按字面；recordedAt 落在落库时刻列上。
func TestOperatingResultsPageAcrossTheThreePartIdentity(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	result := func(scope, period, basis string, recordedAfter time.Duration) {
		fixture.seed(t,
			`INSERT INTO settlement_accounting.operating_result
				(tenant_id, scope_ref, period_ref, basis, currency, components, margin_minor,
				 version, as_of, content_digest, recorded_at)
			 VALUES ($1, $2, $3, $4, 'SYN-CUR-01',
			         '[{"source":"SYN-RECEIVABLE-01","role":"CUSTOMER_OPERATING_RECEIVABLE","effect":"INCREASES","amountMinor":1000}]'::jsonb,
			         1000, 'SYN-RESV-01', $5, 'SYN-DIGEST', $6)`,
			catalogueTenant, scope, period, basis, catalogueBaseAt, catalogueBaseAt.Add(recordedAfter))
	}
	result("SYN-SCOPE-1", "SYN-PER-1", "ESTIMATED", time.Hour)
	result("SYN-SCOPE-1", "SYN-PER-1", "CONFIRMED", 3*time.Hour)
	result("SYN-SCOPE-1", "SYN-PER-2", "CONFIRMED", 0)
	result("SYN-SCOPE-2", "SYN-PER-1", "SETTLED", 2*time.Hour)
	listBy := func(size int) func(cataloguepage.Query) (cataloguepage.Result[ports.OperatingResultCatalogueRow], error) {
		return func(query cataloguepage.Query) (cataloguepage.Result[ports.OperatingResultCatalogueRow], error) {
			return fixture.operating.ListOperatingResults(t.Context(), tenant, size, query)
		}
	}
	id := func(row ports.OperatingResultCatalogueRow) string {
		return row.Scope + "/" + row.Period + "/" + row.Basis
	}

	list := listBy(2)
	first, err := list(firstPage(t, ports.OperatingResultPages))
	if got := catalogueIDs(first.Rows, id); err != nil ||
		!reflect.DeepEqual(got, []string{"SYN-SCOPE-1/SYN-PER-1/CONFIRMED", "SYN-SCOPE-1/SYN-PER-1/ESTIMATED"}) || first.Total != 4 {
		t.Fatalf("第一页：得 %v / total %d（%v）", got, first.Total, err)
	}
	result("SYN-SCOPE-0", "SYN-PER-1", "CONFIRMED", 4*time.Hour)
	result("SYN-SCOPE-3", "SYN-PER-1", "ESTIMATED", -time.Hour)
	second, err := list(nextPage(t, ports.OperatingResultPages, url.Values{}, first.Next))
	if got := catalogueIDs(second.Rows, id); err != nil ||
		!reflect.DeepEqual(got, []string{"SYN-SCOPE-1/SYN-PER-2/CONFIRMED", "SYN-SCOPE-2/SYN-PER-1/SETTLED"}) || second.Total != 6 {
		t.Fatalf("第二页：得 %v / total %d（%v）", got, second.Total, err)
	}
	last, err := list(nextPage(t, ports.OperatingResultPages, url.Values{}, second.Next))
	if got := catalogueIDs(last.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-SCOPE-3/SYN-PER-1/ESTIMATED"}) || last.Next != "" {
		t.Fatalf("末页：得 %v / next %q（%v）", got, last.Next, err)
	}

	catalogueWalkCases(t, ports.OperatingResultPages, map[string]catalogueWalkCase{
		"缺省序": {
			values: url.Values{},
			want: [][]string{
				{"SYN-SCOPE-0/SYN-PER-1/CONFIRMED"}, {"SYN-SCOPE-1/SYN-PER-1/CONFIRMED"}, {"SYN-SCOPE-1/SYN-PER-1/ESTIMATED"},
				{"SYN-SCOPE-1/SYN-PER-2/CONFIRMED"}, {"SYN-SCOPE-2/SYN-PER-1/SETTLED"}, {"SYN-SCOPE-3/SYN-PER-1/ESTIMATED"},
			},
			total: 6,
		},
		"落库时刻倒序": {
			values: url.Values{"sort": {"-recordedAt"}},
			want: [][]string{
				{"SYN-SCOPE-0/SYN-PER-1/CONFIRMED"}, {"SYN-SCOPE-1/SYN-PER-1/CONFIRMED"}, {"SYN-SCOPE-2/SYN-PER-1/SETTLED"},
				{"SYN-SCOPE-1/SYN-PER-1/ESTIMATED"}, {"SYN-SCOPE-1/SYN-PER-2/CONFIRMED"}, {"SYN-SCOPE-3/SYN-PER-1/ESTIMATED"},
			},
			total: 6,
		},
		"一个基准": {
			values: url.Values{"basis": {"CONFIRMED"}},
			want: [][]string{
				{"SYN-SCOPE-0/SYN-PER-1/CONFIRMED"}, {"SYN-SCOPE-1/SYN-PER-1/CONFIRMED"}, {"SYN-SCOPE-1/SYN-PER-2/CONFIRMED"},
			},
			total: 3,
		},
		"维内为或": {
			values: url.Values{"basis": {"ESTIMATED", "SETTLED"}},
			want: [][]string{
				{"SYN-SCOPE-1/SYN-PER-1/ESTIMATED"}, {"SYN-SCOPE-2/SYN-PER-1/SETTLED"}, {"SYN-SCOPE-3/SYN-PER-1/ESTIMATED"},
			},
			total: 3,
		},
		"q 命中账期":    {values: url.Values{"q": {"syn-per-2"}}, want: [][]string{{"SYN-SCOPE-1/SYN-PER-2/CONFIRMED"}}, total: 1},
		"q 的 % 按字面": {values: url.Values{"q": {"%"}}, want: [][]string{{}}, total: 0},
	}, listBy(1), id)
}

// Covers: ADR-0144 决定一——成本分摊册按分摊标识正序翻页，翻页期间新登的分摊不让后面的页重出或漏行；recordedAt 落在落库时刻列上；
// q 命中来源，% 按字面。
func TestCostAllocationsPageWhileAllocationsArrive(t *testing.T) {
	fixture := newCatalogueFixture(t)
	tenant := catalogueTenantID(t, catalogueTenant)
	allocate := func(allocation, source string, recordedAfter time.Duration) {
		fixture.seed(t,
			`INSERT INTO settlement_accounting.cost_allocation
				(tenant_id, allocation_id, source_ref, source_minor, currency, rule_ref,
				 portions, unallocated_minor, version, allocated_at, content_digest, recorded_at)
			 VALUES ($1, $2, $3, 1000, 'SYN-CUR-01', 'SYN-ARULE-01', '[]'::jsonb, 1000, 'SYN-ALCV-01', $4, 'SYN-DIGEST', $5)`,
			catalogueTenant, allocation, source, catalogueBaseAt, catalogueBaseAt.Add(recordedAfter))
	}
	allocate("SYN-ALC-2", "SYN-SRC-A", time.Hour)
	allocate("SYN-ALC-3", "SYN-SRC-B", 0)
	allocate("SYN-ALC-4", "SYN-SRC-A", 2*time.Hour)
	list := func(query cataloguepage.Query) (cataloguepage.Result[ports.CostAllocationCatalogueRow], error) {
		return fixture.operating.ListCostAllocations(t.Context(), tenant, 2, query)
	}
	id := func(row ports.CostAllocationCatalogueRow) string { return row.Allocation }

	first, err := list(firstPage(t, ports.CostAllocationPages))
	if got := catalogueIDs(first.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-ALC-2", "SYN-ALC-3"}) || first.Total != 3 {
		t.Fatalf("第一页：得 %v / total %d（%v）", got, first.Total, err)
	}
	allocate("SYN-ALC-1", "SYN-SRC-B", 3*time.Hour)
	allocate("SYN-ALC-5", "SYN-SRC-B", -time.Hour)
	second, err := list(nextPage(t, ports.CostAllocationPages, url.Values{}, first.Next))
	if got := catalogueIDs(second.Rows, id); err != nil || !reflect.DeepEqual(got, []string{"SYN-ALC-4", "SYN-ALC-5"}) ||
		second.Total != 5 || second.Next != "" {
		t.Fatalf("第二页：得 %v / total %d / next %q（%v）", got, second.Total, second.Next, err)
	}

	catalogueWalkCases(t, ports.CostAllocationPages, map[string]catalogueWalkCase{
		"落库时刻倒序": {
			values: url.Values{"sort": {"-recordedAt"}},
			want:   [][]string{{"SYN-ALC-1", "SYN-ALC-4"}, {"SYN-ALC-2", "SYN-ALC-3"}, {"SYN-ALC-5"}},
			total:  5,
		},
		"q 命中来源":    {values: url.Values{"q": {"syn-src-a"}}, want: [][]string{{"SYN-ALC-2", "SYN-ALC-4"}}, total: 2},
		"q 的 % 按字面": {values: url.Values{"q": {"%"}}, want: [][]string{{}}, total: 0},
	}, list, id)
}
