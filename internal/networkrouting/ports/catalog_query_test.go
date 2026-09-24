package ports_test

import (
	"errors"
	"net/url"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

var familyCatalogues = map[string]struct {
	catalogue   *cataloguepage.Catalogue
	defaultSort string
}{
	"node":                    {ports.NodeVersionCatalogue, "code"},
	"connection":              {ports.ConnectionVersionCatalogue, "code"},
	"line":                    {ports.LineVersionCatalogue, "code"},
	"service-area":            {ports.ServiceAreaVersionCatalogue, "code"},
	"service-calendar":        {ports.ServiceCalendarVersionCatalogue, "targetKind"},
	"availability-adjustment": {ports.AvailabilityAdjustmentCatalogue, "code"},
	"route-strategy":          {ports.RouteStrategyVersionCatalogue, "code"},
}

// Covers: ADR-0144 决定四「已被用作册子选择器的参数保持原义」——各族声明都放行 family，不当成集外键拒；
// 缺省序与头注写的一致（七张表没有登记时间，缺省序取身份升序）。
func TestEachFamilyAcceptsTheFamilySelectorAndSortsByIdentityByDefault(t *testing.T) {
	for family, declared := range familyCatalogues {
		t.Run(family, func(t *testing.T) {
			query, err := declared.catalogue.Decode(url.Values{"family": {family}})
			if err != nil {
				t.Fatalf("family 选择器被拒：%v", err)
			}
			if query.Sort != (cataloguepage.Sort{Field: declared.defaultSort}) {
				t.Fatalf("缺省序：得 %v，应为 %s 升序", query.Sort, declared.defaultSort)
			}
		})
	}
}

// Covers: 各族的册名互不相同——一族的游标拿到另一族去用，翻出来的是另一份列表的中段（ADR-0144 决定一）。
func TestACursorFromOneFamilyIsRefusedByAnother(t *testing.T) {
	node, err := ports.NodeVersionCatalogue.Decode(nil)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := node.CursorAfter(cataloguepage.Position{Value: "SYN-NODE-01", Identity: []string{"SYN-NODE-01", "1"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ports.LineVersionCatalogue.Decode(url.Values{"after": {cursor}})
	var malformed *cataloguepage.MalformedQuery
	if !errors.As(err, &malformed) {
		t.Fatalf("节点族的游标被线路族接受：%v", err)
	}
}

// Covers: 对象种类与调整种类是封闭词表，取值照答复体的写法（枚举的 String），词表外即拒。
func TestKindFiltersFollowTheEnumerations(t *testing.T) {
	for _, value := range []string{"NODE", "CONNECTION", "LINE"} {
		if _, err := ports.ServiceCalendarVersionCatalogue.Decode(url.Values{"targetKind": {value}}); err != nil {
			t.Fatalf("targetKind=%s 被拒：%v", value, err)
		}
	}
	for _, value := range []string{"SUSPENSION", "CLOSURE", "RESUMPTION", "SCOPE_ADJUSTMENT"} {
		if _, err := ports.AvailabilityAdjustmentCatalogue.Decode(url.Values{"kind": {value}}); err != nil {
			t.Fatalf("kind=%s 被拒：%v", value, err)
		}
	}
	for catalogue, values := range map[*cataloguepage.Catalogue]url.Values{
		ports.ServiceCalendarVersionCatalogue: {"targetKind": {"AREA"}},
		ports.AvailabilityAdjustmentCatalogue: {"kind": {"SYN-KIND"}},
	} {
		if _, err := catalogue.Decode(values); err == nil {
			t.Fatalf("词表外的值被接受：%v", values)
		}
	}
}
