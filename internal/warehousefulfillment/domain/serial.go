package domain

import (
	"strings"
	"unicode/utf8"
)

// 序列号只在出库包裹上出现，不进库存格。过长的号多半是扫进了别的码，先挡住。
const serialMaxRunes = 64

// CanonicalSerial 去掉两端空白。空的、含 NUL 的、超过长度的不收。
func CanonicalSerial(raw string) (string, bool) {
	serial := strings.TrimSpace(raw)
	if serial == "" || strings.ContainsRune(serial, '\x00') || utf8.RuneCountInString(serial) > serialMaxRunes {
		return "", false
	}
	return serial, true
}
