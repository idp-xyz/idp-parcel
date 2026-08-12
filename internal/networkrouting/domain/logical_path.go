package domain

import "errors"

var ErrInvalidPathExecutability = errors.New("network routing: invalid path executability")

// ScheduleVersionReference 指名一次可执行性判定所依据的服务日历/截单/临时可用性版本。
// 两个方向都必备：排除要能复算（矩阵行 2「有效权威依据确定性淘汰」），可执行同样要能
// 复算——没有版本的「可执行」在日历换代后无从比对。必需网络版本读不到不是这里的事实，
// 是端口错误折应用层`未形成判断`（矩阵行 7）。
type ScheduleVersionReference struct{ requiredValue }

func NewScheduleVersionReference(value string) (ScheduleVersionReference, error) {
	required, err := newRequiredValue("schedule version reference", value)
	return ScheduleVersionReference{required}, err
}

// PathExecutabilityOutcome 是一条候选的逻辑路径可执行性二值。刻意没有「未知」格：
// 层次 4 的输入（连接、线路、日历、截单、临时可用性）全部是运营企业自有网络事实，
// 读不到属技术可用性（未形成判断），不存在「向客户要资料」的缺口出口。
type PathExecutabilityOutcome uint8

const (
	PathExecutabilityOutcomeInvalid PathExecutabilityOutcome = iota
	PathExecutable
	PathNotExecutable
)

func (outcome PathExecutabilityOutcome) valid() bool {
	return outcome == PathExecutable || outcome == PathNotExecutable
}

func (outcome PathExecutabilityOutcome) String() string {
	switch outcome {
	case PathExecutable:
		return "EXECUTABLE"
	case PathNotExecutable:
		return "NOT_EXECUTABLE"
	default:
		return ""
	}
}

// PathExecutabilitySpec 是一条候选的逻辑路径可执行性事实所需的全部输入。真实日历、
// 时区与截单属实例半边（PAR-NET-*），机制只规定事实形状与折叠规则。
type PathExecutabilitySpec struct {
	Candidate CandidateID
	Outcome   PathExecutabilityOutcome
	Schedule  ScheduleVersionReference
}

// PathExecutability 是版本化的可执行性事实，不是结论。
type PathExecutability struct {
	candidate CandidateID
	outcome   PathExecutabilityOutcome
	schedule  ScheduleVersionReference
}

func NewPathExecutability(spec PathExecutabilitySpec) (PathExecutability, error) {
	if !spec.Candidate.valid() || !spec.Outcome.valid() || !spec.Schedule.valid() {
		return PathExecutability{}, ErrInvalidPathExecutability
	}
	return PathExecutability{
		candidate: spec.Candidate,
		outcome:   spec.Outcome,
		schedule:  spec.Schedule,
	}, nil
}

func (executability PathExecutability) Candidate() CandidateID {
	return executability.candidate
}

func (executability PathExecutability) Outcome() PathExecutabilityOutcome {
	return executability.outcome
}

// EvaluatePathExecutability 执行候选评估层次 4：排除不可执行路径，淘汰原因携带日历/
// 截单版本依据。已淘汰候选保持原依据；没有事实的候选原样通过——事实是声明式的，缺一条
// 不等于排除。同一候选两条事实互相矛盾时取哪条都是掷硬币，作装配错误上抛。
//
// 本函数不选择当前有效路线也不绑定班次（层次 4 明确边界）：它只做候选级排除。
func EvaluatePathExecutability(
	candidates []RouteCandidate,
	findings []PathExecutability,
) ([]RouteCandidate, error) {
	byCandidate := make(map[CandidateID]PathExecutability, len(findings))
	for _, finding := range findings {
		if !finding.candidate.valid() || !finding.outcome.valid() {
			return nil, ErrInvalidPathExecutability
		}
		if _, duplicated := byCandidate[finding.candidate]; duplicated {
			return nil, ErrInvalidPathExecutability
		}
		byCandidate[finding.candidate] = finding
	}

	evaluated := append([]RouteCandidate(nil), candidates...)
	for index, candidate := range evaluated {
		finding, present := byCandidate[candidate.id]
		if !present || finding.outcome != PathNotExecutable {
			continue
		}
		if candidate.outcome == CandidateEliminated {
			continue
		}
		reason, err := NewCandidateReason("PATH_NOT_EXECUTABLE/" + finding.schedule.String())
		if err != nil {
			return nil, err
		}
		eliminated, err := NewRouteCandidate(candidate.id, CandidateEliminated, reason)
		if err != nil {
			return nil, err
		}
		evaluated[index] = eliminated
	}
	return evaluated, nil
}
