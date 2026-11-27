package postgres_test

import (
	"fmt"
	"net/url"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件证仓册的读面：各仓的当前修订列表（分页、按是否停用筛、q）与一个仓的修订史。

// Covers: 票面「查阅」与 AT-WF-009（仓的部分）——列表只给各仓的当前修订（每个仓取最新那一笔，旧修订不出现），行上带当前修订的
// 全部格；按是否停用筛得出，停用与再启用各是一笔新修订，再启用之后该仓回到「未停用」；total 与本页同一组条件；缺省按仓编码排。
func TestWarehouseCatalogueListsOnlyTheCurrentRevisionOfEachWarehouse(t *testing.T) {
	warehouses, catalogue, transactor, _ := newWarehouseStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	deactivate := func(spec *domain.WarehouseContentSpec) { spec.Deactivated = true }

	mustSave(t, transactor, warehouses,
		// 一号仓：登记、改名、停用——当前修订是 3，已停用。
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, nil),
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 2, func(spec *domain.WarehouseContentSpec) {
			spec.Name = valueOf(t, domain.NewWarehouseName, "合成一号仓（二）")
		}),
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 3, func(spec *domain.WarehouseContentSpec) {
			deactivate(spec)
			spec.Name = valueOf(t, domain.NewWarehouseName, "合成一号仓（二）")
		}),
		// 二号仓：只有修订 1，代理仓，没登记业务时区与同址物流节点。
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-02", 1, func(spec *domain.WarehouseContentSpec) {
			spec.Kind = domain.WarehouseKindAgent
			spec.BusinessTimeZone = domain.BusinessTimeZone{}
			spec.ColocatedNode = domain.ColocatedNode{}
		}),
		// 三号仓：停用之后再启用——当前修订是 3，未停用。
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-03", 1, nil),
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-03", 2, deactivate),
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-03", 3, nil),
		// 别的租户的仓不出现在本租户的列表里。
		revisionFixture(t, "SYN-TENANT-02", "SYN-WH-09", 1, nil),
	)

	for query, want := range map[string][]string{
		"":                                   {"SYN-WH-01", "SYN-WH-02", "SYN-WH-03"},
		"deactivated=true":                   {"SYN-WH-01"},
		"deactivated=false":                  {"SYN-WH-02", "SYN-WH-03"},
		"deactivated=true&deactivated=false": {"SYN-WH-01", "SYN-WH-02", "SYN-WH-03"},
		"q=wh-02":                            {"SYN-WH-02"},
		"q=合成一号仓（二）":                         {"SYN-WH-01"},
		"q=%25":                              {},
	} {
		values, err := url.ParseQuery(query)
		if err != nil {
			t.Fatalf("parse %q: %v", query, err)
		}
		page, err := catalogue.ListWarehouses(ctx, tenant, 50, firstPage(t, values), ports.WarehouseScope{})
		if err != nil {
			t.Fatalf("列表 %q：%v", query, err)
		}
		if got := codesOf(page.Rows); !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Fatalf("列表 %q = %v，want %v", query, got, want)
		}
		if page.Total != int64(len(want)) || page.Next != "" {
			t.Fatalf("列表 %q 的总数 = %d（next %q），want %d 且与本页同一组条件", query, page.Total, page.Next, len(want))
		}
	}

	page, err := catalogue.ListWarehouses(ctx, tenant, 50, firstPage(t, nil), ports.WarehouseScope{})
	if err != nil {
		t.Fatalf("列表：%v", err)
	}
	one, two, three := page.Rows[0], page.Rows[1], page.Rows[2]
	if one.Revision != 3 || !one.Deactivated || one.Name != "合成一号仓（二）" || one.Kind != "SELF_OPERATED" ||
		one.CountryCode != "CN" || one.AddressText != "合成路 1 号" || one.BusinessTimeZone != "Asia/Shanghai" || !one.HasBusinessTimeZone ||
		one.ColocatedNode != "SYN-NODE-01" || !one.HasColocatedNode || one.Registrant != "operator-1" ||
		!one.RegisteredAt.Equal(baseInstant.Add(3*time.Minute)) {
		t.Fatalf("一号仓的行 = %+v，want 当前修订 3 的各格", one)
	}
	if two.Revision != 1 || two.Kind != "AGENT" || two.Deactivated || two.HasBusinessTimeZone || two.HasColocatedNode {
		t.Fatalf("二号仓的行 = %+v，want 修订 1、代理仓、业务时区与节点没登记", two)
	}
	if three.Revision != 3 || three.Deactivated {
		t.Fatalf("三号仓的行 = %+v，want 再启用之后的修订 3、未停用", three)
	}
}

// Covers: ADR-0144 完成判据「翻页期间登记新行，已翻过的页不重出、未翻的页不漏行」——按仓编码排、每页两行；第一页之后登记一个排在
// 已翻过那一头的仓，第二页接着第一页末行往后、总数跟着变，末页取尽、next 为空。
func TestWarehouseCataloguePagesInCodeOrderWithoutRepeatsOrGapsWhileRowsArrive(t *testing.T) {
	warehouses, catalogue, transactor, _ := newWarehouseStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := func(serial int) string { return fmt.Sprintf("SYN-WH-%02d", serial) }
	for serial := 1; serial <= 5; serial++ {
		mustSave(t, transactor, warehouses, revisionFixture(t, "SYN-TENANT-01", code(serial), 1, nil))
	}

	first, err := catalogue.ListWarehouses(ctx, tenant, 2, firstPage(t, nil), ports.WarehouseScope{})
	if err != nil {
		t.Fatalf("第一页：%v", err)
	}
	if got := codesOf(first.Rows); !slices.Equal(got, []string{code(1), code(2)}) || first.Total != 5 || first.Next == "" {
		t.Fatalf("第一页 = %v（共 %d，next %q），want %s,%s、共 5、有下一页", got, first.Total, first.Next, code(1), code(2))
	}

	// 新登的排在已翻过的那头（00 < 01）。偏移分页会把第一页末行挤进第二页开头。
	mustSave(t, transactor, warehouses, revisionFixture(t, "SYN-TENANT-01", code(0), 1, nil))

	second, err := catalogue.ListWarehouses(ctx, tenant, 2, firstPage(t, url.Values{"after": {first.Next}}), ports.WarehouseScope{})
	if err != nil {
		t.Fatalf("第二页：%v", err)
	}
	if got := codesOf(second.Rows); !slices.Equal(got, []string{code(3), code(4)}) || second.Total != 6 || second.Next == "" {
		t.Fatalf("第二页 = %v（共 %d），want %s,%s、总数跟着新登的行变成 6", got, second.Total, code(3), code(4))
	}
	third, err := catalogue.ListWarehouses(ctx, tenant, 2, firstPage(t, url.Values{"after": {second.Next}}), ports.WarehouseScope{})
	if err != nil {
		t.Fatalf("末页：%v", err)
	}
	if got := codesOf(third.Rows); !slices.Equal(got, []string{code(5)}) || third.Total != 6 || third.Next != "" {
		t.Fatalf("末页 = %v（共 %d，next %q），want %s 且 next 为空", got, third.Total, third.Next, code(5))
	}
}

// Covers: AT-WF-009（仓的部分）——一个仓都没登记的租户，列表答空页（总数 0、没有下一页），不答未配置，也没有默认仓；
// 页大小非正在读口上拒绝，不静默答一页没人决定过的页大小。
func TestWarehouseCatalogueOfAnEmptyRegisterAnswersAnEmptyPage(t *testing.T) {
	_, catalogue, _, _ := newWarehouseStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-EMPTY")

	page, err := catalogue.ListWarehouses(ctx, tenant, 50, firstPage(t, nil), ports.WarehouseScope{})
	if err != nil || len(page.Rows) != 0 || page.Total != 0 || page.Next != "" {
		t.Fatalf("空册 = (%d 行、共 %d、next %q, %v)，want 空页", len(page.Rows), page.Total, page.Next, err)
	}
	history, err := catalogue.ListWarehouseRevisions(ctx, tenant, valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"))
	if err != nil || history == nil || len(history) != 0 {
		t.Fatalf("空册的修订史 = (%#v, %v)，want 空切片、无错", history, err)
	}
	for _, limit := range []int{0, -1} {
		if _, err := catalogue.ListWarehouses(ctx, tenant, limit, firstPage(t, nil), ports.WarehouseScope{}); err == nil {
			t.Errorf("页大小 %d 应被拒", limit)
		}
	}
}

// Covers: 查阅「一个仓的修订史」——全部修订按修订号升序，与落册先后无关；每一笔带当时的登记责任方与登记时刻，停用与再启用各是一笔，
// 谁在何时作的看得出；别的仓的修订不混进来。
func TestWarehouseCatalogueHistoryIsOrderedByRevisionNumberAndShowsWhoAndWhen(t *testing.T) {
	warehouses, catalogue, transactor, _ := newWarehouseStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	deactivate := func(spec *domain.WarehouseContentSpec) { spec.Deactivated = true }

	// 故意乱序落册：登记册本身只守主键，连续性归登记用例判；修订史必须按修订号排，不能按落册先后排。
	byOperator := func(revision domain.WarehouseRevision, registrant string) domain.WarehouseRevision {
		return registeredBy(t, revision, registrant, revision.RegisteredAt())
	}
	mustSave(t, transactor, warehouses,
		byOperator(revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 2, deactivate), "operator-2"),
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, nil),
		byOperator(revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 3, nil), "operator-3"),
		revisionFixture(t, "SYN-TENANT-01", "SYN-WH-02", 1, nil),
	)

	history, err := catalogue.ListWarehouseRevisions(ctx, tenant, code)
	if err != nil || len(history) != 3 {
		t.Fatalf("修订史 = (%d 笔, %v)，want 3 笔", len(history), err)
	}
	for index, want := range []struct {
		revision    int
		deactivated bool
		registrant  string
	}{{1, false, "operator-1"}, {2, true, "operator-2"}, {3, false, "operator-3"}} {
		row := history[index]
		if row.WarehouseCode != "SYN-WH-01" || row.Revision != want.revision || row.Deactivated != want.deactivated ||
			row.Registrant != want.registrant || !row.RegisteredAt.Equal(baseInstant.Add(time.Duration(want.revision)*time.Minute)) {
			t.Fatalf("修订史第 %d 笔 = %+v，want 修订 %d、停用 %v、登记责任方 %s", index+1, row, want.revision, want.deactivated, want.registrant)
		}
	}
}
