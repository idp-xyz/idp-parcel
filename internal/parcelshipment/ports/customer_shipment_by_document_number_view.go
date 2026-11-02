package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是客户按委托的单据号（运单号）追踪时本上下文那一半的读口（ADR-0356 决定四；PS CONTEXT「委托」里客户按委托的单据号
// 追踪那一条）。消费方是 visibility-exception 的消费侧适配器（ADR-0025），它经 application 的
// ReadCustomerShipmentByDocumentNumberHandler 问到这里；委托、它的单据号与当前有效包裹都归本上下文，消费方不存副本。

// CustomerShipmentByDocumentNumberRecord 是按（租户，货主客户账户，单据号）找到的那一份委托。
type CustomerShipmentByDocumentNumberRecord struct {
	// State 是委托此刻的生命周期状态。已取消、已完成由包裹结果派生、不另成一个状态，库里仍是已接受，所以都落在已接受这一格。
	State domain.ShipmentRequestState
	// EffectiveParcels 只在已接受时在场：当前有效包裹的全集，按位次升序。其余状态为空——接受之前声明的包裹不是正式包裹
	// （PS CONTEXT），已拒绝、已撤回的委托没有服务。
	EffectiveParcels []domain.EffectiveParcel
}

// CustomerShipmentByDocumentNumberView 按（租户，货主客户账户，单据号）读一份委托。
//
// 客户账户是查询条件之一，不是读出来再比的一格：号属别的账户与查无此号在这一口上同答 found 为假，「存在但不是你的」这一位
// 不出本上下文（ADR-0356 决定四）。实现须把账户放进同一条语句的条件里，不得先按号读出再比账户（ADR-0356 越权风险点 1）。
// 号逐字比，不折算大小写或空白：号由编号规则渲染，折算就是另一个号。
// 没有单据号的委托（上线前开立、开立时编号时区未配置）按号不可达，同答 found 为假。
//
// 读不动交回错误，绝不折成 found 为假：那会把一次该重试的故障说成「没有这单」。
type CustomerShipmentByDocumentNumberView interface {
	FindCustomerShipmentByDocumentNumber(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.CustomerAccountID,
		number domain.ShipmentDocumentNumber,
	) (CustomerShipmentByDocumentNumberRecord, bool, error)
}
