package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: PS CONTEXT「地址要素」——封闭两要素，名字面 POSTAL_CODE / COUNTRY_CODE；条目名由资料范围原词与要素名
// 两段组成（既有 name/value 条目的点号命名法），寄件与收件各有各的条目，同名要素在两个范围里是两条不同的内容。
func TestAddressElementEntryNamesAreScopedByDataGroup(t *testing.T) {
	cases := map[string]string{
		domain.AddressElementEntryName(domain.DeliveryPlaceDataGroup(), domain.PostalCodeElement):  "DELIVERY_PLACE.POSTAL_CODE",
		domain.AddressElementEntryName(domain.DeliveryPlaceDataGroup(), domain.CountryCodeElement): "DELIVERY_PLACE.COUNTRY_CODE",
		domain.AddressElementEntryName(domain.SenderPlaceDataGroup(), domain.PostalCodeElement):    "SENDER_PLACE.POSTAL_CODE",
		domain.AddressElementEntryName(domain.SenderPlaceDataGroup(), domain.CountryCodeElement):   "SENDER_PLACE.COUNTRY_CODE",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("entry name = %q, want %q", got, want)
		}
	}
	if domain.PostalCodeElement.String() != "POSTAL_CODE" || domain.CountryCodeElement.String() != "COUNTRY_CODE" {
		t.Fatalf("element names = %q / %q", domain.PostalCodeElement, domain.CountryCodeElement)
	}
	if (domain.AddressElementName(0)).String() != "" {
		t.Fatal("the zero element name has a word")
	}
}

// Covers: 裁决 1「读口按要素名取值」与「既有快照缺这两个要素即未提供，如实答缺」——从一段范围条目里只认本上下文
// 定名的条目，别的条目（租户约定的名字、别的范围的同名要素）一律不认；值原样交，不规范化；缺席如实。
func TestAddressElementsAreReadByClosedEntryNameOnly(t *testing.T) {
	entries := []domain.CanonicalContentEntry{
		contentEntry(t, "recipient.address", "1 Example Street, Berlin"),
		contentEntry(t, "DELIVERY_PLACE.POSTAL_CODE", " 10115 "),
		contentEntry(t, "SENDER_PLACE.POSTAL_CODE", "200001"),
		contentEntry(t, "SENDER_PLACE.COUNTRY_CODE", "CN"),
		contentEntry(t, "postal_code", "99999"),
	}

	destination := domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(), entries)
	postal, declared := destination.PostalCode()
	if !declared || postal != " 10115 " {
		t.Fatalf("destination postal = %q declared = %v; 值要原样，不去空白不规范化", postal, declared)
	}
	if country, declared := destination.CountryCode(); declared || country != "" {
		t.Fatalf("destination country = %q declared = %v; 收件范围上没报国家 / 地区码，不拿寄件的顶", country, declared)
	}

	origin := domain.AddressElementsOf(domain.SenderPlaceDataGroup(), entries)
	if postal, declared := origin.PostalCode(); !declared || postal != "200001" {
		t.Fatalf("origin postal = %q declared = %v", postal, declared)
	}
	if country, declared := origin.CountryCode(); !declared || country != "CN" {
		t.Fatalf("origin country = %q declared = %v", country, declared)
	}

	if !domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(), nil).Empty() {
		t.Fatal("没有任何条目却报告有要素")
	}

	// 全是空白的值不是空串：它不是 CanonicalContentEntry 定义的「显式清空」，原样在场——为判在场先去空白也是一次规范化。
	blank := domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(), []domain.CanonicalContentEntry{
		contentEntry(t, "DELIVERY_PLACE.COUNTRY_CODE", "   "),
	})
	if country, declared := blank.CountryCode(); !declared || country != "   " {
		t.Fatalf("blank country = %q declared = %v; 全空白的值要原样在场，与「没报」分得开", country, declared)
	}
}

// Covers: CanonicalContentEntry 把「显式清空」定为一条值为空的条目——对地址要素而言清空后的值不是一个邮编，读口按
// 缺席答（消费方拿到空串无处可用），但同名多条时不挑一条：那是坏内容，两条都不认。
func TestAnExplicitlyClearedOrDuplicatedAddressElementReadsAsAbsent(t *testing.T) {
	cleared := domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(), []domain.CanonicalContentEntry{
		contentEntry(t, "DELIVERY_PLACE.POSTAL_CODE", ""),
	})
	if _, declared := cleared.PostalCode(); declared {
		t.Fatal("显式清空的邮编被读成了在场")
	}

	duplicated := domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(), []domain.CanonicalContentEntry{
		contentEntry(t, "DELIVERY_PLACE.POSTAL_CODE", "10115"),
		contentEntry(t, "DELIVERY_PLACE.POSTAL_CODE", "10117"),
	})
	if postal, declared := duplicated.PostalCode(); declared {
		t.Fatalf("同名两条矛盾的邮编被挑了一条 %q", postal)
	}
}
