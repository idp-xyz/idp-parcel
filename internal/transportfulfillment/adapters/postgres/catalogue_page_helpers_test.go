package postgres_test

import (
	"net/url"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// firstPage 是不带任何条件的第一页（缺省序）。迁移到 ADR-0144 之前写的用例只看行，照旧走它。
func firstPage(t testing.TB, catalogue *cataloguepage.Catalogue) cataloguepage.Query {
	t.Helper()
	return pageQuery(t, catalogue, url.Values{})
}

// pageQuery 按查询串解出一次查询，与端点解码同一条路。
func pageQuery(t testing.TB, catalogue *cataloguepage.Catalogue, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := catalogue.Decode(values)
	if err != nil {
		t.Fatalf("解出查询 %v：%v", values, err)
	}
	return query
}

// nextPage 是接着上一页翻的那一问：同一组条件带上上一页给的游标。
func nextPage(t testing.TB, catalogue *cataloguepage.Catalogue, values url.Values, cursor string) cataloguepage.Query {
	t.Helper()
	if cursor == "" {
		t.Fatal("上一页已是末页，没有游标可接")
	}
	next := url.Values{}
	for key, given := range values {
		next[key] = append([]string(nil), given...)
	}
	next.Set("after", cursor)
	return pageQuery(t, catalogue, next)
}

// rowsOf 只取本页行，让只看行的旧用例照旧一行接住读口的两个返回值。
func rowsOf[Row any](page cataloguepage.Result[Row], err error) ([]Row, error) {
	return page.Rows, err
}
