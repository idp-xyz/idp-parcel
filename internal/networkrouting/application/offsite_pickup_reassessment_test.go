package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// pickupPlace 是场外揽收的接货位置：客户场所，不是计划节点。
const pickupPlace = "customer-warehouse-2"

// pickupCommand 是一次场外揽收触发的复核。
func pickupCommand(t *testing.T) application.ReassessRouteCommand {
	t.Helper()
	command := reassessCommand(t, pickupPlace)
	command.Trigger.Control = domain.OffsitePickupControl
	return command
}

func restrictedSelectedCandidate(t *testing.T, evidence ports.InitialRouteEvidence) ports.InitialRouteEvidence {
	t.Helper()
	finding, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
		Candidate:   value(t, domain.NewCandidateID, "candidate-1"),
		Outcome:     domain.RestrictionApplies,
		Restriction: value(t, domain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/CC-REF-9"),
	})
	if err != nil {
		t.Fatalf("new restriction finding: %v", err)
	}
	evidence.HardConstraints = append(evidence.HardConstraints, finding)
	return evidence
}

// Covers: AT-NR-034（ADR-0249 决定一、二）——场外揽收、尚未到节点，选中候选的硬约束仍满足：原计划仍适用、引用原计划
// 版本，揽收地点不与计划节点比。揽收不建立当前可控节点，所以不判冻结（策略版本没声明冻结形态也不落
// FREEZE_FORM_UNCONFIGURED），也不试成本改善切换（有更便宜的候选、阈值为 0 也不改路）；适用性不动。
// 阈值 0 是本测试交入的租户取值。
func TestAPickupKeepsAPlanWhoseHardConstraintsStillHold(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = declareCostImprovement(cheaperAlternative(t, 100), 0)

	result := handleOK(t, fixture, pickupCommand(t))
	if result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q reason = %q, want STILL_APPLICABLE", result.Outcome(), result.NotJudgedReason())
	}
	kept, present := result.Record()
	if !present || kept.ReviewedPlan.String() != "plan-1/v1" {
		t.Fatalf("record = %#v present = %v", kept, present)
	}
	if kept.HasNewPlan || kept.HasDecision || kept.HasSuggestion {
		t.Fatalf("new plan = %v decision = %v suggestion = %v; 揽收复核不试改善切换",
			kept.HasNewPlan, kept.HasDecision, kept.HasSuggestion)
	}
	if kept.LapseBasis.String() != "" {
		t.Fatalf("lapse basis = %q; 仍适用不带失效依据", kept.LapseBasis)
	}
	if !kept.TriggerOccurredAt.Equal(reassessedAt) {
		t.Fatalf("trigger occurred at = %s, want %s——揽收复核计入当前控制位置", kept.TriggerOccurredAt, reassessedAt)
	}
	if len(fixture.store.notJudged) != 0 {
		t.Fatalf("未判登记 = %+v; 揽收复核不判冻结，不落 FREEZE_FORM_UNCONFIGURED", fixture.store.notJudged)
	}
	if len(fixture.applicability.saved) != 0 {
		t.Fatalf("适用性写了 %d 次; 仍适用不动适用性", len(fixture.applicability.saved))
	}
}

// Covers: AT-NR-084（ADR-0249 决定二）——揽收复核时原计划被有效硬限制淘汰，存在替代候选，策略版本允许自动改路：原计划
// 失效、依据点名限制；包裹不在可控节点上，只形成改路建议，阻塞含 NOT_AT_CONTROLLED_NODE、不含
// EXECUTED_PREFIX_AFFECTED（已执行前缀为空），建议带上候选供授权角色决定；不形成自动改路决定与新计划。
// 阈值 0 是本测试交入的租户取值。
func TestAPickupLapseOnlySuggestsBecauseNoControllableNodeIsHeld(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = restrictedSelectedCandidate(t,
		declareCostImprovement(cheaperAlternative(t, 100), 0))

	result := handleOK(t, fixture, pickupCommand(t))
	if result.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("outcome = %q, want PLAN_LAPSED——建议不是改路", result.Outcome())
	}
	lapsed, _ := result.Record()
	if lapsed.LapseBasis.String() != "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-ELIGIBILITY/CC-REF-9" {
		t.Fatalf("lapse basis = %q; 揽收复核只按硬约束失效，不按地点", lapsed.LapseBasis)
	}
	if lapsed.RerouteState != domain.SuggestionOnly || !lapsed.HasSuggestion || lapsed.HasDecision || lapsed.HasNewPlan {
		t.Fatalf("state = %q suggestion = %v decision = %v new plan = %v; 不在可控节点上只形成建议",
			lapsed.RerouteState, lapsed.HasSuggestion, lapsed.HasDecision, lapsed.HasNewPlan)
	}
	blockers := lapsed.Suggestion.Blockers()
	if len(blockers) != 1 || blockers[0] != "NOT_AT_CONTROLLED_NODE" {
		t.Fatalf("blockers = %v, want 只有 NOT_AT_CONTROLLED_NODE——已执行前缀为空，不得出 EXECUTED_PREFIX_AFFECTED", blockers)
	}
	if len(lapsed.Suggestion.Candidates()) == 0 {
		t.Fatal("建议没带候选，授权角色无从决定")
	}
	if len(fixture.applicability.saved) != 1 {
		t.Fatalf("适用性写了 %d 次, want 失效那一次", len(fixture.applicability.saved))
	}
}

// Covers: ADR-0249 决定二末条——原先无当前有效路由、候选评估选出唯一合格候选时，揽收复核照常形成首个当前有效路由
// 计划：形成首个计划不是改路，不要求可控节点。
func TestAPickupFormsTheFirstPlanForAFormerNoRoute(t *testing.T) {
	fixture := newReassessFixture(t)
	noRoute, err := domain.FormNoCurrentRouteJudgment(domain.NoCurrentRouteJudgmentSpec{
		Key: reassessKey(t),
		Candidates: []domain.RouteCandidate{
			eliminated(t, "candidate-1", "PATH_NOT_EXECUTABLE/SCHED-V3"),
		},
		Strategy:     value(t, domain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision: value(t, domain.NewNetworkViewRevision, "net-view-rev-1"),
		JudgedAt:     reassessedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("form prior no-route: %v", err)
	}
	fixture.routes.records[reassessKey(t)] = ports.InitialRouteRecord{Key: reassessKey(t), NoRoute: noRoute, HasNoRoute: true}
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)

	result := handleOK(t, fixture, pickupCommand(t))
	if result.Outcome() != application.ReassessedFirstPlanFormed {
		t.Fatalf("outcome = %q, want FIRST_PLAN_FORMED", result.Outcome())
	}
}

// Covers: ADR-0249 决定三——揽收复核是一次已判的控制触发，计入当前控制位置：业务时间早于揽收的触发后到时入账为
// 「触发早于当前位置」、不判（ADR-0235 决定四）。
func TestAPickupReassessmentCountsAsTheCurrentPosition(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)

	if pickup := handleOK(t, fixture, pickupCommand(t)); pickup.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("揽收复核 = %q, want STILL_APPLICABLE", pickup.Outcome())
	}
	earlier := handleOK(t, fixture,
		reassessCommandAt(t, "trigger-earlier", "node-actual-elsewhere", reassessedAt.Add(-time.Hour)))
	if earlier.Outcome() != application.ReassessTriggerNotJudged ||
		earlier.NotJudgedReason() != ports.NotJudgedTriggerPrecedesCurrentPosition {
		t.Fatalf("较早那次 = %q/%q, want TRIGGER_NOT_JUDGED/TRIGGER_PRECEDES_CURRENT_POSITION",
			earlier.Outcome(), earlier.NotJudgedReason())
	}
}

// Covers: 复核依据里的控制依据分得出揽收与权威运输交接——同一关联、同一位置与来源版本，控制依据换了格就不是重放，
// 答触发冲突（触发指纹含控制依据）。
func TestAPickupAndAHandoverAreDifferentTriggers(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)

	handleOK(t, fixture, pickupCommand(t))
	handover := pickupCommand(t)
	handover.Trigger.Control = domain.TransportHandoverControl
	result, err := fixture.handler.Handle(context.Background(), handover)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReassessTriggerConflict {
		t.Fatalf("outcome = %q, want TRIGGER_CONFLICT", result.Outcome())
	}
}
