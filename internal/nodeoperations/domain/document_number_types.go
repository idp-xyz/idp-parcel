package domain

import "go.idp.xyz/idp-parcel/internal/platform/docnumber"

// DeliveryPreAdviceDocumentNumberType 是送站预报的类型声明（ADR-0355 决定五、六）。网点开立的单据按该网点的业务
// 时区出日期（ADR-0355 决定七），所以声明开立方有业务时区；网点没登时退到租户的单据编号时区。
var DeliveryPreAdviceDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "DELIVERY_PRE_ADVICE",
	DefaultTemplate: "PA-{yyyy}{mm}{dd}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromIssuer,
}

// DeliveryHandoverNoteDocumentNumberType 是送站交接单的类型声明，时区来路同送站预报。号在服务端收下节点开立时取，
// 日期按服务端收下的那一刻，不按设备声明的开立时刻（ADR-0355 决定七）。
var DeliveryHandoverNoteDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "DELIVERY_HANDOVER_NOTE",
	DefaultTemplate: "HO-{yyyy}{mm}{dd}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromIssuer,
}

// DocumentNumberTypes 交回本上下文声明的全部单据编号类型，由组合根汇总进规则册。
func DocumentNumberTypes() []docnumber.TypeDeclaration {
	return []docnumber.TypeDeclaration{DeliveryPreAdviceDocumentNumberType, DeliveryHandoverNoteDocumentNumberType}
}
