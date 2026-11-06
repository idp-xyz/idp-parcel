package identityhttp

import (
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// operatorCatalogueReadLimit 是本包查阅口（作业设备册、操作者册）的页大小。页大小归渠道契约，不采信请求（ADR-0144
// 决定二）；本包的认证件只交回租户与主体、不带页大小，所以定在这里。不随租户变。
const operatorCatalogueReadLimit = 200

// writePageQueryProblem 把目录查询参数的拒绝答成 400 MALFORMED_REQUEST，理由散文原样进 detail（ADR-0144 决定一、四）。
// 查阅口在认证之后解码：渠道未配置时对一切请求同答 403（ADR-0055 决定三），没有凭据的调用方也就探不出本册开放了哪些
// 查询维度。
func writePageQueryProblem(response http.ResponseWriter, err error) {
	detail := ""
	var malformed *cataloguepage.MalformedQuery
	if errors.As(err, &malformed) {
		detail = malformed.Reason
	}
	writeProblemWithDetail(response, http.StatusBadRequest, codeMalformedRequest, detail)
}
