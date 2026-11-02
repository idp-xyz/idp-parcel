package domain

import "go.idp.xyz/idp-parcel/internal/platform/docnumber"

// CustomerStatementDocumentNumberType 是对账单的类型声明（ADR-0355 决定五、九：对账单的固定单号就是它的单据号）。
// 日期按结算账户对账安排的业务时区（ADR-0269）；能发布的对账单总有它（ADR-0355 决定七），所以声明开立方有业务时区。
var CustomerStatementDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "CUSTOMER_STATEMENT",
	DefaultTemplate: "STMT-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromIssuer,
}

// StatementDisputeDocumentNumberType 是费用争议的类型声明。ADR-0355 决定七只点名网点与结算账户两种开立方业务时区，
// 争议是否取被争对账单那个账户的业务时区留给实现票核定；核定之前按开立方没有业务时区声明，日期用租户的单据编号时区。
var StatementDisputeDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "STATEMENT_DISPUTE",
	DefaultTemplate: "DSP-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// AuditedPayableDocumentNumberType 是审核应付的类型声明。开立方业务时区的取法同费用争议，留给实现票核定。
var AuditedPayableDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "AUDITED_PAYABLE",
	DefaultTemplate: "AP-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// DocumentNumberTypes 交回本上下文声明的全部单据编号类型，由组合根汇总进规则册。
func DocumentNumberTypes() []docnumber.TypeDeclaration {
	return []docnumber.TypeDeclaration{
		CustomerStatementDocumentNumberType,
		StatementDisputeDocumentNumberType,
		AuditedPayableDocumentNumberType,
	}
}
