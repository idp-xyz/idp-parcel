package referenceconfig

import (
	"encoding/json"
	"errors"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// 本文件证参考配置的三道门（ADR-0147）：引用串的形状与往返；发布清单与嵌入文件一一对应、已发布
// 原文与钉住的摘要一致；每份文件自报的标识与版本与其路径一致。

// Covers: ADR-0147 决定二、三——「标识@版本」往返，引用串带形状版本前缀；从依据格反解只认带前缀的串。
func TestAReferenceRoundTripsAndCitesWithItsShapeVersion(t *testing.T) {
	reference, err := ParseReference("party-commercial/registration-number-types/CN@1")
	if err != nil {
		t.Fatalf("解析：%v", err)
	}
	if reference.Identifier() != "party-commercial/registration-number-types/CN" || reference.Version() != 1 {
		t.Fatalf("解析出 %q @ %d", reference.Identifier(), reference.Version())
	}
	if got := reference.String(); got != "party-commercial/registration-number-types/CN@1" {
		t.Fatalf("String() = %q", got)
	}
	citation := reference.Citation()
	if citation != "REFCFG-1:party-commercial/registration-number-types/CN@1" {
		t.Fatalf("Citation() = %q", citation)
	}
	cited, isCitation := ParseCitation(citation)
	if !isCitation || cited != reference {
		t.Fatalf("ParseCitation(%q) = %v, %v", citation, cited, isCitation)
	}
	if _, isCitation := ParseCitation("SYN-BASIS-REGNO-CN-01"); isCitation {
		t.Fatalf("不带前缀的依据不是引用串")
	}
}

// Covers: 形状不对的「标识@版本」一律拒收——版本从 1 起、不带前导零；标识恰三段、上下文与目录是小写
// 短横线名。宽收会让同一版有两种写法，依据格里就会出现两串指同一份原文。
func TestAMalformedReferenceIsRefused(t *testing.T) {
	for _, text := range []string{
		"",
		"party-commercial/registration-number-types/CN",
		"party-commercial/registration-number-types/CN@0",
		"party-commercial/registration-number-types/CN@01",
		"party-commercial/registration-number-types/CN@x",
		"party-commercial/registration-number-types/CN@-1",
		"registration-number-types/CN@1",
		"party-commercial/registration-number-types/CN/extra@1",
		"Party-Commercial/registration-number-types/CN@1",
		"party_commercial/registration-number-types/CN@1",
		"party-commercial/registration-number-types/@1",
		"party-commercial/registration-number-types/CN@1@2",
	} {
		if _, err := ParseReference(text); !errors.Is(err, ErrInvalidReference) {
			t.Fatalf("ParseReference(%q) err = %v, want ErrInvalidReference", text, err)
		}
	}
	if _, isCitation := ParseCitation("REFCFG-1:CN@1"); isCitation {
		t.Fatalf("前缀对而标识形状不对的串不是引用串")
	}
}

// Covers: ADR-0147 决定二「发布即不可改写」——嵌入的每份文件都在发布清单里，清单里的每一版都有文件，
// 原文与钉住的摘要一致。改一字、漏登清单、清单指向不存在的文件，任一都在这里红。
func TestEveryEmbeddedFileIsReleasedAndMatchesItsPinnedDigest(t *testing.T) {
	var embedded []string
	if err := fs.WalkDir(assets, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".json") {
			embedded = append(embedded, strings.TrimSuffix(path, ".json"))
		}
		return nil
	}); err != nil {
		t.Fatalf("遍历嵌入文件：%v", err)
	}
	var listed []string
	for _, reference := range Released() {
		if _, err := ParseReference(reference.String()); err != nil {
			t.Fatalf("发布清单里 %q 不是合法的「标识@版本」：%v", reference, err)
		}
		listed = append(listed, reference.String())
		if _, err := Open(reference); err != nil {
			t.Fatalf("Open(%s)：%v", reference, err)
		}
	}
	sort.Strings(embedded)
	sort.Strings(listed)
	if strings.Join(embedded, "\n") != strings.Join(listed, "\n") {
		t.Fatalf("嵌入文件与发布清单不一致：\n嵌入：%v\n清单：%v", embedded, listed)
	}
	if len(listed) == 0 {
		t.Fatalf("发布清单为空")
	}
}

// Covers: 每份文件自报的标识与版本与其路径一致——路径是找回原文的键，文件头是读的人看到的名字，
// 两者不一致时依据格引的是一份、读到的是另一份。
func TestEachReleasedFileDeclaresTheReferenceOfItsPath(t *testing.T) {
	for _, reference := range Released() {
		raw, err := Open(reference)
		if err != nil {
			t.Fatalf("Open(%s)：%v", reference, err)
		}
		var header struct {
			Identifier string `json:"identifier"`
			Version    int    `json:"version"`
		}
		if err := json.Unmarshal(raw, &header); err != nil {
			t.Fatalf("%s 文件头：%v", reference, err)
		}
		if header.Identifier != reference.Identifier() || header.Version != reference.Version() {
			t.Fatalf("%s 自报 %q @ %d", reference, header.Identifier, header.Version)
		}
	}
}

// Covers: 没发布的版本打不开——未采用就没有「最接近的一版」可顶替。
func TestAnUnreleasedReferenceDoesNotOpen(t *testing.T) {
	reference, err := ParseReference("party-commercial/registration-number-types/XA@1")
	if err != nil {
		t.Fatalf("解析：%v", err)
	}
	if _, err := Open(reference); !errors.Is(err, ErrNotReleased) {
		t.Fatalf("Open(未发布) err = %v, want ErrNotReleased", err)
	}
}
