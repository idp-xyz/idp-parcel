package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidRoutePlan          = errors.New("network routing: invalid initial route plan")
	ErrInvalidNoCurrentRoute     = errors.New("network routing: invalid no-current-route judgment")
	ErrCandidateSpaceUndecided   = errors.New("network routing: candidate space still carries undecided candidates")
	ErrSelectedCandidateNotFound = errors.New("network routing: selected candidate is not a qualified member of the evaluated space")
)

// RoutePlanVersionID 是一个包裹级路由计划版本的标识。计划按包裹独立版本化，改路形成
// 新版本而不覆盖本版。
type RoutePlanVersionID struct{ requiredValue }

func NewRoutePlanVersionID(value string) (RoutePlanVersionID, error) {
	required, err := newRequiredValue("route plan version ID", value)
	return RoutePlanVersionID{required}, err
}

// PlanNodeReference 指名一个计划节点（网络节点的引用）。真实节点属实例半边
// （PAR-NET-02..06），机制只要求节点被指名。
type PlanNodeReference struct{ requiredValue }

func NewPlanNodeReference(value string) (PlanNodeReference, error) {
	required, err := newRequiredValue("plan node reference", value)
	return PlanNodeReference{required}, err
}

// ResponsiblePartyReference 指名一段计划履约段的已知责任方。
type ResponsiblePartyReference struct{ requiredValue }

func NewResponsiblePartyReference(value string) (ResponsiblePartyReference, error) {
	required, err := newRequiredValue("responsible party reference", value)
	return ResponsiblePartyReference{required}, err
}

// RouteStrategyReference 指名本次排序采用的路由策略版本。策略取值（权重、阈值、优先级）
// 属实例半边（PAR-NET-14）；「应用层不得自行发明固定权重」靠它可追溯。
type RouteStrategyReference struct{ requiredValue }

func NewRouteStrategyReference(value string) (RouteStrategyReference, error) {
	required, err := newRequiredValue("route strategy reference", value)
	return RouteStrategyReference{required}, err
}

// Valid 供应用层核对证据答复的完整性：没有策略引用的判断复算不了，与修订标识同一条
// 「答复缺件响亮上抛」的纪律。
func (reference RouteStrategyReference) Valid() bool {
	return reference.valid()
}

// WindowBasisReference 指名一个计划时间窗口的形成依据（服务日历、节点处理时间与衔接
// 缓冲的版本引用）。窗口不带依据就复算不了——计划内容清单点名要它。
type WindowBasisReference struct{ requiredValue }

func NewWindowBasisReference(value string) (WindowBasisReference, error) {
	required, err := newRequiredValue("window basis reference", value)
	return WindowBasisReference{required}, err
}

// PlannedTimeWindow 是计划节点或计划履约段的预计范围：最早/最迟边界加形成依据。它不是
// 尚未分配班次的精确时刻（CONTEXT 语言），所以是区间不是时点——一个 earliest==latest
// 的窗口合法但少见，倒挂的窗口不是一个范围。
type PlannedTimeWindow struct {
	earliest time.Time
	latest   time.Time
	basis    WindowBasisReference
}

func NewPlannedTimeWindow(earliest, latest time.Time, basis WindowBasisReference) (PlannedTimeWindow, error) {
	if earliest.IsZero() || latest.IsZero() || latest.Before(earliest) || !basis.valid() {
		return PlannedTimeWindow{}, ErrInvalidRoutePlan
	}
	return PlannedTimeWindow{earliest: earliest.UTC(), latest: latest.UTC(), basis: basis}, nil
}

func (window PlannedTimeWindow) Earliest() time.Time {
	return window.earliest
}

func (window PlannedTimeWindow) Latest() time.Time {
	return window.latest
}

func (window PlannedTimeWindow) Basis() WindowBasisReference {
	return window.basis
}

func (window PlannedTimeWindow) valid() bool {
	return !window.earliest.IsZero() && !window.latest.IsZero() &&
		!window.latest.Before(window.earliest) && window.basis.valid()
}

// PlannedLeg 是两个计划边界之间的逻辑移动段。外部不透明段（CONTEXT：外包合作方内部
// 网络不可可靠观察）与普通段同一形状——from 是已知交接点、to 是预期返回控制点或交付
// 范围，区别只在「不虚构内部节点」这个事实由 opaque 标记声明。
type PlannedLeg struct {
	from        PlanNodeReference
	to          PlanNodeReference
	responsible ResponsiblePartyReference
	window      PlannedTimeWindow
	opaque      bool
}

type PlannedLegSpec struct {
	From        PlanNodeReference
	To          PlanNodeReference
	Responsible ResponsiblePartyReference
	Window      PlannedTimeWindow
	Opaque      bool
}

func NewPlannedLeg(spec PlannedLegSpec) (PlannedLeg, error) {
	if !spec.From.valid() || !spec.To.valid() || spec.From == spec.To ||
		!spec.Responsible.valid() || !spec.Window.valid() {
		return PlannedLeg{}, ErrInvalidRoutePlan
	}
	return PlannedLeg{
		from:        spec.From,
		to:          spec.To,
		responsible: spec.Responsible,
		window:      spec.Window,
		opaque:      spec.Opaque,
	}, nil
}

func (leg PlannedLeg) From() PlanNodeReference {
	return leg.from
}

func (leg PlannedLeg) To() PlanNodeReference {
	return leg.to
}

func (leg PlannedLeg) Responsible() ResponsiblePartyReference {
	return leg.responsible
}

func (leg PlannedLeg) Window() PlannedTimeWindow {
	return leg.window
}

// Opaque 报告本段是否为外部不透明履约段——true 说的是「合作方内部不可可靠观察，本段
// 只表达已知交接点与返回控制边界」，不是段的质量差。
func (leg PlannedLeg) Opaque() bool {
	return leg.opaque
}

func (leg PlannedLeg) valid() bool {
	return leg.from.valid() && leg.to.valid() && leg.from != leg.to &&
		leg.responsible.valid() && leg.window.valid()
}

// InitialRoutePlanSpec 是形成一个包裹级初始路由计划所需的全部输入。
type InitialRoutePlanSpec struct {
	Key           InitialRouteJudgmentKey
	Version       RoutePlanVersionID
	Selected      CandidateID
	Candidates    []RouteCandidate
	Legs          []PlannedLeg
	Strategy      RouteStrategyReference
	ViewRevision  NetworkViewRevision
	JudgedAt      time.Time
	EffectiveFrom time.Time
}

// InitialRoutePlan 是针对一个明确包裹和服务目的、带版本和生效边界的未来路径意图
// （CONTEXT 语言）。它不是客户承诺、班次、装载或实际履约——那些对象这里根本造不出来。
type InitialRoutePlan struct {
	key           InitialRouteJudgmentKey
	version       RoutePlanVersionID
	selected      CandidateID
	candidates    []RouteCandidate
	legs          []PlannedLeg
	strategy      RouteStrategyReference
	viewRevision  NetworkViewRevision
	judgedAt      time.Time
	effectiveFrom time.Time
}

// FormInitialRoutePlan 立三类不变量：
//
//   - 判断身份、版本、策略与修订标识必备——审计清单点名它们，缺一项计划就复算不了；
//   - 被选候选必须是已评估空间里的合格成员（`AT-NR-001`「只形成一个当前有效计划并
//     保留所有候选依据」——选了一个不在册或已被淘汰的候选，依据链当场断裂）；
//   - 段链必须连续且至少一段：计划节点序列由段推导，断链的「路径」不是一条路径。
func FormInitialRoutePlan(spec InitialRoutePlanSpec) (InitialRoutePlan, error) {
	if !spec.Key.MinimumIdentityEstablished() ||
		!spec.Version.valid() ||
		!spec.Strategy.valid() ||
		!spec.ViewRevision.valid() ||
		spec.JudgedAt.IsZero() ||
		spec.EffectiveFrom.IsZero() {
		return InitialRoutePlan{}, ErrInvalidRoutePlan
	}

	if len(spec.Candidates) == 0 {
		return InitialRoutePlan{}, ErrInvalidRoutePlan
	}
	selectedQualified := false
	seen := make(map[CandidateID]struct{}, len(spec.Candidates))
	for _, candidate := range spec.Candidates {
		if !candidate.id.valid() || !candidate.outcome.valid() {
			return InitialRoutePlan{}, ErrInvalidRoutePlan
		}
		if _, duplicated := seen[candidate.id]; duplicated {
			return InitialRoutePlan{}, ErrDuplicateRouteCandidate
		}
		seen[candidate.id] = struct{}{}
		if candidate.id == spec.Selected && candidate.outcome == CandidateQualified {
			selectedQualified = true
		}
	}
	if !spec.Selected.valid() || !selectedQualified {
		return InitialRoutePlan{}, ErrSelectedCandidateNotFound
	}

	if len(spec.Legs) == 0 {
		return InitialRoutePlan{}, ErrInvalidRoutePlan
	}
	for index, leg := range spec.Legs {
		if !leg.valid() {
			return InitialRoutePlan{}, ErrInvalidRoutePlan
		}
		if index > 0 && spec.Legs[index-1].to != leg.from {
			return InitialRoutePlan{}, ErrInvalidRoutePlan
		}
	}

	return InitialRoutePlan{
		key:           spec.Key,
		version:       spec.Version,
		selected:      spec.Selected,
		candidates:    append([]RouteCandidate(nil), spec.Candidates...),
		legs:          append([]PlannedLeg(nil), spec.Legs...),
		strategy:      spec.Strategy,
		viewRevision:  spec.ViewRevision,
		judgedAt:      spec.JudgedAt.UTC(),
		effectiveFrom: spec.EffectiveFrom.UTC(),
	}, nil
}

func (plan InitialRoutePlan) Key() InitialRouteJudgmentKey {
	return plan.key
}

func (plan InitialRoutePlan) Version() RoutePlanVersionID {
	return plan.version
}

func (plan InitialRoutePlan) SelectedCandidate() CandidateID {
	return plan.selected
}

// Candidates 交回全部被评估候选（含淘汰依据）的拷贝——「保留所有候选依据」是计划内容
// 清单的硬句，复核者要看到没被选中的那些为什么落选。
func (plan InitialRoutePlan) Candidates() []RouteCandidate {
	return append([]RouteCandidate(nil), plan.candidates...)
}

func (plan InitialRoutePlan) Legs() []PlannedLeg {
	return append([]PlannedLeg(nil), plan.legs...)
}

// Nodes 按段链推导有序计划节点序列。节点不单独存一份：两份就可能各说各话，而段链连续
// 由构造期保证。
func (plan InitialRoutePlan) Nodes() []PlanNodeReference {
	nodes := make([]PlanNodeReference, 0, len(plan.legs)+1)
	nodes = append(nodes, plan.legs[0].from)
	for _, leg := range plan.legs {
		nodes = append(nodes, leg.to)
	}
	return nodes
}

func (plan InitialRoutePlan) Strategy() RouteStrategyReference {
	return plan.strategy
}

func (plan InitialRoutePlan) ViewRevision() NetworkViewRevision {
	return plan.viewRevision
}

func (plan InitialRoutePlan) JudgedAt() time.Time {
	return plan.judgedAt
}

func (plan InitialRoutePlan) EffectiveFrom() time.Time {
	return plan.effectiveFrom
}

// NoCurrentRouteJudgment 是「无当前有效路由」的领域判断（CONTEXT 语言：已接受包裹当前
// 不存在已生效且适用的路由计划时形成的明确判断）。它阻止需要下一跳依据的后续装载，但
// 不取消委托也不形成终局失败——那些决定属拥有服务变更权的上下文。
type NoCurrentRouteJudgment struct {
	key          InitialRouteJudgmentKey
	candidates   []RouteCandidate
	strategy     RouteStrategyReference
	viewRevision NetworkViewRevision
	judgedAt     time.Time
}

type NoCurrentRouteJudgmentSpec struct {
	Key          InitialRouteJudgmentKey
	Candidates   []RouteCandidate
	Strategy     RouteStrategyReference
	ViewRevision NetworkViewRevision
	JudgedAt     time.Time
}

// FormNoCurrentRouteJudgment 只在候选空间闭合且全部确定性淘汰时成立（`AT-NR-005`）：
// 「必须证明不存在尚未评估、证据未知或版本失配的可能候选」——一个证据未知的候选在场，
// 结论只能是未决，不是无路由；空候选空间分不清是排除还是装配失败，同样拒绝。
func FormNoCurrentRouteJudgment(spec NoCurrentRouteJudgmentSpec) (NoCurrentRouteJudgment, error) {
	if !spec.Key.MinimumIdentityEstablished() ||
		!spec.Strategy.valid() ||
		!spec.ViewRevision.valid() ||
		spec.JudgedAt.IsZero() {
		return NoCurrentRouteJudgment{}, ErrInvalidNoCurrentRoute
	}
	if len(spec.Candidates) == 0 {
		return NoCurrentRouteJudgment{}, ErrCandidateSpaceNotEstablished
	}
	seen := make(map[CandidateID]struct{}, len(spec.Candidates))
	for _, candidate := range spec.Candidates {
		if !candidate.id.valid() || !candidate.outcome.valid() {
			return NoCurrentRouteJudgment{}, ErrInvalidNoCurrentRoute
		}
		if _, duplicated := seen[candidate.id]; duplicated {
			return NoCurrentRouteJudgment{}, ErrDuplicateRouteCandidate
		}
		seen[candidate.id] = struct{}{}
		if candidate.outcome != CandidateEliminated {
			return NoCurrentRouteJudgment{}, ErrCandidateSpaceUndecided
		}
	}
	return NoCurrentRouteJudgment{
		key:          spec.Key,
		candidates:   append([]RouteCandidate(nil), spec.Candidates...),
		strategy:     spec.Strategy,
		viewRevision: spec.ViewRevision,
		judgedAt:     spec.JudgedAt.UTC(),
	}, nil
}

func (judgment NoCurrentRouteJudgment) Key() InitialRouteJudgmentKey {
	return judgment.key
}

// Candidates 交回逐候选淘汰依据的拷贝——`AT-NR-005`「保存逐候选淘汰依据；不创建默认
// 或占位路线」的可查半边。
func (judgment NoCurrentRouteJudgment) Candidates() []RouteCandidate {
	return append([]RouteCandidate(nil), judgment.candidates...)
}

func (judgment NoCurrentRouteJudgment) Strategy() RouteStrategyReference {
	return judgment.strategy
}

func (judgment NoCurrentRouteJudgment) ViewRevision() NetworkViewRevision {
	return judgment.viewRevision
}

func (judgment NoCurrentRouteJudgment) JudgedAt() time.Time {
	return judgment.judgedAt
}
