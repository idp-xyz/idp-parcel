// Package storable 判一个值能不能原样交给库。判据不属传输层：HTTP 的请求目标检查与目录游标的解码都要用它，而目录游标的
// 解码在各上下文的端口里被引用，端口不依赖传输层，所以判据放在两边都能依赖的这一处。
package storable

import (
	"strings"
	"unicode/utf8"
)

// Text 判一个值能不能当文本交给库：合法 UTF-8，且不含 NUL。PostgreSQL 的 text 两样都存不下，参数带着它们进读，库拒收，
// 读口答成 500——调用方据 5xx 退避重发，而这个请求改了才会好。失败按调用方的恢复动作分格（ADR-0029），所以它是坏请求。
func Text(value string) bool {
	return utf8.ValidString(value) && strings.IndexByte(value, 0) < 0
}
