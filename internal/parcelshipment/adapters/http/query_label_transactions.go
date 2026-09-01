package shipmenthttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 面单交易查阅端点（票 admin-skeleton-closure-batch/08，ADR-0084 决定七）。
//
// 本端点只有读。面单交易的写入方是渠道适配器，渠道墙未降前不存在；查阅面收命令等于让读口
// 长出第二种「处置」语义。关闭/重开决定同理——那是授权业务角色形成的追加式决定，另册另票。

// LabelTransactionQuery 是一次已授权的面单交易查阅：作用域只有租户，页大小由接入面按渠道
// 契约裁决，两样都不采信调用方自报。
//
// **它刻意不收 AuthorizedQueryScope。** 那个类型的构造器拒绝空账户集，理由写在它自己的
// 注释里——不让各读口各自决定空集合是「全都可见」还是「全都不可见」。而面单交易压根没有
// 账户维可分：覆盖包裹可以跨委托，一笔交易合法地属于多个客户账户（决定七）。把一个必带
// 账户维的作用域交给一个不按账户过滤的读面，等于在类型上声称它会过滤而它不会；今天命令面
// 与查阅面都还挂着未配置 Intake，没有真作用域到得了这里，于是这个谎今天不出声——真 Intake
// 接上那天它才出声，而那时没有任何东西会变红。所以区别做进类型，不留给注释守。
//
// 这里没有授权凭据引用（AuthorizedQueryScope 带的那个 QueryScopeReference）：读口消费的只有
// （租户, limit），带一个没人读的字段是造第二个来源。运营查阅若日后要留授权痕迹，那是给本
// 上下文引入「运营查阅作用域」值对象的另一笔工作，要过 CONTEXT，不在本票。
type LabelTransactionQuery struct {
	Tenant domain.TenantID
	Limit  int
}

// LabelTransactionQueryIntake 把一次已认证的运营查阅请求翻译成查询。
//
// 它是接口而非解析代码，理由同本包其余 Intake：作用域整组只能来自认证与授权结果
// （`PAR-INT-01` 待提供），采信自报租户会穿透 ADR-0003 的隔离边界。未决期间本包不带任何
// 采信实现，包括「开发用」的采信头部版本。
type LabelTransactionQueryIntake interface {
	IntakeLabelTransactionQuery(ctx context.Context, request *http.Request) (LabelTransactionQuery, error)
}

type LabelTransactionsReader interface {
	ListLabelTransactions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.LabelTransactionRecord, error)
}

// 编译期锁缝：读口形状与端口保持一致——本适配器不新造查询语义。
var _ LabelTransactionsReader = ports.LabelTransactionViews(nil)

// 业务结果的封闭集合。空册仍是 LABEL_TRANSACTIONS_LISTED：渠道墙未降前登记零行是设计，
// 「读取入口已配置、登记册为空」与「尚未实现」必须在答复上分得开——后者根本走不到这里，
// 它答的是 403 + ACCESS_CHANNEL_NOT_CONFIGURED。
const outcomeLabelTransactionsListed = "LABEL_TRANSACTIONS_LISTED"

// continuedAttemptBasisEmptyDecisionHistory 是「继续尝试判断」这一列的派生依据代码。
//
// 它随每次答复交出，为的是让页头能如实说明这一列是**派生**的，而且派生自一段真实为空的
// 决定历史——继续尝试决定登记册尚未落地（ADR-0084 决定六另票）。不写这一条，读的人会把
// 整列「开放」当成「已核对过关闭册」。登记册落地后这个代码随之更换，页头文案跟着变。
const continuedAttemptBasisEmptyDecisionHistory = "DERIVED_FROM_EMPTY_DECISION_HISTORY"

// NewQueryLabelTransactionsEndpoint 交回面单交易查阅的 HTTP 入口（GET /label-transactions）。
// 行粒度是交易 × 包裹：一笔交易覆盖几件包裹就摊几行，摊开在读侧完成（决定七）。
func NewQueryLabelTransactionsEndpoint(
	intake LabelTransactionQueryIntake,
	reader LabelTransactionsReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeLabelTransactionQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		records, err := reader.ListLabelTransactions(request.Context(), query.Tenant, query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		rows := make([]labelTransactionRowBody, 0, len(records))
		for _, record := range records {
			rows = append(rows, labelTransactionRowsOf(record)...)
		}
		writeJSON(response, http.StatusOK, labelTransactionsListResponse{
			Outcome:               outcomeLabelTransactionsListed,
			ContinuedAttemptBasis: continuedAttemptBasisEmptyDecisionHistory,
			Rows:                  rows,
		})
	})
}

type labelTransactionsListResponse struct {
	Outcome               string                    `json:"outcome"`
	ContinuedAttemptBasis string                    `json:"continuedAttemptBasis"`
	Rows                  []labelTransactionRowBody `json:"rows"`
}

// labelTransactionRowBody 是「交易 × 包裹」一行的传输形。
//
// hasParcelResult 与 parcelAccepted 分列，不合成一个三值字符串：JSON 里一个 false 的
// accepted 读起来就是「未受理」，而结果未回时那是假话——与 CONTEXT 禁止的「结果不确定按
// 失败处理」是同一个错，只是错在传输这一层。
type labelTransactionRowBody struct {
	TransactionID          string `json:"transactionId"`
	ParcelID               string `json:"parcelId"`
	ChannelAccount         string `json:"channelAccount"`
	AccountHolder          string `json:"accountHolder"`
	ChannelServicer        string `json:"channelServicer"`
	SettlementCounterparty string `json:"settlementCounterparty"`
	Contract               string `json:"contract"`
	Rate                   string `json:"rate"`
	ResponsibilityBasis    string `json:"responsibilityBasis"`
	TransactionResult      string `json:"transactionResult"`
	Finalized              bool   `json:"finalized"`
	HasParcelResult        bool   `json:"hasParcelResult"`
	ParcelAccepted         bool   `json:"parcelAccepted"`
	ParcelIdentifier       string `json:"parcelIdentifier,omitempty"`
	ParcelResultReason     string `json:"parcelResultReason,omitempty"`
	// 空数组而不是 null：调用方判「这件包裹有没有后续动作」不该先判「有没有这个字段」。
	FollowUpKinds        []string `json:"followUpKinds"`
	ContinuedAttemptOpen bool     `json:"continuedAttemptOpen"`
	EstablishedAt        string   `json:"establishedAt"`
	SubmittedAt          string   `json:"submittedAt,omitempty"`
	ResultObservedAt     string   `json:"resultObservedAt,omitempty"`
	PriorTransactionID   string   `json:"priorTransactionId,omitempty"`
	PriorLinkKind        string   `json:"priorLinkKind,omitempty"`
}

func labelTransactionRowsOf(record ports.LabelTransactionRecord) []labelTransactionRowBody {
	rows := make([]labelTransactionRowBody, 0, len(record.Parcels))
	for _, parcel := range record.Parcels {
		row := labelTransactionRowBody{
			TransactionID:          record.TransactionID.String(),
			ParcelID:               parcel.Parcel.String(),
			ChannelAccount:         record.ChannelAccount,
			AccountHolder:          record.AccountHolder,
			ChannelServicer:        record.ServiceProvider,
			SettlementCounterparty: record.SettlementCounterparty,
			Contract:               record.Contract,
			Rate:                   record.Rate,
			ResponsibilityBasis:    record.ResponsibilityBasis,
			TransactionResult:      record.State.String(),
			Finalized:              record.Finalized,
			HasParcelResult:        parcel.HasResult,
			ParcelAccepted:         parcel.Accepted,
			ParcelIdentifier:       parcel.Identifier,
			ParcelResultReason:     parcel.Reason,
			FollowUpKinds:          followUpKindNames(parcel.FollowUpKinds),
			ContinuedAttemptOpen:   parcel.ContinuedAttemptOpen,
			EstablishedAt:          record.EstablishedAt.UTC().Format(time.RFC3339Nano),
		}
		if !record.SubmittedAt.IsZero() {
			row.SubmittedAt = record.SubmittedAt.UTC().Format(time.RFC3339Nano)
		}
		if !record.ResultObservedAt.IsZero() {
			row.ResultObservedAt = record.ResultObservedAt.UTC().Format(time.RFC3339Nano)
		}
		if record.PriorTransactionID != "" {
			row.PriorTransactionID = record.PriorTransactionID
			row.PriorLinkKind = record.PriorLinkKind.String()
		}
		rows = append(rows, row)
	}
	return rows
}

func followUpKindNames(kinds []domain.FollowUpActionKind) []string {
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, kind.String())
	}
	return names
}
