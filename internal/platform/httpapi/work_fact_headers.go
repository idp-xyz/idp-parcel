package httpapi

import (
	"net/http"
	"strings"
)

// 作业事实登记面的设备与作业范围走请求头，不进事实载荷（ADR-0023：载荷是设备签发的事实内容，身份由接入层铸造并核验，
// 载荷里自报的一份不被采信）。这三个头是设备与服务端之间的线格式，节点作业与运输履约两个上下文的作业事实口读的是同一组名字，
// 所以头名与「读三个头」只在这里定义一次；两个上下文不能互相导入，共用的部分只能落在平台层。
const (
	HeaderWorkDevice         = "Parcel-Device-Id"
	HeaderWorkScopeKind      = "Parcel-Work-Scope-Kind"
	HeaderWorkScopeReference = "Parcel-Work-Scope-Reference"
)

// ReadWorkFactHeaders 读出作业事实面的三个头，各自去掉两端空白后交回；complete 在三个都非空时为真。
//
// 这一层只管读：作业范围种类认哪一种、缺头时答什么，都是调用方所在上下文的事——平台层不判种类，也不造任何上下文的
// 「载荷不成形」错误。纯空白的头值与缺头同算缺。调用方须先看 complete，再使用三个值。
func ReadWorkFactHeaders(request *http.Request) (device, kind, reference string, complete bool) {
	device = strings.TrimSpace(request.Header.Get(HeaderWorkDevice))
	kind = strings.TrimSpace(request.Header.Get(HeaderWorkScopeKind))
	reference = strings.TrimSpace(request.Header.Get(HeaderWorkScopeReference))
	return device, kind, reference, device != "" && kind != "" && reference != ""
}
