package application

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestStockWordMatchesTheGoodsNotTheRestOfTheBook(t *testing.T) {
	named := stockWords{needle: "耳机", names: map[string]string{"SYN-GOODS-01": "蓝牙耳机 syn-bar-01"}}
	if !named.matches("SYN-GOODS-01", "SYN-LOC-A") {
		t.Fatal("名称与条码该认到这一件")
	}
	if named.matches("SYN-GOODS-02", "SYN-LOC-A") {
		t.Fatal("别的商品不该因为库位相同就被认到")
	}
	if !(stockWords{needle: "loc-b"}).matches("SYN-GOODS-02", "SYN-LOC-B") {
		t.Fatal("库位不区分大小写")
	}
	if !(stockWords{}).matches("SYN-GOODS-02", "SYN-LOC-A") {
		t.Fatal("没给检索词时整份都在")
	}
}

func TestStockAgeUsesTheWarehouseTimeZone(t *testing.T) {
	inbound := time.Date(2026, 11, 19, 18, 0, 0, 0, time.UTC)
	now := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	days, configured, err := StockAgeDays(inbound, now, domain.BusinessTimeZone{})
	if err != nil || configured || days != 0 {
		t.Fatalf("没登记 %d %t %v", days, configured, err)
	}
	zone := valueOrFail(t, domain.NewBusinessTimeZone, "Asia/Shanghai")
	days, configured, err = StockAgeDays(inbound, now, zone)
	if err != nil || !configured || days != 1 {
		t.Fatalf("上海 %d %t %v", days, configured, err)
	}
}

func valueOrFail[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
