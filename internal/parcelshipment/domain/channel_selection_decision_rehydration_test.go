package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证渠道择优决定记录的重建门（ADR-0028：重建与构造分属两扇门，重建只校验不重算）。
// 门相信行数据，所以它要把「合法状态」写成显式命题：选中至多一个且与并列互斥、出局必带因由、
// 结论与逐候选结果互证。一行坏数据在这里暴露，不会变成一条看着合法的决定。

func rehydrationSpecOf(t *testing.T, decision domain.ChannelSelectionDecision) domain.RehydrateChannelSelectionDecisionSpec {
	t.Helper()

	spec := domain.RehydrateChannelSelectionDecisionSpec{
		ID:            decision.ID(),
		Tenant:        decision.Tenant(),
		Subject:       decision.Subject(),
		AssembledAsOf: decision.AssembledAsOf(),
		Rule:          decision.Rule(),
		DecidedAt:     decision.DecidedAt(),
		Conclusion:    decision.Conclusion(),
	}
	for _, result := range decision.Results() {
		row := domain.ChannelCandidateResultSpec{Candidate: result.Candidate(), Outcome: result.Outcome()}
		if evaluation, present := result.Evaluation(); present {
			row.Evaluation = evaluation
		}
		if grade, excluded := result.Exclusion(); excluded {
			row.Exclusion = grade
		}
		spec.Results = append(spec.Results, row)
	}
	return spec
}

// Covers: 形成的记录拆成行再重建，逐格相同——包括缺席的评价引用仍然缺席、并列与出局各自的格。
func TestARehydratedChannelSelectionDecisionMatchesTheOneThatWasFormed(t *testing.T) {
	t.Parallel()

	evaluated, err := pricedCost(t, "cand-a", "10.00").
		WithEvaluation(mustValue(t, domain.NewChannelCostEvaluationReference, "eval-a"))
	if err != nil {
		t.Fatalf("带评价引用：%v", err)
	}
	formed, err := domain.FormChannelSelectionDecision(selectionDecisionSpec(t,
		evaluated,
		pricedCost(t, "cand-b", "10.00"),
		unpriceableCost(t, "cand-c", domain.ChannelCostConflict),
	))
	if err != nil {
		t.Fatalf("形成决定记录：%v", err)
	}

	rebuilt, err := domain.RehydrateChannelSelectionDecision(rehydrationSpecOf(t, formed))
	if err != nil {
		t.Fatalf("重建：%v", err)
	}

	if rebuilt.ID() != formed.ID() || rebuilt.Tenant() != formed.Tenant() || rebuilt.Subject() != formed.Subject() ||
		!rebuilt.AssembledAsOf().Equal(formed.AssembledAsOf()) || rebuilt.Rule() != formed.Rule() ||
		!rebuilt.DecidedAt().Equal(formed.DecidedAt()) || rebuilt.Conclusion() != formed.Conclusion() {
		t.Fatalf("头部不等：\n重建 %+v\n形成 %+v", rebuilt, formed)
	}
	formedResults, rebuiltResults := formed.Results(), rebuilt.Results()
	if len(formedResults) != len(rebuiltResults) {
		t.Fatalf("结果条数 %d vs %d", len(rebuiltResults), len(formedResults))
	}
	for index := range formedResults {
		if formedResults[index] != rebuiltResults[index] {
			t.Fatalf("第 %d 条结果不等：重建 %+v 形成 %+v", index, rebuiltResults[index], formedResults[index])
		}
	}
}

// Covers: 门拒绝每一种在 FormChannelSelectionDecision 里造不出来、但一行坏数据能造出来的组合。
// 断言到哨兵，不断言到某条具体校验——门要挡的是「看着合法」，不是某一格。
func TestTheRehydrationGateRejectsEveryInconsistentDecisionRow(t *testing.T) {
	t.Parallel()

	formed, err := domain.FormChannelSelectionDecision(selectionDecisionSpec(t,
		pricedCost(t, "cand-a", "10.00"),
		pricedCost(t, "cand-b", "12.00"),
		unpriceableCost(t, "cand-c", domain.ChannelCostNotFormed),
	))
	if err != nil {
		t.Fatalf("形成决定记录：%v", err)
	}

	cases := map[string]func(spec *domain.RehydrateChannelSelectionDecisionSpec){
		"两个选中者": func(spec *domain.RehydrateChannelSelectionDecisionSpec) {
			spec.Results[1].Outcome = domain.ChannelCandidateSelected
		},
		"选中与并列同在": func(spec *domain.RehydrateChannelSelectionDecisionSpec) {
			spec.Results[1].Outcome = domain.ChannelCandidateTied
		},
		"出局却没有因由": func(spec *domain.RehydrateChannelSelectionDecisionSpec) {
			spec.Results[2].Exclusion = domain.ChannelCostUnavailabilityInvalid
		},
		"落选却带着因由": func(spec *domain.RehydrateChannelSelectionDecisionSpec) {
			spec.Results[1].Exclusion = domain.ChannelCostPendingEvidence
		},
		"结论说并列而结果里没有并列": func(spec *domain.RehydrateChannelSelectionDecisionSpec) {
			spec.Conclusion = domain.ChannelSelectionConcludedTied
		},
		"结论说无人参选而有人选中": func(spec *domain.RehydrateChannelSelectionDecisionSpec) {
			spec.Conclusion = domain.ChannelSelectionConcludedNoneQualified
		},
		"只有一个并列者": func(spec *domain.RehydrateChannelSelectionDecisionSpec) {
			spec.Results[0].Outcome = domain.ChannelCandidateTied
			spec.Conclusion = domain.ChannelSelectionConcludedTied
		},
		"同一候选两行": func(spec *domain.RehydrateChannelSelectionDecisionSpec) {
			spec.Results[1].Candidate = spec.Results[0].Candidate
			spec.Results[1].Outcome = domain.ChannelCandidateNotSelected
		},
		"没有结果行": func(spec *domain.RehydrateChannelSelectionDecisionSpec) { spec.Results = nil },
		"规则不在封闭集里": func(spec *domain.RehydrateChannelSelectionDecisionSpec) {
			spec.Rule = domain.ChannelSelectionRule("MULTI_CRITERIA")
		},
		"没有决定时刻": func(spec *domain.RehydrateChannelSelectionDecisionSpec) { spec.DecidedAt = time.Time{} },
		"没有装配时点": func(spec *domain.RehydrateChannelSelectionDecisionSpec) { spec.AssembledAsOf = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			spec := rehydrationSpecOf(t, formed)
			mutate(&spec)
			if _, err := domain.RehydrateChannelSelectionDecision(spec); !errors.Is(err, domain.ErrInvalidRehydratedChannelSelectionDecision) {
				t.Fatalf("err = %v，want %v", err, domain.ErrInvalidRehydratedChannelSelectionDecision)
			}
		})
	}
}
