package httpapi

import (
	"net/http"
	"net/url"

	"go.idp.xyz/idp-parcel/internal/platform/storable"
)

// StorableText 判一个值能不能当文本交给库，判据与理由在 storable.Text——目录游标的解码也用那一份，而它不该依赖传输层。
// 各口判单个值、本包判整条请求目标，都经这一处转到那里。
func StorableText(value string) bool {
	return storable.Text(value)
}

// StorableRequestTarget 判请求目标解码之后是不是处处 StorableText：路径，与查询串里每一个键、每一个值。
//
// 判整条目标、不只判各口读的那几格：哪几格会进读只有处理器知道，在它之前判就只能一视同仁。口不读的键带着 NUL 也一并答坏请求，
// 这条请求目标本就不是文本。
//
// 查询串照 url.ParseQuery 解：转义坏掉或带分号的那一段它丢下不交，URL.Query() 同样丢下，处理器读不到，这里也就不判；那一段算
// 不算坏请求归各口自己的解码（封闭查询串的口拿 ParseQuery 的错答 400）。路径取解码后的 URL.Path，静态段都是 ASCII，判得到的
// 只有路径参数那几段。
func StorableRequestTarget(request *http.Request) bool {
	if !StorableText(request.URL.Path) {
		return false
	}
	values, _ := url.ParseQuery(request.URL.RawQuery)
	for key, given := range values {
		if !StorableText(key) {
			return false
		}
		for _, value := range given {
			if !StorableText(value) {
				return false
			}
		}
	}
	return true
}

// RefuseUnstorableReadTarget 包住一个读口：GET 请求的目标不是 StorableRequestTarget，就交 refuse 答坏请求，处理器一步不走，
// 认证与读库都不发生。别的方法原样交给处理器：方法门由处理器自守（见 BusinessEndpoint），错方法照旧答它自己的 405。
//
// 坏请求长什么样不归本包定：各族的形状由装配处按族交进来，本包只判、只拦。
func RefuseUnstorableReadTarget(next http.Handler, refuse http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && !StorableRequestTarget(request) {
			refuse(response, request)
			return
		}
		next.ServeHTTP(response, request)
	})
}
