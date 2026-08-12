package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

var reroutedAt = time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)

func allAutoFacts() domain.AutoRerouteFacts {
	return domain.AutoRerouteFacts{
		PolicyAllowsAutomatic:  true,
		AtControlledNode:       true,
		OnlyUnexecutedAffected: true,
	}
}

// Covers: CONTEXT「符合版本化策略、包裹位于当前可控节点、只改变尚未执行部分，并且不
// 存在未解除的硬限制或未经所属上下文处理的……既有责任时，可以自动形成改路决定；不满足
// 自动改路条件时，只形成改路建议」——四条件全真才自动，任一不满足降为建议并说得出缺什么。
func TestAutoRerouteDemandsEveryConditionTogether(t *testing.T) {
	authority, blockers := domain.EvaluateAutoRerouteConditions(allAutoFacts())
	if authority != domain.AutomaticRerouteAllowed || len(blockers) != 0 {
		t.Fatalf("authority = %q blockers = %v, want AUTOMATIC_ALLOWED", authority, blockers)
	}

	offNode := allAutoFacts()
	offNode.AtControlledNode = false
	authority, blockers = domain.EvaluateAutoRerouteConditions(offNode)
	if authority != domain.SuggestionOnly || len(blockers) != 1 || blockers[0] != "NOT_AT_CONTROLLED_NODE" {
		t.Fatalf("authority = %q blockers = %v, want SUGGESTION_ONLY/NOT_AT_CONTROLLED_NODE", authority, blockers)
	}

	busy := allAutoFacts()
	busy.OutstandingResponsibilities = []domain.ResponsibilityReference{
		mustValue(t, domain.NewResponsibilityReference, "BOOKING/TF-9"),
	}
	authority, blockers = domain.EvaluateAutoRerouteConditions(busy)
	if authority != domain.SuggestionOnly || blockers[0] != "OUTSTANDING_RESPONSIBILITY/BOOKING/TF-9" {
		t.Fatalf("authority = %q blockers = %v; 既有责任未处理不得自动改路", authority, blockers)
	}
}

// Covers: CONTEXT「人工决定同样不得绕过硬限制」——未解除限制在场即`禁行`：自动与人工
// 两种方式的决定都立不成，建议清单指名每条限制。
func TestUnresolvedRestrictionsBarBothModes(t *testing.T) {
	barred := allAutoFacts()
	barred.UnresolvedRestrictions = []domain.RestrictionReference{
		mustValue(t, domain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/CC-REF-9"),
	}
	authority, blockers := domain.EvaluateAutoRerouteConditions(barred)
	if authority != domain.RerouteBarred || blockers[0] != "UNRESOLVED_RESTRICTION/CUSTOMS-ELIGIBILITY/CC-REF-9" {
		t.Fatalf("authority = %q blockers = %v, want BARRED", authority, blockers)
	}

	spec := rerouteSpec(t, authority, domain.AutomaticReroute)
	if _, err := domain.FormRerouteDecision(spec); !errors.Is(err, domain.ErrRerouteBarred) {
		t.Fatalf("err = %v, want ErrRerouteBarred", err)
	}
	spec.Mode = domain.AuthorizedRoleReroute
	spec.DecidedBy = mustValue(t, domain.NewAuthorizedRoleReference, "OPS-ROLE-1")
	if _, err := domain.FormRerouteDecision(spec); !errors.Is(err, domain.ErrRerouteBarred) {
		t.Fatalf("err = %v; 人工决定绕过了硬限制", err)
	}
}

// Covers: CONTEXT「任何改路都保留原计划、触发原因、输入依据、候选、决定方式及生效边界」
// 与「改路决定：针对尚未执行旅程形成的新路由版本及其业务生效边界……保留与原计划的替代
// 关系」——决定携带方式、触发、替代关系与生效节点（新计划段链首节点）；自动方式只在
// 条件全真时可用，人工方式必须指名角色；新旧版本不得重号。
func TestARerouteDecisionKeepsItsModeBasisAndBoundary(t *testing.T) {
	automatic, err := domain.FormRerouteDecision(rerouteSpec(t, domain.AutomaticRerouteAllowed, domain.AutomaticReroute))
	if err != nil {
		t.Fatalf("form automatic decision: %v", err)
	}
	if automatic.Mode() != domain.AutomaticReroute {
		t.Fatalf("mode = %q", automatic.Mode())
	}
	if automatic.EffectiveFromNode().String() != "node-origin" {
		t.Fatalf("effective from = %s; 生效边界是新计划段链首节点", automatic.EffectiveFromNode())
	}
	if automatic.OriginalPlan().String() != "plan-1/v1" || automatic.NewPlan().Version().String() != "plan-1/v2" {
		t.Fatal("替代关系没有随决定保全")
	}

	if _, err := domain.FormRerouteDecision(rerouteSpec(t, domain.SuggestionOnly, domain.AutomaticReroute)); !errors.Is(err, domain.ErrAutomaticRerouteNotAllowed) {
		t.Fatalf("err = %v; 条件不满足只形成建议，自动决定立不成", err)
	}

	manual := rerouteSpec(t, domain.SuggestionOnly, domain.AuthorizedRoleReroute)
	if _, err := domain.FormRerouteDecision(manual); !errors.Is(err, domain.ErrInvalidReroute) {
		t.Fatalf("err = %v; 人工决定没指名角色", err)
	}
	manual.DecidedBy = mustValue(t, domain.NewAuthorizedRoleReference, "OPS-ROLE-1")
	decided, err := domain.FormRerouteDecision(manual)
	if err != nil {
		t.Fatalf("form manual decision: %v", err)
	}
	role, present := decided.DecidedBy()
	if !present || role.String() != "OPS-ROLE-1" {
		t.Fatal("实际决定方没有随决定保全")
	}

	samePlan := rerouteSpec(t, domain.AutomaticRerouteAllowed, domain.AutomaticReroute)
	samePlan.OriginalPlan = samePlan.NewPlan.Version()
	if _, err := domain.FormRerouteDecision(samePlan); !errors.Is(err, domain.ErrInvalidReroute) {
		t.Fatalf("err = %v; 新旧版本重号分不出两代", err)
	}
}

// Covers: CONTEXT「改路建议：候选处置意见，建议本身不修改当前有效路由」——建议是纯记录，
// 候选与「为什么没自动」清单必备，授权角色才有据以决定的材料。
func TestASuggestionCarriesCandidatesAndItsBlockers(t *testing.T) {
	suggestion, err := domain.NewRerouteSuggestion(domain.RerouteSuggestionSpec{
		Key:         judgmentKeyFor(t, "parcel-1"),
		Trigger:     mustValue(t, domain.NewRerouteTriggerReference, "LINE-CLOSED/NET-ADJ-7"),
		Candidates:  []domain.RouteCandidate{qualifiedRouteCandidate(t, "cand-alt")},
		Blockers:    []string{"NOT_AT_CONTROLLED_NODE"},
		SuggestedAt: reroutedAt,
	})
	if err != nil {
		t.Fatalf("new suggestion: %v", err)
	}
	if len(suggestion.Candidates()) != 1 || suggestion.Blockers()[0] != "NOT_AT_CONTROLLED_NODE" {
		t.Fatalf("suggestion = %#v", suggestion)
	}

	if _, err := domain.NewRerouteSuggestion(domain.RerouteSuggestionSpec{
		Key:         judgmentKeyFor(t, "parcel-1"),
		Trigger:     mustValue(t, domain.NewRerouteTriggerReference, "LINE-CLOSED/NET-ADJ-7"),
		Candidates:  []domain.RouteCandidate{qualifiedRouteCandidate(t, "cand-alt")},
		SuggestedAt: reroutedAt,
	}); !errors.Is(err, domain.ErrInvalidReroute) {
		t.Fatalf("err = %v; 说不出为什么没自动的建议立不起来", err)
	}
}

func rerouteSpec(
	t *testing.T,
	authority domain.RerouteAuthority,
	mode domain.RerouteDecisionMode,
) domain.RerouteDecisionSpec {
	t.Helper()
	newPlan := planSpec(t)
	newPlan.Version = mustValue(t, domain.NewRoutePlanVersionID, "plan-1/v2")
	plan, err := domain.FormInitialRoutePlan(newPlan)
	if err != nil {
		t.Fatalf("form new plan: %v", err)
	}
	return domain.RerouteDecisionSpec{
		Authority:    authority,
		Mode:         mode,
		Trigger:      mustValue(t, domain.NewRerouteTriggerReference, "LINE-CLOSED/NET-ADJ-7"),
		OriginalPlan: mustValue(t, domain.NewRoutePlanVersionID, "plan-1/v1"),
		NewPlan:      plan,
		DecidedAt:    reroutedAt,
	}
}
