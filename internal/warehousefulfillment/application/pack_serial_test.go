package application

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestParcelSerialsMatchTheTrackedGoodsAndStayInsideOneOrder(t *testing.T) {
	goods, err := domain.NewGoodsCode("SYN-GOODS-01")
	if err != nil {
		t.Fatal(err)
	}
	unit, err := domain.NewGoodsUnitName("件")
	if err != nil {
		t.Fatal(err)
	}
	parcel := func(serials ...string) PackParcelCommand {
		return PackParcelCommand{Sequence: 1, Quantity: int64(len(serials)), Goods: goods, Unit: unit, Serials: serials}
	}
	tracked := func(code domain.GoodsCode) (bool, bool, error) {
		return code == goods, true, nil
	}
	if _, ok, err := normalizePackSerials([]PackParcelCommand{parcel("SN-1", "SN-2")}, tracked); err != nil || !ok {
		t.Fatal("两件两个号应当收下")
	}
	short := PackParcelCommand{Sequence: 1, Quantity: 2, Goods: goods, Unit: unit, Serials: []string{"SN-1"}}
	if _, ok, err := normalizePackSerials([]PackParcelCommand{short}, tracked); err != nil || ok {
		t.Fatal("少一个号不应当收下")
	}
	if _, ok, err := normalizePackSerials([]PackParcelCommand{parcel("SN-1", "SN-1")}, tracked); err != nil || ok {
		t.Fatal("同一张单里重复的号不应当收下")
	}
	plain := func(domain.GoodsCode) (bool, bool, error) { return false, true, nil }
	if _, ok, err := normalizePackSerials([]PackParcelCommand{{Sequence: 1, Quantity: 1, Goods: goods, Unit: unit}}, plain); err != nil || !ok {
		t.Fatal("不跟踪的货不带号应当收下")
	}
	if _, ok, err := normalizePackSerials([]PackParcelCommand{parcel("SN-1")}, plain); err != nil || ok {
		t.Fatal("不跟踪的货带了号不应当收下")
	}
}
