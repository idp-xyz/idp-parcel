package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件钉 AtRegistryPrecision 这一条规则本身，以及它在领域里的两种用法：同键内容相等与严格先后的构造门。
// 夹具全为合成值（S 级）。

var (
	// registryNanos 带纳秒尾巴；registryMicros 是它在登记册上的样子，测试里独立按微秒截出，不经被测函数。
	registryNanos  = time.Date(2026, 9, 1, 9, 0, 0, 123456789, time.UTC)
	registryMicros = registryNanos.Truncate(time.Microsecond)
)

// Covers: 票 operator-channel/30 裁定 1——去掉纳秒尾巴、跨一微秒仍不同、非 UTC 与对应的 UTC 时刻同值、零值仍是零值。
func TestAtRegistryPrecisionFoldsAnInstantToTheMicrosecond(t *testing.T) {
	if got := domain.AtRegistryPrecision(registryNanos); !got.Equal(registryMicros) {
		t.Fatalf("纳秒尾巴应去掉：%s，期望 %s", got.Format(time.RFC3339Nano), registryMicros.Format(time.RFC3339Nano))
	}
	if domain.AtRegistryPrecision(registryNanos).Equal(domain.AtRegistryPrecision(registryNanos.Add(time.Microsecond))) {
		t.Fatal("相差一微秒折完仍应不同")
	}
	shanghai := registryNanos.In(time.FixedZone("CST", 8*3600))
	folded := domain.AtRegistryPrecision(shanghai)
	if folded != domain.AtRegistryPrecision(registryNanos) || folded.Location() != time.UTC {
		t.Fatalf("非 UTC 输入应与对应的 UTC 时刻同值：%s（%s）", folded.Format(time.RFC3339Nano), folded.Location())
	}
	if !domain.AtRegistryPrecision(time.Time{}).IsZero() {
		t.Fatal("零值折完仍应是零值：开放的终点、首版没有的改变时间都靠它")
	}
}
