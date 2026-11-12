package cataloguepage_test

import (
	"reflect"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件钉分面计数与多值筛选维（ADR-0378 决定四、五）：点名的维各拼一句、条件不含该维自己；多值维按数组交集筛、
// 分面时按数组里的每个值各记一次；没点名的册一句都不拼。

// faceted 是测试用的一册：status 是单值维、tags 是多值维，两维都点名给分面；owner 是不设词表、不给分面的一维。
func facetedSpec() cataloguepage.Spec {
	return cataloguepage.Spec{
		Name:        "syn-faceted",
		Sorts:       []cataloguepage.SortDimension{{Name: "id", Kind: cataloguepage.Text}},
		DefaultSort: cataloguepage.Sort{Field: "id"},
		Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
		Filters: []cataloguepage.FilterDimension{
			{Name: "status", Vocabulary: []string{"OPEN", "CLOSED", "HELD"}, Facet: true},
			{Name: "tags", Vocabulary: []string{"RED", "GREEN", "BLUE"}, Facet: true},
			{Name: "owner"},
		},
	}
}

func facetedColumns() cataloguepage.Columns {
	return cataloguepage.Columns{
		Sorts:       map[string]string{"id": "row_id"},
		Identity:    []string{"row_id"},
		Filters:     map[string]string{"status": "status", "tags": "tags", "owner": "owner_id"},
		MultiValued: []string{"tags"},
		Keyword:     []string{"row_id", "title"},
	}
}

var (
	faceted        = cataloguepage.MustCatalogue(facetedSpec())
	facetedBinding = faceted.MustBind(facetedColumns())
)

func facetedQuery(t *testing.T, raw string) cataloguepage.Query {
	t.Helper()
	query, err := faceted.DecodeRaw(raw)
	if err != nil {
		t.Fatalf("解 %q：%v", raw, err)
	}
	return query
}

// Covers: 多值维的条件是数组交集——所给的值里有一个在行的数组里即匹配；值照旧去重升序进参数。
func TestAMultiValuedDimensionFiltersByArrayOverlap(t *testing.T) {
	plan, err := facetedBinding.Plan(facetedQuery(t, "tags=RED&tags=BLUE&status=OPEN"), "tenant_id = $1", []any{tenant}, 50)
	if err != nil {
		t.Fatalf("拼片段：%v", err)
	}
	if want := "(tenant_id = $1) AND status = ANY($2) AND tags && $3"; plan.Where != want {
		t.Fatalf("Where = %q, want %q", plan.Where, want)
	}
	if want := []any{tenant, []string{"OPEN"}, []string{"BLUE", "RED"}}; !reflect.DeepEqual(plan.Args, want) {
		t.Fatalf("Args = %#v, want %#v", plan.Args, want)
	}
}

// Covers: 每个点名的维一句，照声明的次序；条件取本次的 q 与其余各维，不含该维自己；单值维按列分组，多值维把数组
// 拆开、一行记进它属于的每一格。
func TestFacetPlansLeaveOutTheDimensionBeingCounted(t *testing.T) {
	plans, err := facetedBinding.FacetPlans(
		facetedQuery(t, "status=OPEN&tags=RED&owner=SYN-OWNER-1&q=abc"), "tenant_id = $1", []any{tenant},
	)
	if err != nil {
		t.Fatalf("拼分面：%v", err)
	}
	if len(plans) != 2 || plans[0].Dimension != "status" || plans[1].Dimension != "tags" {
		t.Fatalf("分面的维 = %v, want status 与 tags 各一句、照声明次序", plans)
	}

	status := plans[0]
	if want := "SELECT status AS facet_value, count(*) FROM listed WHERE (tenant_id = $1) AND owner_id = ANY($2)" +
		" AND tags && $3 AND (row_id ILIKE $4 OR title ILIKE $4) GROUP BY facet_value"; status.SQL("listed") != want {
		t.Fatalf("status 的分面 = %q, want %q", status.SQL("listed"), want)
	}
	if want := []any{tenant, []string{"SYN-OWNER-1"}, []string{"RED"}, "%abc%"}; !reflect.DeepEqual(status.Args, want) {
		t.Fatalf("status 的参数 = %#v, want %#v", status.Args, want)
	}

	tags := plans[1]
	if want := "SELECT facet_value, count(*) FROM listed, unnest(tags) AS facet_value WHERE (tenant_id = $1)" +
		" AND owner_id = ANY($2) AND status = ANY($3) AND (row_id ILIKE $4 OR title ILIKE $4) GROUP BY facet_value"; tags.SQL("listed") != want {
		t.Fatalf("tags 的分面 = %q, want %q", tags.SQL("listed"), want)
	}
	if want := []any{tenant, []string{"SYN-OWNER-1"}, []string{"OPEN"}, "%abc%"}; !reflect.DeepEqual(tags.Args, want) {
		t.Fatalf("tags 的参数 = %#v, want %#v", tags.Args, want)
	}
}

// Covers: 不带任何条件时分面的条件就是读面的固定条件；连固定条件也没有时是 TRUE。
func TestFacetPlansWithoutConditions(t *testing.T) {
	plans, err := facetedBinding.FacetPlans(facetedQuery(t, ""), "", nil)
	if err != nil {
		t.Fatalf("拼分面：%v", err)
	}
	if want := "SELECT status AS facet_value, count(*) FROM listed WHERE TRUE GROUP BY facet_value"; plans[0].SQL("listed") != want {
		t.Fatalf("分面 = %q, want %q", plans[0].SQL("listed"), want)
	}
	if len(plans[0].Args) != 0 {
		t.Fatalf("参数 = %#v, want 空", plans[0].Args)
	}
}

// Covers: 答复里的一格照词表次序列出词表上的每个值，没取到的记零；词表之外的值不列。
func TestFacetCountsListEveryWordInVocabularyOrder(t *testing.T) {
	plans, err := facetedBinding.FacetPlans(facetedQuery(t, ""), "", nil)
	if err != nil {
		t.Fatalf("拼分面：%v", err)
	}
	got := plans[0].Counts(map[string]int64{"HELD": 2, "OPEN": 7, "SOMETHING_ELSE": 9})
	want := []cataloguepage.FacetCount{{Value: "OPEN", Count: 7}, {Value: "CLOSED", Count: 0}, {Value: "HELD", Count: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Counts = %v, want %v", got, want)
	}
}

// Covers: 没点名分面的册一句都不拼——读面据此不发任何分面查询，答复里没有 facets 一格。
func TestACatalogueWithoutFacetsPlansNone(t *testing.T) {
	query, err := accounts.DecodeRaw("status=EFFECTIVE")
	if err != nil {
		t.Fatalf("解查询：%v", err)
	}
	plans, err := accountsBinding.FacetPlans(query, "tenant_id = $1", []any{tenant})
	if err != nil || len(plans) != 0 {
		t.Fatalf("FacetPlans = (%v, %v), want 空、无错", plans, err)
	}
}

// Covers: 查询对象须由本册解出——拿别册的查询来拼分面是编程错误，与 Plan 同一道门。
func TestFacetPlansRefuseAQueryOfAnotherCatalogue(t *testing.T) {
	query, err := accounts.DecodeRaw("")
	if err != nil {
		t.Fatalf("解查询：%v", err)
	}
	if _, err := facetedBinding.FacetPlans(query, "", nil); err == nil {
		t.Fatal("别册的查询对象被收下了")
	}
}

// Covers: 声明与落列写错在装配时就报——没有词表的维不能给分面（没有「每个值」可列）；给分面的词表里不能有重复的值；
// 多值维要在声明里、不能带代码换算；给分面的维不能带代码换算（分出来的是代码，对不上词表）。
func TestIllFormedFacetDeclarationsAreRefused(t *testing.T) {
	withoutVocabulary := facetedSpec()
	withoutVocabulary.Filters[2].Facet = true
	if _, err := cataloguepage.NewCatalogue(withoutVocabulary); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("没有词表的维点名给分面：err = %v, want 点名 owner 的拒绝", err)
	}

	repeated := facetedSpec()
	repeated.Filters[0].Vocabulary = []string{"OPEN", "CLOSED", "OPEN"}
	if _, err := cataloguepage.NewCatalogue(repeated); err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("给分面的词表有重复值：err = %v, want 点名 status 的拒绝", err)
	}

	cases := map[string]func(*cataloguepage.Columns){
		"多值维不在声明里": func(columns *cataloguepage.Columns) { columns.MultiValued = []string{"kind"} },
		"多值维带代码换算": func(columns *cataloguepage.Columns) {
			columns.Codes = map[string]func(string) (int64, error){"tags": statusCode}
		},
		"给分面的维带代码换算": func(columns *cataloguepage.Columns) {
			columns.Codes = map[string]func(string) (int64, error){"status": statusCode}
		},
	}
	for name, spoil := range cases {
		columns := facetedColumns()
		spoil(&columns)
		if _, err := faceted.Bind(columns); err == nil {
			t.Errorf("%s：落列被收下了", name)
		}
	}
}
