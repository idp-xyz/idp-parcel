package networkhttp

import (
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// writePageQueryProblem 把列表查询参数的拒绝答成 400 MALFORMED_REQUEST，理由散文原样进 detail（ADR-0144 决定一、三、四）。
// 调用方都在 Intake 之后才解码：未配置的渠道对一切请求同答 403，答复不随查询串变（ADR-0055 决定三）。
func writePageQueryProblem(response http.ResponseWriter, err error) {
	var malformed *cataloguepage.MalformedQuery
	if errors.As(err, &malformed) {
		writeProblemWithDetail(response, http.StatusBadRequest, codeMalformedRequest, malformed.Reason)
		return
	}
	writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
}
