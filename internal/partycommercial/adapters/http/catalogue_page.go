package commercialhttp

import (
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// writePageQueryProblem 答列表查询参数不成立的那一格（ADR-0144 决定四、六）：after、sort、筛选维与 q 属传输形状，各列表
// 端点在 Intake 之前按本册声明解码，解不开即答 400，理由散文放进 detail——与 /commercial-customer-accounts 同一答法，
// 未配置渠道时写错参数也先答 400 而不是 403。
func writePageQueryProblem(response http.ResponseWriter, err error) {
	var malformed *cataloguepage.MalformedQuery
	if errors.As(err, &malformed) {
		writeProblemWithDetail(response, http.StatusBadRequest, codeMalformedRequest, malformed.Reason)
		return
	}
	writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
}
