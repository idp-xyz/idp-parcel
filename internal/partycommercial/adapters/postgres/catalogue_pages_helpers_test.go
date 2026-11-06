package postgres_test

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是票 catalogue-read-pagination-rollout/11 各册真库用例共用的取页助手：查询对象一律按本册声明解出，与端点解的是
// 同一份声明。只看行、不翻页的用例取缺省序的第一页（firstPage）。

func firstPage(t *testing.T, catalogue *cataloguepage.Catalogue) cataloguepage.Query {
	t.Helper()
	return pageQuery(t, catalogue, url.Values{})
}

func pageQuery(t *testing.T, catalogue *cataloguepage.Catalogue, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := catalogue.Decode(values)
	if err != nil {
		t.Fatalf("解出查询 %v：%v", values, err)
	}
	return query
}

// rawPageQuery 按一段查询串解出查询对象，表驱动的筛选与 q 用例用它。
func rawPageQuery(t *testing.T, catalogue *cataloguepage.Catalogue, raw string) cataloguepage.Query {
	t.Helper()
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query %q: %v", raw, err)
	}
	return pageQuery(t, catalogue, values)
}

// nextPage 在同一组条件上接上一页的游标。
func nextPage(t *testing.T, catalogue *cataloguepage.Catalogue, raw, cursor string) cataloguepage.Query {
	t.Helper()
	if cursor == "" {
		t.Fatal("上一页已是末页，没有游标可接")
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query %q: %v", raw, err)
	}
	values.Set("after", cursor)
	return pageQuery(t, catalogue, values)
}

func rowsOf[Row any](page cataloguepage.Result[Row], err error) ([]Row, error) {
	return page.Rows, err
}

// joinedBy 把一页的行按某一格拼成逗号串，次序即读面答的次序。
func joinedBy[Row any](rows []Row, cell func(Row) string) string {
	cells := make([]string, len(rows))
	for index, row := range rows {
		cells[index] = cell(row)
	}
	return strings.Join(cells, ",")
}

// pageLister 是一册读口在用例里的样子：按查询对象与页大小取一页。
type pageLister[Row any] func(query cataloguepage.Query, limit int) (cataloguepage.Result[Row], error)

// assertPagesWithoutRepeatsOrGaps 钉完成判据「翻页期间插入新行，已翻过的页不重出、未翻的页不漏行」——ADR-0144 否决偏移分页的
// 理由就是这一格：新登的排在最前，偏移会把上一页末尾那行挤进下一页开头。save 按 1…6 的次序登记 prefix 加序号的行，后登的
// 在缺省序上排前；每页两行，第一页之后再登第六行，它落在已翻过的那头：第二页接着第一页末行往后、总数跟着变成 6，第三页
// 取尽、next 为空。
func assertPagesWithoutRepeatsOrGaps[Row any](
	t *testing.T,
	catalogue *cataloguepage.Catalogue,
	list pageLister[Row],
	save func(id string),
	idOf func(Row) string,
	prefix string,
) {
	t.Helper()
	id := func(serial int) string { return prefix + strconv.Itoa(serial) }
	for serial := 1; serial <= 5; serial++ {
		save(id(serial))
	}

	first, err := list(firstPage(t, catalogue), 2)
	if err != nil {
		t.Fatalf("%s 第一页：%v", prefix, err)
	}
	if got := joinedBy(first.Rows, idOf); got != id(5)+","+id(4) || first.Total != 5 || first.Next == "" {
		t.Fatalf("%s 第一页 = %s（共 %d，next %q），want %s,%s、共 5、有下一页", prefix, got, first.Total, first.Next, id(5), id(4))
	}

	save(id(6))

	second, err := list(nextPage(t, catalogue, "", first.Next), 2)
	if err != nil {
		t.Fatalf("%s 第二页：%v", prefix, err)
	}
	if got := joinedBy(second.Rows, idOf); got != id(3)+","+id(2) || second.Total != 6 || second.Next == "" {
		t.Fatalf("%s 第二页 = %s（共 %d）；新登的 %s 该落在已翻过的那头，总数跟着它变", prefix, got, second.Total, id(6))
	}
	third, err := list(nextPage(t, catalogue, "", second.Next), 2)
	if err != nil {
		t.Fatalf("%s 第三页：%v", prefix, err)
	}
	if got := joinedBy(third.Rows, idOf); got != id(1) || third.Total != 6 || third.Next != "" {
		t.Fatalf("%s 末页 = %s（共 %d，next %q），want %s 且 next 为空", prefix, got, third.Total, third.Next, id(1))
	}
}

// pageCase 是筛选与 q 表里的一行：一段查询串与按读面次序拼起来的期望标识。
type pageCase struct {
	query string
	want  string
}

// assertFilteredPages 钉完成判据「total 随筛选与 q 变」与「q 里的 % 按字面匹配」：每一行的总数都与本页出自同一组条件
// （ADR-0144 决定五），页大小够大，本页即全部、总数即行数。
func assertFilteredPages[Row any](
	t *testing.T,
	name string,
	catalogue *cataloguepage.Catalogue,
	list pageLister[Row],
	idOf func(Row) string,
	cases []pageCase,
) {
	t.Helper()
	for _, tc := range cases {
		page, err := list(rawPageQuery(t, catalogue, tc.query), 50)
		if err != nil {
			t.Fatalf("%s %q：%v", name, tc.query, err)
		}
		if got := joinedBy(page.Rows, idOf); got != tc.want || page.Total != int64(len(page.Rows)) || page.Next != "" {
			t.Fatalf("%s %q = %s（共 %d，next %q），want %s 且总数与本页同一组条件",
				name, tc.query, got, page.Total, page.Next, tc.want)
		}
	}
}

// assertEmptyCatalogue 钉完成判据「空册答空页、total 为 0」：从未登记过的租户不是错误，是一页空的答案。
func assertEmptyCatalogue[Row any](t *testing.T, name string, catalogue *cataloguepage.Catalogue, list pageLister[Row]) {
	t.Helper()
	page, err := list(firstPage(t, catalogue), 50)
	if err != nil || len(page.Rows) != 0 || page.Total != 0 || page.Next != "" {
		t.Fatalf("%s 空册 = (%d 行、共 %d、next %q, %v)，want 空页、共 0", name, len(page.Rows), page.Total, page.Next, err)
	}
}
