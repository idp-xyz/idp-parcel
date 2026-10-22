package shipmenthttp

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// problemWriter 写一份问题体：状态、本包的传输层码，与只谈调用方自己载荷的 detail（没有就是空串）。
//
// 本包两种形状并存。客户面的处理器——首方客户族的提交口与撤回、资料修订、受控补充、逐包裹取消各命令口——写 RFC 9457 问题详情（ADR-0140 决定三），首方
// 契约在首个兼容性基线之前定形；操作者族各口仍写 {"error":{"code"}}，迁到问题详情另立后续票。哪个处理器用哪一种在它自己的
// 构造里定，各格怎么映射只写一份（answerIntakeFailure、answerCommandFailure），形状不同不复制映射。
type problemWriter func(status int, code, detail string)

// problemDetails 是客户面处理器的写法：媒体类型、成员与 requestId 都归 httpapi，本包只交码。
func problemDetails(response http.ResponseWriter, request *http.Request) problemWriter {
	return func(status int, code, detail string) {
		httpapi.WriteProblemWithDetail(response, request, status, code, detail)
	}
}

// legacyProblems 是操作者族各口今天的写法。
func legacyProblems(response http.ResponseWriter) problemWriter {
	return func(status int, code, detail string) {
		writeJSON(response, status, problemResponse{Error: problemDetail{Code: code, Detail: detail}})
	}
}
