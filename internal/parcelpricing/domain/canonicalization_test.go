package domain

import "testing"

// ADR-0014 makes a content digest comparable only within the canonicalization
// version that produced it, so an evaluation that does not record which shape
// it used has an incomplete digest: nothing later can tell whether a difference
// means changed content or a changed shape.
func TestEvaluationRecordsTheCanonicalizationVersionThatProducedItsDigest(t *testing.T) {
	plan, input := replayIntegrityFixture(t)
	id, _ := NewEvaluationID("eval-canonicalization-recorded")
	request, err := NewEvaluationRequest(id, plan, input, EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	evaluation := EvaluatePricing(request)
	if plan.CanonicalizationVersion() != CurrentCanonicalizationVersion() {
		t.Fatalf("plan canonicalization = %q, want %q", plan.CanonicalizationVersion(), CurrentCanonicalizationVersion())
	}
	if evaluation.PlanCanonicalizationVersion() != plan.CanonicalizationVersion() {
		t.Fatalf("evaluation recorded %q for a plan canonicalized as %q", evaluation.PlanCanonicalizationVersion(), plan.CanonicalizationVersion())
	}
}

// An evaluation saved by an earlier build carries that build's shape, so its
// digest and today's are not comparable and a difference between them is no
// evidence that the plan's content changed. Reporting a version content
// conflict here would condemn every historical replay the moment the shape
// widens, which is the outcome ADR-0014 exists to prevent.
func TestReplayDoesNotReadACrossVersionDigestAsChangedContent(t *testing.T) {
	plan, input := replayIntegrityFixture(t)
	originalID, _ := NewEvaluationID("eval-canonicalization-original")
	request, err := NewEvaluationRequest(originalID, plan, input, EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	original := EvaluatePricing(request)
	original.planCanonicalization = "PPC-0"
	original.planContentDigest = "digest-produced-under-PPC-0"
	if !original.valid() {
		t.Fatal("an evaluation saved under an earlier shape must still be structurally valid")
	}

	replayID, _ := NewEvaluationID("eval-canonicalization-replay")
	replayed, err := ReplayPricingEvaluation(replayID, original, plan, EvidenceSynthetic)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.Status() == EvaluationConflict {
		t.Fatalf("cross-version replay was reported as a conflict: %#v", replayed.Issues())
	}
	for _, issue := range replayed.Issues() {
		if issue.Code() == "PLAN_CONTENT_MISMATCH" || issue.Code() == "REPLAY_RESULT_MISMATCH" {
			t.Fatalf("cross-version digest was read as changed content: %s", issue.Code())
		}
	}
	// This build can only canonicalize under the current shape, so it says it
	// cannot perform the replay rather than claiming a faithful one.
	if replayed.Status() != EvaluationFailed || len(replayed.Issues()) != 1 || replayed.Issues()[0].Code() != "CANONICALIZATION_VERSION_UNSUPPORTED" {
		t.Fatalf("cross-version replay = %s %#v", replayed.Status(), replayed.Issues())
	}
}

// Within one canonicalization version the digest keeps its original job:
// changed content behind an unchanged version reference is still a conflict.
// The version must widen what a digest difference can mean, not weaken it.
func TestReplayStillReportsChangedContentWithinOneCanonicalizationVersion(t *testing.T) {
	plan, input := replayIntegrityFixture(t)
	originalID, _ := NewEvaluationID("eval-same-version-original")
	request, err := NewEvaluationRequest(originalID, plan, input, EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	original := EvaluatePricing(request)
	original.planContentDigest = "digest-produced-by-different-content"
	original.semanticDigest = original.calculateSemanticDigest()

	replayID, _ := NewEvaluationID("eval-same-version-replay")
	replayed, err := ReplayPricingEvaluation(replayID, original, plan, EvidenceSynthetic)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.Status() != EvaluationConflict || len(replayed.Issues()) != 1 || replayed.Issues()[0].Code() != "PLAN_CONTENT_MISMATCH" {
		t.Fatalf("same-version replay = %s %#v", replayed.Status(), replayed.Issues())
	}
}
