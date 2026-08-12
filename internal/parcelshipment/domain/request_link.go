package domain

import "errors"

var (
	ErrInvalidRequestLink = errors.New("parcel shipment: invalid request link")
	// ErrPriorStateIncompatibleWithLink 与 ErrInvalidRequestLink 分开（ADR-0029 分格）：
	// 前者说「原委托不在这个关联方向要求的终态上」，客户该走的是资料修订或等决定；
	// 后者说这份关联请求本身立不起来。两者的续办动作不同，压成一格会把人指错路。
	ErrPriorStateIncompatibleWithLink = errors.New("parcel shipment: prior request state does not admit this link")
)

// RequestLinkKind 是关联新委托的封闭方向集合，每个方向锚在原委托的一个终态上：
//
//   - 已拒绝委托修正资料重提（`AT-PS-036`②）；
//   - 已接受委托增删拆并成员或新增服务需求（`AT-PS-036`③，UC-PS-006「原地替换类需求
//     形成关联新委托」）；
//   - 已撤回后重新提出需求（`AT-PS-076`「原委托不恢复」）。
//
// 刻意没有「已提交」方向：待决委托的普通纠错走同一委托的新提交版本（BD-PS-005），
// 给它开关联方向等于给绕过版本机制留门。
type RequestLinkKind uint8

const (
	RequestLinkKindInvalid RequestLinkKind = iota
	LinkRejectedCorrection
	LinkAcceptedReshaping
	LinkWithdrawnResubmission
)

func (kind RequestLinkKind) valid() bool {
	return kind >= LinkRejectedCorrection && kind <= LinkWithdrawnResubmission
}

func (kind RequestLinkKind) String() string {
	switch kind {
	case LinkRejectedCorrection:
		return "REJECTED_CORRECTION"
	case LinkAcceptedReshaping:
		return "ACCEPTED_RESHAPING"
	case LinkWithdrawnResubmission:
		return "WITHDRAWN_RESUBMISSION"
	default:
		return ""
	}
}

// requiredPriorState 是方向与原委托终态的互证表。方向不只是标签：它声称原委托处于某个
// 终态，声称不实的关联建立不起来——一份指着待决委托的「已拒绝修正」会让读关联的人以为
// 那份委托已经拒绝了。
func (kind RequestLinkKind) requiredPriorState() ShipmentRequestState {
	switch kind {
	case LinkRejectedCorrection:
		return ShipmentRequestRejected
	case LinkAcceptedReshaping:
		return ShipmentRequestAccepted
	case LinkWithdrawnResubmission:
		return ShipmentRequestWithdrawn
	default:
		return ShipmentRequestStateInvalid
	}
}

// PriorRequestLink 是关联新委托出生即携带的关联出处：指回原委托并声明方向。它只存在于
// **新**委托上——原委托的版本、决定与历史一概不动（`AT-PS-036`「三者都保留原版本或原
// 决定，不覆盖接受基线」），所以关联是新委托的出生属性，不是原委托的一次状态转移。
type PriorRequestLink struct {
	prior ShipmentRequestID
	kind  RequestLinkKind
}

// EstablishPriorRequestLink 用原委托本体（不是一个裸 ID）建立关联出处：方向与原委托
// 终态必须互证。拿聚合作参数正是为了让「声称已拒绝、实则待决」这类关联在建立处就死，
// 而不是等读它的人去核对。
func EstablishPriorRequestLink(prior ShipmentRequest, kind RequestLinkKind) (PriorRequestLink, error) {
	if !kind.valid() || !prior.shipmentRequestID.valid() {
		return PriorRequestLink{}, ErrInvalidRequestLink
	}
	if prior.state != kind.requiredPriorState() {
		return PriorRequestLink{}, ErrPriorStateIncompatibleWithLink
	}
	return PriorRequestLink{prior: prior.shipmentRequestID, kind: kind}, nil
}

func (link PriorRequestLink) PriorRequestID() ShipmentRequestID {
	return link.prior
}

func (link PriorRequestLink) Kind() RequestLinkKind {
	return link.kind
}

// established 区分「没有关联」与「关联成立」。零值即缺席：首次委托没有出处可指。
func (link PriorRequestLink) established() bool {
	return link.kind.valid() && link.prior.valid()
}
