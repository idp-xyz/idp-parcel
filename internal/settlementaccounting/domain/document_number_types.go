package domain

import "go.idp.xyz/idp-parcel/internal/platform/docnumber"

// CustomerStatementDocumentNumberType 是对账单的类型声明（ADR-0355 决定五、九：对账单的固定单号就是它的单据号）。
// 日期按结算账户对账安排的业务时区（ADR-0269）；能发布的对账单总有它（ADR-0355 决定七），所以声明开立方有业务时区。
var CustomerStatementDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "CUSTOMER_STATEMENT",
	DefaultTemplate: "STMT-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromIssuer,
}

// StatementDisputeDocumentNumberType 是费用争议的类型声明。争议只对一张已发布的对账单提出，那张单的结算账户总有
// 对账安排的业务时区，所以按开立方有业务时区声明：日期与被争对账单同一个时区，客户看到的单号与争议号落在同一个
// 账期日历里；那个账户若读不到对账安排，退到租户的单据编号时区。
var StatementDisputeDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "STATEMENT_DISPUTE",
	DefaultTemplate: "DSP-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromIssuer,
}

// AuditedPayableDocumentNumberType 是审核应付的类型声明。应付归入的结算账户登了对账安排时，日期按那份安排的业务
// 时区——结算账户的业务时区只登在对账安排上（ADR-0269），不另造一处；没登（应付账户多半不对账）就用租户的单据
// 编号时区。
var AuditedPayableDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "AUDITED_PAYABLE",
	DefaultTemplate: "AP-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromIssuer,
}

// DocumentNumberTypes 交回本上下文声明的全部单据编号类型，由组合根汇总进规则册。
func DocumentNumberTypes() []docnumber.TypeDeclaration {
	return []docnumber.TypeDeclaration{
		CustomerStatementDocumentNumberType,
		StatementDisputeDocumentNumberType,
		AuditedPayableDocumentNumberType,
	}
}
