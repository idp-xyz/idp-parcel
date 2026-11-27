package accessidentity

import (
	"errors"
	"strings"
	"testing"
)

// 姓名去掉首尾空白再落；空白、超长（按字符数，不按字节）、带控制字符的都不是能登的姓名。
func TestNewOperatorNameTrimsAndRefusesWhatIsNotAName(t *testing.T) {
	longest := strings.Repeat("甲", MaxOperatorNameLength)
	for raw, want := range map[string]string{
		"  合成甲 ":       "合成甲",
		"SYN Operator": "SYN Operator",
		longest:        longest,
	} {
		if got, err := NewOperatorName(raw); err != nil || got != want {
			t.Fatalf("NewOperatorName(%q) = %q, %v；want %q", raw, got, err, want)
		}
	}
	for label, raw := range map[string]string{
		"空串":    "",
		"空白":    " \t ",
		"超长":    longest + "甲",
		"带换行":   "合成\n甲",
		"带 NUL": "合成\x00甲",
	} {
		if _, err := NewOperatorName(raw); !errors.Is(err, ErrInvalidOperatorName) {
			t.Fatalf("%s：err = %v, want ErrInvalidOperatorName", label, err)
		}
	}
}
