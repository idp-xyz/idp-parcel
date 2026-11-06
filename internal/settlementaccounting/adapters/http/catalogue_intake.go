// Package settlementhttp 承载 settlement-accounting 四张管理台页的目录查阅 HTTP
// 适配器（ADR-0077，票 admin-skeleton-closure-batch/04）：费用与计费、对账单、
// 收付款核销、经营核算各一个查询端点。
//
// 查阅不确认费用、不发布对账单、不形成映射或核销、不派生经营指标——本包只消费存储
// 读面，不接应用编排（ADR-0077 Decision 一），与 /collection-subledgers 同一条分界。
// 这条分界在本上下文尤其吃紧：25 张表的写入方全在事务链上，读面一旦长出「处置」
// 参数，就会变成一条绕开用例的第二写路。
package settlementhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ErrMalformedRequest 表示接入面认定请求形状立不起来，且重发同样的内容不会改变结果
// （与未配置、依赖故障分三格，恢复动作不同，判据同 ADR-0029：这一格要调用方改报文）。
var ErrMalformedRequest = errors.New("settlement accounting http: malformed request")

// CatalogueQuery 是一次已授权的主数据目录查阅（ADR-0077）：作用域来自认证与授权
// 结果，授权边界只有租户——责任法人、结算账户与币种是账上的归属维，不是查阅方身份
// （见 domain.OperationsQueryScope）；页大小由接入面按渠道契约裁决。两样都不采信
// 调用方自报。
type CatalogueQuery struct {
	Scope domain.OperationsQueryScope
	Limit int
}

// CatalogueQueryIntake 把一次已认证的运营查阅请求翻译成查询。
//
// 它是接口而非解析代码：请求方身份与租户必须同时核对，运营接入面的认证归
// 操作者渠道（ADR-0100），真实现是 OperatorOperationsReadIntake；采信自报租户会穿透 ADR-0003 的隔离边界（ADR-0077
// Decision 三）。本包不带任何采信实现，包括「开发用」的采信头部版本。
//
// 各端点共用一个 Intake 类型：它们是同一上下文同一作用域形状的运营查阅，分设只会
// 让装配点看起来能只配一半（判据同 pricinghttp 同名接口）。
type CatalogueQueryIntake interface {
	IntakeCatalogueQuery(ctx context.Context, request *http.Request) (CatalogueQuery, error)
}

// 传输层错误码。业务判别走 `outcome`，这里只说明为什么没有 `outcome`。
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
	if errors.Is(err, ErrOperatorCredentialRejected) {
		writeProblem(response, http.StatusUnauthorized, codeOperatorCredentialRejected)
		return
	}
	if errors.Is(err, ErrOperatorNotGranted) {
		writeProblem(response, http.StatusForbidden, codeOperatorNotGranted)
		return
	}
	if errors.Is(err, ErrIntegrationClientCredentialRejected) {
		writeProblem(response, http.StatusUnauthorized, codeIntegrationClientCredentialRejected)
		return
	}
	if errors.Is(err, ErrIntegrationClientNotGranted) {
		writeProblem(response, http.StatusForbidden, codeIntegrationClientNotGranted)
		return
	}
	if errors.Is(err, ErrOutsideAdmissionScope) {
		writeProblem(response, http.StatusForbidden, codeOutsideAdmissionScope)
		return
	}
	if errors.Is(err, ErrIdentityDependencyUnavailable) {
		writeProblem(response, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
		return
	}
	if errors.Is(err, ErrMalformedRequest) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

// 各列表端点的门次序一致：方法 → 准入 → 册名 → 翻页参数（翻页参数见 writePageQueryProblem）。
// 册名与翻页参数都排在准入之后：渠道未配置时答复对一切请求内容一致（ADR-0055 决定三），认不认得
// 册名、参数写没写对都不先于准入作答，否则没配好渠道的调用方能从 400 与 403 的分别里探出各册收哪些
// 参数。方法判的是这个路由收不收这个动词，不读请求内容，故在准入之前。两个助手交回 false 时答复
// 已写出，调用方直接返回。

func guardGet(response http.ResponseWriter, request *http.Request) bool {
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", http.MethodGet)
		writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
		return false
	}
	return true
}

func intakeCatalogueQuery(
	response http.ResponseWriter,
	request *http.Request,
	intake CatalogueQueryIntake,
) (CatalogueQuery, bool) {
	query, err := intake.IntakeCatalogueQuery(request.Context(), request)
	if err != nil {
		writeCatalogueIntakeProblem(response, err)
		return CatalogueQuery{}, false
	}
	return query, true
}

type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
	// Detail 是给操作者看的散文，只随「改请求才会好」的 4xx 在场，前端原样示出、不据此分支
	// （同 party-commercial 的同名一格）；没有可说的时候整键不出现。
	Detail string `json:"detail,omitempty"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeProblemWithDetail(response, status, code, "")
}

func writeProblemWithDetail(response http.ResponseWriter, status int, code, detail string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code, Detail: detail}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	// 编码失败无从补救：状态行已写出，只能留给传输层中断。
	_ = json.NewEncoder(response).Encode(value)
}

// minorAmount 把最小单位金额转写成十进制计数串：int64 直投 JSON number 在 2^53 以上
// 的取值会被 JS 读者悄悄取整——串是照实转写，数才是替读者做的算术承诺；小数位属币种
// 语义，本读面不代判精度。
func minorAmount(value int64) string {
	return strconv.FormatInt(value, 10)
}

func rfc3339(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}

// optionalInstant 把可空时刻转写成串，未发生时交回空串配 omitempty 整键不出现。
// 缺席是真话不是缺陷：不为「未确认」「未作废」「未撤销」编造一个零时刻——零时刻是
// 合法时间值，用它兼表没发生会让两态在报文上分不开。
func optionalInstant(at *time.Time) string {
	if at == nil {
		return ""
	}
	return rfc3339(*at)
}
