package pricinghttp

import (
	"errors"
	"net/http"
	"net/url"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// decodePageQuery 按一册的声明解请求的查询串（ADR-0144 决定六）。各列表端点在 Intake 之后调它：渠道未配置时对一切请求
// 同答 403、答复不随请求内容变（ADR-0055 决定三），没有凭据的调用方也就探不出各册开放了哪些查询维度。
//
// 查询串照 url.ParseQuery 解、它报错就整条拒，不用 URL.Query()：后者把转义坏掉或带分号的那一段静默丢下，一个写坏的筛选值
// 会被当成没给、答出整册。这几册的查询串是封闭的，那一段算坏请求（httpapi.StorableRequestTarget 头注把这一格留给各口）。
func decodePageQuery(catalogue *cataloguepage.Catalogue, request *http.Request) (cataloguepage.Query, error) {
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return cataloguepage.Query{}, &cataloguepage.MalformedQuery{Reason: "查询串解不开：百分号转义不完整，或用了分号分隔"}
	}
	return catalogue.Decode(values)
}

// writePageQueryProblem 把目录查询参数的拒绝答成 400 MALFORMED_REQUEST，理由散文原样进 detail（ADR-0144 决定一、四）。
func writePageQueryProblem(response http.ResponseWriter, err error) {
	detail := ""
	var malformed *cataloguepage.MalformedQuery
	if errors.As(err, &malformed) {
		detail = malformed.Reason
	}
	writeJSON(response, http.StatusBadRequest, problemResponse{Error: problemDetail{Code: codeMalformedRequest, Detail: detail}})
}
