package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉仍适用线上改善改路形成的复核记录带什么依据（票 docs-consistency/code/53）：`已改路`的依据是失效依据或改善依据，
// 恰有其一，复核册的逐结论 CHECK 同此。阈值是本测试交入的租户取值，不是产品默认。

// improvementFixture 预置原计划仍可执行、冻结形态已声明且未越过边界、另有一条更便宜候选的前提：原计划所选 candidate-1
// 1000 分，candidate-2 100 分，改善 900 分。
func improvementFixture(t *testing.T, threshold int) *reassessFixture {
	t.Helper()
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	evidence := declareCostImprovement(cheaperAlternative(t, 100), threshold)
	limit := 0
	evidence.FreezeForm = domain.RemainingSegmentCountFreeze
	evidence.FreezeRemainingSegmentLimit = &limit
	fixture.evidence.byParcel["parcel-1"] = evidence
	return fixture
}

// Covers: ADR-0173 决定一——改善严格大于阈值时自动切换，复核记录不带失效依据，带改善依据：被替代计划成本、新计划成本、
// 比较所用币种与阈值；候选评估记有候选（落得过复核册「`已改路`必有候选评估状态」那一条）。
func TestAnImprovementRerouteRecordsItsImprovementBasis(t *testing.T) {
	fixture := improvementFixture(t, 500)

	result := handleOK(t, fixture, reassessCommand(t, "node-origin"))
	if result.Outcome() != application.ReassessedRerouted {
		t.Fatalf("outcome = %q, want REROUTED", result.Outcome())
	}
	record, _ := result.Record()
	if record.LapseBasis.String() != "" {
		t.Fatalf("仍可执行的原计划被记了失效依据 %q", record.LapseBasis)
	}
	if record.CandidateState != ports.CandidatesAvailable {
		t.Fatalf("候选评估 = %q, want CANDIDATES_AVAILABLE", record.CandidateState)
	}
	if !record.HasImprovementBasis {
		t.Fatal("改善改路没带改善依据")
	}
	basis := record.ImprovementBasis
	if basis.SupersededCostMinor() != 1000 || basis.SuccessorCostMinor() != 100 ||
		basis.Currency() != "CNY" || basis.ThresholdMinor() != 500 {
		t.Fatalf("改善依据 = %+v, want 1000 → 100、CNY、阈值 500", basis)
	}
	if !record.HasNewPlan || record.NewPlan.SelectedCandidate().String() != "candidate-2" || !record.HasDecision {
		t.Fatalf("新计划选中 %s、决定 %v, want candidate-2 与自动决定", record.NewPlan.SelectedCandidate(), record.HasDecision)
	}
}

// Covers: 改善恰等于阈值不切换：仍适用、原计划保留，记录不带改善依据，也不碰适用性。
func TestAnImprovementAtTheThresholdKeepsThePlanWithoutABasis(t *testing.T) {
	fixture := improvementFixture(t, 900)

	result := handleOK(t, fixture, reassessCommand(t, "node-origin"))
	if result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE", result.Outcome())
	}
	record, _ := result.Record()
	if record.HasImprovementBasis || record.HasNewPlan || record.CandidateState != ports.CandidateReviewStateInvalid {
		t.Fatalf("record = %+v; 没切换不得带改善依据、新计划或候选评估状态", record)
	}
	if len(fixture.applicability.saved) != 0 {
		t.Fatalf("没切换却写了适用性 %d 次", len(fixture.applicability.saved))
	}
}

// Covers: 失效线自动改路的依据是失效依据，不带改善依据——两者恰有其一。
func TestALapseRerouteRecordsNoImprovementBasis(t *testing.T) {
	fixture := rerouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = declareCostImprovement(cheaperAlternative(t, 100), 0)

	result := handleOK(t, fixture, reassessCommand(t, "node-actual-elsewhere"))
	if result.Outcome() != application.ReassessedRerouted {
		t.Fatalf("outcome = %q, want REROUTED", result.Outcome())
	}
	record, _ := result.Record()
	if record.LapseBasis.String() == "" || record.HasImprovementBasis {
		t.Fatalf("失效线改路 = 失效依据 %q、改善依据 %v, want 只有失效依据", record.LapseBasis, record.HasImprovementBasis)
	}
}
