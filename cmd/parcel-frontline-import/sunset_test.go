package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

func mustInstant(t *testing.T, value string) time.Time {
	t.Helper()
	instant, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("解析时刻 %q：%v", value, err)
	}
	return instant
}

// TestStructuralSunsetGuardBoundary 把钟拨到期限两侧证守卫的边：期限时刻本身放行（「超过」
// 才拒），晚一纳秒即拒；同一时刻换时区表示不改变判断。
func TestStructuralSunsetGuardBoundary(t *testing.T) {
	sunset := mustInstant(t, structuralSunsetLiteral)
	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"前一天", sunset.Add(-24 * time.Hour), true},
		{"期限时刻", sunset, true},
		{"期限后一纳秒", sunset.Add(time.Nanosecond), false},
		{"期限后一秒", sunset.Add(time.Second), false},
		{"次年", sunset.AddDate(1, 0, 0), false},
		{"同一时刻的 UTC 表示", mustInstant(t, "2026-12-31T15:59:59Z"), true},
		{"UTC 表示晚一秒", mustInstant(t, "2026-12-31T16:00:00Z"), false},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			err := guardStructuralSunset(spec.now)
			if allowed := err == nil; allowed != spec.want {
				t.Fatalf("now=%s 放行=%v，要 %v（err=%v）", spec.now.Format(time.RFC3339Nano), allowed, spec.want, err)
			}
			if err != nil && !errors.Is(err, ErrStructuralSunsetReached) {
				t.Fatalf("拒绝没有带 ErrStructuralSunsetReached：%v", err)
			}
		})
	}
}

// TestRunRefusesToStartAfterSunset 证守卫在 run 的第一行且没有绕过口：拨钟到期限后，给一份
// 合格模板、一个若真去连会答未决（3）的坏 DSN、再加一个自称能豁免的环境变量——仍旧
// 是启动拒绝（4），标准错误点名期限与 ADR-0089，标准输出一行都没有。
func TestRunRefusesToStartAfterSunset(t *testing.T) {
	sample, err := os.ReadFile(filepath.Join("testdata", "intake-v1.csv"))
	if err != nil {
		t.Fatalf("读样例模板：%v", err)
	}
	file := filepath.Join(t.TempDir(), "intake.csv")
	if err := os.WriteFile(file, sample, 0o600); err != nil {
		t.Fatalf("写模板：%v", err)
	}
	env := map[string]string{
		envDatabaseDSN: "postgres://nobody:nobody@127.0.0.1:1/never?sslmode=disable",
		"IDP_PARCEL_FRONTLINE_IMPORT_IGNORE_SUNSET": "1",
	}
	getenv := func(key string) string { return env[key] }
	afterSunset := fixedClock{at: mustInstant(t, structuralSunsetLiteral).Add(time.Second)}

	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{commandIntake, "-tenant", "SYN-T1", "-file", file}, getenv, afterSunset, &out, &errOut)
	if code != exitSunset {
		t.Fatalf("过期启动退出码 = %d（stderr=%q），要 %d", code, errOut.String(), exitSunset)
	}
	if !strings.Contains(errOut.String(), "拆除期限") || !strings.Contains(errOut.String(), "ADR-0089") {
		t.Fatalf("拒绝报文没点名期限与 ADR：%q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("过期启动不该有任何标准输出：%q", out.String())
	}
}

// TestRunSunsetGuardPrecedesUsage 证守卫排在用法解释之前：过期时连缺参数都不解释。
func TestRunSunsetGuardPrecedesUsage(t *testing.T) {
	afterSunset := fixedClock{at: mustInstant(t, structuralSunsetLiteral).Add(24 * time.Hour)}
	var errOut bytes.Buffer
	code := run(context.Background(), nil, func(string) string { return "" }, afterSunset, io.Discard, &errOut)
	if code != exitSunset {
		t.Fatalf("过期且无参数退出码 = %d，要 %d", code, exitSunset)
	}
	if strings.Contains(errOut.String(), "用法") {
		t.Fatalf("过期的导入口不该解释用法：%q", errOut.String())
	}
}
