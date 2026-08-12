package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func plainPreference(t *testing.T, satisfies ...string) domain.RouteRequirement {
	t.Helper()
	requirement, err := domain.NewRouteRequirement(domain.RouteRequirementSpec{
		Requirement: mustValue(t, domain.NewRouteRequirementReference, "PREFER_ROUTE_X"),
		Binding:     domain.PlainPreference,
		Satisfies:   candidateIDs(t, satisfies...),
	})
	if err != nil {
		t.Fatalf("new plain preference: %v", err)
	}
	return requirement
}

func committedRequirement(t *testing.T, satisfies ...string) domain.RouteRequirement {
	t.Helper()
	requirement, err := domain.NewRouteRequirement(domain.RouteRequirementSpec{
		Requirement: mustValue(t, domain.NewRouteRequirementReference, "REQUIRE_ROUTE_X"),
		Binding:     domain.CommittedRequirement,
		Basis:       mustValue(t, domain.NewCommitmentBasisReference, "CONTRACT-V7/ROUTE-X"),
		Satisfies:   candidateIDs(t, satisfies...),
	})
	if err != nil {
		t.Fatalf("new committed requirement: %v", err)
	}
	return requirement
}

func candidateIDs(t *testing.T, raw ...string) []domain.CandidateID {
	t.Helper()
	ids := make([]domain.CandidateID, 0, len(raw))
	for _, value := range raw {
		ids = append(ids, mustValue(t, domain.NewCandidateID, value))
	}
	return ids
}

func evaluatedCandidates(t *testing.T, resolutions ...domain.ServiceAreaResolution) []domain.RouteCandidate {
	t.Helper()
	candidates, _, err := domain.EvaluateServiceAreas(resolutions)
	if err != nil {
		t.Fatalf("evaluate service areas: %v", err)
	}
	return candidates
}

// Covers: `AT-NR-026`「客户偏好某线路，但产品和合同没有把它设为承诺——普通偏好不成为
// 硬约束，不使其他合格逻辑路径变成不可达」。偏好只指向 cand-1，cand-2 照样合格，结论
// 照样可达。
func TestAPlainPreferenceEliminatesNothing(t *testing.T) {
	candidates := evaluatedCandidates(t,
		coveringResolution(t, "cand-1"),
		coveringResolution(t, "cand-2"),
	)

	evaluated, err := domain.EvaluateRouteRequirements(candidates, []domain.RouteRequirement{
		plainPreference(t, "cand-1"),
	})
	if err != nil {
		t.Fatalf("evaluate route requirements: %v", err)
	}

	for _, candidate := range evaluated {
		if candidate.Outcome() != domain.CandidateQualified {
			t.Fatalf("candidate %s = %q; 普通偏好淘汰了候选", candidate.ID(), candidate.Outcome())
		}
	}
	finding, err := domain.ConcludeReachability(evaluated, nil)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	if finding.Value() != domain.Reachable {
		t.Fatalf("finding = %q, want REACHABLE——偏好不得使本来可达的包裹变成不可达", finding.Value())
	}
}

// Covers: 同一硬句的承诺半边——被产品或合同承诺的要求是硬约束，不满足它的候选确定性
// 淘汰且原因携带承诺依据；证据未知的候选同样被承诺排除（这一格的确定性不依赖缺失的
// 证据），其缺口留作复算痕迹但不再驱动结论。
func TestACommittedRequirementNarrowsWithItsBasis(t *testing.T) {
	candidates, gaps, err := domain.EvaluateServiceAreas([]domain.ServiceAreaResolution{
		coveringResolution(t, "cand-1"),
		coveringResolution(t, "cand-2"),
		insufficientResolution(t, "cand-3"),
		excludingResolution(t, "cand-4"),
	})
	if err != nil {
		t.Fatalf("evaluate service areas: %v", err)
	}

	evaluated, err := domain.EvaluateRouteRequirements(candidates, []domain.RouteRequirement{
		committedRequirement(t, "cand-1"),
	})
	if err != nil {
		t.Fatalf("evaluate route requirements: %v", err)
	}

	byID := make(map[string]domain.RouteCandidate, len(evaluated))
	for _, candidate := range evaluated {
		byID[candidate.ID().String()] = candidate
	}
	if byID["cand-1"].Outcome() != domain.CandidateQualified {
		t.Fatalf("cand-1 = %q, want QUALIFIED", byID["cand-1"].Outcome())
	}
	if byID["cand-2"].Outcome() != domain.CandidateEliminated ||
		byID["cand-2"].Reason().String() != "ROUTE_COMMITMENT_NOT_SATISFIED/CONTRACT-V7/ROUTE-X" {
		t.Fatalf("cand-2 = %q/%q; 承诺淘汰必须携带承诺依据", byID["cand-2"].Outcome(), byID["cand-2"].Reason())
	}
	if byID["cand-3"].Outcome() != domain.CandidateEliminated {
		t.Fatalf("cand-3 = %q; 被承诺排除的确定性不依赖缺失的证据", byID["cand-3"].Outcome())
	}
	if byID["cand-4"].Reason().String() != "SERVICE_AREA_EXCLUDES_DESTINATION/AREA-V1" {
		t.Fatalf("cand-4 reason = %q; 先到的淘汰依据不得被承诺原因覆盖", byID["cand-4"].Reason())
	}

	finding, err := domain.ConcludeReachability(evaluated, gaps)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	if finding.Value() != domain.Reachable {
		t.Fatalf("finding = %q, want REACHABLE——满足承诺的合格候选仍在", finding.Value())
	}
}

// Covers: 硬句「不得绕过硬限制制造可达结果」——被区域明确排除的候选即便满足承诺也不
// 复活，原淘汰原因不被覆盖；承诺只收窄候选空间。全部候选都出局时是诚实的不可达。
func TestACommitmentNeverResurrectsAnEliminatedCandidate(t *testing.T) {
	candidates := evaluatedCandidates(t,
		excludingResolution(t, "cand-1"),
		coveringResolution(t, "cand-2"),
	)

	evaluated, err := domain.EvaluateRouteRequirements(candidates, []domain.RouteRequirement{
		committedRequirement(t, "cand-1"),
	})
	if err != nil {
		t.Fatalf("evaluate route requirements: %v", err)
	}

	byID := make(map[string]domain.RouteCandidate, len(evaluated))
	for _, candidate := range evaluated {
		byID[candidate.ID().String()] = candidate
	}
	if byID["cand-1"].Outcome() != domain.CandidateEliminated ||
		byID["cand-1"].Reason().String() != "SERVICE_AREA_EXCLUDES_DESTINATION/AREA-V1" {
		t.Fatalf("cand-1 = %q/%q; 满足承诺不得复活已被硬限制淘汰的候选", byID["cand-1"].Outcome(), byID["cand-1"].Reason())
	}
	if byID["cand-2"].Outcome() != domain.CandidateEliminated {
		t.Fatalf("cand-2 = %q; 不满足承诺的候选应被淘汰", byID["cand-2"].Outcome())
	}

	finding, err := domain.ConcludeReachability(evaluated, nil)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	if finding.Value() != domain.Unreachable {
		t.Fatalf("finding = %q, want UNREACHABLE——全部候选各有确定性淘汰依据", finding.Value())
	}
}

// Covers: 承诺/偏好分界的构造期防线——承诺没有产品/合同依据、偏好却带依据、约束力缺格，
// 三种形状两种读法会给出相反的评估结果，必须构造即死。
func TestARouteRequirementRefusesIncoherentShapes(t *testing.T) {
	cases := map[string]domain.RouteRequirementSpec{
		"committed without a basis": {
			Requirement: mustValue(t, domain.NewRouteRequirementReference, "REQUIRE_ROUTE_X"),
			Binding:     domain.CommittedRequirement,
		},
		"plain preference carrying a basis": {
			Requirement: mustValue(t, domain.NewRouteRequirementReference, "PREFER_ROUTE_X"),
			Binding:     domain.PlainPreference,
			Basis:       mustValue(t, domain.NewCommitmentBasisReference, "CONTRACT-V7/ROUTE-X"),
		},
		"missing binding": {
			Requirement: mustValue(t, domain.NewRouteRequirementReference, "REQUIRE_ROUTE_X"),
		},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewRouteRequirement(spec); !errors.Is(err, domain.ErrInvalidRouteRequirement) {
				t.Fatalf("err = %v, want ErrInvalidRouteRequirement", err)
			}
		})
	}
}
