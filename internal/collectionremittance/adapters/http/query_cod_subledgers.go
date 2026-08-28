package collectionhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// CatalogueQuery 是一次已授权的主数据目录查阅(ADR-0077):作用域来自认证与授权结果,
// 授权边界只有租户——分户账是租户内部的受托保管账,没有客户维不是「可选缺席」,是
// 这个维不存在;页大小由接入面按渠道契约裁决——两样都不采信调用方自报。
type CatalogueQuery struct {
	Scope domain.OperationsQueryScope
	Limit int
}

// CatalogueQueryIntake 把一次已认证的运营查阅请求翻译成查询。
//
// 它是接口而非解析代码,理由同 customshttp 的同名接口:请求方身份与租户必须同时
// 核对,运营接入面的认证方式属 `PAR-INT-01` 待提供;采信自报租户会穿透 ADR-0003 的
// 隔离边界。未决期间本包不带任何采信实现,包括「开发用」的采信头部版本。
type CatalogueQueryIntake interface {
	IntakeCatalogueQuery(ctx context.Context, request *http.Request) (CatalogueQuery, error)
}

// ErrMalformedRequest 表示接入面认定请求形状立不起来(与未配置、依赖故障分三格,
// 恢复动作不同,判据同 ADR-0029:这一格要调用方改报文)。
var ErrMalformedRequest = errors.New("collection remittance http: malformed request")

// 问题码命名状态不命名参数(ADR-0055 同句)。
const (
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeMalformedRequest = "MALFORMED_REQUEST"
	codeNoAnswerFormed   = "NO_ANSWER_FORMED"
	codeIntakeFailed     = "INTAKE_FAILED"
)

// CodSubledgerCatalogueReader 是本端点消费的读口:代收分户账册的列表读面(票
// admin-remainder-mechanism-batch/04)。查阅不触发判断、决定或披露——接存储读面,
// 不接应用编排,与 /customs-ports-paths 同一条分界(ADR-0077 Decision 一)。
type CodSubledgerCatalogueReader interface {
	ListCodSubledgers(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CodSubledgerCatalogueRow, error)
}

// 编译期锁缝:读口形状与端口保持一致——本端点不新造查询语义。
var _ CodSubledgerCatalogueReader = ports.CodSubledgerCatalogueRead(nil)

// 业务结果只有一格:分户账册上列。空册如实答空列表走 2xx 成格,不折成未配置
// (ADR-0077 Decision 四:空册的续办是登记责任方去登记口开立,未配置的续办是接入方
// 去配置渠道)。
const outcomeCodSubledgersListed = "COD_SUBLEDGERS_LISTED"

// NewQueryCodSubledgersEndpoint 交回代收分户账查阅的 HTTP 入口
// (GET /collection-subledgers,票 admin-remainder-mechanism-batch/04)。
//
// 分户账册只有一本,不设分派参数——封闭集为一时参数只会造出一个恒定值(判据同
// /customs-gate-conditions 文件头那句)。指令册、事实册与差异事项册不在本端点:
// 它们是各自独立的登记册,长出查阅语义时各立入口,不在这里预留分派格。
//
// 全部已开立分户账连同派生余额原样上列,**不下推清分或汇付的判断参数**:哪笔该清分、
// 哪批该汇付是记账写入方伺候的判断输入,查阅面收下判断参数就等于让目录读口长出第二
// 种「处置」语义(裁决同 /customs-ports-paths 不收评估时点那条)。
func NewQueryCodSubledgersEndpoint(
	intake CatalogueQueryIntake,
	reader CodSubledgerCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		entries, err := reader.ListCodSubledgers(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			// 读不回是答案未形成,不是「空册」——伪装成后者会让一次该重试的故障变成终局。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		// 空列表交回空数组而不是 null:调用方判「没有行」不该先判「有没有字段」。
		bodies := make([]codSubledgerBody, 0, len(entries))
		for _, entry := range entries {
			bodies = append(bodies, subledgerBodyOf(entry))
		}
		writeJSON(response, http.StatusOK, codSubledgerListResponse{
			Outcome:    outcomeCodSubledgersListed,
			Subledgers: bodies,
		})
	})
}

func subledgerBodyOf(entry ports.CodSubledgerCatalogueRow) codSubledgerBody {
	// 批次空切片转写成空数组而不是 null,判据同上——「尚无批次」是内容,页面据它
	// 显式说「未配置」。
	batches := make([]remittanceBatchBody, 0, len(entry.Batches))
	for _, batch := range entry.Batches {
		batches = append(batches, remittanceBatchBody{
			Batch:            batch.Batch,
			State:            batch.State,
			CollectedThrough: batch.CollectedThrough.UTC().Format(time.RFC3339Nano),
			FormedAt:         batch.FormedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return codSubledgerBody{
		Customer:     entry.Customer,
		LegalEntity:  entry.LegalEntity,
		Currency:     entry.Currency,
		Channel:      entry.Channel,
		CustodyBasis: entry.CustodyBasis,
		OpenedAt:     entry.OpenedAt.UTC().Format(time.RFC3339Nano),
		PostingCount: entry.PostingCount,
		Balances: positionBalancesBody{
			InTransitAtChannel: minorAmount(entry.Balances.InTransitAtChannelMinor),
			AwaitingAllocation: minorAmount(entry.Balances.AwaitingAllocationMinor),
			PayableToCustomer:  minorAmount(entry.Balances.PayableToCustomerMinor),
			Remitted:           minorAmount(entry.Balances.RemittedMinor),
			Shortfall:          minorAmount(entry.Balances.ShortfallMinor),
			Surplus:            minorAmount(entry.Balances.SurplusMinor),
		},
		RemittanceBatches: batches,
	}
}

// minorAmount 把最小单位余额转写成十进制计数串:int64 直投 JSON number 在 2^53 以上
// 的取值会被 JS 读者悄悄取整——串是照实转写,数才是替读者做的算术承诺;小数位属币种
// 语义,本读面不代判精度。
func minorAmount(value int64) string {
	return strconv.FormatInt(value, 10)
}

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

type codSubledgerListResponse struct {
	Outcome    string             `json:"outcome"`
	Subledgers []codSubledgerBody `json:"subledgers"`
}

// codSubledgerBody 逐字段透出一本分户账:四维键、保管依据引用、开立时间、六位置
// 派生余额、记账笔数与引用本账的回汇批次。custodyBasis 是标识引用——「引用的商业
// 责任此刻是否在册」是读者拿 party-commercial 册子对照的判断,本行不代答(裁量同
// 申报路径引用口岸那条)。postingCount 分开「从未记账」与「记账相抵为零」两态——
// 它是行数不是金额,故用数不用串。
type codSubledgerBody struct {
	Customer          string                `json:"customer"`
	LegalEntity       string                `json:"legalEntity"`
	Currency          string                `json:"currency"`
	Channel           string                `json:"channel"`
	CustodyBasis      string                `json:"custodyBasis"`
	OpenedAt          string                `json:"openedAt"`
	PostingCount      int64                 `json:"postingCount"`
	Balances          positionBalancesBody  `json:"balances"`
	RemittanceBatches []remittanceBatchBody `json:"remittanceBatches"`
}

// positionBalancesBody 六位置派生余额,最小单位十进制计数串(见 minorAmount)。
type positionBalancesBody struct {
	InTransitAtChannel string `json:"inTransitAtChannel"`
	AwaitingAllocation string `json:"awaitingAllocation"`
	PayableToCustomer  string `json:"payableToCustomer"`
	Remitted           string `json:"remitted"`
	Shortfall          string `json:"shortfall"`
	Surplus            string `json:"surplus"`
}

// remittanceBatchBody 逐字段透出一个回汇批次:标识、单向状态、归集截点与形成时刻。
// 键与截点在形成时冻结,本行照册转写,不为批次编造成员清单——成员就是引用批次的
// 汇付记账(迁移 0001 批次表自注)。
type remittanceBatchBody struct {
	Batch            string `json:"batch"`
	State            string `json:"state"`
	CollectedThrough string `json:"collectedThrough"`
	FormedAt         string `json:"formedAt"`
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
	// 编码失败无从补救:状态行已写出,只能留给传输层中断。
	_ = json.NewEncoder(response).Encode(value)
}
