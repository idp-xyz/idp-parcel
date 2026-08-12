package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

type autoRerouteFactsDouble struct {
	facts      domain.AutoRerouteFacts
	configured bool
	err        error
}

func (double *autoRerouteFactsDouble) LoadAutoRerouteFacts(
	_ context.Context,
	_ domain.InitialRouteJudgmentKey,
) (domain.AutoRerouteFacts, bool, error) {
	if double.err != nil {
		return domain.AutoRerouteFacts{}, false, double.err
	}
	return double.facts, double.configured, nil
}

// rerouteFixture 在标准复核夹具上接入自动改路事实缝，并预置「计划失效+候选可用」的
// 前提（实际位置不在计划上、证据可选出 candidate-1）。
func rerouteFixture(t *testing.T, facts *autoRerouteFactsDouble) *reassessFixture {
	t.Helper()
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
	fixture.handler = application.NewReassessRouteHandler(application.ReassessRouteDeps{
		Routes:        fixture.routes,
		Evidence:      fixture.evidence,
		Applicability: fixture.applicability,
		Store:         fixture.store,
		Log:           fixture.log,
		Identities:    &routeIdentityDouble{},
		Clock:         fixedClock{at: reassessedAt},
		AutoReroute:   facts,
	})
	return fixture
}

func allConditionsMet() domain.AutoRerouteFacts {
	return domain.AutoRerouteFacts{
		PolicyAllowsAutomatic:  true,
		AtControlledNode:       true,
		OnlyUnexecutedAffected: true,
	}
}

// Covers: UC-NR-003 7B「四个条件同时满足才允许自动改路」的编排面——失效已定、候选可用
// 且条件全立时，同一份复核记录升格为`已改路`：自动模式决定在场、新计划版本不同于原
// 计划、原计划的失效照常落库（改路不豁免失效）。
func TestAllConditionsMetFormsAnAutomaticRerouteDecision(t *testing.T) {
	facts := &autoRerouteFactsDouble{facts: allConditionsMet(), configured: true}
	fixture := rerouteFixture(t, facts)

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

// Covers: 7C「不满足自动条件时形成带候选与阻塞清单的改路建议，等授权角色决定」——
// 政策不允许自动：结论仍是`已失效`，建议在场、阻塞点名 POLICY_DOES_NOT_ALLOW_AUTOMATIC、
// 候选随建议保全；不形成决定。
func TestUnmetConditionsLeaveASuggestionForTheAuthorizedRole(t *testing.T) {
	facts := allConditionsMet()
	facts.PolicyAllowsAutomatic = false
	fixture := rerouteFixture(t, &autoRerouteFactsDouble{facts: facts, configured: true})

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
	if len(record.Suggestion.Blockers()) != 1 || record.Suggestion.Blockers()[0] != "POLICY_DOES_NOT_ALLOW_AUTOMATIC" {
		t.Fatalf("blockers = %v", record.Suggestion.Blockers())
	}
	if len(record.Suggestion.Candidates()) == 0 {
		t.Fatal("建议没带候选，授权角色无从决定")
	}
}

// Covers: 硬句「存在未解除硬限制时自动与人工都不能绕过」——禁行只记录禁行依据：结论
// `已失效`、三态为禁行、阻塞点名未解除限制；建议与决定都不形成。
func TestUnresolvedRestrictionsBarRerouteEntirely(t *testing.T) {
	facts := allConditionsMet()
	facts.UnresolvedRestrictions = []domain.RestrictionReference{
		value(t, domain.NewRestrictionReference, "CUSTOMS_HOLD/CC-7"),
	}
	fixture := rerouteFixture(t, &autoRerouteFactsDouble{facts: facts, configured: true})

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

// Covers: 实例半边纪律——自动改路事实目录未配置：失效照常落库，改路评估整段不做（三态
// 保持未评估、无建议无决定），不猜政策也不猜节点受控性。
func TestUnconfiguredRerouteFactsFallBackToAPureLapse(t *testing.T) {
	fixture := rerouteFixture(t, &autoRerouteFactsDouble{configured: false})

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-actual-elsewhere"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	record, _ := result.Record()
	if record.RerouteState != domain.RerouteAuthorityInvalid {
		t.Fatalf("reroute state = %q; 未配置不得给出三态判定", record.RerouteState)
	}
	if record.HasSuggestion || record.HasDecision || record.HasNewPlan {
		t.Fatal("未配置的事实缝长出了建议、决定或新计划")
	}
	if record.CandidateState != ports.CandidatesAvailable {
		t.Fatalf("candidate state = %q; 候选评估状态照常记录", record.CandidateState)
	}
}
