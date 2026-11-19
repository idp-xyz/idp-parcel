package postgres_test

import (
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件证库位类型册的读面：各库位类型的当前修订列表（分页、按是否停用筛、q）与一个库位类型的修订史。

// Covers: 票面「库位类型」查阅与 AT-WF-009（库位类型的部分）——列表只给各库位类型的当前修订（每个库位类型取最新那一笔，旧修订不出现），行上带
// 当前修订的全部格；按是否停用筛得出，停用与再启用各是一笔新修订，再启用之后该库位类型回到「未停用」；total 与本页同一组条件；缺省按编码排；
// 别的租户的库位类型不出现。
func TestLocationTypeCatalogueListsOnlyTheCurrentRevisionOfEachType(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	deactivate := func(spec *domain.LocationTypeContentSpec) { spec.Deactivated = true }

	mustSaveLocationTypes(t, stores,
		// A 型：登记、改名、停用——当前修订是 3，已停用。
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-A", 1, nil),
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-A", 2, func(spec *domain.LocationTypeContentSpec) {
			spec.Name = valueOf(t, domain.NewLocationTypeName, "合成货架位（二）")
		}),
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-A", 3, func(spec *domain.LocationTypeContentSpec) {
			deactivate(spec)
			spec.Name = valueOf(t, domain.NewLocationTypeName, "合成货架位（二）")
		}),
		// B 型：只有修订 1。
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-B", 1, func(spec *domain.LocationTypeContentSpec) {
			spec.Name = valueOf(t, domain.NewLocationTypeName, "合成托盘位")
		}),
		// C 型：停用之后再启用——当前修订是 3，未停用。
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-C", 1, nil),
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-C", 2, deactivate),
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-C", 3, nil),
		// 别的租户的库位类型不出现在本租户的列表里。
		locationTypeFixture(t, "SYN-TENANT-02", "SYN-LT-X", 1, nil),
	)

	all := []string{"SYN-LT-A", "SYN-LT-B", "SYN-LT-C"}
	for query, want := range map[string][]string{
		"":                                   all,
		"deactivated=true":                   {"SYN-LT-A"},
		"deactivated=false":                  {"SYN-LT-B", "SYN-LT-C"},
		"deactivated=true&deactivated=false": all,
		"q=lt-b":                             {"SYN-LT-B"},
		"q=托盘":                               {"SYN-LT-B"},
		"q=合成货架位（二）":                         {"SYN-LT-A"},
		"q=%25":                              {},
		"sort=-locationTypeCode":             {"SYN-LT-C", "SYN-LT-B", "SYN-LT-A"},
	} {
		values, err := url.ParseQuery(query)
		if err != nil {
			t.Fatalf("parse %q: %v", query, err)
		}
		page, err := stores.catalogue.ListLocationTypes(ctx, tenant, 50, locationTypeFirstPage(t, values))
		if err != nil {
			t.Fatalf("列表 %q：%v", query, err)
		}
		if got := locationTypeCodesOf(page.Rows); !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Fatalf("列表 %q = %v，want %v", query, got, want)
		}
		if page.Total != int64(len(want)) || page.Next != "" {
			t.Fatalf("列表 %q 的总数 = %d（next %q），want %d 且与本页同一组条件", query, page.Total, page.Next, len(want))
		}
	}

	page, err := stores.catalogue.ListLocationTypes(ctx, tenant, 50, locationTypeFirstPage(t, nil))
	if err != nil {
		t.Fatalf("列表：%v", err)
	}
	typeA, typeB, typeC := page.Rows[0], page.Rows[1], page.Rows[2]
	if typeA.Revision != 3 || !typeA.Deactivated || typeA.Name != "合成货架位（二）" ||
		typeA.Registrant != "operator-1" || !typeA.RegisteredAt.Equal(baseInstant.Add(3*time.Minute)) {
		t.Fatalf("A 型的行 = %+v，want 当前修订 3 的各格", typeA)
	}
	if typeB.Revision != 1 || typeB.Name != "合成托盘位" || typeB.Deactivated {
		t.Fatalf("B 型的行 = %+v，want 修订 1", typeB)
	}
	if typeC.Revision != 3 || typeC.Deactivated {
		t.Fatalf("C 型的行 = %+v，want 再启用之后的修订 3、未停用", typeC)
	}
}

// Covers: ADR-0144 完成判据「翻页期间登记新行，已翻过的页不重出、未翻的页不漏行」——按编码排、每页两行；第一页之后登记一个排在已翻过那一头的
// 库位类型，第二页接着第一页末行往后、总数跟着变，末页取尽、next 为空。
func TestLocationTypeCataloguePagesInCodeOrderWithoutRepeatsOrGapsWhileRowsArrive(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := func(serial int) string { return fmt.Sprintf("SYN-LT-%02d", serial) }
	for _, serial := range []int{1, 2, 3, 4, 5} {
		mustSaveLocationTypes(t, stores, locationTypeFixture(t, "SYN-TENANT-01", code(serial), 1, nil))
	}

	first, err := stores.catalogue.ListLocationTypes(ctx, tenant, 2, locationTypeFirstPage(t, nil))
	if err != nil {
		t.Fatalf("第一页：%v", err)
	}
	if got := locationTypeCodesOf(first.Rows); !slices.Equal(got, []string{code(1), code(2)}) || first.Total != 5 || first.Next == "" {
		t.Fatalf("第一页 = %v（共 %d，next %q），want %s,%s、共 5、有下一页", got, first.Total, first.Next, code(1), code(2))
	}

	// 新登的排在已翻过的那头（编码 00 < 01）。偏移分页会把第一页末行挤进第二页开头。
	mustSaveLocationTypes(t, stores, locationTypeFixture(t, "SYN-TENANT-01", code(0), 1, nil))

	second, err := stores.catalogue.ListLocationTypes(ctx, tenant, 2, locationTypeFirstPage(t, url.Values{"after": {first.Next}}))
	if err != nil {
		t.Fatalf("第二页：%v", err)
	}
	if got := locationTypeCodesOf(second.Rows); !slices.Equal(got, []string{code(3), code(4)}) || second.Total != 6 || second.Next == "" {
		t.Fatalf("第二页 = %v（共 %d），want %s,%s、总数跟着新登的行变成 6", got, second.Total, code(3), code(4))
	}
	third, err := stores.catalogue.ListLocationTypes(ctx, tenant, 2, locationTypeFirstPage(t, url.Values{"after": {second.Next}}))
	if err != nil {
		t.Fatalf("末页：%v", err)
	}
	if got := locationTypeCodesOf(third.Rows); !slices.Equal(got, []string{code(5)}) || third.Total != 6 || third.Next != "" {
		t.Fatalf("末页 = %v（共 %d，next %q），want %s 且 next 为空", got, third.Total, third.Next, code(5))
	}
}

// Covers: AT-WF-009（库位类型的部分）——一个仓都没登记的租户，列表答空页（总数 0、没有下一页），不答未配置，也没有默认库位类型；
// 页大小非正在读口上拒绝，不静默答一页没人决定过的页大小。
func TestLocationTypeCatalogueOfAnEmptyRegisterAnswersAnEmptyPage(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-EMPTY")

	page, err := stores.catalogue.ListLocationTypes(ctx, tenant, 50, locationTypeFirstPage(t, nil))
	if err != nil || len(page.Rows) != 0 || page.Total != 0 || page.Next != "" {
		t.Fatalf("空册 = (%d 行、共 %d、next %q, %v)，want 空页", len(page.Rows), page.Total, page.Next, err)
	}
	history, err := stores.catalogue.ListLocationTypeRevisions(ctx, tenant, valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01"))
	if err != nil || history == nil || len(history) != 0 {
		t.Fatalf("空册的修订史 = (%#v, %v)，want 空切片、无错", history, err)
	}
	for _, limit := range []int{0, -1} {
		if _, err := stores.catalogue.ListLocationTypes(ctx, tenant, limit, locationTypeFirstPage(t, nil)); err == nil {
			t.Errorf("页大小 %d 应被拒", limit)
		}
	}
}

// Covers: 查阅「一个库位类型的修订史」——全部修订按修订号升序，与落册先后无关；每一笔带当时的登记责任方与登记时刻，停用与再启用各是一笔，
// 谁在何时作的看得出；别的库位类型的修订不混进来。
func TestLocationTypeCatalogueHistoryIsOrderedByRevisionNumberAndShowsWhoAndWhen(t *testing.T) {
	stores := newLocationTypeStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01")
	deactivate := func(spec *domain.LocationTypeContentSpec) { spec.Deactivated = true }

	// 故意乱序落册：登记册本身只守主键，连续性归登记用例判；修订史必须按修订号排，不能按落册先后排。
	byOperator := func(revision domain.LocationTypeRevision, registrant string) domain.LocationTypeRevision {
		return locationTypeRegisteredBy(t, revision, registrant, revision.RegisteredAt())
	}
	mustSaveLocationTypes(t, stores,
		byOperator(locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-01", 2, deactivate), "operator-2"),
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-01", 1, nil),
		byOperator(locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-01", 3, nil), "operator-3"),
		locationTypeFixture(t, "SYN-TENANT-01", "SYN-LT-02", 1, nil),
	)

	history, err := stores.catalogue.ListLocationTypeRevisions(ctx, tenant, code)
	if err != nil || len(history) != 3 {
		t.Fatalf("修订史 = (%d 笔, %v)，want 3 笔", len(history), err)
	}
	for index, want := range []struct {
		revision    int
		deactivated bool
		registrant  string
	}{{1, false, "operator-1"}, {2, true, "operator-2"}, {3, false, "operator-3"}} {
		row := history[index]
		if row.LocationTypeCode != "SYN-LT-01" || row.Revision != want.revision || row.Deactivated != want.deactivated ||
			row.Registrant != want.registrant || !row.RegisteredAt.Equal(baseInstant.Add(time.Duration(want.revision)*time.Minute)) {
			t.Fatalf("修订史第 %d 笔 = %+v，want 修订 %d、停用 %v、登记责任方 %s", index+1, row, want.revision, want.deactivated, want.registrant)
		}
	}
}
