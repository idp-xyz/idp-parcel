package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Covers: 用法面——缺命令、未知命令、缺 -input、缺连接串都在连库之前答 1，用法文本列出全部命令。
func TestUsageIsRefusedBeforeTouchingTheDatabase(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(input, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	noDSN := func(string) string { return "" }
	for name, args := range map[string][]string{
		"缺命令":   nil,
		"未知命令":  {"capacity-reservation", "-input", input},
		"缺输入":   {commandSchedule},
		"缺连接串":  {commandScheduleTemplate, "-input", input},
		"输入不存在": {commandSchedule, "-input", filepath.Join(t.TempDir(), "missing.json")},
	} {
		var out, errOut bytes.Buffer
		if code := run(context.Background(), args, noDSN, &out, &errOut); code != exitUsage {
			t.Fatalf("%s：退出码 = %d（%s），要 %d", name, code, errOut.String(), exitUsage)
		}
	}
	var out, errOut bytes.Buffer
	run(context.Background(), nil, noDSN, &out, &errOut)
	for _, command := range allCommands {
		if !strings.Contains(errOut.String(), command) {
			t.Fatalf("用法文本缺 %s：%s", command, errOut.String())
		}
	}
}

// Covers: 译装面——输入与在线口同一份载荷再加 tenantId；未知键、缺租户、解不出的时刻、一份文件多条登记都在进事务
// 之前拒。
func TestTranslationRefusesBeforeTheTransaction(t *testing.T) {
	for name, testCase := range map[string]struct {
		command string
		raw     string
	}{
		"未知键":    {commandScheduleTemplate, `{"tenantId": "SYN-TENANT-01", "template": "T", "direction": "CN-US"}`},
		"缺租户":    {commandScheduleTemplate, `{"template": "T"}`},
		"解不出的时刻": {commandSchedule, `{"tenantId": "SYN-TENANT-01", "schedule": "S", "departsAt": "tomorrow"}`},
		"一份文件两条": {commandSchedule, `{"tenantId": "SYN-TENANT-01"} {"tenantId": "SYN-TENANT-01"}`},
	} {
		if _, err := commandFor(testCase.command, []byte(testCase.raw)); err == nil {
			t.Fatalf("%s：应在译装处拒", name)
		}
	}
	if _, err := commandFor(commandSchedule, []byte(`{"tenantId": "SYN-TENANT-01", "template": "T", "runningDate": "2026-10-21"}`)); err != nil {
		t.Fatalf("物化形输入应能译装：%v", err)
	}
}
