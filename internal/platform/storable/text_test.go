package storable_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/storable"
)

// Covers: 库存得下的文本才放行——合法 UTF-8 且不含 NUL；空串与多字节文字照常是文本。
func TestTextIsWhatTheDatabaseCanStore(t *testing.T) {
	cases := map[string]struct {
		value string
		want  bool
	}{
		"空串":      {value: "", want: true},
		"ASCII":   {value: "SYN-PARCEL-1", want: true},
		"中文":      {value: "合成租户", want: true},
		"带 NUL":   {value: "SYN\x00PARCEL", want: false},
		"非 UTF-8": {value: "SYN\xffPARCEL", want: false},
		"截断的多字节字": {value: "合"[:2], want: false},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if got := storable.Text(test.value); got != test.want {
				t.Fatalf("storable.Text(%q) = %v，应为 %v", test.value, got, test.want)
			}
		})
	}
}
