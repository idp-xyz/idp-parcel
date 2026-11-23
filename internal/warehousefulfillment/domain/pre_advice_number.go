package domain

import "go.idp.xyz/idp-parcel/internal/platform/docnumber"

// InboundPreAdviceDocumentNumberType 是入库预报的单据号类型声明。日期时区来路是开立方：
// 交仓已登记的业务时区，没登记就留空，由编号组件改用租户的单据编号时区。
var InboundPreAdviceDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "INBOUND_PRE_ADVICE",
	DefaultTemplate: "IB-{yyyy}{mm}{dd}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromIssuer,
}

// OutboundRequestDocumentNumberType 是出库请求的单据号类型声明。日期时区来路与入库预报相同：交仓已登记的业务时区。
var OutboundRequestDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "OUTBOUND_REQUEST",
	DefaultTemplate: "OB-{yyyy}{mm}{dd}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromIssuer,
}

// ReturnPreAdviceDocumentNumberType 是退货预报的单据号类型声明。日期时区来路与入库预报相同。
var ReturnPreAdviceDocumentNumberType = docnumber.TypeDeclaration{
	Code:            "RETURN_PRE_ADVICE",
	DefaultTemplate: "RT-{yyyy}{mm}{dd}-{seq:5}",
	DateTimeZone:    docnumber.DateTimeZoneFromIssuer,
}

// DocumentNumberTypes 交回本上下文声明的全部单据编号类型，由组合根汇总进规则册。
func DocumentNumberTypes() []docnumber.TypeDeclaration {
	return []docnumber.TypeDeclaration{
		InboundPreAdviceDocumentNumberType, OutboundRequestDocumentNumberType, ReturnPreAdviceDocumentNumberType,
	}
}
