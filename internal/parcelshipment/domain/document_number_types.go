package domain

import "go.idp.xyz/idp-parcel/internal/platform/docnumber"

// ShipmentDocumentNumberType 是委托单据号的类型声明（ADR-0355 决定五、六；界面词「运单号」）。委托的开立方没有
// 已登记的业务时区（ADR-0355 越权风险点 5 的例子），日期按租户的单据编号时区；开立时若找得到可用的业务时区
// （例如发件网点），由实现票改这一格并写进本上下文 CONTEXT。运单号不带短横，便于条码与客户系统录入。
var ShipmentDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "SHIPMENT",
	DefaultTemplate: "P{yyyy}{mm}{dd}{seq:6}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// DocumentNumberTypes 交回本上下文声明的全部单据编号类型，由组合根汇总进规则册。
func DocumentNumberTypes() []docnumber.TypeDeclaration {
	return []docnumber.TypeDeclaration{ShipmentDocumentNumberType}
}
