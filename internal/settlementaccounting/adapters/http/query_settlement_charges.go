package settlementhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ChargeCatalogueReader 是本端点消费的读口：客户费用册与供应商预期成本版本册两本
// 册子的列表读面（票 admin-skeleton-closure-batch/04）。
type ChargeCatalogueReader interface {
	ListCustomerCharges(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CustomerChargeCatalogueRow, error)
	ListSupplierExpectedCosts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.SupplierExpectedCostCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ ChargeCatalogueReader = ports.ChargeCatalogueRead(nil)

// 业务结果的封闭集合：两本册子各占一格。空册如实答空列表走 2xx 成格，不折成未配置
// （ADR-0077 Decision 四：空册的续办是登记责任方去登记口登记，未配置的续办是接入方
// 去配置渠道）。
const (
	outcomeCustomerChargesListed       = "CUSTOMER_CHARGES_LISTED"
	outcomeSupplierExpectedCostsListed = "SUPPLIER_EXPECTED_COSTS_LISTED"
)

// registry 查询参数的封闭集（未知值坏请求），判据同 /customs-ports-paths。
const (
	registryCustomerCharge       = "customer-charge"
	registrySupplierExpectedCost = "supplier-expected-cost"
)

// NewQuerySettlementChargesEndpoint 交回费用与计费页两本册子的 HTTP 入口
// （GET /settlement-charges，票 admin-skeleton-closure-batch/04）。
//
// 两册共用一个端点按 `registry` 分派，各自成形：客户费用与供应商预期成本的行形状
// 互不相同（前者有阶段与确认依据、无采购规则版本，后者有版本链与纠错原因、无阶段），
// 共享壳要么空出半数字段、要么把强类型折成 any（判据逐字同 visibilityhttp 的六种
// 目录行）。CONTEXT 另把两者立为两个词条并明禁「内部预期伪装成供应商主张」——合成
// 一张表，栏目里就再没有东西说得出这一行是内部预期。
//
// 全部行原样上列，**不下推确认条件、账期或口径参数**：哪笔该确认、该进哪个账期、
// 按哪个口径采用是各自唯一创建用例伺候的判断输入，查阅面收下判断参数就等于让目录
// 读口长出第二种「处置」语义（裁决同 /customs-ports-paths 不收评估时点那条）。
func NewQuerySettlementChargesEndpoint(
	intake CatalogueQueryIntake,
	reader ChargeCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		registry := request.URL.Query().Get("registry")
		if registry != registryCustomerCharge && registry != registrySupplierExpectedCost {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registryCustomerCharge:
			serveCustomerCharges(response, request, reader, tenant, query.Limit)
		case registrySupplierExpectedCost:
			serveSupplierExpectedCosts(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveCustomerCharges(
	response http.ResponseWriter,
	request *http.Request,
	reader ChargeCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListCustomerCharges(request.Context(), tenant, limit)
	if err != nil {
		// 读不回是答案未形成，不是「空册」——伪装成后者会让一次该重试的故障变成终局。
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	// 空列表交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
	bodies := make([]customerChargeBody, 0, len(rows))
	for _, row := range rows {
		bases := make([]chargeConfirmationBasisBody, 0, len(row.ConfirmationBases))
		for _, basis := range row.ConfirmationBases {
			bases = append(bases, chargeConfirmationBasisBody{
				BasisKind:  basis.BasisKind,
				Basis:      basis.Basis,
				RecordedAt: rfc3339(basis.RecordedAt),
			})
		}
		bodies = append(bodies, customerChargeBody{
			Charge:             row.Charge,
			FeeItem:            row.FeeItem,
			Evaluation:         row.Evaluation,
			Stage:              row.Stage,
			OriginalCurrency:   row.OriginalCurrency,
			OriginalAmount:     minorAmount(row.OriginalMinor),
			SettlementCurrency: row.SettlementCurrency,
			SettlementAmount:   minorAmount(row.SettlementMinor),
			ConversionStep:     row.ConversionStep,
			ConfirmationBasis:  row.ConfirmationBasis,
			FormedAt:           rfc3339(row.FormedAt),
			ConfirmedAt:        optionalInstant(row.ConfirmedAt),
			RequiredBasisKind:  row.RequiredBasisKind,
			ConfirmationBases:  bases,
		})
	}
	writeJSON(response, http.StatusOK, customerChargeListResponse{
		Outcome: outcomeCustomerChargesListed,
		Charges: bodies,
	})
}

func serveSupplierExpectedCosts(
	response http.ResponseWriter,
	request *http.Request,
	reader ChargeCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListSupplierExpectedCosts(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]supplierExpectedCostBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, supplierExpectedCostBody{
			Version:             row.Version,
			Occurrence:          row.Occurrence,
			OccurrenceReason:    row.OccurrenceReason,
			OccurrenceVersion:   row.OccurrenceVersion,
			OccurredAt:          rfc3339(row.OccurredAt),
			FeeItem:             row.FeeItem,
			PurchaseRuleVersion: row.PurchaseRuleVersion,
			Agreement:           row.Agreement,
			Evaluation:          row.Evaluation,
			OriginalCurrency:    row.OriginalCurrency,
			OriginalAmount:      minorAmount(row.OriginalMinor),
			SettlementCurrency:  row.SettlementCurrency,
			SettlementAmount:    minorAmount(row.SettlementMinor),
			ConversionStep:      row.ConversionStep,
			PriorVersion:        row.PriorVersion,
			CorrectionReason:    row.CorrectionReason,
			RecordedAt:          rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, supplierExpectedCostListResponse{
		Outcome: outcomeSupplierExpectedCostsListed,
		Costs:   bodies,
	})
}

type customerChargeListResponse struct {
	Outcome string               `json:"outcome"`
	Charges []customerChargeBody `json:"charges"`
}

// customerChargeBody 逐字段透出一条客户费用。
//
// 币种三件组（原币、合同结算币、换算依据）整组透出不拆散：CONTEXT 明写三件是一条
// 费用从同一个评价采用来的一组。同币种时 conversionStep 整键不出现，那是「这一步
// 不存在」的正面形状，不是缺数据。
//
// confirmedAt 未确认时整键不出现（不为它编造零时刻）；requiredBasisKind 为空表示该
// 费用项目在确认条件目录里没有行，与「配了但依据没到」（有 requiredBasisKind 而
// confirmationBases 里没有那一种）分成两格摆开，由读的人判——两者的续办不同：前者
// 要人去配条件，后者要人去催依据。
type customerChargeBody struct {
	Charge             string                        `json:"charge"`
	FeeItem            string                        `json:"feeItem"`
	Evaluation         string                        `json:"evaluation"`
	Stage              string                        `json:"stage"`
	OriginalCurrency   string                        `json:"originalCurrency"`
	OriginalAmount     string                        `json:"originalAmount"`
	SettlementCurrency string                        `json:"settlementCurrency"`
	SettlementAmount   string                        `json:"settlementAmount"`
	ConversionStep     string                        `json:"conversionStep,omitempty"`
	ConfirmationBasis  string                        `json:"confirmationBasis,omitempty"`
	FormedAt           string                        `json:"formedAt"`
	ConfirmedAt        string                        `json:"confirmedAt,omitempty"`
	RequiredBasisKind  string                        `json:"requiredBasisKind,omitempty"`
	ConfirmationBases  []chargeConfirmationBasisBody `json:"confirmationBases"`
}

// chargeConfirmationBasisBody 逐字段透出一种已到达的确认依据。「到了哪几种」与
// 「这类费用要哪一种」是两张表两个答案，本端点分两格上列，不代算交集——迁移 0009
// 分两张表正是为了让「确认条件已满足」没有第三条成立路径。
type chargeConfirmationBasisBody struct {
	BasisKind  string `json:"basisKind"`
	Basis      string `json:"basis"`
	RecordedAt string `json:"recordedAt"`
}

type supplierExpectedCostListResponse struct {
	Outcome string                     `json:"outcome"`
	Costs   []supplierExpectedCostBody `json:"costs"`
}

// supplierExpectedCostBody 逐字段透出一版供应商预期成本。
//
// 没有账单主张、审核应付或付款字段，与迁移 0008 的表面一致：预期成本属预估口径，
// 那道分界在表上与在报文上同样是结构性的。priorVersion 与 correctionReason 成对
// 缺席表示这是首版——计价纠错换版本、原版本保留，一份成本的历史因此是多行而不是
// 一行被改写。
type supplierExpectedCostBody struct {
	Version             string `json:"version"`
	Occurrence          string `json:"occurrence"`
	OccurrenceReason    string `json:"occurrenceReason"`
	OccurrenceVersion   string `json:"occurrenceVersion"`
	OccurredAt          string `json:"occurredAt"`
	FeeItem             string `json:"feeItem"`
	PurchaseRuleVersion string `json:"purchaseRuleVersion"`
	Agreement           string `json:"agreement"`
	Evaluation          string `json:"evaluation"`
	OriginalCurrency    string `json:"originalCurrency"`
	OriginalAmount      string `json:"originalAmount"`
	SettlementCurrency  string `json:"settlementCurrency"`
	SettlementAmount    string `json:"settlementAmount"`
	ConversionStep      string `json:"conversionStep,omitempty"`
	PriorVersion        string `json:"priorVersion,omitempty"`
	CorrectionReason    string `json:"correctionReason,omitempty"`
	RecordedAt          string `json:"recordedAt"`
}
