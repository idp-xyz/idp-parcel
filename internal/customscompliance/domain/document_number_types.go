package domain

import "go.idp.xyz/idp-parcel/internal/platform/docnumber"

// CustomsCaseDocumentNumberType 是关务案件的类型声明（ADR-0355 决定五、六）。案件的开立方没有已登记的业务时区，
// 日期按租户的单据编号时区；监管方分配的编号是外部标识，不在此列。
var CustomsCaseDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "CUSTOMS_CASE",
	DefaultTemplate: "CC-{yyyy}{mm}{dd}-{seq:4}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// DocumentNumberTypes 交回本上下文声明的全部单据编号类型，由组合根汇总进规则册。
func DocumentNumberTypes() []docnumber.TypeDeclaration {
	return []docnumber.TypeDeclaration{CustomsCaseDocumentNumberType}
}
