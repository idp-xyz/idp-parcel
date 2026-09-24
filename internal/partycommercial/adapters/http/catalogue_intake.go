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
// 它是接口而非解析代码:认证归操作者渠道(ADR-0100),其真 Intake 未就位,采信自报租户会穿透
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
	// Detail 是散文不是代数（纪律同 registrationAnswer.Cause）：前端原样示出、不查表、不据此分支。它把 Intake 已经写在
	// 错误里的拒绝理由交到线的这一头——同一个 MALFORMED_REQUEST 底下是自报租户、未知键还是多出一项，只交 code 时多种
	// 错在线上长一张脸，操作者只能挨个试。要可判别的理由代数得在用例侧立封闭枚举（先例 CatalogRefusalReason），不在
	// 传输层按字符串拼；调用侧一旦对着这些句子分支，措辞改一个字就会拆掉它。
	//
	// 只随 4xx 在场。5xx 的成因是依赖故障的内部原文，不外泄。
	Detail string `json:"detail,omitempty"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

// writeProblemWithDetail 是 writeProblem 带理由散文的兄弟。单立一个函数而不是给 writeProblem 加变参：交不交 detail 在
// 调用点要一眼读得出来——5xx（INTAKE_FAILED / NO_ANSWER_FORMED）照旧走 writeProblem 只交 code，那是依赖故障的内部原文，
// 不外泄；只有「改载荷才会好」的 4xx 才有理由让登记方看见。detail 为空时那一格缺席，不写空串。
func writeProblemWithDetail(response http.ResponseWriter, status int, code string, detail string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code, Detail: detail}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func rfc3339(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}
