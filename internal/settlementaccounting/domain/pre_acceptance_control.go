package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidControlPolicy = errors.New("settlement accounting: invalid pre-acceptance control policy")
	ErrInvalidControlAsOf   = errors.New("settlement accounting: invalid control asOf")
)

// TenantID 是运营集团租户。按 ADR-0003 它是最高数据隔离边界，因此余额、冻结与控制结果
// 的读取都必须在签名上带着它，而不是从 context 里补。
type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

// ControlBasisReference 指名「本范围接受前无财务控制」所依据的商业事实。CONTEXT 要求
// 这条不适用依据由 `party-commercial` 提供，本上下文只引用不自造。
type ControlBasisReference struct{ requiredValue }

func NewControlBasisReference(value string) (ControlBasisReference, error) {
	required, err := newRequiredValue("control basis reference", value)
	return ControlBasisReference{required}, err
}

type AsOfSemantic struct{ requiredValue }

func NewAsOfSemantic(value string) (AsOfSemantic, error) {
	required, err := newRequiredValue("asOf semantic", value)
	return AsOfSemantic{required}, err
}

type AsOfStrategyVersion struct{ requiredValue }

func NewAsOfStrategyVersion(value string) (AsOfStrategyVersion, error) {
	required, err := newRequiredValue("asOf strategy version", value)
	return AsOfStrategyVersion{required}, err
}

// ControlAsOf 是选出接受前财务控制策略所依据的业务时点。CONTEXT 要求每项控制结果保存
// 实际采用的 `asOf` 语义和值，所以语义、取值与策略版本三项一同携带——只留一个时刻证明
// 不了它来自哪条策略。它与冻结发生时间是两回事：`asOf` 决定按哪一版策略判断，冻结时间
// 说明资金何时被占用。
type ControlAsOf struct {
	semantic        AsOfSemantic
	at              time.Time
	strategyVersion AsOfStrategyVersion
}

func NewControlAsOf(semantic AsOfSemantic, at time.Time, strategyVersion AsOfStrategyVersion) (ControlAsOf, error) {
	if !semantic.valid() || at.IsZero() || !strategyVersion.valid() {
		return ControlAsOf{}, ErrInvalidControlAsOf
	}
	return ControlAsOf{semantic: semantic, at: at.UTC(), strategyVersion: strategyVersion}, nil
}

func (asOf ControlAsOf) Semantic() AsOfSemantic {
	return asOf.semantic
}

func (asOf ControlAsOf) At() time.Time {
	return asOf.at
}

func (asOf ControlAsOf) StrategyVersion() AsOfStrategyVersion {
	return asOf.strategyVersion
}

func (asOf ControlAsOf) Valid() bool {
	return asOf.semantic.valid() && !asOf.at.IsZero() && asOf.strategyVersion.valid()
}

// ControlRequirement 回答合同对本范围要不要执行接受前财务控制。两个取值都不是控制结果：
// `不要求`说的是这项控制不该做，而`业务限制`说的是做了但余额不够。
type ControlRequirement uint8

const (
	ControlRequirementInvalid ControlRequirement = iota
	ControlRequired
	ControlNotRequired
)

func (requirement ControlRequirement) valid() bool {
	return requirement >= ControlRequired && requirement <= ControlNotRequired
}

func (requirement ControlRequirement) String() string {
	switch requirement {
	case ControlRequired:
		return "REQUIRED"
	case ControlNotRequired:
		return "NOT_REQUIRED"
	default:
		return ""
	}
}

// PreAcceptanceControlPolicy 是商业侧对「这个范围要不要接受前财务控制」的回答。
//
// `不要求`必须携带依据。CONTEXT 明写不得用一次虚假零金额冻结或默认信用通过冒充无控制，
// 而没有依据的`不要求`正是「默认信用通过」的做法——它让一次未执行的控制看起来像通过了。
// 零金额那条路已经被 `NewFreezeRequest` 在构造期堵死，这里堵的是另一条。
type PreAcceptanceControlPolicy struct {
	requirement ControlRequirement
	basis       ControlBasisReference
}

func NewPreAcceptanceControlPolicy(
	requirement ControlRequirement,
	basis ControlBasisReference,
) (PreAcceptanceControlPolicy, error) {
	if !requirement.valid() {
		return PreAcceptanceControlPolicy{}, ErrInvalidControlPolicy
	}
	if requirement == ControlNotRequired && !basis.valid() {
		return PreAcceptanceControlPolicy{}, ErrInvalidControlPolicy
	}
	return PreAcceptanceControlPolicy{requirement: requirement, basis: basis}, nil
}

func (policy PreAcceptanceControlPolicy) Requirement() ControlRequirement {
	return policy.requirement
}

func (policy PreAcceptanceControlPolicy) Basis() ControlBasisReference {
	return policy.basis
}

// ControlRequired 报告是否应当继续执行控制。零值答否且不带依据，因此拿它去构造一个
// 「无控制」结果会缺依据——这是拦住「端口没答话被当成不要求控制」的办法。
func (policy PreAcceptanceControlPolicy) ControlRequired() bool {
	return policy.requirement == ControlRequired
}
