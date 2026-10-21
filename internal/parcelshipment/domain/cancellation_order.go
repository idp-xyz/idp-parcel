package domain

import (
	"errors"
	"time"
)

// 本文件是取消与边界事实的先后判法（ADR-0254）：取消已提交之后才到的边界事实，两边都在册之后只比
// 两个业务时间。判据只放这一处，网络服务的收寄采用与面单渠道服务的终局判断都调它——两种服务形态
// 各写一份比较，同一对时间就会在两边得出相反结论（ADR-0254「候选与反方」末条否决的正是这个）。

// ErrInvalidCancellationBoundaryFact 说比较的两侧有一侧立不起：取消缺标识或业务时间，或边界事实缺
// 种类、引用或发生时间。没有时间就没有先后可比，判法不替调用方补。
var ErrInvalidCancellationBoundaryFact = errors.New("parcel shipment: invalid cancellation boundary fact")

// CancellationBoundaryKind 指名与取消比先后的那类边界事实，按服务形态各一格（PS CONTEXT 取消一条
// 的括注：网络服务被采用的收寄来源，面单渠道服务的实际承运商首次有效收寄）。
type CancellationBoundaryKind uint8

const (
	CancellationBoundaryKindInvalid CancellationBoundaryKind = iota
	NetworkIntakeBoundary
	CarrierFirstEffectivePickupBoundary
)

func (kind CancellationBoundaryKind) String() string {
	switch kind {
	case NetworkIntakeBoundary:
		return "NETWORK_INTAKE"
	case CarrierFirstEffectivePickupBoundary:
		return "CARRIER_FIRST_EFFECTIVE_PICKUP"
	default:
		return ""
	}
}

func (kind CancellationBoundaryKind) valid() bool {
	return kind == NetworkIntakeBoundary || kind == CarrierFirstEffectivePickupBoundary
}

// CancellationBoundaryReference 指名那条边界事实的那一代：网络服务是（来源种类，来源版本），面单渠道
// 服务是（收寄事实，版本）。必须带版本：来源更正换了一代，就是另一次比较、另一份可能的冲突。
type CancellationBoundaryReference struct{ requiredValue }

func NewCancellationBoundaryReference(value string) (CancellationBoundaryReference, error) {
	required, err := newRequiredValue("cancellation boundary reference", value)
	return CancellationBoundaryReference{required}, err
}

// CancellationBoundaryFact 是比较的边界一侧。OccurredAt 是边界事实的业务发生时间：网络服务取被采用
// 来源的实际发生时间，面单渠道服务取 TF 判过的首次有效收寄业务发生时间（ADR-0135 决定三）——都不是
// 本上下文读到它的时间。
type CancellationBoundaryFact struct {
	Kind       CancellationBoundaryKind
	Reference  CancellationBoundaryReference
	OccurredAt time.Time
}

func (boundary CancellationBoundaryFact) valid() bool {
	return boundary.Kind.valid() && boundary.Reference.valid() && !boundary.OccurredAt.IsZero()
}

// CancellationOrderConflict 是 ADR-0254 决定二第 3 格：取消已提交，此后才到的边界事实发生不晚于
// 取消的业务时间——取消形成时包裹其实已越过边界，只是本系统还不知道。它只记下两份事实与各自的
// 业务时间，不是决定：不改写取消，不形成责任起点或非取消终局；解开它的是另一份明确的业务决定。
type CancellationOrderConflict struct {
	parcel       DeclaredParcelID
	cancellation ParcelCancellationID
	cancelledAt  time.Time
	boundary     CancellationBoundaryFact
}

func (conflict CancellationOrderConflict) Parcel() DeclaredParcelID {
	return conflict.parcel
}

func (conflict CancellationOrderConflict) Cancellation() ParcelCancellationID {
	return conflict.cancellation
}

// CancelledAt 是取消一侧参与比较的业务时间，即取消决定上的 RequestedAt。
func (conflict CancellationOrderConflict) CancelledAt() time.Time {
	return conflict.cancelledAt
}

func (conflict CancellationOrderConflict) Boundary() CancellationBoundaryFact {
	return conflict.boundary
}

// JudgeCancellationOrder 比一份已提交的取消与一条此后才到的边界事实（ADR-0254 决定二第 2、3 格）。
// conflicted 为 false 即取消在先：边界事实严格晚于取消的业务时间。为 true 时交回冲突。
//
// 两个时间相等按冲突：同一时刻哪一边在先都没有依据（ADR-0254 决定二）。若改判相等为取消在先，改的
// 只是这里的比较符。决定提交时刻、消息到达时刻都不进来——它们说的是本系统何时知道。
func JudgeCancellationOrder(
	cancellation ParcelCancellation,
	boundary CancellationBoundaryFact,
) (CancellationOrderConflict, bool, error) {
	if !cancellation.id.valid() || !cancellation.parcel.valid() || cancellation.requestedAt.IsZero() ||
		!boundary.valid() {
		return CancellationOrderConflict{}, false, ErrInvalidCancellationBoundaryFact
	}
	if cancellation.requestedAt.Before(boundary.OccurredAt) {
		return CancellationOrderConflict{}, false, nil
	}
	return CancellationOrderConflict{
		parcel:       cancellation.parcel,
		cancellation: cancellation.id,
		cancelledAt:  cancellation.requestedAt,
		boundary: CancellationBoundaryFact{
			Kind:       boundary.Kind,
			Reference:  boundary.Reference,
			OccurredAt: boundary.OccurredAt.UTC(),
		},
	}, true, nil
}
