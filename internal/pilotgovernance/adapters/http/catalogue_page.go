package governancehttp

import (
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// writePageQueryProblem 把目录查询参数的拒绝答成 400 MALFORMED_REQUEST，理由散文原样进 detail（ADR-0144 决定一、四）。
// 列表端点在 Intake 之后解码：渠道未配置时对一切请求同答 403（ADR-0055 决定三），没有凭据的调用方也就探不出各册开放了
// 哪些查询维度。
func writePageQueryProblem(response http.ResponseWriter, err error) {
	detail := ""
	var malformed *cataloguepage.MalformedQuery
	if errors.As(err, &malformed) {
		detail = malformed.Reason
	}
	writeProblemWithDetail(response, http.StatusBadRequest, codeMalformedRequest, detail)
}
