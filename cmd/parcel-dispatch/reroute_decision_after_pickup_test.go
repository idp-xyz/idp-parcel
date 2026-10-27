package main

import (
	"context"
	"testing"
	"time"

	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 证授权改路决定接在复核两路之间（ADR-0284；票 docs-consistency/code/29）：揽收复核在失效线上形成改路
// 建议，授权角色据它择一候选形成决定，包裹随后交进节点时，交接复核从决定形成的计划起判。复核两路与生产同形，只换证据视图与
// 成本取数侧；决定编排的判断库各册、受理基线地理投影读口（经 PS 委托仓储）与生产同一份，parcel-api 那一侧的生产装配另有用例。
// 夹具全部合成，只记 `S`。

type fixedDecisionClock struct{ at time.Time }

func (clock fixedDecisionClock) Now() time.Time { return clock.at }

// Covers: AT-NR-084 的决定那一半与 AT-NR-095（ADR-0249 决定二、ADR-0284 决定三、四、五）——揽收时原计划所选的候选被有效硬限制淘汰，另一条
// 候选合格，策略版本没声明自动改路：复核只形成建议。授权角色在包裹交进首个节点之前决定：选被淘汰的那一条答不在建议之列；选
// 合格的那一条形成授权角色方式的改路决定，新计划从决定的时刻起生效、首节点是包裹将交进的枢纽，证据按受理基线那一版投影取。
// 包裹随后交进新加坡枢纽，交接复核的起点是决定形成的计划，仍适用、引用它。
func TestARerouteDecisionOnAPickupSuggestionBecomesTheNextStartingPointOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	declareOneRestrictedOfTwoCandidates(t, fixture)

	pickup := fixture.intakeAt(t, psdomain.OffsitePickupSource, pickupReassessVersion, pickupReassessPlace, pickupReassessAt)
	deliver(t, fixture.syn, fixture.intake, pickup, pickup)
	lapsed := reassessmentOf(t, fixture.syn, pickupReassessCorrel)
	if lapsed.Conclusion != nrports.ReassessmentPlanLapsed || !lapsed.HasSuggestion {
		t.Fatalf("揽收复核 = %s，建议 %v, want PLAN_LAPSED 带建议", lapsed.Conclusion, lapsed.HasSuggestion)
	}

	decidedAt := pickupReassessAt.Add(time.Hour)
	decider := rerouteDecisionHandlerOn(t, fixture, decidedAt)
	command := nrapplication.DecideRerouteCommand{
		Tenant:     mustNR(t, nrdomain.NewTenantID, "SYN-TENANT-01"),
		Parcel:     mustNR(t, nrdomain.NewDeclaredParcelID, reassessHandoverParcel),
		Suggestion: mustNR(t, nrdomain.NewRequestCorrelationID, pickupReassessCorrel),
		Candidate:  mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		Basis:      mustNR(t, nrdomain.NewRerouteDecisionBasisReference, "SYN-APPROVAL-29"),
		DecidedBy:  mustNR(t, nrdomain.NewAuthorizedRoleReference, "OPERATOR/http://127.0.0.1:5556/dex#SYN-OPERATOR-CONFIG-01"),
	}
	if refused := decideWithin(t, fixture, decider, command); refused.Outcome() != nrapplication.RerouteCandidateNotSuggested {
		t.Fatalf("选建议里被淘汰的候选：outcome = %q, want CANDIDATE_NOT_SUGGESTED", refused.Outcome())
	}
	command.Candidate = mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-02")
	result := decideWithin(t, fixture, decider, command)
	if result.Outcome() != nrapplication.RerouteDecided {
		t.Fatalf("outcome = %q reason = %q, want REROUTE_DECIDED", result.Outcome(), result.UndecidedReason())
	}
	recorded, _ := result.Decision()
	plan := recorded.Decision.NewPlan()
	if recorded.Decision.Mode() != nrdomain.AuthorizedRoleReroute || recorded.Decision.OriginalPlan().String() != "SYN-RPV-63" ||
		plan.SelectedCandidate().String() != "SYN-CANDIDATE-02" || !plan.EffectiveFrom().Equal(decidedAt) ||
		recorded.Decision.EffectiveFromNode().String() != "SYN-NODE-SHA-HUB" {
		t.Fatalf("决定 = %+v", recorded)
	}
	decisionGeo := fixture.evidence.seen[len(fixture.evidence.seen)-1].Geo

	onward := fixture.handoverInto(t, "after-decision", reassessHandoverReceiving, reassessHandoverAt)
	deliver(t, fixture.syn, fixture.handover, onward, onward)
	kept := reassessmentOf(t, fixture.syn, handoverCorrelation("after-decision"))
	if kept.Conclusion != nrports.ReassessmentStillApplicable || kept.ReviewedPlan != plan.Version() {
		t.Fatalf("交接复核 = %s / %s, want STILL_APPLICABLE、复核决定形成的 %s", kept.Conclusion, kept.ReviewedPlan, plan.Version())
	}
	// 决定与交接复核经同一个读口取受理基线那一版投影，两次交到证据视图的是同一份。
	if handoverGeo := fixture.evidence.seen[len(fixture.evidence.seen)-1].Geo; !decisionGeo.Carried() ||
		decisionGeo.ContentDigest() != handoverGeo.ContentDigest() {
		t.Fatalf("决定取证据的投影 = %s（带了 %v）, want 与交接复核同一份 %s",
			decisionGeo.ContentDigest(), decisionGeo.Carried(), handoverGeo.ContentDigest())
	}
}

// declareOneRestrictedOfTwoCandidates 让复核夹具的证据有两条都可行的候选（01 比 02 便宜），其中 01 被有效硬限制淘汰：原计划所选
// 的是 01 时，复核判出失效、只形成建议（策略版本没声明自动改路），建议里只有 02。
func declareOneRestrictedOfTwoCandidates(t *testing.T, fixture *reassessBothLanesFixture) {
	t.Helper()
	evidence := &fixture.evidence.evidence
	evidence.RankingForm = nrdomain.CostSingleDimensionRanking
	for _, candidate := range []struct {
		id     string
		amount int64
	}{{"SYN-CANDIDATE-01", 4200}, {"SYN-CANDIDATE-02", 5100}} {
		area, err := nrdomain.NewServiceAreaResolution(nrdomain.ServiceAreaResolutionSpec{
			Candidate:   mustNR(t, nrdomain.NewCandidateID, candidate.id),
			Outcome:     nrdomain.AreaCoversDestination,
			AreaVersion: mustNR(t, nrdomain.NewServiceAreaVersionReference, "SYN-AREA/v1"),
		})
		if err != nil {
			t.Fatalf("服务区域：%v", err)
		}
		cost, err := nrdomain.NewPricedCandidateCost(mustNR(t, nrdomain.NewCandidateID, candidate.id), candidate.amount, "CNY")
		if err != nil {
			t.Fatalf("候选成本：%v", err)
		}
		evidence.ServiceAreas = append(evidence.ServiceAreas, area)
		evidence.CandidateCosts = append(evidence.CandidateCosts, cost)
		evidence.Paths = append(evidence.Paths, nrports.CandidatePath{
			Candidate: mustNR(t, nrdomain.NewCandidateID, candidate.id),
			Legs:      legsThrough(t, "SYN-NODE-SHA-HUB", reassessHandoverReceiving, "SYN-NODE-SIN-LM"),
		})
	}
	restriction, err := nrdomain.NewHardConstraintFinding(nrdomain.HardConstraintFindingSpec{
		Candidate:   mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		Outcome:     nrdomain.RestrictionApplies,
		Restriction: mustNR(t, nrdomain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/SYN-CC-29"),
	})
	if err != nil {
		t.Fatalf("硬限制：%v", err)
	}
	evidence.HardConstraints = []nrdomain.HardConstraintFinding{restriction}
}

// rerouteDecisionHandlerOn 用生产的判断库各册与受理基线地理投影读口装一份决定编排，证据视图与成本取数侧取复核夹具那一份。
func rerouteDecisionHandlerOn(t *testing.T, fixture *reassessBothLanesFixture, at time.Time) *nrapplication.DecideRerouteHandler {
	t.Helper()
	db := fixture.syn.db
	reassessments, err := nrpostgres.NewRouteReassessments(db)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := nrpostgres.NewAuthorizedRerouteDecisions(db)
	if err != nil {
		t.Fatal(err)
	}
	applicabilities, err := nrpostgres.NewPlanApplicabilities(db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := nrpostgres.NewRouteIdentities(db)
	if err != nil {
		t.Fatal(err)
	}
	baselines, err := nrparcelshipment.NewAcceptedCarriedGeoSource(fixture.syn.requests)
	if err != nil {
		t.Fatal(err)
	}
	return nrapplication.NewDecideRerouteHandler(nrapplication.DecideRerouteDeps{
		Reassessments: reassessments,
		Decisions:     decisions,
		Applicability: applicabilities,
		Baselines:     baselines,
		Evidence:      fixture.evidence,
		Costs:         fixture.costs,
		Identities:    identities,
		Clock:         fixedDecisionClock{at: at},
	})
}

func decideWithin(
	t *testing.T,
	fixture *reassessBothLanesFixture,
	decider *nrapplication.DecideRerouteHandler,
	command nrapplication.DecideRerouteCommand,
) nrapplication.DecideRerouteResult {
	t.Helper()
	var result nrapplication.DecideRerouteResult
	mustWithinTX(t, fixture.syn.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = decider.Decide(txCtx, command)
		return err
	})
	return result
}
