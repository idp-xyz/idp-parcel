package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func projectionFor(t *testing.T, candidate string, latest time.Time) domain.CandidateTimeProjection {
	t.Helper()
	window, err := domain.NewPlannedTimeWindow(latest.Add(-6*time.Hour), latest,
		mustValue(t, domain.NewWindowBasisReference, "CALENDAR-V1/BUFFER-V1"))
	if err != nil {
		t.Fatalf("new window: %v", err)
	}
	projection, err := domain.NewCandidateTimeProjection(mustValue(t, domain.NewCandidateID, candidate), window)
	if err != nil {
		t.Fatalf("new projection: %v", err)
	}
	return projection
}

// Covers: UC-NR-001 层次 3「预计承诺、服务日历、截单、节点处理时间、衔接缓冲和计划窗口
// ——淘汰不满足候选」——投影最迟边界超过承诺边界的合格候选带承诺引用淘汰；赶得上的
// 原样通过；承诺没有时间边界时没有要求可淘汰。
func TestTimeFeasibilityEliminatesByTheCommittedBound(t *testing.T) {
	deadline := time.Date(2026, 8, 10, 18, 0, 0, 0, time.UTC)
	bound, err := domain.NewCommittedTimeBound(deadline,
		mustValue(t, domain.NewCommitmentReference, "commitment-1/v1"))
	if err != nil {
		t.Fatalf("new committed bound: %v", err)
	}
	candidates := []domain.RouteCandidate{
		qualifiedRouteCandidate(t, "cand-on-time"),
		qualifiedRouteCandidate(t, "cand-late"),
		eliminatedCandidate(t, "cand-banned", "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-1"),
	}
	projections := []domain.CandidateTimeProjection{
		projectionFor(t, "cand-on-time", deadline.Add(-2*time.Hour)),
		projectionFor(t, "cand-late", deadline.Add(3*time.Hour)),
	}

	evaluated, err := domain.EvaluateTimeFeasibility(candidates, projections, bound)
	if err != nil {
		t.Fatalf("evaluate time feasibility: %v", err)
	}

	byID := make(map[string]domain.RouteCandidate, len(evaluated))
	for _, candidate := range evaluated {
		byID[candidate.ID().String()] = candidate
	}
	if byID["cand-on-time"].Outcome() != domain.CandidateQualified {
		t.Fatalf("cand-on-time = %q; 赶得上的候选被改动了", byID["cand-on-time"].Outcome())
	}
	if byID["cand-late"].Outcome() != domain.CandidateEliminated ||
		byID["cand-late"].Reason().String() != "TIME_COMMITMENT_NOT_MET/commitment-1/v1" {
		t.Fatalf("cand-late = %q/%q; 超承诺的淘汰必须指得回那份承诺", byID["cand-late"].Outcome(), byID["cand-late"].Reason())
	}
	if byID["cand-banned"].Reason().String() != "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-1" {
		t.Fatal("先到的淘汰原因被时间可行性改写了")
	}

	unbounded, err := domain.EvaluateTimeFeasibility(candidates, projections, domain.CommittedTimeBound{})
	if err != nil {
		t.Fatalf("evaluate without a bound: %v", err)
	}
	for _, candidate := range unbounded {
		if candidate.ID().String() == "cand-late" && candidate.Outcome() != domain.CandidateQualified {
			t.Fatal("没有时间边界的承诺淘汰了候选——没有要求就没有不满足")
		}
	}
}

// Covers: 事实装配防线——合格候选缺投影不是满足是缺输入（静默放行等于用缺席冒充满足）；
// 同一候选两条矛盾投影拒收。
func TestTimeFeasibilityRefusesMissingOrContradictoryProjections(t *testing.T) {
	deadline := time.Date(2026, 8, 10, 18, 0, 0, 0, time.UTC)
	bound, err := domain.NewCommittedTimeBound(deadline,
		mustValue(t, domain.NewCommitmentReference, "commitment-1/v1"))
	if err != nil {
		t.Fatalf("new committed bound: %v", err)
	}
	candidates := []domain.RouteCandidate{qualifiedRouteCandidate(t, "cand-1")}

	if _, err := domain.EvaluateTimeFeasibility(candidates, nil, bound); !errors.Is(err, domain.ErrInvalidTimeFeasibility) {
		t.Fatalf("err = %v; 缺投影的比较不是比较", err)
	}

	contradictory := []domain.CandidateTimeProjection{
		projectionFor(t, "cand-1", deadline.Add(-2*time.Hour)),
		projectionFor(t, "cand-1", deadline.Add(2*time.Hour)),
	}
	if _, err := domain.EvaluateTimeFeasibility(candidates, contradictory, bound); !errors.Is(err, domain.ErrInvalidTimeFeasibility) {
		t.Fatalf("err = %v; 两条矛盾投影取哪条都是掷硬币", err)
	}
}
