// Package domain 承载 PN-08 试点治理的记录对象：候选版本组、阶段评审决定与生产权威
// 区间。它是产品级治理编排的机制半边，不是新的限界上下文——不建全局运输状态机，不
// 拥有任何源领域事实；对象里只有脱敏引用，真实客户、线路、伙伴与阈值留在参数登记册。
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBlankValue          = errors.New("pilot governance: blank value")
	ErrInvalidCandidateSet = errors.New("pilot governance: invalid candidate version set")
	ErrInvalidStageReview  = errors.New("pilot governance: invalid stage review decision")
	ErrInvalidInterval     = errors.New("pilot governance: invalid authority interval")
)

type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

func (value requiredValue) valid() bool {
	return strings.TrimSpace(value.value) != ""
}

// CandidateVersionSetID 是候选版本组的标识。
type CandidateVersionSetID struct{ requiredValue }

func NewCandidateVersionSetID(value string) (CandidateVersionSetID, error) {
	required, err := newRequiredValue("candidate version set ID", value)
	return CandidateVersionSetID{required}, err
}

// ScopeVersionReference 指名 pilotScopeVersion（脱敏引用——客户、法人、产品、线路、
// 伙伴等的引用组合，真实值在参数登记册）。
type ScopeVersionReference struct{ requiredValue }

func NewScopeVersionReference(value string) (ScopeVersionReference, error) {
	required, err := newRequiredValue("scope version reference", value)
	return ScopeVersionReference{required}, err
}

// ParameterSnapshotReference 指名 parameterSnapshot（适用 PAR-* 的 ID、修订与登记册
// 版本；不复制真实值）。
type ParameterSnapshotReference struct{ requiredValue }

func NewParameterSnapshotReference(value string) (ParameterSnapshotReference, error) {
	required, err := newRequiredValue("parameter snapshot reference", value)
	return ParameterSnapshotReference{required}, err
}

// RuleVersionsReference 指名 ruleAndContractVersions（商业、网络、关务、可见性、结算
// 规则与外部契约的实际采用版本）。
type RuleVersionsReference struct{ requiredValue }

func NewRuleVersionsReference(value string) (RuleVersionsReference, error) {
	required, err := newRequiredValue("rule versions reference", value)
	return RuleVersionsReference{required}, err
}

// CandidateVersionSet 是一次阶段评审固定的不可扩张候选版本组：三样引用一次进入，值
// 类型没有任何事后改写入口——「不可扩张」是结构性的；范围扩大或版本变更生成新的组，
// 不改这一组。
type CandidateVersionSet struct {
	id         CandidateVersionSetID
	scope      ScopeVersionReference
	parameters ParameterSnapshotReference
	rules      RuleVersionsReference
	formedAt   time.Time
}

func FixCandidateVersionSet(
	id CandidateVersionSetID,
	scope ScopeVersionReference,
	parameters ParameterSnapshotReference,
	rules RuleVersionsReference,
	formedAt time.Time,
) (CandidateVersionSet, error) {
	if !id.valid() || !scope.valid() || !parameters.valid() || !rules.valid() || formedAt.IsZero() {
		return CandidateVersionSet{}, ErrInvalidCandidateSet
	}
	return CandidateVersionSet{
		id:         id,
		scope:      scope,
		parameters: parameters,
		rules:      rules,
		formedAt:   formedAt.UTC(),
	}, nil
}

func (set CandidateVersionSet) ID() CandidateVersionSetID {
	return set.id
}

func (set CandidateVersionSet) Scope() ScopeVersionReference {
	return set.scope
}

func (set CandidateVersionSet) Parameters() ParameterSnapshotReference {
	return set.parameters
}

func (set CandidateVersionSet) Rules() RuleVersionsReference {
	return set.rules
}

func (set CandidateVersionSet) FormedAt() time.Time {
	return set.formedAt
}

// ExecutionStage 是当前执行阶段的封闭集合。`尚未进入执行阶段`独立一格：首次评审进入
// 历史回放时明确记录它，不虚构一个已在跑的阶段。
type ExecutionStage uint8

const (
	ExecutionStageInvalid ExecutionStage = iota
	NotYetInExecution
	HistoricalReplay
	ShadowRun
	LimitedProduction
)

func (stage ExecutionStage) valid() bool {
	return stage >= NotYetInExecution && stage <= LimitedProduction
}

func (stage ExecutionStage) String() string {
	switch stage {
	case NotYetInExecution:
		return "NOT_YET_IN_EXECUTION"
	case HistoricalReplay:
		return "HISTORICAL_REPLAY"
	case ShadowRun:
		return "SHADOW_RUN"
	case LimitedProduction:
		return "LIMITED_PRODUCTION"
	default:
		return ""
	}
}

// ReviewVerdict 是评审结论二值。
type ReviewVerdict uint8

const (
	ReviewVerdictInvalid ReviewVerdict = iota
	StageGo
	StageNoGo
)

func (verdict ReviewVerdict) String() string {
	switch verdict {
	case StageGo:
		return "GO"
	case StageNoGo:
		return "NO_GO"
	default:
		return ""
	}
}

// NoGoDisposition 是 No-Go 处理方式的封闭四值（交接硬句：「必须显式写明是保持当前
// 范围、暂停新准入、修复后重评还是对象级接管，不能由系统自动推断」）。
type NoGoDisposition uint8

const (
	NoGoDispositionInvalid NoGoDisposition = iota
	KeepCurrentScope
	SuspendNewAdmission
	FixAndReassess
	ObjectLevelTakeover
)

func (disposition NoGoDisposition) valid() bool {
	return disposition >= KeepCurrentScope && disposition <= ObjectLevelTakeover
}

func (disposition NoGoDisposition) String() string {
	switch disposition {
	case KeepCurrentScope:
		return "KEEP_CURRENT_SCOPE"
	case SuspendNewAdmission:
		return "SUSPEND_NEW_ADMISSION"
	case FixAndReassess:
		return "FIX_AND_REASSESS"
	case ObjectLevelTakeover:
		return "OBJECT_LEVEL_TAKEOVER"
	default:
		return ""
	}
}

// AcceptedDeviation 是随 Go 保留的非关键偏差：六件齐全才可保留（交接硬句逐项）。
// 风险接受不改变参数状态、不扩大范围、不覆盖历史——这里只是记录。
type AcceptedDeviation struct {
	Scope         string
	Control       string
	Owner         string
	CloseBy       time.Time
	ResidualRisk  string
	AcceptanceRef string
}

func (deviation AcceptedDeviation) complete() bool {
	return strings.TrimSpace(deviation.Scope) != "" &&
		strings.TrimSpace(deviation.Control) != "" &&
		strings.TrimSpace(deviation.Owner) != "" &&
		!deviation.CloseBy.IsZero() &&
		strings.TrimSpace(deviation.ResidualRisk) != "" &&
		strings.TrimSpace(deviation.AcceptanceRef) != ""
}

// StageReviewDecisionSpec 是一次阶段评审决定所需的全部输入。
type StageReviewDecisionSpec struct {
	Stage        ExecutionStage
	Objective    string
	Scope        ScopeVersionReference
	Candidates   CandidateVersionSetID
	EvidencePack string
	Verdict      ReviewVerdict
	Disposition  NoGoDisposition
	Deviations   []AcceptedDeviation
	DecidedBy    string
	DecidedAt    time.Time
	EffectiveAt  time.Time
}

// StageReviewDecision 是一次不可覆盖的阶段评审决定。Go 与 No-Go 各有形状约束：
// No-Go 必带显式处理方式且不得保留偏差（No-Go 没有「随 Go 保留」一说）；Go 不带
// 处理方式，保留的每件偏差六件齐全。
type StageReviewDecision struct {
	stage        ExecutionStage
	objective    string
	scope        ScopeVersionReference
	candidates   CandidateVersionSetID
	evidencePack string
	verdict      ReviewVerdict
	disposition  NoGoDisposition
	deviations   []AcceptedDeviation
	decidedBy    string
	decidedAt    time.Time
	effectiveAt  time.Time
}

func RecordStageReview(spec StageReviewDecisionSpec) (StageReviewDecision, error) {
	if !spec.Stage.valid() ||
		strings.TrimSpace(spec.Objective) == "" ||
		!spec.Scope.valid() ||
		!spec.Candidates.valid() ||
		strings.TrimSpace(spec.EvidencePack) == "" ||
		strings.TrimSpace(spec.DecidedBy) == "" ||
		spec.DecidedAt.IsZero() ||
		spec.EffectiveAt.IsZero() {
		return StageReviewDecision{}, ErrInvalidStageReview
	}
	switch spec.Verdict {
	case StageGo:
		if spec.Disposition != NoGoDispositionInvalid {
			return StageReviewDecision{}, ErrInvalidStageReview
		}
		for _, deviation := range spec.Deviations {
			if !deviation.complete() {
				return StageReviewDecision{}, ErrInvalidStageReview
			}
		}
	case StageNoGo:
		if !spec.Disposition.valid() || len(spec.Deviations) != 0 {
			return StageReviewDecision{}, ErrInvalidStageReview
		}
	default:
		return StageReviewDecision{}, ErrInvalidStageReview
	}
	return StageReviewDecision{
		stage:        spec.Stage,
		objective:    spec.Objective,
		scope:        spec.Scope,
		candidates:   spec.Candidates,
		evidencePack: spec.EvidencePack,
		verdict:      spec.Verdict,
		disposition:  spec.Disposition,
		deviations:   append([]AcceptedDeviation(nil), spec.Deviations...),
		decidedBy:    spec.DecidedBy,
		decidedAt:    spec.DecidedAt.UTC(),
		effectiveAt:  spec.EffectiveAt.UTC(),
	}, nil
}

func (decision StageReviewDecision) Stage() ExecutionStage {
	return decision.stage
}

func (decision StageReviewDecision) Verdict() ReviewVerdict {
	return decision.verdict
}

// Disposition 只在 No-Go 上给出。
func (decision StageReviewDecision) Disposition() (NoGoDisposition, bool) {
	return decision.disposition, decision.verdict == StageNoGo
}

func (decision StageReviewDecision) Candidates() CandidateVersionSetID {
	return decision.candidates
}

func (decision StageReviewDecision) Deviations() []AcceptedDeviation {
	return append([]AcceptedDeviation(nil), decision.deviations...)
}

func (decision StageReviewDecision) DecidedAt() time.Time {
	return decision.decidedAt
}

// Objective、Scope、EvidencePack、DecidedBy 与 EffectiveAt 是持久化重建的必需读口
// ——评审决定十一件里这五件没有出口，记录库连原样写回都做不到。
func (decision StageReviewDecision) Objective() string {
	return decision.objective
}

func (decision StageReviewDecision) Scope() ScopeVersionReference {
	return decision.scope
}

func (decision StageReviewDecision) EvidencePack() string {
	return decision.evidencePack
}

func (decision StageReviewDecision) DecidedBy() string {
	return decision.decidedBy
}

func (decision StageReviewDecision) EffectiveAt() time.Time {
	return decision.effectiveAt
}

// AuthorityInterval 是生产权威区间：对象范围 × 能力 × 事实类型在生效区间内的唯一
// 写入方。To 为零值表示开放区间（尚未关闭）。
type AuthorityInterval struct {
	ObjectScope string
	Capability  string
	FactKind    string
	Authority   string
	From        time.Time
	To          time.Time
}

func (interval AuthorityInterval) valid() bool {
	if strings.TrimSpace(interval.ObjectScope) == "" ||
		strings.TrimSpace(interval.Capability) == "" ||
		strings.TrimSpace(interval.FactKind) == "" ||
		strings.TrimSpace(interval.Authority) == "" ||
		interval.From.IsZero() {
		return false
	}
	return interval.To.IsZero() || interval.To.After(interval.From)
}

// overlaps 判断两区间是否在同一对象范围×能力×事实类型上时间重叠。不同权威方与相同
// 权威方一视同仁——同一维度两个区间本身就是记录错误，续办不该靠猜哪份是真的。
func (interval AuthorityInterval) overlaps(other AuthorityInterval) bool {
	if interval.ObjectScope != other.ObjectScope ||
		interval.Capability != other.Capability ||
		interval.FactKind != other.FactKind {
		return false
	}
	if !interval.To.IsZero() && !interval.To.After(other.From) {
		return false
	}
	if !other.To.IsZero() && !other.To.After(interval.From) {
		return false
	}
	return true
}

// AuthorityConflict 指名一对重叠区间。
type AuthorityConflict struct {
	First  AuthorityInterval
	Second AuthorityInterval
}

// DetectAuthorityConflicts 找出全部重叠区间对（交接失败场景：「生产权威区间重叠→
// 立即阻断受影响范围的新准入并保留冲突证据」，错误结果是「双写后靠人工对账消除
// 冲突」——冲突要在准入前被这张检查抓住）。任一区间形状不立即整体拒绝：形状坏的
// 区间参与比对只会漏报。
func DetectAuthorityConflicts(intervals []AuthorityInterval) ([]AuthorityConflict, error) {
	for _, interval := range intervals {
		if !interval.valid() {
			return nil, ErrInvalidInterval
		}
	}
	conflicts := make([]AuthorityConflict, 0)
	for left := 0; left < len(intervals); left++ {
		for right := left + 1; right < len(intervals); right++ {
			if intervals[left].overlaps(intervals[right]) {
				conflicts = append(conflicts, AuthorityConflict{
					First:  intervals[left],
					Second: intervals[right],
				})
			}
		}
	}
	return conflicts, nil
}
