package domain

import (
	"errors"
	"time"
)

// 本文件是内部合规限制（CC CONTEXT「内部合规限制」，UC-CC-011，ADR-0334）：运营企业依据贸易合规规则形成的内部阻断决定，带限制责任来源、
// 依据、覆盖项与解除条件，只有原限制责任来源追加的决定才改变它，可部分解除。它与 RegulatoryRestriction 是两种限制（ADR-0241）：那一种挂监管
// 决定、只凭责任来源接受的监管结果整条解除，没有责任来源、覆盖项与部分解除；这一种不挂监管决定，也不收监管结果引用。两者之间没有共用字段，
// 谁也不解除谁。

var (
	ErrInvalidInternalRestriction = errors.New("customs compliance: invalid internal compliance restriction")
	ErrInvalidRestrictionDecision = errors.New("customs compliance: invalid restriction decision")

	// ErrConditionNotEvidenced 说决定的其余要素都齐，只缺证据引用：解除要有条件满足的证据，确认要有确认依据（UC-CC-011「解除不成立」）。
	ErrConditionNotEvidenced = errors.New("customs compliance: the restriction decision names no evidence")

	// ErrNotTheResponsibleSource 说出具决定的来源不是限制的责任来源。限制只能由原责任来源确认或解除；系统管理员、商业批准、异常案件关闭、
	// 节点执行、运输交接、付款与外部放行在这里没有对应的构造入口，换不成一条决定。
	ErrNotTheResponsibleSource  = errors.New("customs compliance: the decision is not formed by the restriction's responsible source")
	ErrCoverageNotInRestriction = errors.New("customs compliance: the decision names a coverage item the restriction does not hold")
	ErrCoverageNotDecidable     = errors.New("customs compliance: the decision does not apply to the coverage item as it stands")
)

// InternalRestrictionID 是内部合规限制的标识。
type InternalRestrictionID struct{ requiredValue }

func NewInternalRestrictionID(value string) (InternalRestrictionID, error) {
	required, err := newRequiredValue("internal restriction ID", value)
	return InternalRestrictionID{required}, err
}

// RestrictionSource 是限制责任来源：对某项内部硬约束及其解除判断负有业务责任的来源主体（CC CONTEXT「限制责任来源」）。
type RestrictionSource struct{ requiredValue }

func NewRestrictionSource(value string) (RestrictionSource, error) {
	required, err := newRequiredValue("restriction source", value)
	return RestrictionSource{required}, err
}

// RestrictionBasis 引用限制的依据：适用的规则版本、合格来源事实或已登记的授权人工决定。
type RestrictionBasis struct{ requiredValue }

func NewRestrictionBasis(value string) (RestrictionBasis, error) {
	required, err := newRequiredValue("restriction basis", value)
	return RestrictionBasis{required}, err
}

// RestrictionReleaseCondition 引用限制的解除条件。条件满足只给解除提供依据，不改变限制当前适用性（CC CONTEXT「解除条件」）。
type RestrictionReleaseCondition struct{ requiredValue }

func NewRestrictionReleaseCondition(value string) (RestrictionReleaseCondition, error) {
	required, err := newRequiredValue("restriction release condition", value)
	return RestrictionReleaseCondition{required}, err
}

// RestrictionDecider 是实际形成限制或决定的一方；它可以与责任来源不同，代表来源时须有授权依据（RestrictionAuthority）。
type RestrictionDecider struct{ requiredValue }

func NewRestrictionDecider(value string) (RestrictionDecider, error) {
	required, err := newRequiredValue("restriction decider", value)
	return RestrictionDecider{required}, err
}

// RestrictionAuthority 引用实际决定方代表责任来源作决定的当前委派或授权。这里只保存引用，不核验委派是否仍有效。
type RestrictionAuthority struct{ requiredValue }

func NewRestrictionAuthority(value string) (RestrictionAuthority, error) {
	required, err := newRequiredValue("restriction authority", value)
	return RestrictionAuthority{required}, err
}

// RestrictionEvidence 引用决定的证据：解除条件满足的证据，或确认限制适用的依据。
type RestrictionEvidence struct{ requiredValue }

func NewRestrictionEvidence(value string) (RestrictionEvidence, error) {
	required, err := newRequiredValue("restriction evidence", value)
	return RestrictionEvidence{required}, err
}

// RestrictionDecisionID 是一条决定的标识，由请求方给出：同一标识同一内容重放答已有，异内容是冲突。
type RestrictionDecisionID struct{ requiredValue }

func NewRestrictionDecisionID(value string) (RestrictionDecisionID, error) {
	required, err := newRequiredValue("restriction decision ID", value)
	return RestrictionDecisionID{required}, err
}

// DecisionMode 是限制的决定方式：适用规则化判断自动形成，或授权人工决定。
type DecisionMode uint8

const (
	DecisionModeInvalid DecisionMode = iota
	RuleBasedDecision
	ManualDecision
)

func (mode DecisionMode) valid() bool {
	return mode >= RuleBasedDecision && mode <= ManualDecision
}

func (mode DecisionMode) String() string {
	switch mode {
	case RuleBasedDecision:
		return "RULE"
	case ManualDecision:
		return "MANUAL"
	default:
		return ""
	}
}

// CoverageStanding 是一个覆盖项开始时的适用性。待确认与冲突是来源如实写下的业务状态，不是技术没读到：前者说来源、规则、依据或范围
// 还没确认，后者说同一限制的适用依据无法裁决。两者既不阻断也不放行，准入对它们答未决（ADR-0241 决定三）。
type CoverageStanding uint8

const (
	CoverageStandingInvalid CoverageStanding = iota
	CoverageInForce
	CoverageUnconfirmed
	CoverageConflicting
)

func (standing CoverageStanding) valid() bool {
	return standing >= CoverageInForce && standing <= CoverageConflicting
}

func (standing CoverageStanding) String() string {
	switch standing {
	case CoverageInForce:
		return "IN_FORCE"
	case CoverageUnconfirmed:
		return "UNCONFIRMED"
	case CoverageConflicting:
		return "CONFLICTING"
	default:
		return ""
	}
}

// CoverageKey 指名一个覆盖项：一个明确范围上的一个受限动作。覆盖按范围加动作成立；同袋、同单、同班次或同客户不自动扩大。
type CoverageKey struct {
	Scope  DecisionScopeReference
	Action GuardedAction
}

func (key CoverageKey) valid() bool {
	return key.Scope.valid() && key.Action.valid()
}

// CoverageSpec 是一个覆盖项与它开始时的适用性。
type CoverageSpec struct {
	Scope    DecisionScopeReference
	Action   GuardedAction
	Standing CoverageStanding
}

func (spec CoverageSpec) key() CoverageKey {
	return CoverageKey{Scope: spec.Scope, Action: spec.Action}
}

// RestrictionDecisionKind 是限制责任来源追加的决定的种类。确认让限制变有效，解除让限制不再约束，结果相反，不并成一种。
type RestrictionDecisionKind uint8

const (
	RestrictionDecisionKindInvalid RestrictionDecisionKind = iota
	// ConfirmCoverage 把待确认或冲突的覆盖项变成有效：来源说它确实约束这个动作。来源判定它不约束，走解除。
	ConfirmCoverage
	// ReleaseCoverage 让覆盖项不再约束。可以只解除一部分覆盖项。
	ReleaseCoverage
)

func (kind RestrictionDecisionKind) valid() bool {
	return kind >= ConfirmCoverage && kind <= ReleaseCoverage
}

func (kind RestrictionDecisionKind) String() string {
	switch kind {
	case ConfirmCoverage:
		return "CONFIRM"
	case ReleaseCoverage:
		return "RELEASE"
	default:
		return ""
	}
}

// RestrictionDecisionSpec 是一条决定所需的全部输入。
type RestrictionDecisionSpec struct {
	ID        RestrictionDecisionID
	Kind      RestrictionDecisionKind
	Source    RestrictionSource
	Covers    []CoverageKey
	Evidence  RestrictionEvidence
	DecidedBy RestrictionDecider
	Authority RestrictionAuthority
	At        time.Time
}

// RestrictionDecision 是限制责任来源对限制自己的若干覆盖项形成的追加式决定：关联原限制的覆盖项、证据、决定方、授权与生效时间，
// 不删除原限制历史，也不作用于其他来源的限制。
type RestrictionDecision struct {
	id        RestrictionDecisionID
	kind      RestrictionDecisionKind
	source    RestrictionSource
	covers    []CoverageKey
	evidence  RestrictionEvidence
	decidedBy RestrictionDecider
	authority RestrictionAuthority
	at        time.Time
}

// NewRestrictionDecision 校验决定自身的形状；它是否适用于某条限制由 InternalRestriction.Decide 判。
func NewRestrictionDecision(spec RestrictionDecisionSpec) (RestrictionDecision, error) {
	if !spec.ID.valid() ||
		!spec.Kind.valid() ||
		!spec.Source.valid() ||
		!spec.DecidedBy.valid() ||
		!spec.Authority.valid() ||
		spec.At.IsZero() ||
		len(spec.Covers) == 0 {
		return RestrictionDecision{}, ErrInvalidRestrictionDecision
	}
	seen := make(map[CoverageKey]bool, len(spec.Covers))
	for _, key := range spec.Covers {
		if !key.valid() || seen[key] {
			return RestrictionDecision{}, ErrInvalidRestrictionDecision
		}
		seen[key] = true
	}
	if !spec.Evidence.valid() {
		return RestrictionDecision{}, ErrConditionNotEvidenced
	}
	return RestrictionDecision{
		id:        spec.ID,
		kind:      spec.Kind,
		source:    spec.Source,
		covers:    append([]CoverageKey(nil), spec.Covers...),
		evidence:  spec.Evidence,
		decidedBy: spec.DecidedBy,
		authority: spec.Authority,
		at:        spec.At.UTC(),
	}, nil
}

func (decision RestrictionDecision) ID() RestrictionDecisionID      { return decision.id }
func (decision RestrictionDecision) Kind() RestrictionDecisionKind  { return decision.kind }
func (decision RestrictionDecision) Source() RestrictionSource      { return decision.source }
func (decision RestrictionDecision) Evidence() RestrictionEvidence  { return decision.evidence }
func (decision RestrictionDecision) DecidedBy() RestrictionDecider  { return decision.decidedBy }
func (decision RestrictionDecision) Authority() RestrictionAuthority { return decision.authority }
func (decision RestrictionDecision) At() time.Time                  { return decision.at }

func (decision RestrictionDecision) Covers() []CoverageKey {
	return append([]CoverageKey(nil), decision.covers...)
}

func (decision RestrictionDecision) covers_(key CoverageKey) bool {
	for _, covered := range decision.covers {
		if covered == key {
			return true
		}
	}
	return false
}

// SameContentAs 报告两条决定除标识外是否同一内容（种类、来源、覆盖项集合、证据、决定方、授权、生效时间）：同标识同内容是重放，
// 异内容是冲突。
func (decision RestrictionDecision) SameContentAs(other RestrictionDecision) bool {
	if decision.kind != other.kind ||
		decision.source != other.source ||
		decision.evidence != other.evidence ||
		decision.decidedBy != other.decidedBy ||
		decision.authority != other.authority ||
		!decision.at.Equal(other.at) ||
		len(decision.covers) != len(other.covers) {
		return false
	}
	for _, key := range decision.covers {
		if !other.covers_(key) {
			return false
		}
	}
	return true
}

// InternalRestrictionSpec 是一份内部合规限制所需的全部输入。
type InternalRestrictionSpec struct {
	ID        InternalRestrictionID
	Source    RestrictionSource
	Basis     RestrictionBasis
	Mode      DecisionMode
	DecidedBy RestrictionDecider
	Condition RestrictionReleaseCondition
	Coverage  []CoverageSpec
	StartedAt time.Time
}

// InternalRestriction 是一份内部合规限制：限制头（建立后不改写）、逐项保存的覆盖项，以及限制责任来源追加的决定。限制本身没有「已解除」
// 之类可覆盖的状态——当前适用性每次由覆盖项开始时的状态与决定按动作时点派生（UC-CC-011 不得维护能覆盖限制、覆盖、解除的状态字段）。
type InternalRestriction struct {
	id        InternalRestrictionID
	source    RestrictionSource
	basis     RestrictionBasis
	mode      DecisionMode
	decidedBy RestrictionDecider
	condition RestrictionReleaseCondition
	startedAt time.Time
	coverage  []CoverageSpec
	decisions []RestrictionDecision
}

// EstablishInternalRestriction 形成一份内部合规限制。至少有一个覆盖项：没有范围或动作的请求不造「冻结全部」的占位限制。同一范围同一
// 动作的覆盖项不重。
func EstablishInternalRestriction(spec InternalRestrictionSpec) (InternalRestriction, error) {
	if !spec.ID.valid() ||
		!spec.Source.valid() ||
		!spec.Basis.valid() ||
		!spec.Mode.valid() ||
		!spec.DecidedBy.valid() ||
		!spec.Condition.valid() ||
		spec.StartedAt.IsZero() ||
		len(spec.Coverage) == 0 {
		return InternalRestriction{}, ErrInvalidInternalRestriction
	}
	seen := make(map[CoverageKey]bool, len(spec.Coverage))
	for _, item := range spec.Coverage {
		if !item.key().valid() || !item.Standing.valid() || seen[item.key()] {
			return InternalRestriction{}, ErrInvalidInternalRestriction
		}
		seen[item.key()] = true
	}
	return InternalRestriction{
		id:        spec.ID,
		source:    spec.Source,
		basis:     spec.Basis,
		mode:      spec.Mode,
		decidedBy: spec.DecidedBy,
		condition: spec.Condition,
		startedAt: spec.StartedAt.UTC(),
		coverage:  append([]CoverageSpec(nil), spec.Coverage...),
	}, nil
}

func (restriction InternalRestriction) ID() InternalRestrictionID             { return restriction.id }
func (restriction InternalRestriction) Source() RestrictionSource             { return restriction.source }
func (restriction InternalRestriction) Basis() RestrictionBasis               { return restriction.basis }
func (restriction InternalRestriction) Mode() DecisionMode                    { return restriction.mode }
func (restriction InternalRestriction) DecidedBy() RestrictionDecider         { return restriction.decidedBy }
func (restriction InternalRestriction) Condition() RestrictionReleaseCondition { return restriction.condition }
func (restriction InternalRestriction) StartedAt() time.Time                  { return restriction.startedAt }

// Coverage 交回覆盖项逐项开始时的状态（不含决定的影响）。
func (restriction InternalRestriction) Coverage() []CoverageSpec {
	return append([]CoverageSpec(nil), restriction.coverage...)
}

// Decisions 按接受次序交回已追加的决定。
func (restriction InternalRestriction) Decisions() []RestrictionDecision {
	return append([]RestrictionDecision(nil), restriction.decisions...)
}

// Decision 按决定标识取已追加的那条。
func (restriction InternalRestriction) Decision(id RestrictionDecisionID) (RestrictionDecision, bool) {
	for _, decision := range restriction.decisions {
		if decision.id == id {
			return decision, true
		}
	}
	return RestrictionDecision{}, false
}

// coverageItemState 汇总决定对一个覆盖项已有的影响。
type coverageItemState struct {
	confirmed bool
	released  bool
	lastAt    time.Time
}

func (restriction InternalRestriction) itemState(key CoverageKey) coverageItemState {
	var state coverageItemState
	for _, decision := range restriction.decisions {
		if !decision.covers_(key) {
			continue
		}
		switch decision.kind {
		case ConfirmCoverage:
			state.confirmed = true
		case ReleaseCoverage:
			state.released = true
		}
		if decision.at.After(state.lastAt) {
			state.lastAt = decision.at
		}
	}
	return state
}

// Decide 追加一条决定，返新值不改旧值。只有限制责任来源出具的决定成立；决定整条成立或整条不成立，夹带一个不适用的覆盖项不留下一半。
// 确认只作用于开始时待确认或冲突、尚未确认也未解除的覆盖项，解除只作用于尚未解除的覆盖项；决定时间不得早于限制开始，也不得早于所作用
// 覆盖项上一次决定。
func (restriction InternalRestriction) Decide(decision RestrictionDecision) (InternalRestriction, error) {
	if !restriction.id.valid() || !decision.id.valid() {
		return InternalRestriction{}, ErrInvalidRestrictionDecision
	}
	if decision.source != restriction.source {
		return InternalRestriction{}, ErrNotTheResponsibleSource
	}
	if _, recorded := restriction.Decision(decision.id); recorded || decision.at.Before(restriction.startedAt) {
		return InternalRestriction{}, ErrInvalidRestrictionDecision
	}
	for _, key := range decision.covers {
		item, held := restriction.coverageItem(key)
		if !held {
			return InternalRestriction{}, ErrCoverageNotInRestriction
		}
		state := restriction.itemState(key)
		if state.released || decision.at.Before(state.lastAt) {
			return InternalRestriction{}, ErrCoverageNotDecidable
		}
		if decision.kind == ConfirmCoverage && (item.Standing == CoverageInForce || state.confirmed) {
			return InternalRestriction{}, ErrCoverageNotDecidable
		}
	}
	decided := restriction
	decided.coverage = append([]CoverageSpec(nil), restriction.coverage...)
	decided.decisions = append(append([]RestrictionDecision(nil), restriction.decisions...), decision)
	return decided, nil
}

func (restriction InternalRestriction) coverageItem(key CoverageKey) (CoverageSpec, bool) {
	for _, item := range restriction.coverage {
		if item.key() == key {
			return item, true
		}
	}
	return CoverageSpec{}, false
}

// standingAt 说一个覆盖项在给定业务时点的适用性；第二个值为 false 即那一刻它不约束（限制尚未开始，或已解除）。确认从其生效时刻起有效
// 且不回溯，解除从其生效时刻起不再约束——动作时点早于解除的动作仍读到解除之前的状态，同监管限制按动作时点判（ADR-0206）。
func (restriction InternalRestriction) standingAt(item CoverageSpec, at time.Time) (CoverageStanding, bool) {
	if at.Before(restriction.startedAt) {
		return CoverageStandingInvalid, false
	}
	standing := item.Standing
	for _, decision := range restriction.decisions {
		if !decision.covers_(item.key()) || decision.at.After(at) {
			continue
		}
		switch decision.kind {
		case ReleaseCoverage:
			return CoverageStandingInvalid, false
		case ConfirmCoverage:
			standing = CoverageInForce
		}
	}
	return standing, true
}

// RestrictionApplicability 是对（动作、范围、动作时点）读册得出的适用性四格（ADR-0241 决定一）。
type RestrictionApplicability uint8

const (
	RestrictionApplicabilityInvalid RestrictionApplicability = iota
	// RestrictionNoneRecorded 是册上没有一条限制约束这个动作。它只说册上的事实，不是来源出具的「明确无内部限制」结论：那需要
	// 必须参与判断的限制来源清单，而清单没有登记（CC CONTEXT「不得按无记录……推导无内部限制」，ADR-0334 决定五）。
	RestrictionNoneRecorded
	RestrictionInForce
	RestrictionUnconfirmed
	RestrictionConflicting
)

func (applicability RestrictionApplicability) String() string {
	switch applicability {
	case RestrictionNoneRecorded:
		return "NONE_RECORDED"
	case RestrictionInForce:
		return "IN_FORCE"
	case RestrictionUnconfirmed:
		return "UNCONFIRMED"
	case RestrictionConflicting:
		return "CONFLICTING"
	default:
		return ""
	}
}

// InternalApplicability 是一次适用性判断：哪一格，以及各格所涉的限制标识。
type InternalApplicability struct {
	inForce     []InternalRestrictionID
	unconfirmed []InternalRestrictionID
	conflicting []InternalRestrictionID
}

// Applicability 取最重的一格：有效限制压冲突，冲突压待确认，三者都没有即册上无记录。
func (judged InternalApplicability) Applicability() RestrictionApplicability {
	switch {
	case len(judged.inForce) > 0:
		return RestrictionInForce
	case len(judged.conflicting) > 0:
		return RestrictionConflicting
	case len(judged.unconfirmed) > 0:
		return RestrictionUnconfirmed
	default:
		return RestrictionNoneRecorded
	}
}

func (judged InternalApplicability) InForce() []InternalRestrictionID {
	return append([]InternalRestrictionID(nil), judged.inForce...)
}

func (judged InternalApplicability) Unconfirmed() []InternalRestrictionID {
	return append([]InternalRestrictionID(nil), judged.unconfirmed...)
}

func (judged InternalApplicability) Conflicting() []InternalRestrictionID {
	return append([]InternalRestrictionID(nil), judged.conflicting...)
}

// JudgeInternalApplicability 对一个方向性动作、一个明确范围和一个动作时点读内部合规限制：范围不合或不约束此动作的覆盖项不参与，
// 每格列出所涉的全部限制——处置者要知道等谁，一个都不能少。
func JudgeInternalApplicability(
	action GuardedAction,
	scope DecisionScopeReference,
	restrictions []InternalRestriction,
	at time.Time,
) (InternalApplicability, error) {
	if !action.valid() || !scope.valid() || at.IsZero() {
		return InternalApplicability{}, ErrInvalidAdmissibility
	}
	wanted := CoverageKey{Scope: scope, Action: action}
	var judged InternalApplicability
	for _, restriction := range restrictions {
		item, held := restriction.coverageItem(wanted)
		if !held {
			continue
		}
		standing, effective := restriction.standingAt(item, at)
		if !effective {
			continue
		}
		switch standing {
		case CoverageInForce:
			judged.inForce = append(judged.inForce, restriction.id)
		case CoverageUnconfirmed:
			judged.unconfirmed = append(judged.unconfirmed, restriction.id)
		case CoverageConflicting:
			judged.conflicting = append(judged.conflicting, restriction.id)
		}
	}
	return judged, nil
}
