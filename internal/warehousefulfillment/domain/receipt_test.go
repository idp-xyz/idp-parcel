package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestOverageQuantityLandsOnTheCrossingReceipt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		advised, prior int64
		this, want     int64
	}{
		{"先收 60 没有多收", 100, 0, 60, 0},
		{"再收 50 记多收 10", 100, 60, 50, 10},
		{"按箱折成基本单位后多收 10", 240, 0, 250, 10},
		{"先前已经超过，这一次全部记多收", 100, 110, 5, 5},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, ok := domain.OverageQuantity(test.advised, test.prior, test.this)
			if !ok || got != test.want {
				t.Fatalf("多收 = %d ok=%v，want %d", got, ok, test.want)
			}
		})
	}
}

func TestRecordedReceiptSameContentIgnoresArrivalBookkeeping(t *testing.T) {
	t.Parallel()
	line := receiptLine(t, "SYN-GOODS-01", 60)
	posted, err := line.AsPosted(valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"))
	if err != nil {
		t.Fatal(err)
	}
	occurred := time.Date(2026, 11, 20, 1, 2, 3, 123456789, time.FixedZone("CST", 8*3600))
	receipt := recordedReceipt(t, occurred, []domain.ReceiptLine{posted}, domain.ReceiptDiscrepancyID{}, nil)
	sameInstant := time.Date(2026, 11, 19, 17, 2, 3, 123456000, time.UTC)
	if !receipt.SameContent(receiptSubmission(t, sameInstant, []domain.ReceiptLine{line})) {
		t.Fatal("同一瞬间、同一行应当算同一份内容")
	}
	changed := receiptLine(t, "SYN-GOODS-01", 50)
	if receipt.SameContent(receiptSubmission(t, occurred, []domain.ReceiptLine{changed})) {
		t.Fatal("数量不同不该算同一份内容")
	}
}

func receiptLine(t *testing.T, observed string, quantity int64) domain.ReceiptLine {
	t.Helper()
	line, err := domain.NewReceiptLine(
		observed, valueOf(t, domain.NewGoodsUnitName, "件"), domain.StockStateGood, quantity,
		domain.BatchNumber{}, domain.ExpiryDate{}, domain.GoodsAliasCode{},
		valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func receiptSubmission(t *testing.T, occurred time.Time, lines []domain.ReceiptLine) domain.ReceiptSubmission {
	t.Helper()
	return domain.ReceiptSubmission{
		Device:     valueOf(t, domain.NewReceiptDeviceID, "SYN-DEVICE-01"),
		Operator:   valueOf(t, domain.NewReceiptOperatorID, "operator-1"),
		OccurredAt: occurred, Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		PreAdvice: valueOf(t, domain.NewPreAdviceID, "IPA-1"), Shape: domain.InboundByGoods, Lines: lines,
	}
}

func recordedReceipt(
	t *testing.T,
	occurred time.Time,
	lines []domain.ReceiptLine,
	discrepancy domain.ReceiptDiscrepancyID,
	differences []domain.ReceiptDiscrepancyLine,
) domain.RecordedReceipt {
	t.Helper()
	receipt, err := domain.NewRecordedReceipt(domain.RecordedReceiptSpec{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		FactID:     valueOf(t, domain.NewReceiptFactID, "SYN-FACT-01"),
		Device:     valueOf(t, domain.NewReceiptDeviceID, "SYN-DEVICE-01"),
		Operator:   valueOf(t, domain.NewReceiptOperatorID, "operator-1"),
		OccurredAt: occurred, ReceivedAt: occurred.Add(time.Minute),
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		PreAdvice: valueOf(t, domain.NewPreAdviceID, "IPA-1"),
		Shape:     domain.InboundByGoods, Outcome: domain.RecordedReceiptConfirmed,
		Lines: lines, Discrepancy: discrepancy, Differences: differences,
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}
