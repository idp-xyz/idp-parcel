package domain

import (
	"errors"
	"time"
)

var ErrInvalidControlTransition = errors.New("node operations: invalid control transition")

// AuthoritativeHandoverReference 指名 transport-fulfillment 的一版权威运输交接：载运对象、交接范围与结果版本，三格都是 TF 的引用。
// 本上下文不解析它们：对象是正式包裹身份还是集运单元，由本上下文自己的册子认（ADR-0214 决定二）；版本只用来认重放与更正链。TF 的
// 更正保留对象与范围、只换版本，所以同一（对象，范围）下的各版是一条更正链。
type AuthoritativeHandoverReference struct {
	object  requiredValue
	scope   requiredValue
	version requiredValue
}

func NewAuthoritativeHandoverReference(object, scope, version string) (AuthoritativeHandoverReference, error) {
	objectValue, err := newRequiredValue("authoritative handover object", object)
	if err != nil {
		return AuthoritativeHandoverReference{}, err
	}
	scopeValue, err := newRequiredValue("authoritative handover scope", scope)
	if err != nil {
		return AuthoritativeHandoverReference{}, err
	}
	versionValue, err := newRequiredValue("authoritative handover version", version)
	if err != nil {
		return AuthoritativeHandoverReference{}, err
	}
	return AuthoritativeHandoverReference{object: objectValue, scope: scopeValue, version: versionValue}, nil
}

func (reference AuthoritativeHandoverReference) Object() string {
	return reference.object.String()
}

func (reference AuthoritativeHandoverReference) Scope() string {
	return reference.scope.String()
}

func (reference AuthoritativeHandoverReference) Version() string {
	return reference.version.String()
}

// SameChain 报告两版是不是同一条更正链上的：对象与范围相同。
func (reference AuthoritativeHandoverReference) SameChain(other AuthoritativeHandoverReference) bool {
	return reference.object == other.object && reference.scope == other.scope
}

func (reference AuthoritativeHandoverReference) valid() bool {
	return reference.object.valid() && reference.scope.valid() && reference.version.valid()
}

// ControlTransitionDirection 是一行控制转移的方向：交出节点那一侧的转出，或接收节点那一侧的转入（转入由票 demo-chain-gaps/62 落）。
type ControlTransitionDirection uint8

const (
	ControlTransitionDirectionInvalid ControlTransitionDirection = iota
	ControlTransitionOut
	ControlTransitionIn
)

func (direction ControlTransitionDirection) String() string {
	switch direction {
	case ControlTransitionOut:
		return "OUT"
	case ControlTransitionIn:
		return "IN"
	default:
		return ""
	}
}

func ParseControlTransitionDirection(name string) (ControlTransitionDirection, error) {
	for _, direction := range []ControlTransitionDirection{ControlTransitionOut, ControlTransitionIn} {
		if direction.String() == name {
			return direction, nil
		}
	}
	return ControlTransitionDirectionInvalid, ErrInvalidControlTransition
}

// ControlTransitionOutcome 是一版交接在一个方向上推进了控制没有。
type ControlTransitionOutcome uint8

const (
	ControlTransitionOutcomeInvalid ControlTransitionOutcome = iota
	ControlAdvanced
	ControlNotAdvanced
)

func (outcome ControlTransitionOutcome) String() string {
	switch outcome {
	case ControlAdvanced:
		return "ADVANCED"
	case ControlNotAdvanced:
		return "NOT_ADVANCED"
	default:
		return ""
	}
}

func ParseControlTransitionOutcome(name string) (ControlTransitionOutcome, error) {
	for _, outcome := range []ControlTransitionOutcome{ControlAdvanced, ControlNotAdvanced} {
		if outcome.String() == name {
			return outcome, nil
		}
	}
	return ControlTransitionOutcomeInvalid, ErrInvalidControlTransition
}

// ControlNotAdvancedReason 是`已交接`没能推进节点控制的原因（ADR-0214 决定二、四、五）。每一格都入账、不推进：控制只在对得上一件
// 作业实物、它恰有一份在身控制时转出；对不上的不猜，也不留给重投——原样重投答的还是同一件事。
type ControlNotAdvancedReason uint8

const (
	ControlNotAdvancedReasonAbsent ControlNotAdvancedReason = iota
	// NoAssociatedUnit：本租户册上没有一件作业实物此刻关联到交接对象这件正式包裹。
	NoAssociatedUnit
	// SeveralAssociatedUnits：不止一件作业实物此刻关联到它，说不出交走的是哪一件。
	SeveralAssociatedUnits
	// ConsolidationUnitPendingRule：交接对象是本租户的集运单元。单元控制与成员「随单元受控」的规则还没立，不拿单元的交接替成员
	// 逐件转出。
	ConsolidationUnitPendingRule
	// NoActiveControl：对上的那件实物此刻没有在身的节点控制（从未在节点成立，或已经转出）。
	NoActiveControl
	// SeveralActiveControls：那件实物此刻不止一份在身控制，说不出交接结束的是哪一份。
	SeveralActiveControls
	// HandoverPrecedesControl：交接的业务时间早于那份在身控制成立，它说的是更早的一次占有，结束不了此刻这一份。
	HandoverPrecedesControl
	// ControlSourceChainPendingRecompute：同一条更正链上先前的`已交接`已经转出过控制，这一版把它更正成了拒收或待确认。控制不回退给
	// 交出方（TF CONTEXT 禁止「直接把当前控制回退给早期交出方」），控制来源链留待重算。
	ControlSourceChainPendingRecompute
)

func (reason ControlNotAdvancedReason) String() string {
	switch reason {
	case NoAssociatedUnit:
		return "NO_ASSOCIATED_UNIT"
	case SeveralAssociatedUnits:
		return "SEVERAL_ASSOCIATED_UNITS"
	case ConsolidationUnitPendingRule:
		return "CONSOLIDATION_UNIT_PENDING_RULE"
	case NoActiveControl:
		return "NO_ACTIVE_CONTROL"
	case SeveralActiveControls:
		return "SEVERAL_ACTIVE_CONTROLS"
	case HandoverPrecedesControl:
		return "HANDOVER_PRECEDES_CONTROL"
	case ControlSourceChainPendingRecompute:
		return "CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE"
	default:
		return ""
	}
}

func ParseControlNotAdvancedReason(name string) (ControlNotAdvancedReason, error) {
	for _, reason := range []ControlNotAdvancedReason{
		NoAssociatedUnit, SeveralAssociatedUnits, ConsolidationUnitPendingRule,
		NoActiveControl, SeveralActiveControls, HandoverPrecedesControl, ControlSourceChainPendingRecompute,
	} {
		if reason.String() == name {
			return reason, nil
		}
	}
	return ControlNotAdvancedReasonAbsent, ErrInvalidControlTransition
}

// unitless 报告这一格没有作业实物可写：交接对象没对上任何一件，或对上的是集运单元。
func (reason ControlNotAdvancedReason) unitless() bool {
	return reason == NoAssociatedUnit || reason == SeveralAssociatedUnits || reason == ConsolidationUnitPendingRule
}

// ControlTransition 是节点控制转移册上的一行（ADR-0214 决定三）：一版权威运输交接在一个方向上推进了控制，或入账不推进。
//
// 收寄册是只追加的判断链，控制转移不是判断，所以另立这本册，而不改写收寄行的控制：控制此刻是否在身，要经折叠读口把这本册上的
// 转出折进收寄的控制里去读。
//
// 推进了的转出带被转出的那一份控制（已带转出依据与时刻）；入账的几格按原因带或不带作业实物与控制，形状由各构造函数与
// RehydrateControlTransition 守。
type ControlTransition struct {
	tenantID   TenantID
	handover   AuthoritativeHandoverReference
	direction  ControlTransitionDirection
	outcome    ControlTransitionOutcome
	reason     ControlNotAdvancedReason
	unit       HandlingUnitID
	control    PhysicalControl
	occurredAt time.Time
}

func (transition ControlTransition) TenantID() TenantID {
	return transition.tenantID
}

func (transition ControlTransition) Handover() AuthoritativeHandoverReference {
	return transition.handover
}

func (transition ControlTransition) Direction() ControlTransitionDirection {
	return transition.direction
}

func (transition ControlTransition) Outcome() ControlTransitionOutcome {
	return transition.outcome
}

// Reason 只在没推进时非零。
func (transition ControlTransition) Reason() ControlNotAdvancedReason {
	return transition.reason
}

// Unit 只在这一行对上了一件作业实物时给出。
func (transition ControlTransition) Unit() (HandlingUnitID, bool) {
	return transition.unit, transition.unit.valid()
}

// Control 只在这一行带控制时给出：转出了的那一份、交接早于其成立的那一份在身控制，或待重算的那一次先前转出。
func (transition ControlTransition) Control() (PhysicalControl, bool) {
	return transition.control, controlEstablished(transition.control)
}

// controlEstablished 认一格控制在不在场：零值从未成立。与 PhysicalControl.Active 分开——已转出的控制不在身，却仍在场。
func controlEstablished(control PhysicalControl) bool {
	return !control.establishedAt.IsZero()
}

// OccurredAt 是这一版交接的业务时间（交接判断时刻），不是本上下文的记录时刻。
func (transition ControlTransition) OccurredAt() time.Time {
	return transition.occurredAt
}

// JudgeTransferOut 依据一版`已交接`判这件作业实物此刻在身的控制怎样转出（ADR-0214 决定四）。active 是这件实物此刻在身的各份控制
// （收寄成立与交接转入成立都算），调用方在取了实物锁之后读出。
//
//   - 恰一份、交接不早于它成立：转出，转出依据是交接给出的引用，时刻取交接的业务时间。
//   - 零份或不止一份：入账，不推进。不止一份时说不出交接结束的是哪一份，不挑一份，也不一并转出。
//   - 交接早于那一份成立：入账，带那一份控制——这次交接说的是更早的一次占有。
func JudgeTransferOut(
	tenant TenantID,
	unit HandlingUnitID,
	active []PhysicalControl,
	handover AuthoritativeHandoverReference,
	reference TransferOutReference,
	at time.Time,
) (ControlTransition, error) {
	if !tenant.valid() || !unit.valid() || !handover.valid() || !reference.valid() || at.IsZero() {
		return ControlTransition{}, ErrInvalidControlTransition
	}
	for _, control := range active {
		if !control.Active() || control.tenantID != tenant || control.unit != unit {
			return ControlTransition{}, ErrInvalidControlTransition
		}
	}
	switch len(active) {
	case 0:
		return notAdvanced(tenant, handover, ControlTransitionOut, NoActiveControl, unit, PhysicalControl{}, at)
	case 1:
	default:
		return notAdvanced(tenant, handover, ControlTransitionOut, SeveralActiveControls, unit, PhysicalControl{}, at)
	}
	control := active[0]
	if at.Before(control.EstablishedAt()) {
		return notAdvanced(tenant, handover, ControlTransitionOut, HandoverPrecedesControl, unit, control, at)
	}
	transferred, err := control.TransferOut(reference, at)
	if err != nil {
		return ControlTransition{}, err
	}
	return ControlTransition{
		tenantID:   tenant,
		handover:   handover,
		direction:  ControlTransitionOut,
		outcome:    ControlAdvanced,
		unit:       unit,
		control:    transferred,
		occurredAt: at.UTC(),
	}, nil
}

// ObjectNotResolved 入账一版对不上作业实物的交接：交接对象没对上、对上不止一件，或对象是集运单元（ADR-0214 决定二）。
func ObjectNotResolved(
	tenant TenantID,
	handover AuthoritativeHandoverReference,
	direction ControlTransitionDirection,
	reason ControlNotAdvancedReason,
	at time.Time,
) (ControlTransition, error) {
	if !reason.unitless() {
		return ControlTransition{}, ErrInvalidControlTransition
	}
	return notAdvanced(tenant, handover, direction, reason, HandlingUnitID{}, PhysicalControl{}, at)
}

// PendingControlSourceRecompute 入账一版撤回控制的更正（ADR-0214 决定五）：earlier 是同一条更正链上先前推进了的那一行转出，
// withdrawing 是把它更正成拒收或待确认的这一版。已转出的控制不回退，这一行带着先前那次转出，留给控制来源链的重算。
func PendingControlSourceRecompute(
	earlier ControlTransition,
	withdrawing AuthoritativeHandoverReference,
	at time.Time,
) (ControlTransition, error) {
	if earlier.direction != ControlTransitionOut || earlier.outcome != ControlAdvanced ||
		!withdrawing.valid() || !earlier.handover.SameChain(withdrawing) || earlier.handover == withdrawing {
		return ControlTransition{}, ErrInvalidControlTransition
	}
	return notAdvanced(earlier.tenantID, withdrawing, ControlTransitionOut, ControlSourceChainPendingRecompute,
		earlier.unit, earlier.control, at)
}

func notAdvanced(
	tenant TenantID,
	handover AuthoritativeHandoverReference,
	direction ControlTransitionDirection,
	reason ControlNotAdvancedReason,
	unit HandlingUnitID,
	control PhysicalControl,
	at time.Time,
) (ControlTransition, error) {
	transition := ControlTransition{
		tenantID:   tenant,
		handover:   handover,
		direction:  direction,
		outcome:    ControlNotAdvanced,
		reason:     reason,
		unit:       unit,
		control:    control,
		occurredAt: at.UTC(),
	}
	if err := transition.validate(); err != nil {
		return ControlTransition{}, err
	}
	return transition, nil
}

// ControlTransitionSpec 是读回一行控制转移所需的全部格。Control 由适配器经 EstablishPhysicalControl（与转出时的 TransferOut）重建。
type ControlTransitionSpec struct {
	TenantID   TenantID
	Handover   AuthoritativeHandoverReference
	Direction  ControlTransitionDirection
	Outcome    ControlTransitionOutcome
	Reason     ControlNotAdvancedReason
	Unit       HandlingUnitID
	Control    PhysicalControl
	OccurredAt time.Time
}

// RehydrateControlTransition 读回一行控制转移，按与各构造函数同一套形状规则验它：一行坏记录在这里响，而不是读成一次合法的转出。
func RehydrateControlTransition(spec ControlTransitionSpec) (ControlTransition, error) {
	transition := ControlTransition{
		tenantID:   spec.TenantID,
		handover:   spec.Handover,
		direction:  spec.Direction,
		outcome:    spec.Outcome,
		reason:     spec.Reason,
		unit:       spec.Unit,
		control:    spec.Control,
		occurredAt: spec.OccurredAt.UTC(),
	}
	if err := transition.validate(); err != nil {
		return ControlTransition{}, err
	}
	return transition, nil
}

// validate 是一行控制转移的形状规则，与迁移 0017 的 CHECK 是同一份，库里再守一遍。
func (transition ControlTransition) validate() error {
	if !transition.tenantID.valid() || !transition.handover.valid() || transition.occurredAt.IsZero() {
		return ErrInvalidControlTransition
	}
	if transition.direction != ControlTransitionOut && transition.direction != ControlTransitionIn {
		return ErrInvalidControlTransition
	}
	control, carries := transition.Control()
	if carries && (!transition.unit.valid() || control.tenantID != transition.tenantID || control.unit != transition.unit) {
		return ErrInvalidControlTransition
	}
	_, releasedAt, released := control.Release()
	switch transition.outcome {
	case ControlAdvanced:
		if transition.reason != ControlNotAdvancedReasonAbsent || !carries {
			return ErrInvalidControlTransition
		}
		if transition.direction == ControlTransitionOut && (!released || !releasedAt.Equal(transition.occurredAt)) {
			return ErrInvalidControlTransition
		}
		if transition.direction == ControlTransitionIn &&
			(released || control.EstablishmentKind() != EstablishedByHandoverIn) {
			return ErrInvalidControlTransition
		}
		return nil
	case ControlNotAdvanced:
		return transition.validateNotAdvanced(carries, released)
	default:
		return ErrInvalidControlTransition
	}
}

func (transition ControlTransition) validateNotAdvanced(carries, released bool) error {
	reason := transition.reason
	switch {
	case reason.unitless():
		if transition.unit.valid() || carries {
			return ErrInvalidControlTransition
		}
	case reason == NoActiveControl, reason == SeveralActiveControls:
		if !transition.unit.valid() || carries {
			return ErrInvalidControlTransition
		}
	case reason == HandoverPrecedesControl:
		if !carries || released || !transition.occurredAt.Before(transition.control.EstablishedAt()) {
			return ErrInvalidControlTransition
		}
	case reason == ControlSourceChainPendingRecompute:
		if transition.direction != ControlTransitionOut || !carries || !released {
			return ErrInvalidControlTransition
		}
	default:
		return ErrInvalidControlTransition
	}
	return nil
}
