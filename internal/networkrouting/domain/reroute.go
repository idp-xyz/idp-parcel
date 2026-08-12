package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidReroute = errors.New("network routing: invalid reroute")
	// ErrRerouteBarred 是硬限制那条线的落点：自动和人工都不能绕过仍然适用的限制——
	// 未解除的硬限制在场时连「由授权角色决定」这条路都不存在。
	ErrRerouteBarred = errors.New("network routing: unresolved hard restrictions bar any reroute")
	// ErrAutomaticRerouteNotAllowed 说自动条件不成立：这时只能形成改路建议，由适用授权
	// 的运营角色决定。
	ErrAutomaticRerouteNotAllowed = errors.New("network routing: automatic reroute conditions are not met")
)

// ResponsibilityReference 指名一项未经所属上下文处理的既有责任（订舱、装载、申报……）。
// 责任对象属各自上下文，这里只引用。
type ResponsibilityReference struct{ requiredValue }

func NewResponsibilityReference(value string) (ResponsibilityReference, error) {
	required, err := newRequiredValue("responsibility reference", value)
	return ResponsibilityReference{required}, err
}

// AutoRerouteFacts 是自动改路四条件的事实输入。前三件由取数侧按策略版本、控制节点与
// 已执行前缀折成布尔（阈值与冻结边界的取值属 PAR-NET-14 实例半边），后两件是引用清单
// ——不满足时建议要说得出缺什么。
type AutoRerouteFacts struct {
	PolicyAllowsAutomatic       bool
	AtControlledNode            bool
	OnlyUnexecutedAffected      bool
	UnresolvedRestrictions      []RestrictionReference
	OutstandingResponsibilities []ResponsibilityReference
}

// RerouteAuthority 是自动条件判定的封闭三态。`禁行`单独一格：硬限制未解除时自动和人工
// 都不能绕过（CONTEXT 硬句），与「不满足自动条件、可由授权角色决定」是两条完全不同的
// 后续路。
type RerouteAuthority uint8

const (
	RerouteAuthorityInvalid RerouteAuthority = iota
	AutomaticRerouteAllowed
	SuggestionOnly
	RerouteBarred
)

func (authority RerouteAuthority) String() string {
	switch authority {
	case AutomaticRerouteAllowed:
		return "AUTOMATIC_ALLOWED"
	case SuggestionOnly:
		return "SUGGESTION_ONLY"
	case RerouteBarred:
		return "BARRED"
	default:
		return ""
	}
}

// EvaluateAutoRerouteConditions 执行 CONTEXT 的自动改路判定：「符合版本化策略、包裹位于
// 当前可控节点、只改变尚未执行部分，并且不存在未解除的硬限制或未经所属上下文处理的
// 既有责任时，可以自动形成改路决定。」未解除限制在场即禁行；其余任一不满足只形成建议。
// 返回的 blockers 是建议要携带的「为什么没自动」清单。
func EvaluateAutoRerouteConditions(facts AutoRerouteFacts) (RerouteAuthority, []string) {
	if len(facts.UnresolvedRestrictions) > 0 {
		blockers := make([]string, 0, len(facts.UnresolvedRestrictions))
		for _, restriction := range facts.UnresolvedRestrictions {
			blockers = append(blockers, "UNRESOLVED_RESTRICTION/"+restriction.String())
		}
		return RerouteBarred, blockers
	}

	blockers := make([]string, 0, 4)
	if !facts.PolicyAllowsAutomatic {
		blockers = append(blockers, "POLICY_DOES_NOT_ALLOW_AUTOMATIC")
	}
	if !facts.AtControlledNode {
		blockers = append(blockers, "NOT_AT_CONTROLLED_NODE")
	}
	if !facts.OnlyUnexecutedAffected {
		blockers = append(blockers, "EXECUTED_PREFIX_AFFECTED")
	}
	for _, responsibility := range facts.OutstandingResponsibilities {
		blockers = append(blockers, "OUTSTANDING_RESPONSIBILITY/"+responsibility.String())
	}
	if len(blockers) > 0 {
		return SuggestionOnly, blockers
	}
	return AutomaticRerouteAllowed, nil
}

// RerouteTriggerReference 指名触发本次改路的原因事实（连接关闭、错过截单、实测变化……）。
type RerouteTriggerReference struct{ requiredValue }

func NewRerouteTriggerReference(value string) (RerouteTriggerReference, error) {
	required, err := newRequiredValue("reroute trigger reference", value)
	return RerouteTriggerReference{required}, err
}

// AuthorizedRoleReference 指名形成人工改路决定的授权运营角色。授权目录属 party-commercial，
// 这里只引用实际决定方。
type AuthorizedRoleReference struct{ requiredValue }

func NewAuthorizedRoleReference(value string) (AuthorizedRoleReference, error) {
	required, err := newRequiredValue("authorized role reference", value)
	return AuthorizedRoleReference{required}, err
}

// RerouteSuggestion 是自动条件不成立时的候选处置意见。它是纯记录：建议本身不修改当前
// 有效路由（CONTEXT 语言），没有任何状态转移长在它身上。
type RerouteSuggestion struct {
	key         InitialRouteJudgmentKey
	trigger     RerouteTriggerReference
	candidates  []RouteCandidate
	blockers    []string
	suggestedAt time.Time
}

type RerouteSuggestionSpec struct {
	Key         InitialRouteJudgmentKey
	Trigger     RerouteTriggerReference
	Candidates  []RouteCandidate
	Blockers    []string
	SuggestedAt time.Time
}

// NewRerouteSuggestion 要求候选与「为什么没自动」清单都在场：一份说不出候选也说不出
// 阻塞原因的建议，授权角色无从据以决定。
func NewRerouteSuggestion(spec RerouteSuggestionSpec) (RerouteSuggestion, error) {
	if !spec.Key.MinimumIdentityEstablished() ||
		!spec.Trigger.valid() ||
		len(spec.Candidates) == 0 ||
		len(spec.Blockers) == 0 ||
		spec.SuggestedAt.IsZero() {
		return RerouteSuggestion{}, ErrInvalidReroute
	}
	for _, blocker := range spec.Blockers {
		if blocker == "" {
			return RerouteSuggestion{}, ErrInvalidReroute
		}
	}
	return RerouteSuggestion{
		key:         spec.Key,
		trigger:     spec.Trigger,
		candidates:  append([]RouteCandidate(nil), spec.Candidates...),
		blockers:    append([]string(nil), spec.Blockers...),
		suggestedAt: spec.SuggestedAt.UTC(),
	}, nil
}

func (suggestion RerouteSuggestion) Key() InitialRouteJudgmentKey {
	return suggestion.key
}

func (suggestion RerouteSuggestion) Trigger() RerouteTriggerReference {
	return suggestion.trigger
}

func (suggestion RerouteSuggestion) Candidates() []RouteCandidate {
	return append([]RouteCandidate(nil), suggestion.candidates...)
}

// Blockers 交回自动条件不满足的清单——建议为什么只是建议。
func (suggestion RerouteSuggestion) Blockers() []string {
	return append([]string(nil), suggestion.blockers...)
}

func (suggestion RerouteSuggestion) SuggestedAt() time.Time {
	return suggestion.suggestedAt
}

// RerouteDecisionMode 是决定方式的封闭二值：自动或授权角色。CONTEXT 要求决定保留决定
// 方式，审计要能答「这条路是谁改的」。
type RerouteDecisionMode uint8

const (
	RerouteDecisionModeInvalid RerouteDecisionMode = iota
	AutomaticReroute
	AuthorizedRoleReroute
)

func (mode RerouteDecisionMode) String() string {
	switch mode {
	case AutomaticReroute:
		return "AUTOMATIC"
	case AuthorizedRoleReroute:
		return "AUTHORIZED_ROLE"
	default:
		return ""
	}
}

// RerouteDecisionSpec 是形成一个改路决定所需的全部输入。
type RerouteDecisionSpec struct {
	Authority    RerouteAuthority
	Mode         RerouteDecisionMode
	DecidedBy    AuthorizedRoleReference
	Trigger      RerouteTriggerReference
	OriginalPlan RoutePlanVersionID
	NewPlan      InitialRoutePlan
	DecidedAt    time.Time
}

// RerouteDecision 是针对尚未执行旅程的新路由版本及其业务生效边界（CONTEXT 语言）。
// 原因、策略版本与输入依据由新计划本体携带（strategy/viewRevision），这里补决定方式
// 与替代关系。
type RerouteDecision struct {
	mode         RerouteDecisionMode
	decidedBy    AuthorizedRoleReference
	trigger      RerouteTriggerReference
	originalPlan RoutePlanVersionID
	newPlan      InitialRoutePlan
	decidedAt    time.Time
}

// FormRerouteDecision 立三道门：
//
//   - 权限门：`禁行`下自动和人工都立不成决定（人工同样不得绕过硬限制）；自动方式只在
//     自动条件全部成立时可用（不满足只形成建议）；人工方式必须指名授权角色。
//   - 替代关系门：新计划版本不得与原版本重号——「与原计划的替代关系」要求两代分得开；
//     原计划引用必备，没有替代关系的改路是凭空第二计划。
//   - 生效边界门：新计划从当前或下一可控节点生效——生效节点即新计划段链的首节点，由
//     计划本体的段链连续保证，这里校验决定时刻不缺。
func FormRerouteDecision(spec RerouteDecisionSpec) (RerouteDecision, error) {
	switch spec.Authority {
	case RerouteBarred:
		return RerouteDecision{}, ErrRerouteBarred
	case AutomaticRerouteAllowed, SuggestionOnly:
	default:
		return RerouteDecision{}, ErrInvalidReroute
	}
	switch spec.Mode {
	case AutomaticReroute:
		if spec.Authority != AutomaticRerouteAllowed {
			return RerouteDecision{}, ErrAutomaticRerouteNotAllowed
		}
	case AuthorizedRoleReroute:
		if !spec.DecidedBy.valid() {
			return RerouteDecision{}, ErrInvalidReroute
		}
	default:
		return RerouteDecision{}, ErrInvalidReroute
	}
	if !spec.Trigger.valid() ||
		!spec.OriginalPlan.valid() ||
		spec.DecidedAt.IsZero() {
		return RerouteDecision{}, ErrInvalidReroute
	}
	if spec.NewPlan.Version() == spec.OriginalPlan {
		return RerouteDecision{}, ErrInvalidReroute
	}
	if !spec.NewPlan.Version().valid() {
		return RerouteDecision{}, ErrInvalidReroute
	}
	return RerouteDecision{
		mode:         spec.Mode,
		decidedBy:    spec.DecidedBy,
		trigger:      spec.Trigger,
		originalPlan: spec.OriginalPlan,
		newPlan:      spec.NewPlan,
		decidedAt:    spec.DecidedAt.UTC(),
	}, nil
}

func (decision RerouteDecision) Mode() RerouteDecisionMode {
	return decision.mode
}

// DecidedBy 只在人工方式下给出。
func (decision RerouteDecision) DecidedBy() (AuthorizedRoleReference, bool) {
	return decision.decidedBy, decision.decidedBy.valid()
}

func (decision RerouteDecision) Trigger() RerouteTriggerReference {
	return decision.trigger
}

func (decision RerouteDecision) OriginalPlan() RoutePlanVersionID {
	return decision.originalPlan
}

func (decision RerouteDecision) NewPlan() InitialRoutePlan {
	return decision.newPlan
}

// EffectiveFromNode 交回生效边界：新计划段链的首节点，即当前或下一可控节点。
func (decision RerouteDecision) EffectiveFromNode() PlanNodeReference {
	return decision.newPlan.Nodes()[0]
}

func (decision RerouteDecision) DecidedAt() time.Time {
	return decision.decidedAt
}
