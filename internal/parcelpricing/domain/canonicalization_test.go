package domain

import "testing"

// Covers: CONTEXT「摘要携带产生它的规范化版本，只在同一规范化版本内可比；不携带规范化
// 版本的摘要视为不完整」（该版本化见 ADR-0014）— 不记下自己用的是哪一套形状的评价，其摘要
// 就是不完整的：日后没有任何东西分得出一处差异是内容变了，还是形状变了。
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

// Covers: CONTEXT「跨版本的摘要差异本身不构成内容已变的证据」— 早先构建保存的评价带着那次
// 构建的形状，它的摘要与今天的不可比。在这里报版本内容冲突，等于形状一放宽就把全部历史重放
// 一并判死，而那正是 ADR-0014 要挡住的结果。
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
	// 本次构建只能按当前形状规范化，所以它声明自己做不了这次重放，而不是宣称做了一次
	// 忠实的重放。
	if replayed.Status() != EvaluationFailed || len(replayed.Issues()) != 1 || replayed.Issues()[0].Code() != "CANONICALIZATION_VERSION_UNSUPPORTED" {
		t.Fatalf("cross-version replay = %s %#v", replayed.Status(), replayed.Issues())
	}
}

// Covers: CONTEXT「摘要不一致只有在规范化版本相同时才是版本内容冲突」— 在同一个规范化版本
// 之内，摘要仍干它原来的活：版本引用没变而内容变了，依旧是冲突。规范化版本要拓宽的是「摘要
// 差异可能意味着什么」，不是削弱它。
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
