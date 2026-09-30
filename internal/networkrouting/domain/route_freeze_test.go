package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func TestExecutedPrefixSplitsOnTheControllableNode(t *testing.T) {
	nodes := []string{"origin", "hub", "dest"}
	prefix := domain.JudgeExecutedPrefix(nodes, domain.NodeIntakeControl, "hub")
	if prefix.Grade() != domain.ExecutedPrefixEstablished {
		t.Fatalf("grade = %d", prefix.Grade())
	}
	if got := prefix.Prefix(); len(got) != 2 || got[0] != "origin" || got[1] != "hub" {
		t.Fatalf("prefix = %v", got)
	}
	if got := prefix.Remaining(); len(got) != 2 || got[0] != "hub" || got[1] != "dest" || prefix.RemainingSegments() != 1 {
		t.Fatalf("remaining = %v segments %d", got, prefix.RemainingSegments())
	}
}

func TestExecutedPrefixDoesNotInventANodeFromAScan(t *testing.T) {
	nodes := []string{"origin", "dest"}
	prefix := domain.JudgeExecutedPrefix(nodes, domain.SourceHint, "origin")
	if prefix.Grade() != domain.ExecutedPrefixNoControllableNode || len(prefix.Prefix()) != 0 {
		t.Fatalf("scan established a prefix: %+v", prefix.Prefix())
	}
}

func TestExecutedPrefixWhenTheNodeIsNotOnThePlan(t *testing.T) {
	prefix := domain.JudgeExecutedPrefix([]string{"origin", "dest"}, domain.TransportHandoverControl, "elsewhere")
	if prefix.Grade() != domain.ExecutedPrefixNodeNotOnPlan {
		t.Fatalf("grade = %d", prefix.Grade())
	}
}

func TestFreezeStaysUnconfiguredWhenTheFormIsUndeclared(t *testing.T) {
	limit := 0
	judgment, err := domain.JudgeFreezeBoundary(domain.FreezeFormUndeclared, &limit, 0)
	if err != nil || !judgment.Unconfigured() || judgment.Crossed() {
		t.Fatalf("judgment crossed=%v unconfigured=%v err=%v", judgment.Crossed(), judgment.Unconfigured(), err)
	}
}

func TestFreezeCrossesWhenRemainingSegmentsAreWithinTheRegisteredLimit(t *testing.T) {
	limit := 1
	crossed, err := domain.JudgeFreezeBoundary(domain.RemainingSegmentCountFreeze, &limit, 1)
	if err != nil || !crossed.Crossed() {
		t.Fatalf("crossed = %v err=%v", crossed.Crossed(), err)
	}
	open, err := domain.JudgeFreezeBoundary(domain.RemainingSegmentCountFreeze, &limit, 2)
	if err != nil || open.Crossed() || open.Unconfigured() {
		t.Fatalf("open crossed=%v unconfigured=%v", open.Crossed(), open.Unconfigured())
	}
}
