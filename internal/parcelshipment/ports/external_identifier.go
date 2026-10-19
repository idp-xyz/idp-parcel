package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ParcelExternalIdentifierView 按（租户，号码）核对包裹身份（ADR-0184 决定五、ADR-0195）。号码逐字比对；命中多件即
// 交回多件、去重，不按形成时间、来源或分配方挑一件——消费方据候选与歧义信号判身份。不交分配方、客户账户或委托
// （CONTEXT-MAP `parcel-shipment → node-operations`）。查无交空；依赖读不动作为错误返回，空与「没查到」不是一回事。
//
// 号码收成品类型：过不了 NewExternalIdentifierNumber 的号（ErrExternalIdentifierNumberNotStorable）交不进来，它也不可能在册，
// 消费方在构造那一步就答查无此号、不必来问；把它当成读不动，会让一个确定不在册的号落进永远续不上的重核。
type ParcelExternalIdentifierView interface {
	FindParcelsByExternalIdentifier(
		ctx context.Context,
		tenant domain.TenantID,
		number domain.ExternalIdentifierNumber,
	) (ExternalIdentifierParcels, error)
}

// ExternalIdentifierParcels 是一次按号码核对的答复。两半出自同一次读：分两次读，一份委托恰在两次之间被接受，就会
// 前一半没看见它的关联、后一半又看见它已不是`已提交`，唯一命中于是不带信号。
type ExternalIdentifierParcels struct {
	// Parcels 是此刻适用、标识包裹的关联所指的包裹。次序只为稳定，不表示谁优先。
	Parcels []domain.DeclaredParcelID
	// DeclaredBySubmittedRequest 是歧义信号：号码此刻同时被本租户里一份`已提交`委托声明为客户包裹件号（PS CONTEXT
	// 「歧义信号」）。声明不是关联，那件声明包裹不进 Parcels；信号也不说是哪份委托、哪个客户账户。
	DeclaredBySubmittedRequest bool
}
