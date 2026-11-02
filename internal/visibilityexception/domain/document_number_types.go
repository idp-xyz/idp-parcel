package domain

import "go.idp.xyz/idp-parcel/internal/platform/docnumber"

// ExceptionCaseDocumentNumberType 是异常案件的类型声明（ADR-0355 决定五、六）。案件的开立方没有已登记的业务时区
// （ADR-0355 越权风险点 5 的例子），日期按租户的单据编号时区。
var ExceptionCaseDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "EXCEPTION_CASE",
	DefaultTemplate: "EX-{yyyy}{mm}{dd}-{seq:4}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// ClaimDocumentNumberType 是客户索赔项的类型声明，日期按租户的单据编号时区。
var ClaimDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "CLAIM",
	DefaultTemplate: "CLM-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// DocumentNumberTypes 交回本上下文声明的全部单据编号类型，由组合根汇总进规则册。
func DocumentNumberTypes() []docnumber.TypeDeclaration {
	return []docnumber.TypeDeclaration{ExceptionCaseDocumentNumberType, ClaimDocumentNumberType}
}
