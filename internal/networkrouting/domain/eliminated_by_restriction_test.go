package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// Covers: ADR-0284 决定二——授权改路决定重判所择候选时，「被有效硬限制淘汰」与其余层次的淘汰是两格，续办不同：前者要限制的
// 来源责任方解除，后者是另择一条。只有硬约束那一层留下的淘汰算前者；合格与证据未知都不算。
func TestOnlyAHardConstraintRestrictionCountsAsEliminatedByRestriction(t *testing.T) {
	candidates := evaluatedCandidates(t,
		coveringResolution(t, "cand-restricted"),
		coveringResolution(t, "cand-unknown"),
		coveringResolution(t, "cand-open"),
	)
	evaluated, _, err := domain.EvaluateHardConstraints(candidates, []domain.HardConstraintFinding{
		restrictionApplies(t, "cand-restricted", "CUSTOMS-ELIGIBILITY/CC-REF-9"),
		constraintUnknown(t, "cand-unknown"),
		constraintSatisfied(t, "cand-open"),
	})
	if err != nil {
		t.Fatalf("evaluate hard constraints: %v", err)
	}
	want := map[string]bool{"cand-restricted": true, "cand-unknown": false, "cand-open": false}
	for _, candidate := range evaluated {
		if got := candidate.EliminatedByRestriction(); got != want[candidate.ID().String()] {
			t.Errorf("%s EliminatedByRestriction = %v, want %v", candidate.ID(), got, want[candidate.ID().String()])
		}
	}

	// 别的层次淘汰的候选不算，哪怕原因串里提到限制。
	other, err := domain.NewRouteCandidate(mustValue(t, domain.NewCandidateID, "cand-late"), domain.CandidateEliminated,
		mustValue(t, domain.NewCandidateReason, "TIME_COMMITMENT_NOT_MET/HARD_CONSTRAINT_RESTRICTION"))
	if err != nil {
		t.Fatalf("new route candidate: %v", err)
	}
	if other.EliminatedByRestriction() {
		t.Error("时效层淘汰被读成了限制淘汰")
	}
}
