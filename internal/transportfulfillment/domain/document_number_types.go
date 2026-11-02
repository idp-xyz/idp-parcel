package domain

import "go.idp.xyz/idp-parcel/internal/platform/docnumber"

// BookingDocumentNumberType 是订舱的类型声明（ADR-0355 决定五、六）。订舱的开立方没有已登记的业务时区，日期按租户的
// 单据编号时区；承运方答复里的订舱号是外部标识，不在此列。
var BookingDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "BOOKING",
	DefaultTemplate: "BK-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// TransportCommissionDocumentNumberType 是运输委托的类型声明，日期按租户的单据编号时区。
var TransportCommissionDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "TRANSPORT_COMMISSION",
	DefaultTemplate: "TC-{yyyy}{mm}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromTenant,
}

// DocumentNumberTypes 交回本上下文声明的全部单据编号类型，由组合根汇总进规则册。
func DocumentNumberTypes() []docnumber.TypeDeclaration {
	return []docnumber.TypeDeclaration{BookingDocumentNumberType, TransportCommissionDocumentNumberType}
}
