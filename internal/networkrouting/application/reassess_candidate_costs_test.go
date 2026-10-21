package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉复核取候选成本（票 demo-chain-gaps/115）：生产证据视图不折成本，复核要像初始路由一样向候选成本取数侧要，排序前
// 填进证据；形成的计划只带选中候选的逐段出处（票 demo-chain-gaps/105）。下面的证据都先去掉成本事实，与生产证据视图交回的
// 一样，成本只从取数侧来。

// reassessCosts 是复核的候选成本取数侧替身。answer 与 err 都空时照 passthroughCosts 交回证据里的事实：既有用例的证据里
// 直接带着成本事实。另记下每次交进来的携带内容。
type reassessCosts struct {
	answer  *ports.RouteCandidateCosts
	err     error
	carried []ports.RequestCarriedContent
}

func (costs *reassessCosts) LoadCandidateCosts(
	_ context.Context,
	_ domain.InitialRouteJudgmentKey,
	evidence ports.InitialRouteEvidence,
	carried ports.RequestCarriedContent,
) (ports.RouteCandidateCosts, error) {
	costs.carried = append(costs.carried, carried)
	if costs.err != nil {
		return ports.RouteCandidateCosts{}, costs.err
	}
	if costs.answer != nil {
		return *costs.answer, nil
	}
	return ports.RouteCandidateCosts{Facts: evidence.CandidateCosts}, nil
}

func withoutCosts(evidence ports.InitialRouteEvidence) ports.InitialRouteEvidence {
	evidence.CandidateCosts = nil
	return evidence
}

// pricedBySource 是取数侧给两条候选出的价：candidate-1 1000、candidate-2 100 分，各带一段出处。
func pricedBySource(t *testing.T) *ports.RouteCandidateCosts {
	t.Helper()
	return &ports.RouteCandidateCosts{
		Facts: []domain.CandidateCostFact{
			pricedCandidateCost(t, "candidate-1", 1000, "CNY"),
			pricedCandidateCost(t, "candidate-2", 100, "CNY"),
		},
		Citations: append(legCostCitations(t, "candidate-1", 1), legCostCitations(t, "candidate-2", 1)...),
	}
}

// assertCitesOnly 断言计划的逐段出处全是给定候选的：评价标识由 legCostCitations 带候选名铸。
func assertCitesOnly(t *testing.T, plan domain.InitialRoutePlan, candidate string) {
	t.Helper()
	citations := plan.CostCitations()
	if len(citations) != 1 || citations[0].EvaluationID() != "EST-"+candidate+"-1" {
		t.Fatalf("计划出处 = %+v, want 只有 %s 那一段", citations, candidate)
	}
}

var reassessCarried = ports.RequestCarriedContent{Geo: domain.NewGeoResolutionProjection(
	domain.NewGeoResolutionSide("CN", true, "200000", true),
	domain.NewGeoResolutionSide("SG", true, "018956", true),
)}

// Covers: AT-NR-039（编排半边，候选成本那一格）——原计划失效之后走无路由那条线，证据视图不带成本：复核向取数侧取候选成本，
// 成本单维下选出便宜的那条形成首个当前有效路由计划，计划只带它的逐段出处；取数侧收到的携带内容就是触发带来的那一份。
func TestTheNoRouteLineFormsAFirstPlanFromCostsTheSourceLoads(t *testing.T) {
	fixture := newReassessFixture(t)
	fixture.costs.answer = pricedBySource(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = withoutCosts(restrictedSelectedCandidate(t, evidenceBeforeFreeze(t)))
	lapsedOnPickup(t, fixture)

	fixture.evidence.byParcel["parcel-1"] = withoutCosts(cheaperAlternative(t, 100))
	command := reassessCommandAt(t, "trigger-2", "node-origin", reassessedAt.Add(time.Hour))
	command.Carried = reassessCarried
	result := handleOK(t, fixture, command)
	if result.Outcome() != application.ReassessedFirstPlanFormed {
		t.Fatalf("outcome = %q/%q, want FIRST_PLAN_FORMED", result.Outcome(), result.UndecidedReason())
	}
	formed, _ := result.Record()
	if formed.NewPlan.SelectedCandidate().String() != "candidate-2" {
		t.Fatalf("选中 %s, want 便宜的 candidate-2", formed.NewPlan.SelectedCandidate())
	}
	assertCitesOnly(t, formed.NewPlan, "candidate-2")
	last := fixture.costs.carried[len(fixture.costs.carried)-1]
	if last.Geo.ContentDigest() != reassessCarried.Geo.ContentDigest() {
		t.Fatalf("取数侧收到的投影 %s, want 触发带来的那一份", last.Geo.ContentDigest())
	}
}

// Covers: AT-NR-042、AT-NR-037（失效线候选成本那一格）——原计划失效，证据视图不带成本：候选评估用取数侧交回的成本判出
// 有候选。策略版本没声明自动改路时形成改路建议；声明了、改善超过阈值时自动改路，新计划只带选中候选的逐段出处。阈值 0 是
// 本测试交入的租户取值。
func TestALapsedPlanReviewsCandidatesWithCostsTheSourceLoads(t *testing.T) {
	t.Run("没声明自动改路", func(t *testing.T) {
		fixture := rerouteFixture(t)
		fixture.costs.answer = pricedBySource(t)
		fixture.evidence.byParcel["parcel-1"] = withoutCosts(cheaperAlternative(t, 100))

		result := handleOK(t, fixture, reassessCommand(t, "node-actual-elsewhere"))
		record, _ := result.Record()
		if result.Outcome() != application.ReassessedPlanLapsed || record.CandidateState != ports.CandidatesAvailable {
			t.Fatalf("outcome = %q、候选评估 %q, want PLAN_LAPSED、CANDIDATES_AVAILABLE", result.Outcome(), record.CandidateState)
		}
		if !record.HasSuggestion || !slices.Contains(record.Suggestion.Blockers(), "AUTO_REROUTE_UNCONFIGURED") {
			t.Fatalf("建议 = %v %v, want 形成建议、阻塞点名自动改路未配置", record.HasSuggestion, record.Suggestion.Blockers())
		}
	})
	t.Run("声明了自动改路", func(t *testing.T) {
		fixture := rerouteFixture(t)
		fixture.costs.answer = pricedBySource(t)
		fixture.evidence.byParcel["parcel-1"] = withoutCosts(declareCostImprovement(cheaperAlternative(t, 100), 0))

		result := handleOK(t, fixture, reassessCommand(t, "node-actual-elsewhere"))
		if result.Outcome() != application.ReassessedRerouted {
			t.Fatalf("outcome = %q, want REROUTED", result.Outcome())
		}
		record, _ := result.Record()
		assertCitesOnly(t, record.NewPlan, "candidate-2")
	})
}

// Covers: ADR-0173 决定三（仍适用线候选成本那一格）——原计划仍可执行、未越过冻结边界，证据视图不带成本：取数侧交回的成本
// 让改善判得出，严格大于阈值即自动切换，新计划只带选中候选的逐段出处。阈值 0 是本测试交入的租户取值。
func TestAnImprovementRerouteComparesCostsTheSourceLoads(t *testing.T) {
	fixture := newReassessFixture(t)
	fixture.costs.answer = pricedBySource(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	evidence := withoutCosts(declareCostImprovement(cheaperAlternative(t, 100), 0))
	limit := 0
	evidence.FreezeForm = domain.RemainingSegmentCountFreeze
	evidence.FreezeRemainingSegmentLimit = &limit
	fixture.evidence.byParcel["parcel-1"] = evidence

	result := handleOK(t, fixture, reassessCommand(t, "node-origin"))
	if result.Outcome() != application.ReassessedRerouted {
		t.Fatalf("outcome = %q, want REROUTED", result.Outcome())
	}
	switched, _ := result.Record()
	assertCitesOnly(t, switched.NewPlan, "candidate-2")
}

// Covers: 票 demo-chain-gaps/115「要裁的」——成本取数侧没接与读不回按恢复动作分格。读不回是依赖故障，重投推得动：无路由那
// 条线答未决 REASSESS_COST_SOURCE_UNAVAILABLE，不落复核、不入账未判。没接重投推不动：无路由那条线照常入账，仍无当前有效
// 路由、候选评估记未决，不形成首个计划。失效线上两格都不拦失效落库，候选评估记未决、不挂建议；仍适用线上两格都只是不试
// 改善，原计划照旧保留。
func TestCostSourceGapsSettlePerLine(t *testing.T) {
	unreadable := errors.New("parcel pricing read timed out")
	gaps := map[string]func(*reassessFixture){
		"读不回": func(fixture *reassessFixture) { fixture.costs.err = unreadable },
		"没接":  func(fixture *reassessFixture) { fixture.costs.err = ports.ErrRouteCostSourceNotConfigured },
		"没装配": func(fixture *reassessFixture) {
			fixture.handler = application.NewReassessRouteHandler(application.ReassessRouteDeps{
				Routes:        fixture.routes,
				Evidence:      fixture.evidence,
				Applicability: fixture.applicability,
				Store:         fixture.store,
				Log:           fixture.log,
				Identities:    &routeIdentityDouble{},
				Clock:         fixedClock{at: reassessedAt},
			})
		},
	}
	for name, gap := range gaps {
		t.Run("无路由线/"+name, func(t *testing.T) {
			fixture := newReassessFixture(t)
			record := currentPlanRecord(t)
			fixture.routes.records[record.Key] = record
			fixture.evidence.byParcel["parcel-1"] = withoutCosts(restrictedSelectedCandidate(t, evidenceBeforeFreeze(t)))
			lapsedOnPickup(t, fixture)
			gap(fixture)

			fixture.evidence.byParcel["parcel-1"] = withoutCosts(cheaperAlternative(t, 100))
			result := handleOK(t, fixture, reassessCommandAt(t, "trigger-2", "node-origin", reassessedAt.Add(time.Hour)))
			_, recorded := fixture.store.byCorrelation[value(t, domain.NewRequestCorrelationID, "trigger-2")]
			if name == "读不回" {
				if result.Outcome() != application.ReassessUndecided ||
					result.UndecidedReason() != application.ReassessCostSourceUnavailable {
					t.Fatalf("outcome = %q/%q, want UNDECIDED/REASSESS_COST_SOURCE_UNAVAILABLE", result.Outcome(), result.UndecidedReason())
				}
				if recorded || len(fixture.store.notJudged) != 0 {
					t.Fatalf("未决却落了复核 %v 或未判 %d 行", recorded, len(fixture.store.notJudged))
				}
				return
			}
			stayed, _ := result.Record()
			if result.Outcome() != application.ReassessedPlanLapsed || stayed.HasNewPlan ||
				stayed.CandidateState != ports.CandidateReviewUndecided {
				t.Fatalf("outcome = %q、新计划 %v、候选评估 %q, want PLAN_LAPSED、无新计划、CANDIDATE_REVIEW_UNDECIDED",
					result.Outcome(), stayed.HasNewPlan, stayed.CandidateState)
			}
		})
		t.Run("失效线/"+name, func(t *testing.T) {
			fixture := rerouteFixture(t)
			gap(fixture)
			fixture.evidence.byParcel["parcel-1"] = withoutCosts(declareCostImprovement(cheaperAlternative(t, 100), 0))

			result := handleOK(t, fixture, reassessCommand(t, "node-actual-elsewhere"))
			record, _ := result.Record()
			if result.Outcome() != application.ReassessedPlanLapsed || record.CandidateState != ports.CandidateReviewUndecided ||
				record.HasSuggestion || record.HasNewPlan {
				t.Fatalf("outcome = %q、候选评估 %q、建议 %v、新计划 %v, want 失效照落、候选评估未决、不挂建议",
					result.Outcome(), record.CandidateState, record.HasSuggestion, record.HasNewPlan)
			}
			if state := fixture.applicability.byPlan[value(t, domain.NewRoutePlanVersionID, "plan-1/v1")].State(); state != domain.PlanLapsed {
				t.Fatalf("原计划适用性 = %q, want LAPSED", state)
			}
		})
		t.Run("仍适用线/"+name, func(t *testing.T) {
			fixture := newReassessFixture(t)
			gap(fixture)
			record := currentPlanRecord(t)
			fixture.routes.records[record.Key] = record
			evidence := withoutCosts(declareCostImprovement(cheaperAlternative(t, 100), 0))
			limit := 0
			evidence.FreezeForm = domain.RemainingSegmentCountFreeze
			evidence.FreezeRemainingSegmentLimit = &limit
			fixture.evidence.byParcel["parcel-1"] = evidence

			result := handleOK(t, fixture, reassessCommand(t, "node-origin"))
			kept, _ := result.Record()
			if result.Outcome() != application.ReassessedStillApplicable || kept.HasNewPlan {
				t.Fatalf("outcome = %q、新计划 %v, want STILL_APPLICABLE、原计划保留", result.Outcome(), kept.HasNewPlan)
			}
			if len(fixture.applicability.saved) != 0 {
				t.Fatalf("适用性写了 %d 次, want 不动", len(fixture.applicability.saved))
			}
		})
	}
}
