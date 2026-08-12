package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

var reviewedAt = time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func reviewSpec(t *testing.T, verdict domain.ReviewVerdict) domain.StageReviewDecisionSpec {
	t.Helper()
	return domain.StageReviewDecisionSpec{
		Stage:        domain.NotYetInExecution,
		Objective:    "ENTER_HISTORICAL_REPLAY",
		Scope:        mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v1"),
		Candidates:   mustValue(t, domain.NewCandidateVersionSetID, "candidate-set-1"),
		EvidencePack: "evidence-pack/v1",
		Verdict:      verdict,
		DecidedBy:    "pilot-business-owner",
		DecidedAt:    reviewedAt,
		EffectiveAt:  reviewedAt.Add(time.Hour),
	}
}

// Covers: PN-08 交接「每次阶段评审必须固定一个不可扩张的候选版本组」与「首次评审进入
// 历史回放时，当前执行阶段明确记录为『尚未进入执行阶段』」——三样引用一次进入且值
// 类型无事后改写入口（不可扩张是结构性的）；`尚未进入`是显式一格不是缺省。
func TestACandidateSetIsFixedAndTheFirstReviewSaysNotYet(t *testing.T) {
	set, err := domain.FixCandidateVersionSet(
		mustValue(t, domain.NewCandidateVersionSetID, "candidate-set-1"),
		mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v1"),
		mustValue(t, domain.NewParameterSnapshotReference, "par-register/rev-42"),
		mustValue(t, domain.NewRuleVersionsReference, "rule-versions/v7"),
		reviewedAt,
	)
	if err != nil {
		t.Fatalf("fix candidate version set: %v", err)
	}
	if set.Parameters().String() != "par-register/rev-42" {
		t.Fatalf("parameters = %s", set.Parameters())
	}

	review, err := domain.RecordStageReview(reviewSpec(t, domain.StageGo))
	if err != nil {
		t.Fatalf("record stage review: %v", err)
	}
	if review.Stage() != domain.NotYetInExecution {
		t.Fatalf("stage = %q; 首评必须如实记录尚未进入执行阶段", review.Stage())
	}

	missing := reviewSpec(t, domain.StageGo)
	missing.Candidates = domain.CandidateVersionSetID{}
	if _, err := domain.RecordStageReview(missing); !errors.Is(err, domain.ErrInvalidStageReview) {
		t.Fatalf("err = %v; 没有候选版本组的评审被收下了", err)
	}
}

// Covers: PN-08 交接「`No-Go` 的处理方式必须显式写明……不能由系统自动推断」与「非
// 关键偏差只有在六件齐全时才可随 `Go` 保留」——No-Go 必带封闭四值处理方式且不得保留
// 偏差；Go 不带处理方式，偏差缺任一件拒。
func TestVerdictShapesAreMutuallyExclusive(t *testing.T) {
	noGo := reviewSpec(t, domain.StageNoGo)
	if _, err := domain.RecordStageReview(noGo); !errors.Is(err, domain.ErrInvalidStageReview) {
		t.Fatalf("err = %v; 没有处理方式的 No-Go 被收下了", err)
	}
	noGo.Disposition = domain.FixAndReassess
	decision, err := domain.RecordStageReview(noGo)
	if err != nil {
		t.Fatalf("record no-go: %v", err)
	}
	disposition, present := decision.Disposition()
	if !present || disposition != domain.FixAndReassess {
		t.Fatalf("disposition = %q present = %v", disposition, present)
	}

	goWithDisposition := reviewSpec(t, domain.StageGo)
	goWithDisposition.Disposition = domain.KeepCurrentScope
	if _, err := domain.RecordStageReview(goWithDisposition); !errors.Is(err, domain.ErrInvalidStageReview) {
		t.Fatalf("err = %v; Go 带上了 No-Go 的处理方式", err)
	}

	incomplete := reviewSpec(t, domain.StageGo)
	incomplete.Deviations = []domain.AcceptedDeviation{{
		Scope:   "ETA_ALERT_DELAY",
		Control: "manual watch",
		Owner:   "ops-lead",
		// CloseBy 缺席——六件缺一。
		ResidualRisk:  "low",
		AcceptanceRef: "risk-acceptance/7",
	}}
	if _, err := domain.RecordStageReview(incomplete); !errors.Is(err, domain.ErrInvalidStageReview) {
		t.Fatalf("err = %v; 六件不全的偏差随 Go 保留了", err)
	}

	complete := reviewSpec(t, domain.StageGo)
	complete.Deviations = []domain.AcceptedDeviation{{
		Scope:         "ETA_ALERT_DELAY",
		Control:       "manual watch",
		Owner:         "ops-lead",
		CloseBy:       reviewedAt.Add(72 * time.Hour),
		ResidualRisk:  "low",
		AcceptanceRef: "risk-acceptance/7",
	}}
	if _, err := domain.RecordStageReview(complete); err != nil {
		t.Fatalf("record go with complete deviation: %v", err)
	}
}

// Covers: PN-08 失败场景「生产权威区间重叠——立即阻断受影响范围的新准入并保留冲突
// 证据」（错误结果是双写后人工对账）——同对象范围×能力×事实类型的时间重叠逐对列出；
// 开放区间与任何后续区间重叠；不同维度不冲突。
func TestOverlappingAuthorityIntervalsAreDetectedPairwise(t *testing.T) {
	open := domain.AuthorityInterval{
		ObjectScope: "pilot-scope/v1",
		Capability:  "SHIPMENT_ACCEPTANCE",
		FactKind:    "ACCEPTANCE_DECISION",
		Authority:   "idp-parcel",
		From:        reviewedAt,
	}
	late := domain.AuthorityInterval{
		ObjectScope: "pilot-scope/v1",
		Capability:  "SHIPMENT_ACCEPTANCE",
		FactKind:    "ACCEPTANCE_DECISION",
		Authority:   "legacy-system",
		From:        reviewedAt.Add(24 * time.Hour),
	}
	otherFact := domain.AuthorityInterval{
		ObjectScope: "pilot-scope/v1",
		Capability:  "SHIPMENT_ACCEPTANCE",
		FactKind:    "SOURCE_DATA_VERSION",
		Authority:   "idp-parcel",
		From:        reviewedAt,
	}

	conflicts, err := domain.DetectAuthorityConflicts([]domain.AuthorityInterval{open, late, otherFact})
	if err != nil {
		t.Fatalf("detect conflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1（开放区间与后续区间重叠；异维不冲突）", len(conflicts))
	}
	if conflicts[0].First.Authority != "idp-parcel" || conflicts[0].Second.Authority != "legacy-system" {
		t.Fatalf("conflict = %#v", conflicts[0])
	}

	closed := open
	closed.To = reviewedAt.Add(12 * time.Hour)
	conflicts, err = domain.DetectAuthorityConflicts([]domain.AuthorityInterval{closed, late})
	if err != nil {
		t.Fatalf("detect closed: %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %d; 已关闭区间与其后的区间不重叠", len(conflicts))
	}

	if _, err := domain.DetectAuthorityConflicts([]domain.AuthorityInterval{{
		ObjectScope: "pilot-scope/v1",
	}}); !errors.Is(err, domain.ErrInvalidInterval) {
		t.Fatalf("err = %v; 形状坏的区间参与比对只会漏报", err)
	}
}
