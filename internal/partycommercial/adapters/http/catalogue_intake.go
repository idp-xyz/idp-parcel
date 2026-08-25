// Package commercialhttp 承载 party-commercial 主数据目录查阅的 HTTP 适配器
// (ADR-0077):服务产品目录与商业策略目录两个查询端点。查阅不触发判断、决定或
// 披露——本包只消费存储读面,不接应用编排,与 /shipment-request-views 同一条分界。
package commercialhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// ErrMalformedRequest 表示这次请求构造不出目录查询,且重发同样的内容不会改变结果。
var ErrMalformedRequest = errors.New("party commercial http: malformed request")

// CatalogueQuery 是一次已授权的主数据目录查阅(ADR-0077):作用域来自认证与授权
// 结果,授权边界只有租户——没有客户维;页大小由接入面按渠道契约裁决——两样都不
// 采信调用方自报。
type CatalogueQuery struct {
	Scope domain.OperationsQueryScope
	Limit int
}

// CommercialCatalogueIntake 把一次已认证的目录查阅请求翻译成查询。
//
// 它是接口而非解析代码:认证方式属 `PAR-INT-01` 待提供,采信自报租户会穿透
// ADR-0003 的隔离边界(ADR-0077 Decision 三)。未决期间本包不带任何实现,包括
// 「开发用」的采信头部版本。两个目录端点共用一个 Intake 类型:它们是同一上下文
// 同一作用域形状的运营查阅,分设只会让装配点看起来能只配一半。
type CommercialCatalogueIntake interface {
	IntakeCatalogueQuery(ctx context.Context, request *http.Request) (CatalogueQuery, error)
}

// 传输层错误码。业务判别走 `outcome`,这里只说明为什么没有 `outcome`。
const (
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeMalformedRequest = "MALFORMED_REQUEST"
	codeIntakeFailed     = "INTAKE_FAILED"
	codeNoAnswerFormed   = "NO_ANSWER_FORMED"
)

func writeCatalogueIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	if errors.Is(err, ErrMalformedRequest) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func rfc3339(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}
