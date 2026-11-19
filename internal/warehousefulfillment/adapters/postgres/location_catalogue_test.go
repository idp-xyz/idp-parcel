package postgres_test

import (
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件证库位册的读面：各库位的当前修订列表（分页、按仓编码、库区编码与是否停用筛、q）与一个库位的修订史。

// Covers: 票面「库位」查阅与 AT-WF-009（库位的部分）——列表只给各库位的当前修订（每个库位取最新那一笔，旧修订不出现），行上带当前修订的
// 全部格（没带的库位类型与优先级是 nil，不是空串或 0）；按仓编码、库区编码、是否停用筛得出，停用与再启用各是一笔新修订，再启用之后该库位
// 回到「未停用」；total 与本页同一组条件；缺省先按仓、再按库位编码排；别的租户的库位不出现。
func TestLocationCatalogueListsOnlyTheCurrentRevisionOfEachLocation(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")

	mustSaveLocations(t, stores,
		// 一号仓 A 位：登记、改优先级、停用——当前修订是 3，已停用，带类型与优先级 4。
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-A", 1, combined(inZoneOf(t, "SYN-ZONE-01"), typedAs(t, "SYN-LT-01"), prioritisedAs(1))),
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-A", 2, combined(inZoneOf(t, "SYN-ZONE-01"), typedAs(t, "SYN-LT-01"), prioritisedAs(4))),
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-A", 3, combined(inZoneOf(t, "SYN-ZONE-01"), typedAs(t, "SYN-LT-01"), prioritisedAs(4), deactivatedLocation)),
		// 一号仓 B 位：只有修订 1，在另一个库区，没带类型与优先级。
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-B", 1, inZoneOf(t, "SYN-ZONE-02")),
		// 二号仓 A 位：同一个库位编码，另一个仓；停用之后再启用——当前修订是 3，未停用，优先级 0。
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-LOC-A", 1, inZoneOf(t, "SYN-ZONE-01")),
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-LOC-A", 2, combined(inZoneOf(t, "SYN-ZONE-01"), deactivatedLocation)),
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-LOC-A", 3, combined(inZoneOf(t, "SYN-ZONE-01"), prioritisedAs(0))),
		// 别的租户的库位不出现在本租户的列表里。
		locationFixture(t, "SYN-TENANT-02", "SYN-WH-01", "SYN-LOC-X", 1, nil),
	)

	all := []string{"SYN-WH-01/SYN-LOC-A", "SYN-WH-01/SYN-LOC-B", "SYN-WH-02/SYN-LOC-A"}
	for query, want := range map[string][]string{
		"":                        all,
		"warehouseCode=SYN-WH-01": {"SYN-WH-01/SYN-LOC-A", "SYN-WH-01/SYN-LOC-B"},
		"warehouseCode=SYN-WH-02": {"SYN-WH-02/SYN-LOC-A"},
		"warehouseCode=syn-wh-01": {},
		"warehouseCode=SYN-WH-01&warehouseCode=SYN-WH-02": all,
		"warehouseCode=SYN-WH-NEVER":                      {},
		"zoneCode=SYN-ZONE-01":                            {"SYN-WH-01/SYN-LOC-A", "SYN-WH-02/SYN-LOC-A"},
		"zoneCode=SYN-ZONE-02":                            {"SYN-WH-01/SYN-LOC-B"},
		"zoneCode=syn-zone-01":                            {},
		"zoneCode=SYN-ZONE-01&zoneCode=SYN-ZONE-02":       all,
		"warehouseCode=SYN-WH-01&zoneCode=SYN-ZONE-01":    {"SYN-WH-01/SYN-LOC-A"},
		"warehouseCode=SYN-WH-02&zoneCode=SYN-ZONE-02":    {},
		"deactivated=true":                                {"SYN-WH-01/SYN-LOC-A"},
		"deactivated=false":                               {"SYN-WH-01/SYN-LOC-B", "SYN-WH-02/SYN-LOC-A"},
		"deactivated=true&deactivated=false":              all,
		"warehouseCode=SYN-WH-01&deactivated=false":       {"SYN-WH-01/SYN-LOC-B"},
		"zoneCode=SYN-ZONE-01&deactivated=false":          {"SYN-WH-02/SYN-LOC-A"},
		"q=loc-b":                                         {"SYN-WH-01/SYN-LOC-B"},
		"q=zone-02":                                       {"SYN-WH-01/SYN-LOC-B"},
		"q=wh-02":                                         {"SYN-WH-02/SYN-LOC-A"},
		"q=%25":                                           {},
		"sort=-warehouseCode":                             {"SYN-WH-02/SYN-LOC-A", "SYN-WH-01/SYN-LOC-B", "SYN-WH-01/SYN-LOC-A"},
	} {
		values, err := url.ParseQuery(query)
		if err != nil {
			t.Fatalf("parse %q: %v", query, err)
		}
		page, err := stores.catalogue.ListLocations(ctx, tenant, 50, locationFirstPage(t, values))
		if err != nil {
			t.Fatalf("列表 %q：%v", query, err)
		}
		if got := locationKeysOf(page.Rows); !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Fatalf("列表 %q = %v，want %v", query, got, want)
		}
		if page.Total != int64(len(want)) || page.Next != "" {
			t.Fatalf("列表 %q 的总数 = %d（next %q），want %d 且与本页同一组条件", query, page.Total, page.Next, len(want))
		}
	}

	page, err := stores.catalogue.ListLocations(ctx, tenant, 50, locationFirstPage(t, nil))
	if err != nil {
		t.Fatalf("列表：%v", err)
	}
	oneA, oneB, twoA := page.Rows[0], page.Rows[1], page.Rows[2]
	if oneA.Revision != 3 || !oneA.Deactivated || oneA.ZoneCode != "SYN-ZONE-01" || oneA.LocationTypeCode == nil || *oneA.LocationTypeCode != "SYN-LT-01" ||
		oneA.Priority == nil || *oneA.Priority != 4 || oneA.Registrant != "operator-1" || !oneA.RegisteredAt.Equal(baseInstant.Add(3*time.Minute)) {
		t.Fatalf("一号仓 A 位的行 = %+v，want 当前修订 3 的各格", oneA)
	}
	if oneB.Revision != 1 || oneB.ZoneCode != "SYN-ZONE-02" || oneB.LocationTypeCode != nil || oneB.Priority != nil || oneB.Deactivated {
		t.Fatalf("一号仓 B 位的行 = %+v，want 修订 1、没有库位类型与优先级", oneB)
	}
	if twoA.Revision != 3 || twoA.Deactivated || twoA.LocationTypeCode != nil || twoA.Priority == nil || *twoA.Priority != 0 {
		t.Fatalf("二号仓 A 位的行 = %+v，want 再启用之后的修订 3、未停用、没有库位类型、优先级 0（给了，是 0）", twoA)
	}
}

// Covers: ADR-0144 完成判据「翻页期间登记新行，已翻过的页不重出、未翻的页不漏行」——按（仓，库位编码）排、每页两行，行标识是两段；
// 第一页之后登记一个排在已翻过那一头的库位，第二页接着第一页末行往后、总数跟着变，末页取尽、next 为空。
func TestLocationCataloguePagesInWarehouseThenLocationOrderWithoutRepeatsOrGapsWhileRowsArrive(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	key := func(warehouse, serial int) (string, string) {
		return fmt.Sprintf("SYN-WH-%02d", warehouse), fmt.Sprintf("SYN-LOC-%02d", serial)
	}
	save := func(warehouse, serial int) {
		wh, location := key(warehouse, serial)
		mustSaveLocations(t, stores, locationFixture(t, "SYN-TENANT-01", wh, location, 1, nil))
	}
	label := func(warehouse, serial int) string {
		wh, location := key(warehouse, serial)
		return wh + "/" + location
	}
	// 两个仓、每仓三个库位：仓内编码相同的库位在两个仓里各有一份，行标识的第二段不足以区分，要靠两段合起来。
	for _, warehouse := range []int{1, 2} {
		for _, serial := range []int{1, 2, 3} {
			save(warehouse, serial)
		}
	}

	first, err := stores.catalogue.ListLocations(ctx, tenant, 2, locationFirstPage(t, nil))
	if err != nil {
		t.Fatalf("第一页：%v", err)
	}
	if got := locationKeysOf(first.Rows); !slices.Equal(got, []string{label(1, 1), label(1, 2)}) || first.Total != 6 || first.Next == "" {
		t.Fatalf("第一页 = %v（共 %d，next %q），want %s,%s、共 6、有下一页", got, first.Total, first.Next, label(1, 1), label(1, 2))
	}

	// 新登的排在已翻过的那头（仓 01 的库位 00 < 01）。偏移分页会把第一页末行挤进第二页开头。
	save(1, 0)

	second, err := stores.catalogue.ListLocations(ctx, tenant, 2, locationFirstPage(t, url.Values{"after": {first.Next}}))
	if err != nil {
		t.Fatalf("第二页：%v", err)
	}
	if got := locationKeysOf(second.Rows); !slices.Equal(got, []string{label(1, 3), label(2, 1)}) || second.Total != 7 || second.Next == "" {
		t.Fatalf("第二页 = %v（共 %d），want %s,%s、总数跟着新登的行变成 7", got, second.Total, label(1, 3), label(2, 1))
	}
	third, err := stores.catalogue.ListLocations(ctx, tenant, 2, locationFirstPage(t, url.Values{"after": {second.Next}}))
	if err != nil {
		t.Fatalf("第三页：%v", err)
	}
	if got := locationKeysOf(third.Rows); !slices.Equal(got, []string{label(2, 2), label(2, 3)}) || third.Total != 7 || third.Next != "" {
		t.Fatalf("末页 = %v（共 %d，next %q），want %s,%s 且 next 为空", got, third.Total, third.Next, label(2, 2), label(2, 3))
	}
}

// Covers: AT-WF-009（库位的部分）——一个仓都没登记的租户，列表答空页（总数 0、没有下一页），不答未配置，也没有默认库位；
// 页大小非正在读口上拒绝，不静默答一页没人决定过的页大小。
func TestLocationCatalogueOfAnEmptyRegisterAnswersAnEmptyPage(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-EMPTY")

	page, err := stores.catalogue.ListLocations(ctx, tenant, 50, locationFirstPage(t, nil))
	if err != nil || len(page.Rows) != 0 || page.Total != 0 || page.Next != "" {
		t.Fatalf("空册 = (%d 行、共 %d、next %q, %v)，want 空页", len(page.Rows), page.Total, page.Next, err)
	}
	history, err := stores.catalogue.ListLocationRevisions(ctx, tenant,
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), valueOf(t, domain.NewLocationCode, "SYN-LOC-01"))
	if err != nil || history == nil || len(history) != 0 {
		t.Fatalf("空册的修订史 = (%#v, %v)，want 空切片、无错", history, err)
	}
	for _, limit := range []int{0, -1} {
		if _, err := stores.catalogue.ListLocations(ctx, tenant, limit, locationFirstPage(t, nil)); err == nil {
			t.Errorf("页大小 %d 应被拒", limit)
		}
	}
}

// Covers: 查阅「一个库位的修订史」——全部修订按修订号升序，与落册先后无关；每一笔带当时的登记责任方、登记时刻与那一笔的库位类型和优先级，
// 停用与再启用各是一笔，谁在何时作的看得出；别的库位（含别的仓下的同编码库位）的修订不混进来。
func TestLocationCatalogueHistoryIsOrderedByRevisionNumberAndShowsWhoAndWhen(t *testing.T) {
	stores := newLocationStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewLocationCode, "SYN-LOC-01")

	// 故意乱序落册：登记册本身只守主键，连续性归登记用例判；修订史必须按修订号排，不能按落册先后排。
	byOperator := func(revision domain.LocationRevision, registrant string) domain.LocationRevision {
		return locationRegisteredBy(t, revision, registrant, revision.RegisteredAt())
	}
	mustSaveLocations(t, stores,
		byOperator(locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-01", 2, combined(typedAs(t, "SYN-LT-01"), deactivatedLocation)), "operator-2"),
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-01", 1, typedAs(t, "SYN-LT-01")),
		byOperator(locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-01", 3, prioritisedAs(8)), "operator-3"),
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-02", 1, nil),
		locationFixture(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-LOC-01", 1, nil),
	)

	history, err := stores.catalogue.ListLocationRevisions(ctx, tenant, warehouse, code)
	if err != nil || len(history) != 3 {
		t.Fatalf("修订史 = (%d 笔, %v)，want 3 笔", len(history), err)
	}
	for index, want := range []struct {
		revision     int
		deactivated  bool
		registrant   string
		hasType      bool
		wantPriority *int32
	}{{1, false, "operator-1", true, nil}, {2, true, "operator-2", true, nil}, {3, false, "operator-3", false, ptrTo(8)}} {
		row := history[index]
		if row.WarehouseCode != "SYN-WH-01" || row.LocationCode != "SYN-LOC-01" || row.Revision != want.revision ||
			row.Deactivated != want.deactivated || row.Registrant != want.registrant || (row.LocationTypeCode != nil) != want.hasType ||
			(row.Priority == nil) != (want.wantPriority == nil) || (row.Priority != nil && *row.Priority != *want.wantPriority) ||
			!row.RegisteredAt.Equal(baseInstant.Add(time.Duration(want.revision)*time.Minute)) {
			t.Fatalf("修订史第 %d 笔 = %+v，want 修订 %d、停用 %v、登记责任方 %s、带类型 %v、优先级 %v",
				index+1, row, want.revision, want.deactivated, want.registrant, want.hasType, want.wantPriority)
		}
	}
}
