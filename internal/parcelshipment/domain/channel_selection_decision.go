package domain

import (
	"errors"
	"time"
)

// 本文件是「渠道择优决定」记录（票 `label-channel/14` 裁决，MCP-3 2026-09-04）。
//
// 它是本上下文自己形成的一次**判断结果的记录**，与实际承运商判断（ADR-0103）、权威交接判断同属
// 「本上下文形成、只追加」一族：随每次择优各成一条，重跑形成新记录不改旧记录，同一对象多条记录
// 按决定时刻构成择优历史。它不是来源事实（ADR-0005 意义上的外部发生），也不是可丢弃的过程日志。
//
// 为什么今天就立：`PAR-NET-16` 已确认的机制句「日常择优可在已确认候选集合内自动进行并逐次留痕」；
// CONTEXT-MAP「未被选中的候选评价继续有效并留痕」由 `parcel-pricing` 不删评价保证的是**评价**那半，
// 择优决定本体（这一次、对这一票、在这些候选里、按这条规则、选了谁、谁出局及为何）此前算完即散。
// 留痕要求与逐票样本（留多久、给谁看）在登记册里待提供，那是实例半边；决定记录的**形状**由机制句
// 推得出，现在就做。
//
// **只引用，不拷贝**：候选与评价都只记引用，不拷金额、不拷评价内容——金额留在 `parcel-pricing` 的
// 评价上，本记录答的是「谁赢谁出局及为何」，不是「多少钱」。

// ErrInvalidChannelSelectionDecision 说决定记录的形状立不起来：候选集为空、同一候选重复、时刻
// 或引用缺席。它只关形状——择优本身的四种出口（选出、无人参选、并列、币种不齐）由比较器交出，
// 本记录照实记，不重判。
var ErrInvalidChannelSelectionDecision = errors.New("parcel shipment: invalid channel selection decision")

// ChannelSelectionDecisionID 是一条决定记录的身份。由编排经签发口铸，不从内容派生：同一票、同一
// 批候选重跑两次是两条记录，它们的内容可以逐字相同。
type ChannelSelectionDecisionID struct{ requiredValue }

func NewChannelSelectionDecisionID(value string) (ChannelSelectionDecisionID, error) {
	required, err := newRequiredValue("channel selection decision ID", value)
	return ChannelSelectionDecisionID{required}, err
}

// ChannelCostEvaluationReference 指认一个候选的成本取值来自 `parcel-pricing` 的哪一份 `BUY` 评价。
// 只引用：评价的金额、清单与解释都在那边，本上下文一列不复制。
type ChannelCostEvaluationReference struct{ requiredValue }

func NewChannelCostEvaluationReference(value string) (ChannelCostEvaluationReference, error) {
	required, err := newRequiredValue("channel cost evaluation reference", value)
	return ChannelCostEvaluationReference{required}, err
}

// ChannelSelectionSubject 是被择优的对象引用：在哪个商业范围下、按哪笔产品—渠道映射。它照择优编排
// 的入参取（裁决「按票 12 择优编排的入参取」）——今天编排的入参里没有面单交易或包裹的引用，
// 这里就不凭空造一个；那两者进入参那天，本引用随之加格，旧记录这一格读回为缺席。
type ChannelSelectionSubject struct {
	scope   CommercialScopeReference
	mapping ProductChannelMappingReference
}

func NewChannelSelectionSubject(
	scope CommercialScopeReference,
	mapping ProductChannelMappingReference,
) (ChannelSelectionSubject, error) {
	if !scope.valid() || !mapping.valid() {
		return ChannelSelectionSubject{}, ErrInvalidChannelSelectionDecision
	}
	return ChannelSelectionSubject{scope: scope, mapping: mapping}, nil
}

func (subject ChannelSelectionSubject) Scope() CommercialScopeReference { return subject.scope }
func (subject ChannelSelectionSubject) Mapping() ProductChannelMappingReference {
	return subject.mapping
}
func (subject ChannelSelectionSubject) valid() bool {
	return subject.scope.valid() && subject.mapping.valid()
}

// ChannelSelectionRule 是采用的择优规则引用。首发只有成本单维（`PAR-NET-16`），仍然要写——它是日后
// 多维时的版本锚：两条记录规则不同，结果就不该拿来互相比。
type ChannelSelectionRule string

const (
	// ChannelSelectionByCostOnly 即 SelectChannelCandidateByCost 那条规则：同币种内按成本单维取最低，
	// 最低并列为冲突。
	ChannelSelectionByCostOnly ChannelSelectionRule = "COST_ONLY"
)

func (rule ChannelSelectionRule) String() string { return string(rule) }

func (rule ChannelSelectionRule) valid() bool { return rule == ChannelSelectionByCostOnly }

// ChannelCandidateOutcome 是逐候选结果的封闭四格（裁决原文）。
type ChannelCandidateOutcome uint8

const (
	ChannelCandidateOutcomeInvalid ChannelCandidateOutcome = iota
	// ChannelCandidateSelected：这一次选的就是它。一条记录里至多一个。
	ChannelCandidateSelected
	// ChannelCandidateNotSelected：可计价，但不是最优。它与出局在「没赢」上同形，续办却不同——
	// 落选者什么都不必做，出局者要按因由续办。
	ChannelCandidateNotSelected
	// ChannelCandidateExcluded：成本没能确立，出局。必带因由（ChannelCostUnavailability 四格）。
	ChannelCandidateExcluded
	// ChannelCandidateTied：与另一候选在最低价那一格上并列，选不出唯一一条（`PAR-NET-16`：交人工
	// 裁决）。并列的每一家各记一条，整条记录因此没有选中者。
	ChannelCandidateTied
)

func (outcome ChannelCandidateOutcome) String() string {
	switch outcome {
	case ChannelCandidateSelected:
		return "SELECTED"
	case ChannelCandidateNotSelected:
		return "NOT_SELECTED"
	case ChannelCandidateExcluded:
		return "EXCLUDED"
	case ChannelCandidateTied:
		return "TIED"
	default:
		return ""
	}
}

func (outcome ChannelCandidateOutcome) valid() bool { return outcome.String() != "" }

// ChannelSelectionConclusion 是整条决定的结论三格，与比较器的三种非错误出口一一对应：选出唯一一条、
// 最低价并列、无人有资格参选。币种不齐不在此列——那一次没有比较发生，也就没有决定可记。
type ChannelSelectionConclusion uint8

const (
	ChannelSelectionConclusionInvalid ChannelSelectionConclusion = iota
	ChannelSelectionConcludedSelected
	ChannelSelectionConcludedTied
	ChannelSelectionConcludedNoneQualified
)

func (conclusion ChannelSelectionConclusion) String() string {
	switch conclusion {
	case ChannelSelectionConcludedSelected:
		return "SELECTED"
	case ChannelSelectionConcludedTied:
		return "TIED"
	case ChannelSelectionConcludedNoneQualified:
		return "NONE_QUALIFIED"
	default:
		return ""
	}
}

func (conclusion ChannelSelectionConclusion) valid() bool { return conclusion.String() != "" }

// ChannelCandidateResult 是一个候选在这一次择优里的结果：候选引用、（有则）所用评价引用、四格之一、
// 出局时的因由。
type ChannelCandidateResult struct {
	candidate  ChannelCandidateID
	evaluation ChannelCostEvaluationReference
	outcome    ChannelCandidateOutcome
	exclusion  ChannelCostUnavailability
}

func (result ChannelCandidateResult) Candidate() ChannelCandidateID { return result.candidate }

func (result ChannelCandidateResult) Outcome() ChannelCandidateOutcome { return result.outcome }

// Evaluation 交出所用评价引用，第二个返回值为 false 即缺席——没登记价卡的候选没经过评价。
func (result ChannelCandidateResult) Evaluation() (ChannelCostEvaluationReference, bool) {
	return result.evaluation, result.evaluation.valid()
}

// Exclusion 交出出局因由，第二个返回值为 false 即「不是出局」。
func (result ChannelCandidateResult) Exclusion() (ChannelCostUnavailability, bool) {
	if result.outcome != ChannelCandidateExcluded {
		return ChannelCostUnavailabilityInvalid, false
	}
	return result.exclusion, true
}

// valid 是单条结果的形状：出局必带因由、非出局不得带因由。
func (result ChannelCandidateResult) valid() bool {
	if !result.candidate.valid() || !result.outcome.valid() {
		return false
	}
	if result.outcome == ChannelCandidateExcluded {
		return result.exclusion.valid()
	}
	return result.exclusion == ChannelCostUnavailabilityInvalid
}

// ChannelSelectionDecision 是一条决定记录。字段一律不导出、没有任何修改方法：它落库后就是历史，
// 重跑择优铸新记录。
type ChannelSelectionDecision struct {
	id            ChannelSelectionDecisionID
	tenant        TenantID
	subject       ChannelSelectionSubject
	assembledAsOf time.Time
	rule          ChannelSelectionRule
	decidedAt     time.Time
	conclusion    ChannelSelectionConclusion
	results       []ChannelCandidateResult
}

// ChannelSelectionDecisionSpec 是形成一条决定记录所需的输入：身份、租户、对象引用、装配时点、
// 决定时刻，以及比较器刚刚比过的那一批成本取值。逐候选的四格由本文件从这批取值判出，调用方不给
// ——给了就有第二个口径。
type ChannelSelectionDecisionSpec struct {
	ID            ChannelSelectionDecisionID
	Tenant        TenantID
	Subject       ChannelSelectionSubject
	AssembledAsOf time.Time
	DecidedAt     time.Time
	Costs         []ChannelCandidateCost
}

// FormChannelSelectionDecision 把一次择优记成一条决定。逐候选四格与比较器 SelectChannelCandidateByCost
// 出自同一个排序（rankChannelCandidatesByCost）：记录里说「选中」的，就是比较器交回的那一个；两处
// 若各排一遍，迟早在某个边界上各说各话。
//
// 币种不齐时交回 ErrChannelCostCurrencyMismatch 而不成记录——那一次没有比较发生。
func FormChannelSelectionDecision(spec ChannelSelectionDecisionSpec) (ChannelSelectionDecision, error) {
	if !spec.ID.valid() || !spec.Tenant.valid() || !spec.Subject.valid() ||
		spec.AssembledAsOf.IsZero() || spec.DecidedAt.IsZero() || len(spec.Costs) == 0 {
		return ChannelSelectionDecision{}, ErrInvalidChannelSelectionDecision
	}
	seen := make(map[ChannelCandidateID]struct{}, len(spec.Costs))
	for _, cost := range spec.Costs {
		if !cost.candidate.valid() {
			return ChannelSelectionDecision{}, ErrInvalidChannelSelectionDecision
		}
		if _, duplicate := seen[cost.candidate]; duplicate {
			return ChannelSelectionDecision{}, ErrInvalidChannelSelectionDecision
		}
		seen[cost.candidate] = struct{}{}
	}

	ranking, err := rankChannelCandidatesByCost(spec.Costs)
	if err != nil {
		return ChannelSelectionDecision{}, err
	}

	results := make([]ChannelCandidateResult, 0, len(spec.Costs))
	for _, cost := range spec.Costs {
		result := ChannelCandidateResult{candidate: cost.candidate, evaluation: cost.evaluation}
		if grade, unavailable := cost.Unavailability(); unavailable {
			result.outcome, result.exclusion = ChannelCandidateExcluded, grade
		} else {
			result.outcome = ranking.outcomeOf(cost)
		}
		results = append(results, result)
	}

	return ChannelSelectionDecision{
		id:            spec.ID,
		tenant:        spec.Tenant,
		subject:       spec.Subject,
		assembledAsOf: spec.AssembledAsOf.UTC(),
		rule:          ChannelSelectionByCostOnly,
		decidedAt:     spec.DecidedAt.UTC(),
		conclusion:    ranking.conclusion(),
		results:       results,
	}, nil
}

func (decision ChannelSelectionDecision) ID() ChannelSelectionDecisionID   { return decision.id }
func (decision ChannelSelectionDecision) Tenant() TenantID                 { return decision.tenant }
func (decision ChannelSelectionDecision) Subject() ChannelSelectionSubject { return decision.subject }
func (decision ChannelSelectionDecision) AssembledAsOf() time.Time         { return decision.assembledAsOf }
func (decision ChannelSelectionDecision) Rule() ChannelSelectionRule       { return decision.rule }
func (decision ChannelSelectionDecision) DecidedAt() time.Time             { return decision.decidedAt }
func (decision ChannelSelectionDecision) Conclusion() ChannelSelectionConclusion {
	return decision.conclusion
}

// Results 按候选进入比较时的次序交出逐候选结果的副本。
func (decision ChannelSelectionDecision) Results() []ChannelCandidateResult {
	return append([]ChannelCandidateResult(nil), decision.results...)
}

// Selected 交出选中者，第二个返回值为 false 即这一次没有选中者（并列冲突或无人参选）。
func (decision ChannelSelectionDecision) Selected() (ChannelCandidateID, bool) {
	for _, result := range decision.results {
		if result.outcome == ChannelCandidateSelected {
			return result.candidate, true
		}
	}
	return ChannelCandidateID{}, false
}

// valid 是整条记录的跨字段一致性（ADR-0028 要求把「合法状态」写成显式命题）：选中至多一个且与并列
// 互斥；结论与逐候选结果互证——SELECTED 恰有一个选中、TIED 至少两个并列且无选中、NONE_QUALIFIED
// 全部出局。
func (decision ChannelSelectionDecision) valid() bool {
	if !decision.id.valid() || !decision.tenant.valid() || !decision.subject.valid() ||
		decision.assembledAsOf.IsZero() || !decision.rule.valid() || decision.decidedAt.IsZero() ||
		!decision.conclusion.valid() || len(decision.results) == 0 {
		return false
	}
	var selected, tied, excluded int
	seen := make(map[ChannelCandidateID]struct{}, len(decision.results))
	for _, result := range decision.results {
		if !result.valid() {
			return false
		}
		if _, duplicate := seen[result.candidate]; duplicate {
			return false
		}
		seen[result.candidate] = struct{}{}
		switch result.outcome {
		case ChannelCandidateSelected:
			selected++
		case ChannelCandidateTied:
			tied++
		case ChannelCandidateExcluded:
			excluded++
		}
	}
	switch decision.conclusion {
	case ChannelSelectionConcludedSelected:
		return selected == 1 && tied == 0
	case ChannelSelectionConcludedTied:
		return selected == 0 && tied >= 2
	case ChannelSelectionConcludedNoneQualified:
		return selected == 0 && tied == 0 && excluded == len(decision.results)
	default:
		return false
	}
}
