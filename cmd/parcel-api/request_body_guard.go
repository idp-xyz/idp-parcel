package main

import (
	"mime"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件给写口挂请求体检查（票 document-numbering/16）：JSON 请求体里的字符串含 NUL、原始字节不是合法 UTF-8、或 \u 转义出
// 孤代理，处理器读请求体时读到的是一个错，各口照自己「请求体读不出」那一格答 400，处理器之后的写一笔都不发生。修之前，NUL
// 进了写，库拒收，答成可重试的 500 NO_ANSWER_FORMED；非法 UTF-8 与孤代理被 encoding/json 换成 U+FFFD 照常受理，落库的已不是
// 调用方送来的值。判据只有 httpapi.StorableJSONBody 一份。
//
// 不按上下文豁免 NUL：运输履约、关务合规的构造门有的格拒 NUL、答 200 未受理，可同一上下文里也有只判空白的自由文本格，NUL
// 照旧进写、答 500（如送站派送重问节奏登记与关务案件要件规则登记的 basis）。豁免罩不准哪一格，逐格核验又不封顶；在入站一律
// 拦下，才确定哪一格都不出 500。
// 那几格修前答的未受理因此改成 400，两格都是「改输入」，恢复动作不变（ADR-0029）。
//
// 挂在登记处，与读口检查（guardReadTargets）同一处、同一个理由：没有各上下文共用的解码件，各口解请求体的助手散在各自的适配器
// 里，逐个改总有漏的，新加的口也得有人记得；登记处知道每一行的方法，加一行写口自动带上。
//
// 与读口检查不同，这里自己不答，只换请求体：
//   - 各口先认证、后读请求体，令牌不过的请求，请求体一个字节都不读。在处理器之前判，就得先把请求体读进来，那正是各口守着不
//     做的事。边读边判不动这个次序。
//   - 各口「请求体读不出」那一格已经是 400 坏请求，形状归族：操作者各族是旧形状，首方族是问题详情。读错落进那一格，登记处
//     不用再替各族写一份形状，首方族也不必另挂。这一格靠各口的解码助手把读错一律包成坏请求；哪个口把读错答成别的格，
//     TestEveryWriteLineReadsItsBodyThroughTheCheck 与 TestEveryOwnIntakeWriteLineReadsItsBodyThroughTheCheck 逐行就红。

// requestBodyGuard 交回一行的请求体检查；nil 即这一行不挂。
//
// GET 行不读请求体，不挂。声明了 multipartUpload 的行，请求体是 multipart/form-data 时放过：上传的是二进制工作簿，不是 JSON
// 文本；表单里唯一的文本、文件名，由上传解码自己判（pricinghttp 的 DecodePriceCardUpload）。这几行的处理器照同一个头分派，
// multipart 走上传解码，application/json 走结构化工作簿，所以放过的恰是处理器不按 JSON 解的那一种。别的行不看 Content-Type：
// 它们的处理器不看这个头，一律按 JSON 解，按头放过就是给坏请求体留一扇门。
func requestBodyGuard(line endpointLine) func(http.Handler) http.Handler {
	if line.method == http.MethodGet {
		return nil
	}
	if !line.multipartUpload {
		return httpapi.RequireStorableJSONBody
	}
	return func(next http.Handler) http.Handler {
		guarded := httpapi.RequireStorableJSONBody(next)
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type")); err == nil && mediaType == "multipart/form-data" {
				next.ServeHTTP(response, request)
				return
			}
			guarded.ServeHTTP(response, request)
		})
	}
}

// requestBodyGuards 在登记时为一个特性的各行算好请求体检查。
func requestBodyGuards(lines map[string]endpointLine) map[string]func(http.Handler) http.Handler {
	guards := map[string]func(http.Handler) http.Handler{}
	for pattern, line := range lines {
		if guard := requestBodyGuard(line); guard != nil {
			guards[pattern] = guard
		}
	}
	return guards
}

// guardRequestBodies 给 mount 出来的端点逐行套上请求体检查。生产装配与装配测试都经登记处的同一处调用它，两边挂的是同一道。
func guardRequestBodies(guards map[string]func(http.Handler) http.Handler, mounted []httpapi.BusinessEndpoint) []httpapi.BusinessEndpoint {
	guarded := make([]httpapi.BusinessEndpoint, 0, len(mounted))
	for _, endpoint := range mounted {
		if guard, ok := guards[endpoint.Pattern]; ok {
			endpoint.Handler = guard(endpoint.Handler)
		}
		guarded = append(guarded, endpoint)
	}
	return guarded
}
