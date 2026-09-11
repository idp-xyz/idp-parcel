package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidRestriction    = errors.New("customs compliance: invalid regulatory restriction")
	ErrInvalidAdmissibility  = errors.New("customs compliance: invalid admissibility input")
	ErrRestrictionNotCurrent = errors.New("customs compliance: the restriction is not current")
)

// GuardedAction 是受监管门禁约束的方向性动作封闭集合（CONTEXT 硬句逐词：出库、装载
// 出发、跨关务区域移动、交付）。接收、隔离、测量、查验协作与已经授权的处置执行刻意
// 不在此枚举里——「不因此阻止」在类型上落地：没有格的动作根本问不出「被阻断了吗」。
type GuardedAction uint8

const (
	GuardedActionInvalid GuardedAction = iota
	OutboundRelease
	LoadingDeparture
	CrossCustomsMovement
	FinalDelivery
)

func (action GuardedAction) valid() bool {
	return action >= OutboundRelease && action <= FinalDelivery
}

func (action GuardedAction) String() string {
	switch action {
	case OutboundRelease:
		return "OUTBOUND_RELEASE"
	case LoadingDeparture:
		return "LOADING_DEPARTURE"
	case CrossCustomsMovement:
		return "CROSS_CUSTOMS_MOVEMENT"
	case FinalDelivery:
		return "FINAL_DELIVERY"
	default:
		return ""
	}
}

// RestrictionID 是监管限制的标识。
type RestrictionID struct{ requiredValue }

func NewRestrictionID(value string) (RestrictionID, error) {
	required, err := newRequiredValue("restriction ID", value)
	return RestrictionID{required}, err
}

// RegulatoryReleaseReference 指名责任来源接受的监管结果——它是解除限制的唯一依据
// 类型。执行方形成的控制或隔离事实不能解除扣留（CONTEXT 硬句）：那些事实在这里没有
// 对应的构造入口，从源头就换不成这个引用。
type RegulatoryReleaseReference struct{ requiredValue }

func NewRegulatoryReleaseReference(value string) (RegulatoryReleaseReference, error) {
	required, err := newRequiredValue("regulatory release reference", value)
	return RegulatoryReleaseReference{required}, err
}

// RegulatoryRestrictionSpec 是一份监管限制所需的全部输入。
type RegulatoryRestrictionSpec struct {
	ID          RestrictionID
	Decision    RegulatoryDecisionID
	Scope       DecisionScopeReference
	Constrains  []GuardedAction
	EffectiveAt time.Time
}

// RegulatoryRestriction 是作用于明确对象范围的监管限制：明确列出它约束哪些方向性
// 动作。约束集必须显式声明——「阻断其明确约束的」动作，没列的动作不受它管。
type RegulatoryRestriction struct {
	id          RestrictionID
	decision    RegulatoryDecisionID
	scope       DecisionScopeReference
	constrains  []GuardedAction
	effectiveAt time.Time
	releasedBy  RegulatoryReleaseReference
	releasedAt  time.Time
}

func EstablishRestriction(spec RegulatoryRestrictionSpec) (RegulatoryRestriction, error) {
	if !spec.ID.valid() ||
		!spec.Decision.valid() ||
		!spec.Scope.valid() ||
		len(spec.Constrains) == 0 ||
		spec.EffectiveAt.IsZero() {
		return RegulatoryRestriction{}, ErrInvalidRestriction
	}
	seen := make(map[GuardedAction]bool, len(spec.Constrains))
	for _, action := range spec.Constrains {
		if !action.valid() || seen[action] {
			return RegulatoryRestriction{}, ErrInvalidRestriction
		}
		seen[action] = true
	}
	return RegulatoryRestriction{
		id:          spec.ID,
		decision:    spec.Decision,
		scope:       spec.Scope,
		constrains:  append([]GuardedAction(nil), spec.Constrains...),
		effectiveAt: spec.EffectiveAt.UTC(),
	}, nil
}

func (restriction RegulatoryRestriction) ID() RestrictionID {
	return restriction.id
}

func (restriction RegulatoryRestriction) Decision() RegulatoryDecisionID {
	return restriction.decision
}

func (restriction RegulatoryRestriction) Scope() DecisionScopeReference {
	return restriction.scope
}

func (restriction RegulatoryRestriction) Constrains() []GuardedAction {
	return append([]GuardedAction(nil), restriction.constrains...)
}

func (restriction RegulatoryRestriction) EffectiveAt() time.Time {
	return restriction.effectiveAt
}

// Current 报告限制是否仍然有效（成立过且尚未解除）。
func (restriction RegulatoryRestriction) Current() bool {
	return !restriction.effectiveAt.IsZero() && restriction.releasedAt.IsZero()
}

// Release 报告解除依据与时刻，只在已解除的限制上给出。
func (restriction RegulatoryRestriction) Release() (RegulatoryReleaseReference, time.Time, bool) {
	return restriction.releasedBy, restriction.releasedAt, !restriction.releasedAt.IsZero()
}

// ReleaseByRegulatoryOutcome 依据责任来源接受的监管结果解除限制。已解除的限制不再
// 解除第二次；解除时刻不得早于生效。
func (restriction RegulatoryRestriction) ReleaseByRegulatoryOutcome(
	release RegulatoryReleaseReference,
	at time.Time,
) (RegulatoryRestriction, error) {
	if !restriction.Current() {
		return RegulatoryRestriction{}, ErrRestrictionNotCurrent
	}
	if !release.valid() || at.IsZero() || at.Before(restriction.effectiveAt) {
		return RegulatoryRestriction{}, ErrInvalidRestriction
	}
	released := restriction
	released.releasedBy = release
	released.releasedAt = at.UTC()
	return released, nil
}

// ActionAdmissibility 是一次动作准入判断：放行，或被哪些限制阻断。
type ActionAdmissibility struct {
	admissible bool
	blockedBy  []RestrictionID
}

// JudgeActionAdmissibility 判断一个方向性动作对明确对象是否可继续：CONTEXT「只有作用于当前对象和拟执行动作的全部阻断性限制均已解除，相应动作才可继续」。
// 范围不合或不约束
// 此动作的限制不参与；仍有效且约束此动作的限制逐一列进阻断清单——部分解除仍阻断。
func JudgeActionAdmissibility(
	action GuardedAction,
	scope DecisionScopeReference,
	restrictions []RegulatoryRestriction,
) (ActionAdmissibility, error) {
	if !action.valid() || !scope.valid() {
		return ActionAdmissibility{}, ErrInvalidAdmissibility
	}
	blocked := make([]RestrictionID, 0, len(restrictions))
	for _, restriction := range restrictions {
		if !restriction.Current() || restriction.scope != scope {
			continue
		}
		for _, constrained := range restriction.constrains {
			if constrained == action {
				blocked = append(blocked, restriction.id)
				break
			}
		}
	}
	return ActionAdmissibility{
		admissible: len(blocked) == 0,
		blockedBy:  blocked,
	}, nil
}

func (admissibility ActionAdmissibility) Admissible() bool {
	return admissibility.admissible
}

// BlockedBy 给出全部阻断限制——处置者要知道等谁解除，一个都不能少。
func (admissibility ActionAdmissibility) BlockedBy() []RestrictionID {
	return append([]RestrictionID(nil), admissibility.blockedBy...)
}
