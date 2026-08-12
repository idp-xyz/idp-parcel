package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func criterion(t *testing.T, name string) domain.RankingCriterion {
	t.Helper()
	return mustValue(t, domain.NewRankingCriterion, name)
}

func scoresFor(t *testing.T, candidate string, pairs map[string]int64) domain.CandidateScores {
	t.Helper()
	scores := make([]domain.CriterionScore, 0, len(pairs))
	for name, value := range pairs {
		score, err := domain.NewCriterionScore(criterion(t, name), value)
		if err != nil {
			t.Fatalf("new criterion score: %v", err)
		}
		scores = append(scores, score)
	}
	built, err := domain.NewCandidateScores(mustValue(t, domain.NewCandidateID, candidate), scores)
	if err != nil {
		t.Fatalf("new candidate scores: %v", err)
	}
	return built
}

// Covers: `AT-NR-001` 层次 4「按版本化策略规定的……优先级排序，选出排序最高的候选」——
// 比较按策略声明的准则序做字典序：第一准则分高下，平则看第二准则；被淘汰候选不参选。
// 权重与量纲折算属策略版本（PAR-NET-14），领域只比较不发明。
func TestSelectionFollowsTheStrategysDeclaredCriterionOrder(t *testing.T) {
	candidates := []domain.RouteCandidate{
		qualifiedRouteCandidate(t, "cand-fast"),
		qualifiedRouteCandidate(t, "cand-cheap"),
		eliminatedCandidate(t, "cand-banned", "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-1"),
	}
	scores := []domain.CandidateScores{
		scoresFor(t, "cand-fast", map[string]int64{"TIMELINESS": 1, "COST": 9}),
		scoresFor(t, "cand-cheap", map[string]int64{"TIMELINESS": 3, "COST": 2}),
		scoresFor(t, "cand-banned", map[string]int64{"TIMELINESS": 0, "COST": 0}),
	}

	timeFirst, err := domain.SelectRouteCandidate(candidates, scores,
		[]domain.RankingCriterion{criterion(t, "TIMELINESS"), criterion(t, "COST")})
	if err != nil {
		t.Fatalf("select (time first): %v", err)
	}
	if timeFirst.String() != "cand-fast" {
		t.Fatalf("selected = %s, want cand-fast——时效优先的策略序", timeFirst)
	}

	costFirst, err := domain.SelectRouteCandidate(candidates, scores,
		[]domain.RankingCriterion{criterion(t, "COST"), criterion(t, "TIMELINESS")})
	if err != nil {
		t.Fatalf("select (cost first): %v", err)
	}
	if costFirst.String() != "cand-cheap" {
		t.Fatalf("selected = %s, want cand-cheap——同一份事实换个策略序换个答案，权重不在领域", costFirst)
	}
}

// Covers: 排序必须全序——全部声明准则打平时按候选标识收尾，同一份输入两次判断给出同一个
// 计划（幂等硬句的排序半边）；没有合格候选是`无路由`或未决的信号，不是这里替策略作答。
func TestSelectionIsTotalAndRefusesToInventAnAnswer(t *testing.T) {
	tied := []domain.RouteCandidate{
		qualifiedRouteCandidate(t, "cand-b"),
		qualifiedRouteCandidate(t, "cand-a"),
	}
	tiedScores := []domain.CandidateScores{
		scoresFor(t, "cand-a", map[string]int64{"COST": 5}),
		scoresFor(t, "cand-b", map[string]int64{"COST": 5}),
	}
	first, err := domain.SelectRouteCandidate(tied, tiedScores, []domain.RankingCriterion{criterion(t, "COST")})
	if err != nil {
		t.Fatalf("select tied: %v", err)
	}
	second, err := domain.SelectRouteCandidate(tied, tiedScores, []domain.RankingCriterion{criterion(t, "COST")})
	if err != nil {
		t.Fatalf("select tied again: %v", err)
	}
	if first != second || first.String() != "cand-a" {
		t.Fatalf("selected %s then %s; 平局必须按稳定序收尾", first, second)
	}

	onlyEliminated := []domain.RouteCandidate{eliminatedCandidate(t, "cand-1", "PATH_NOT_EXECUTABLE/SCHED-V3")}
	if _, err := domain.SelectRouteCandidate(onlyEliminated, nil, []domain.RankingCriterion{criterion(t, "COST")}); !errors.Is(err, domain.ErrNoQualifiedCandidate) {
		t.Fatalf("err = %v, want ErrNoQualifiedCandidate", err)
	}

	missingScore := []domain.RouteCandidate{qualifiedRouteCandidate(t, "cand-1")}
	partial := []domain.CandidateScores{scoresFor(t, "cand-1", map[string]int64{"COST": 5})}
	if _, err := domain.SelectRouteCandidate(missingScore, partial,
		[]domain.RankingCriterion{criterion(t, "COST"), criterion(t, "TIMELINESS")}); !errors.Is(err, domain.ErrInvalidRanking) {
		t.Fatalf("err = %v; 缺一个声明准则取值的比较不是比较", err)
	}

	if _, err := domain.SelectRouteCandidate(missingScore, partial, nil); !errors.Is(err, domain.ErrInvalidRanking) {
		t.Fatalf("err = %v; 没有声明序领域不替策略排", err)
	}
}
