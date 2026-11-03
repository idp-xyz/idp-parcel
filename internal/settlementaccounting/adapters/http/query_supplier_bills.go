package settlementhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 供应商账单与应付页的两个读口（UC-SA-004；票 document-numbering/11）：一份账单的各行连同匹配与审核去向，以及审核应付册。
// 账单的列表仍是 GET /settlement-statements?registry=supplier-bill-reception 那一本，这里不另立第二个列表口。两口同本包其余读口：
// 只读、租户只取自接入面、读不回答 NO_ANSWER_FORMED 而不答空。

// —— 一份供应商账单的各行 ——

// SupplierBillLineReader 是账单各行查阅消费的读口。
type SupplierBillLineReader interface {
	FindSupplierBillLines(ctx context.Context, tenant domain.TenantID, claim, version string) (ports.SupplierBillLines, bool, error)
}

var _ SupplierBillLineReader = ports.SupplierBillLineRead(nil)

const (
	outcomeSupplierBillLinesListed = "SUPPLIER_BILL_LINES_LISTED"
	// 主张与版本不在本租户册上是如实答案（200），不是读口故障。
	outcomeSupplierBillNotFound = "SUPPLIER_BILL_NOT_FOUND"
)

type supplierBillLinesResponse struct {
	Outcome                  string                 `json:"outcome"`
	Claim                    string                 `json:"claim,omitempty"`
	ClaimVersion             string                 `json:"claimVersion,omitempty"`
	Supplier                 string                 `json:"supplier,omitempty"`
	LegalEntity              string                 `json:"legalEntity,omitempty"`
	Period                   string                 `json:"period,omitempty"`
	Currency                 string                 `json:"currency,omitempty"`
	ReceivedAt               string                 `json:"receivedAt,omitempty"`
	SupplementsClaim         string                 `json:"supplementsClaim,omitempty"`
	AuditAuthorityConfigured *bool                  `json:"auditAuthorityConfigured,omitempty"`
	OperationEvidence        string                 `json:"operationEvidence,omitempty"`
	RecordedAt               string                 `json:"recordedAt,omitempty"`
	Lines                    []supplierBillLineBody `json:"lines,omitempty"`
}

// supplierBillLineBody 逐字段透出一行主张、它的匹配与审核去向。
//
// classification 是逐行匹配分类原名；缺席即接收记录上这一行没有匹配，不补「未匹配」。expectedVersion 缺席是「没有对应
// 发生项」的正面形状，那时也没有 expectedAmount 与 varianceAmount。varianceAmount 是主张减预期，正为供应商多收。
// payable 缺席即这一行还没有审核应付——不代填「待审核」，这一行审不审得了由审核口答。
type supplierBillLineBody struct {
	Line            string                   `json:"line"`
	FeeItem         string                   `json:"feeItem"`
	ClaimedAmount   string                   `json:"claimedAmount"`
	Classification  string                   `json:"classification,omitempty"`
	ExpectedVersion string                   `json:"expectedVersion,omitempty"`
	ExpectedAmount  string                   `json:"expectedAmount,omitempty"`
	VarianceAmount  string                   `json:"varianceAmount,omitempty"`
	Basis           string                   `json:"basis,omitempty"`
	MatchedAt       string                   `json:"matchedAt,omitempty"`
	Payable         *supplierBillPayableBody `json:"payable,omitempty"`
}

// supplierBillPayableBody 是挂在一行上的审核应付。documentNumber 是应付单号，与 documentNumberAbsence 恰出其一（没有号的不答
// 空串、不拿应付标识顶，ADR-0355 决定十一）。auditor 是审核授权册上的审核方，operationEvidence 是点下这一次审核的操作者，
// 两者不是一回事（ADR-0340 决定三）。
type supplierBillPayableBody struct {
	Payable               string `json:"payable"`
	DocumentNumber        string `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string `json:"documentNumberAbsence,omitempty"`
	Amount                string `json:"amount"`
	Currency              string `json:"currency"`
	Auditor               string `json:"auditor"`
	OperationEvidence     string `json:"operationEvidence,omitempty"`
	AuditedAt             string `json:"auditedAt"`
}

// NewQuerySupplierBillLinesEndpoint 交回一份账单各行查阅的 HTTP 入口（GET /settlement-supplier-bill-lines?claim=&version=）。账单册
// 目录行只带计数（明细归详情面）；审核要从这份账单自己的行里挑已匹配、还没有应付的那一行。claim 或 version 缺即坏请求。
func NewQuerySupplierBillLinesEndpoint(intake CatalogueQueryIntake, reader SupplierBillLineReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		claim, version := request.URL.Query().Get("claim"), request.URL.Query().Get("version")
		if claim == "" || version == "" {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		found, has, err := reader.FindSupplierBillLines(request.Context(), query.Scope.Tenant(), claim, version)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !has {
			writeJSON(response, http.StatusOK, supplierBillLinesResponse{Outcome: outcomeSupplierBillNotFound})
			return
		}
		configured := found.AuditAuthorityConfigured
		body := supplierBillLinesResponse{
			Outcome: outcomeSupplierBillLinesListed, Claim: found.Claim, ClaimVersion: found.ClaimVersion,
			Supplier: found.Supplier, LegalEntity: found.LegalEntity, Period: found.Period, Currency: found.Currency,
			ReceivedAt: rfc3339(found.ReceivedAt), SupplementsClaim: found.Supplements, AuditAuthorityConfigured: &configured,
			OperationEvidence: found.OperationEvidence, RecordedAt: rfc3339(found.RecordedAt),
			Lines: make([]supplierBillLineBody, 0, len(found.Lines)),
		}
		for _, line := range found.Lines {
			body.Lines = append(body.Lines, supplierBillLineBodyOf(line))
		}
		writeJSON(response, http.StatusOK, body)
	})
}

func supplierBillLineBodyOf(line ports.SupplierBillLineEntry) supplierBillLineBody {
	body := supplierBillLineBody{
		Line: line.Line, FeeItem: line.FeeItem, ClaimedAmount: minorAmount(line.ClaimedMinor),
		Classification: line.Classification, ExpectedVersion: line.ExpectedVersion, Basis: line.Basis,
		MatchedAt: optionalInstant(line.MatchedAt),
	}
	if line.ExpectedVersion != "" {
		body.ExpectedAmount = minorAmount(line.ExpectedMinor)
		body.VarianceAmount = minorAmount(line.ClaimedMinor - line.ExpectedMinor)
	}
	if payable := line.Payable; payable != nil {
		body.Payable = &supplierBillPayableBody{
			Payable: payable.Payable, DocumentNumber: payable.DocumentNumber, DocumentNumberAbsence: payable.DocumentNumberAbsence,
			Amount: minorAmount(payable.AmountMinor), Currency: payable.Currency, Auditor: payable.AuditorRef,
			OperationEvidence: payable.OperationEvidence, AuditedAt: rfc3339(payable.AuditedAt),
		}
	}
	return body
}

// —— 审核应付册 ——

// AuditedPayableCatalogueReader 是审核应付册查阅消费的读口。
type AuditedPayableCatalogueReader interface {
	ListAuditedPayables(
		ctx context.Context,
		tenant domain.TenantID,
		filter ports.AuditedPayableFilter,
		limit int,
	) ([]ports.AuditedPayableCatalogueRow, error)
}

var _ AuditedPayableCatalogueReader = ports.AuditedPayableCatalogueRead(nil)

// 空册仍是 LISTED：「还没有形成过审核应付」是如实答案。
const outcomeAuditedPayablesListed = "AUDITED_PAYABLES_LISTED"

// auditedPayableListResponse 的 truncated 为真：同样的检索维之下，页外还有更早形成的应付。检索维在库里对全册筛，不是在这一页
// 里筛，所以按应付单号检索，页外的那一笔照样找得到。
type auditedPayableListResponse struct {
	Outcome   string               `json:"outcome"`
	Truncated bool                 `json:"truncated"`
	Payables  []auditedPayableBody `json:"payables"`
}

// auditedPayableBody 逐字段透出一笔审核应付。documentNumber 与 documentNumberAbsence 恰出其一，理由同 supplierBillPayableBody；
// payable 是调用方生成的应付标识（幂等键），给技术详情，不当应付单号给人看。没有付款、收款或净额字段：审核应付不表示供应商
// 已付款或已收款，贷项也不净入它（UC-SA-004 结果语义）。
type auditedPayableBody struct {
	Payable               string `json:"payable"`
	DocumentNumber        string `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string `json:"documentNumberAbsence,omitempty"`
	Claim                 string `json:"claim"`
	Line                  string `json:"line"`
	ExpectedVersion       string `json:"expectedVersion"`
	Supplier              string `json:"supplier"`
	Period                string `json:"period"`
	LegalEntity           string `json:"legalEntity"`
	Account               string `json:"account"`
	Currency              string `json:"currency"`
	Amount                string `json:"amount"`
	Auditor               string `json:"auditor"`
	OperationEvidence     string `json:"operationEvidence,omitempty"`
	AuditedAt             string `json:"auditedAt"`
	RecordedAt            string `json:"recordedAt"`
}

// NewQueryAuditedPayablesEndpoint 交回审核应付册的 HTTP 入口（GET /settlement-audited-payables）。可选检索维 documentNumber、
// claim、supplier 各自精确相等、同给取交集（ports.AuditedPayableFilter）；不收别的参数，理由同 NewQuerySettlementStatementsEndpoint。
func NewQueryAuditedPayablesEndpoint(intake CatalogueQueryIntake, reader AuditedPayableCatalogueReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		values := request.URL.Query()
		filter := ports.AuditedPayableFilter{
			DocumentNumber: values.Get("documentNumber"), Claim: values.Get("claim"), Supplier: values.Get("supplier"),
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		// 多取一行只为判页外还有没有，理由同账单册。
		rows, err := reader.ListAuditedPayables(request.Context(), query.Scope.Tenant(), filter, query.Limit+1)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		truncated := len(rows) > query.Limit
		if truncated {
			rows = rows[:query.Limit]
		}
		bodies := make([]auditedPayableBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, auditedPayableBody{
				Payable: row.Payable, DocumentNumber: row.DocumentNumber, DocumentNumberAbsence: row.DocumentNumberAbsence,
				Claim: row.Claim, Line: row.Line, ExpectedVersion: row.ExpectedVersion, Supplier: row.Supplier, Period: row.Period,
				LegalEntity: row.LegalEntity, Account: row.Account, Currency: row.Currency, Amount: minorAmount(row.AmountMinor),
				Auditor: row.AuditorRef, OperationEvidence: row.OperationEvidence,
				AuditedAt: rfc3339(row.AuditedAt), RecordedAt: rfc3339(row.RecordedAt),
			})
		}
		writeJSON(response, http.StatusOK, auditedPayableListResponse{
			Outcome: outcomeAuditedPayablesListed, Truncated: truncated, Payables: bodies,
		})
	})
}
