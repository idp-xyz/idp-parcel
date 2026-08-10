package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func candidate(t *testing.T, id string, outcome domain.CandidateOutcome, reason string) domain.RouteCandidate {
	t.Helper()
	built, err := domain.NewRouteCandidate(
		mustValue(t, domain.NewCandidateID, id),
		outcome,
		mustValue(t, domain.NewCandidateReason, reason),
	)
	if err != nil {
		t.Fatalf("new route candidate %q: %v", id, err)
	}
	return built
}

func gap(t *testing.T, reference string, scope domain.EvidenceGapScope, affected ...string) domain.EvidenceGap {
	t.Helper()
	ids := make([]domain.CandidateID, len(affected))
	for index, value := range affected {
		ids[index] = mustValue(t, domain.NewCandidateID, value)
	}
	built, err := domain.NewEvidenceGap(mustValue(t, domain.NewEvidenceGapReference, reference), scope, ids)
	if err != nil {
		t.Fatalf("new evidence gap %q: %v", reference, err)
	}
	return built
}

// Covers: AT-NR-016 与证据判定矩阵第一行 — 至少一个候选完整验证合格即为可达。
func TestOneQualifiedCandidateIsReachable(t *testing.T) {
	finding, err := domain.ConcludeReachability([]domain.RouteCandidate{
		candidate(t, "cand-1", domain.CandidateQualified, "MEETS_ALL_CONSTRAINTS"),
		candidate(t, "cand-2", domain.CandidateEliminated, "LANE_NOT_SERVED"),
	}, nil)
	if err != nil {
		t.Fatalf("conclude: %v", err)
	}
	if finding.Value() != domain.Reachable {
		t.Fatalf("finding = %q, want REACHABLE", finding.Value())
	}
	qualified := finding.QualifiedCandidates()
	if len(qualified) != 1 || qualified[0].ID().String() != "cand-1" {
		t.Fatalf("reachable finding does not name its qualified candidate: %v", qualified)
	}
}

// Covers: AT-NR-017、AT-NR-023 与矩阵第二行 — 候选空间闭合且全部确定性淘汰才是不可达，
// 且必须逐候选带淘汰依据。
func TestAllCandidatesEliminatedIsUnreachable(t *testing.T) {
	candidates := []domain.RouteCandidate{
		candidate(t, "cand-1", domain.CandidateEliminated, "SERVICE_AREA_EXCLUDES_DESTINATION"),
		candidate(t, "cand-2", domain.CandidateEliminated, "PROHIBITED_COMMODITY"),
	}

	finding, err := domain.ConcludeReachability(candidates, nil)
	if err != nil {
		t.Fatalf("conclude: %v", err)
	}
	if finding.Value() != domain.Unreachable {
		t.Fatalf("finding = %q, want UNREACHABLE", finding.Value())
	}
	if len(finding.EliminatedCandidates()) != 2 {
		t.Fatal("an unreachable finding did not retain every elimination")
	}
	for _, eliminated := range finding.EliminatedCandidates() {
		if eliminated.Reason().String() == "" {
			t.Fatalf("candidate %q was eliminated without a stable reason", eliminated.ID())
		}
	}
}

// Covers: AT-NR-024 与矩阵末段 — 部分淘汰、剩余因证据不足无法判断时整体是资料不足，
// 绝不能仅凭已淘汰的那些形成不可达。
func TestPartiallyEliminatedWithUnknownRemainderIsInsufficient(t *testing.T) {
	finding, err := domain.ConcludeReachability([]domain.RouteCandidate{
		candidate(t, "cand-1", domain.CandidateEliminated, "LANE_NOT_SERVED"),
		candidate(t, "cand-2", domain.CandidateEvidenceUnknown, "CUSTOMS_ELIGIBILITY_UNKNOWN"),
	}, []domain.EvidenceGap{
		gap(t, "CUSTOMS_ELIGIBILITY", domain.CandidateScopedGap, "cand-2"),
	})
	if err != nil {
		t.Fatalf("conclude: %v", err)
	}
	if finding.Value() != domain.InsufficientEvidence {
		t.Fatalf("finding = %q, want INSUFFICIENT_EVIDENCE", finding.Value())
	}
	if len(finding.EvidenceGaps()) != 1 {
		t.Fatal("an insufficient finding did not report what is missing")
	}
}

// Covers: AT-NR-025 — 已有一个完整合格候选时，其他候选的局部证据缺口不推翻它，仍为可达。
func TestCandidateScopedGapsElsewhereDoNotBlockReachable(t *testing.T) {
	finding, err := domain.ConcludeReachability([]domain.RouteCandidate{
		candidate(t, "cand-1", domain.CandidateQualified, "MEETS_ALL_CONSTRAINTS"),
		candidate(t, "cand-2", domain.CandidateEvidenceUnknown, "CUSTOMS_ELIGIBILITY_UNKNOWN"),
	}, []domain.EvidenceGap{
		gap(t, "CUSTOMS_ELIGIBILITY", domain.CandidateScopedGap, "cand-2"),
	})
	if err != nil {
		t.Fatalf("conclude: %v", err)
	}
	if finding.Value() != domain.Reachable {
		t.Fatalf("finding = %q, want REACHABLE; a gap on another candidate blocked a proven one", finding.Value())
	}
	if len(finding.EvidenceGaps()) != 1 {
		t.Fatal("a reachable finding discarded the unknown candidate's gap instead of retaining it")
	}
}

// Covers: 证据判定矩阵第一行的但书 — 仍缺少的全局证据若能推翻已合格候选，结论退回
// 资料不足，而不是可达。
func TestGlobalGapOverturnsAnOtherwiseQualifiedCandidate(t *testing.T) {
	finding, err := domain.ConcludeReachability([]domain.RouteCandidate{
		candidate(t, "cand-1", domain.CandidateQualified, "MEETS_ALL_CONSTRAINTS"),
	}, []domain.EvidenceGap{
		gap(t, "NETWORK_AVAILABILITY_ADJUSTMENT_UNKNOWN", domain.GlobalGap),
	})
	if err != nil {
		t.Fatalf("conclude: %v", err)
	}
	if finding.Value() != domain.InsufficientEvidence {
		t.Fatalf("finding = %q, want INSUFFICIENT_EVIDENCE", finding.Value())
	}
}

// Covers: 矩阵第二行「必须证明不存在尚未评估、证据未知或版本失配的可能候选」—
// 空候选集合证明不了闭合，既不是不可达也不是资料不足，域内无法结论。
func TestEmptyCandidateSpaceCannotConclude(t *testing.T) {
	if _, err := domain.ConcludeReachability(nil, nil); !errors.Is(err, domain.ErrCandidateSpaceNotEstablished) {
		t.Fatalf("error = %v, want ErrCandidateSpaceNotEstablished", err)
	}
}

// Covers: UC-NR-002「判断不得包含或暗示」以及三组结果必须分离 — 领域只结论三值，
// 不存在第四个可以表达接受、拒绝或技术故障的取值。
func TestFindingValueSetIsExactlyThreeValued(t *testing.T) {
	labels := map[string]struct{}{}
	for _, value := range []domain.ReachabilityValue{domain.Reachable, domain.Unreachable, domain.InsufficientEvidence} {
		label := value.String()
		if label == "" {
			t.Fatalf("value %d has no label", value)
		}
		labels[label] = struct{}{}
	}
	if len(labels) != 3 {
		t.Fatalf("three-valued finding collapsed into %d labels", len(labels))
	}
	if domain.ReachabilityValue(len(labels)+1).String() != "" {
		t.Fatal("a fourth reachability value carries a label, which invites a non-domain result to hide in the set")
	}
}

// Covers: UC-NR-002 判断内容 — 淘汰与证据未知都必须带稳定原因，空原因不得构造。
func TestEveryNonQualifiedCandidateNeedsAStableReason(t *testing.T) {
	for _, outcome := range []domain.CandidateOutcome{domain.CandidateEliminated, domain.CandidateEvidenceUnknown} {
		if _, err := domain.NewRouteCandidate(
			mustValue(t, domain.NewCandidateID, "cand-x"),
			outcome,
			domain.CandidateReason{},
		); !errors.Is(err, domain.ErrInvalidRouteCandidate) {
			t.Fatalf("outcome %q constructed without a reason: err = %v", outcome, err)
		}
	}
}
