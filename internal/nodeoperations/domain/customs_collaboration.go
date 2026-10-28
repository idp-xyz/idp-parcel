package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCollaborationAcceptance = errors.New("node operations: invalid collaboration acceptance")
	// ErrCollaborationRefused：拒接的事项没有可执行的东西——先重新承接再执行。
	ErrCollaborationRefused = errors.New("node operations: the collaboration was refused")
	// ErrOutsideAcceptedScope：执行只在承接的对象与动作范围内成立——越权执行在构造期
	// 就被拒，与形状错误分格（恢复动作是扩承接范围，不是补字段）。
	ErrOutsideAcceptedScope = errors.New("node operations: execution is outside the accepted scope")
	ErrInvalidExecutionFact = errors.New("node operations: invalid execution fact")
	// ErrCollaborationPendingSupplement：待补充的事项没有承接范围，也就没有可执行的东西——
	// 恢复动作是补齐缺口后形成新决定，与拒接（重新承接）和越权（扩范围）分格。
	ErrCollaborationPendingSupplement = errors.New("node operations: the collaboration is pending supplement")
)

// CollaborationItemReference 指名 customs-compliance 拥有的关务执行协作事项。协作
// 事项不等于节点任务——事项表达监管要求什么，承接与执行是节点这边的决定和事实。
type CollaborationItemReference struct{ requiredValue }

func NewCollaborationItemReference(value string) (CollaborationItemReference, error) {
	required, err := newRequiredValue("collaboration item reference", value)
	return CollaborationItemReference{required}, err
}

// CollaborationActionKind 是节点侧协作执行动作的封闭五值：开封、隔离、呈验、清点、
// 观察。放行、查验结论、处置决定不在其中——那些是监管的话，节点说不了。
type CollaborationActionKind uint8

const (
	CollaborationActionKindInvalid CollaborationActionKind = iota
	UnsealAction
	IsolateAction
	PresentAction
	TallyAction
	ObserveAction
)

func (kind CollaborationActionKind) valid() bool {
	return kind >= UnsealAction && kind <= ObserveAction
}

func (kind CollaborationActionKind) String() string {
	switch kind {
	case UnsealAction:
		return "UNSEAL"
	case IsolateAction:
		return "ISOLATE"
	case PresentAction:
		return "PRESENT"
	case TallyAction:
		return "TALLY"
	case ObserveAction:
		return "OBSERVE"
	default:
		return ""
	}
}

// AcceptanceDecisionKind 是承接决定的封闭四值：接受、拒接、部分承接、待补充。
type AcceptanceDecisionKind uint8

const (
	AcceptanceDecisionKindInvalid AcceptanceDecisionKind = iota
	CollaborationAccepted
	CollaborationDeclined
	CollaborationPartiallyAccepted
	// CollaborationPendingSupplement 是请求合法、但对象、动作、节点、授权依据或必要作业条件
	// 不足以承接的决定：不形成承接范围，带缺口清单。
	CollaborationPendingSupplement
)

func (kind AcceptanceDecisionKind) valid() bool {
	return kind >= CollaborationAccepted && kind <= CollaborationPendingSupplement
}

func (kind AcceptanceDecisionKind) String() string {
	switch kind {
	case CollaborationAccepted:
		return "ACCEPTED"
	case CollaborationDeclined:
		return "DECLINED"
	case CollaborationPartiallyAccepted:
		return "PARTIALLY_ACCEPTED"
	case CollaborationPendingSupplement:
		return "PENDING_SUPPLEMENT"
	default:
		return ""
	}
}

// SupplementGapKind 是待补充缺口的封闭五值，逐一对应 CONTEXT 规则列出的不足之处：
// 对象、动作、节点、授权依据、必要作业条件。必要作业条件的具体取值归租户节点登记，
// 这里只指名「缺哪一类」，不判取值。
type SupplementGapKind uint8

const (
	SupplementGapKindInvalid SupplementGapKind = iota
	MissingObjectGap
	MissingActionGap
	MissingNodeGap
	MissingAuthorityGap
	MissingWorkConditionGap
)

func (kind SupplementGapKind) valid() bool {
	return kind >= MissingObjectGap && kind <= MissingWorkConditionGap
}

func (kind SupplementGapKind) String() string {
	switch kind {
	case MissingObjectGap:
		return "OBJECT"
	case MissingActionGap:
		return "ACTION"
	case MissingNodeGap:
		return "NODE"
	case MissingAuthorityGap:
		return "AUTHORITY"
	case MissingWorkConditionGap:
		return "WORK_CONDITION"
	default:
		return ""
	}
}

// AcceptanceAuthorityReference 指名节点作出承接决定的授权与能力依据。
type AcceptanceAuthorityReference struct{ requiredValue }

func NewAcceptanceAuthorityReference(value string) (AcceptanceAuthorityReference, error) {
	required, err := newRequiredValue("acceptance authority reference", value)
	return AcceptanceAuthorityReference{required}, err
}

// AcceptanceBasisReference 指名拒接原因或部分承接中未承接范围的原因。
type AcceptanceBasisReference struct{ requiredValue }

func NewAcceptanceBasisReference(value string) (AcceptanceBasisReference, error) {
	required, err := newRequiredValue("acceptance basis reference", value)
	return AcceptanceBasisReference{required}, err
}

// CollaborationAcceptanceSpec 是形成一次承接决定所需的全部输入。
type CollaborationAcceptanceSpec struct {
	TenantID        TenantID
	Node            NodeReference
	Item            CollaborationItemReference
	Decision        AcceptanceDecisionKind
	AcceptedUnits   []HandlingUnitID
	AcceptedActions []CollaborationActionKind
	Authority       AcceptanceAuthorityReference
	Basis           AcceptanceBasisReference
	// Gaps 只在待补充时给，且至少一项。
	Gaps      []SupplementGapKind
	DecidedAt time.Time
}

// CollaborationAcceptance 是节点对协作事项作出的自己的决定（UC-NO-001）：接受、拒接
// 带因、部分承接或待补充（带缺口、不形成承接）。承接只确认节点将在明确对象与动作范围内执行——它不等于执行完成，
// 类型上没有任何已执行或完成字段；执行事实逐件另行形成。
type CollaborationAcceptance struct {
	tenantID        TenantID
	node            NodeReference
	item            CollaborationItemReference
	decision        AcceptanceDecisionKind
	acceptedUnits   []HandlingUnitID
	acceptedActions []CollaborationActionKind
	authority       AcceptanceAuthorityReference
	basis           AcceptanceBasisReference
	gaps            []SupplementGapKind
	decidedAt       time.Time
}

// DecideCollaborationAcceptance 逐格校验四值各自的完备性：
//   - `接受`/`部分承接`必须给出非空的对象与动作范围——没有范围的承接执行时无从守界；
//   - `拒接`必须带因且不携带任何承接范围——带了范围的拒接与部分承接混格；
//   - `部分承接`必须带因——未承接的那部分为什么不接，事项所有方（CC）要看得见；
//   - `待补充`必须给出至少一项缺口且不携带任何承接范围——待补充不形成承接，带了范围就
//     成了没有名分的部分承接；其余三值不得带缺口。
func DecideCollaborationAcceptance(spec CollaborationAcceptanceSpec) (CollaborationAcceptance, error) {
	if !spec.TenantID.valid() ||
		!spec.Node.valid() ||
		!spec.Item.valid() ||
		!spec.Decision.valid() ||
		!spec.Authority.valid() ||
		spec.DecidedAt.IsZero() {
		return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
	}
	switch spec.Decision {
	case CollaborationDeclined:
		if len(spec.AcceptedUnits) != 0 || len(spec.AcceptedActions) != 0 {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
		if !spec.Basis.valid() {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
	case CollaborationPendingSupplement:
		if len(spec.AcceptedUnits) != 0 || len(spec.AcceptedActions) != 0 || len(spec.Gaps) == 0 {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
	case CollaborationPartiallyAccepted:
		if !spec.Basis.valid() {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
		fallthrough
	case CollaborationAccepted:
		if len(spec.AcceptedUnits) == 0 || len(spec.AcceptedActions) == 0 {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
	}
	if spec.Decision != CollaborationPendingSupplement && len(spec.Gaps) != 0 {
		return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
	}
	seenGaps := make(map[SupplementGapKind]struct{}, len(spec.Gaps))
	for _, gap := range spec.Gaps {
		if !gap.valid() {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
		if _, exists := seenGaps[gap]; exists {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
		seenGaps[gap] = struct{}{}
	}
	seenUnits := make(map[HandlingUnitID]struct{}, len(spec.AcceptedUnits))
	for _, unit := range spec.AcceptedUnits {
		if !unit.valid() {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
		if _, exists := seenUnits[unit]; exists {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
		seenUnits[unit] = struct{}{}
	}
	seenActions := make(map[CollaborationActionKind]struct{}, len(spec.AcceptedActions))
	for _, action := range spec.AcceptedActions {
		if !action.valid() {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
		if _, exists := seenActions[action]; exists {
			return CollaborationAcceptance{}, ErrInvalidCollaborationAcceptance
		}
		seenActions[action] = struct{}{}
	}
	return CollaborationAcceptance{
		tenantID:        spec.TenantID,
		node:            spec.Node,
		item:            spec.Item,
		decision:        spec.Decision,
		acceptedUnits:   append([]HandlingUnitID(nil), spec.AcceptedUnits...),
		acceptedActions: append([]CollaborationActionKind(nil), spec.AcceptedActions...),
		authority:       spec.Authority,
		basis:           spec.Basis,
		gaps:            append([]SupplementGapKind(nil), spec.Gaps...),
		decidedAt:       spec.DecidedAt.UTC(),
	}, nil
}

func (acceptance CollaborationAcceptance) TenantID() TenantID {
	return acceptance.tenantID
}

func (acceptance CollaborationAcceptance) Node() NodeReference {
	return acceptance.node
}

func (acceptance CollaborationAcceptance) Item() CollaborationItemReference {
	return acceptance.item
}

func (acceptance CollaborationAcceptance) Decision() AcceptanceDecisionKind {
	return acceptance.decision
}

func (acceptance CollaborationAcceptance) AcceptedUnits() []HandlingUnitID {
	return append([]HandlingUnitID(nil), acceptance.acceptedUnits...)
}

func (acceptance CollaborationAcceptance) AcceptedActions() []CollaborationActionKind {
	return append([]CollaborationActionKind(nil), acceptance.acceptedActions...)
}

func (acceptance CollaborationAcceptance) Authority() AcceptanceAuthorityReference {
	return acceptance.authority
}

// Gaps 交回待补充的缺口清单；其余三值为空。
func (acceptance CollaborationAcceptance) Gaps() []SupplementGapKind {
	return append([]SupplementGapKind(nil), acceptance.gaps...)
}

// Basis 在拒接与部分承接时交回原因（待补充可选）；全量接受没有它。
func (acceptance CollaborationAcceptance) Basis() (AcceptanceBasisReference, bool) {
	if !acceptance.basis.valid() {
		return AcceptanceBasisReference{}, false
	}
	return acceptance.basis, true
}

func (acceptance CollaborationAcceptance) DecidedAt() time.Time {
	return acceptance.decidedAt
}

// CoversUnit 与 CoversAction 是执行守界的问口。
func (acceptance CollaborationAcceptance) CoversUnit(unit HandlingUnitID) bool {
	for _, covered := range acceptance.acceptedUnits {
		if covered == unit {
			return true
		}
	}
	return false
}

func (acceptance CollaborationAcceptance) CoversAction(action CollaborationActionKind) bool {
	for _, covered := range acceptance.acceptedActions {
		if covered == action {
			return true
		}
	}
	return false
}

// ExecutionEvidenceReference 指名一次执行事实的证据。
type ExecutionEvidenceReference struct{ requiredValue }

func NewExecutionEvidenceReference(value string) (ExecutionEvidenceReference, error) {
	required, err := newRequiredValue("execution evidence reference", value)
	return ExecutionEvidenceReference{required}, err
}

// NodeExecutionFact 是节点在承接范围内对单件实物实际执行动作留下的事实（开封、隔离、
// 呈验、清点、观察）。逐件形成：一次协作覆盖多件时每件各有事实。
//
// 它不冒充监管查验结果（CC CONTEXT 177）：查验结论、放行与处置决定归监管与
// customs-compliance，本类型上没有任何结论或放行字段可以承载它们——节点只能说
// 「做了什么」，说不了「监管认定了什么」。
type NodeExecutionFact struct {
	tenantID    TenantID
	node        NodeReference
	item        CollaborationItemReference
	unit        HandlingUnitID
	action      CollaborationActionKind
	evidence    ExecutionEvidenceReference
	performedAt time.Time
}

// RecordExecutionFact 依附承接决定记录一次执行。拒接的事项没有可执行的东西；对象或
// 动作越出承接范围即拒（授权范围内执行）；证据必备；执行不早于承接。
func RecordExecutionFact(
	acceptance CollaborationAcceptance,
	unit HandlingUnitID,
	action CollaborationActionKind,
	evidence ExecutionEvidenceReference,
	performedAt time.Time,
) (NodeExecutionFact, error) {
	if !acceptance.item.valid() || !unit.valid() || !action.valid() ||
		performedAt.IsZero() {
		return NodeExecutionFact{}, ErrInvalidExecutionFact
	}
	if acceptance.decision == CollaborationDeclined {
		return NodeExecutionFact{}, ErrCollaborationRefused
	}
	if acceptance.decision == CollaborationPendingSupplement {
		return NodeExecutionFact{}, ErrCollaborationPendingSupplement
	}
	if !acceptance.CoversUnit(unit) || !acceptance.CoversAction(action) {
		return NodeExecutionFact{}, ErrOutsideAcceptedScope
	}
	if !evidence.valid() {
		return NodeExecutionFact{}, ErrInvalidExecutionFact
	}
	if performedAt.Before(acceptance.decidedAt) {
		return NodeExecutionFact{}, ErrInvalidExecutionFact
	}
	return NodeExecutionFact{
		tenantID:    acceptance.tenantID,
		node:        acceptance.node,
		item:        acceptance.item,
		unit:        unit,
		action:      action,
		evidence:    evidence,
		performedAt: performedAt.UTC(),
	}, nil
}

func (fact NodeExecutionFact) TenantID() TenantID {
	return fact.tenantID
}

func (fact NodeExecutionFact) Node() NodeReference {
	return fact.node
}

func (fact NodeExecutionFact) Item() CollaborationItemReference {
	return fact.item
}

func (fact NodeExecutionFact) Unit() HandlingUnitID {
	return fact.unit
}

func (fact NodeExecutionFact) Action() CollaborationActionKind {
	return fact.action
}

func (fact NodeExecutionFact) Evidence() ExecutionEvidenceReference {
	return fact.evidence
}

func (fact NodeExecutionFact) PerformedAt() time.Time {
	return fact.performedAt
}
