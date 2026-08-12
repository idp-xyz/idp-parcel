package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidParcelCancellation = errors.New("parcel shipment: invalid parcel cancellation")
	// ErrIntakeBoundaryCrossed 说明有效网络收寄已先行成立：不能形成取消，请求走收寄后
	// 处置路（UC-PS-006 决定矩阵「有效网络收寄已经先行成立→不得回退为已取消」）。
	ErrIntakeBoundaryCrossed = errors.New("parcel shipment: the intake boundary has been crossed")
)

// ParcelCancellationID 是包裹取消决定的标识。
type ParcelCancellationID struct{ requiredValue }

func NewParcelCancellationID(value string) (ParcelCancellationID, error) {
	required, err := newRequiredValue("parcel cancellation ID", value)
	return ParcelCancellationID{required}, err
}

// CancellationAuthorityReference 指名允许本次取消的规则依据。与委托撤回的授权分开：
// 撤回终止未决委托，取消终止已接受包裹的服务——两条授权目录不同。
type CancellationAuthorityReference struct{ requiredValue }

func NewCancellationAuthorityReference(value string) (CancellationAuthorityReference, error) {
	required, err := newRequiredValue("cancellation authority reference", value)
	return CancellationAuthorityReference{required}, err
}

// CancellationRequesterReference 指名提出取消的客户或授权运营角色。
type CancellationRequesterReference struct{ requiredValue }

func NewCancellationRequesterReference(value string) (CancellationRequesterReference, error) {
	required, err := newRequiredValue("cancellation requester reference", value)
	return CancellationRequesterReference{required}, err
}

// CancellationReasonReference 指名取消原因。
type CancellationReasonReference struct{ requiredValue }

func NewCancellationReasonReference(value string) (CancellationReasonReference, error) {
	required, err := newRequiredValue("cancellation reason reference", value)
	return CancellationReasonReference{required}, err
}

// CurrentIntakeFact 是取消边界核验所需的「当前有效收寄事实」视图：在场与否加实际发生
// 时间。它是核验输入不是判断——事实由采用记录持有，这里只带过来比对。
type CurrentIntakeFact struct {
	Present    bool
	OccurredAt time.Time
}

// ParcelCancellationSpec 是形成一份包裹取消决定所需的全部输入。
type ParcelCancellationSpec struct {
	ID          ParcelCancellationID
	Parcel      DeclaredParcelID
	Requester   CancellationRequesterReference
	Authority   CancellationAuthorityReference
	Reason      CancellationReasonReference
	RequestedAt time.Time
}

// ParcelCancellation 是逐包裹的取消终局决定。取消权按包裹判断——它挂在明确包裹上，
// 不是委托级状态；身份与接受基线随决定保留不删除。
type ParcelCancellation struct {
	id          ParcelCancellationID
	parcel      DeclaredParcelID
	requester   CancellationRequesterReference
	authority   CancellationAuthorityReference
	reason      CancellationReasonReference
	requestedAt time.Time
}

// DecideParcelCancellation 在取消边界核验之后形成取消决定（AT-PS-077 的领域面）。
// 有效网络收寄已在场即拒绝——无论收寄发生在请求前还是请求后：先合法成立者赢
// （AT-PS-079），本函数在决定提交边界执行，提交前收寄成立就是收寄赢。授权、请求方
// 与原因缺一不可——没有授权依据的取消与运营误操作分不开。
func DecideParcelCancellation(
	spec ParcelCancellationSpec,
	intake CurrentIntakeFact,
) (ParcelCancellation, error) {
	if intake.Present {
		return ParcelCancellation{}, ErrIntakeBoundaryCrossed
	}
	if !spec.ID.valid() ||
		!spec.Parcel.valid() ||
		!spec.Requester.valid() ||
		!spec.Authority.valid() ||
		!spec.Reason.valid() ||
		spec.RequestedAt.IsZero() {
		return ParcelCancellation{}, ErrInvalidParcelCancellation
	}
	return ParcelCancellation{
		id:          spec.ID,
		parcel:      spec.Parcel,
		requester:   spec.Requester,
		authority:   spec.Authority,
		reason:      spec.Reason,
		requestedAt: spec.RequestedAt.UTC(),
	}, nil
}

func (cancellation ParcelCancellation) ID() ParcelCancellationID {
	return cancellation.id
}

func (cancellation ParcelCancellation) Parcel() DeclaredParcelID {
	return cancellation.parcel
}

func (cancellation ParcelCancellation) Requester() CancellationRequesterReference {
	return cancellation.requester
}

func (cancellation ParcelCancellation) Authority() CancellationAuthorityReference {
	return cancellation.authority
}

func (cancellation ParcelCancellation) Reason() CancellationReasonReference {
	return cancellation.reason
}

// RequestedAt 是取消的业务时间——与收寄并发时按权威事实的业务发生顺序裁决，不按
// 消息到达顺序（UC-PS-003/006 共同的硬句）。
func (cancellation ParcelCancellation) RequestedAt() time.Time {
	return cancellation.requestedAt
}
