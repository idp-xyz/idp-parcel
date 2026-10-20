package markedregion

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	begin = "<!-- 开始 -->"
	end   = "<!-- 结束 -->"
)

func TestReplaceRewritesOnlyBetweenTheMarkers(t *testing.T) {
	content := "手写一\n\n" + begin + "\n旧的生成物\n" + end + "\n\n手写二\n"
	got, err := Replace(content, begin, end, "新的一\n新的二")
	if err != nil {
		t.Fatalf("替换失败：%v", err)
	}
	want := "手写一\n\n" + begin + "\n新的一\n新的二\n" + end + "\n\n手写二\n"
	if got != want {
		t.Errorf("替换结果 =\n%q\n想要\n%q", got, want)
	}
}

func TestReplaceRefusesMarkersThatAreNotAloneOnTheirLine(t *testing.T) {
	cases := map[string]string{
		"缺开始":   "正文\n" + end + "\n",
		"缺结束":   begin + "\n正文\n",
		"开始在行内": "前缀 " + begin + "\n" + end + "\n",
		"颠倒":    end + "\n" + begin + "\n",
		"开始重复":  begin + "\n" + begin + "\n" + end + "\n",
		"结束重复":  begin + "\n" + end + "\n" + end + "\n",
	}
	for name, content := range cases {
		if _, err := Replace(content, begin, end, "x\n"); err == nil {
			t.Errorf("%s：想要报错，却替换成功", name)
		}
	}
}

func TestSyncCheckReportsStaleWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.md")
	if err := os.WriteFile(path, []byte("旧"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Sync(path, "新", true)
	if !errors.Is(err, ErrStale) {
		t.Fatalf("只核对模式遇到不同，想要 ErrStale，得到 %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "旧" {
		t.Errorf("只核对模式写了盘：%q", data)
	}

	if err := Sync(path, "新", false); err != nil {
		t.Fatalf("写回失败：%v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "新" {
		t.Errorf("写回后内容 = %q，想要 %q", data, "新")
	}
	if err := Sync(path, "新", true); err != nil {
		t.Errorf("写回之后再核对，想要通过，得到 %v", err)
	}
}

func TestReplaceToleratesCarriageReturnsAroundMarkers(t *testing.T) {
	content := begin + "\r\n旧\r\n" + end + "\r\n"
	got, err := Replace(content, begin, end, "新\n")
	if err != nil {
		t.Fatalf("替换失败：%v", err)
	}
	if !strings.Contains(got, "新\n"+end) {
		t.Errorf("替换结果 = %q", got)
	}
}
