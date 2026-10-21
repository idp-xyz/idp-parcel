package main

import (
	"testing"
	"time"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件对真实 PostgreSQL 证仍适用线上的改善改路落得了复核册（票 docs-consistency/code/53）：`已改路`那一行不带失效依据，
// 带改善依据，过 route_reassessment 的逐结论 CHECK。夹具全部合成，只记 `S`；阈值是本用例交入的租户取值。

// declareImprovementReroute 在两腿夹具上声明成本单维排序与自动改路，登两条都合格的候选：原计划所选的 SYN-CANDIDATE-01
// 4200 分，同一条节点链上的 SYN-CANDIDATE-02 3100 分，改善 1100 分。
func declareImprovementReroute(t *testing.T, fixture *reassessBothLanesFixture, threshold int) {
	t.Helper()
	evidence := &fixture.evidence.evidence
	evidence.RankingForm = nrdomain.CostSingleDimensionRanking
	evidence.AutoRerouteForm = nrdomain.CostImprovementAutoReroute
	evidence.AutoRerouteImprovementThresholdMinor = &threshold
	for _, candidate := range []string{"SYN-CANDIDATE-01", "SYN-CANDIDATE-02"} {
		area, err := nrdomain.NewServiceAreaResolution(nrdomain.ServiceAreaResolutionSpec{
			Candidate:   mustNR(t, nrdomain.NewCandidateID, candidate),
			Outcome:     nrdomain.AreaCoversDestination,
			AreaVersion: mustNR(t, nrdomain.NewServiceAreaVersionReference, "SYN-AREA/v1"),
		})
		if err != nil {
			t.Fatalf("服务区域：%v", err)
		}
		evidence.ServiceAreas = append(evidence.ServiceAreas, area)
		evidence.Paths = append(evidence.Paths, nrports.CandidatePath{
			Candidate: mustNR(t, nrdomain.NewCandidateID, candidate),
			Legs:      legsThrough(t, "SYN-NODE-SHA-HUB", reassessHandoverReceiving, "SYN-NODE-SIN-LM"),
		})
	}
	priced := func(candidate string, amount int64) nrdomain.CandidateCostFact {
		fact, err := nrdomain.NewPricedCandidateCost(mustNR(t, nrdomain.NewCandidateID, candidate), amount, "CNY")
		if err != nil {
			t.Fatalf("候选成本：%v", err)
		}
		return fact
	}
	fixture.costs.answer = &nrports.RouteCandidateCosts{
		Facts:     []nrdomain.CandidateCostFact{priced("SYN-CANDIDATE-01", 4200), priced("SYN-CANDIDATE-02", 3100)},
		Citations: append(costCitationsFor(t, "SYN-CANDIDATE-01", 2), costCitationsFor(t, "SYN-CANDIDATE-02", 2)...),
	}
}

// Covers: AT-NR-085（真库，交接一路）——策略版本声明了自动改路、阈值 1000 分；原计划 SYN-RPV-63 仍可执行，包裹交进新加坡
// 枢纽（当前可控节点）、未越过冻结边界，便宜的候选改善 1100 分、严格大于阈值：
//  1. 落一行`已改路`：被复核计划是原计划，不带失效依据，带改善依据（被替代计划成本 4200、新计划成本 3100、阈值 1000、
//     CNY，被替代那一侧带 SYN-CANDIDATE-01 这次取到的两段出处），候选评估有候选，新计划选中 SYN-CANDIDATE-02，带自动
//     改路决定；信封入账，不回滚重投；
//  2. 适用性册里原计划已被替代、接班的是新计划，两者都在触发的业务时间转移（ADR-0272）；
//  3. 又一次交进新加坡枢纽：起点是那份新计划，仍适用引用它，不再改路。
func TestAnImprovementRerouteBeforeTheFreezeBoundaryLandsOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	declareImprovementReroute(t, fixture, 1000)

	hub := fixture.handoverInto(t, "hub", reassessHandoverReceiving, reassessHandoverAt)
	if err := fixture.handover.Consume(t.Context(), outboxEnvelope(t, fixture.syn, hub, hub)); err != nil {
		t.Fatalf("投递交接 = %v, want 入账——改善改路那一行须落得了复核册", err)
	}
	if status := inboxStatusOf(t, fixture.syn, reassessOnHandoverConsumerName, hub); status != "PROCESSED" {
		t.Fatalf("inbox status = %q, want PROCESSED", status)
	}
	switched := reassessmentOf(t, fixture.syn, handoverCorrelation("hub"))
	if switched.Conclusion != nrports.ReassessmentRerouted ||
		switched.ReviewedPlan.String() != "SYN-RPV-63" ||
		switched.LapseBasis.String() != "" ||
		switched.CandidateState != nrports.CandidatesAvailable ||
		switched.RerouteState != nrdomain.AutomaticRerouteAllowed {
		t.Fatalf("复核头部 = %+v, want REROUTED、复核 SYN-RPV-63、无失效依据、有候选、允许自动", switched)
	}
	if !switched.HasNewPlan || switched.NewPlan.SelectedCandidate().String() != "SYN-CANDIDATE-02" ||
		!switched.HasDecision || switched.Decision.Mode() != nrdomain.AutomaticReroute ||
		switched.Decision.OriginalPlan().String() != "SYN-RPV-63" {
		t.Fatalf("新计划 %v 选中 %s、决定 %v, want 选中 SYN-CANDIDATE-02 的新计划与自动决定",
			switched.HasNewPlan, switched.NewPlan.SelectedCandidate(), switched.HasDecision)
	}
	if !switched.HasImprovementBasis {
		t.Fatal("改善改路那一行没带改善依据")
	}
	basis := switched.ImprovementBasis
	if basis.SupersededCostMinor() != 4200 || basis.SuccessorCostMinor() != 3100 ||
		basis.ThresholdMinor() != 1000 || basis.Currency() != "CNY" {
		t.Fatalf("改善依据 = %+v, want 4200 → 3100、阈值 1000、CNY", basis)
	}
	cited := basis.SupersededCostCitations()
	if len(cited) != 2 || cited[0].EvaluationID() != "EST-SYN-CANDIDATE-01-1" || cited[1].EvaluationID() != "EST-SYN-CANDIDATE-01-2" {
		t.Fatalf("被替代那一侧的出处 = %+v, want SYN-CANDIDATE-01 这次取到的两段", cited)
	}

	applicabilities, err := nrpostgres.NewPlanApplicabilities(fixture.syn.db)
	if err != nil {
		t.Fatalf("构造适用性库：%v", err)
	}
	original, found, err := applicabilities.FindByPlan(t.Context(), mustNR(t, nrdomain.NewRoutePlanVersionID, "SYN-RPV-63"))
	successor, hasSuccessor := original.Successor()
	if err != nil || !found || original.State() != nrdomain.PlanSuperseded || !hasSuccessor ||
		successor != switched.NewPlan.Version() || !original.TransitionedAt().Equal(reassessHandoverAt) {
		t.Fatalf("原计划适用性 = %s 接班 %s 于 %s（found %v err %v）, want SUPERSEDED、接班新计划、于触发业务时间",
			original.State(), successor, original.TransitionedAt(), found, err)
	}

	onward := fixture.handoverInto(t, "onward", reassessHandoverReceiving, reassessHandoverAt.Add(time.Hour))
	deliver(t, fixture.syn, fixture.handover, onward, onward)
	kept := reassessmentOf(t, fixture.syn, handoverCorrelation("onward"))
	if kept.Conclusion != nrports.ReassessmentStillApplicable || kept.ReviewedPlan != switched.NewPlan.Version() {
		t.Fatalf("改路之后 = %s / %s, want STILL_APPLICABLE、复核新计划 %s",
			kept.Conclusion, kept.ReviewedPlan, switched.NewPlan.Version())
	}
}
