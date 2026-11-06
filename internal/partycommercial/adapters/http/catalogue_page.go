package commercialhttp

import (
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// writePageQueryProblem 答列表查询参数不成立的那一格（ADR-0144 决定一、四）：after、sort、筛选维与 q 按本册声明解不开即答
// 400，理由散文原样进 detail。各列表端点在 Intake 之后解码：渠道未配置时对一切请求同答 403（ADR-0055 决定三），没有
// 凭据的调用方也就探不出各册开放了哪些查询维度。
func writePageQueryProblem(response http.ResponseWriter, err error) {
	var malformed *cataloguepage.MalformedQuery
	if errors.As(err, &malformed) {
		writeProblemWithDetail(response, http.StatusBadRequest, codeMalformedRequest, malformed.Reason)
		return
	}
	writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
}
