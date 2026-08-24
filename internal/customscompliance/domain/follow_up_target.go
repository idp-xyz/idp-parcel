package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidFollowUpTarget   = errors.New("customs compliance: invalid follow-up declaration target")
	ErrReplacementNotEffective = errors.New("customs compliance: the replacement is not yet effective")
)

// FollowUpActionKind 是后续申报动作的封闭四值（CONTEXT 硬句 172：「同版本技术再次
// 尝试、原案内补充、原案内更正、撤销动作和重报替代必须分别表达」——技术再次尝试走
// SubmissionAttempt 的受控重发，不在这里；其余四道各占一格）。
type FollowUpActionKind uint8

const (
	FollowUpActionKindInvalid FollowUpActionKind = iota
	InCaseSupplement
	InCaseCorrection
	WithdrawalAction
	ResubmissionReplacement
)

func (kind FollowUpActionKind) valid() bool {
	return kind >= InCaseSupplement && kind <= ResubmissionReplacement
}

func (kind FollowUpActionKind) String() string {
	switch kind {
	case InCaseSupplement:
		return "IN_CASE_SUPPLEMENT"
	case InCaseCorrection:
		return "IN_CASE_CORRECTION"
	case WithdrawalAction:
		return "WITHDRAWAL"
	case ResubmissionReplacement:
		return "RESUBMISSION_REPLACEMENT"
	default:
		return ""
	}
}

// FollowUpTriggerReference 指名触发依据（已接受监管要求或当前有效合规判断）。
type FollowUpTriggerReference struct{ requiredValue }

func NewFollowUpTriggerReference(value string) (FollowUpTriggerReference, error) {
	required, err := newRequiredValue("follow-up trigger reference", value)
	return FollowUpTriggerReference{required}, err
}

// FollowUpTargetSpec 是形成一个后续申报动作目标所需的全部输入（CONTEXT「后续申报
// 动作目标」语言：必须关联触发依据、原案件、原申报单元、原提交版本、明确范围和拟
// 提交动作）。
type FollowUpTargetSpec struct {
	Kind     FollowUpActionKind
	Trigger  FollowUpTriggerReference
	CaseRef  CustomsCaseID
	Unit     DeclarationUnitID
	Version  SubmissionVersionID
	Scope    DecisionScopeReference
	FormedAt time.Time
}

// FollowUpTarget 是针对已提交申报的后续动作目标。目标形成不等于资料已准备、已经
// 提交或监管结果已经成立（CONTEXT 语言）——类型上没有资料/提交/结果字段；补充与
// 更正走原案内新资料与新提交版本、撤销是需要自身提交的新监管动作、重报必须建立
// 替代申报单元，这些下游动作各有自己的对象，这里只立目标。
type FollowUpTarget struct {
	kind     FollowUpActionKind
	trigger  FollowUpTriggerReference
	caseRef  CustomsCaseID
	unit     DeclarationUnitID
	version  SubmissionVersionID
	scope    DecisionScopeReference
	formedAt time.Time
}

func FormFollowUpTarget(spec FollowUpTargetSpec) (FollowUpTarget, error) {
	if !spec.Kind.valid() ||
		!spec.Trigger.valid() ||
		!spec.CaseRef.valid() ||
		!spec.Unit.valid() ||
		!spec.Version.valid() ||
		!spec.Scope.valid() ||
		spec.FormedAt.IsZero() {
		return FollowUpTarget{}, ErrInvalidFollowUpTarget
	}
	return FollowUpTarget{
		kind:     spec.Kind,
		trigger:  spec.Trigger,
		caseRef:  spec.CaseRef,
		unit:     spec.Unit,
		version:  spec.Version,
		scope:    spec.Scope,
		formedAt: spec.FormedAt.UTC(),
	}, nil
}

func (target FollowUpTarget) Kind() FollowUpActionKind {
	return target.kind
}

func (target FollowUpTarget) Trigger() FollowUpTriggerReference {
	return target.trigger
}

func (target FollowUpTarget) Unit() DeclarationUnitID {
	return target.unit
}

func (target FollowUpTarget) Version() SubmissionVersionID {
	return target.version
}

func (target FollowUpTarget) CaseRef() CustomsCaseID {
	return target.caseRef
}

func (target FollowUpTarget) Scope() DecisionScopeReference {
	return target.scope
}

func (target FollowUpTarget) FormedAt() time.Time {
	return target.formedAt
}

// ReplacementRelation 是重报替代的新旧对象关系。建立时只能是拟替代（CONTEXT「申报
// 替代关系」语言）；只有真实程序要求的提交及外部结果已经成立，才可形成有效替代——
// 原对象及其全部历史永久保留，拟替代目标不得把原申报改成已撤销、已作废或已被有效
// 替代（175）。
type ReplacementRelation struct {
	target          FollowUpTarget
	replacementUnit DeclarationUnitID
	effective       bool
	externalResult  string
	effectiveAt     time.Time
}

// ProposeReplacement 依据重报替代目标建立拟替代关系。非重报目标建立不了替代关系；
// 替代单元必须是不同于原单元的新身份（原地修改原案件吸收被 174 明禁）。
func ProposeReplacement(
	target FollowUpTarget,
	replacementUnit DeclarationUnitID,
) (ReplacementRelation, error) {
	if target.kind != ResubmissionReplacement {
		return ReplacementRelation{}, ErrInvalidFollowUpTarget
	}
	if !replacementUnit.valid() || replacementUnit == target.unit {
		return ReplacementRelation{}, ErrInvalidFollowUpTarget
	}
	return ReplacementRelation{
		target:          target,
		replacementUnit: replacementUnit,
	}, nil
}

func (relation ReplacementRelation) ReplacementUnit() DeclarationUnitID {
	return relation.replacementUnit
}

func (relation ReplacementRelation) Target() FollowUpTarget {
	return relation.target
}

func (relation ReplacementRelation) EffectiveAt() (time.Time, bool) {
	return relation.effectiveAt, relation.effective
}

// Effective 报告替代是否已生效。拟替代不是有效替代。
func (relation ReplacementRelation) Effective() bool {
	return relation.effective
}

// ExternalResult 只在有效替代上给出成立依据。
func (relation ReplacementRelation) ExternalResult() (string, bool) {
	return relation.externalResult, relation.effective
}

// TakeEffect 依据真实程序要求的权威外部结果把拟替代升为有效替代（CONTEXT 生命周期
// 267：「真实程序要求的撤销、重报及外部结果均已满足→拟替代关系可以形成有效替代
// 关系」）。外部结果必备——内部决定、请求发出或技术成功都不等于替代成立；有效替代
// 不删除原对象（这里只有引用，删无可删）；已生效不再生效第二次。
func (relation ReplacementRelation) TakeEffect(
	externalResult string,
	at time.Time,
) (ReplacementRelation, error) {
	if relation.effective {
		return ReplacementRelation{}, ErrInvalidFollowUpTarget
	}
	if externalResult == "" || at.IsZero() {
		return ReplacementRelation{}, ErrReplacementNotEffective
	}
	effective := relation
	effective.effective = true
	effective.externalResult = externalResult
	effective.effectiveAt = at.UTC()
	return effective, nil
}
