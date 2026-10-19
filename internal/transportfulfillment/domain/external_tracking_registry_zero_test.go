package domain

import (
	"errors"
	"testing"
	"time"
)

// 本文件钉票 tf-face-algebra/14 判零在外部承运轨迹事实上的那几道门（规则见 AtRegistryPrecision）：认领的发生时间与
// 接收时间、有效时间的两种判断。放在包内，是因为认领夹具 trackingFactSpec 只在包内可见。夹具全为合成值（S 级）。

// trackingSubMicroAfterZero 在登记册上都是零点；与包外测试的同名夹具各自独立写出，不经被测函数。
var trackingSubMicroAfterZero = map[string]time.Time{
	"零点后 1ns":            time.Time{}.Add(time.Nanosecond),
	"零点后 999ns":          time.Time{}.Add(999 * time.Nanosecond),
	"+14:00 写法零点后 500ns": time.Date(1, 1, 1, 14, 0, 0, 500, time.FixedZone("SYN+14", 14*3600)),
}

func pinTrackingRegistryZeroGate(t *testing.T, atZero, afterZero error, gate func(at time.Time) error) {
	t.Helper()
	if err := gate(time.Time{}); !errors.Is(err, atZero) {
		t.Errorf("恰为零点：err = %v，期望 %v", err, atZero)
	}
	for name, at := range trackingSubMicroAfterZero {
		if err := gate(at); !errors.Is(err, afterZero) {
			t.Errorf("%s：err = %v，期望 %v", name, err, afterZero)
		}
	}
	if err := gate(time.Time{}.Add(time.Microsecond)); err != nil {
		t.Errorf("零点后 1µs：err = %v，期望照常", err)
	}
}

// Covers: 票 tf-face-algebra/14 判零——发生时间恰为零值仍答「源未给」；源给了、登记册上却是零点的那一格是库收不下的
// 取值，答形状错误而不是「源未给」——那条素材不该进留痕。
func TestExternalTrackingGatesJudgeZeroOnTheRegistry(t *testing.T) {
	t.Run("AdoptExternalCarrierTracking 发生时间", func(t *testing.T) {
		pinTrackingRegistryZeroGate(t, ErrOccurredAtNotGivenBySource, ErrInvalidExternalTrackingFact, func(at time.Time) error {
			spec := trackingFactSpec(t)
			spec.OccurredAt = at
			_, err := AdoptExternalCarrierTracking(spec)
			return err
		})
	})
	t.Run("AdoptExternalCarrierTracking 接收时间", func(t *testing.T) {
		pinTrackingRegistryZeroGate(t, ErrInvalidExternalTrackingFact, ErrInvalidExternalTrackingFact, func(at time.Time) error {
			spec := trackingFactSpec(t)
			spec.ReceivedAt = at
			_, err := AdoptExternalCarrierTracking(spec)
			return err
		})
	})
	t.Run("JudgeEffectiveTimeExplicitly", func(t *testing.T) {
		pinTrackingRegistryZeroGate(t, ErrInvalidEffectiveTimeJudgment, ErrInvalidEffectiveTimeJudgment, func(at time.Time) error {
			_, err := JudgeEffectiveTimeExplicitly(at)
			return err
		})
	})
	t.Run("JudgeEffectiveTimeByRule", func(t *testing.T) {
		rule, err := NewEffectiveTimeRuleReference("SYN-RULE-1", "SYN-RULE-V1")
		if err != nil {
			t.Fatalf("规则引用：%v", err)
		}
		pinTrackingRegistryZeroGate(t, ErrInvalidEffectiveTimeJudgment, ErrInvalidEffectiveTimeJudgment, func(at time.Time) error {
			_, err := JudgeEffectiveTimeByRule(rule, at)
			return err
		})
	})
}
