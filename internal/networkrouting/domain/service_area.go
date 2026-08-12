package domain

import "errors"

var ErrInvalidServiceAreaResolution = errors.New("network routing: invalid service area resolution")

// ServiceAreaVersionReference 指名一次解析所依据的已发布服务区域版本。矩阵行 5 要求
// 「相关版本和解析依据完整」才允许确定性淘汰——没有版本的排除与拍脑袋排除分不开。
type ServiceAreaVersionReference struct{ requiredValue }

func NewServiceAreaVersionReference(value string) (ServiceAreaVersionReference, error) {
	required, err := newRequiredValue("service area version reference", value)
	return ServiceAreaVersionReference{required}, err
}

// ReassessmentCondition 是`资料不足`的出口：什么条件成立后值得再判一次（`AT-NR-018`
// 「指出缺少内容和再次判断条件」）。没有它，资料不足是一句没有出口的否定。
type ReassessmentCondition struct{ requiredValue }

func NewReassessmentCondition(value string) (ReassessmentCondition, error) {
	required, err := newRequiredValue("reassessment condition", value)
	return ReassessmentCondition{required}, err
}

// ServiceAreaResolutionOutcome 是一条候选的区域解析事实的封闭三值。`明确排除`与`地址
// 信息不足`分开正是矩阵行 5/6 的分界：前者是确定性覆盖结论（可淘汰），后者是证据缺口
// （不得假设默认区域，也不得记成不可达）。
type ServiceAreaResolutionOutcome uint8

const (
	ServiceAreaResolutionOutcomeInvalid ServiceAreaResolutionOutcome = iota
	AreaCoversDestination
	AreaExcludesDestination
	AddressInformationInsufficient
)

func (outcome ServiceAreaResolutionOutcome) valid() bool {
	return outcome >= AreaCoversDestination && outcome <= AddressInformationInsufficient
}

func (outcome ServiceAreaResolutionOutcome) String() string {
	switch outcome {
	case AreaCoversDestination:
		return "COVERS_DESTINATION"
	case AreaExcludesDestination:
		return "EXCLUDES_DESTINATION"
	case AddressInformationInsufficient:
		return "ADDRESS_INFORMATION_INSUFFICIENT"
	default:
		return ""
	}
}

// ServiceAreaResolutionSpec 是一条候选的区域解析事实所需的全部输入。真实区域表与地址
// 语义属实例半边（PAR-NET-*），机制只规定事实的形状与判定规则。
type ServiceAreaResolutionSpec struct {
	Candidate   CandidateID
	Outcome     ServiceAreaResolutionOutcome
	AreaVersion ServiceAreaVersionReference
	Missing     EvidenceGapReference
	Reassess    ReassessmentCondition
}

// ServiceAreaResolution 是版本化的解析事实，不是结论：结论由 EvaluateServiceAreas 按
// 矩阵行 5/6 折出来。
type ServiceAreaResolution struct {
	candidate   CandidateID
	outcome     ServiceAreaResolutionOutcome
	areaVersion ServiceAreaVersionReference
	missing     EvidenceGapReference
	reassess    ReassessmentCondition
}

// NewServiceAreaResolution 按结果分片校验：可判定结论（覆盖/排除）必须带完整版本依据且
// 不得携带缺口字段——带了会把定论与缺口混成一格；`地址信息不足`必须带缺少内容与再次判断
// 条件，版本可缺（解析没走完本就可能还没选定版本）。
func NewServiceAreaResolution(spec ServiceAreaResolutionSpec) (ServiceAreaResolution, error) {
	if !spec.Candidate.valid() || !spec.Outcome.valid() {
		return ServiceAreaResolution{}, ErrInvalidServiceAreaResolution
	}
	switch spec.Outcome {
	case AreaCoversDestination, AreaExcludesDestination:
		if !spec.AreaVersion.valid() || spec.Missing.valid() || spec.Reassess.valid() {
			return ServiceAreaResolution{}, ErrInvalidServiceAreaResolution
		}
	case AddressInformationInsufficient:
		if !spec.Missing.valid() || !spec.Reassess.valid() {
			return ServiceAreaResolution{}, ErrInvalidServiceAreaResolution
		}
	}
	return ServiceAreaResolution{
		candidate:   spec.Candidate,
		outcome:     spec.Outcome,
		areaVersion: spec.AreaVersion,
		missing:     spec.Missing,
		reassess:    spec.Reassess,
	}, nil
}

func (resolution ServiceAreaResolution) Candidate() CandidateID {
	return resolution.candidate
}

func (resolution ServiceAreaResolution) Outcome() ServiceAreaResolutionOutcome {
	return resolution.outcome
}

func (resolution ServiceAreaResolution) AreaVersion() ServiceAreaVersionReference {
	return resolution.areaVersion
}

// EvaluateServiceAreas 执行矩阵行 5/6：明确排除折成带依据的淘汰（原因引用携带区域版本，
// `AT-NR-023` 要求排除依据与解析一并保存，而淘汰留下的痕迹就是这条原因）；地址信息不足
// 折成`证据未知`候选加候选级缺口（缺什么、什么条件再判）；覆盖折成合格候选——后续增量的
// 日历/截单与硬约束评估在合格之上继续收窄，本函数不越位。
//
// 同一候选出现两条解析事实是装配错误：两条可能互相矛盾，取哪条都是掷硬币。
func EvaluateServiceAreas(resolutions []ServiceAreaResolution) ([]RouteCandidate, []EvidenceGap, error) {
	seen := make(map[CandidateID]struct{}, len(resolutions))
	candidates := make([]RouteCandidate, 0, len(resolutions))
	gaps := make([]EvidenceGap, 0)

	for _, resolution := range resolutions {
		if !resolution.candidate.valid() || !resolution.outcome.valid() {
			return nil, nil, ErrInvalidServiceAreaResolution
		}
		if _, duplicated := seen[resolution.candidate]; duplicated {
			return nil, nil, ErrInvalidServiceAreaResolution
		}
		seen[resolution.candidate] = struct{}{}

		switch resolution.outcome {
		case AreaCoversDestination:
			candidate, err := NewRouteCandidate(resolution.candidate, CandidateQualified, CandidateReason{})
			if err != nil {
				return nil, nil, err
			}
			candidates = append(candidates, candidate)
		case AreaExcludesDestination:
			// 原因引用携带区域版本：淘汰的稳定原因与它所依据的那一版一起留在候选上，
			// `不可达`的复算才有处可查。
			reason, err := NewCandidateReason("SERVICE_AREA_EXCLUDES_DESTINATION/" + resolution.areaVersion.String())
			if err != nil {
				return nil, nil, err
			}
			candidate, err := NewRouteCandidate(resolution.candidate, CandidateEliminated, reason)
			if err != nil {
				return nil, nil, err
			}
			candidates = append(candidates, candidate)
		case AddressInformationInsufficient:
			reason, err := NewCandidateReason("ADDRESS_INFORMATION_INSUFFICIENT")
			if err != nil {
				return nil, nil, err
			}
			candidate, err := NewRouteCandidate(resolution.candidate, CandidateEvidenceUnknown, reason)
			if err != nil {
				return nil, nil, err
			}
			candidates = append(candidates, candidate)
			gap, err := NewEvidenceGap(resolution.missing, CandidateScopedGap, []CandidateID{resolution.candidate}, resolution.reassess)
			if err != nil {
				return nil, nil, err
			}
			gaps = append(gaps, gap)
		}
	}
	return candidates, gaps, nil
}
