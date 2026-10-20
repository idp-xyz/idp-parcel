package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func adr(heading, status string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(heading + "\n\n" + status + "\nDate: 2026-10-19\n\n## Context\n\nStatus: 正文里的同名行不算\n")}
}

func TestRenderTakesTitleFromHeadingAndNoteVerbatimFromStatus(t *testing.T) {
	fsys := fstest.MapFS{
		"0002-b.md": adr("# ADR-0002：乙", "Status: Accepted（2026-10-19，某会话代决）"+NoteMarker+"｜**部分停用**：决定一「甲｜乙」一句已由 [ADR-0003](./0003-c.md) 停用；其余各条不变  "),
		"0001-a.md": adr("# ADR-0001: 甲", "Status: Accepted  "),
		"0003-c.md": adr("# ADR-0003：丙", "Status: Proposed"),
		"README.md": &fstest.MapFile{Data: []byte("不是记录")},
	}
	records, err := ReadRecords(fsys)
	if err != nil {
		t.Fatalf("读记录失败：%v", err)
	}
	accepted, superseded := Render(records)

	want := "- [ADR-0001：甲](./0001-a.md)\n" +
		"- [ADR-0002：乙](./0002-b.md)｜**部分停用**：决定一「甲｜乙」一句已由 [ADR-0003](./0003-c.md) 停用；其余各条不变\n" +
		"- [ADR-0003：丙](./0003-c.md)\n"
	if accepted != want {
		t.Errorf("已接受列表 =\n%s\n想要\n%s", accepted, want)
	}
	if superseded != "" {
		t.Errorf("没有被取代的记录，取代列表却是 %q", superseded)
	}
}

func TestSupersededGoesToItsOwnListWithTheTargetFromStatus(t *testing.T) {
	fsys := fstest.MapFS{
		"0011-old.md": adr("# ADR-0011: 旧", "Status: Superseded by [ADR-0012](./0012-new.md)"+NoteMarker+"：同一决策改写。  "),
		"0012-new.md": adr("# ADR-0012：新", "Status: Accepted"),
	}
	records, err := ReadRecords(fsys)
	if err != nil {
		t.Fatalf("读记录失败：%v", err)
	}
	accepted, superseded := Render(records)

	if accepted != "- [ADR-0012：新](./0012-new.md)\n" {
		t.Errorf("已接受列表 = %q", accepted)
	}
	if want := "- [ADR-0011：旧](./0011-old.md) → 由 [ADR-0012](./0012-new.md) 取代：同一决策改写。\n"; superseded != want {
		t.Errorf("取代列表 =\n%s\n想要\n%s", superseded, want)
	}
}

func TestReadRecordsRefusesWhatWouldSilentlyMisindex(t *testing.T) {
	cases := map[string]fstest.MapFS{
		"H1 号与文件名不符":  {"0001-a.md": adr("# ADR-0002：甲", "Status: Accepted")},
		"没有 H1":       {"0001-a.md": adr("ADR-0001：甲", "Status: Accepted")},
		"标题含方括号":      {"0001-a.md": adr("# ADR-0001：甲 [乙]", "Status: Accepted")},
		"没有 Status 行": {"0001-a.md": {Data: []byte("# ADR-0001：甲\n\n## Context\n\nStatus: Accepted\n")}},
		"标记写两次":       {"0001-a.md": adr("# ADR-0001：甲", "Status: Accepted"+NoteMarker+"｜一"+NoteMarker+"｜二")},
		"标记写走了样":      {"0001-a.md": adr("# ADR-0001：甲", "Status: Accepted<!--索引注记-->｜一")},
		"取代者不存在": {
			"0001-a.md": adr("# ADR-0001：甲", "Status: Superseded by [ADR-0002](./0002-b.md)"),
		},
		"取代者文件名不对": {
			"0001-a.md": adr("# ADR-0001：甲", "Status: Superseded by [ADR-0002](./0002-x.md)"),
			"0002-b.md": adr("# ADR-0002：乙", "Status: Accepted"),
		},
		"取代者链接文字与文件不符": {
			"0001-a.md": adr("# ADR-0001：甲", "Status: Superseded by [ADR-0003](./0002-b.md)"),
			"0002-b.md": adr("# ADR-0002：乙", "Status: Accepted"),
		},
		"Superseded 后面不是链接": {"0001-a.md": adr("# ADR-0001：甲", "Status: Superseded，见 ADR-0002")},
		"号重复": {
			"0001-a.md": adr("# ADR-0001：甲", "Status: Accepted"),
			"0001-b.md": adr("# ADR-0001：乙", "Status: Accepted"),
		},
	}
	for name, fsys := range cases {
		if _, err := ReadRecords(fsys); err == nil {
			t.Errorf("%s：想要报错，却读成功了", name)
		}
	}
}

// 整条链路对一棵临时仓库跑：标记外的手写说明一字不动，-check 在重生成前后各答一次。
func TestRunRewritesOnlyTheMarkedListsAndCheckFollows(t *testing.T) {
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "adr")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(adrDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("0001-a.md", string(adr("# ADR-0001：甲", "Status: Superseded by [ADR-0002](./0002-b.md)").Data))
	write("0002-b.md", string(adr("# ADR-0002：乙", "Status: Accepted"+NoteMarker+"｜**判据**：一句").Data))
	head := "# 架构决策记录\n\n手写说明。\n\n## 已接受决策\n\n"
	middle := "\n## 已被取代决策\n\n手写一句。\n\n"
	write("README.md", head+acceptedBegin+"\n- 旧行\n"+acceptedEnd+"\n"+middle+supersededBegin+"\n"+supersededEnd+"\n")

	if err := run(root, true); err == nil {
		t.Fatal("README 还是旧的，-check 却通过了")
	}
	if err := run(root, false); err != nil {
		t.Fatalf("生成失败：%v", err)
	}
	if err := run(root, true); err != nil {
		t.Fatalf("刚生成完，-check 却不通过：%v", err)
	}

	data, err := os.ReadFile(filepath.Join(adrDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	want := head + acceptedBegin + "\n- [ADR-0002：乙](./0002-b.md)｜**判据**：一句\n" + acceptedEnd + "\n" + middle +
		supersededBegin + "\n- [ADR-0001：甲](./0001-a.md) → 由 [ADR-0002](./0002-b.md) 取代\n" + supersededEnd + "\n"
	if got != want {
		t.Errorf("README =\n%s\n想要\n%s", got, want)
	}
	if strings.Contains(got, "旧行") {
		t.Error("标记之间的旧内容没被换掉")
	}
}
