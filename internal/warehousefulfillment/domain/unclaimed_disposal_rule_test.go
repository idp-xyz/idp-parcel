package domain_test

import (
	"math"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 认领期限按经过秒数算。秒数乘进 time.Duration 会在超过可表示范围时回绕成负数，
// 期限未到会被处置判成已到。这里守登记门与比较本身，不依赖库上的 CHECK。

func TestDisposalRuleRejectsAClaimWindowThatWouldWrapADuration(t *testing.T) {
	maxSeconds := math.MaxInt64 / int64(time.Second)
	method := valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE")
	methods := []domain.UnclaimedDisposalMethod{method}

	fits := domain.NewDisposalRuleContent(maxSeconds, methods)
	if fits.InvalidField() != domain.DisposalRuleFieldInvalid {
		t.Fatalf("最大可表示整秒应受理，实得 %s", fits.InvalidField())
	}

	wraps := domain.NewDisposalRuleContent(maxSeconds+1, methods)
	if wraps.InvalidField() != domain.DisposalRuleFieldClaimWindowSeconds {
		t.Fatalf("再大一秒应指出认领期限，实得 %s", wraps.InvalidField())
	}
	// 评审取过的具体值：10000000000 秒乘 time.Second 回绕为负。
	knownWrap := domain.NewDisposalRuleContent(10_000_000_000, methods)
	if knownWrap.InvalidField() != domain.DisposalRuleFieldClaimWindowSeconds {
		t.Fatalf("10000000000 秒应指出认领期限，实得 %s", knownWrap.InvalidField())
	}

	occurred := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	if knownWrap.ClaimWindowElapsed(occurred, occurred.Add(time.Hour)) {
		t.Fatal("放不进 time.Duration 的期限不得被判成已走完")
	}
	if !fits.ClaimWindowElapsed(occurred, occurred.Add(time.Duration(maxSeconds)*time.Second)) {
		t.Fatal("刚好走完最大可表示整秒应判成已走完")
	}
	if fits.ClaimWindowElapsed(occurred, occurred.Add(time.Duration(maxSeconds)*time.Second-time.Nanosecond)) {
		t.Fatal("差一纳秒不得判成已走完")
	}
}

func TestClaimWindowElapsedUsesWholeSecondsFromTheArrivalInstant(t *testing.T) {
	content := domain.NewDisposalRuleContent(3600, []domain.UnclaimedDisposalMethod{
		valueOf(t, domain.NewUnclaimedDisposalMethod, "DONATE"),
	})
	occurred := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	if content.ClaimWindowElapsed(occurred, occurred.Add(3599*time.Second)) {
		t.Fatal("3599 秒尚未走完 3600 秒")
	}
	if !content.ClaimWindowElapsed(occurred, occurred.Add(3600*time.Second)) {
		t.Fatal("刚好 3600 秒应已走完")
	}
	if content.ClaimWindowElapsed(occurred, occurred.Add(-time.Second)) {
		t.Fatal("发生时刻之前不得判成已走完")
	}
}
