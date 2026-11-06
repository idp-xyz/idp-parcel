package cataloguepage_test

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

const tenant = "SYN-TENANT-01"

// accountsColumns 把测试册落到读面的列：三个可排维、两段标识、两个筛选维与两列 q。
func accountsColumns() cataloguepage.Columns {
	return cataloguepage.Columns{
		Sorts:    map[string]string{"registeredAt": "recorded_at", "accountId": "account_id", "revision": "revision"},
		Identity: []string{"account_id", "revision"},
		Filters:  map[string]string{"status": "status", "customerPartyId": "customer_party_id"},
		Keyword:  []string{"account_id", "party_name"},
	}
}

var accountsBinding = accounts.MustBind(accountsColumns())

func planOf(t *testing.T, binding *cataloguepage.Binding, query cataloguepage.Query) cataloguepage.Plan {
	t.Helper()
	plan, err := binding.Plan(query, "tenant_id = $1", []any{tenant}, 200)
	if err != nil {
		t.Fatalf("拼片段：%v", err)
	}
	return plan
}

func TestAWellFormedBindingIsAccepted(t *testing.T) {
	if _, err := accounts.Bind(accountsColumns()); err != nil {
		t.Fatalf("合规落列被拒：%v", err)
	}
}

// Covers: 落列是各册写死的常量，错在这里就是编程错误，要在装配时报出来。漏落一维，那一维的查询拼不出列；多落一列，
// 说明声明与读面已经对不上；q 不落列，Decode 照收的检索词会被静默忽略（ADR-0144 决定四要防的正是这种对不上）。
func TestAnIllFormedBindingIsRefused(t *testing.T) {
	cases := map[string]func(*cataloguepage.Columns){
		"漏落可排维":   func(columns *cataloguepage.Columns) { delete(columns.Sorts, "revision") },
		"可排维列为空":  func(columns *cataloguepage.Columns) { columns.Sorts["revision"] = " " },
		"多落一个可排维": func(columns *cataloguepage.Columns) { columns.Sorts["name"] = "party_name" },
		"标识段数少一段": func(columns *cataloguepage.Columns) { columns.Identity = []string{"account_id"} },
		"标识列为空":   func(columns *cataloguepage.Columns) { columns.Identity[1] = "" },
		"漏落筛选维":   func(columns *cataloguepage.Columns) { delete(columns.Filters, "status") },
		"多落一个筛选维": func(columns *cataloguepage.Columns) { columns.Filters["kind"] = "kind" },
		"代码换算的维不在声明里": func(columns *cataloguepage.Columns) {
			columns.Codes = map[string]func(string) (int64, error){"kind": statusCode}
		},
		"代码换算为空": func(columns *cataloguepage.Columns) {
			columns.Codes = map[string]func(string) (int64, error){"status": nil}
		},
		"q 没有列":   func(columns *cataloguepage.Columns) { columns.Keyword = nil },
		"q 的一列为空": func(columns *cataloguepage.Columns) { columns.Keyword = []string{"account_id", ""} },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			columns := accountsColumns()
			change(&columns)
			if _, err := accounts.Bind(columns); err == nil {
				t.Fatal("不合规的落列应被拒")
			}
		})
	}
}

func TestMustBindPanicsOnAnIllFormedBinding(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("不合规的落列应 panic")
		}
	}()
	columns := accountsColumns()
	columns.Keyword = nil
	accounts.MustBind(columns)
}

// Covers: 第一页只有读面的固定条件；次序是缺省序加各段标识，全部同向（ADR-0144 决定三）；页大小多取一行。
func TestTheFirstPageIsTheBaseConditionInTheDefaultOrder(t *testing.T) {
	plan := planOf(t, accountsBinding, decode(t, url.Values{}))
	want := cataloguepage.Plan{
		Where:     "(tenant_id = $1)",
		Args:      []any{tenant},
		PageWhere: "(tenant_id = $1)",
		OrderBy:   "recorded_at DESC, account_id DESC, revision DESC",
		Limit:     "$2",
		PageArgs:  []any{tenant, 201},
	}
	if !reflect.DeepEqual(plan, want) {
		t.Fatalf("得 %#v，应为 %#v", plan, want)
	}
}

// Covers: ADR-0144 决定四「维内为或、维间为与」；维按名排序拼，同一组条件每次拼出同一句、参数编号也稳定。
func TestFiltersAreOrWithinAndAndAcrossInDimensionOrder(t *testing.T) {
	plan := planOf(t, accountsBinding, decode(t, url.Values{
		"status":          {"REGISTERED", "EFFECTIVE"},
		"customerPartyId": {"SYN-PARTY-01"},
	}))
	if want := "(tenant_id = $1) AND customer_party_id = ANY($2) AND status = ANY($3)"; plan.Where != want {
		t.Fatalf("条件：得 %q，应为 %q", plan.Where, want)
	}
	wantArgs := []any{tenant, []string{"SYN-PARTY-01"}, []string{"EFFECTIVE", "REGISTERED"}}
	if !reflect.DeepEqual(plan.Args, wantArgs) {
		t.Fatalf("参数：得 %#v，应为 %#v", plan.Args, wantArgs)
	}
	if plan.Limit != "$4" || !reflect.DeepEqual(plan.PageArgs, append(wantArgs, 201)) {
		t.Fatalf("页大小占位接在筛选参数之后：得 %s / %#v", plan.Limit, plan.PageArgs)
	}
}

// Covers: q 在点名的各列上做不分大小写的字面包含——% 与 _ 是 LIKE 的通配符、反斜杠是缺省转义符，三者都要转义，
// 否则检索「50%」会匹配到不含百分号的行。各列共用同一个参数。
func TestTheKeywordMatchesLiterallyAcrossItsColumns(t *testing.T) {
	plan := planOf(t, accountsBinding, decode(t, url.Values{"q": {` 50%_off\ `}}))
	if want := "(tenant_id = $1) AND (account_id ILIKE $2 OR party_name ILIKE $2)"; plan.Where != want {
		t.Fatalf("条件：得 %q，应为 %q", plan.Where, want)
	}
	if got := plan.Args[1]; got != `%50\%\_off\\%` {
		t.Fatalf("检索词参数：得 %q", got)
	}
}

// Covers: ADR-0144 决定一「按『排序维值 + 标识』取严格排在其后的行」——倒序时比较号是 <；游标位置按声明的值形状
// 换回列的 Go 值（时刻、文本、整数），参数才按列的类型比较。总数那半句不带键集，与第一页同一组条件（决定五）。
func TestACursorAddsTheKeysetAfterTheConditions(t *testing.T) {
	values := url.Values{"status": {"EFFECTIVE"}}
	first := planOf(t, accountsBinding, decode(t, values))
	next := planOf(t, accountsBinding, decode(t, with(values, cursorAfter(t, values, lastRow()))))

	if next.Where != first.Where || !reflect.DeepEqual(next.Args, first.Args) {
		t.Fatalf("翻页不改总数的条件：得 %q %#v，应为 %q %#v", next.Where, next.Args, first.Where, first.Args)
	}
	if want := first.Where + " AND (recorded_at, account_id, revision) < ($3, $4, $5)"; next.PageWhere != want {
		t.Fatalf("取页条件：得 %q，应为 %q", next.PageWhere, want)
	}
	wantArgs := append(append([]any(nil), first.Args...), registeredAt, "SYN-ACC-07", int64(3), 201)
	if !reflect.DeepEqual(next.PageArgs, wantArgs) || next.Limit != "$6" {
		t.Fatalf("取页参数：得 %#v / %s，应为 %#v / $6", next.PageArgs, next.Limit, wantArgs)
	}
}

// Covers: 顺序时比较号与次序一起翻成 > 与 ASC——决胜键与排序同向（决定三），游标才稳定。
func TestAnAscendingSortFlipsTheComparisonAndTheOrder(t *testing.T) {
	values := url.Values{"sort": {"accountId"}}
	last := cataloguepage.Position{Value: "SYN-ACC-07", Identity: []string{"SYN-ACC-07", cataloguepage.FormatInteger(3)}}
	plan := planOf(t, accountsBinding, decode(t, with(values, cursorAfter(t, values, last))))
	if want := "account_id ASC, account_id ASC, revision ASC"; plan.OrderBy != want {
		t.Fatalf("次序：得 %q，应为 %q", plan.OrderBy, want)
	}
	if want := "(tenant_id = $1) AND (account_id, account_id, revision) > ($2, $3, $4)"; plan.PageWhere != want {
		t.Fatalf("取页条件：得 %q，应为 %q", plan.PageWhere, want)
	}
}

func statusCode(status string) (int64, error) {
	switch status {
	case "REGISTERED":
		return 1, nil
	case "EFFECTIVE":
		return 2, nil
	case "DEACTIVATED":
		return 3, nil
	}
	return 0, errors.New("SYN 未知状态")
}

// Covers: 状态在登记表里存成代码时，按维给的换算把词换成代码进参数，读面不必为了筛选另导一列文本；换不成就报错，
// 不拼出一句静默答空的 SQL。
func TestACodedFilterBindsTheStoredCodes(t *testing.T) {
	columns := accountsColumns()
	columns.Codes = map[string]func(string) (int64, error){"status": statusCode}
	plan := planOf(t, accounts.MustBind(columns), decode(t, url.Values{"status": {"REGISTERED", "EFFECTIVE"}}))
	if want := "(tenant_id = $1) AND status = ANY($2)"; plan.Where != want {
		t.Fatalf("条件：得 %q", plan.Where)
	}
	if !reflect.DeepEqual(plan.Args[1], []int64{2, 1}) {
		t.Fatalf("代码参数：得 %#v", plan.Args[1])
	}

	columns.Codes = map[string]func(string) (int64, error){"status": func(string) (int64, error) {
		return 0, errors.New("SYN 换不成")
	}}
	if _, err := accounts.MustBind(columns).Plan(decode(t, url.Values{"status": {"EFFECTIVE"}}), "", nil, 200); err == nil {
		t.Fatal("换不成代码应报错")
	}
}

// Covers: 读面前半句已占用的参数（租户之外常还有选择器），新参数接在其后编号，不撞号。
func TestTheReadFacesArgumentsShiftTheNumbering(t *testing.T) {
	plan, err := accountsBinding.Plan(decode(t, url.Values{"customerPartyId": {"SYN-PARTY-01"}}),
		"tenant_id = $1 AND kind = $2", []any{tenant, "SYN-KIND"}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if want := "(tenant_id = $1 AND kind = $2) AND customer_party_id = ANY($3)"; plan.Where != want {
		t.Fatalf("条件：得 %q", plan.Where)
	}
	if plan.Limit != "$4" || plan.PageArgs[3] != 51 {
		t.Fatalf("页大小：得 %s / %#v", plan.Limit, plan.PageArgs)
	}
}

// Covers: 读面把租户写在 CTE 里时不给固定条件，没有任何条件就是 TRUE，拼出的句子照样成立。
func TestNoConditionsAtAllIsTrue(t *testing.T) {
	plan, err := accountsBinding.Plan(decode(t, url.Values{}), "", nil, 200)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Where != "TRUE" || plan.PageWhere != "TRUE" || plan.Limit != "$1" {
		t.Fatalf("得 %#v", plan)
	}
}

// Covers: 查询对象须由本册的 Decode 解出——另一册的查询带着另一套可排维与游标摘要，拿来拼片段等于翻另一份列表；
// 页大小非正是读面的编程错误。两者都答错误，不拼出一句错 SQL。
func TestPlanRefusesAForeignQueryAndANonPositiveLimit(t *testing.T) {
	spec := accountsSpec()
	spec.Name = "SYN-other"
	foreign, err := cataloguepage.MustCatalogue(spec).Decode(url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	for name, query := range map[string]cataloguepage.Query{"他册解出的": foreign, "零值": {}} {
		if _, err := accountsBinding.Plan(query, "", nil, 200); err == nil || !strings.Contains(err.Error(), "Decode") {
			t.Fatalf("%s查询对象应被拒，得 %v", name, err)
		}
	}
	if _, err := accountsBinding.Plan(decode(t, url.Values{}), "", nil, 0); err == nil {
		t.Fatal("页大小为零应被拒")
	}
}

func TestTheTwoStatementsShareTheConditions(t *testing.T) {
	plan := planOf(t, accountsBinding, decode(t, url.Values{"q": {"SYN"}}))
	if want := "SELECT count(*) FROM listed WHERE " + plan.Where; plan.CountSQL("listed") != want {
		t.Fatalf("总数：得 %q", plan.CountSQL("listed"))
	}
	want := "SELECT account_id, revision FROM listed WHERE " + plan.PageWhere + " ORDER BY " + plan.OrderBy + " LIMIT " + plan.Limit
	if got := plan.PageSQL("account_id, revision", "listed"); got != want {
		t.Fatalf("取页：得 %q", got)
	}
}
