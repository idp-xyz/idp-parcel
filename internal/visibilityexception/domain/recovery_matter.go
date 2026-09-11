package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidRecovery       = errors.New("visibility exception: invalid recovery matter")
	ErrInvalidRecoveryAction = errors.New("visibility exception: invalid recovery action")
)

// RecoveryMatterID 是追偿事项的标识。
type RecoveryMatterID struct{ requiredValue }

func NewRecoveryMatterID(value string) (RecoveryMatterID, error) {
	required, err := newRequiredValue("recovery matter ID", value)
	return RecoveryMatterID{required}, err
}

// CounterpartyReference 指名责任相对方（供应商或保险人）。
type CounterpartyReference struct{ requiredValue }

func NewCounterpartyReference(value string) (CounterpartyReference, error) {
	required, err := newRequiredValue("counterparty reference", value)
	return CounterpartyReference{required}, err
}

// LiabilityBasisReference 指名责任依据（协议或保险条款版本）。
type LiabilityBasisReference struct{ requiredValue }

func NewLiabilityBasisReference(value string) (LiabilityBasisReference, error) {
	required, err := newRequiredValue("liability basis reference", value)
	return LiabilityBasisReference{required}, err
}

// LegalEntityReference 指名运营责任法人。
type LegalEntityReference struct{ requiredValue }

func NewLegalEntityReference(value string) (LegalEntityReference, error) {
	required, err := newRequiredValue("legal entity reference", value)
	return LegalEntityReference{required}, err
}

// RecoveryMatterSpec 是建立一项追偿事项所需的全部输入：按责任相对方和责任依据分别
// 固定运营责任法人、协议/条款版本、责任范围、证据范围与适用期限（CONTEXT「按责任相对方和责任依据分别固定」）。
type RecoveryMatterSpec struct {
	ID           RecoveryMatterID
	Case         CaseID
	Counterparty CounterpartyReference
	Basis        LiabilityBasisReference
	LegalEntity  LegalEntityReference
	Scope        RequestScopeReference
	Evidence     RequestEvidenceReference
	Deadline     time.Time
	OpenedAt     time.Time
}

// RecoveryMatter 是供应商或保险追偿事项。它在通知或主张条件成立时独立发起——无需
// 等待客户索赔、客户责任结论或客户赔付（CONTEXT「无需等待客户提出索赔」）：类型上没有任何客户索赔
// 前置字段。一个案件可关联多个追偿事项，各自拥有资格、时限与证据。
type RecoveryMatter struct {
	id           RecoveryMatterID
	caseID       CaseID
	counterparty CounterpartyReference
	basis        LiabilityBasisReference
	legalEntity  LegalEntityReference
	scope        RequestScopeReference
	evidence     RequestEvidenceReference
	deadline     time.Time
	openedAt     time.Time
}

func OpenRecoveryMatter(spec RecoveryMatterSpec) (RecoveryMatter, error) {
	if !spec.ID.valid() ||
		!spec.Case.valid() ||
		!spec.Counterparty.valid() ||
		!spec.Basis.valid() ||
		!spec.LegalEntity.valid() ||
		!spec.Scope.valid() ||
		!spec.Evidence.valid() ||
		spec.Deadline.IsZero() ||
		spec.OpenedAt.IsZero() ||
		!spec.Deadline.After(spec.OpenedAt) {
		return RecoveryMatter{}, ErrInvalidRecovery
	}
	return RecoveryMatter{
		id:           spec.ID,
		caseID:       spec.Case,
		counterparty: spec.Counterparty,
		basis:        spec.Basis,
		legalEntity:  spec.LegalEntity,
		scope:        spec.Scope,
		evidence:     spec.Evidence,
		deadline:     spec.Deadline.UTC(),
		openedAt:     spec.OpenedAt.UTC(),
	}, nil
}

func (matter RecoveryMatter) ID() RecoveryMatterID {
	return matter.id
}

func (matter RecoveryMatter) Case() CaseID {
	return matter.caseID
}

func (matter RecoveryMatter) Counterparty() CounterpartyReference {
	return matter.counterparty
}

func (matter RecoveryMatter) Basis() LiabilityBasisReference {
	return matter.basis
}

func (matter RecoveryMatter) Deadline() time.Time {
	return matter.deadline
}

// Scope 是（案件+相对方+范围）幂等键的第三维——不导出，存储连键都立不起来。
func (matter RecoveryMatter) Scope() RequestScopeReference {
	return matter.scope
}

func (matter RecoveryMatter) LegalEntity() LegalEntityReference {
	return matter.legalEntity
}

func (matter RecoveryMatter) Evidence() RequestEvidenceReference {
	return matter.evidence
}

func (matter RecoveryMatter) OpenedAt() time.Time {
	return matter.openedAt
}

// RecoveryActionKind 是追偿动作的封闭二值：预先通知与正式主张
// CONTEXT「不能合并为一个模糊的“已追偿”」。
type RecoveryActionKind uint8

const (
	RecoveryActionKindInvalid RecoveryActionKind = iota
	PreliminaryNotice
	FormalAssertion
)

func (kind RecoveryActionKind) valid() bool {
	return kind == PreliminaryNotice || kind == FormalAssertion
}

func (kind RecoveryActionKind) String() string {
	switch kind {
	case PreliminaryNotice:
		return "PRELIMINARY_NOTICE"
	case FormalAssertion:
		return "FORMAL_ASSERTION"
	default:
		return ""
	}
}

// RecoveryActionMilestone 是一次动作的过程节点封闭集合（CONTEXT「必须分别记录准备完成、对外提交」那一句：
// 准备完成、对外提交、渠道接受、送达、对方确认、提交失败和送达失败分别记录）。
type RecoveryActionMilestone uint8

const (
	RecoveryActionMilestoneInvalid RecoveryActionMilestone = iota
	ActionPrepared
	ActionSubmitted
	ChannelAccepted
	ActionDelivered
	CounterpartyAcknowledged
	SubmissionFailed
	DeliveryFailed
)

func (milestone RecoveryActionMilestone) valid() bool {
	return milestone >= ActionPrepared && milestone <= DeliveryFailed
}

func (milestone RecoveryActionMilestone) String() string {
	switch milestone {
	case ActionPrepared:
		return "PREPARED"
	case ActionSubmitted:
		return "SUBMITTED"
	case ChannelAccepted:
		return "CHANNEL_ACCEPTED"
	case ActionDelivered:
		return "DELIVERED"
	case CounterpartyAcknowledged:
		return "ACKNOWLEDGED"
	case SubmissionFailed:
		return "SUBMISSION_FAILED"
	case DeliveryFailed:
		return "DELIVERY_FAILED"
	default:
		return ""
	}
}

// RecoveryAction 是一次追偿通知或主张动作的记录：种类、内容版本、过程节点与时间。
// 哪个节点满足期限义务来自适用协议或条款（obligation 引用）——准备完成、渠道接受和
// 内部审批都不能默认满足对外义务；提交或送达失败是外部动作结果，不是对方拒绝责任
// （类型上没有「对方拒绝」字段——对方响应是另一个对象）。
type RecoveryAction struct {
	matter     RecoveryMatterID
	kind       RecoveryActionKind
	contentRef string
	milestone  RecoveryActionMilestone
	obligation LiabilityBasisReference
	occurredAt time.Time
	attempt    int
}

// RecordRecoveryAction 记录一次动作节点。失败节点允许期限内重试（attempt 递增，所有
// 尝试和内容版本保留——重试是新记录不是改写）。
func RecordRecoveryAction(
	matter RecoveryMatterID,
	kind RecoveryActionKind,
	contentRef string,
	milestone RecoveryActionMilestone,
	obligation LiabilityBasisReference,
	occurredAt time.Time,
	attempt int,
) (RecoveryAction, error) {
	if !matter.valid() || !kind.valid() || contentRef == "" ||
		!milestone.valid() || !obligation.valid() ||
		occurredAt.IsZero() || attempt <= 0 {
		return RecoveryAction{}, ErrInvalidRecoveryAction
	}
	return RecoveryAction{
		matter:     matter,
		kind:       kind,
		contentRef: contentRef,
		milestone:  milestone,
		obligation: obligation,
		occurredAt: occurredAt.UTC(),
		attempt:    attempt,
	}, nil
}

func (action RecoveryAction) Matter() RecoveryMatterID {
	return action.matter
}

func (action RecoveryAction) Kind() RecoveryActionKind {
	return action.kind
}

func (action RecoveryAction) Milestone() RecoveryActionMilestone {
	return action.milestone
}

func (action RecoveryAction) ContentRef() string {
	return action.contentRef
}

func (action RecoveryAction) Obligation() LiabilityBasisReference {
	return action.obligation
}

func (action RecoveryAction) Attempt() int {
	return action.attempt
}

func (action RecoveryAction) OccurredAt() time.Time {
	return action.occurredAt
}
