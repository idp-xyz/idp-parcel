package partycommercial

import (
	"testing"
	"time"
)

// 构造门与重建门都不放行零值的系统接收时间，FormAsOfValue 拿不到这样一份委托。
// 这一支是损坏数据的最后一道，测折法本身：零值报错，不答未配置。
func TestSubmissionReceiptInstantRejectsZeroReceivedAt(t *testing.T) {
	if _, err := submissionReceiptInstant(time.Time{}); err == nil {
		t.Fatal("系统接收时间为零却交回了时点")
	}
	received := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	at, err := submissionReceiptInstant(received)
	if err != nil || !at.Equal(received) {
		t.Fatalf("at=%v err=%v", at, err)
	}
}
