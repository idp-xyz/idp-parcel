package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func restrictionApplies(t *testing.T, candidate, restriction string) domain.HardConstraintFinding {
	t.Helper()
	finding, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
		Candidate:   mustValue(t, domain.NewCandidateID, candidate),
		Outcome:     domain.RestrictionApplies,
		Restriction: mustValue(t, domain.NewRestrictionReference, restriction),
	})
	if err != nil {
		t.Fatalf("new restriction finding: %v", err)
	}
	return finding
}

func constraintSatisfied(t *testing.T, candidate string) domain.HardConstraintFinding {
	t.Helper()
	finding, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
		Candidate: mustValue(t, domain.NewCandidateID, candidate),
		Outcome:   domain.ConstraintSatisfied,
	})
	if err != nil {
		t.Fatalf("new satisfied finding: %v", err)
	}
	return finding
}

func constraintUnknown(t *testing.T, candidate string) domain.HardConstraintFinding {
	t.Helper()
	finding, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
		Candidate: mustValue(t, domain.NewCandidateID, candidate),
		Outcome:   domain.ConstraintStatusUnknown,
		Missing:   mustValue(t, domain.NewEvidenceGapReference, "DANGEROUS_GOODS_CLASSIFICATION"),
		Reassess:  mustValue(t, domain.NewReassessmentCondition, "WHEN_GOODS_CLASSIFICATION_CONFIRMED"),
	})
	if err != nil {
		t.Fatalf("new unknown finding: %v", err)
	}
	return finding
}

// Covers: `AT-NR-031`「关务权威依据淘汰一个候选，另一个候选仍完整满足全部约束——形成
// 可达并保存两条候选各自依据；不绕过关务限制」。淘汰只落在受限那条候选上，原因指得回
// 限制来源责任方（限制只能由它解除）。
func TestACustomsRestrictionEliminatesOnlyItsCandidate(t *testing.T) {
	candidates := evaluatedCandidates(t,
		coveringResolution(t, "cand-1"),
		coveringResolution(t, "cand-2"),
	)

	evaluated, gaps, err := domain.EvaluateHardConstraints(candidates, []domain.HardConstraintFinding{
		restrictionApplies(t, "cand-1", "CUSTOMS-ELIGIBILITY/CC-REF-9"),
		constraintSatisfied(t, "cand-2"),
	})
	if err != nil {
		t.Fatalf("evaluate hard constraints: %v", err)
	}
	if len(gaps) != 0 {
		t.Fatalf("gaps = %#v; 确定性淘汰不产生缺口", gaps)
	}

	byID := make(map[string]domain.RouteCandidate, len(evaluated))
	for _, candidate := range evaluated {
		byID[candidate.ID().String()] = candidate
	}
	if byID["cand-1"].Outcome() != domain.CandidateEliminated ||
		byID["cand-1"].Reason().String() != "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-ELIGIBILITY/CC-REF-9" {
		t.Fatalf("cand-1 = %q/%q; 淘汰依据必须指名限制来源", byID["cand-1"].Outcome(), byID["cand-1"].Reason())
	}
	if byID["cand-2"].Outcome() != domain.CandidateQualified {
		t.Fatalf("cand-2 = %q; 另一条候选不受牵连", byID["cand-2"].Outcome())
	}

	finding, err := domain.ConcludeReachability(evaluated, gaps)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	if finding.Value() != domain.Reachable {
		t.Fatalf("finding = %q, want REACHABLE——两条候选各自依据都在", finding.Value())
	}
	if len(finding.EliminatedCandidates()) != 1 || len(finding.QualifiedCandidates()) != 1 {
		t.Fatal("判断没有同时保存淘汰依据与合格依据")
	}
}

// Covers: 候选评估层次 5「状态未知时资料不足」——唯一候选的限制状态未知降为`证据未知`
// 并留下候选级缺口（缺什么、何时再判），整体结论`资料不足`；不得记成不可达（矩阵：
// 不得仅依据已淘汰候选形成不可达，未知更不是淘汰）。
func TestAnUnknownConstraintStatusIsAGapNotAnElimination(t *testing.T) {
	candidates := evaluatedCandidates(t, coveringResolution(t, "cand-1"))

	evaluated, gaps, err := domain.EvaluateHardConstraints(candidates, []domain.HardConstraintFinding{
		constraintUnknown(t, "cand-1"),
	})
	if err != nil {
		t.Fatalf("evaluate hard constraints: %v", err)
	}

	if evaluated[0].Outcome() != domain.CandidateEvidenceUnknown {
		t.Fatalf("cand-1 = %q, want EVIDENCE_UNKNOWN", evaluated[0].Outcome())
	}
	if len(gaps) != 1 || gaps[0].Reference().String() != "DANGEROUS_GOODS_CLASSIFICATION" {
		t.Fatalf("gaps = %#v; 状态未知必须指出缺少内容", gaps)
	}
	if gaps[0].ReassessmentCondition().String() != "WHEN_GOODS_CLASSIFICATION_CONFIRMED" {
		t.Fatal("缺口没带再次判断条件")
	}

	finding, err := domain.ConcludeReachability(evaluated, gaps)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	if finding.Value() != domain.InsufficientEvidence {
		t.Fatalf("finding = %q, want INSUFFICIENT_EVIDENCE", finding.Value())
	}
}

// Covers: 已淘汰候选保持原依据——限制叠不上去（先到的区域排除仍是原因链头），状态未知
// 也翻不回来（不升不降，缺口照记作复算痕迹）。
func TestHardConstraintsNeverRewriteAnEliminatedCandidate(t *testing.T) {
	candidates := evaluatedCandidates(t,
		excludingResolution(t, "cand-1"),
		excludingResolution(t, "cand-2"),
	)

	evaluated, gaps, err := domain.EvaluateHardConstraints(candidates, []domain.HardConstraintFinding{
		restrictionApplies(t, "cand-1", "CUSTOMS-ELIGIBILITY/CC-REF-9"),
		constraintUnknown(t, "cand-2"),
	})
	if err != nil {
		t.Fatalf("evaluate hard constraints: %v", err)
	}

	for _, candidate := range evaluated {
		if candidate.Outcome() != domain.CandidateEliminated ||
			candidate.Reason().String() != "SERVICE_AREA_EXCLUDES_DESTINATION/AREA-V1" {
			t.Fatalf("candidate %s = %q/%q; 已淘汰候选被改写", candidate.ID(), candidate.Outcome(), candidate.Reason())
		}
	}
	if len(gaps) != 1 {
		t.Fatalf("gaps = %#v; 复算痕迹缺口仍应记录", gaps)
	}
}

// Covers: 层次 5 的构造与装配防线——三种结果各自的必备件与禁带件（定论与缺口不得同格），
// 指向候选空间之外的事实是装配错误而不是静默丢弃。
func TestAHardConstraintFindingRefusesIncoherentShapes(t *testing.T) {
	cases := map[string]domain.HardConstraintFindingSpec{
		"restriction without a source": {
			Candidate: mustValue(t, domain.NewCandidateID, "cand-1"),
			Outcome:   domain.RestrictionApplies,
		},
		"unknown without a reassessment condition": {
			Candidate: mustValue(t, domain.NewCandidateID, "cand-1"),
			Outcome:   domain.ConstraintStatusUnknown,
			Missing:   mustValue(t, domain.NewEvidenceGapReference, "DANGEROUS_GOODS_CLASSIFICATION"),
		},
		"satisfied carrying a restriction": {
			Candidate:   mustValue(t, domain.NewCandidateID, "cand-1"),
			Outcome:     domain.ConstraintSatisfied,
			Restriction: mustValue(t, domain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/CC-REF-9"),
		},
		"restriction carrying a gap": {
			Candidate:   mustValue(t, domain.NewCandidateID, "cand-1"),
			Outcome:     domain.RestrictionApplies,
			Restriction: mustValue(t, domain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/CC-REF-9"),
			Missing:     mustValue(t, domain.NewEvidenceGapReference, "DANGEROUS_GOODS_CLASSIFICATION"),
		},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewHardConstraintFinding(spec); !errors.Is(err, domain.ErrInvalidHardConstraintFinding) {
				t.Fatalf("err = %v, want ErrInvalidHardConstraintFinding", err)
			}
		})
	}

	_, _, err := domain.EvaluateHardConstraints(
		evaluatedCandidates(t, coveringResolution(t, "cand-1")),
		[]domain.HardConstraintFinding{restrictionApplies(t, "cand-9", "CUSTOMS-ELIGIBILITY/CC-REF-9")},
	)
	if !errors.Is(err, domain.ErrInvalidHardConstraintFinding) {
		t.Fatalf("err = %v, want ErrInvalidHardConstraintFinding（候选空间外的事实不得静默丢弃）", err)
	}
}
