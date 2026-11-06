package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件给读口挂请求目标检查（票 document-numbering/15）：GET 行的路径参数或查询值含 NUL、不是合法 UTF-8，在处理器之前答
// 400，不认证、不读库。这样的值原样进 SQL 参数，库拒收，各口答成 500 NO_ANSWER_FORMED——调用方据 5xx 重发一个永远不会好的
// 请求（ADR-0029 按恢复动作分格）。挂在登记处而不逐口去改：哪几格进读散在各上下文的处理器里，逐口改总有漏的，新加的口也得
// 有人记得；登记处知道每一行的方法与族，加一行 GET 自动带上。判据只有 httpapi.StorableText 一份。
//
// 检查在认证之前。方法门仍归处理器，错方法照旧答 405；渠道未配置时坏目标答 400 而不是 403，理由与各口把查询参数的在场与封闭集
// 当传输形状、先于 Intake 判相同（如 /tracking-projections 两个定位参数同在即 400）。它不泄露「本端点认哪些键」：判的是整条
// 目标，每一格一视同仁，答复与端点、与键都无关。首方客户族例外，见 unstorableTargetRefusal。
//
// 与 ADR-0055 决定三「答复对一切请求内容与自报身份一致」不相抵：请求目标解码后是不是文本属传输形状，与方法、与请求行能不能
// 解析同一层，不算请求内容。未配置格对一切能解成文本的请求仍同答 403；解不成文本的请求，同 net/http 对解析不了的请求行先答
// 400 一样，到不了任何一格 Intake。

// targetCheckExemptions 是不挂检查的 GET 行，值是豁免理由。进这张表要两条同时成立：
//
//   - 这一行不把任何查询值或路径段带进库，修前就不答 500，检查在这里护不住任何东西；
//   - 它的坏请求分格里没有 MALFORMED_REQUEST，挂上就凭空多一格，改掉既有分格（票面「不做」）。
//
// 只成立前一条的不进：坏请求分格本有 MALFORMED_REQUEST 的口，检查答的仍是它自己的一格，新加的 GET 行默认挂上。豁免只在这一处
// 写，不散到各特性的行声明里；表里的路径必须是登记处声明过的 GET 行，由 TestTargetCheckExemptionsNameDeclaredReadLines 核。
var targetCheckExemptions = map[string]string{
	"/reference-data/countries":  referenceDataExemption,
	"/reference-data/currencies": referenceDataExemption,
	"/reference-data/units":      referenceDataExemption,
	"/operator-binding":          operatorIdentityExemption,
	"/operator-grants":           operatorIdentityExemption,
}

const (
	referenceDataExemption = "不把查询值带进库，既有分格不动：q、region、category 只在内存目录上检索与比对；" +
		"包头契约的失败格是 REGION_NOT_RECOGNIZED / CATEGORY_NOT_RECOGNIZED，没有 MALFORMED_REQUEST"
	operatorIdentityExemption = "不把查询值带进库，既有分格不动：这几口根本不读查询串，也没有路径参数；读口的码集里没有 MALFORMED_REQUEST"
)

// unstorableTargetRefusal 交回一行在请求目标不是文本时答的坏请求；nil 即这一行不在装配处挂检查。
//
// 只挂 GET 行：命令口不读查询串，挂上去只会让一条今天照常受理的命令因为一个它不读的键被拒。targetCheckExemptions 里的行不挂。
// 首方客户族不挂：本族先认证、后读查询串，令牌不过的调用方看不到查询串校验的结果，所以它的读口在 Intake 里、认证之后判
// （visibilityhttp 的 refuseUnstorableQueryValue，判据同一份）。
//
// 挂旧形状的只列今天有 GET 行、处理器写的坏请求确是 {"error":{"code"}} 的族：主数据与运营查阅读、登记册配置写、隔离读、操作者
// 接入管理。别的族哪天挂上 GET 行，先在这里定它的坏请求形状——不预先认定，族没列到就在登记处的 init 里停。哪一族迁到问题详情
// （ADR-0140 决定三的后续票），就把它从旧形状那一格挪出去。
func unstorableTargetRefusal(pattern string, line endpointLine) http.HandlerFunc {
	if line.method != http.MethodGet {
		return nil
	}
	if _, exempt := targetCheckExemptions[pattern]; exempt {
		return nil
	}
	switch line.face.family {
	case faceCustomerClient:
		return nil
	case faceOperationsRead, faceRegistryWrite, faceIsolatedRead, faceOperatorAccessAdministration:
		return writeLegacyMalformedRequest
	}
	panic(fmt.Sprintf("parcel-api: GET endpoint %s declares face family %q, whose bad-request shape is not settled here", pattern, line.face.family))
}

// writeLegacyMalformedRequest 写旧形状的坏请求：`{"error":{"code":"MALFORMED_REQUEST"}}`，application/json，与各上下文处理器
// 自己答的那一份同字节（TestUnstorableReadValuesAreBadRequestsAgainstARealDatabase 拿 /tracking-projections 处理器自己答的那份
// 逐字节比）。不带 detail：旧形状里带 detail 的只是少数上下文，统一带上会给其余各口凭空多出一格。
func writeLegacyMalformedRequest(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(response).Encode(legacyProblem{Error: legacyProblemCode{Code: "MALFORMED_REQUEST"}})
}

type legacyProblem struct {
	Error legacyProblemCode `json:"error"`
}

type legacyProblemCode struct {
	Code string `json:"code"`
}

// unstorableTargetRefusals 在登记时为一个特性的各行算好拒法：族写漏在 init 里就停，不等到开池之后的装配。
func unstorableTargetRefusals(lines map[string]endpointLine) map[string]http.HandlerFunc {
	refusals := map[string]http.HandlerFunc{}
	for pattern, line := range lines {
		if refuse := unstorableTargetRefusal(pattern, line); refuse != nil {
			refusals[pattern] = refuse
		}
	}
	return refusals
}

// guardReadTargets 给 mount 出来的端点逐行套上检查。生产装配与装配测试都经登记处的同一处调用它，两边挂的是同一道检查。
func guardReadTargets(refusals map[string]http.HandlerFunc, mounted []httpapi.BusinessEndpoint) []httpapi.BusinessEndpoint {
	guarded := make([]httpapi.BusinessEndpoint, 0, len(mounted))
	for _, endpoint := range mounted {
		if refuse, ok := refusals[endpoint.Pattern]; ok {
			endpoint.Handler = httpapi.RefuseUnstorableReadTarget(endpoint.Handler, refuse)
		}
		guarded = append(guarded, endpoint)
	}
	return guarded
}
