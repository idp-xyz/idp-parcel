package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/tools/internal/markedregion"
)

func TestDeclaredATsTakesOnlyTheFirstCellOfATableRow(t *testing.T) {
	root := t.TempDir()
	ucDir := filepath.Join(root, filepath.FromSlash(applicationDir), "ctx")
	if err := os.MkdirAll(ucDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"UC-X-001.md": "| AT | 场景 |\n|---|---|\n| `AT-PS-001` | a |\r\n| AT-PS-002 | 不带反引号也算 |\n" +
			"正文里替代 `AT-PS-900` 的那一行不是立号。\n" +
			"| `AT-PS-001`、`AT-PS-901` | 交接表并列几个号的首格不算立号 |\n",
		"UC-X-002.md": "| `AT-PS-001` | 交接表里再引一次，仍记首次立号的文件 |\n| `AT-NO-003` | a |\n",
		"notes.txt":   "| `AT-PS-902` | 不是 .md |\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(ucDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	declared, err := DeclaredATs(root)
	if err != nil {
		t.Fatalf("收立号失败：%v", err)
	}
	want := map[string]string{
		"AT-PS-001": "docs/application/ctx/UC-X-001.md",
		"AT-PS-002": "docs/application/ctx/UC-X-001.md",
		"AT-NO-003": "docs/application/ctx/UC-X-002.md",
	}
	if len(declared) != len(want) {
		t.Fatalf("立号 = %v，想要 %v", declared, want)
	}
	for id, file := range want {
		if declared[id] != file {
			t.Errorf("%s 立于 %q，想要 %q", id, declared[id], file)
		}
	}

	if _, err := DeclaredATs(t.TempDir()); err == nil {
		t.Error("没有 docs/application，想要报错，却交回了空集——空集会让每一行都被当成多出的")
	}
}

func TestReconcileNamesEveryMissingAndExtraRowAndHowToFixThem(t *testing.T) {
	census, err := Count(inventory)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]string{
		"AT-PS-001": "docs/application/ps/UC-PS-001.md",
		"AT-NO-002": "docs/application/no/UC-NO-001.md",
		"AT-PS-003": "docs/application/ps/UC-PS-001.md",
		"AT-NO-004": "docs/application/no/UC-NO-001.md",
	}
	if err := Reconcile(declared, census); err != nil {
		t.Fatalf("立号与表行一一对得上，却报：%v", err)
	}

	declared["AT-TF-010"] = "docs/application/tf/UC-TF-005.md"
	declared["AT-TF-009"] = "docs/application/tf/UC-TF-004.md"
	delete(declared, "AT-NO-004")
	err = Reconcile(declared, census)
	var gap *CoverageError
	if !errors.As(err, &gap) {
		t.Fatalf("缺行又多行，想要 CoverageError，得到 %v", err)
	}
	message := err.Error()
	for _, piece := range []string{
		"AT-TF-009（docs/application/tf/UC-TF-004.md）",
		"AT-TF-010（docs/application/tf/UC-TF-005.md）",
		"分桶表里没有行的 2 个",
		"AT-NO-004（② 机制候选）",
		"已没有这个号的 1 个",
		inventoryPath,
		"补一行",
		"加数不算数",
	} {
		if !strings.Contains(message, piece) {
			t.Errorf("报文缺「%s」：\n%s", piece, message)
		}
	}
	if strings.Index(message, "AT-TF-009") > strings.Index(message, "AT-TF-010") {
		t.Errorf("缺行没按号排：\n%s", message)
	}
}

func TestRunRefusesToRecountWhileARowIsMissing(t *testing.T) {
	root := t.TempDir()
	path := writeRepo(t, root, inventory, acceptance+"| `AT-TF-010` | 新立未补行 | b |\n")

	for _, check := range []bool{true, false} {
		err := run(root, check)
		var gap *CoverageError
		if !errors.As(err, &gap) {
			t.Fatalf("check=%v：缺一行，想要 CoverageError，得到 %v", check, err)
		}
		if errors.Is(err, markedregion.ErrStale) {
			t.Errorf("check=%v：缺行不是「生成物过期」——重跑生成器补不上它，退出码要与过期分开", check)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "| 旧 | 999 |") {
		t.Error("缺行时仍重写了总览：照少了一行的表算出的数会被当成真的提交")
	}

	withRow := strings.Replace(inventory, "| `AT-NO-004` | 场景 | UC |", "| `AT-NO-004` | 场景 | UC |\n| `AT-TF-010` | 新立 | UC |", 1)
	writeRepo(t, root, withRow, acceptance+"| `AT-TF-010` | 新立未补行 | b |\n")
	if err := run(root, false); err != nil {
		t.Fatalf("补上那一行之后仍重算失败：%v", err)
	}
}
