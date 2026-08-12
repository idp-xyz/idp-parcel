// Package domain 承载关务合规的领域模型：已接收监管决定、执行协作与处置执行核对。
// 监管机构拥有最终监管决定权——这里保存、解释并核对已接收决定的明确适用范围，不以
// 内部规则或派生状态代替监管机构决定；物理执行事实属 node-operations 与
// transport-fulfillment，这里只引用。
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBlankValue                = errors.New("customs compliance: blank value")
	ErrInvalidRegulatoryDecision = errors.New("customs compliance: invalid regulatory decision")
	ErrInvalidExecutionFact      = errors.New("customs compliance: invalid execution fact")
	ErrInvalidVerificationInput  = errors.New("customs compliance: invalid verification input")
)

type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

func (value requiredValue) valid() bool {
	return strings.TrimSpace(value.value) != ""
}

type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

// RegulatoryDecisionID 是已接收监管决定的唯一关联身份（CONTEXT 硬句：必须可唯一
// 关联）。
type RegulatoryDecisionID struct{ requiredValue }

func NewRegulatoryDecisionID(value string) (RegulatoryDecisionID, error) {
	required, err := newRequiredValue("regulatory decision ID", value)
	return RegulatoryDecisionID{required}, err
}

// RegulatoryAuthorityReference 指名作出决定的监管来源。
type RegulatoryAuthorityReference struct{ requiredValue }

func NewRegulatoryAuthorityReference(value string) (RegulatoryAuthorityReference, error) {
	required, err := newRequiredValue("regulatory authority reference", value)
	return RegulatoryAuthorityReference{required}, err
}

// LegalActionReference 指名决定的法律动作语义（销毁、退运、扣留、放行……）。它是
// 开放引用——分类必须依据来源结果层和真实法律语义，不能依据「隔离」「移交」等动作词
// 选择类别（CONTEXT 硬句），所以这里不设本地枚举。
type LegalActionReference struct{ requiredValue }

func NewLegalActionReference(value string) (LegalActionReference, error) {
	required, err := newRequiredValue("legal action reference", value)
	return LegalActionReference{required}, err
}

// DecisionScopeReference 指名决定明确覆盖的对象或范围。查验、退回、扣留、放行或
// 处置只作用于监管决定明确覆盖的范围，不因合报关系自动扩大。
type DecisionScopeReference struct{ requiredValue }

func NewDecisionScopeReference(value string) (DecisionScopeReference, error) {
	required, err := newRequiredValue("decision scope reference", value)
	return DecisionScopeReference{required}, err
}

// RequiredQuantity 是来源实际提供的数量核对维度。Provided 为 false 即「来源未提供」
// ——必须明确记录，不能猜测补齐（CONTEXT 硬句）；未提供时数量不是核对维度。
type RequiredQuantity struct {
	Provided bool
	Units    int
}

// RegulatoryDecisionSpec 是一份已接收监管决定引用所需的全部输入。
type RegulatoryDecisionSpec struct {
	ID         RegulatoryDecisionID
	Authority  RegulatoryAuthorityReference
	Action     LegalActionReference
	Scope      DecisionScopeReference
	Quantity   RequiredQuantity
	ReceivedAt time.Time
}

// RegulatoryDecision 是对已接收监管决定的只读引用。决定身份、来源、法律动作语义与
// 明确适用范围缺一不可唯一关联；数量只在来源实际提供时在场。
type RegulatoryDecision struct {
	id         RegulatoryDecisionID
	authority  RegulatoryAuthorityReference
	action     LegalActionReference
	scope      DecisionScopeReference
	quantity   RequiredQuantity
	receivedAt time.Time
}

func NewRegulatoryDecision(spec RegulatoryDecisionSpec) (RegulatoryDecision, error) {
	if !spec.ID.valid() ||
		!spec.Authority.valid() ||
		!spec.Action.valid() ||
		!spec.Scope.valid() ||
		spec.ReceivedAt.IsZero() {
		return RegulatoryDecision{}, ErrInvalidRegulatoryDecision
	}
	if spec.Quantity.Provided && spec.Quantity.Units <= 0 {
		return RegulatoryDecision{}, ErrInvalidRegulatoryDecision
	}
	return RegulatoryDecision{
		id:         spec.ID,
		authority:  spec.Authority,
		action:     spec.Action,
		scope:      spec.Scope,
		quantity:   spec.Quantity,
		receivedAt: spec.ReceivedAt.UTC(),
	}, nil
}

func (decision RegulatoryDecision) ID() RegulatoryDecisionID {
	return decision.id
}

func (decision RegulatoryDecision) Authority() RegulatoryAuthorityReference {
	return decision.authority
}

func (decision RegulatoryDecision) Action() LegalActionReference {
	return decision.action
}

func (decision RegulatoryDecision) Scope() DecisionScopeReference {
	return decision.scope
}

func (decision RegulatoryDecision) Quantity() RequiredQuantity {
	return decision.quantity
}

func (decision RegulatoryDecision) ReceivedAt() time.Time {
	return decision.receivedAt
}

// ExecutorReference 指名实际执行方（节点或运输履约范围）。
type ExecutorReference struct{ requiredValue }

func NewExecutorReference(value string) (ExecutorReference, error) {
	required, err := newRequiredValue("executor reference", value)
	return ExecutorReference{required}, err
}

// ExecutionFactReference 指名执行方那份物理执行事实本体。
type ExecutionFactReference struct{ requiredValue }

func NewExecutionFactReference(value string) (ExecutionFactReference, error) {
	required, err := newRequiredValue("execution fact reference", value)
	return ExecutionFactReference{required}, err
}

// ExecutionFactSpec 是一份执行事实引用所需的全部输入。
type ExecutionFactSpec struct {
	Executor   ExecutorReference
	Fact       ExecutionFactReference
	Scope      DecisionScopeReference
	Units      int
	OccurredAt time.Time
}

// ExecutionFact 是对执行方实际物理执行事实的只读引用（部分执行与再次执行各是一份）。
// 实际执行由执行事实证明，不能由决定本身推导（CONTEXT 硬句）——所以核对的输入是它，
// 不是决定的另一半。
type ExecutionFact struct {
	executor   ExecutorReference
	fact       ExecutionFactReference
	scope      DecisionScopeReference
	units      int
	occurredAt time.Time
}

func NewExecutionFact(spec ExecutionFactSpec) (ExecutionFact, error) {
	if !spec.Executor.valid() ||
		!spec.Fact.valid() ||
		!spec.Scope.valid() ||
		spec.Units <= 0 ||
		spec.OccurredAt.IsZero() {
		return ExecutionFact{}, ErrInvalidExecutionFact
	}
	return ExecutionFact{
		executor:   spec.Executor,
		fact:       spec.Fact,
		scope:      spec.Scope,
		units:      spec.Units,
		occurredAt: spec.OccurredAt.UTC(),
	}, nil
}

func (fact ExecutionFact) Executor() ExecutorReference {
	return fact.executor
}

func (fact ExecutionFact) Fact() ExecutionFactReference {
	return fact.fact
}

func (fact ExecutionFact) Scope() DecisionScopeReference {
	return fact.scope
}

func (fact ExecutionFact) Units() int {
	return fact.units
}

func (fact ExecutionFact) OccurredAt() time.Time {
	return fact.occurredAt
}

// VerificationConclusion 是处置执行核对的封闭五值（CONTEXT 语言逐词）。
type VerificationConclusion uint8

const (
	VerificationConclusionInvalid VerificationConclusion = iota
	ExecutionCovered
	ExecutionPartiallyCovered
	ExecutionDeviation
	ExecutionFactConflict
	ExecutionEvidenceInsufficient
)

func (conclusion VerificationConclusion) String() string {
	switch conclusion {
	case ExecutionCovered:
		return "COVERED"
	case ExecutionPartiallyCovered:
		return "PARTIALLY_COVERED"
	case ExecutionDeviation:
		return "DEVIATION"
	case ExecutionFactConflict:
		return "FACT_CONFLICT"
	case ExecutionEvidenceInsufficient:
		return "EVIDENCE_INSUFFICIENT"
	default:
		return ""
	}
}

// DispositionVerification 是一次版本化的处置执行核对判断。类型上没有任何「义务终结」
// 或「案件关闭」字段——完成本次核对不自动终结监管义务，也不代替监管机构改变决定
// （CONTEXT 语言），义务是否终结依据真实程序另行判断。
type DispositionVerification struct {
	decision   RegulatoryDecision
	facts      []ExecutionFact
	conclusion VerificationConclusion
	executed   int
	verifiedAt time.Time
}

// VerifyDispositionExecution 把执行事实与决定及其核对维度逐范围比较（UC-CC 处置
// 核对的机制半边）：
//
//   - 无执行事实 → 证据不足（决定本身推导不出执行，AT-PS-091 在 PS 侧引用的正是
//     这半边）。
//   - 事实与决定范围不符 → 事实冲突（拿别的范围的执行凑数分不出真假）。
//   - 数量由来源提供时按合计比较：等于 → 已覆盖；少于 → 部分覆盖；多于 → 差异
//     （执行超出决定明确覆盖的范围同样是差异，不是「多多益善」）。
//   - 来源未提供数量时数量不是核对维度：有范围相符的执行事实即已覆盖。
func VerifyDispositionExecution(
	decision RegulatoryDecision,
	facts []ExecutionFact,
	verifiedAt time.Time,
) (DispositionVerification, error) {
	if !decision.id.valid() || verifiedAt.IsZero() {
		return DispositionVerification{}, ErrInvalidVerificationInput
	}
	verification := DispositionVerification{
		decision:   decision,
		facts:      append([]ExecutionFact(nil), facts...),
		verifiedAt: verifiedAt.UTC(),
	}
	if len(facts) == 0 {
		verification.conclusion = ExecutionEvidenceInsufficient
		return verification, nil
	}

	executed := 0
	for _, fact := range facts {
		if !fact.fact.valid() {
			return DispositionVerification{}, ErrInvalidVerificationInput
		}
		if fact.scope != decision.scope {
			verification.conclusion = ExecutionFactConflict
			return verification, nil
		}
		executed += fact.units
	}
	verification.executed = executed

	if !decision.quantity.Provided {
		verification.conclusion = ExecutionCovered
		return verification, nil
	}
	switch {
	case executed == decision.quantity.Units:
		verification.conclusion = ExecutionCovered
	case executed < decision.quantity.Units:
		verification.conclusion = ExecutionPartiallyCovered
	default:
		verification.conclusion = ExecutionDeviation
	}
	return verification, nil
}

func (verification DispositionVerification) Decision() RegulatoryDecision {
	return verification.decision
}

func (verification DispositionVerification) Facts() []ExecutionFact {
	return append([]ExecutionFact(nil), verification.facts...)
}

func (verification DispositionVerification) Conclusion() VerificationConclusion {
	return verification.conclusion
}

// ExecutedUnits 给出范围相符执行事实的合计数量。
func (verification DispositionVerification) ExecutedUnits() int {
	return verification.executed
}

func (verification DispositionVerification) VerifiedAt() time.Time {
	return verification.verifiedAt
}
