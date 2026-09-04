package domain

import (
	"errors"
	"time"
)

// ErrInvalidRehydratedChannelSelectionDecision 是渠道择优决定记录从持久化行重建失败的信号
// （ADR-0028 的纪律：重建要返回 error，库里读出的东西同样得过一遍不变量）。
var ErrInvalidRehydratedChannelSelectionDecision = errors.New("parcel shipment: rehydrated channel selection decision violates its invariants")

// ChannelCandidateResultSpec 是一行逐候选结果。Evaluation 以零值表示缺席；Exclusion 只在
// Outcome 为出局时有值——两者的成对关系由重建门核，不由行结构保证。
type ChannelCandidateResultSpec struct {
	Candidate  ChannelCandidateID
	Evaluation ChannelCostEvaluationReference
	Outcome    ChannelCandidateOutcome
	Exclusion  ChannelCostUnavailability
}

// RehydrateChannelSelectionDecisionSpec 是从行数据重建一条决定记录所需的全部字段。
//
// FormChannelSelectionDecision 不能兼职重建：它从成本取值**重判**四格，而行里存的是当时判出
// 的四格本身，金额早已不在（记录只引用不拷贝）——重建门以结果自证一致，不逆推形成过程
// （ADR-0028「重建只校验，不重算」）。
type RehydrateChannelSelectionDecisionSpec struct {
	ID            ChannelSelectionDecisionID
	Tenant        TenantID
	Subject       ChannelSelectionSubject
	AssembledAsOf time.Time
	Rule          ChannelSelectionRule
	DecidedAt     time.Time
	Conclusion    ChannelSelectionConclusion
	Results       []ChannelCandidateResultSpec
}

// RehydrateChannelSelectionDecision 把行数据收成一条决定记录，并把 ChannelSelectionDecision.valid
// 那组跨字段命题逐条过一遍：选中至多一个且与并列互斥、出局必带因由且非出局不带、结论与逐候选
// 结果互证、候选不重复。任何一条不成立即拒绝——一行「有选中也有并列」的记录与静默改写分不开。
func RehydrateChannelSelectionDecision(spec RehydrateChannelSelectionDecisionSpec) (ChannelSelectionDecision, error) {
	results := make([]ChannelCandidateResult, 0, len(spec.Results))
	for _, row := range spec.Results {
		results = append(results, ChannelCandidateResult{
			candidate:  row.Candidate,
			evaluation: row.Evaluation,
			outcome:    row.Outcome,
			exclusion:  row.Exclusion,
		})
	}
	decision := ChannelSelectionDecision{
		id:            spec.ID,
		tenant:        spec.Tenant,
		subject:       spec.Subject,
		assembledAsOf: spec.AssembledAsOf.UTC(),
		rule:          spec.Rule,
		decidedAt:     spec.DecidedAt.UTC(),
		conclusion:    spec.Conclusion,
		results:       results,
	}
	if !decision.valid() {
		return ChannelSelectionDecision{}, ErrInvalidRehydratedChannelSelectionDecision
	}
	return decision, nil
}
