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

// acceptance 是与 inventory 四条表行一一对得上的 UC 验收表。
const acceptance = "# UC\n\n| AT | 场景 | 结果 |\n|---|---|---|\n" +
	"| `AT-PS-001` | a | b |\n| `AT-NO-002` | a | b |\n| `AT-PS-003` | a | b |\n| `AT-NO-004` | a | b |\n"

// writeRepo 在 root 下摆出盘点与一份 UC，交回盘点的路径。
func writeRepo(t *testing.T, root, inventoryContent, ucContent string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(inventoryPath))
	uc := filepath.Join(root, filepath.FromSlash(applicationDir), "ctx", "UC-X-001.md")
	for file, content := range map[string]string{path: inventoryContent, uc: ucContent} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestRunRewritesOnlyTheOverviewAndCheckFollows(t *testing.T) {
	root := t.TempDir()
	path := writeRepo(t, root, inventory, acceptance)

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
