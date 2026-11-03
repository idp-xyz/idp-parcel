package main

import (
	"mime"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件给写口挂请求体检查（票 document-numbering/16）：JSON 请求体里的字符串含 NUL、原始字节不是合法 UTF-8、或 \u 转义出
// 孤代理，处理器读请求体时读到的是一个错，各口照自己「请求体读不出」那一格答 400，处理器之后的写一笔都不发生。修之前，NUL
// 进了写，库拒收，答成可重试的 500 NO_ANSWER_FORMED；非法 UTF-8 与孤代理被 encoding/json 换成 U+FFFD 照常受理，落库的已不是
// 调用方送来的值。判据只在 httpapi 一处：StorableJSONBody，与放过 NUL 的 UnrewrittenJSONBody 共用一个扫描。构造门自己判
// NUL、答着既有一格的上下文，NUL 留给那道门，只判后两种（bodyNULExemptions）。
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

// bodyNULExemptions 是请求体里的 NUL 交给处理器自判的上下文，键是该上下文各行共有的路径前缀，值是豁免理由。照
// targetCheckExemptions 的判法，进这张表要两条同时成立：
//
//   - 修前 NUL 就不答 500：这一上下文的构造门按产品规则拒 NUL，答它既有的一格；
//   - 挂上会改那一格：构造门答的多是业务答案（200 的未受理一类），不是本检查答的 400 MALFORMED_REQUEST。
//
// 只放过 NUL：非法 UTF-8 与孤代理照判（httpapi.UnrewrittenJSONBody），那两种修前是悄悄改写成 U+FFFD，没有哪一格是为它们设的。
// 按上下文豁免、不逐行列：判 NUL 的是上下文的构造门，规则管整个上下文，这一上下文新加的写口照样归那道门。前缀必须罩得住登记处
// 声明过的写口，由 TestBodyNULExemptionsCoverDeclaredWriteLines 核；别的上下文修前 NUL 进写答 500，或构造门本就答 400
// MALFORMED_REQUEST（节点作业、编号规则册、首方提交的外部标识号），挂上不改格，不进表。
var bodyNULExemptions = map[string]string{
	"/transport-fulfillment": "运输履约的构造门拒 NUL、答未受理（票 tf-face-algebra/01 与 /14 的裁定，ADR-0213、ADR-0303；" +
		"承运订舱答复的对接文档把它写成 INPUT_NOT_ACCEPTED）",
	"/customs": "关务合规的必备值构造门拒 NUL、答「改输入」（domain 的 newRequiredValue 与报送事实门），" +
		"在编排里构造的格答未受理一类业务答案（如技术回执），在译装时构造的格答 400",
}

// requestBodyGuard 交回一行的请求体检查；nil 即这一行不挂。
//
// GET 行不读请求体，不挂。bodyNULExemptions 罩住的行只判非法 UTF-8 与孤代理。声明了 multipartUpload 的行，请求体是
// multipart/form-data 时放过：上传的是二进制工作簿，不是 JSON 文本。这几行的处理器照同一个头分派，multipart 走上传解码，
// application/json 走结构化工作簿，所以放过的恰是处理器不按 JSON 解的那一种。别的行不看 Content-Type：它们的处理器不看这个头，
// 一律按 JSON 解，按头放过就是给坏请求体留一扇门。
func requestBodyGuard(pattern string, line endpointLine) func(http.Handler) http.Handler {
	if line.method == http.MethodGet {
		return nil
	}
	require := httpapi.RequireStorableJSONBody
	if bodyNULJudgedByHandler(pattern) {
		require = httpapi.RequireUnrewrittenJSONBody
	}
	if !line.multipartUpload {
		return require
	}
	return func(next http.Handler) http.Handler {
		guarded := require(next)
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type")); err == nil && mediaType == "multipart/form-data" {
				next.ServeHTTP(response, request)
				return
			}
			guarded.ServeHTTP(response, request)
		})
	}
}

func bodyNULJudgedByHandler(pattern string) bool {
	for prefix := range bodyNULExemptions {
		if strings.HasPrefix(pattern, prefix) {
			return true
		}
	}
	return false
}

// requestBodyGuards 在登记时为一个特性的各行算好请求体检查。
func requestBodyGuards(lines map[string]endpointLine) map[string]func(http.Handler) http.Handler {
	guards := map[string]func(http.Handler) http.Handler{}
	for pattern, line := range lines {
		if guard := requestBodyGuard(pattern, line); guard != nil {
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
