package domain

import "errors"

var ErrInvalidNetworkEligibility = errors.New("network routing: invalid network eligibility")

// EligibilityBasisReference 指名判定「本服务不要求运营企业形成网络可达性判断」所依据的
// 商业事实。它是引用而非自由文本，理由与候选淘汰依据相同：结果要能按依据维度复核，而不是
// 退化成检索日志字符串。
type EligibilityBasisReference struct{ requiredValue }

func NewEligibilityBasisReference(value string) (EligibilityBasisReference, error) {
	required, err := newRequiredValue("eligibility basis reference", value)
	return EligibilityBasisReference{required}, err
}

// NetworkJudgmentRequirement 回答本次服务要不要本上下文形成可达性判断。它只有两个取值，
// 都不是三值判断的一部分：`不要求`说的是这个问题不该问，而`不可达`说的是问过了答案是否定的。
type NetworkJudgmentRequirement uint8

const (
	NetworkJudgmentRequirementInvalid NetworkJudgmentRequirement = iota
	NetworkJudgmentRequired
	NetworkJudgmentNotRequired
)

func (requirement NetworkJudgmentRequirement) valid() bool {
	return requirement >= NetworkJudgmentRequired && requirement <= NetworkJudgmentNotRequired
}

func (requirement NetworkJudgmentRequirement) String() string {
	switch requirement {
	case NetworkJudgmentRequired:
		return "REQUIRED"
	case NetworkJudgmentNotRequired:
		return "NOT_REQUIRED"
	default:
		return ""
	}
}

// NetworkEligibility 是商业侧对「这个服务要不要判断网络可达性」的回答。
//
// `不要求`必须携带依据，`要求`不必：用例禁止以`不适用`代替`不可达`，也禁止虚构运营网络。
// 没有依据的`不要求`正是这两件事共同的做法——它让一次未作出的判断看起来像一个结论。构造
// 期就拦住，比留到调用方去检查可靠。
type NetworkEligibility struct {
	requirement NetworkJudgmentRequirement
	basis       EligibilityBasisReference
}

func NewNetworkEligibility(
	requirement NetworkJudgmentRequirement,
	basis EligibilityBasisReference,
) (NetworkEligibility, error) {
	if !requirement.valid() {
		return NetworkEligibility{}, ErrInvalidNetworkEligibility
	}
	if requirement == NetworkJudgmentNotRequired && !basis.valid() {
		return NetworkEligibility{}, ErrInvalidNetworkEligibility
	}
	return NetworkEligibility{requirement: requirement, basis: basis}, nil
}

func (eligibility NetworkEligibility) Requirement() NetworkJudgmentRequirement {
	return eligibility.requirement
}

func (eligibility NetworkEligibility) Basis() EligibilityBasisReference {
	return eligibility.basis
}

// JudgmentRequired 报告是否应当继续形成可达性判断。零值答否但不带依据，因此调用方拿它去
// 构造一个`不适用`结果会在构造期失败——这正是拦住「端口没答话被当成不要求」的办法。
func (eligibility NetworkEligibility) JudgmentRequired() bool {
	return eligibility.requirement == NetworkJudgmentRequired
}
