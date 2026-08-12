package domain

import "errors"

var ErrInvalidRouteRequirement = errors.New("network routing: invalid route requirement")

// RouteRequirementReference 指名客户提出的线路、节点、口岸或渠道要求。它是引用而非
// 自由文本，与 CandidateReason 同一个理由：结果要能按要求维度统计与复算。
type RouteRequirementReference struct{ requiredValue }

func NewRouteRequirementReference(value string) (RouteRequirementReference, error) {
	required, err := newRequiredValue("route requirement reference", value)
	return RouteRequirementReference{required}, err
}

// CommitmentBasisReference 指名把一条客户要求升格为约束的产品或合同承诺依据。没有
// 依据的「承诺」与拍脑袋升格分不开——UC-NR-002 硬句：要求只有被产品或合同明确承诺
// 时才成为约束。
type CommitmentBasisReference struct{ requiredValue }

func NewCommitmentBasisReference(value string) (CommitmentBasisReference, error) {
	required, err := newRequiredValue("commitment basis reference", value)
	return CommitmentBasisReference{required}, err
}

// RequirementBinding 是一条客户要求的约束力二值：普通偏好或产品/合同承诺。封闭集合，
// 没有第三格——「可能有约束力」读不出任何一条可执行的评估规则。
type RequirementBinding uint8

const (
	RequirementBindingInvalid RequirementBinding = iota
	PlainPreference
	CommittedRequirement
)

func (binding RequirementBinding) valid() bool {
	return binding == PlainPreference || binding == CommittedRequirement
}

func (binding RequirementBinding) String() string {
	switch binding {
	case PlainPreference:
		return "PLAIN_PREFERENCE"
	case CommittedRequirement:
		return "COMMITTED_REQUIREMENT"
	default:
		return ""
	}
}

// RouteRequirementSpec 是一条客户要求事实所需的全部输入。哪些候选满足该要求由事实
// 提供方解析（真实线路与渠道匹配属实例半边），领域只拥有约束力分界的判定规则。
type RouteRequirementSpec struct {
	Requirement RouteRequirementReference
	Binding     RequirementBinding
	Basis       CommitmentBasisReference
	Satisfies   []CandidateID
}

// RouteRequirement 是约束力已判定的客户要求事实。
type RouteRequirement struct {
	requirement RouteRequirementReference
	binding     RequirementBinding
	basis       CommitmentBasisReference
	satisfies   []CandidateID
}

// NewRouteRequirement 按约束力分片校验：承诺必须带产品/合同依据——依据正是它与偏好的
// 分界；偏好不得带依据——有承诺依据却声称偏好，两种读法会给出相反的评估结果，这种
// 形状必须在构造期就死。
func NewRouteRequirement(spec RouteRequirementSpec) (RouteRequirement, error) {
	if !spec.Requirement.valid() || !spec.Binding.valid() {
		return RouteRequirement{}, ErrInvalidRouteRequirement
	}
	switch spec.Binding {
	case CommittedRequirement:
		if !spec.Basis.valid() {
			return RouteRequirement{}, ErrInvalidRouteRequirement
		}
	case PlainPreference:
		if spec.Basis.valid() {
			return RouteRequirement{}, ErrInvalidRouteRequirement
		}
	}
	for _, id := range spec.Satisfies {
		if !id.valid() {
			return RouteRequirement{}, ErrInvalidRouteRequirement
		}
	}
	return RouteRequirement{
		requirement: spec.Requirement,
		binding:     spec.Binding,
		basis:       spec.Basis,
		satisfies:   append([]CandidateID(nil), spec.Satisfies...),
	}, nil
}

func (requirement RouteRequirement) Requirement() RouteRequirementReference {
	return requirement.requirement
}

func (requirement RouteRequirement) Binding() RequirementBinding {
	return requirement.binding
}

// EvaluateRouteRequirements 执行承诺/偏好分界（`AT-NR-026`）：
//
//   - 普通偏好不参与淘汰也不制造可达——候选空间原样通过，本来可达的包裹不会因为偏好
//     未被满足而变成不可达。
//   - 承诺是硬约束：不满足承诺的候选确定性淘汰，原因携带承诺依据。证据未知的候选同样
//     淘汰——是否满足承诺由事实提供方解析完毕，这一格的确定性不依赖那份缺失的证据；
//     它的缺口仍留在判断上作复算痕迹，但不再驱动结论（缺补齐也改变不了被承诺排除）。
//   - 已淘汰的候选保持原淘汰原因：承诺只收窄不复活，满足承诺绕不过先前的硬限制
//     （硬句「不得绕过硬限制制造可达结果」）。
func EvaluateRouteRequirements(
	candidates []RouteCandidate,
	requirements []RouteRequirement,
) ([]RouteCandidate, error) {
	evaluated := append([]RouteCandidate(nil), candidates...)
	for _, requirement := range requirements {
		if !requirement.requirement.valid() || !requirement.binding.valid() {
			return nil, ErrInvalidRouteRequirement
		}
		if requirement.binding == PlainPreference {
			continue
		}
		satisfied := make(map[CandidateID]struct{}, len(requirement.satisfies))
		for _, id := range requirement.satisfies {
			satisfied[id] = struct{}{}
		}
		for index, candidate := range evaluated {
			if candidate.outcome == CandidateEliminated {
				continue
			}
			if _, ok := satisfied[candidate.id]; ok {
				continue
			}
			reason, err := NewCandidateReason(
				"ROUTE_COMMITMENT_NOT_SATISFIED/" + requirement.basis.String())
			if err != nil {
				return nil, err
			}
			eliminated, err := NewRouteCandidate(candidate.id, CandidateEliminated, reason)
			if err != nil {
				return nil, err
			}
			evaluated[index] = eliminated
		}
	}
	return evaluated, nil
}
