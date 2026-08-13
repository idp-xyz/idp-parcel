package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

var reviewAt = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type candidateStoreDouble struct {
	byID map[domain.CandidateVersionSetID]domain.CandidateVersionSet
}

func (double *candidateStoreDouble) FindByID(
	_ context.Context,
	id domain.CandidateVersionSetID,
) (domain.CandidateVersionSet, bool, error) {
	set, found := double.byID[id]
	return set, found, nil
}

func (double *candidateStoreDouble) Save(_ context.Context, set domain.CandidateVersionSet) error {
	double.byID[set.ID()] = set
	return nil
}

type reviewStoreDouble struct {
	byKey map[ports.ReviewKey]domain.StageReviewDecision
	saved int
}

func (double *reviewStoreDouble) FindByKey(
	_ context.Context,
	key ports.ReviewKey,
) (domain.StageReviewDecision, bool, error) {
	decision, found := double.byKey[key]
	return decision, found, nil
}

func (double *reviewStoreDouble) Save(
	_ context.Context,
	key ports.ReviewKey,
	decision domain.StageReviewDecision,
) (ports.ReviewSaveOutcome, error) {
	if _, exists := double.byKey[key]; exists {
		return ports.ReviewAlreadyRecorded, nil
	}
	double.byKey[key] = decision
	double.saved++
	return ports.ReviewSaved, nil
}

type intervalStoreDouble struct {
	intervals []domain.AuthorityInterval
	appendErr error
}

func (double *intervalStoreDouble) ListCurrent(_ context.Context) ([]domain.AuthorityInterval, error) {
	return append([]domain.AuthorityInterval(nil), double.intervals...), nil
}

func (double *intervalStoreDouble) Append(_ context.Context, interval domain.AuthorityInterval) error {
	if double.appendErr != nil {
		return double.appendErr
	}
	double.intervals = append(double.intervals, interval)
	return nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type reviewFixture struct {
	handler    *application.RecordStageReviewHandler
	candidates *candidateStoreDouble
	reviews    *reviewStoreDouble
	intervals  *intervalStoreDouble
}

func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()
	set, err := domain.FixCandidateVersionSet(
		mustValue(t, domain.NewCandidateVersionSetID, "candidate-set-1"),
		mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v1"),
		mustValue(t, domain.NewParameterSnapshotReference, "par-register/rev-42"),
		mustValue(t, domain.NewRuleVersionsReference, "rule-versions/v7"),
		reviewAt.Add(-24*time.Hour),
	)
	if err != nil {
		t.Fatalf("fix candidate set: %v", err)
	}
	fixture := &reviewFixture{
		candidates: &candidateStoreDouble{byID: map[domain.CandidateVersionSetID]domain.CandidateVersionSet{
			set.ID(): set,
		}},
		reviews:   &reviewStoreDouble{byKey: map[ports.ReviewKey]domain.StageReviewDecision{}},
		intervals: &intervalStoreDouble{},
	}
	fixture.handler = application.NewRecordStageReviewHandler(application.RecordStageReviewDeps{
		Candidates: fixture.candidates,
		Reviews:    fixture.reviews,
		Intervals:  fixture.intervals,
		Clock:      fixedClock{at: reviewAt},
	})
	return fixture
}

func reviewCommand(t *testing.T, objective string) application.RecordStageReviewCommand {
	t.Helper()
	return application.RecordStageReviewCommand{Review: domain.StageReviewDecisionSpec{
		Stage:        domain.NotYetInExecution,
		Objective:    objective,
		Scope:        mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v1"),
		Candidates:   mustValue(t, domain.NewCandidateVersionSetID, "candidate-set-1"),
		EvidencePack: "evidence-pack/v1",
		Verdict:      domain.StageGo,
		DecidedBy:    "pilot-business-owner",
		DecidedAt:    reviewAt,
		EffectiveAt:  reviewAt.Add(time.Hour),
	}}
}

// Covers: PN-08 交接「每次阶段评审必须固定一个不可扩张的候选版本组」的编排面——
// 引用不存在的候选组未受理（范围身份悬空，改请求不是重试）；同目标同候选组只决定
// 一次（重复按已有决定作答，不重复形成）。点名验收矩阵 GOV-03「阶段 Go/No-Go」的
// 机制半边（决定固定评审目标、候选组与依据；`P` 级证据属实例半边不在此宣称）。
func TestAReviewAnchorsItsCandidateSetAndDecidesOnce(t *testing.T) {
	fixture := newReviewFixture(t)

	first, err := fixture.handler.Handle(context.Background(), reviewCommand(t, "ENTER_HISTORICAL_REPLAY"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.ReviewRecorded {
		t.Fatalf("outcome = %q", first.Outcome())
	}

	replay, err := fixture.handler.Handle(context.Background(), reviewCommand(t, "ENTER_HISTORICAL_REPLAY"))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.ReviewExistingDecision {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if fixture.reviews.saved != 1 {
		t.Fatalf("saved = %d; 同目标同候选组决定了两次", fixture.reviews.saved)
	}

	dangling := reviewCommand(t, "ENTER_SHADOW_RUN")
	dangling.Review.Candidates = mustValue(t, domain.NewCandidateVersionSetID, "candidate-set-9")
	refused, err := fixture.handler.Handle(context.Background(), dangling)
	if err != nil {
		t.Fatalf("dangling handle: %v", err)
	}
	if refused.Outcome() != application.ReviewNotAccepted {
		t.Fatalf("outcome = %q; 引用不存在的候选组被收下了", refused.Outcome())
	}
}

// Covers: PN-08 失败场景「生产权威区间重叠——立即阻断受影响范围的新准入并保留冲突
// 证据」的编排面，即验收矩阵 GOV-04「唯一生产权威方：同一范围和生效区间只有一个权威
// 写入方」的机制半边——Go 进限量生产携带的新区间先过冲突预检：撞上既有区间即阻断带
// 全部冲突对且决定不落库（先于任何落库，双写后人工对账是被点名的错误结果）；无冲突时
// 决定与区间都落。
func TestAGrantedIntervalIsBlockedByOverlapBeforeAnythingLands(t *testing.T) {
	fixture := newReviewFixture(t)
	fixture.intervals.intervals = []domain.AuthorityInterval{{
		ObjectScope: "pilot-scope/v1",
		Capability:  "SHIPMENT_ACCEPTANCE",
		FactKind:    "ACCEPTANCE_DECISION",
		Authority:   "legacy-system",
		From:        reviewAt.Add(-48 * time.Hour),
	}}

	command := reviewCommand(t, "ENTER_LIMITED_PRODUCTION")
	command.Review.Stage = domain.ShadowRun
	command.GrantedInterval = &domain.AuthorityInterval{
		ObjectScope: "pilot-scope/v1",
		Capability:  "SHIPMENT_ACCEPTANCE",
		FactKind:    "ACCEPTANCE_DECISION",
		Authority:   "idp-parcel",
		From:        reviewAt,
	}

	blocked, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("blocked handle: %v", err)
	}
	if blocked.Outcome() != application.AuthorityConflictBlocked {
		t.Fatalf("outcome = %q, want AUTHORITY_CONFLICT", blocked.Outcome())
	}
	if len(blocked.Conflicts()) != 1 {
		t.Fatalf("conflicts = %d; 冲突对必须列全", len(blocked.Conflicts()))
	}
	if fixture.reviews.saved != 0 {
		t.Fatal("冲突阻断后决定还落了库")
	}

	fixture.intervals.intervals[0].To = reviewAt.Add(-time.Hour)
	granted, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("granted handle: %v", err)
	}
	if granted.Outcome() != application.ReviewRecorded {
		t.Fatalf("outcome = %q; 原区间关闭后新区间不再重叠", granted.Outcome())
	}
	if len(fixture.intervals.intervals) != 2 {
		t.Fatalf("intervals = %d; 新权威区间没有追加", len(fixture.intervals.intervals))
	}
}

// Covers: 评审发现②的修法——区间追加失败不翻已落库的决定，但必须交回续办引用（决定
// 在册而区间缺失且无处可知，正是权威不明的双帐分岔）；重放路凭同一命令补追加同一份
// 区间，补上后续办引用清空；已在册的区间不重复追加。
func TestAFailedIntervalAppendLeavesAContinuationAndReplayHeals(t *testing.T) {
	fixture := newReviewFixture(t)
	fixture.intervals.appendErr = errors.New("interval store unreachable")

	command := reviewCommand(t, "ENTER_LIMITED_PRODUCTION")
	command.Review.Stage = domain.ShadowRun
	command.GrantedInterval = &domain.AuthorityInterval{
		ObjectScope: "pilot-scope/v1",
		Capability:  "SHIPMENT_ACCEPTANCE",
		FactKind:    "ACCEPTANCE_DECISION",
		Authority:   "idp-parcel",
		From:        reviewAt,
	}

	first, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.ReviewRecorded {
		t.Fatalf("outcome = %q; 追加失败不得翻决定", first.Outcome())
	}
	if first.IntervalContinuation() == "" {
		t.Fatal("追加失败没有留续办引用——决定与区间分岔无处可知")
	}
	if fixture.reviews.saved != 1 {
		t.Fatalf("saved = %d", fixture.reviews.saved)
	}

	fixture.intervals.appendErr = nil
	replay, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.ReviewExistingDecision {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if replay.IntervalContinuation() != "" {
		t.Fatal("补追加成功后续办引用还挂着")
	}
	if len(fixture.intervals.intervals) != 1 {
		t.Fatalf("intervals = %d; 重放没有补上区间", len(fixture.intervals.intervals))
	}

	again, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("second replay handle: %v", err)
	}
	if again.IntervalContinuation() != "" || len(fixture.intervals.intervals) != 1 {
		t.Fatalf("intervals = %d; 已在册的区间被重复追加", len(fixture.intervals.intervals))
	}
}
