package domain

import "go.idp.xyz/idp-parcel/internal/platform/docnumber"

// ShipmentDocumentNumberType 是委托单据号的类型声明（ADR-0355 决定五、六；界面词「运单号」）。日期按租户的单据编号
// 时区（PS CONTEXT「委托」生命周期草稿 → 已提交那一句）：委托在提交时还没有发件网点——网点要到接受之后的路由里才定，
// 提交载荷里也没有这一格；客户账户本身不登业务时区（对账安排上的业务时区属结算账户，管的是归属日与截单）。开立时
// 因此找不到一个已登记的开立方业务时区可取。运单号不带短横，便于条码与客户系统录入。
var ShipmentDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "SHIPMENT",
	DefaultTemplate: "P{yyyy}{mm}{dd}{seq:6}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// DocumentNumberTypes 交回本上下文声明的全部单据编号类型，由组合根汇总进规则册。
func DocumentNumberTypes() []docnumber.TypeDeclaration {
	return []docnumber.TypeDeclaration{ShipmentDocumentNumberType}
}
