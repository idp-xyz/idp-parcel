package httpapi

import (
	"net/http"
	"strings"
)

// BearerToken 取 `Authorization: Bearer …` 的令牌（RFC 6750 第 2.1 节，方案名不分大小写）。缺席或不是 Bearer 即空串：
// 空串交给核验方答「令牌缺失」，这一层不替它判，也不看令牌长什么样。
func BearerToken(request *http.Request) string {
	scheme, token, found := strings.Cut(request.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
