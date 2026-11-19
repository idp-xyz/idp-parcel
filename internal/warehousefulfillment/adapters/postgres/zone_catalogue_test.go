package postgres_test

import (
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件证库区册的读面：各库区的当前修订列表（分页、按仓编码与是否停用筛、q）与一个库区的修订史。

// Covers: 票面「库区」查阅与 AT-WF-009（库区的部分）——列表只给各库区的当前修订（每个库区取最新那一笔，旧修订不出现），行上带当前修订的
// 全部格；按仓编码、是否停用筛得出，停用与再启用各是一笔新修订，再启用之后该库区回到「未停用」；total 与本页同一组条件；缺省先按仓、
// 再按库区编码排；别的租户的库区不出现。
func TestZoneCatalogueListsOnlyTheCurrentRevisionOfEachZone(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	deactivate := func(spec *domain.ZoneContentSpec) { spec.Deactivated = true }

	mustSaveZones(t, stores,
		// 一号仓 A 区：登记、改名、停用——当前修订是 3，已停用。
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-A", 1, nil),
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-A", 2, func(spec *domain.ZoneContentSpec) {
			spec.Name = valueOf(t, domain.NewZoneName, "合成存储区（二）")
		}),
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-A", 3, func(spec *domain.ZoneContentSpec) {
			deactivate(spec)
			spec.Name = valueOf(t, domain.NewZoneName, "合成存储区（二）")
		}),
		// 一号仓 B 区：只有修订 1，拣选区。
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-B", 1, func(spec *domain.ZoneContentSpec) {
			spec.Usage = domain.ZoneUsagePicking
		}),
		// 二号仓 A 区：同一个库区编码，另一个仓；停用之后再启用——当前修订是 3，未停用。
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-ZONE-A", 1, func(spec *domain.ZoneContentSpec) {
			spec.Usage = domain.ZoneUsageReceivingStaging
		}),
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-ZONE-A", 2, func(spec *domain.ZoneContentSpec) {
			spec.Usage = domain.ZoneUsageReceivingStaging
			deactivate(spec)
		}),
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-ZONE-A", 3, func(spec *domain.ZoneContentSpec) {
			spec.Usage = domain.ZoneUsageReceivingStaging
		}),
		// 别的租户的库区不出现在本租户的列表里。
		zoneFixture(t, "SYN-TENANT-02", "SYN-WH-01", "SYN-ZONE-X", 1, nil),
	)

	all := []string{"SYN-WH-01/SYN-ZONE-A", "SYN-WH-01/SYN-ZONE-B", "SYN-WH-02/SYN-ZONE-A"}
	for query, want := range map[string][]string{
		"":                        all,
		"warehouseCode=SYN-WH-01": {"SYN-WH-01/SYN-ZONE-A", "SYN-WH-01/SYN-ZONE-B"},
		"warehouseCode=SYN-WH-02": {"SYN-WH-02/SYN-ZONE-A"},
		"warehouseCode=syn-wh-01": {},
		"warehouseCode=SYN-WH-01&warehouseCode=SYN-WH-02": all,
		"warehouseCode=SYN-WH-NEVER":                      {},
		"deactivated=true":                                {"SYN-WH-01/SYN-ZONE-A"},
		"deactivated=false":                               {"SYN-WH-01/SYN-ZONE-B", "SYN-WH-02/SYN-ZONE-A"},
		"deactivated=true&deactivated=false":              all,
		"warehouseCode=SYN-WH-01&deactivated=false":       {"SYN-WH-01/SYN-ZONE-B"},
		"q=zone-b":            {"SYN-WH-01/SYN-ZONE-B"},
		"q=合成存储区（二）":          {"SYN-WH-01/SYN-ZONE-A"},
		"q=wh-02":             {"SYN-WH-02/SYN-ZONE-A"},
		"q=%25":               {},
		"sort=-warehouseCode": {"SYN-WH-02/SYN-ZONE-A", "SYN-WH-01/SYN-ZONE-B", "SYN-WH-01/SYN-ZONE-A"},
	} {
		values, err := url.ParseQuery(query)
		if err != nil {
			t.Fatalf("parse %q: %v", query, err)
		}
		page, err := stores.catalogue.ListZones(ctx, tenant, 50, zoneFirstPage(t, values))
		if err != nil {
			t.Fatalf("列表 %q：%v", query, err)
		}
		if got := zoneKeysOf(page.Rows); !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Fatalf("列表 %q = %v，want %v", query, got, want)
		}
		if page.Total != int64(len(want)) || page.Next != "" {
			t.Fatalf("列表 %q 的总数 = %d（next %q），want %d 且与本页同一组条件", query, page.Total, page.Next, len(want))
		}
	}

	page, err := stores.catalogue.ListZones(ctx, tenant, 50, zoneFirstPage(t, nil))
	if err != nil {
		t.Fatalf("列表：%v", err)
	}
	oneA, oneB, twoA := page.Rows[0], page.Rows[1], page.Rows[2]
	if oneA.Revision != 3 || !oneA.Deactivated || oneA.Name != "合成存储区（二）" || oneA.Usage != "STORAGE" ||
		oneA.Registrant != "operator-1" || !oneA.RegisteredAt.Equal(baseInstant.Add(3*time.Minute)) {
		t.Fatalf("一号仓 A 区的行 = %+v，want 当前修订 3 的各格", oneA)
	}
	if oneB.Revision != 1 || oneB.Usage != "PICKING" || oneB.Deactivated {
		t.Fatalf("一号仓 B 区的行 = %+v，want 修订 1、拣选", oneB)
	}
	if twoA.Revision != 3 || twoA.Deactivated || twoA.Usage != "RECEIVING_STAGING" {
		t.Fatalf("二号仓 A 区的行 = %+v，want 再启用之后的修订 3、未停用", twoA)
	}
}

// Covers: ADR-0144 完成判据「翻页期间登记新行，已翻过的页不重出、未翻的页不漏行」——按（仓，库区编码）排、每页两行，行标识是两段；
// 第一页之后登记一个排在已翻过那一头的库区，第二页接着第一页末行往后、总数跟着变，末页取尽、next 为空。
func TestZoneCataloguePagesInWarehouseThenZoneOrderWithoutRepeatsOrGapsWhileRowsArrive(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	key := func(warehouse, serial int) (string, string) {
		return fmt.Sprintf("SYN-WH-%02d", warehouse), fmt.Sprintf("SYN-ZONE-%02d", serial)
	}
	save := func(warehouse, serial int) {
		wh, zone := key(warehouse, serial)
		mustSaveZones(t, stores, zoneFixture(t, "SYN-TENANT-01", wh, zone, 1, nil))
	}
	label := func(warehouse, serial int) string {
		wh, zone := key(warehouse, serial)
		return wh + "/" + zone
	}
	// 两个仓、每仓三个库区：仓内编码相同的库区在两个仓里各有一份，行标识的第二段不足以区分，要靠两段合起来。
	for _, warehouse := range []int{1, 2} {
		for _, serial := range []int{1, 2, 3} {
			save(warehouse, serial)
		}
	}

	first, err := stores.catalogue.ListZones(ctx, tenant, 2, zoneFirstPage(t, nil))
	if err != nil {
		t.Fatalf("第一页：%v", err)
	}
	if got := zoneKeysOf(first.Rows); !slices.Equal(got, []string{label(1, 1), label(1, 2)}) || first.Total != 6 || first.Next == "" {
		t.Fatalf("第一页 = %v（共 %d，next %q），want %s,%s、共 6、有下一页", got, first.Total, first.Next, label(1, 1), label(1, 2))
	}

	// 新登的排在已翻过的那头（仓 01 的库区 00 < 01）。偏移分页会把第一页末行挤进第二页开头。
	save(1, 0)

	second, err := stores.catalogue.ListZones(ctx, tenant, 2, zoneFirstPage(t, url.Values{"after": {first.Next}}))
	if err != nil {
		t.Fatalf("第二页：%v", err)
	}
	if got := zoneKeysOf(second.Rows); !slices.Equal(got, []string{label(1, 3), label(2, 1)}) || second.Total != 7 || second.Next == "" {
		t.Fatalf("第二页 = %v（共 %d），want %s,%s、总数跟着新登的行变成 7", got, second.Total, label(1, 3), label(2, 1))
	}
	third, err := stores.catalogue.ListZones(ctx, tenant, 2, zoneFirstPage(t, url.Values{"after": {second.Next}}))
	if err != nil {
		t.Fatalf("第三页：%v", err)
	}
	if got := zoneKeysOf(third.Rows); !slices.Equal(got, []string{label(2, 2), label(2, 3)}) || third.Total != 7 || third.Next != "" {
		t.Fatalf("末页 = %v（共 %d，next %q），want %s,%s 且 next 为空", got, third.Total, third.Next, label(2, 2), label(2, 3))
	}
}

// Covers: AT-WF-009（库区的部分）——一个仓都没登记的租户，列表答空页（总数 0、没有下一页），不答未配置，也没有默认库区；
// 页大小非正在读口上拒绝，不静默答一页没人决定过的页大小。
func TestZoneCatalogueOfAnEmptyRegisterAnswersAnEmptyPage(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-EMPTY")

	page, err := stores.catalogue.ListZones(ctx, tenant, 50, zoneFirstPage(t, nil))
	if err != nil || len(page.Rows) != 0 || page.Total != 0 || page.Next != "" {
		t.Fatalf("空册 = (%d 行、共 %d、next %q, %v)，want 空页", len(page.Rows), page.Total, page.Next, err)
	}
	history, err := stores.catalogue.ListZoneRevisions(ctx, tenant,
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), valueOf(t, domain.NewZoneCode, "SYN-ZONE-01"))
	if err != nil || history == nil || len(history) != 0 {
		t.Fatalf("空册的修订史 = (%#v, %v)，want 空切片、无错", history, err)
	}
	for _, limit := range []int{0, -1} {
		if _, err := stores.catalogue.ListZones(ctx, tenant, limit, zoneFirstPage(t, nil)); err == nil {
			t.Errorf("页大小 %d 应被拒", limit)
		}
	}
}

// Covers: 查阅「一个库区的修订史」——全部修订按修订号升序，与落册先后无关；每一笔带当时的登记责任方与登记时刻，停用与再启用各是一笔，
// 谁在何时作的看得出；别的库区（含别的仓下的同编码库区）的修订不混进来。
func TestZoneCatalogueHistoryIsOrderedByRevisionNumberAndShowsWhoAndWhen(t *testing.T) {
	stores := newZoneStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewZoneCode, "SYN-ZONE-01")
	deactivate := func(spec *domain.ZoneContentSpec) { spec.Deactivated = true }

	// 故意乱序落册：登记册本身只守主键，连续性归登记用例判；修订史必须按修订号排，不能按落册先后排。
	byOperator := func(revision domain.ZoneRevision, registrant string) domain.ZoneRevision {
		return zoneRegisteredBy(t, revision, registrant, revision.RegisteredAt())
	}
	mustSaveZones(t, stores,
		byOperator(zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 2, deactivate), "operator-2"),
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 1, nil),
		byOperator(zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 3, nil), "operator-3"),
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-02", 1, nil),
		zoneFixture(t, "SYN-TENANT-01", "SYN-WH-02", "SYN-ZONE-01", 1, nil),
	)

	history, err := stores.catalogue.ListZoneRevisions(ctx, tenant, warehouse, code)
	if err != nil || len(history) != 3 {
		t.Fatalf("修订史 = (%d 笔, %v)，want 3 笔", len(history), err)
	}
	for index, want := range []struct {
		revision    int
		deactivated bool
		registrant  string
	}{{1, false, "operator-1"}, {2, true, "operator-2"}, {3, false, "operator-3"}} {
		row := history[index]
		if row.WarehouseCode != "SYN-WH-01" || row.ZoneCode != "SYN-ZONE-01" || row.Revision != want.revision ||
			row.Deactivated != want.deactivated || row.Registrant != want.registrant ||
			!row.RegisteredAt.Equal(baseInstant.Add(time.Duration(want.revision)*time.Minute)) {
			t.Fatalf("修订史第 %d 笔 = %+v，want 修订 %d、停用 %v、登记责任方 %s", index+1, row, want.revision, want.deactivated, want.registrant)
		}
	}
}
