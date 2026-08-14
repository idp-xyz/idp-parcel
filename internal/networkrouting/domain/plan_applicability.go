package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidPlanApplicability = errors.New("network routing: invalid plan applicability")
	// ErrPlanNoLongerCurrent 是生命周期硬句的落点：已失效或已被替代的计划不得原地恢复
	// 为当前有效——原路径后来重新可用时，必须重新评估并形成新的计划版本。
	ErrPlanNoLongerCurrent = errors.New("network routing: the plan is no longer currently effective")
)

// ApplicabilityBasisReference 指名一次适用性转移的依据：替代它的新版本、证明失效的硬
// 依据（限制、关闭、实测差异）或责任范围结束的事实。没有依据的失效与拍脑袋下线分不开。
type ApplicabilityBasisReference struct{ requiredValue }

func NewApplicabilityBasisReference(value string) (ApplicabilityBasisReference, error) {
	required, err := newRequiredValue("applicability basis reference", value)
	return ApplicabilityBasisReference{required}, err
}

// PlanApplicabilityState 是路由计划适用性的封闭四态（CONTEXT 生命周期）。`已失效`与
// `已被替代`分开：前者说计划不可执行了（若无新计划包裹立即进入无当前有效路由），后者
// 说新版本接了班——两者的后续动作不同，压成一格会让「该不该有新计划」答不出来。
type PlanApplicabilityState uint8

const (
	PlanApplicabilityStateInvalid PlanApplicabilityState = iota
	PlanCurrentlyEffective
	PlanSuperseded
	PlanLapsed
	PlanConcluded
)

func (state PlanApplicabilityState) valid() bool {
	return state >= PlanCurrentlyEffective && state <= PlanConcluded
}

func (state PlanApplicabilityState) String() string {
	switch state {
	case PlanCurrentlyEffective:
		return "CURRENTLY_EFFECTIVE"
	case PlanSuperseded:
		return "SUPERSEDED"
	case PlanLapsed:
		return "LAPSED"
	case PlanConcluded:
		return "CONCLUDED"
	default:
		return ""
	}
}

// PlanApplicability 是一个计划版本的适用性记录。计划本体不可变（判断产物），适用性另立
// 一份记录：同一份计划的「说了什么」与「现在还算不算数」是两个问题，改后者不得动前者。
type PlanApplicability struct {
	plan           RoutePlanVersionID
	state          PlanApplicabilityState
	transitionedAt time.Time
	basis          ApplicabilityBasisReference
	successor      RoutePlanVersionID
}

// EstablishPlanApplicability 让一个计划版本进入当前有效。生效即有时刻——一个不知何时
// 生效的计划说不清它覆盖哪段旅程。
func EstablishPlanApplicability(plan RoutePlanVersionID, effectiveAt time.Time) (PlanApplicability, error) {
	if !plan.valid() || effectiveAt.IsZero() {
		return PlanApplicability{}, ErrInvalidPlanApplicability
	}
	return PlanApplicability{
		plan:           plan,
		state:          PlanCurrentlyEffective,
		transitionedAt: effectiveAt.UTC(),
	}, nil
}

func (applicability PlanApplicability) Plan() RoutePlanVersionID {
	return applicability.plan
}

func (applicability PlanApplicability) State() PlanApplicabilityState {
	return applicability.state
}

func (applicability PlanApplicability) TransitionedAt() time.Time {
	return applicability.transitionedAt
}

// Basis 交回离开当前有效时的转移依据；仍当前有效时缺席。
func (applicability PlanApplicability) Basis() (ApplicabilityBasisReference, bool) {
	return applicability.basis, applicability.basis.valid()
}

// Successor 只在`已被替代`时交回接班版本。
func (applicability PlanApplicability) Successor() (RoutePlanVersionID, bool) {
	return applicability.successor, applicability.successor.valid()
}

// Supersede 由新版本接班：当前有效 → 已被替代。旧计划、已执行前缀和选择依据继续保留
// ——保留由计划本体的不可变承担，这里只记录接班关系。自代（新版本就是自己）是装配错误。
func (applicability PlanApplicability) Supersede(
	successor RoutePlanVersionID,
	basis ApplicabilityBasisReference,
	at time.Time,
) (PlanApplicability, error) {
	if err := applicability.leaveCurrent(basis, at); err != nil {
		return PlanApplicability{}, err
	}
	if !successor.valid() || successor == applicability.plan {
		return PlanApplicability{}, ErrInvalidPlanApplicability
	}
	applicability.state = PlanSuperseded
	applicability.successor = successor
	applicability.basis = basis
	applicability.transitionedAt = at.UTC()
	return applicability, nil
}

// Lapse 按硬依据证明失效：当前有效 → 已失效。若尚无新的当前有效计划，包裹立即形成
// 无当前有效路由——那个联动由编排在提交边界完成，这里守的是状态与依据。
func (applicability PlanApplicability) Lapse(
	basis ApplicabilityBasisReference,
	at time.Time,
) (PlanApplicability, error) {
	if err := applicability.leaveCurrent(basis, at); err != nil {
		return PlanApplicability{}, err
	}
	applicability.state = PlanLapsed
	applicability.basis = basis
	applicability.transitionedAt = at.UTC()
	return applicability, nil
}

// Conclude 结束计划责任范围：当前有效 → 已结束。已结束不等于运输、异常或财务结果由
// 路由上下文完成（CONTEXT 原句）。
func (applicability PlanApplicability) Conclude(
	basis ApplicabilityBasisReference,
	at time.Time,
) (PlanApplicability, error) {
	if err := applicability.leaveCurrent(basis, at); err != nil {
		return PlanApplicability{}, err
	}
	applicability.state = PlanConcluded
	applicability.basis = basis
	applicability.transitionedAt = at.UTC()
	return applicability, nil
}

// leaveCurrent 是三条离场转移共用的门：只有当前有效能离场（已失效或已被替代不得原地
// 恢复，也不得二次离场——那会改写第一次离场的依据），离场必须带依据与不早于生效的时刻。
func (applicability PlanApplicability) leaveCurrent(
	basis ApplicabilityBasisReference,
	at time.Time,
) error {
	if applicability.state != PlanCurrentlyEffective {
		return ErrPlanNoLongerCurrent
	}
	if !basis.valid() || at.IsZero() || at.Before(applicability.transitionedAt) {
		return ErrInvalidPlanApplicability
	}
	return nil
}

// RehydratePlanApplicabilitySpec 是适用性行在库里的样子。Establish 造不出离场态——
// 离场是转换门，读回不重放 Supersede/Lapse/Conclude。
type RehydratePlanApplicabilitySpec struct {
	Plan           RoutePlanVersionID
	State          PlanApplicabilityState
	TransitionedAt time.Time
	Basis          ApplicabilityBasisReference
	Successor      RoutePlanVersionID
}

func RehydratePlanApplicability(spec RehydratePlanApplicabilitySpec) (PlanApplicability, error) {
	if !spec.Plan.valid() || !spec.State.valid() || spec.TransitionedAt.IsZero() {
		return PlanApplicability{}, ErrInvalidPlanApplicability
	}
	hasBasis := spec.Basis.valid()
	hasSuccessor := spec.Successor.valid()
	switch spec.State {
	case PlanCurrentlyEffective:
		if hasBasis || hasSuccessor {
			return PlanApplicability{}, ErrInvalidPlanApplicability
		}
	case PlanSuperseded:
		if !hasBasis || !hasSuccessor || spec.Successor == spec.Plan {
			return PlanApplicability{}, ErrInvalidPlanApplicability
		}
	case PlanLapsed, PlanConcluded:
		if !hasBasis || hasSuccessor {
			return PlanApplicability{}, ErrInvalidPlanApplicability
		}
	}
	return PlanApplicability{
		plan:           spec.Plan,
		state:          spec.State,
		transitionedAt: spec.TransitionedAt.UTC(),
		basis:          spec.Basis,
		successor:      spec.Successor,
	}, nil
}
