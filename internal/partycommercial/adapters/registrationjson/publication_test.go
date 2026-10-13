package registrationjson

import (
	"testing"
	"time"
)

// Covers: ADR-0119 Decision 五——批文时长只认 ISO-8601 的 `P[nD][T[nH][nM][nS]]` 子集：整数、按序至多一次、
// 至少一段；年 / 月 / 周 / 小数 / 逆序 / 重复 / 空 T 都拒。零时长由领域拒，不在本表。
func TestISODurationSubsetParsesExactlyTheDeclaredShape(t *testing.T) {
	accepted := map[string]time.Duration{
		"P3D":          72 * time.Hour,
		"PT72H":        72 * time.Hour,
		"P1DT2H30M15S": 26*time.Hour + 30*time.Minute + 15*time.Second,
		"PT90M":        90 * time.Minute,
		"P0D":          0,
	}
	for raw, want := range accepted {
		t.Run(raw, func(t *testing.T) {
			got, err := parseISODurationSubset(raw)
			if err != nil || got != want {
				t.Fatalf("parse %q = %v, %v; want %v", raw, got, err, want)
			}
		})
	}
	rejected := []string{"", "P", "PT", "3D", "P1Y", "P1M", "P1W", "P1.5D", "PT2H1H", "PT1M2H", "P1DT", "P1D2H", "PT1D", "P-1D", "P1DT2Hx"}
	for _, raw := range rejected {
		t.Run("拒 "+raw, func(t *testing.T) {
			if _, err := parseISODurationSubset(raw); err == nil {
				t.Fatalf("parse %q 被收下了", raw)
			}
		})
	}
}
