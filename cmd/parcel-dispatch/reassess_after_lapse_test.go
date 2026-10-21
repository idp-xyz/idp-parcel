package main

import (
	"context"
	"testing"
	"time"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 证复核的判断起点（票 docs-consistency/code/52）：原计划失效之后，下一次控制触发从「无当前有效
// 路由」判，复核形成的新计划成为再下一次的起点。收寄采用与交接两路都与生产同形，只换证据视图。夹具全部合成，只记 `S`。

// reassessmentOf 经复核册的生产读口按关联读回一次复核：可空列与新计划都按适配器的口径翻译，不在测试里重拼。
func reassessmentOf(t *testing.T, syn *synVerticalFixture, correlation string) nrports.ReassessmentRecord {
	t.Helper()
	store, err := nrpostgres.NewRouteReassessments(syn.db)
	if err != nil {
		t.Fatalf("构造复核库：%v", err)
	}
	record, found, err := store.FindByCorrelation(t.Context(),
		mustNR(t, nrdomain.NewTenantID, "SYN-TENANT-01"), mustNR(t, nrdomain.NewRequestCorrelationID, correlation))
	if err != nil || !found {
		t.Fatalf("读复核 %s：found %v err %v；未判 = %+v", correlation, found, err, notJudgedRows(t, syn))
	}
	return record
}

// Covers: AT-NR-039（真库，收寄采用与交接两路）与 ADR-0249 越权风险点 3——揽收复核时选中候选被有效硬限制淘汰，原计划失效、
// 没有别的合格候选。随后：
//  1. 交进新加坡枢纽（在原计划上）、限制仍在：走无路由那条线，入账一行 `PLAN_LAPSED`、没有被复核计划、候选评估
//     `NO_QUALIFIED_CANDIDATES`；信封入账，不回滚重投；
//  2. 限制撤掉、再一次交进新加坡枢纽：不对已失效的计划答仍适用，形成首个当前有效路由计划，换新的计划版本；原计划适用性
//     仍是 `LAPSED`；
//  3. 又一次交进新加坡枢纽：起点是那份新计划，仍适用引用它。
//
// 两行无路由的形状都过得了复核册的逐结论 CHECK。
func TestAfterALapseTheNextTriggersReassessFromNoRouteOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	evidence := &fixture.evidence.evidence
	evidence.RankingForm = nrdomain.CostSingleDimensionRanking
	area, err := nrdomain.NewServiceAreaResolution(nrdomain.ServiceAreaResolutionSpec{
		Candidate:   mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		Outcome:     nrdomain.AreaCoversDestination,
		AreaVersion: mustNR(t, nrdomain.NewServiceAreaVersionReference, "SYN-AREA/v1"),
	})
	if err != nil {
		t.Fatalf("服务区域：%v", err)
	}
	cost, err := nrdomain.NewPricedCandidateCost(mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"), 4200, "CNY")
	if err != nil {
		t.Fatalf("候选成本：%v", err)
	}
	evidence.ServiceAreas = []nrdomain.ServiceAreaResolution{area}
	evidence.CandidateCosts = []nrdomain.CandidateCostFact{cost}
	evidence.Paths = []nrports.CandidatePath{{
		Candidate: mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		Legs:      legsThrough(t, "SYN-NODE-SHA-HUB", reassessHandoverReceiving, "SYN-NODE-SIN-LM"),
	}}
	restriction, err := nrdomain.NewHardConstraintFinding(nrdomain.HardConstraintFindingSpec{
		Candidate:   mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		Outcome:     nrdomain.RestrictionApplies,
		Restriction: mustNR(t, nrdomain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/SYN-CC-52"),
	})
	if err != nil {
		t.Fatalf("硬限制：%v", err)
	}
	evidence.HardConstraints = []nrdomain.HardConstraintFinding{restriction}

	pickup := fixture.intakeAt(t, psdomain.OffsitePickupSource, pickupReassessVersion, pickupReassessPlace, pickupReassessAt)
	deliver(t, fixture.syn, fixture.intake, pickup, pickup)
	if lapsed := reassessmentOf(t, fixture.syn, pickupReassessCorrel); lapsed.Conclusion != nrports.ReassessmentPlanLapsed ||
		lapsed.ReviewedPlan.String() != "SYN-RPV-63" {
		t.Fatalf("揽收复核 = %s / %s, want PLAN_LAPSED、复核 SYN-RPV-63", lapsed.Conclusion, lapsed.ReviewedPlan)
	}

	restricted := fixture.handoverInto(t, "restricted", reassessHandoverReceiving, reassessHandoverAt.Add(-time.Hour))
	deliver(t, fixture.syn, fixture.handover, restricted, restricted)
	noRoute := reassessmentOf(t, fixture.syn, handoverCorrelation("restricted"))
	if noRoute.Conclusion != nrports.ReassessmentPlanLapsed || noRoute.ReviewedPlan.String() != "" ||
		noRoute.LapseBasis.String() != "" || noRoute.HasNewPlan || noRoute.CandidateState != nrports.NoQualifiedCandidates {
		t.Fatalf("限制仍在 = %+v, want PLAN_LAPSED、无被复核计划、无失效依据、NO_QUALIFIED_CANDIDATES", noRoute)
	}
	if status := inboxStatusOf(t, fixture.syn, reassessOnHandoverConsumerName, restricted); status != "PROCESSED" {
		t.Fatalf("交接信封的 inbox status = %q, want PROCESSED——入账，不回滚重投", status)
	}

	evidence.HardConstraints = nil
	lifted := fixture.handoverInto(t, "lifted", reassessHandoverReceiving, reassessHandoverAt)
	deliver(t, fixture.syn, fixture.handover, lifted, lifted)
	formed := reassessmentOf(t, fixture.syn, handoverCorrelation("lifted"))
	if formed.Conclusion != nrports.ReassessmentFirstPlanFormed || !formed.HasNewPlan ||
		formed.NewPlan.Version().String() == "SYN-RPV-63" || formed.ReviewedPlan.String() != "" {
		t.Fatalf("限制撤掉 = %s、新计划 %s、被复核 %q, want FIRST_PLAN_FORMED、新的计划版本——已失效的计划不得原地恢复",
			formed.Conclusion, formed.NewPlan.Version(), formed.ReviewedPlan)
	}
	applicabilities, err := nrpostgres.NewPlanApplicabilities(fixture.syn.db)
	if err != nil {
		t.Fatalf("构造适用性库：%v", err)
	}
	original, found, err := applicabilities.FindByPlan(t.Context(), mustNR(t, nrdomain.NewRoutePlanVersionID, "SYN-RPV-63"))
	if err != nil || !found || original.State() != nrdomain.PlanLapsed {
		t.Fatalf("原计划适用性 = %s（found %v err %v）, want LAPSED", original.State(), found, err)
	}

	onward := fixture.handoverInto(t, "onward", reassessHandoverReceiving, reassessHandoverAt.Add(time.Hour))
	deliver(t, fixture.syn, fixture.handover, onward, onward)
	kept := reassessmentOf(t, fixture.syn, handoverCorrelation("onward"))
	if kept.Conclusion != nrports.ReassessmentStillApplicable || kept.ReviewedPlan != formed.NewPlan.Version() {
		t.Fatalf("新计划之后 = %s / %s, want STILL_APPLICABLE、复核新计划 %s",
			kept.Conclusion, kept.ReviewedPlan, formed.NewPlan.Version())
	}
}

// seedFormedPlan 在复核册落一行形成了新计划的复核（触发业务时间与处理时刻由用例给），lapsed 为真时把那份计划的适用性记为
// 已失效。计划沿上海枢纽 → 新加坡枢纽 → 新加坡末端，真经 NR 领域形成。
func seedFormedPlan(
	t *testing.T, syn *synVerticalFixture, correlation, version string, at, processedAt time.Time, lapsed bool,
) {
	t.Helper()
	candidate, err := nrdomain.NewRouteCandidate(mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		nrdomain.CandidateQualified, nrdomain.CandidateReason{})
	if err != nil {
		t.Fatalf("候选：%v", err)
	}
	plan, err := nrdomain.FormInitialRoutePlan(nrdomain.InitialRoutePlanSpec{
		Key:           reassessHandoverKey(t),
		Version:       mustNR(t, nrdomain.NewRoutePlanVersionID, version),
		Selected:      candidate.ID(),
		Candidates:    []nrdomain.RouteCandidate{candidate},
		Legs:          legsThrough(t, "SYN-NODE-SHA-HUB", reassessHandoverReceiving, "SYN-NODE-SIN-LM"),
		Strategy:      mustNR(t, nrdomain.NewRouteStrategyReference, "SYN-STRATEGY/v1"),
		ViewRevision:  mustNR(t, nrdomain.NewNetworkViewRevision, "SYN-VIEW-REV-1"),
		JudgedAt:      processedAt,
		EffectiveFrom: at,
	})
	if err != nil {
		t.Fatalf("计划：%v", err)
	}
	store, err := nrpostgres.NewRouteReassessments(syn.db)
	if err != nil {
		t.Fatalf("构造复核库：%v", err)
	}
	applicabilities, err := nrpostgres.NewPlanApplicabilities(syn.db)
	if err != nil {
		t.Fatalf("构造适用性库：%v", err)
	}
	record := nrports.ReassessmentRecord{
		Correlation:       mustNR(t, nrdomain.NewRequestCorrelationID, correlation),
		Key:               reassessHandoverKey(t),
		Conclusion:        nrports.ReassessmentFirstPlanFormed,
		CandidateState:    nrports.CandidatesAvailable,
		NewPlan:           plan,
		HasNewPlan:        true,
		TriggerOccurredAt: at,
		ReassessedAt:      processedAt,
	}
	mustWithinTX(t, syn.transactor, t.Context(), func(txCtx context.Context) error {
		if _, err := store.Save(txCtx, record.Correlation, record); err != nil {
			return err
		}
		if !lapsed {
			return nil
		}
		return applicabilities.Save(txCtx, lapsedApplicabilityOf(t, plan.Version(), plan.EffectiveFrom()))
	})
}

func lapsedApplicabilityOf(t *testing.T, plan nrdomain.RoutePlanVersionID, effectiveFrom time.Time) nrdomain.PlanApplicability {
	t.Helper()
	established, err := nrdomain.EstablishPlanApplicability(plan, effectiveFrom)
	if err != nil {
		t.Fatalf("适用性：%v", err)
	}
	lapsed, err := established.Lapse(mustNR(t, nrdomain.NewApplicabilityBasisReference, "HARD_CONSTRAINT_RESTRICTION/SYN-CC-52"),
		effectiveFrom)
	if err != nil {
		t.Fatalf("失效：%v", err)
	}
	return lapsed
}

// Covers: 复核判断起点（真库，交接一路）——路由历史里有三份复核形成的计划：一份是迁移 0022 之前落的、没有触发业务时间，
// 已失效；另两份形成于同一业务时刻，先判的那份已失效，后判的那份当前有效，而处理时钟回拨让后判的那次处理时刻更早。下一次
// 交接复核的起点是当前有效的那一份，不按处理时刻取到已失效的那份、也不因业务时间缺席的旧行乱了阵脚。
func TestTheStartingPointIgnoresClockOrderAndLegacyRowsOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	syn := fixture.syn
	applicabilities, err := nrpostgres.NewPlanApplicabilities(syn.db)
	if err != nil {
		t.Fatalf("构造适用性库：%v", err)
	}
	mustWithinTX(t, syn.transactor, t.Context(), func(txCtx context.Context) error {
		return applicabilities.Save(txCtx, lapsedApplicabilityOf(t,
			mustNR(t, nrdomain.NewRoutePlanVersionID, "SYN-RPV-63"), reassessHandoverAt.Add(-72*time.Hour)))
	})
	same := reassessHandoverAt.Add(-2 * time.Hour)
	seedFormedPlan(t, syn, "SYN-SEED/52/legacy", "SYN-RPV-52-LEGACY", reassessHandoverAt.Add(-3*time.Hour), same, true)
	seedFormedPlan(t, syn, "SYN-SEED/52/first", "SYN-RPV-52-FIRST", same, same.Add(2*time.Minute), true)
	seedFormedPlan(t, syn, "SYN-SEED/52/second", "SYN-RPV-52-SECOND", same, same.Add(time.Minute), false)
	// 演迁移 0022 之前落的一行：CHECK 只拦新行，这里在本用例独占的库上拆掉它再抹去业务时间。
	if _, err := syn.pool.Exec(t.Context(), `ALTER TABLE network_routing.route_reassessment
		DROP CONSTRAINT route_reassessment_trigger_occurred_at_present`); err != nil {
		t.Fatalf("拆业务时间约束：%v", err)
	}
	if _, err := syn.pool.Exec(t.Context(), `UPDATE network_routing.route_reassessment SET trigger_occurred_at = NULL
		WHERE tenant_id = 'SYN-TENANT-01' AND correlation_id = 'SYN-SEED/52/legacy'`); err != nil {
		t.Fatalf("抹业务时间：%v", err)
	}

	handover := fixture.handoverInto(t, "after-tie", reassessHandoverReceiving, reassessHandoverAt)
	deliver(t, syn, fixture.handover, handover, handover)
	kept := reassessmentOf(t, syn, handoverCorrelation("after-tie"))
	if kept.Conclusion != nrports.ReassessmentStillApplicable || kept.ReviewedPlan.String() != "SYN-RPV-52-SECOND" {
		t.Fatalf("复核 = %s / %s, want STILL_APPLICABLE、复核当前有效的 SYN-RPV-52-SECOND", kept.Conclusion, kept.ReviewedPlan)
	}
}
