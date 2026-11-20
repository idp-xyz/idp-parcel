package postgres_test

import (
	"net/url"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func TestGoodsCatalogueListsCurrentRevisionsAndHistory(t *testing.T) {
	stores := newGoodsStores(t)
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	withoutAlias := func(spec *domain.GoodsContentSpec) { spec.Aliases = nil }
	saveGoods(t, stores, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-B", 1, "REQ-B", withoutAlias))
	saveGoods(t, stores, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-A", 1, "REQ-A1", withoutAlias))
	saveGoods(t, stores, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-A", 2, "REQ-A2", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = nil
		spec.Name = valueOf(t, domain.NewGoodsName, "第二笔")
	}))
	saveGoods(t, stores, goodsRevisionFixture(t, "SYN-OWNER-02", "SYN-A", 1, "REQ-OTHER", withoutAlias))

	query, err := ports.GoodsPages.Decode(url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := stores.catalogue.ListGoods(t.Context(), tenant, owner, 10, query)
	if err != nil || page.Total != 2 || len(page.Rows) != 2 || page.Rows[0].GoodsCode != "SYN-A" || page.Rows[0].Revision != 2 || page.Rows[1].GoodsCode != "SYN-B" {
		t.Fatalf("列表 = %+v，%v", page, err)
	}
	history, err := stores.catalogue.ListGoodsRevisions(t.Context(), tenant, owner, valueOf(t, domain.NewGoodsCode, "SYN-A"))
	if err != nil || len(history) != 2 || history[0].Revision != 1 || history[1].Name != "第二笔" || history[1].SubmissionPath != "OWNER" {
		t.Fatalf("修订史 = %+v，%v", history, err)
	}
	current, found, err := stores.catalogue.GetCurrentGoods(t.Context(), tenant, owner, valueOf(t, domain.NewGoodsCode, "SYN-A"))
	if err != nil || !found || current.Revision != 2 {
		t.Fatalf("当前修订 = %+v %v %v", current, found, err)
	}
	if _, found, err := stores.catalogue.GetCurrentGoods(t.Context(), tenant, valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02"), valueOf(t, domain.NewGoodsCode, "SYN-B")); err != nil || found {
		t.Fatalf("别的货主不该读到 SYN-B：%v %v", found, err)
	}
}

func TestGoodsCatalogueFindsByGoodsCodeOrACurrentlyValidAlias(t *testing.T) {
	stores := newGoodsStores(t)
	aliasFrom := baseInstant.Add(time.Minute)
	aliasTo := baseInstant.Add(30 * 24 * time.Hour)
	saveGoods(t, stores, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{domain.NewGoodsAlias(
			valueOf(t, domain.NewGoodsAliasCode, "SYN-BAR-01"), domain.GoodsAliasBarcode, domain.PlatformName{},
			domain.NewAliasValidity(aliasFrom, aliasTo),
		)}
	}))
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	find := func(code string, at time.Time) (ports.GoodsRow, bool) {
		t.Helper()
		row, found, err := stores.catalogue.FindGoodsByCode(t.Context(), tenant, owner, valueOf(t, domain.NewIdentifierCode, code), at)
		if err != nil {
			t.Fatalf("按码找：%v", err)
		}
		return row, found
	}
	// 登记之前没有生效修订：商品编码与别名都找不到。
	if _, found := find("SYN-GOODS-01", baseInstant.Add(-time.Minute)); found {
		t.Fatal("登记之前不该命中")
	}
	if row, found := find("SYN-GOODS-01", aliasFrom); !found || row.GoodsCode != "SYN-GOODS-01" || row.Revision != 1 {
		t.Fatalf("商品编码应命中生效修订：%+v %v", row, found)
	}
	if _, found := find("SYN-BAR-01", aliasFrom); !found {
		t.Fatal("区间起应命中")
	}
	if _, found := find("SYN-BAR-01", aliasTo); found {
		t.Fatal("区间止不应命中")
	}
	if _, found, err := stores.catalogue.FindGoodsByCode(t.Context(), tenant, valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02"), valueOf(t, domain.NewIdentifierCode, "SYN-GOODS-01"), aliasFrom); err != nil || found {
		t.Fatalf("别的货主不该按这个码找到：%v %v", found, err)
	}
}

// Covers: 离线事实的识别码按发生时刻生效的修订解析——旧修订上的别名在新修订删掉之后，发生时刻仍落在旧修订上时还能认到。
func TestGoodsCatalogueResolvesAliasFromTheRevisionInForceAtTheInstant(t *testing.T) {
	stores := newGoodsStores(t)
	saveGoods(t, stores, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{domain.NewGoodsAlias(
			valueOf(t, domain.NewGoodsAliasCode, "SYN-OLD-BAR"), domain.GoodsAliasBarcode, domain.PlatformName{},
			domain.NewAliasValidity(baseInstant, time.Time{}),
		)}
	}))
	saveGoods(t, stores, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 2, "REQ-2", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = nil
		spec.Name = valueOf(t, domain.NewGoodsName, "第二笔")
	}))
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	between := baseInstant.Add(90 * time.Second)
	row, found, err := stores.catalogue.FindGoodsByCode(t.Context(), tenant, owner, valueOf(t, domain.NewIdentifierCode, "SYN-OLD-BAR"), between)
	if err != nil || !found || row.Revision != 1 || row.GoodsCode != "SYN-GOODS-01" {
		t.Fatalf("旧修订上的别名：%+v %v %v", row, found, err)
	}
	after := baseInstant.Add(3 * time.Minute)
	if _, found, err := stores.catalogue.FindGoodsByCode(t.Context(), tenant, owner, valueOf(t, domain.NewIdentifierCode, "SYN-OLD-BAR"), after); err != nil || found {
		t.Fatalf("新修订已经删掉别名：%v %v", found, err)
	}
}
