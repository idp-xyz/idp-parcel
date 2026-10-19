package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉编排一侧的交付段（ADR-0217，票 demo-chain-gaps/64）：选中候选的交付段随计划一并形成，改路形成的新计划
// 同样带上；复核的冻结边界只数节点间段。证据视图怎样折出交付段由真库用例钉（adapters/postgres）。

// withDeliveryLeg 给证据里每条候选路径接上一段交付段：起于末个节点，止于 AREA-V1。
func withDeliveryLeg(t *testing.T, evidence ports.InitialRouteEvidence) ports.InitialRouteEvidence {
	t.Helper()
	paths := make([]ports.CandidatePath, 0, len(evidence.Paths))
	for _, path := range evidence.Paths {
		last := path.Legs[len(path.Legs)-1]
		window, err := domain.NewPlannedTimeWindow(last.Window().Latest(), last.Window().Latest().Add(3*time.Hour),
			value(t, domain.NewWindowBasisReference, "NODE/"+last.To().String()+"@2"))
		if err != nil {
			t.Fatalf("交付段窗口：%v", err)
		}
		delivery, err := domain.NewDeliveryLeg(domain.DeliveryLegSpec{
			From:        last.To(),
			Scope:       value(t, domain.NewServiceAreaVersionReference, "AREA-V1"),
			Responsible: value(t, domain.NewResponsiblePartyReference, "AREA-V1"),
			Window:      window,
		})
		if err != nil {
			t.Fatalf("交付段：%v", err)
		}
		path.Delivery, path.HasDelivery = delivery, true
		paths = append(paths, path)
	}
	evidence.Paths = paths
	return evidence
}

// Covers: AT-NR-054 的编排半边——选中候选带交付段时，计划段链是节点间段接交付段；交付段是计划的末段。
func TestTheSelectedPathCarriesItsDeliveryLegIntoThePlan(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = withDeliveryLeg(t, routableEvidence(t))

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	plan, formed := result.Parcels()[0].Plan()
	if !formed {
		t.Fatalf("outcome = %q, want 形成计划", result.Parcels()[0].Outcome())
	}
	legs := plan.Legs()
	if len(legs) != 2 || legs[0].Kind() != domain.BetweenNodesLeg || legs[1].Kind() != domain.DeliveryLeg {
		t.Fatalf("段链 = %+v，想要一段节点间段接一段交付段", legs)
	}
	if scope, _ := legs[1].DeliveryScope(); scope.String() != "AREA-V1" || legs[1].From().String() != "node-destination" {
		t.Fatalf("交付段 = %s → %s", legs[1].From(), scope)
	}
	if nodes := plan.Nodes(); len(nodes) != 2 {
		t.Fatalf("计划节点 = %v，交付范围不进节点序列", nodes)
	}
}

// Covers: AT-NR-059——带交付段的计划在复核时剩余段数只数节点间段：起点节点上剩一段节点间段，冻结段数 1 即越过冻结
// 边界，成本改善不改路；把交付段也数进去会剩两段、不冻结，换成改路。
func TestTheFreezeBoundaryCountsOnlyLegsBetweenNodes(t *testing.T) {
	fixture := newReassessFixture(t)
	evidence := withDeliveryLeg(t, routableEvidence(t))
	plan, err := domain.FormInitialRoutePlan(domain.InitialRoutePlanSpec{
		Key:           reassessKey(t),
		Version:       value(t, domain.NewRoutePlanVersionID, "plan-1/v1"),
		Selected:      value(t, domain.NewCandidateID, "candidate-1"),
		Candidates:    []domain.RouteCandidate{mustQualified(t, "candidate-1")},
		Legs:          evidence.Paths[0].PlanLegs(),
		Strategy:      evidence.Strategy,
		ViewRevision:  evidence.ViewRevision,
		JudgedAt:      reassessedAt.Add(-time.Hour),
		EffectiveFrom: reassessedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("form plan: %v", err)
	}
	fixture.routes.records[reassessKey(t)] = ports.InitialRouteRecord{Key: reassessKey(t), Plan: plan, HasPlan: true}

	improved := withDeliveryLeg(t, declareCostImprovement(cheaperAlternative(t, 100), 0))
	limit := 1
	improved.FreezeForm = domain.RemainingSegmentCountFreeze
	improved.FreezeRemainingSegmentLimit = &limit
	fixture.evidence.byParcel["parcel-1"] = improved

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE——交付段不算剩余段", result.Outcome())
	}
}

// Covers: AT-NR-054 的改路半边——复核形成的新计划同样取被选候选的整条段链，交付段跟着新计划走。
func TestAReroutedPlanCarriesTheDeliveryLegOfItsCandidate(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	evidence := withDeliveryLeg(t, declareCostImprovement(cheaperAlternative(t, 100), 0))
	limit := 0
	evidence.FreezeForm = domain.RemainingSegmentCountFreeze
	evidence.FreezeRemainingSegmentLimit = &limit
	fixture.evidence.byParcel["parcel-1"] = evidence

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReassessedRerouted {
		t.Fatalf("outcome = %q, want REROUTED", result.Outcome())
	}
	switched, _ := result.Record()
	legs := switched.NewPlan.Legs()
	if len(legs) != 2 || legs[1].Kind() != domain.DeliveryLeg {
		t.Fatalf("新计划段链 = %+v，想要末段是交付段", legs)
	}
}
