package ports_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func TestCutBookKeepsTheOrderAndTheTotal(t *testing.T) {
	rows := []string{"a", "b", "c"}
	window, book := ports.CutBook(rows, 2, 2)
	if len(window) != 1 || window[0] != "c" || book.Page != 2 || book.PageSize != 2 || book.Total != 3 {
		t.Fatalf("第二页 %+v %+v", window, book)
	}
	past, book := ports.CutBook(rows, 3, 2)
	if len(past) != 0 || book.Total != 3 {
		t.Fatalf("越过末页 %+v %+v", past, book)
	}
	whole, book := ports.CutBook(rows, 0, 0)
	if len(whole) != 3 || book.Page != 0 {
		t.Fatalf("不切页 %+v %+v", whole, book)
	}
}
