package nodeopshttp

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// problemWriter 写一份问题体：状态、本包的传输层码，与只谈调用方自己载荷的 detail（没有就是空串）。
//
// 本包两种形状并存。首方客户族各口——送站预报的提交与撤回、交接单的客户结论——写 RFC 9457 问题详情（ADR-0140 决定三），
// 首方契约在首个兼容性基线之前定形；作业事实与操作者族各口仍写 {"error":{"code"}}，迁到问题详情另立后续票。哪个口用哪一种
// 在它自己的构造里定，各格怎么映射只写一份。
type problemWriter func(status int, code, detail string)

// problemDetails 是首方客户族各口的写法：媒体类型、成员与 requestId 都归 httpapi，本包只交码。
func problemDetails(response http.ResponseWriter, request *http.Request) problemWriter {
	return func(status int, code, detail string) {
		httpapi.WriteProblemWithDetail(response, request, status, code, detail)
	}
}

// legacyProblems 是作业事实与操作者族各口今天的写法，旧形状不带 detail。
func legacyProblems(response http.ResponseWriter, _ *http.Request) problemWriter {
	return func(status int, code, _ string) {
		writeProblem(response, status, code)
	}
}
