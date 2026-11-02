package domain

import (
	"errors"
	"strings"
	"testing"
)

// Covers: AT-NR-111（领域半边）——名称给了就要成形：恰在上限照收、原样保留；空白、首尾空白、超长、控制 / 格式 / 行段
// 分隔字符、坏 UTF-8 一律拒，不截断不改写。零值即没登。
func TestANodeDisplayNameIsTakenAsWrittenOrRefused(t *testing.T) {
	accepted := []string{
		"上海枢纽",
		"Singapore Hub",
		"深圳 皇岗 口岸",
		strings.Repeat("枢", NodeDisplayNameMaxRunes),
	}
	for _, text := range accepted {
		name, err := NewNodeDisplayName(text)
		if err != nil {
			t.Fatalf("NewNodeDisplayName(%q) = %v, want accepted", text, err)
		}
		if !name.Registered() || name.String() != text {
			t.Fatalf("NewNodeDisplayName(%q) kept %q (registered %v), want it verbatim", text, name.String(), name.Registered())
		}
	}

	refused := map[string]string{
		"empty":                "",
		"blank":                "   ",
		"leading space":        " 上海枢纽",
		"trailing space":       "上海枢纽 ",
		"trailing nbsp":        "上海枢纽 ",
		"one rune too many":    strings.Repeat("枢", NodeDisplayNameMaxRunes+1),
		"newline inside":       "上海\n枢纽",
		"tab inside":           "上海\t枢纽",
		"bidi override":        "上海‮枢纽",
		"zero width joiner":    "上海‍枢纽",
		"line separator":       "上海 枢纽",
		"paragraph separator":  "上海 枢纽",
		"invalid utf-8":        "上海\xff",
		"delete control":       "上海\x7f",
		"next line control":    "上海\u0085枢纽",
		"escape control":       "上海\x1b枢纽",
		"null byte":            "上海\x00枢纽",
		"only zero width char": "​",
	}
	for label, text := range refused {
		name, err := NewNodeDisplayName(text)
		if !errors.Is(err, ErrNodeDisplayNameMalformed) {
			t.Fatalf("%s: NewNodeDisplayName(%q) = %v, want ErrNodeDisplayNameMalformed", label, text, err)
		}
		if name.Registered() {
			t.Fatalf("%s: a refused name must not come back registered", label)
		}
	}

	if (NodeDisplayName{}).Registered() {
		t.Fatal("the zero value must read as no name registered")
	}
}
