package customshttp

import (
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// writePageQueryProblem 把目录查询参数的拒绝答成 400 MALFORMED_REQUEST，理由散文原样进 detail（ADR-0144 决定一、四）。
// 各列表端点在 Intake 之前解码：查询参数属传输形状，渠道未配置时写错参数也先答 400。
func writePageQueryProblem(response http.ResponseWriter, err error) {
	detail := ""
	var malformed *cataloguepage.MalformedQuery
	if errors.As(err, &malformed) {
		detail = malformed.Reason
	}
	writeJSON(response, http.StatusBadRequest, problemResponse{
		Error: problemDetail{Code: codeMalformedRequest, Detail: detail},
	})
}
