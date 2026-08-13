package domain

import (
	"errors"
	"time"
)

var (
	// ErrInvalidRegulatoryDisposition 是承接决定立不起来：三格形状不合（接受不带对象、
	// 拒接反带对象或缺拒因、部分承接缺任何一半）或必备引用缺席。
	ErrInvalidRegulatoryDisposition = errors.New("transport fulfillment: invalid regulatory disposition acceptance")
	// ErrMovementAuthorityMissing 单列：扣留或处置决定本身不是移动授权（UC-TF-001——
	// 「不把扣留决定本身解释为移动授权」），缺当前有效移动授权时承接不成立，但那是
	// 待补充的续办，不是形状错。
	ErrMovementAuthorityMissing = errors.New("transport fulfillment: movement authority missing")
)

// CollaborationItemReference 指名 `UC-CC-008` 形成的运输协作事项。事项本体归关务，
// 这里只引用。
type CollaborationItemReference struct{ requiredValue }

func NewCollaborationItemReference(value string) (CollaborationItemReference, error) {
	required, err := newRequiredValue("collaboration item reference", value)
	return CollaborationItemReference{required}, err
}

// MovementAuthorityReference 指名当前有效的移动授权依据。它与监管决定引用分开——
// 决定说「要处置」，授权说「允许为此移动」，两样都在承接才立得住。
type MovementAuthorityReference struct{ requiredValue }

func NewMovementAuthorityReference(value string) (MovementAuthorityReference, error) {
	required, err := newRequiredValue("movement authority reference", value)
	return MovementAuthorityReference{required}, err
}

// DispositionAcceptanceKind 是承接决定的封闭三格（UC-TF-001 结果契约：已承接、部分
// 承接、已拒绝；待补充与未受理是编排层答复，不是决定）。
type DispositionAcceptanceKind uint8

const (
	DispositionAcceptanceKindInvalid DispositionAcceptanceKind = iota
	DispositionAccepted
	DispositionPartiallyAccepted
	DispositionDeclined
)

func (kind DispositionAcceptanceKind) valid() bool {
	return kind >= DispositionAccepted && kind <= DispositionDeclined
}

func (kind DispositionAcceptanceKind) String() string {
	switch kind {
	case DispositionAccepted:
		return "ACCEPTED"
	case DispositionPartiallyAccepted:
		return "PARTIALLY_ACCEPTED"
	case DispositionDeclined:
		return "DECLINED"
	default:
		return ""
	}
}

// RegulatoryTransportDispositionSpec 是形成一份承接决定所需的全部输入。
type RegulatoryTransportDispositionSpec struct {
	Tenant            TenantID
	Item              CollaborationItemReference
	Basis             DispositionBasisReference
	Kind              DispositionAcceptanceKind
	AcceptedObjects   []CarriedObjectReference
	DeclineBasis      string
	MovementAuthority MovementAuthorityReference
	DecidedAt         time.Time
}

// RegulatoryTransportDisposition 是运输履约对一项监管运输协作的承接决定。承接不等于
// 建立旅程或开始移动（UC-TF-001 六层分离）——类型上没有旅程、交接或移动字段；旅程
// 启动消费的是它携带的监管依据引用（BasisKind 恒为监管格，普通处置不经关务协作链）。
type RegulatoryTransportDisposition struct {
	tenant            TenantID
	item              CollaborationItemReference
	basis             DispositionBasisReference
	kind              DispositionAcceptanceKind
	accepted          []CarriedObjectReference
	declineBasis      string
	movementAuthority MovementAuthorityReference
	decidedAt         time.Time
}

// FormRegulatoryTransportDisposition 形成承接决定。三格各守其形：接受必带非空无重
// 对象集与移动授权；拒接必带拒因、不带对象也不需要授权（不动就不需要动的许可）；
// 部分承接两半都要——已承接对象加未承接范围的拒因，缺任何一半就成了整批话术。
func FormRegulatoryTransportDisposition(
	spec RegulatoryTransportDispositionSpec,
) (RegulatoryTransportDisposition, error) {
	if !spec.Tenant.valid() ||
		!spec.Item.valid() ||
		!spec.Basis.valid() ||
		!spec.Kind.valid() ||
		spec.DecidedAt.IsZero() {
		return RegulatoryTransportDisposition{}, ErrInvalidRegulatoryDisposition
	}

	switch spec.Kind {
	case DispositionAccepted, DispositionPartiallyAccepted:
		if len(spec.AcceptedObjects) == 0 {
			return RegulatoryTransportDisposition{}, ErrInvalidRegulatoryDisposition
		}
		if !spec.MovementAuthority.valid() {
			return RegulatoryTransportDisposition{}, ErrMovementAuthorityMissing
		}
		if spec.Kind == DispositionAccepted && spec.DeclineBasis != "" {
			return RegulatoryTransportDisposition{}, ErrInvalidRegulatoryDisposition
		}
		if spec.Kind == DispositionPartiallyAccepted && spec.DeclineBasis == "" {
			return RegulatoryTransportDisposition{}, ErrInvalidRegulatoryDisposition
		}
	case DispositionDeclined:
		if len(spec.AcceptedObjects) != 0 || spec.DeclineBasis == "" {
			return RegulatoryTransportDisposition{}, ErrInvalidRegulatoryDisposition
		}
	}

	seen := map[CarriedObjectReference]struct{}{}
	for _, object := range spec.AcceptedObjects {
		if !object.valid() {
			return RegulatoryTransportDisposition{}, ErrInvalidRegulatoryDisposition
		}
		if _, exists := seen[object]; exists {
			return RegulatoryTransportDisposition{}, ErrInvalidRegulatoryDisposition
		}
		seen[object] = struct{}{}
	}

	return RegulatoryTransportDisposition{
		tenant:            spec.Tenant,
		item:              spec.Item,
		basis:             spec.Basis,
		kind:              spec.Kind,
		accepted:          append([]CarriedObjectReference(nil), spec.AcceptedObjects...),
		declineBasis:      spec.DeclineBasis,
		movementAuthority: spec.MovementAuthority,
		decidedAt:         spec.DecidedAt.UTC(),
	}, nil
}

func (decision RegulatoryTransportDisposition) Tenant() TenantID {
	return decision.tenant
}

func (decision RegulatoryTransportDisposition) Item() CollaborationItemReference {
	return decision.item
}

// BasisKind 恒为监管处置格：本决定只从关务协作链来，旅程启动据此分流双链。
func (decision RegulatoryTransportDisposition) BasisKind() DispositionBasisKind {
	return RegulatoryDispositionDecision
}

// Basis 是旅程启动消费的监管依据引用——承接决定产出它，不产出旅程。
func (decision RegulatoryTransportDisposition) Basis() DispositionBasisReference {
	return decision.basis
}

func (decision RegulatoryTransportDisposition) Kind() DispositionAcceptanceKind {
	return decision.kind
}

// AcceptedObjects 只在接受/部分承接时非空（副本）。
func (decision RegulatoryTransportDisposition) AcceptedObjects() []CarriedObjectReference {
	return append([]CarriedObjectReference(nil), decision.accepted...)
}

// DeclineBasis 只在拒接/部分承接时非空——未承接范围的逐项原因锚。
func (decision RegulatoryTransportDisposition) DeclineBasis() string {
	return decision.declineBasis
}

// MovementAuthority 只在接受/部分承接时有效。
func (decision RegulatoryTransportDisposition) MovementAuthority() (MovementAuthorityReference, bool) {
	return decision.movementAuthority, decision.movementAuthority.valid()
}

func (decision RegulatoryTransportDisposition) DecidedAt() time.Time {
	return decision.decidedAt
}
