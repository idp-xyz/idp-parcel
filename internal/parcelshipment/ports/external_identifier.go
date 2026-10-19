package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ParcelExternalIdentifierView 按（租户，号码）核对包裹身份（ADR-0184 决定五、ADR-0195、ADR-0212 决定四）。号码逐字比对；命中多件
// 即交回多件、去重，不按形成时间、来源或分配方挑一件——消费方据候选、范围相容与歧义信号判身份。不交分配方、客户账户或委托
// （CONTEXT-MAP `parcel-shipment → node-operations`）。查无交空；依赖读不动作为错误返回，空与「没查到」不是一回事。
//
// deliveryScope 是 node-operations 给出的这件实物的可验证交付范围（一组客户账户），可以为空。它不收窄候选：候选照旧在租户内按号码
// 全部交回，范围只用来逐候选答范围相容、把歧义信号收到范围内客户账户的委托。答复里的范围相容只是是否值，所比的客户账户就是调用方
// 给的这一组，本上下文不回传任何别的客户账户。
//
// 号码收核对用的类型 ExternalIdentifierLookupNumber，不收入册的 ExternalIdentifierNumber：入册门只管写，读的一侧不拿它挡已在册
// 的号，门立起来之前进册的超长号照样比得上。带 NUL 的号构造不出来，它也不可能在册，消费方在构造那一步就答查无此号、不必
// 来问；把它当成读不动，会让一个确定不在册的号落进永远续不上的重核。
type ParcelExternalIdentifierView interface {
	FindParcelsByExternalIdentifier(
		ctx context.Context,
		tenant domain.TenantID,
		number domain.ExternalIdentifierLookupNumber,
		deliveryScope []domain.CustomerAccountID,
	) (ExternalIdentifierParcels, error)
}

// ExternalIdentifierParcels 是一次按号码核对的答复。候选、范围相容与信号出自同一次读：分两次读，一份委托恰在两次之间被接受，就会
// 前一半没看见它的关联、后一半又看见它已不是`已提交`，那一件于是既不是候选也不带信号。
type ExternalIdentifierParcels struct {
	// Candidates 是此刻适用、标识包裹的关联所指的包裹，逐件带范围相容。次序只为稳定，不表示谁优先。
	Candidates []ExternalIdentifierCandidate
	// DeclaredBySubmittedRequest 是歧义信号：号码此刻同时被一份`已提交`委托声明为客户包裹件号（PS CONTEXT「歧义信号」）。核对带了
	// 范围时只认范围内客户账户的委托，没带时认本租户的。声明不是关联，那件声明包裹不进候选；信号也不说是哪份委托、哪个客户账户。
	DeclaredBySubmittedRequest bool
}

// ExternalIdentifierCandidate 是一个包裹候选。ScopeCompatible 说号码是否经适用范围不在客户账户编号空间的关联（渠道包裹件号），或
// 适用范围是核对所带客户账户之一编号空间的关联，标识这件包裹（PS CONTEXT「范围相容」）；核对没带范围时，只经客户包裹件号命中的
// 候选都不范围相容。
type ExternalIdentifierCandidate struct {
	Parcel          domain.DeclaredParcelID
	ScopeCompatible bool
}
