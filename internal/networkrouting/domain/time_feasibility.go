package domain

import (
	"errors"
	"time"
)

var ErrInvalidTimeFeasibility = errors.New("network routing: invalid time feasibility facts")

// CommitmentReference 指名接受时形成的预计承诺版本。承诺属 parcel-shipment，这里只引用
// ——路由计划引用而不修改承诺（UC-NR-001 输入组硬句）。
type CommitmentReference struct{ requiredValue }

func NewCommitmentReference(value string) (CommitmentReference, error) {
	required, err := newRequiredValue("commitment reference", value)
	return CommitmentReference{required}, err
}

// CommittedTimeBound 是预计承诺的时间边界事实：最迟完成时刻加承诺引用。零值即本次服务
// 的承诺没有时间边界（纯服务范围承诺）——层次 3 于是没有时间要求可淘汰。
type CommittedTimeBound struct {
	latest    time.Time
	reference CommitmentReference
}

func NewCommittedTimeBound(latest time.Time, reference CommitmentReference) (CommittedTimeBound, error) {
	if latest.IsZero() || !reference.valid() {
		return CommittedTimeBound{}, ErrInvalidTimeFeasibility
	}
	return CommittedTimeBound{latest: latest.UTC(), reference: reference}, nil
}

func (bound CommittedTimeBound) Latest() time.Time {
	return bound.latest
}

func (bound CommittedTimeBound) Reference() CommitmentReference {
	return bound.reference
}

func (bound CommittedTimeBound) declared() bool {
	return !bound.latest.IsZero()
}

// CandidateTimeProjection 是取数侧按版本化服务日历、截单、节点处理时间与衔接缓冲为一个
// 候选算出的预计完成窗口（UC-NR-001 层次 3 的输入清单）。它是带依据的范围而不是精确
// 时刻——「不得用尚未分配的精确班次时间伪装计划确定性」由 PlannedTimeWindow 的区间
// 形状承担。窗口计算所用的日历与缓冲取值属实例半边（PAR-NET-14）。
type CandidateTimeProjection struct {
	candidate CandidateID
	window    PlannedTimeWindow
}

func NewCandidateTimeProjection(candidate CandidateID, window PlannedTimeWindow) (CandidateTimeProjection, error) {
	if !candidate.valid() || !window.valid() {
		return CandidateTimeProjection{}, ErrInvalidTimeFeasibility
	}
	return CandidateTimeProjection{candidate: candidate, window: window}, nil
}

func (projection CandidateTimeProjection) Candidate() CandidateID {
	return projection.candidate
}

func (projection CandidateTimeProjection) Window() PlannedTimeWindow {
	return projection.window
}

// EvaluateTimeFeasibility 执行候选评估层次 3：投影窗口的最迟边界落在承诺边界之后的合格
// 候选被确定性淘汰，原因携带承诺引用——复算「为什么这条不行」要指得回那份承诺。
//
//   - 承诺没有时间边界时原样通过：没有要求就没有不满足；
//   - 已淘汰候选保持先到的原因（与承诺分界、硬约束同一条纪律），证据未知候选不升不降
//     ——时间可行性判的是「赶不赶得上」，答不了「证据齐不齐」；
//   - 合格候选缺投影是装配错误：一个没有窗口的候选比较不出可行性，静默放行等于用缺席
//     冒充满足。同一候选两条投影互相矛盾，同样拒绝。
func EvaluateTimeFeasibility(
	candidates []RouteCandidate,
	projections []CandidateTimeProjection,
	bound CommittedTimeBound,
) ([]RouteCandidate, error) {
	indexed := make(map[CandidateID]CandidateTimeProjection, len(projections))
	for _, projection := range projections {
		if !projection.candidate.valid() || !projection.window.valid() {
			return nil, ErrInvalidTimeFeasibility
		}
		if _, duplicated := indexed[projection.candidate]; duplicated {
			return nil, ErrInvalidTimeFeasibility
		}
		indexed[projection.candidate] = projection
	}

	if !bound.declared() {
		return append([]RouteCandidate(nil), candidates...), nil
	}

	evaluated := append([]RouteCandidate(nil), candidates...)
	for index, candidate := range evaluated {
		if candidate.outcome != CandidateQualified {
			continue
		}
		projection, present := indexed[candidate.id]
		if !present {
			return nil, ErrInvalidTimeFeasibility
		}
		if !projection.window.latest.After(bound.latest) {
			continue
		}
		reason, err := NewCandidateReason("TIME_COMMITMENT_NOT_MET/" + bound.reference.String())
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
