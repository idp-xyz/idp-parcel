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

// SettlementMethod 是预付/账期的封闭二值（SA 自有词汇，与 PC 的解析结果对应但不 import）。
// 刻意没有第三格「客户默认」：未解析出方式的范围没有可执行的控制分支，那是`待判断`不是
// 某种通行做法（ADR-0047，呼应 ADR-0044 的方式是解析输出）。
type SettlementMethod uint8

const (
	SettlementMethodInvalid SettlementMethod = iota
	PrepaidSettlement
	TermsSettlement
)

func (method SettlementMethod) valid() bool {
	return method == PrepaidSettlement || method == TermsSettlement
}

func (method SettlementMethod) String() string {
	switch method {
	case PrepaidSettlement:
		return "PREPAID"
	case TermsSettlement:
		return "TERMS"
	default:
		return ""
	}
}

// AdoptedPolicyReference 指名本次控制实际采用的结算政策。CONTEXT 硬句：每项冻结与信用
// 暴露必须保存实际采用的结算政策、预付/账期方式及其适用范围——没有它，事后无从回答
// 「这笔控制凭什么走的这条路」。
type AdoptedPolicyReference struct{ requiredValue }

func NewAdoptedPolicyReference(value string) (AdoptedPolicyReference, error) {
	required, err := newRequiredValue("adopted policy reference", value)
	return AdoptedPolicyReference{required}, err
}

// PreAcceptanceControlPolicy 是商业侧对「这个范围要不要接受前财务控制、按哪种结算方式」
// 的回答。
//
// `不要求`必须携带依据。CONTEXT 明写不得用一次虚假零金额冻结或默认信用通过冒充无控制，
// 而没有依据的`不要求`正是「默认信用通过」的做法——它让一次未执行的控制看起来像通过了。
// 零金额那条路已经被 `NewFreezeRequest` 在构造期堵死，这里堵的是另一条。
//
// `要求`必须携带方式与采用的政策引用（ADR-0047）：方式决定走资金冻结还是信用暴露，
// 政策引用让控制结果保存得下「实际采用的结算政策」。两种形状经各自的构造函数进来，
// 混搭（要求带不适用依据、不要求带方式）没有入口。
type PreAcceptanceControlPolicy struct {
	requirement   ControlRequirement
	basis         ControlBasisReference
	method        SettlementMethod
	adoptedPolicy AdoptedPolicyReference
}

// NewNoControlPolicy 造「本范围接受前无财务控制」的回答，商业不适用依据必备。
func NewNoControlPolicy(basis ControlBasisReference) (PreAcceptanceControlPolicy, error) {
	if !basis.valid() {
		return PreAcceptanceControlPolicy{}, ErrInvalidControlPolicy
	}
	return PreAcceptanceControlPolicy{requirement: ControlNotRequired, basis: basis}, nil
}

// NewRequiredControlPolicy 造「本范围要求接受前财务控制」的回答，方式与采用政策必备。
func NewRequiredControlPolicy(
	method SettlementMethod,
	adoptedPolicy AdoptedPolicyReference,
) (PreAcceptanceControlPolicy, error) {
	if !method.valid() || !adoptedPolicy.valid() {
		return PreAcceptanceControlPolicy{}, ErrInvalidControlPolicy
	}
	return PreAcceptanceControlPolicy{
		requirement:   ControlRequired,
		method:        method,
		adoptedPolicy: adoptedPolicy,
	}, nil
}

func (policy PreAcceptanceControlPolicy) Requirement() ControlRequirement {
	return policy.requirement
}

func (policy PreAcceptanceControlPolicy) Basis() ControlBasisReference {
	return policy.basis
}

func (policy PreAcceptanceControlPolicy) Method() SettlementMethod {
	return policy.method
}

func (policy PreAcceptanceControlPolicy) AdoptedPolicy() AdoptedPolicyReference {
	return policy.adoptedPolicy
}

// ControlRequired 报告是否应当继续执行控制。零值答否且不带依据，因此拿它去构造一个
// 「无控制」结果会缺依据——这是拦住「端口没答话被当成不要求控制」的办法。
func (policy PreAcceptanceControlPolicy) ControlRequired() bool {
	return policy.requirement == ControlRequired
}
