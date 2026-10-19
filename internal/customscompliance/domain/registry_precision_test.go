package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// Covers: 本上下文按登记册存得下的精度认时刻（AtRegistryPrecision）——微秒以下的尾巴去掉、跨一微秒仍是两个时刻、
// 非 UTC 的写法与它的 UTC 同值。
func TestTheRegistryPrecisionKeepsMicrosecondsAndDropsTheTail(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.UTC)
	if got, want := domain.AtRegistryPrecision(at), time.Date(2026, 1, 1, 0, 0, 0, 123456000, time.UTC); !got.Equal(want) {
		t.Fatalf("AtRegistryPrecision(%s) = %s，want %s", at.Format(time.RFC3339Nano), got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
	if domain.AtRegistryPrecision(at).Equal(domain.AtRegistryPrecision(at.Add(time.Microsecond))) {
		t.Fatal("跨一微秒的两个时刻折完成了同一个——登记册上它们是两个值")
	}
	shanghai := time.FixedZone("CST", 8*3600)
	folded := domain.AtRegistryPrecision(at.In(shanghai))
	if !folded.Equal(domain.AtRegistryPrecision(at)) || folded.Location() != time.UTC {
		t.Fatalf("非 UTC 输入折成 %s（%s），want 与其 UTC 同值且落在 UTC", folded.Format(time.RFC3339Nano), folded.Location())
	}
}
