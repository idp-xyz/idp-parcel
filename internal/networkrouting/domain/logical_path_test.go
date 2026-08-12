package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func executability(t *testing.T, candidate string, outcome domain.PathExecutabilityOutcome) domain.PathExecutability {
	t.Helper()
	finding, err := domain.NewPathExecutability(domain.PathExecutabilitySpec{
		Candidate: mustValue(t, domain.NewCandidateID, candidate),
		Outcome:   outcome,
		Schedule:  mustValue(t, domain.NewScheduleVersionReference, "SCHED-V3"),
	})
	if err != nil {
		t.Fatalf("new path executability: %v", err)
	}
	return finding
}

// Covers: 候选评估层次 4「网络连接、线路、服务日历、截单、临时可用性——形成可能候选并
// 排除不可执行路径」。不可执行的候选带日历/截单版本依据淘汰；可执行与无事实的候选原样
// 通过；唯一候选被排除时是确定性`不可达`，不是资料不足。
func TestPathExecutabilityExcludesWithItsScheduleBasis(t *testing.T) {
	candidates := evaluatedCandidates(t,
		coveringResolution(t, "cand-1"),
		coveringResolution(t, "cand-2"),
		coveringResolution(t, "cand-3"),
	)

	evaluated, err := domain.EvaluatePathExecutability(candidates, []domain.PathExecutability{
		executability(t, "cand-1", domain.PathNotExecutable),
		executability(t, "cand-2", domain.PathExecutable),
	})
	if err != nil {
		t.Fatalf("evaluate path executability: %v", err)
	}

	byID := make(map[string]domain.RouteCandidate, len(evaluated))
	for _, candidate := range evaluated {
		byID[candidate.ID().String()] = candidate
	}
	if byID["cand-1"].Outcome() != domain.CandidateEliminated ||
		byID["cand-1"].Reason().String() != "PATH_NOT_EXECUTABLE/SCHED-V3" {
		t.Fatalf("cand-1 = %q/%q; 排除不可执行路径必须携带日历/截单版本依据", byID["cand-1"].Outcome(), byID["cand-1"].Reason())
	}
	if byID["cand-2"].Outcome() != domain.CandidateQualified || byID["cand-3"].Outcome() != domain.CandidateQualified {
		t.Fatal("可执行与无事实的候选被改动了")
	}

	sole, err := domain.EvaluatePathExecutability(
		evaluatedCandidates(t, coveringResolution(t, "cand-1")),
		[]domain.PathExecutability{executability(t, "cand-1", domain.PathNotExecutable)},
	)
	if err != nil {
		t.Fatalf("evaluate sole candidate: %v", err)
	}
	finding, err := domain.ConcludeReachability(sole, nil)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	if finding.Value() != domain.Unreachable {
		t.Fatalf("finding = %q, want UNREACHABLE——带版本依据的排除是确定性淘汰", finding.Value())
	}
}

// Covers: 已淘汰候选保持先到的淘汰依据——区域排除在前，不可执行事实叠不上去；复算
// `不可达`时每条候选只有一个稳定原因链头。
func TestPathExecutabilityKeepsAnEarlierEliminationReason(t *testing.T) {
	candidates := evaluatedCandidates(t, excludingResolution(t, "cand-1"))

	evaluated, err := domain.EvaluatePathExecutability(candidates, []domain.PathExecutability{
		executability(t, "cand-1", domain.PathNotExecutable),
	})
	if err != nil {
		t.Fatalf("evaluate path executability: %v", err)
	}
	if evaluated[0].Reason().String() != "SERVICE_AREA_EXCLUDES_DESTINATION/AREA-V1" {
		t.Fatalf("reason = %q; 先到的淘汰依据被覆盖了", evaluated[0].Reason())
	}
}

// Covers: 层次 4 的构造与装配防线——可执行性没有「未知」格（网络日历读不到是矩阵行 7
// 的`未形成判断`，不是缺口），版本依据两个方向都必备；同一候选两条矛盾事实是装配错误。
func TestPathExecutabilityRefusesIncoherentShapes(t *testing.T) {
	if _, err := domain.NewPathExecutability(domain.PathExecutabilitySpec{
		Candidate: mustValue(t, domain.NewCandidateID, "cand-1"),
		Outcome:   domain.PathNotExecutable,
	}); !errors.Is(err, domain.ErrInvalidPathExecutability) {
		t.Fatalf("err = %v, want ErrInvalidPathExecutability（缺日历版本）", err)
	}
	if _, err := domain.NewPathExecutability(domain.PathExecutabilitySpec{
		Candidate: mustValue(t, domain.NewCandidateID, "cand-1"),
		Schedule:  mustValue(t, domain.NewScheduleVersionReference, "SCHED-V3"),
	}); !errors.Is(err, domain.ErrInvalidPathExecutability) {
		t.Fatalf("err = %v, want ErrInvalidPathExecutability（缺结果格）", err)
	}

	_, err := domain.EvaluatePathExecutability(
		evaluatedCandidates(t, coveringResolution(t, "cand-1")),
		[]domain.PathExecutability{
			executability(t, "cand-1", domain.PathExecutable),
			executability(t, "cand-1", domain.PathNotExecutable),
		},
	)
	if !errors.Is(err, domain.ErrInvalidPathExecutability) {
		t.Fatalf("err = %v, want ErrInvalidPathExecutability（矛盾事实取哪条都是掷硬币）", err)
	}
}
