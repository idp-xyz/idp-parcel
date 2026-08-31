package settlementhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// FundsApplicationCatalogueReader 是本端点消费的读口：已采用外部资金事实册的列表
// 读面，映射与核销挂在事实下面。
type FundsApplicationCatalogueReader interface {
	ListExternalFundsFacts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ExternalFundsFactCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ FundsApplicationCatalogueReader = ports.FundsApplicationCatalogueRead(nil)

// 业务结果只有一格：资金事实册上列。空册如实答空列表走 2xx 成格，不折成未配置
// （ADR-0077 Decision 四）。
const outcomeExternalFundsFactsListed = "EXTERNAL_FUNDS_FACTS_LISTED"

// NewQuerySettlementFundsApplicationsEndpoint 交回收付款核销页的 HTTP 入口
// （GET /settlement-funds-applications，票 admin-skeleton-closure-batch/04）。
//
// 页的行对象是已采用的外部资金事实，映射与核销挂在它下面——不设 `registry` 分派：
// 封闭集为一时参数只会造出一个恒定值（判据同 /collection-subledgers）。资金冻结册
// 不在本端点：冻结是接受前财务控制的产物，与真实收付是两条链（CONTEXT 明禁用通知、
// 对方认可或金额确认冒充到账），长出查阅语义时另立入口。
//
// 外部财务或支付系统拥有真实收付款事实，本上下文只有 `UC-SA-005` 能形成映射与核销；
// 本端点是查阅面，**不收任何分配或撤销参数**——收下就等于开出一条绕开 UC-SA-005 的
// 第二写路。
func NewQuerySettlementFundsApplicationsEndpoint(
	intake CatalogueQueryIntake,
	reader FundsApplicationCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}

		rows, err := reader.ListExternalFundsFacts(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			// 读不回是答案未形成，不是「空册」——伪装成后者会让一次该重试的故障变成终局。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		// 空列表交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
		bodies := make([]externalFundsFactBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, externalFundsFactBodyOf(row))
		}
		writeJSON(response, http.StatusOK, externalFundsFactListResponse{
			Outcome: outcomeExternalFundsFactsListed,
			Facts:   bodies,
		})
	})
}

func externalFundsFactBodyOf(row ports.ExternalFundsFactCatalogueRow) externalFundsFactBody {
	mappings := make([]fundsMappingBody, 0, len(row.Mappings))
	for _, mapping := range row.Mappings {
		mappings = append(mappings, fundsMappingBody{
			Mapping:    mapping.Mapping,
			TargetKind: mapping.TargetKind,
			Target:     mapping.Target,
			Basis:      mapping.Basis,
			MappedAt:   rfc3339(mapping.MappedAt),
		})
	}
	applications := make([]settlementApplicationBody, 0, len(row.Applications))
	for _, application := range row.Applications {
		applications = append(applications, settlementApplicationBody{
			Application:     application.Application,
			AppliedAmount:   minorAmount(application.AppliedMinor),
			Basis:           application.Basis,
			AppliedAt:       rfc3339(application.AppliedAt),
			AllocationCount: application.AllocationCount,
			ReversalBasis:   application.ReversalBasis,
			ReversedAt:      optionalInstant(application.ReversedAt),
		})
	}
	return externalFundsFactBody{
		Fact:             row.Fact,
		Source:           row.Source,
		Kind:             row.Kind,
		Currency:         row.Currency,
		Amount:           minorAmount(row.AmountMinor),
		Version:          row.Version,
		OccurredAt:       rfc3339(row.OccurredAt),
		Corrects:         row.Corrects,
		CorrectedAt:      optionalInstant(row.CorrectedAt),
		AppliedAmount:    minorAmount(row.AppliedMinor),
		ReversedAmount:   minorAmount(row.ReversedMinor),
		UnappliedAmount:  minorAmount(row.UnappliedMinor),
		ApplicationCount: row.ApplicationCount,
		Mappings:         mappings,
		Applications:     applications,
	}
}

type externalFundsFactListResponse struct {
	Outcome string                  `json:"outcome"`
	Facts   []externalFundsFactBody `json:"facts"`
}

// externalFundsFactBody 逐字段透出一笔已采用的外部资金事实。
//
// **appliedAmount 与 reversedAmount 是两笔各算各的和，不是一个净额**：前者只加未撤销
// 的核销，后者只加已撤销的，unappliedAmount = amount − appliedAmount。三个数摆开是
// 因为「从未核销过」与「核销过又撤销了」在一个净额上长着同一张脸，而这两态的续办相反
// （前者要人去分配，后者要人去看当初为什么撤）。撤销不删历史，那一截金额必须在某处
// 仍然看得见。
//
// kind 照实透出不译成收付方向：封闭集 RECEIPT_CONFIRMED / PAYMENT_FAILED /
// FUNDS_RETURNED 里只有一格是「收到了钱」，折成收/付两向会把「付款失败」与「资金
// 退回」压成同一格。
type externalFundsFactBody struct {
	Fact             string                      `json:"fact"`
	Source           string                      `json:"source"`
	Kind             string                      `json:"kind"`
	Currency         string                      `json:"currency"`
	Amount           string                      `json:"amount"`
	Version          string                      `json:"version"`
	OccurredAt       string                      `json:"occurredAt"`
	Corrects         string                      `json:"corrects,omitempty"`
	CorrectedAt      string                      `json:"correctedAt,omitempty"`
	AppliedAmount    string                      `json:"appliedAmount"`
	ReversedAmount   string                      `json:"reversedAmount"`
	UnappliedAmount  string                      `json:"unappliedAmount"`
	ApplicationCount int64                       `json:"applicationCount"`
	Mappings         []fundsMappingBody          `json:"mappings"`
	Applications     []settlementApplicationBody `json:"applications"`
}

// fundsMappingBody 逐字段透出一条真实收付映射。映射不是核销——事实接收、金额责任
// 确认、真实到账与核销是不同结果（CONTEXT「真实收付映射」词条），所以映射与核销
// 分两个数组，不合成一格。
type fundsMappingBody struct {
	Mapping    string `json:"mapping"`
	TargetKind string `json:"targetKind"`
	Target     string `json:"target"`
	Basis      string `json:"basis"`
	MappedAt   string `json:"mappedAt"`
}

// settlementApplicationBody 逐字段透出一次核销。allocationCount 是带方向分配片段的
// **片段数**，片段明细属详情面。reversalBasis 与 reversedAt 成对缺席表示未撤销——
// 撤销形成可追溯的反向关系，不删除原核销历史。
type settlementApplicationBody struct {
	Application     string `json:"application"`
	AppliedAmount   string `json:"appliedAmount"`
	Basis           string `json:"basis"`
	AppliedAt       string `json:"appliedAt"`
	AllocationCount int64  `json:"allocationCount"`
	ReversalBasis   string `json:"reversalBasis,omitempty"`
	ReversedAt      string `json:"reversedAt,omitempty"`
}
