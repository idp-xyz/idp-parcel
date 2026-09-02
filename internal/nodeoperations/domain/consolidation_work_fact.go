package domain

import (
	"errors"
	"strings"
	"time"
)

// ErrInvalidWorkFactSource 是集运作业事实缺来源表达的信号。它与 ErrInvalidConsolidation
// 分格：后者说「这一步在当前三相下做不得」，恢复动作是先开封或先移出；本条说「这次作业
// 报不出自己是谁报的」，恢复动作是把来源身份、执行方、证据与业务时间补齐再报一次。压成
// 一个取值会让现场按三相去找一处并不存在的状态错。
var ErrInvalidWorkFactSource = errors.New("node operations: invalid consolidation work fact source")

// PerformingPartyReference 指名实际执行本次节点作业的岗位、人员或设备来源，即 UC-NO-003
// 结果契约「作业事实已形成」里必须保存的那个`执行方`（CONTEXT 受控开封一条称`执行人`）。
//
// 它与 DeliveringPartyReference 分立而不是复用：那个是把实物交进节点来的一方，本条是节点
// 这边动手的一方。同一次集运里两者可以都在场且不是同一个人，合成一个词就再也分不开。
type PerformingPartyReference struct{ requiredValue }

func NewPerformingPartyReference(value string) (PerformingPartyReference, error) {
	required, err := newRequiredValue("performing party reference", value)
	return PerformingPartyReference{required}, err
}

// ConsolidationActionKind 是集运作业事实的封闭六值，与集运口一一对应。它进来源事实登记，
// 让「同一来源身份报的是哪件事」在登记面上读得出来——只记来源身份与单元的话，开启与关闭
// 在登记里长得一模一样。
type ConsolidationActionKind uint8

const (
	ConsolidationActionKindInvalid ConsolidationActionKind = iota
	OpenUnitAction
	AddMemberAction
	RemoveMemberAction
	SealUnitAction
	UnsealUnitAction
	CloseUnitAction
)

func (kind ConsolidationActionKind) valid() bool {
	return kind >= OpenUnitAction && kind <= CloseUnitAction
}

func (kind ConsolidationActionKind) String() string {
	switch kind {
	case OpenUnitAction:
		return "OPEN_UNIT"
	case AddMemberAction:
		return "ADD_MEMBER"
	case RemoveMemberAction:
		return "REMOVE_MEMBER"
	case SealUnitAction:
		return "SEAL_UNIT"
	case UnsealUnitAction:
		return "UNSEAL_UNIT"
	case CloseUnitAction:
		return "CLOSE_UNIT"
	default:
		return ""
	}
}

// WorkFactSource 是一次集运作业事实自带的来源那一层（ADR-0005 的`来源事实`）：谁做的、
// 依据哪条来源身份报进来、证据是什么、业务上什么时候发生。集运口此前直接改派生状态，
// 这一层整个缺席。
//
// 四格都必备，因为 UC-NO-003 结果契约把`执行方、来源和证据`与`发生时间`一并列进「必须
// 保存」；其中来源身份还兼幂等键，缺它连 AT-NO-043 的重复与冲突都判不了。
//
// occurredAt 是**业务发生时间**，由现场事实自带（ADR-0023：作业事实的身份与时间由设备
// 铸造）。它不是记录时刻——记录时刻属服务端，另由登记行的 RecordedAt 承担，两者不可互相
// 顶替：拿处理时的系统时钟当封装时间，正是那条 ADR 禁止的服务端代铸。
//
// 证据复用 ExecutionEvidenceReference 而不另起一个词：那个类型的定义就是「一次执行事实的
// 证据」，不带关务限定，而封装与开封本就在 CONTEXT 点名的执行事实之列。ReceptionEvidence-
// Reference 之所以独立，是因为它另担着「明确接收与到站扫描的分界」这条不变量；本处没有
// 对应的分界要守，再造一个词只会让同一个概念有两个名字。
type WorkFactSource struct {
	sourceID    string
	performedBy PerformingPartyReference
	evidence    ExecutionEvidenceReference
	occurredAt  time.Time
}

// NewWorkFactSource 收下一次作业的来源表达。四格缺一即拒——不给任何一格兜底默认值，
// 尤其不拿系统时钟顶替业务时间。
func NewWorkFactSource(
	sourceID string,
	performedBy PerformingPartyReference,
	evidence ExecutionEvidenceReference,
	occurredAt time.Time,
) (WorkFactSource, error) {
	if strings.TrimSpace(sourceID) == "" ||
		!performedBy.valid() ||
		!evidence.valid() ||
		occurredAt.IsZero() {
		return WorkFactSource{}, ErrInvalidWorkFactSource
	}
	return WorkFactSource{
		sourceID:    sourceID,
		performedBy: performedBy,
		evidence:    evidence,
		occurredAt:  occurredAt.UTC(),
	}, nil
}

// SourceID 是来源身份：同一来源身份重复报同一内容返回原结果，报不同内容形成冲突。
func (source WorkFactSource) SourceID() string {
	return source.sourceID
}

func (source WorkFactSource) PerformedBy() PerformingPartyReference {
	return source.performedBy
}

func (source WorkFactSource) Evidence() ExecutionEvidenceReference {
	return source.evidence
}

func (source WorkFactSource) OccurredAt() time.Time {
	return source.occurredAt
}

func (source WorkFactSource) valid() bool {
	return strings.TrimSpace(source.sourceID) != "" &&
		source.performedBy.valid() &&
		source.evidence.valid() &&
		!source.occurredAt.IsZero()
}
