package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const inventory = "# AT 覆盖度盘点\n\n## 总览\n\n" + countsBegin + "\n| 桶 | 数量 |\n| 旧 | 999 |\n" + countsEnd + "\n\n" +
	"## ① 已钉住（AT → 测试）\n\n| AT | 测试 |\n|---|---|\n" +
	"| `AT-PS-001` | a_test.go::TestA |\n| `AT-NO-002` | b_test.go::TestB |\n| `AT-PS-003` | c_test.go::TestC |\n\n" +
	"## ② 机制候选\n\n| AT | 场景 | UC |\n|---|---|---|\n| `AT-NO-004` | 场景 | UC |\n\n" +
	"## 重盘变化摘要\n\n| 桶 | 首盘 |\n|---|---|\n| `AT-PS-999` | 不在分桶节里，不算 |\n"

func TestCountTakesBucketsFromCircledHeadingsAndRowsUnderThem(t *testing.T) {
	census, err := Count(inventory)
	if err != nil {
		t.Fatalf("重算失败：%v", err)
	}
	got := Render(census)
	want := "| 桶 | 数量 |\n|---|---|\n| ① 已钉住 | 3 |\n| ② 机制候选 | 1 |\n| 合计 | 4 |\n\n" +
		"| 前缀 | 总数 | ① | ② |\n|---|---|---|---|\n| NO | 2 | 1 | 1 |\n| PS | 2 | 2 | 0 |\n"
	if got != want {
		t.Errorf("总览 =\n%s\n想要\n%s", got, want)
	}
}

func TestCountRefusesAnATListedTwice(t *testing.T) {
	twice := strings.Replace(inventory, "| `AT-NO-004` | 场景 | UC |", "| `AT-PS-001` | 场景 | UC |", 1)
	if _, err := Count(twice); err == nil {
		t.Error("同一条 AT 在两个桶里各一行，想要报错，却重算成功")
	}
	if _, err := Count("# 没有分桶\n\n## 总览\n"); err == nil {
		t.Error("没有分桶节，想要报错，却重算成功")
	}
}

func TestRunRewritesOnlyTheOverviewAndCheckFollows(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(inventoryPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(inventory), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run(root, true); err == nil {
		t.Fatal("计数还是旧的，-check 却通过了")
	}
	if err := run(root, false); err != nil {
		t.Fatalf("重算失败：%v", err)
	}
	if err := run(root, true); err != nil {
		t.Fatalf("刚重算完，-check 却不通过：%v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "| 旧 | 999 |") {
		t.Error("标记之间的旧计数没被换掉")
	}
	if !strings.HasSuffix(got, "## 重盘变化摘要\n\n| 桶 | 首盘 |\n|---|---|\n| `AT-PS-999` | 不在分桶节里，不算 |\n") {
		t.Error("标记之外的正文被改动了")
	}
}
