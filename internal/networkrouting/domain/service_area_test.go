package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func coveringResolution(t *testing.T, candidate string) domain.ServiceAreaResolution {
	t.Helper()
	resolution, err := domain.NewServiceAreaResolution(domain.ServiceAreaResolutionSpec{
		Candidate:   mustValue(t, domain.NewCandidateID, candidate),
		Outcome:     domain.AreaCoversDestination,
		AreaVersion: mustValue(t, domain.NewServiceAreaVersionReference, "AREA-V1"),
	})
	if err != nil {
		t.Fatalf("new covering resolution: %v", err)
	}
	return resolution
}

func excludingResolution(t *testing.T, candidate string) domain.ServiceAreaResolution {
	t.Helper()
	resolution, err := domain.NewServiceAreaResolution(domain.ServiceAreaResolutionSpec{
		Candidate:   mustValue(t, domain.NewCandidateID, candidate),
		Outcome:     domain.AreaExcludesDestination,
		AreaVersion: mustValue(t, domain.NewServiceAreaVersionReference, "AREA-V1"),
	})
	if err != nil {
		t.Fatalf("new excluding resolution: %v", err)
	}
	return resolution
}

func insufficientResolution(t *testing.T, candidate string) domain.ServiceAreaResolution {
	t.Helper()
	resolution, err := domain.NewServiceAreaResolution(domain.ServiceAreaResolutionSpec{
		Candidate: mustValue(t, domain.NewCandidateID, candidate),
		Outcome:   domain.AddressInformationInsufficient,
		Missing:   mustValue(t, domain.NewEvidenceGapReference, "ADDRESS_POSTCODE"),
		Reassess:  mustValue(t, domain.NewReassessmentCondition, "WHEN_ADDRESS_POSTCODE_PROVIDED"),
	})
	if err != nil {
		t.Fatalf("new insufficient resolution: %v", err)
	}
	return resolution
}

// Covers: 证据判定矩阵行 5/6 与 `AT-NR-023`「明确不覆盖 → 不可达而不是资料不足，保存服务
// 区域解析和排除依据」、`AT-NR-018`「缺地址业务信息 → 资料不足，指出缺少内容和再次判断
// 条件；不使用默认区域或记录不可达」——三种解析事实各折各格，排除的原因引用携带区域版本。
func TestServiceAreaEvaluationFoldsTheMatrixRows(t *testing.T) {
	candidates, gaps, err := domain.EvaluateServiceAreas([]domain.ServiceAreaResolution{
		coveringResolution(t, "cand-1"),
		excludingResolution(t, "cand-2"),
		insufficientResolution(t, "cand-3"),
	})
	if err != nil {
		t.Fatalf("evaluate service areas: %v", err)
	}

	if len(candidates) != 3 {
		t.Fatalf("candidates = %d, want 3——三种解析事实各占一席", len(candidates))
	}
	if candidates[0].Outcome() != domain.CandidateQualified {
		t.Fatalf("covered candidate = %q, want QUALIFIED", candidates[0].Outcome())
	}
	if candidates[1].Outcome() != domain.CandidateEliminated ||
		candidates[1].Reason().String() != "SERVICE_AREA_EXCLUDES_DESTINATION/AREA-V1" {
		t.Fatalf("excluded candidate = %q/%q; 排除必须带上区域版本作依据", candidates[1].Outcome(), candidates[1].Reason())
	}
	if candidates[2].Outcome() != domain.CandidateEvidenceUnknown {
		t.Fatalf("insufficient candidate = %q, want EVIDENCE_UNKNOWN——缺地址信息不是不可达", candidates[2].Outcome())
	}
	if len(gaps) != 1 || gaps[0].Reference().String() != "ADDRESS_POSTCODE" {
		t.Fatalf("gaps = %#v; 资料不足必须指出缺少内容", gaps)
	}
	if gaps[0].ReassessmentCondition().String() != "WHEN_ADDRESS_POSTCODE_PROVIDED" {
		t.Fatal("缺口没带再次判断条件——资料不足成了没有出口的否定")
	}

	// 端到端进三值矩阵：有合格候选即可达，别处的缺口推翻不了它（AT-NR-025 既有行为）。
	finding, err := domain.ConcludeReachability(candidates, gaps)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	if finding.Value() != domain.Reachable {
		t.Fatalf("finding = %q, want REACHABLE", finding.Value())
	}
}

// Covers: 同一矩阵的另两端——全部明确排除是`不可达`（确定性覆盖结论），全部资料不足是
// `资料不足`；两条链都从解析事实直通三值结果，中间没有人工补格。
func TestServiceAreaEvaluationDrivesTheThreeValuedConclusion(t *testing.T) {
	t.Run("all excluded is unreachable", func(t *testing.T) {
		candidates, gaps, err := domain.EvaluateServiceAreas([]domain.ServiceAreaResolution{
			excludingResolution(t, "cand-1"),
			excludingResolution(t, "cand-2"),
		})
		if err != nil {
			t.Fatalf("evaluate: %v", err)
		}
		finding, err := domain.ConcludeReachability(candidates, gaps)
		if err != nil {
			t.Fatalf("conclude: %v", err)
		}
		if finding.Value() != domain.Unreachable {
			t.Fatalf("finding = %q, want UNREACHABLE", finding.Value())
		}
	})

	t.Run("insufficient address stays insufficient", func(t *testing.T) {
		candidates, gaps, err := domain.EvaluateServiceAreas([]domain.ServiceAreaResolution{
			insufficientResolution(t, "cand-1"),
		})
		if err != nil {
			t.Fatalf("evaluate: %v", err)
		}
		finding, err := domain.ConcludeReachability(candidates, gaps)
		if err != nil {
			t.Fatalf("conclude: %v", err)
		}
		if finding.Value() != domain.InsufficientEvidence {
			t.Fatalf("finding = %q, want INSUFFICIENT_EVIDENCE——缺信息被记成了别的", finding.Value())
		}
	})
}

// Covers: 矩阵行 5「相关版本和解析依据完整」与行 6「不得假设默认区域」的构造期防线——
// 没有版本的排除、没有出口的资料不足、定论携带缺口字段，都表达不出来。
func TestAServiceAreaResolutionRefusesIncoherentShapes(t *testing.T) {
	cases := map[string]domain.ServiceAreaResolutionSpec{
		"exclusion without a version": {
			Candidate: mustValue(t, domain.NewCandidateID, "cand-1"),
			Outcome:   domain.AreaExcludesDestination,
		},
		"coverage without a version": {
			Candidate: mustValue(t, domain.NewCandidateID, "cand-1"),
			Outcome:   domain.AreaCoversDestination,
		},
		"insufficiency without a reassessment condition": {
			Candidate: mustValue(t, domain.NewCandidateID, "cand-1"),
			Outcome:   domain.AddressInformationInsufficient,
			Missing:   mustValue(t, domain.NewEvidenceGapReference, "ADDRESS_POSTCODE"),
		},
		"a verdict carrying gap fields": {
			Candidate:   mustValue(t, domain.NewCandidateID, "cand-1"),
			Outcome:     domain.AreaCoversDestination,
			AreaVersion: mustValue(t, domain.NewServiceAreaVersionReference, "AREA-V1"),
			Missing:     mustValue(t, domain.NewEvidenceGapReference, "ADDRESS_POSTCODE"),
		},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewServiceAreaResolution(spec); !errors.Is(err, domain.ErrInvalidServiceAreaResolution) {
				t.Fatalf("error = %v, want ErrInvalidServiceAreaResolution", err)
			}
		})
	}

	t.Run("two resolutions for one candidate are refused", func(t *testing.T) {
		_, _, err := domain.EvaluateServiceAreas([]domain.ServiceAreaResolution{
			coveringResolution(t, "cand-1"),
			excludingResolution(t, "cand-1"),
		})
		if !errors.Is(err, domain.ErrInvalidServiceAreaResolution) {
			t.Fatalf("error = %v, want ErrInvalidServiceAreaResolution——两条矛盾事实取哪条都是掷硬币", err)
		}
	})
}
