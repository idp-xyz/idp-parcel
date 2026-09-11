package domain

import (
	"errors"
	"sort"
)

var (
	ErrInvalidDutyPaymentGateRule = errors.New("customs compliance: invalid duty payment gate rule")
	ErrDutyPaymentGateUndecided   = errors.New("customs compliance: the duty payment gate cannot be judged from this verification")
)

// 放行门禁里「税费付款」那一道的读法（票 sa-cc/06，ADR-0137 决定三）。CONTEXT 的话：「税费支付是否
// 是放行前置条件，取决于当前监管程序的适用规则；本上下文不得统一假设『先税后放』或『先放后税』」
// ——所以折法不是编排常量，是登记进来的规则；规则的取值（哪个程序下什么差额能放行）属实例半边
// `PAR-CUS-0x`，这里一个都不拟，连「已覆盖 · 无差额 · 有效才满足」也不拟。

// DutyPaymentPrecondition 是「税费付款」那一道前置条件在门禁清单里的引用。它是机制侧的名字（CONTEXT
// 把「税费付款核对」点名为放行门禁绑定的前置条件之一），不是实例参数；登记方给这一道登的是规则而
// 不是认定（ADR-0137 决定三），所以认定登记口对这个引用关门。
var DutyPaymentPrecondition = PreconditionReference{requiredValue{value: "DUTY_PAYMENT"}}

// DutyPaymentGateRule 是规则正文，两形之一：「税费付款不构成本动作在本边界的前置条件」；或三个接受
// 集合——覆盖 ⊆ {无覆盖, 部分覆盖, 已覆盖}、差额 ⊆ {无差额, 不足, 超额}、有效性 ⊆ {有效, 失效}，
// 三态各落在自己的接受集合内才满足，任一不在即未满足。`待确认` / `冲突` 不可登记为接受：待确认是
// 「还没答」，不是一个能被接受的答案（UC-CC-003「未知不能当作可选、不适用、有效或已解除」）。
type DutyPaymentGateRule struct {
	waived   bool
	coverage []DutyCoverage
	delta    []DutyDelta
	validity []DutyFactValidity
}

// DutyPaymentNotAPrecondition 形成「不构成前置条件」那一形：这个程序下付款根本不是放行前置，门禁
// 如实放过这一道——它不读核对、不判三态。
func DutyPaymentNotAPrecondition() DutyPaymentGateRule {
	return DutyPaymentGateRule{waived: true}
}

// AcceptDutyPaymentWhen 形成接受集合那一形。每个集合非空——空集合等于「永不满足」，那是常量不是
// 规则；集外取值拒；`待确认` / `冲突` 拒。集合去重后按枚举序排定，让同一条规则不论怎么写都只有
// 一种形（登记册重放比对靠它）。
func AcceptDutyPaymentWhen(
	coverage []DutyCoverage,
	delta []DutyDelta,
	validity []DutyFactValidity,
) (DutyPaymentGateRule, error) {
	if len(coverage) == 0 || len(delta) == 0 || len(validity) == 0 {
		return DutyPaymentGateRule{}, ErrInvalidDutyPaymentGateRule
	}
	for _, member := range coverage {
		if !member.valid() {
			return DutyPaymentGateRule{}, ErrInvalidDutyPaymentGateRule
		}
	}
	for _, member := range delta {
		if !member.valid() || member == DeltaPending {
			return DutyPaymentGateRule{}, ErrInvalidDutyPaymentGateRule
		}
	}
	for _, member := range validity {
		if !member.valid() || member == FundsFactConflicting || member == FundsFactPending {
			return DutyPaymentGateRule{}, ErrInvalidDutyPaymentGateRule
		}
	}
	return DutyPaymentGateRule{
		coverage: canonicalSet(coverage),
		delta:    canonicalSet(delta),
		validity: canonicalSet(validity),
	}, nil
}

func canonicalSet[T ~uint8](members []T) []T {
	seen := make(map[T]bool, len(members))
	out := make([]T, 0, len(members))
	for _, member := range members {
		if !seen[member] {
			seen[member] = true
			out = append(out, member)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (rule DutyPaymentGateRule) valid() bool {
	return rule.waived || (len(rule.coverage) > 0 && len(rule.delta) > 0 && len(rule.validity) > 0)
}

// NotAPrecondition 报告规则是不是「不构成前置条件」那一形。
func (rule DutyPaymentGateRule) NotAPrecondition() bool {
	return rule.waived
}

// Accepts 交回三个接受集合（副本）；「不构成前置条件」那一形三个都为空。
func (rule DutyPaymentGateRule) Accepts() ([]DutyCoverage, []DutyDelta, []DutyFactValidity) {
	return append([]DutyCoverage(nil), rule.coverage...),
		append([]DutyDelta(nil), rule.delta...),
		append([]DutyFactValidity(nil), rule.validity...)
}

// Judge 拿一版付款核对对着接受集合折出这一道的判断。三态任一为`待确认` / `冲突`即未决
// （ErrDutyPaymentGateUndecided）——不是未满足；「不构成前置条件」那一形没有东西可判，拿它来判是
// 调用方编程错误。读数带三态原值，核对版本引用由调用方从登记册的键补上（领域核对对象不带版本指纹）。
func (rule DutyPaymentGateRule) Judge(verification DutyPaymentVerification) (DutyPaymentGateReading, error) {
	if !rule.valid() || rule.waived {
		return DutyPaymentGateReading{}, ErrInvalidDutyPaymentGateRule
	}
	coverage, delta, validity := verification.Coverage(), verification.Delta(), verification.Validity()
	if !coverage.valid() || !delta.valid() || !validity.valid() {
		return DutyPaymentGateReading{}, ErrInvalidDutyVerification
	}
	if delta == DeltaPending || validity == FundsFactConflicting || validity == FundsFactPending {
		return DutyPaymentGateReading{}, ErrDutyPaymentGateUndecided
	}
	state := PreconditionUnmet
	if contains(rule.coverage, coverage) && contains(rule.delta, delta) && contains(rule.validity, validity) {
		state = PreconditionMet
	}
	return DutyPaymentGateReading{
		State:    state,
		Coverage: coverage,
		Delta:    delta,
		Validity: validity,
		Verification: DutyVerificationReference{
			Duty:  verification.Duty(),
			Funds: verification.Funds(),
		},
	}, nil
}

func contains[T comparable](set []T, member T) bool {
	for _, candidate := range set {
		if candidate == member {
			return true
		}
	}
	return false
}

// DutyVerificationReference 指名门禁读的是哪一版付款核对：核对身份两维（税费引用、资金事实引用）加
// 版本指纹；申报范围与门禁同一个，不重复带。引用不快照（票 sa-cc/06 裁决 2）：三轴原值另在读数上
// 带，是为了「分别表达」不是为了替代回读。
type DutyVerificationReference struct {
	Duty    AssessedDutyReference
	Funds   ExternalFundsFactReference
	Version string
}

func (reference DutyVerificationReference) valid() bool {
	return reference.Duty.valid() && reference.Funds.valid() && reference.Version != ""
}

// DutyPaymentGateReading 是门禁记录上「税费付款」那一道的读数：按规则折出的判断、三态原值、核对版本
// 引用。没有合成布尔——CONTEXT「覆盖状态、差额状态和有效性状态分别表达，不能实现为一组互斥总状态」。
type DutyPaymentGateReading struct {
	State        PreconditionState
	Coverage     DutyCoverage
	Delta        DutyDelta
	Validity     DutyFactValidity
	Verification DutyVerificationReference
}

func (reading DutyPaymentGateReading) valid() bool {
	return (reading.State == PreconditionMet || reading.State == PreconditionUnmet) &&
		reading.Coverage.valid() &&
		reading.Delta.valid() && reading.Delta != DeltaPending &&
		reading.Validity.valid() && reading.Validity != FundsFactConflicting && reading.Validity != FundsFactPending &&
		reading.Verification.valid()
}

// WithDutyPayment 交回挂上税费付款读数的一份门禁判断副本（判断是不可变版本，原值不动）。读数只能挂在
// 清单含「税费付款」那一项的判断上——清单里没有这一道却带读数，说的是两件事；带`待确认` / `冲突`
// 轴的读数进不来：那一格是未决，不入册。
func (gate ReleaseGateVerification) WithDutyPayment(reading DutyPaymentGateReading) (ReleaseGateVerification, error) {
	if !reading.valid() || !contains(gate.preconditions, DutyPaymentPrecondition) {
		return ReleaseGateVerification{}, ErrInvalidGateVerification
	}
	attached := gate
	attached.preconditions = append([]PreconditionReference(nil), gate.preconditions...)
	attached.dutyPayment = reading
	attached.hasDutyPayment = true
	return attached, nil
}

// DutyPayment 交回税费付款那一道的读数；没挂即这版判断形成时那一道不是规则判出来的（不构成前置条件，
// 或是本册加读数之前的旧版）。
func (gate ReleaseGateVerification) DutyPayment() (DutyPaymentGateReading, bool) {
	return gate.dutyPayment, gate.hasDutyPayment
}
