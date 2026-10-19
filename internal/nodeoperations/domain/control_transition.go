package domain

import (
	"errors"
	"strconv"
	"strings"
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

// ControlTransitionDirection 是一行控制转移的方向：交出节点那一侧的转出，或接收节点那一侧的转入。
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
	// ControlSourceChainPendingRecompute：同一条更正链上先前的`已交接`已经推进过控制（转出或转入），这一版把它更正成了拒收或待确认。
	// 控制不回退（TF CONTEXT 禁止「直接把当前控制回退给早期交出方」），转入成立的那一份也不撤，控制来源链留待重算。
	ControlSourceChainPendingRecompute
	// ControlStillActive（只在转入）：对上的那件实物此刻仍有在身的节点控制，是同一版先跑的转出没结束掉的那份。转出照唯一在身的那一份
	// 转、不看交出节点声明，漏登的交出不会落到这里；落到这里的是转出没推进（不止一份在身、交接早于控制成立），或这一版的转出没动
	// 在身控制（同链前版已转出，或这一版的转出行早已在册）而转入此前没立起、其间实物又有了在身控制。不另立一份：两份在身控制说不出
	// 实物在哪个节点，此后的转出也只能停在「不止一份」。
	ControlStillActive
	// HandoverPrecedesRelease（只在转入）：这件实物最近一次控制转出晚于这一版交接的业务时间。这一版说的是更早的一次转入，那段占有在
	// 本上下文看来已经被更晚的交接结束；此刻立起来，就再没有交接来结束它。
	HandoverPrecedesRelease
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
	case ControlStillActive:
		return "CONTROL_STILL_ACTIVE"
	case HandoverPrecedesRelease:
		return "HANDOVER_PRECEDES_RELEASE"
	default:
		return ""
	}
}

func ParseControlNotAdvancedReason(name string) (ControlNotAdvancedReason, error) {
	for _, reason := range []ControlNotAdvancedReason{
		NoAssociatedUnit, SeveralAssociatedUnits, ConsolidationUnitPendingRule,
		NoActiveControl, SeveralActiveControls, HandoverPrecedesControl, ControlSourceChainPendingRecompute,
		ControlStillActive, HandoverPrecedesRelease,
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

// directed 报告这一格只属哪个方向；两个方向都可能落的格交回 ControlTransitionDirectionInvalid。
func (reason ControlNotAdvancedReason) directed() ControlTransitionDirection {
	switch reason {
	case NoActiveControl, SeveralActiveControls, HandoverPrecedesControl:
		return ControlTransitionOut
	case ControlStillActive, HandoverPrecedesRelease:
		return ControlTransitionIn
	default:
		return ControlTransitionDirectionInvalid
	}
}

// handoverInControlBasisRoot 起头交接转入成立的控制依据，与收寄成立的 `NODE-INTAKE/` 并列。
const handoverInControlBasisRoot = "HANDOVER-IN/"

// HandoverInControlBasis 是一版权威运输交接在接收节点立起的那一份控制的依据（ADR-0214 决定七）。
//
// 依据要在同一件实物的各份控制之间唯一：读控制时的折叠与「一份控制至多转出一次」都按（作业实物，控制依据）认。TF 给的转出引用
// `TRANSPORT-HANDOVER/<版本>` 不够——交接版本只在（对象，范围）之内唯一，同一件包裹在两个交接范围下可以各有一个同名版本。所以把交接键
// 三件各写成「字节长度 + 冒号 + 原串」依次拼上：三件都是只拒空白的不透明串，以斜杠直拼不单射，长度前缀才让不同的键铸不出同一个依据。
// 不取摘要：依据会出现在查阅册上，人要读得出它是哪一次交接。
func HandoverInControlBasis(handover AuthoritativeHandoverReference) ControlBasisReference {
	var basis strings.Builder
	basis.WriteString(handoverInControlBasisRoot)
	for index, part := range []string{handover.Object(), handover.Scope(), handover.Version()} {
		if index > 0 {
			basis.WriteByte('/')
		}
		basis.WriteString(strconv.Itoa(len(part)))
		basis.WriteByte(':')
		basis.WriteString(part)
	}
	return ControlBasisReference{requiredValue{value: basis.String()}}
}

// ControlTransition 是节点控制转移册上的一行（ADR-0214 决定三）：一版权威运输交接在一个方向上推进了控制，或入账不推进。
//
// 收寄册是只追加的判断链，控制转移不是判断，所以另立这本册，而不改写收寄行的控制：控制此刻是否在身，要经折叠读口把这本册上的
// 转出折进收寄的控制里去读。
//
// 推进了的转出带被转出的那一份控制（已带转出依据与时刻），推进了的转入带在接收节点立起的那一份；入账的几格按原因带或不带作业实物
// 与控制，形状由各构造函数与 RehydrateControlTransition 守。
type ControlTransition struct {
	tenantID   TenantID
	handover   AuthoritativeHandoverReference
	direction  ControlTransitionDirection
	outcome    ControlTransitionOutcome
	reason     ControlNotAdvancedReason
	unit       HandlingUnitID
	control    PhysicalControl
	occurredAt time.Time
	// mismatchedReleasingNode 是信息格：交接声明的交出节点与被转出那一份控制的节点不符时，记声明的那一个（ADR-0214 决定四末条）。
	// 只在推进了的转出上出现；没声明、或声明的就是那个节点时都不在场。
	mismatchedReleasingNode NodeReference
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

// Control 只在这一行带控制时给出：转出了的那一份、转入立起的那一份、交接早于其成立的那一份在身控制，或待重算的那一次先前推进。
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

// ReleasingNodeMismatch 只在推进了的转出、且交接声明的交出节点不是被转出那一份控制的节点时给出声明的那一个。控制照转：转出看的是
// 此刻在身的那一份，声明不符只说明交出节点的声明与本上下文的记录对不上，可能是声明错了，也可能漏登了一次中间交接。
func (transition ControlTransition) ReleasingNodeMismatch() (NodeReference, bool) {
	return transition.mismatchedReleasingNode, transition.mismatchedReleasingNode.valid()
}

// JudgeTransferOut 依据一版`已交接`判这件作业实物此刻在身的控制怎样转出（ADR-0214 决定四）。active 是这件实物此刻在身的各份控制
// （收寄成立与交接转入成立都算），调用方在取了实物锁之后读出。declaredReleasing 是交接声明的交出节点，零值即没有声明（ADR-0213
// 决定二：缺席是一种答案）。
//
//   - 恰一份、交接不早于它成立：转出，转出依据是交接给出的引用，时刻取交接的业务时间。声明的交出节点不是那一份的节点时照转，
//     记信息格。
//   - 零份或不止一份：入账，不推进。不止一份时说不出交接结束的是哪一份，不挑一份，也不一并转出。
//   - 交接早于那一份成立：入账，带那一份控制——这次交接说的是更早的一次占有。
func JudgeTransferOut(
	tenant TenantID,
	unit HandlingUnitID,
	active []PhysicalControl,
	handover AuthoritativeHandoverReference,
	reference TransferOutReference,
	declaredReleasing NodeReference,
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
	transition := ControlTransition{
		tenantID:   tenant,
		handover:   handover,
		direction:  ControlTransitionOut,
		outcome:    ControlAdvanced,
		unit:       unit,
		control:    transferred,
		occurredAt: at.UTC(),
	}
	if declaredReleasing.valid() && declaredReleasing != control.node {
		transition.mismatchedReleasingNode = declaredReleasing
	}
	return transition, nil
}

// JudgeTransferIn 依据一版`已交接`判能不能在它声明的接收节点为这件作业实物立 HANDOVER_IN 控制（ADR-0214 决定七）。known 是这件实物
// 在本上下文有过的各份控制，在身的与已转出的都算，调用方在取了实物锁之后读出——同一版交接先转出、后转入，known 里已带这一版刚落的
// 转出，节点之间直接交接时交出节点那一份已不在身。
//
//   - 没有在身控制、也没有晚于这一版业务时间的转出：在接收节点立控制，依据是 HandoverInControlBasis，成立时刻取交接的业务时间。
//   - 仍有在身控制：入账，不推进，不另立第二份。
//   - 最近一次转出晚于这一版：入账，不推进——晚到的旧交接立起的控制再没有交接来结束。
//
// 对不上作业实物的那几格不经这一口，走 ObjectNotResolved：待识别实物只能由现场观察建立，转入不造实物。
func JudgeTransferIn(
	tenant TenantID,
	unit HandlingUnitID,
	known []PhysicalControl,
	handover AuthoritativeHandoverReference,
	receiving NodeReference,
	at time.Time,
) (ControlTransition, error) {
	if !tenant.valid() || !unit.valid() || !handover.valid() || !receiving.valid() || at.IsZero() {
		return ControlTransition{}, ErrInvalidControlTransition
	}
	basis := HandoverInControlBasis(handover)
	stillActive := false
	var latestRelease time.Time
	for _, control := range known {
		// 已带这一版依据的控制在册，说明这一版已经转入过：编排的已在册与更正链检查被绕过了，不再立第二份。
		if !controlEstablished(control) || control.tenantID != tenant || control.unit != unit || control.basis == basis {
			return ControlTransition{}, ErrInvalidControlTransition
		}
		if control.Active() {
			stillActive = true
			continue
		}
		if _, releasedAt, _ := control.Release(); releasedAt.After(latestRelease) {
			latestRelease = releasedAt
		}
	}
	if stillActive {
		return notAdvanced(tenant, handover, ControlTransitionIn, ControlStillActive, unit, PhysicalControl{}, at)
	}
	if latestRelease.After(at) {
		return notAdvanced(tenant, handover, ControlTransitionIn, HandoverPrecedesRelease, unit, PhysicalControl{}, at)
	}
	control, err := EstablishPhysicalControl(PhysicalControlSpec{
		TenantID:      tenant,
		Unit:          unit,
		Node:          receiving,
		Kind:          EstablishedByHandoverIn,
		Basis:         basis,
		EstablishedAt: at,
	})
	if err != nil {
		return ControlTransition{}, err
	}
	transition := ControlTransition{
		tenantID:   tenant,
		handover:   handover,
		direction:  ControlTransitionIn,
		outcome:    ControlAdvanced,
		unit:       unit,
		control:    control,
		occurredAt: at.UTC(),
	}
	if err := transition.validate(); err != nil {
		return ControlTransition{}, err
	}
	return transition, nil
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

// PendingControlSourceRecompute 入账一版撤回控制的更正（ADR-0214 决定五；转入一侧同此，决定七落地补记）：earlier 是同一条更正链上
// 先前推进了的那一行，withdrawing 是把它更正成拒收或待确认的这一版，这一行与 earlier 同方向。已转出的控制不回退，转入成立的控制也
// 不撤；这一行带着先前推进的那一份控制（转出带转出依据，转入带立起时的那一份），留给控制来源链的重算。
func PendingControlSourceRecompute(
	earlier ControlTransition,
	withdrawing AuthoritativeHandoverReference,
	at time.Time,
) (ControlTransition, error) {
	if (earlier.direction != ControlTransitionOut && earlier.direction != ControlTransitionIn) || earlier.outcome != ControlAdvanced ||
		!withdrawing.valid() || !earlier.handover.SameChain(withdrawing) || earlier.handover == withdrawing {
		return ControlTransition{}, ErrInvalidControlTransition
	}
	return notAdvanced(earlier.tenantID, withdrawing, earlier.direction, ControlSourceChainPendingRecompute,
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
	TenantID                TenantID
	Handover                AuthoritativeHandoverReference
	Direction               ControlTransitionDirection
	Outcome                 ControlTransitionOutcome
	Reason                  ControlNotAdvancedReason
	Unit                    HandlingUnitID
	Control                 PhysicalControl
	OccurredAt              time.Time
	MismatchedReleasingNode NodeReference
}

// RehydrateControlTransition 读回一行控制转移，按与各构造函数同一套形状规则验它：一行坏记录在这里响，而不是读成一次合法的转出。
func RehydrateControlTransition(spec ControlTransitionSpec) (ControlTransition, error) {
	transition := ControlTransition{
		tenantID:                spec.TenantID,
		handover:                spec.Handover,
		direction:               spec.Direction,
		outcome:                 spec.Outcome,
		reason:                  spec.Reason,
		unit:                    spec.Unit,
		control:                 spec.Control,
		occurredAt:              spec.OccurredAt.UTC(),
		mismatchedReleasingNode: spec.MismatchedReleasingNode,
	}
	if err := transition.validate(); err != nil {
		return ControlTransition{}, err
	}
	return transition, nil
}

// validate 是一行控制转移的形状规则，迁移 0017 与 0018 的 CHECK 在库里逐条再守一遍。控制与本行同租户、同实物一条在库面不另写：一行
// 只有一个租户列与一个实物列，列形本身就守住了。
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
	if mismatched, present := transition.ReleasingNodeMismatch(); present &&
		(transition.direction != ControlTransitionOut || transition.outcome != ControlAdvanced || !carries || mismatched == control.node) {
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
		// 转入立起的那一份成立时刻就是这一版的业务时间：读回时比的「最近一次转出晚不晚于它」，与立的时候比的是同一个时刻。
		if transition.direction == ControlTransitionIn &&
			(released || control.EstablishmentKind() != EstablishedByHandoverIn ||
				!control.EstablishedAt().Equal(transition.occurredAt)) {
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
	if directed := reason.directed(); directed != ControlTransitionDirectionInvalid && directed != transition.direction {
		return ErrInvalidControlTransition
	}
	switch {
	case reason.unitless():
		if transition.unit.valid() || carries {
			return ErrInvalidControlTransition
		}
	case reason == NoActiveControl, reason == SeveralActiveControls, reason == ControlStillActive, reason == HandoverPrecedesRelease:
		if !transition.unit.valid() || carries {
			return ErrInvalidControlTransition
		}
	case reason == HandoverPrecedesControl:
		// 构造时取严格早于（JudgeTransferOut）；这里只拒晚于。控制成立时刻在收寄行上留到纳秒，落转移册截到微秒：纳秒上严格早于的
		// 两个时刻截断之后可能相等，读回门若也取严格，会把一行合法的入账拒成坏行（迁移 0017 的 CHECK 同取不晚于）。
		if !carries || released || transition.occurredAt.After(transition.control.EstablishedAt()) {
			return ErrInvalidControlTransition
		}
	case reason == ControlSourceChainPendingRecompute:
		if !carries {
			return ErrInvalidControlTransition
		}
		// 转出一侧带先前那次转出；转入一侧带立起时的那一份，转入行不记转出——它此后被谁转出，看同一份控制依据的转出行。
		if transition.direction == ControlTransitionOut && !released {
			return ErrInvalidControlTransition
		}
		if transition.direction == ControlTransitionIn && (released || transition.control.EstablishmentKind() != EstablishedByHandoverIn) {
			return ErrInvalidControlTransition
		}
	default:
		return ErrInvalidControlTransition
	}
	return nil
}
