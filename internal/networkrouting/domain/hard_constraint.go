package domain

import "errors"

var ErrInvalidHardConstraintFinding = errors.New("network routing: invalid hard constraint finding")

// RestrictionReference 指名一条适用限制及其来源责任方（禁限运、关务资格、节点/线路/
// 法人资格）。限制只能由来源责任方解除（层次 5 明确边界），所以淘汰依据必须指得回
// 那个来源——不然「已解除」永远核对不了。
type RestrictionReference struct{ requiredValue }

func NewRestrictionReference(value string) (RestrictionReference, error) {
	required, err := newRequiredValue("restriction reference", value)
	return RestrictionReference{required}, err
}

// HardConstraintOutcome 是一条候选的硬约束判定封闭三值。`状态未知`单独一格：层次 5
// 要求「由证据缺口还是依赖失败决定结果」——依赖失败根本到不了这里（端口错误折
// `未形成判断`），能落成事实的未知只有业务证据缺口一种，它有缺什么与何时再判的出口。
type HardConstraintOutcome uint8

const (
	HardConstraintOutcomeInvalid HardConstraintOutcome = iota
	ConstraintSatisfied
	RestrictionApplies
	ConstraintStatusUnknown
)

func (outcome HardConstraintOutcome) valid() bool {
	return outcome >= ConstraintSatisfied && outcome <= ConstraintStatusUnknown
}

func (outcome HardConstraintOutcome) String() string {
	switch outcome {
	case ConstraintSatisfied:
		return "SATISFIED"
	case RestrictionApplies:
		return "RESTRICTION_APPLIES"
	case ConstraintStatusUnknown:
		return "STATUS_UNKNOWN"
	default:
		return ""
	}
}

// HardConstraintFindingSpec 是一条候选的硬约束判定事实所需的全部输入。真实禁限运表、
// 关务资格与法人资格属实例半边（PAR-CUS-*/PAR-NET-*），机制只规定形状与折叠规则。
type HardConstraintFindingSpec struct {
	Candidate   CandidateID
	Outcome     HardConstraintOutcome
	Restriction RestrictionReference
	Missing     EvidenceGapReference
	Reassess    ReassessmentCondition
}

// HardConstraintFinding 是硬约束判定事实，不是结论。
type HardConstraintFinding struct {
	candidate   CandidateID
	outcome     HardConstraintOutcome
	restriction RestrictionReference
	missing     EvidenceGapReference
	reassess    ReassessmentCondition
}

// NewHardConstraintFinding 按结果分片校验：适用限制必须指名限制来源且不得携带缺口
// 字段；状态未知必须带缺少内容与再次判断条件且不得指名限制——两头都带的事实读不出
// 它到底是定论还是缺口；满足则三者全免。
func NewHardConstraintFinding(spec HardConstraintFindingSpec) (HardConstraintFinding, error) {
	if !spec.Candidate.valid() || !spec.Outcome.valid() {
		return HardConstraintFinding{}, ErrInvalidHardConstraintFinding
	}
	switch spec.Outcome {
	case ConstraintSatisfied:
		if spec.Restriction.valid() || spec.Missing.valid() || spec.Reassess.valid() {
			return HardConstraintFinding{}, ErrInvalidHardConstraintFinding
		}
	case RestrictionApplies:
		if !spec.Restriction.valid() || spec.Missing.valid() || spec.Reassess.valid() {
			return HardConstraintFinding{}, ErrInvalidHardConstraintFinding
		}
	case ConstraintStatusUnknown:
		if spec.Restriction.valid() || !spec.Missing.valid() || !spec.Reassess.valid() {
			return HardConstraintFinding{}, ErrInvalidHardConstraintFinding
		}
	}
	return HardConstraintFinding{
		candidate:   spec.Candidate,
		outcome:     spec.Outcome,
		restriction: spec.Restriction,
		missing:     spec.Missing,
		reassess:    spec.Reassess,
	}, nil
}

func (finding HardConstraintFinding) Candidate() CandidateID {
	return finding.candidate
}

func (finding HardConstraintFinding) Outcome() HardConstraintOutcome {
	return finding.outcome
}

// EvaluateHardConstraints 执行候选评估层次 5 与 `AT-NR-031`：
//
//   - 适用限制确定性淘汰，原因携带限制来源引用——不淘汰整次判断，只淘汰这条候选，
//     其余候选各自依据保留（`AT-NR-031`「保存两条候选各自依据」）。本函数不解除限制
//     也不建立正式关务案件。
//   - 状态未知把合格候选降为`证据未知`并留下候选级缺口（缺什么、何时再判）；它推翻
//     不了别的已合格候选（`AT-NR-025` 既有矩阵），也不升不降已淘汰候选。
//   - 已淘汰候选保持原依据：限制叠不上去，未知也翻不回来。
//
// 同一候选多条事实允许（多种限制并检），但矛盾组合（既适用限制又状态未知）按先淘汰
// 后未知的折叠自然收敛——淘汰先落地则未知不再降级，未知先落地也挡不住后来的确定淘汰。
func EvaluateHardConstraints(
	candidates []RouteCandidate,
	findings []HardConstraintFinding,
) ([]RouteCandidate, []EvidenceGap, error) {
	evaluated := append([]RouteCandidate(nil), candidates...)
	position := make(map[CandidateID]int, len(evaluated))
	for index, candidate := range evaluated {
		position[candidate.id] = index
	}

	gaps := make([]EvidenceGap, 0)
	for _, finding := range findings {
		if !finding.candidate.valid() || !finding.outcome.valid() {
			return nil, nil, ErrInvalidHardConstraintFinding
		}
		index, known := position[finding.candidate]
		if !known {
			// 指向候选空间之外的事实是装配错误：静默丢弃会把一条本该淘汰的限制吞掉。
			return nil, nil, ErrInvalidHardConstraintFinding
		}
		candidate := evaluated[index]

		switch finding.outcome {
		case ConstraintSatisfied:
			continue
		case RestrictionApplies:
			if candidate.outcome == CandidateEliminated {
				continue
			}
			reason, err := NewCandidateReason("HARD_CONSTRAINT_RESTRICTION/" + finding.restriction.String())
			if err != nil {
				return nil, nil, err
			}
			eliminated, err := NewRouteCandidate(candidate.id, CandidateEliminated, reason)
			if err != nil {
				return nil, nil, err
			}
			evaluated[index] = eliminated
		case ConstraintStatusUnknown:
			// 只有合格候选降为证据未知；已淘汰不回翻、已未知保持首个依据。缺口一律照记
			// 作复算痕迹——候选级缺口不驱动结论（ConcludeReachability 只数未知候选与
			// 全局缺口），但复核者要看到形成结论时还有什么是未知的。
			if candidate.outcome == CandidateQualified {
				reason, err := NewCandidateReason("HARD_CONSTRAINT_STATUS_UNKNOWN")
				if err != nil {
					return nil, nil, err
				}
				unknown, err := NewRouteCandidate(candidate.id, CandidateEvidenceUnknown, reason)
				if err != nil {
					return nil, nil, err
				}
				evaluated[index] = unknown
			}
			gap, err := NewEvidenceGap(finding.missing, CandidateScopedGap, []CandidateID{finding.candidate}, finding.reassess)
			if err != nil {
				return nil, nil, err
			}
			gaps = append(gaps, gap)
		}
	}
	return evaluated, gaps, nil
}
