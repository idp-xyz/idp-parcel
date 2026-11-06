package main

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// tfFirstPage 是运输履约一册不带任何条件的第一页（缺省序，ADR-0144）。本包的装配用例只看行，照旧走它。
func tfFirstPage(t *testing.T, catalogue *cataloguepage.Catalogue) cataloguepage.Query {
	t.Helper()
	query, err := catalogue.Decode(nil)
	if err != nil {
		t.Fatalf("解出第一页：%v", err)
	}
	return query
}

// tfRowsOf 只取本页行，让只看行的旧用例照旧一行接住读口的两个返回值。
func tfRowsOf[Row any](page cataloguepage.Result[Row], err error) ([]Row, error) {
	return page.Rows, err
}
