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
// 它是接口而非解析代码：请求方身份与租户必须同时核对，运营接入面的认证方式属
// `PAR-INT-01` 待提供；采信自报租户会穿透 ADR-0003 的隔离边界（ADR-0077 Decision
// 三）。未决期间本包不带任何采信实现，包括「开发用」的采信头部版本。
//
// 四个端点共用一个 Intake 类型：它们是同一上下文同一作用域形状的运营查阅，分设只会
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
	if errors.Is(err, ErrMalformedRequest) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

// 四个端点的门次序一致：方法 → 册名 → 准入（照 /customs-ports-paths）。分成两个
// 助手而不是一个，是因为带分派的三个端点要在两道门之间插册名判——册名不认识是请求
// 形状的事，判它不需要先知道调用方是谁。两个助手交回 false 时答复已写出，调用方
// 直接返回。

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
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
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
