package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证复核取候选成本（票 demo-chain-gaps/115）：生产证据视图不折成本，复核向候选成本取数侧要，原计划
// 失效之后的无路由那条线按成本单维选出唯一合格候选、形成首个当前有效路由计划，计划只带选中候选的逐段出处（票
// demo-chain-gaps/105）落进复核册。夹具全部合成，只记 `S`。

// costCitationsFor 给一条候选逐段造出处，评价标识带候选名，计划里留下的是谁的一眼认得出。
func costCitationsFor(t *testing.T, candidate string, legs int) []nrports.CandidateLegCostCitation {
	t.Helper()
	citations := make([]nrports.CandidateLegCostCitation, 0, legs)
	for ordinal := 1; ordinal <= legs; ordinal++ {
		citation, err := nrdomain.NewPlannedLegCostCitation(nrdomain.PlannedLegCostCitationSpec{
			Ordinal:            ordinal,
			EvaluationID:       "EST-" + candidate + "-" + string(rune('0'+ordinal)),
			PlanReference:      "SYN-PLAN-" + candidate + "/v1",
			ComparisonAmount:   "10",
			ComparisonCurrency: "CNY",
		})
		if err != nil {
			t.Fatalf("段出处：%v", err)
		}
		citations = append(citations, nrports.CandidateLegCostCitation{
			Candidate: mustNR(t, nrdomain.NewCandidateID, candidate), PlannedLegCostCitation: citation,
		})
	}
	return citations
}

// Covers: AT-NR-039、AT-NR-053（真库，交接一路）——两条候选都合格，证据视图不带成本，成本由取数侧给：经上海枢纽的
// SYN-CANDIDATE-01（原计划所选）4200 分，经深圳口岸的 SYN-CANDIDATE-02 3100 分。
//  1. 交进吉隆坡枢纽（不在计划上）：原计划失效；候选评估用取数侧的成本判出有候选，策略版本没声明自动改路，形成建议；
//  2. 交进新加坡枢纽：走无路由那条线，按成本选出便宜的 SYN-CANDIDATE-02，形成首个当前有效路由计划，只带它那三段出处；
//     复核册读回的新计划与之相同；
//  3. 又一次交进新加坡枢纽：起点是那份新计划（经复核册重建），仍适用引用它。
func TestAfterALapseTheCheaperCandidateFormsTheFirstPlanOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	evidence := &fixture.evidence.evidence
	evidence.RankingForm = nrdomain.CostSingleDimensionRanking
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
	}
	evidence.Paths = []nrports.CandidatePath{
		{
			Candidate: mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
			Legs:      legsThrough(t, "SYN-NODE-SHA-HUB", reassessHandoverReceiving, "SYN-NODE-SIN-LM"),
		},
		{
			Candidate: mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-02"),
			Legs:      legsThrough(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE", reassessHandoverReceiving, "SYN-NODE-SIN-LM"),
		},
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
		Citations: append(costCitationsFor(t, "SYN-CANDIDATE-01", 2), costCitationsFor(t, "SYN-CANDIDATE-02", 3)...),
	}

	astray := fixture.handoverInto(t, "astray", "SYN-NODE-KUL-HUB", reassessHandoverAt.Add(-time.Hour))
	deliver(t, fixture.syn, fixture.handover, astray, astray)
	lapsed := reassessmentOf(t, fixture.syn, handoverCorrelation("astray"))
	if lapsed.Conclusion != nrports.ReassessmentPlanLapsed || lapsed.CandidateState != nrports.CandidatesAvailable ||
		!lapsed.HasSuggestion {
		t.Fatalf("交进吉隆坡 = %s、候选评估 %s、建议 %v, want PLAN_LAPSED、CANDIDATES_AVAILABLE、形成建议",
			lapsed.Conclusion, lapsed.CandidateState, lapsed.HasSuggestion)
	}

	back := fixture.handoverInto(t, "back", reassessHandoverReceiving, reassessHandoverAt)
	deliver(t, fixture.syn, fixture.handover, back, back)
	formed := reassessmentOf(t, fixture.syn, handoverCorrelation("back"))
	if formed.Conclusion != nrports.ReassessmentFirstPlanFormed || !formed.HasNewPlan ||
		formed.NewPlan.SelectedCandidate().String() != "SYN-CANDIDATE-02" {
		t.Fatalf("交进新加坡 = %s、选中 %s, want FIRST_PLAN_FORMED、便宜的 SYN-CANDIDATE-02",
			formed.Conclusion, formed.NewPlan.SelectedCandidate())
	}
	citations := formed.NewPlan.CostCitations()
	if len(citations) != 3 {
		t.Fatalf("计划出处 %d 条, want 选中候选三段各一条：%+v", len(citations), citations)
	}
	for _, citation := range citations {
		if !strings.HasPrefix(citation.EvaluationID(), "EST-SYN-CANDIDATE-02-") {
			t.Fatalf("计划出处 = %+v, want 只有 SYN-CANDIDATE-02 的", citation)
		}
	}

	onward := fixture.handoverInto(t, "onward", reassessHandoverReceiving, reassessHandoverAt.Add(time.Hour))
	deliver(t, fixture.syn, fixture.handover, onward, onward)
	if kept := reassessmentOf(t, fixture.syn, handoverCorrelation("onward")); kept.Conclusion != nrports.ReassessmentStillApplicable ||
		kept.ReviewedPlan != formed.NewPlan.Version() {
		t.Fatalf("新计划之后 = %s / %s, want STILL_APPLICABLE、复核新计划 %s", kept.Conclusion, kept.ReviewedPlan, formed.NewPlan.Version())
	}
}

// acceptDemoParcelWithGeoAndWeight 照 SYN-V0 的提交、判断与形成决定走一遍，提交命令上同时报两端地址要素与客户声明 2.5 KG：
// 证据视图要投影解服务区域，候选成本取数侧要那份声明出价。
func acceptDemoParcelWithGeoAndWeight(t *testing.T, syn *synVerticalFixture) {
	t.Helper()
	command := syn.submitCommand(t)
	command.DeclaredElements = synDeclaredCNToSG(t)
	command.DeclaredProfiles = []psdomain.DeclaredParcelProfile{declaredProfile(t, reassessHandoverParcel, "2.5", "KG")}
	var outcome psapplication.SubmitOutcome
	mustWithinTX(t, syn.transactor, t.Context(), func(txCtx context.Context) error {
		result, err := syn.submitHandler.Handle(txCtx, command)
		outcome = result.Outcome()
		return err
	})
	if outcome != psapplication.OutcomeSubmitted {
		t.Fatalf("提交 = %q, want SUBMITTED", outcome)
	}
	syn.recordPassingJudgments(t, t.Context())
	if result := syn.formDecision(t, t.Context()); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
}

// Covers: AT-NR-039（生产装配那一半）——演示租户采用了演示网络，登了一张与演示成本卡同身份的 CNY 成本卡（三段各 Z1 85 CNY），
// 委托已接受且带客户声明 2.5 KG。证据视图、关务来源、候选成本取数侧与复核编排全是生产接法，经生产路由表一拍：
//  1. 交进吉隆坡枢纽（不在计划上）：原计划失效，候选评估用取数侧出的价判出有候选，没声明自动改路，形成建议；
//  2. 交进新加坡枢纽：走无路由那条线，形成首个当前有效路由计划，选中演示线路，三段出处都指那张成本卡、评价标识出自
//     parcel-pricing 的试算入口。
//
// 成本卡没照抄演示种子那张（种子卡绑了分区目录）：复核交给取数侧的输入与初始路由的相同，绑目录的卡怎么经邮编路线解区已由
// route_cost_reading_completion_wiring_test.go 钉住，这里只证复核接上了取数侧。包裹事实取客户声明是过渡做法，见
// reassessRouteHandler 的注释。
func TestAfterALapseTheDemoNetworkFormsTheFirstPlanThroughTheProductionRoute(t *testing.T) {
	syn := newSYNVerticalFixture(t)
	acceptDemoParcelWithGeoAndWeight(t, syn)
	registerDemoNetworkSeeds(t, syn.db)
	registerCostCard(t, syn.db, demoShapedCostCard(t, "CNY", ppdomain.PricingPlanStructures{}))
	seedWiringCustomsCatalog(t, syn.db, demoTenant, true)
	savePlan(t, syn, "SYN-LINE-CN-SG-01@1",
		"SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE", reassessHandoverReceiving, "SYN-NODE-SIN-LM")
	store, err := outbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	register := newHandoverRegistration(t, syn.db, store)
	handover := func(tag, receiving string, at time.Time) {
		command := handoverCommand(t, reassessHandoverParcel, "SYN-HANDOVER/132/"+tag, tfdomain.ObjectHandedOver, "", receiving)
		command.Scope = reassessHandoverScope + "-" + tag
		command.JudgedAt = at
		eventID := registerHandover(t, syn, register, command)
		if _, err := syn.beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("一拍：%v", err)
		}
		if status := outboxStatus(t, syn.db, eventID); status != "PUBLISHED" {
			t.Fatalf("交接 %s 信封 status = %s，失败码 = %q, want PUBLISHED", tag, status, recordedFailureCode(t, syn.db, eventID))
		}
	}

	handover("astray", "SYN-NODE-KUL-HUB", reassessHandoverAt.Add(-time.Hour))
	lapsed := reassessmentOf(t, syn, handoverCorrelation("astray"))
	if lapsed.Conclusion != nrports.ReassessmentPlanLapsed || lapsed.CandidateState != nrports.CandidatesAvailable ||
		!lapsed.HasSuggestion {
		t.Fatalf("交进吉隆坡 = %s、候选评估 %s、建议 %v, want PLAN_LAPSED、CANDIDATES_AVAILABLE、形成建议",
			lapsed.Conclusion, lapsed.CandidateState, lapsed.HasSuggestion)
	}

	handover("back", reassessHandoverReceiving, reassessHandoverAt)
	formed := reassessmentOf(t, syn, handoverCorrelation("back"))
	if formed.Conclusion != nrports.ReassessmentFirstPlanFormed || !formed.HasNewPlan ||
		formed.NewPlan.SelectedCandidate().String() != "SYN-LINE-CN-SG-01@1" {
		t.Fatalf("交进新加坡 = %s、选中 %s, want FIRST_PLAN_FORMED、演示线路", formed.Conclusion, formed.NewPlan.SelectedCandidate())
	}
	citations := formed.NewPlan.CostCitations()
	if len(citations) != 3 {
		t.Fatalf("计划出处 %d 条, want 三段各一条：%+v", len(citations), citations)
	}
	for _, citation := range citations {
		if citation.PlanReference() != "SYN-PLAN-CN-SG-COST-01/v1" || !strings.HasPrefix(citation.EvaluationID(), "EST-") {
			t.Fatalf("计划出处 = %+v, want 指那张成本卡、评价标识出自 PP 的试算入口", citation)
		}
	}
}
