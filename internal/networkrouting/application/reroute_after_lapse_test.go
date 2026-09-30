package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// rerouteFixture 预置「计划失效+候选可用」的前提（实际位置不在计划上）。
// 自动改路由证据上的策略声明折出，不读事实目录。
func rerouteFixture(t *testing.T) *reassessFixture {
	t.Helper()
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
	return fixture
}

func declareCostImprovement(evidence ports.InitialRouteEvidence, threshold int) ports.InitialRouteEvidence {
	evidence.AutoRerouteForm = domain.CostImprovementAutoReroute
	evidence.AutoRerouteImprovementThresholdMinor = &threshold
	return evidence
}

// Covers: AT-NR-037 当前有效实测使原线路不再合格，存在替代候选且允许自动改路 → 新计划。
// 阈值 0 是本测试交入的租户取值，不是产品默认。
func TestAllConditionsMetFormsAnAutomaticRerouteDecision(t *testing.T) {
	fixture := rerouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = declareCostImprovement(cheaperAlternative(t, 100), 0)

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-actual-elsewhere"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReassessedRerouted {
		t.Fatalf("outcome = %q, want REROUTED", result.Outcome())
	}
	record, _ := result.Record()
	if !record.HasDecision || record.Decision.Mode() != domain.AutomaticReroute {
		t.Fatalf("decision = %#v; 自动模式决定必须在场", record.Decision)
	}
	if !record.HasNewPlan || record.NewPlan.Version() == record.ReviewedPlan {
		t.Fatalf("new plan = %s reviewed = %s; 新计划必须换版本", record.NewPlan.Version(), record.ReviewedPlan)
	}
	if record.LapseBasis.String() == "" {
		t.Fatal("改路豁免了失效依据")
	}
	if len(fixture.applicability.saved) == 0 {
		t.Fatal("原计划的失效没有落库")
	}
}

// Covers: AT-NR-042 候选可行但成本改善没有超过已登记阈值 → 只形成建议，计划不自动换。
func TestUnmetConditionsLeaveASuggestionForTheAuthorizedRole(t *testing.T) {
	fixture := rerouteFixture(t)
	// 改善是 900，阈值 1000 是本测试交入的租户取值。
	fixture.evidence.byParcel["parcel-1"] = declareCostImprovement(cheaperAlternative(t, 100), 1000)

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-actual-elsewhere"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("outcome = %q, want PLAN_LAPSED——建议不是改路", result.Outcome())
	}
	record, _ := result.Record()
	if record.RerouteState != domain.SuggestionOnly {
		t.Fatalf("reroute state = %q", record.RerouteState)
	}
	if !record.HasSuggestion || record.HasDecision {
		t.Fatalf("suggestion = %v decision = %v; 建议与决定互斥", record.HasSuggestion, record.HasDecision)
	}
	if len(record.Suggestion.Blockers()) != 1 || record.Suggestion.Blockers()[0] != "IMPROVEMENT_BELOW_THRESHOLD" {
		t.Fatalf("blockers = %v", record.Suggestion.Blockers())
	}
	if len(record.Suggestion.Candidates()) == 0 {
		t.Fatal("建议没带候选，授权角色无从决定")
	}
}

// Covers: `PAR-NET-16`「并列……交人工裁决，不得任选」在复核这一侧——四个自动条件都立，但最低
// 成本并列选不出唯一一条：不形成自动改路决定也不换新计划，只形成改路建议交授权角色（UC-NR-003
// 7C），阻塞逐家点名并列候选，候选随建议保全。
func TestALowestCostTieOnlyLeavesASuggestion(t *testing.T) {
	fixture := rerouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = declareCostImprovement(tiedEvidence(t), 0)

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-actual-elsewhere"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("outcome = %q, want PLAN_LAPSED——并列不是改路", result.Outcome())
	}
	record, _ := result.Record()
	if record.RerouteState != domain.SuggestionOnly {
		t.Fatalf("reroute state = %q, want SUGGESTION_ONLY", record.RerouteState)
	}
	if !record.HasSuggestion || record.HasDecision || record.HasNewPlan {
		t.Fatalf("suggestion = %v, decision = %v, new plan = %v; 并列只成建议",
			record.HasSuggestion, record.HasDecision, record.HasNewPlan)
	}
	want := map[string]bool{"LOWEST_COST_TIED/candidate-1": true, "LOWEST_COST_TIED/candidate-2": true}
	blockers := record.Suggestion.Blockers()
	if len(blockers) != 2 || !want[blockers[0]] || !want[blockers[1]] {
		t.Fatalf("blockers = %v, want both tied candidates named", blockers)
	}
	if len(record.RerouteBlockers) != 2 {
		t.Fatalf("record blockers = %v; 记录上的阻塞要与建议一致", record.RerouteBlockers)
	}
	if len(record.Suggestion.Candidates()) != 2 {
		t.Fatalf("suggestion candidates = %v; 授权角色要看得见在哪几家之间裁", record.Suggestion.Candidates())
	}
}

// Covers: 硬句「存在未解除硬限制时自动与人工都不能绕过」——禁行只记录禁行依据：结论
// `已失效`、三态为禁行、阻塞点名未解除限制；建议与决定都不形成。
func TestUnresolvedRestrictionsBarRerouteEntirely(t *testing.T) {
	fixture := rerouteFixture(t)
	evidence := declareCostImprovement(cheaperAlternative(t, 100), 0)
	evidence.UnresolvedRestrictions = []domain.RestrictionReference{
		value(t, domain.NewRestrictionReference, "CUSTOMS_HOLD/CC-7"),
	}
	fixture.evidence.byParcel["parcel-1"] = evidence

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-actual-elsewhere"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	record, _ := result.Record()
	if record.RerouteState != domain.RerouteBarred {
		t.Fatalf("reroute state = %q, want BARRED", record.RerouteState)
	}
	if record.HasSuggestion || record.HasDecision {
		t.Fatal("禁行还形成了建议或决定")
	}
	if len(record.RerouteBlockers) != 1 || record.RerouteBlockers[0] != "UNRESOLVED_RESTRICTION/CUSTOMS_HOLD/CC-7" {
		t.Fatalf("blockers = %v", record.RerouteBlockers)
	}
}

// Covers: 策略版本没声明自动改路时只形成建议、不自动，答未配置。不当成允许或不允许。
func TestUndeclaredAutoRerouteOnlySuggests(t *testing.T) {
	fixture := rerouteFixture(t)

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-actual-elsewhere"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	record, _ := result.Record()
	if record.RerouteState != domain.SuggestionOnly {
		t.Fatalf("reroute state = %q, want SUGGESTION_ONLY", record.RerouteState)
	}
	if !record.HasSuggestion || record.HasDecision || record.HasNewPlan {
		t.Fatal("未声明自动改路不得形成决定或新计划")
	}
	if len(record.Suggestion.Blockers()) != 1 || record.Suggestion.Blockers()[0] != "AUTO_REROUTE_UNCONFIGURED" {
		t.Fatalf("blockers = %v", record.Suggestion.Blockers())
	}
	if record.CandidateState != ports.CandidatesAvailable {
		t.Fatalf("candidate state = %q; 候选评估状态照常记录", record.CandidateState)
	}
}
