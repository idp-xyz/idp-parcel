package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉复核的判断起点（票 docs-consistency/code/52）：起点是这件包裹此刻的当前有效计划，或无当前有效路由，不是受理时
// 落的那一行初始路由。原计划失效之后，下一次控制触发走「原先无当前有效路由」那条线（CONTEXT 生命周期、ADR-0249 决定二末
// 两条）；复核形成的新计划成为下一次复核的起点。

func lapsedOnPickup(t *testing.T, fixture *reassessFixture) {
	t.Helper()
	first := handleOK(t, fixture, pickupCommand(t))
	if first.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("揽收复核 = %q, want PLAN_LAPSED", first.Outcome())
	}
}

// Covers: AT-NR-053、AT-NR-039（编排半边）与 ADR-0249 越权风险点 3——揽收复核按硬限制让原计划失效（无替代候选），之后：
//  1. 节点收寄、限制仍在：走无路由那条线，候选评估确认没有合格候选，结论仍是无当前有效路由；入账，不答
//     REASSESS_STORE_UNAVAILABLE、不回滚重投，也不再动已失效计划的适用性；
//  2. 限制撤掉：不对已失效的计划答仍适用（已失效不得原地恢复），而是重新评估、形成首个当前有效路由计划，换新的计划版本；
//  3. 再一次节点收寄：起点是那份新计划，仍适用引用它。
func TestAfterALapseTheNextTriggerReassessesFromNoRoute(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = restrictedSelectedCandidate(t, evidenceBeforeFreeze(t))
	lapsedOnPickup(t, fixture)
	original := value(t, domain.NewRoutePlanVersionID, "plan-1/v1")

	stillRestricted := handleOK(t, fixture, reassessCommandAt(t, "trigger-2", "node-origin", reassessedAt.Add(time.Hour)))
	if stillRestricted.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("限制仍在 = %q/%q, want PLAN_LAPSED——无路由线入账，不回滚重投",
			stillRestricted.Outcome(), stillRestricted.UndecidedReason())
	}
	noRoute, _ := stillRestricted.Record()
	if noRoute.ReviewedPlan.String() != "" || noRoute.LapseBasis.String() != "" || noRoute.HasNewPlan {
		t.Fatalf("记录 = %+v; 无路由线没有被复核计划，也不再失效一次", noRoute)
	}
	if noRoute.CandidateState != ports.NoQualifiedCandidates {
		t.Fatalf("候选评估 = %q, want NO_QUALIFIED_CANDIDATES", noRoute.CandidateState)
	}
	if len(fixture.applicability.saved) != 1 {
		t.Fatalf("适用性写了 %d 次, want 只有揽收失效那一次", len(fixture.applicability.saved))
	}

	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
	lifted := handleOK(t, fixture, reassessCommandAt(t, "trigger-3", "node-origin", reassessedAt.Add(2*time.Hour)))
	if lifted.Outcome() != application.ReassessedFirstPlanFormed {
		t.Fatalf("限制撤掉 = %q, want FIRST_PLAN_FORMED——已失效的计划不得原地恢复", lifted.Outcome())
	}
	formed, _ := lifted.Record()
	if !formed.HasNewPlan || formed.NewPlan.Version() == original || formed.ReviewedPlan.String() != "" {
		t.Fatalf("新计划 = %s、被复核计划 = %q; 必须形成新的计划版本，不复核已失效的那份",
			formed.NewPlan.Version(), formed.ReviewedPlan)
	}
	if state := fixture.applicability.byPlan[original].State(); state != domain.PlanLapsed {
		t.Fatalf("原计划适用性 = %q, want 仍是 LAPSED", state)
	}

	next := handleOK(t, fixture, reassessCommandAt(t, "trigger-4", "node-origin", reassessedAt.Add(3*time.Hour)))
	if next.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("新计划之后 = %q, want STILL_APPLICABLE", next.Outcome())
	}
	if kept, _ := next.Record(); kept.ReviewedPlan != formed.NewPlan.Version() {
		t.Fatalf("被复核计划 = %s, want 新计划 %s", kept.ReviewedPlan, formed.NewPlan.Version())
	}
}

// Covers: ADR-0249 决定二末条、越权风险点 3——失效之后走无路由线，候选评估未决或最低成本并列时继续无当前有效路由，候选
// 评估记未决；不形成首个计划，也不挂建议（这条线没有被复核计划）。两格都入账。
func TestAfterALapseAnUndecidedOrTiedReviewStaysNoRoute(t *testing.T) {
	unknown, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
		Candidate: value(t, domain.NewCandidateID, "candidate-1"),
		Outcome:   domain.ConstraintStatusUnknown,
		Missing:   value(t, domain.NewEvidenceGapReference, "CUSTOMS_ELIGIBILITY"),
		Reassess:  value(t, domain.NewReassessmentCondition, "WHEN_CUSTOMS_AUTHORITY_ANSWERS"),
	})
	if err != nil {
		t.Fatalf("new unknown constraint: %v", err)
	}
	undecided := evidenceBeforeFreeze(t)
	undecided.HardConstraints = []domain.HardConstraintFinding{unknown}
	for name, evidence := range map[string]ports.InitialRouteEvidence{
		"候选评估未决": undecided,
		"最低成本并列": tiedEvidence(t),
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newReassessFixture(t)
			record := currentPlanRecord(t)
			fixture.routes.records[record.Key] = record
			fixture.evidence.byParcel["parcel-1"] = restrictedSelectedCandidate(t, evidenceBeforeFreeze(t))
			lapsedOnPickup(t, fixture)

			fixture.evidence.byParcel["parcel-1"] = evidence
			result := handleOK(t, fixture, reassessCommandAt(t, "trigger-2", "node-origin", reassessedAt.Add(time.Hour)))
			if result.Outcome() != application.ReassessedPlanLapsed {
				t.Fatalf("outcome = %q/%q, want PLAN_LAPSED——仍无当前有效路由", result.Outcome(), result.UndecidedReason())
			}
			stayed, _ := result.Record()
			if stayed.ReviewedPlan.String() != "" || stayed.HasNewPlan || stayed.HasSuggestion || stayed.HasDecision {
				t.Fatalf("记录 = %+v; 无路由线不复核已失效的计划、不形成计划、不挂建议", stayed)
			}
			if stayed.CandidateState != ports.CandidateReviewUndecided {
				t.Fatalf("候选评估 = %q, want CANDIDATE_REVIEW_UNDECIDED", stayed.CandidateState)
			}
		})
	}
}

// Covers: AT-NR-037 之后的那一次复核——原计划失效、自动改路形成新计划：下一次控制触发以新计划为起点，仍适用引用新计划，
// 不再拿已失效的原计划去比。阈值 0 是本测试交入的租户取值。
func TestARerouteAfterALapseIsTheNextStartingPoint(t *testing.T) {
	fixture := rerouteFixture(t)
	evidence := declareCostImprovement(cheaperAlternative(t, 100), 0)
	limit := 0
	evidence.FreezeForm = domain.RemainingSegmentCountFreeze
	evidence.FreezeRemainingSegmentLimit = &limit
	fixture.evidence.byParcel["parcel-1"] = evidence

	rerouted := handleOK(t, fixture, reassessCommandAt(t, "trigger-1", "node-actual-elsewhere", reassessedAt))
	if rerouted.Outcome() != application.ReassessedRerouted {
		t.Fatalf("outcome = %q, want REROUTED", rerouted.Outcome())
	}
	switched, _ := rerouted.Record()

	next := handleOK(t, fixture, reassessCommandAt(t, "trigger-2", "node-origin", reassessedAt.Add(time.Hour)))
	if next.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q/%q, want STILL_APPLICABLE", next.Outcome(), next.NotJudgedReason())
	}
	if kept, _ := next.Record(); kept.ReviewedPlan != switched.NewPlan.Version() {
		t.Fatalf("被复核计划 = %s, want 改路形成的新计划 %s", kept.ReviewedPlan, switched.NewPlan.Version())
	}
}

// Covers: 仍适用线上按改善自动改路之后的那一次复核——原计划已被替代：起点是接班的新计划，不再复核已被替代的原计划。
// 阈值 0 是本测试交入的租户取值。这条线在真库上还落不了复核册（docs-consistency/code/53），这里只钉编排取起点。
func TestAnImprovementRerouteIsTheNextStartingPoint(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	evidence := declareCostImprovement(cheaperAlternative(t, 100), 0)
	limit := 0
	evidence.FreezeForm = domain.RemainingSegmentCountFreeze
	evidence.FreezeRemainingSegmentLimit = &limit
	fixture.evidence.byParcel["parcel-1"] = evidence

	switched := handleOK(t, fixture, reassessCommandAt(t, "trigger-1", "node-origin", reassessedAt))
	if switched.Outcome() != application.ReassessedRerouted {
		t.Fatalf("outcome = %q, want REROUTED", switched.Outcome())
	}
	successor, _ := switched.Record()

	next := handleOK(t, fixture, reassessCommandAt(t, "trigger-2", "node-origin", reassessedAt.Add(time.Hour)))
	if next.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE", next.Outcome())
	}
	if kept, _ := next.Record(); kept.ReviewedPlan != successor.NewPlan.Version() {
		t.Fatalf("被复核计划 = %s, want 接班计划 %s", kept.ReviewedPlan, successor.NewPlan.Version())
	}
}

// Covers: 起点自相矛盾是坏数据，响亮报错：路由历史里最近一份计划已被替代，却没有更晚的计划接班——这件包裹答不出当前计划
// 还是无路由；按任何一边判都是凭空假定。
func TestASupersededLatestPlanWithoutASuccessorIsLoud(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
	established, err := domain.EstablishPlanApplicability(record.Plan.Version(), record.Plan.EffectiveFrom())
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	superseded, err := established.Supersede(value(t, domain.NewRoutePlanVersionID, "plan-elsewhere"),
		value(t, domain.NewApplicabilityBasisReference, "COST_IMPROVEMENT"), reassessedAt.Add(-time.Minute))
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	fixture.applicability.byPlan[record.Plan.Version()] = superseded

	if _, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin")); err == nil {
		t.Fatal("起点自相矛盾却作答了")
	}
	if fixture.store.saved != 0 {
		t.Fatalf("坏起点上落了 %d 条复核", fixture.store.saved)
	}
}

// Covers: 读不出起点是依赖故障，答未决、回滚重投：复核册读不出最近一份新计划，答路由历史读不回；适用性读不出，答适用性登记
// 读不回。两格都不落复核、不动适用性。
func TestAnUnreadableStartingPointStaysUndecided(t *testing.T) {
	history := newReassessFixture(t)
	record := currentPlanRecord(t)
	history.routes.records[record.Key] = record
	history.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
	history.store.newPlanErr = errors.New("pool closed")
	stalled := handleOK(t, history, reassessCommand(t, "node-origin"))
	if stalled.Outcome() != application.ReassessUndecided || stalled.UndecidedReason() != application.RoutingHistoryUnavailable {
		t.Fatalf("读不出新计划 = %q/%q, want UNDECIDED/ROUTING_HISTORY_UNAVAILABLE", stalled.Outcome(), stalled.UndecidedReason())
	}

	applicability := newReassessFixture(t)
	applicability.routes.records[record.Key] = record
	applicability.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
	applicability.applicability.findErr = errors.New("pool closed")
	stalled = handleOK(t, applicability, reassessCommand(t, "node-origin"))
	if stalled.Outcome() != application.ReassessUndecided || stalled.UndecidedReason() != application.ApplicabilityStoreUnavailable {
		t.Fatalf("读不出适用性 = %q/%q, want UNDECIDED/APPLICABILITY_STORE_UNAVAILABLE", stalled.Outcome(), stalled.UndecidedReason())
	}
	for _, fixture := range []*reassessFixture{history, applicability} {
		if fixture.store.saved != 0 || len(fixture.applicability.saved) != 0 {
			t.Fatalf("复核 %d 条、适用性写 %d 次, want 都是 0", fixture.store.saved, len(fixture.applicability.saved))
		}
	}
}

// Covers: 同一触发的两笔事务并发：后到的一方等在判断键锁上，先到的已经提交复核并换了起点。后到的读回赢家，答已有结果，
// 不按换过的起点再判一次，也不再写适用性。
func TestAConcurrentDuplicateReadsBackTheWinnerAfterTheLock(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = restrictedSelectedCandidate(t, evidenceBeforeFreeze(t))
	command := pickupCommand(t)
	winner := ports.ReassessmentRecord{
		Correlation:       command.Trigger.Correlation,
		Key:               reassessKey(t),
		Conclusion:        ports.ReassessmentPlanLapsed,
		ReviewedPlan:      record.Plan.Version(),
		LapseBasis:        value(t, domain.NewApplicabilityBasisReference, "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-ELIGIBILITY/CC-REF-9"),
		CandidateState:    ports.NoQualifiedCandidates,
		TriggerOccurredAt: reassessedAt,
		ReassessedAt:      reassessedAt,
	}
	fixture.store.onLatest = func() {
		fixture.store.byCorrelation[winner.Correlation] = winner
		fixture.store.onLatest = nil
	}

	result := handleOK(t, fixture, command)
	if result.Outcome() != application.ReassessExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", result.Outcome())
	}
	if read, _ := result.Record(); read.ReviewedPlan != record.Plan.Version() || read.Conclusion != ports.ReassessmentPlanLapsed {
		t.Fatalf("读回 = %+v, want 赢家那一行", read)
	}
	if fixture.store.saved != 0 || len(fixture.applicability.saved) != 0 {
		t.Fatalf("复核 %d 条、适用性写 %d 次, want 都是 0——后到的一方不再判", fixture.store.saved, len(fixture.applicability.saved))
	}
}
