package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

// ProblemMediaType 是 4xx/5xx 的媒体类型（RFC 9457；ADR-0140 决定三）。
const ProblemMediaType = "application/problem+json"

// problemTypeNamespace 拼出每个错误码的 `type`。取 tag URI（RFC 4151）而不是一个 https 地址：`type` 要的是本产品拥有、永不
// 改变的标识，可解引用的文档页落在哪个域名今天没有人定过，先写一个可访问地址就等于替部署做了决定，日后换地址又成了破坏性
// 变更。tag URI 只需要一个某时刻归本方所有的域名与那个日期，不必解析，也不像自造 NID 的 URN 那样要登记命名空间。
const problemTypeNamespace = "tag:idp.xyz,2026:problem:"

// problemBody 是问题详情的成员。错误码与它的含义归各处理器包（本包只收码不定码）；`title` 从码机械派生，同一个码永远同一句，
// 不另立一张码到短语的表——两张表会漂。
type problemBody struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// WriteProblem 写一份不带 detail 的问题详情。它不是业务答案，只说明为什么没有 `outcome`（ADR-0022）。
func WriteProblem(response http.ResponseWriter, request *http.Request, status int, code string) {
	WriteProblemWithDetail(response, request, status, code, "")
}

// WriteProblemWithDetail 写一份问题详情，detail 只说调用方自己这份载荷错在哪。403 与 404 受探针同答约束、5xx 是依赖故障的
// 内部原文，这几格交来的 detail 一律不写出去（ADR-0140 决定三）：守在这一处，任何一个处理器都没法把它漏出去。
//
// `requestId` 取 middleware.RequestID 放进上下文的值，是集成方与运维对账的唯一句柄；请求没经过那个中间件就没有这个值，整键不出现，
// 不在这里另造一个。
func WriteProblemWithDetail(response http.ResponseWriter, request *http.Request, status int, code, detail string) {
	body := problemBody{
		Type:   problemTypeNamespace + code,
		Title:  problemTitle(code),
		Status: status,
		Code:   code,
	}
	if detailAllowed(status) {
		body.Detail = detail
	}
	if request != nil {
		body.RequestID = middleware.GetReqID(request.Context())
	}
	response.Header().Set("Content-Type", ProblemMediaType)
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}

func detailAllowed(status int) bool {
	return status >= 400 && status < 500 && status != http.StatusForbidden && status != http.StatusNotFound
}

// problemTitle 把 `ACCESS_CHANNEL_NOT_CONFIGURED` 写成 `Access channel not configured`。
func problemTitle(code string) string {
	words := strings.ToLower(strings.ReplaceAll(code, "_", " "))
	if words == "" {
		return ""
	}
	return strings.ToUpper(words[:1]) + words[1:]
}
