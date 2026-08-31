package settlementhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// StatementCatalogueReader 是本端点消费的读口：客户对账单册（UC-SA-003）与供应商
// 账单接收册（UC-SA-004）两本册子的列表读面。
type StatementCatalogueReader interface {
	ListCustomerStatements(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CustomerStatementCatalogueRow, error)
	ListSupplierBillReceptions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.SupplierBillReceptionCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ StatementCatalogueReader = ports.StatementCatalogueRead(nil)

const (
	outcomeCustomerStatementsListed     = "CUSTOMER_STATEMENTS_LISTED"
	outcomeSupplierBillReceptionsListed = "SUPPLIER_BILL_RECEPTIONS_LISTED"
)

const (
	registryCustomerStatement     = "customer-statement"
	registrySupplierBillReception = "supplier-bill-reception"
)

// NewQuerySettlementStatementsEndpoint 交回对账单页两本册子的 HTTP 入口
// （GET /settlement-statements，票 admin-skeleton-closure-batch/04）。
//
// 只上已发布对账单：草稿可以重新计算，发布后单号、费用范围与金额才不可覆盖——把
// 可变的草稿与冻结的已发布单摆进同一张目录，读者就无从知道手上这一行还会不会变。
//
// **不下推账期归集或裁定参数**：哪些费用进这一期由 UC-SA-003 按截单时刻归集，异议
// 如何裁定由裁定用例形成；查阅面收下这些参数就等于让目录读口长出第二种「处置」语义。
func NewQuerySettlementStatementsEndpoint(
	intake CatalogueQueryIntake,
	reader StatementCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		registry := request.URL.Query().Get("registry")
		if registry != registryCustomerStatement && registry != registrySupplierBillReception {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registryCustomerStatement:
			serveCustomerStatements(response, request, reader, tenant, query.Limit)
		case registrySupplierBillReception:
			serveSupplierBillReceptions(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveCustomerStatements(
	response http.ResponseWriter,
	request *http.Request,
	reader StatementCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListCustomerStatements(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]customerStatementBody, 0, len(rows))
	for _, row := range rows {
		disputes := make([]statementDisputeBody, 0, len(row.Disputes))
		for _, dispute := range row.Disputes {
			disputes = append(disputes, statementDisputeBody{
				Dispute:        dispute.Dispute,
				Charge:         dispute.Charge,
				DisputedAmount: minorAmount(dispute.DisputedMinor),
				Reason:         dispute.Reason,
				OpenedAt:       rfc3339(dispute.OpenedAt),
				Resolution:     dispute.Resolution,
				ResolutionRef:  dispute.ResolutionRef,
				ResolvedAt:     optionalInstant(dispute.ResolvedAt),
			})
		}
		inclusions := make([]subsequentInclusionBody, 0, len(row.SubsequentInclusions))
		for _, inclusion := range row.SubsequentInclusions {
			inclusions = append(inclusions, subsequentInclusionBody{
				Inclusion:        inclusion.Inclusion,
				Kind:             inclusion.Kind,
				OriginalPeriod:   inclusion.OriginalPeriod,
				SubsequentPeriod: inclusion.SubsequentPeriod,
				Charge:           inclusion.Charge,
				Adjustment:       inclusion.Adjustment,
				IncludedAt:       rfc3339(inclusion.IncludedAt),
			})
		}
		bodies = append(bodies, customerStatementBody{
			StatementNumber:      row.StatementNumber,
			Account:              row.Account,
			Period:               row.Period,
			Currency:             row.Currency,
			TotalAmount:          minorAmount(row.TotalMinor),
			LineCount:            row.LineCount,
			AdjustmentCount:      row.AdjustmentCount,
			PublishedAt:          rfc3339(row.PublishedAt),
			VoidBasis:            row.VoidBasis,
			VoidedAt:             optionalInstant(row.VoidedAt),
			Disputes:             disputes,
			SubsequentInclusions: inclusions,
		})
	}
	writeJSON(response, http.StatusOK, customerStatementListResponse{
		Outcome:    outcomeCustomerStatementsListed,
		Statements: bodies,
	})
}

func serveSupplierBillReceptions(
	response http.ResponseWriter,
	request *http.Request,
	reader StatementCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListSupplierBillReceptions(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]supplierBillReceptionBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, supplierBillReceptionBody{
			Claim:                    row.Claim,
			ClaimVersion:             row.ClaimVersion,
			Supplier:                 row.Supplier,
			LegalEntity:              row.LegalEntity,
			Period:                   row.Period,
			Currency:                 row.Currency,
			LineCount:                row.LineCount,
			MatchCount:               row.MatchCount,
			AuditAuthorityConfigured: row.AuditAuthorityConfigured,
			RecordedAt:               rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, supplierBillReceptionListResponse{
		Outcome:    outcomeSupplierBillReceptionsListed,
		Receptions: bodies,
	})
}

type customerStatementListResponse struct {
	Outcome    string                  `json:"outcome"`
	Statements []customerStatementBody `json:"statements"`
}

// customerStatementBody 逐字段透出一张已发布对账单。
//
// lineCount 与 adjustmentCount 是费用行与调整行的**行数**，不是金额：明细是详情面
// 的内容，但「这张单里有几行」与「一行都没有」得分得开。总额不由行数派生，两者各自
// 照实转写——CONTEXT 要求总额严格等于所含明细之和，那是写口的不变量，读口重算一遍
// 只会在两处各说一套。两个计数是行数不是金额，故用数不用串。
//
// voidBasis 与 voidedAt 成对缺席表示未作废。作废不删行不改总额，替代单用新单号，
// 所以「已作废」是这一行上的留痕而不是它的消失。
type customerStatementBody struct {
	StatementNumber      string                    `json:"statementNumber"`
	Account              string                    `json:"account"`
	Period               string                    `json:"period"`
	Currency             string                    `json:"currency"`
	TotalAmount          string                    `json:"totalAmount"`
	LineCount            int64                     `json:"lineCount"`
	AdjustmentCount      int64                     `json:"adjustmentCount"`
	PublishedAt          string                    `json:"publishedAt"`
	VoidBasis            string                    `json:"voidBasis,omitempty"`
	VoidedAt             string                    `json:"voidedAt,omitempty"`
	Disputes             []statementDisputeBody    `json:"disputes"`
	SubsequentInclusions []subsequentInclusionBody `json:"subsequentInclusions"`
}

// statementDisputeBody 逐字段透出一项异议。裁定三件（结论、依据、时刻）同在或同缺；
// 未裁定时三键皆不出现，不代填「待处理」——那会把「还没人裁」写成一个看起来已经有人
// 处置过的结论。无争议部分继续确认、开票、付款或核销，故异议挂在单上而不改单的总额。
type statementDisputeBody struct {
	Dispute        string `json:"dispute"`
	Charge         string `json:"charge"`
	DisputedAmount string `json:"disputedAmount"`
	Reason         string `json:"reason"`
	OpenedAt       string `json:"openedAt"`
	Resolution     string `json:"resolution,omitempty"`
	ResolutionRef  string `json:"resolutionRef,omitempty"`
	ResolvedAt     string `json:"resolvedAt,omitempty"`
}

// subsequentInclusionBody 逐字段透出一笔后续账期纳入。没有金额键——金额永远在费用或
// 调整本体上，纳入只拥有关系（`UC-SA-003` 不创建任何调整）。adjustment 缺席是
// LATE_CHARGE 的正面形状（迁移 0007：迟到费用不得指名调整），不是漏登。
type subsequentInclusionBody struct {
	Inclusion        string `json:"inclusion"`
	Kind             string `json:"kind"`
	OriginalPeriod   string `json:"originalPeriod"`
	SubsequentPeriod string `json:"subsequentPeriod"`
	Charge           string `json:"charge"`
	Adjustment       string `json:"adjustment,omitempty"`
	IncludedAt       string `json:"includedAt"`
}

type supplierBillReceptionListResponse struct {
	Outcome    string                      `json:"outcome"`
	Receptions []supplierBillReceptionBody `json:"receptions"`
}

// supplierBillReceptionBody 逐字段透出一份供应商账单主张。
//
// auditAuthorityConfigured 照实转写而不折成「可审核」：授权未配置时审核停在未决，
// 不默认放行也不虚构授权人（UC-SA-004）；把它译成一个动作可用性，就是在读面上替
// 审核步骤做了那个判断。
type supplierBillReceptionBody struct {
	Claim                    string `json:"claim"`
	ClaimVersion             string `json:"claimVersion"`
	Supplier                 string `json:"supplier"`
	LegalEntity              string `json:"legalEntity"`
	Period                   string `json:"period"`
	Currency                 string `json:"currency"`
	LineCount                int64  `json:"lineCount"`
	MatchCount               int64  `json:"matchCount"`
	AuditAuthorityConfigured bool   `json:"auditAuthorityConfigured"`
	RecordedAt               string `json:"recordedAt"`
}
