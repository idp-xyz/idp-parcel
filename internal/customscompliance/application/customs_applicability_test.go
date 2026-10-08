package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 本文件钉判断服务那一层（票 routing-first-cut/12）：目录读不到折成状态未知而不是错误
// 上抛、逐候选各答各的、查询坏输入上抛。

type snapshotDouble struct {
	snapshot ports.PortsPathsSnapshot
	err      error
}

var _ ports.PortsPathsSnapshotView = snapshotDouble{}

func (double snapshotDouble) LoadPortsPathsSnapshot(
	context.Context, domain.TenantID,
) (ports.PortsPathsSnapshot, error) {
	return double.snapshot, double.err
}

func judge(t *testing.T, double snapshotDouble, candidates ...ports.CustomsApplicabilityCandidate) []domain.CustomsApplicabilityJudgment {
	t.Helper()
	handler := NewCustomsApplicabilityHandler(CustomsApplicabilityDeps{Catalog: double})
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatalf("NewTenantID: %v", err)
	}
	judgments, err := handler.Handle(context.Background(), ports.CustomsApplicabilityQuery{
		Tenant:     tenant,
		AsOf:       time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Candidates: candidates,
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	return judgments
}

func answeredCandidate(t *testing.T, ref, origin, destination string) ports.CustomsApplicabilityCandidate {
	t.Helper()
	candidate, err := domain.NewRouteCandidateReference(ref)
	if err != nil {
		t.Fatalf("NewRouteCandidateReference: %v", err)
	}
	return ports.CustomsApplicabilityCandidate{
		Candidate: candidate, Origin: origin, HasOrigin: true,
		Destination: destination, HasDestination: true,
	}
}

func TestUnreadableCatalogAnswersUnknownForEveryCandidate(t *testing.T) {
	judgments := judge(t, snapshotDouble{err: errors.New("db down")},
		answeredCandidate(t, "cand-a", "CN", "SG"),
		answeredCandidate(t, "cand-b", "CN", "SG"))
	if len(judgments) != 2 {
		t.Fatalf("len = %d，逐候选作答一条都不能少", len(judgments))
	}
	for _, judgment := range judgments {
		if judgment.Outcome() != domain.CustomsStatusUnknown ||
			judgment.UnknownReason() != domain.CatalogUnreadable {
			t.Fatalf("outcome/unknown = %q/%q，依赖读不到该折成状态未知、不上抛",
				judgment.Outcome().String(), judgment.UnknownReason().String())
		}
	}
}

func TestEachCandidateFoldsItsOwnAnswer(t *testing.T) {
	judgments := judge(t, snapshotDouble{},
		answeredCandidate(t, "cand-domestic", "CN", "CN"),
		answeredCandidate(t, "cand-crossing", "CN", "SG"))
	if len(judgments) != 2 {
		t.Fatalf("len = %d", len(judgments))
	}
	if judgments[0].Outcome() != domain.CustomsAvailable {
		t.Fatalf("同国候选 = %q，该可用", judgments[0].Outcome().String())
	}
	if judgments[1].Outcome() != domain.CustomsStatusUnknown ||
		judgments[1].UnknownReason() != domain.CatalogEmpty {
		t.Fatalf("跨境候选 = %q/%q，目录为空该答状态未知", judgments[1].Outcome().String(), judgments[1].UnknownReason().String())
	}
}

func TestHandlerRefusesBlankCandidateReference(t *testing.T) {
	handler := NewCustomsApplicabilityHandler(CustomsApplicabilityDeps{Catalog: snapshotDouble{}})
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	_, err := handler.Handle(context.Background(), ports.CustomsApplicabilityQuery{
		Tenant: tenant,
		AsOf:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Candidates: []ports.CustomsApplicabilityCandidate{
			{Candidate: domain.RouteCandidateReference{}, HasOrigin: true, HasDestination: true},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "candidate reference is blank") {
		t.Fatalf("err = %v，空白候选引用是编程错误，该响亮上抛", err)
	}
}

func TestHandlerRefusesDegenerateQuery(t *testing.T) {
	handler := NewCustomsApplicabilityHandler(CustomsApplicabilityDeps{Catalog: snapshotDouble{}})
	if _, err := handler.Handle(context.Background(), ports.CustomsApplicabilityQuery{}); err == nil {
		t.Fatal("缺租户与判断时点的查询必须被拒")
	}
}
