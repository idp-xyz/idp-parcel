package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunConsolidationChecksTemplateBeforeDatabase 证 consolidation 子命令已接进 run，且模板
// 整体先核再碰数据库：坏模板退 1、报文挂集运模板根错误、根本不问 DSN；好模板在 DSN 未设
// 时才退 1 报 DSN——两条报文互斥，证的是顺序不是各自存在。
func TestRunConsolidationChecksTemplateBeforeDatabase(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.csv")
	if err := os.WriteFile(bad, []byte(consolidationHeader+consolidationLine("SYN-WORK-1", "PACK", "", "", "", "")), 0o600); err != nil {
		t.Fatalf("写坏模板：%v", err)
	}
	good := filepath.Join(dir, "good.csv")
	if err := os.WriteFile(good, sampleConsolidationTemplate(t), 0o600); err != nil {
		t.Fatalf("写好模板：%v", err)
	}
	noEnv := func(string) string { return "" }
	clock := fixedClock{at: mustInstant(t, "2026-09-08T12:00:00+08:00")}

	var errOut bytes.Buffer
	code := run(context.Background(), []string{commandConsolidation, "-tenant", "SYN-T1", "-file", bad}, noEnv, clock, io.Discard, &errOut)
	if code != exitUsage || !strings.Contains(errOut.String(), "集运模板不合格") || strings.Contains(errOut.String(), envDatabaseDSN) {
		t.Fatalf("坏模板：退出码 = %d，stderr=%q；要 %d 且只报模板不报 DSN", code, errOut.String(), exitUsage)
	}

	errOut.Reset()
	code = run(context.Background(), []string{commandConsolidation, "-tenant", "SYN-T1", "-file", good}, noEnv, clock, io.Discard, &errOut)
	if code != exitUsage || !strings.Contains(errOut.String(), envDatabaseDSN+" 未设置") {
		t.Fatalf("好模板缺 DSN：退出码 = %d，stderr=%q；要 %d 且报 DSN 未设置", code, errOut.String(), exitUsage)
	}
}

// TestRunListsBothCommandsInUsage 证用法与未知命令的报文里两个子命令都在——运维看到的就是
// 这一行，缺了哪个就等于那个口没发出去。
func TestRunListsBothCommandsInUsage(t *testing.T) {
	clock := fixedClock{at: mustInstant(t, "2026-09-08T12:00:00+08:00")}
	var errOut bytes.Buffer
	if code := run(context.Background(), nil, func(string) string { return "" }, clock, io.Discard, &errOut); code != exitUsage {
		t.Fatalf("无参数退出码 = %d，要 %d", code, exitUsage)
	}
	if !strings.Contains(errOut.String(), commandIntake+"|"+commandConsolidation) {
		t.Fatalf("用法没列出两个子命令：%q", errOut.String())
	}
}
