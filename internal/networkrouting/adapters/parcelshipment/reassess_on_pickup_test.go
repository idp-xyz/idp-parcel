package parcelshipment_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// pickupAdoptedRecord 造一份真经 PS 领域形成的场外揽收采用记录：地点是客户场所，不是物流节点码。
func pickupAdoptedRecord(t *testing.T) psports.IntakeAdoptionRecord {
	t.Helper()
	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind:       psdomain.OffsitePickupSource,
		Object:     value(t, psdomain.NewSourceObjectReference, "parcel-1"),
		Parcel:     value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Place:      value(t, psdomain.NewIntakePlaceReference, "customer-warehouse-2"),
		Control:    value(t, psdomain.NewIntakeControlReference, "OFFSITE-PICKUP/TF-CONTROL-3"),
		Version:    value(t, psdomain.NewSourceResultVersion, "pickup-result/v1"),
		OccurredAt: intakeAt,
	})
	if err != nil {
		t.Fatalf("new pickup source: %v", err)
	}
	intake, err := psdomain.AdoptNetworkIntake(source, value(t, psdomain.NewSubmissionVersionID, "version-1"))
	if err != nil {
		t.Fatalf("adopt network intake: %v", err)
	}
	record := adoptedRecord(t)
	record.Key.Kind = psdomain.OffsitePickupSource
	record.Key.Version = value(t, psdomain.NewSourceResultVersion, "pickup-result/v1")
	record.Intake = intake
	return record
}

type pickupStores struct {
	store         *reassessStoreDouble
	applicability *applicabilityStoreDouble
}

func pickupReassessor(t *testing.T, evidence nrports.InitialRouteEvidence) (*adapter.ReassessOnIntakeAdapter, pickupStores) {
	t.Helper()
	stores := pickupStores{
		store:         &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}},
		applicability: &applicabilityStoreDouble{byPlan: map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability{}},
	}
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes:        &routeStoreDouble{records: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{reassessKey(t): planOnFile(t)}},
		Evidence:      evidenceDouble{evidence: evidence},
		Applicability: stores.applicability,
		Store:         stores.store,
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: intakeAt.Add(2 * time.Minute)},
	})
	return adapter.NewReassessOnIntakeAdapter(handler, value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE")), stores
}

// alternativeEvidence 让计划上的 candidate-1 与一条更便宜的 candidate-2 都在候选空间里，策略版本声明成本改善自动改路、
// 阈值 0（本测试交入的租户取值）。冻结形态不声明：揽收复核不判冻结，没声明也不该落未判。
func alternativeEvidence(t *testing.T) nrports.InitialRouteEvidence {
	t.Helper()
	plan := planOnFile(t).Plan
	threshold := 0
	evidence := nrports.InitialRouteEvidence{
		Strategy:                             value(t, nrdomain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision:                         value(t, nrdomain.NewNetworkViewRevision, "net-view-rev-1"),
		RankingForm:                          nrdomain.CostSingleDimensionRanking,
		AutoRerouteForm:                      nrdomain.CostImprovementAutoReroute,
		AutoRerouteImprovementThresholdMinor: &threshold,
	}
	for candidate, amount := range map[string]int64{"candidate-1": 1000, "candidate-2": 100} {
		area, err := nrdomain.NewServiceAreaResolution(nrdomain.ServiceAreaResolutionSpec{
			Candidate:   value(t, nrdomain.NewCandidateID, candidate),
			Outcome:     nrdomain.AreaCoversDestination,
			AreaVersion: value(t, nrdomain.NewServiceAreaVersionReference, "AREA-V1"),
		})
		if err != nil {
			t.Fatalf("new covering area: %v", err)
		}
		cost, err := nrdomain.NewPricedCandidateCost(value(t, nrdomain.NewCandidateID, candidate), amount, "CNY")
		if err != nil {
			t.Fatalf("new candidate cost: %v", err)
		}
		evidence.ServiceAreas = append(evidence.ServiceAreas, area)
		evidence.CandidateCosts = append(evidence.CandidateCosts, cost)
		evidence.Paths = append(evidence.Paths, nrports.CandidatePath{
			Candidate: value(t, nrdomain.NewCandidateID, candidate),
			Legs:      plan.NodeLegs(),
		})
	}
	return evidence
}

// Covers: AT-NR-034 经收寄采用一路端到端（ADR-0249）——场外揽收的采用记录译成揽收控制依据（不再译成权威运输交接），
// 地点原样作复核位置；它不在计划节点上，原计划仍适用；不判冻结、不落未判，也不因更便宜的候选改路。
func TestAnAdoptedOffsitePickupKeepsThePlanWhoseHardConstraintsHold(t *testing.T) {
	subject, stores := pickupReassessor(t, alternativeEvidence(t))

	result, err := subject.TriggerReassessment(context.Background(), pickupAdoptedRecord(t), nrports.RequestCarriedContent{})
	if err != nil {
		t.Fatalf("trigger reassessment: %v", err)
	}
	if result.Outcome() != nrapplication.ReassessedStillApplicable {
		t.Fatalf("outcome = %q reason = %q, want STILL_APPLICABLE——揽收地点不与计划节点比",
			result.Outcome(), result.NotJudgedReason())
	}
	record, _ := result.Record()
	if record.ReviewedPlan.String() != "plan-1/v1" || record.HasNewPlan || record.HasDecision {
		t.Fatalf("record = %#v; 揽收复核保留原计划，不试改善切换", record)
	}
	if len(stores.store.notJudged) != 0 {
		t.Fatalf("未判登记 = %+v; 揽收复核不判冻结", stores.store.notJudged)
	}
}

// Covers: AT-NR-084 经收寄采用一路端到端（ADR-0249）——揽收时原计划被有效硬限制淘汰、策略允许自动改路：原计划失效，
// 只形成改路建议，阻塞只有 NOT_AT_CONTROLLED_NODE，不得出 EXECUTED_PREFIX_AFFECTED。
func TestAnAdoptedOffsitePickupWithARestrictedPlanOnlySuggests(t *testing.T) {
	evidence := alternativeEvidence(t)
	finding, err := nrdomain.NewHardConstraintFinding(nrdomain.HardConstraintFindingSpec{
		Candidate:   value(t, nrdomain.NewCandidateID, "candidate-1"),
		Outcome:     nrdomain.RestrictionApplies,
		Restriction: value(t, nrdomain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/CC-REF-9"),
	})
	if err != nil {
		t.Fatalf("new restriction finding: %v", err)
	}
	evidence.HardConstraints = []nrdomain.HardConstraintFinding{finding}
	subject, _ := pickupReassessor(t, evidence)

	result, err := subject.TriggerReassessment(context.Background(), pickupAdoptedRecord(t), nrports.RequestCarriedContent{})
	if err != nil {
		t.Fatalf("trigger reassessment: %v", err)
	}
	if result.Outcome() != nrapplication.ReassessedPlanLapsed {
		t.Fatalf("outcome = %q, want PLAN_LAPSED", result.Outcome())
	}
	record, _ := result.Record()
	if record.LapseBasis.String() != "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-ELIGIBILITY/CC-REF-9" {
		t.Fatalf("lapse basis = %q", record.LapseBasis)
	}
	if !record.HasSuggestion || record.HasDecision {
		t.Fatalf("suggestion = %v decision = %v; 揽收时只形成建议", record.HasSuggestion, record.HasDecision)
	}
	if blockers := record.Suggestion.Blockers(); len(blockers) != 1 || blockers[0] != "NOT_AT_CONTROLLED_NODE" {
		t.Fatalf("blockers = %v, want [NOT_AT_CONTROLLED_NODE]", blockers)
	}
}
