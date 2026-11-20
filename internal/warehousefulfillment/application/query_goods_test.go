package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件证商品查阅用例只读已落册的修订：读不动原样上抛，按码找商品用注入的时钟判别名此刻是否有效。别的货主读不到是登记册适配器的事。

type stubGoodsCatalogue struct {
	limit   int
	keyword string
	page    cataloguepage.Result[ports.GoodsRow]
	err     error
}

func (stub *stubGoodsCatalogue) ListGoods(
	_ context.Context,
	_ domain.TenantID,
	_ domain.OwnerAccountID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.GoodsRow], error) {
	stub.limit = limit
	stub.keyword = query.Keyword
	if stub.err != nil {
		return cataloguepage.Result[ports.GoodsRow]{}, stub.err
	}
	return stub.page, nil
}

type stubGoodsCurrent struct {
	row   ports.GoodsRow
	found bool
	err   error
	at    time.Time
}

func (stub *stubGoodsCurrent) GetCurrentGoods(
	_ context.Context,
	_ domain.TenantID,
	_ domain.OwnerAccountID,
	_ domain.GoodsCode,
) (ports.GoodsRow, bool, error) {
	if stub.err != nil {
		return ports.GoodsRow{}, false, stub.err
	}
	return stub.row, stub.found, nil
}

func (stub *stubGoodsCurrent) FindGoodsByCode(
	_ context.Context,
	_ domain.TenantID,
	_ domain.OwnerAccountID,
	_ domain.IdentifierCode,
	at time.Time,
) (ports.GoodsRow, bool, error) {
	stub.at = at
	if stub.err != nil {
		return ports.GoodsRow{}, false, stub.err
	}
	return stub.row, stub.found, nil
}

type stubGoodsHistory struct {
	rows []ports.GoodsRow
	err  error
}

func (stub *stubGoodsHistory) ListGoodsRevisions(
	_ context.Context,
	_ domain.TenantID,
	_ domain.OwnerAccountID,
	_ domain.GoodsCode,
) ([]ports.GoodsRow, error) {
	if stub.err != nil {
		return nil, stub.err
	}
	return stub.rows, nil
}

func TestQueryGoodsPassesTheClockWhenFindingByCodeAndWrapsReadFailures(t *testing.T) {
	catalogue := &stubGoodsCatalogue{page: cataloguepage.Result[ports.GoodsRow]{Total: 1}}
	current := &stubGoodsCurrent{found: true, row: ports.GoodsRow{GoodsCode: "SYN-GOODS-01"}}
	history := &stubGoodsHistory{rows: []ports.GoodsRow{{Revision: 1}, {Revision: 2}}}
	handler := application.NewQueryGoodsHandler(catalogue, current, history, nil, &fixedClock{at: registrationInstant})
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	code := valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01")

	row, found, err := handler.CurrentGoods(t.Context(), tenant, owner, code)
	if err != nil || !found || row.GoodsCode != "SYN-GOODS-01" {
		t.Fatalf("当前修订 = %q（%v，%v）", row.GoodsCode, found, err)
	}
	page, err := handler.ListGoods(t.Context(), tenant, owner, 20, cataloguepage.Query{Keyword: "SYN"})
	if err != nil || page.Total != 1 || catalogue.limit != 20 || catalogue.keyword != "SYN" {
		t.Fatalf("列表 = %+v，替身见到 limit %d、q %q，err %v", page, catalogue.limit, catalogue.keyword, err)
	}
	revisions, err := handler.GoodsRevisions(t.Context(), tenant, owner, code)
	if err != nil || len(revisions) != 2 || revisions[0].Revision != 1 {
		t.Fatalf("修订史 = %v，%v", revisions, err)
	}
	identifier := valueOf(t, domain.NewIdentifierCode, "SYN-BAR-01")
	foundRow, found, err := handler.FindGoods(t.Context(), tenant, owner, identifier)
	if err != nil || !found || !current.at.Equal(registrationInstant) || foundRow.GoodsCode != "SYN-GOODS-01" {
		t.Fatalf("按码找 = %q（%v，%v），时钟 %v", foundRow.GoodsCode, found, err, current.at)
	}

	current.err = errGoodsRead
	if _, _, err := handler.CurrentGoods(t.Context(), tenant, owner, code); !errors.Is(err, errGoodsRead) {
		t.Fatalf("当前修订读不动 err = %v", err)
	}
	current.err = nil
	current.found = false
	if _, found, err := handler.FindGoods(t.Context(), tenant, owner, identifier); err != nil || found {
		t.Fatalf("找不到应是没有，不是失败：%v %v", found, err)
	}
	history.err = errGoodsRead
	if _, err := handler.GoodsRevisions(t.Context(), tenant, owner, code); !errors.Is(err, errGoodsRead) {
		t.Fatalf("修订史读不动 err = %v", err)
	}
	catalogue.err = errGoodsRead
	if _, err := handler.ListGoods(t.Context(), tenant, owner, 20, cataloguepage.Query{}); !errors.Is(err, errGoodsRead) {
		t.Fatalf("列表读不动 err = %v", err)
	}
}
