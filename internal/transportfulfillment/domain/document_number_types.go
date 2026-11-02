package domain

import "go.idp.xyz/idp-parcel/internal/platform/docnumber"

// BookingDocumentNumberType 是订舱的类型声明（ADR-0355 决定五、六），订舱申请登记时取号。日期按租户的单据编号时区：订舱由
// 租户的运输运营在委托下提出，开立方不是网点，也不是带对账安排的结算账户，没有已登记的业务时区可用。订舱指名的班次
// 若出自周期安排，周期安排上登的时区说的是那张时刻表怎么读，不是开立方的业务时区；临时班次连这一格都没有，拿它出日期
// 会让同一租户的订舱号按班次来历一半一个时区。承运方答复里的订舱号是外部标识，不在此列。
var BookingDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "BOOKING",
	DefaultTemplate: "BK-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// TransportCommissionDocumentNumberType 是运输委托的类型声明，进入已提交时取号。日期按租户的单据编号时区：委托是租户向
// 服务提供方提出的，请求方与采购责任法人都没有已登记的业务时区，服务提供方是相对方、不是开立方。
var TransportCommissionDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "TRANSPORT_COMMISSION",
	DefaultTemplate: "TC-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// DocumentNumberTypes 交回本上下文声明的全部单据编号类型，由组合根汇总进规则册。
func DocumentNumberTypes() []docnumber.TypeDeclaration {
	return []docnumber.TypeDeclaration{BookingDocumentNumberType, TransportCommissionDocumentNumberType}
}
