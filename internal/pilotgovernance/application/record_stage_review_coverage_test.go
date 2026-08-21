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

// 本文件证覆盖关系随 Go/No-Go 决定一并登记的编排纪律（关系不设独立登记路）：声明
// 构造门与命令内自相矛盾拒绝、册内相悖预检先于任何落库、「同一事实已在册」跳过不
// 重登、边登记失败留续办且重放补登、关系店故障译未决。

type relationStoreDouble struct {
	byPair  map[[2]string]domain.ScopeVersionRelation
	findErr error
	saveErr error
	saves   int
}

func (double *relationStoreDouble) FindByPair(
	_ context.Context,
	successor, predecessor domain.ScopeVersionReference,
) (domain.ScopeVersionRelation, bool, error) {
	if double.findErr != nil {
		return domain.ScopeVersionRelation{}, false, double.findErr
	}
	relation, found := double.byPair[[2]string{successor.String(), predecessor.String()}]
	return relation, found, nil
}

func (double *relationStoreDouble) Save(
	_ context.Context,
	relation domain.ScopeVersionRelation,
) (ports.GovernanceSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.GovernanceSaveOutcomeInvalid, double.saveErr
	}
	key := [2]string{relation.Successor().String(), relation.Predecessor().String()}
	if _, exists := double.byPair[key]; exists {
		return ports.GovernanceAlreadyRecorded, nil
	}
	double.byPair[key] = relation
	double.saves++
	return ports.GovernanceSaved, nil
}

type coverageFixture struct {
	handler   *application.RecordStageReviewHandler
	reviews   *reviewStoreDouble
	relations *relationStoreDouble
}

func newCoverageFixture(t *testing.T) *coverageFixture {
	t.Helper()
	set, err := domain.FixCandidateVersionSet(
		mustValue(t, domain.NewCandidateVersionSetID, "candidate-set-1"),
		mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v1"),
		mustValue(t, domain.NewParameterSnapshotReference, "par-register/rev-42"),
		mustValue(t, domain.NewRuleVersionsReference, "rule-versions/v7"),
		reviewAt.Add(-24*time.Hour),
	)
	if err != nil {
		t.Fatalf("固定候选组：%v", err)
	}
	fixture := &coverageFixture{
		reviews:   &reviewStoreDouble{byKey: map[ports.ReviewKey]domain.StageReviewDecision{}},
		relations: &relationStoreDouble{byPair: map[[2]string]domain.ScopeVersionRelation{}},
	}
	fixture.handler = application.NewRecordStageReviewHandler(application.RecordStageReviewDeps{
		Candidates: &candidateStoreDouble{byID: map[domain.CandidateVersionSetID]domain.CandidateVersionSet{
			set.ID(): set,
		}},
		Reviews:   fixture.reviews,
		Intervals: &intervalStoreDouble{},
		Relations: fixture.relations,
		Clock:     fixedClock{at: reviewAt},
	})
	return fixture
}

func (fixture *coverageFixture) seed(t *testing.T, successor, predecessor string, kind domain.ScopeVersionRelationKind) {
	t.Helper()
	relation, err := domain.RegisterScopeVersionRelation(domain.ScopeVersionRelationSpec{
		Successor:    mustValue(t, domain.NewScopeVersionReference, successor),
		Predecessor:  mustValue(t, domain.NewScopeVersionReference, predecessor),
		Kind:         kind,
		Objective:    "PRIOR_DECISION",
		Candidates:   mustValue(t, domain.NewCandidateVersionSetID, "candidate-set-0"),
		RegisteredAt: reviewAt.Add(-48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("铺垫在册边：%v", err)
	}
	fixture.relations.byPair[[2]string{successor, predecessor}] = relation
}

func coverageCommand(t *testing.T, objective, predecessor string, kind domain.ScopeVersionRelationKind) application.RecordStageReviewCommand {
	t.Helper()
	command := reviewCommand(t, objective)
	command.Coverage = []domain.ScopeCoverageDeclaration{{
		Predecessor: mustValue(t, domain.NewScopeVersionReference, predecessor),
		Kind:        kind,
	}}
	return command
}

// Covers: 关系作为该次决定记录的一格随决定落库——后继取评审范围、所属决定引用取
// （目标+候选组）、登记时点取决定时点；无声明的评审一条边也不登。
func TestACoverageDeclarationLandsWithItsDecision(t *testing.T) {
	fixture := newCoverageFixture(t)

	result, err := fixture.handler.Handle(t.Context(),
		coverageCommand(t, "ENTER_SHADOW_RUN", "pilot-scope/v0", domain.ScopeInheritsSuspensions))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReviewRecorded || result.CoverageContinuation() != "" {
		t.Fatalf("outcome=%s continuation=%q", result.Outcome(), result.CoverageContinuation())
	}
	if fixture.relations.saves != 1 {
		t.Fatalf("落边 %d 条, 想要 1", fixture.relations.saves)
	}
	landed := fixture.relations.byPair[[2]string{"pilot-scope/v1", "pilot-scope/v0"}]
	if landed.Successor().String() != "pilot-scope/v1" ||
		landed.Objective() != "ENTER_SHADOW_RUN" ||
		landed.Candidates().String() != "candidate-set-1" ||
		!landed.RegisteredAt().Equal(reviewAt) {
		t.Fatalf("所属决定引用或时点错位：%+v", landed)
	}
}

// Covers: 相悖声明在决定落库前被拦下带全部相悖对——后到的决定改写不了先到的登记，
// 静默收下等于让关系登记变成绕过恢复决定的旁路。反向的互不相干同样拦承继声明。
func TestACoverageContradictionBlocksBeforeAnythingLands(t *testing.T) {
	samePair := newCoverageFixture(t)
	samePair.seed(t, "pilot-scope/v1", "pilot-scope/v0", domain.ScopeUnrelated)
	blocked, err := samePair.handler.Handle(t.Context(),
		coverageCommand(t, "ENTER_SHADOW_RUN", "pilot-scope/v0", domain.ScopeInheritsSuspensions))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if blocked.Outcome() != application.CoverageConflictBlocked {
		t.Fatalf("outcome = %s, want COVERAGE_CONFLICT", blocked.Outcome())
	}
	if len(blocked.CoverageConflicts()) != 1 {
		t.Fatalf("相悖对 = %d, 想要 1", len(blocked.CoverageConflicts()))
	}
	if samePair.reviews.saved != 0 || samePair.relations.saves != 0 {
		t.Fatal("相悖阻断后仍有东西落了库")
	}

	reverse := newCoverageFixture(t)
	reverse.seed(t, "pilot-scope/v0", "pilot-scope/v1", domain.ScopeUnrelated)
	blocked, err = reverse.handler.Handle(t.Context(),
		coverageCommand(t, "ENTER_SHADOW_RUN", "pilot-scope/v0", domain.ScopeInheritsSuspensions))
	if err != nil {
		t.Fatalf("reverse handle: %v", err)
	}
	if blocked.Outcome() != application.CoverageConflictBlocked {
		t.Fatalf("反向相悖没拦住：outcome = %s", blocked.Outcome())
	}
}

// Covers: 「同一事实已在册」不是错误也不重登——同对同种跳过，对称的互不相干从另一
// 方向登过也跳过；决定照常落。
func TestARedundantCoverageDeclarationIsSkippedNotResaved(t *testing.T) {
	fixture := newCoverageFixture(t)
	fixture.seed(t, "pilot-scope/v1", "pilot-scope/v0", domain.ScopeInheritsSuspensions)

	result, err := fixture.handler.Handle(t.Context(),
		coverageCommand(t, "ENTER_SHADOW_RUN", "pilot-scope/v0", domain.ScopeInheritsSuspensions))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReviewRecorded || result.CoverageContinuation() != "" {
		t.Fatalf("outcome=%s continuation=%q", result.Outcome(), result.CoverageContinuation())
	}
	if fixture.relations.saves != 0 {
		t.Fatalf("已在册的边被重登了 %d 次", fixture.relations.saves)
	}

	symmetric := newCoverageFixture(t)
	symmetric.seed(t, "pilot-scope/v0", "pilot-scope/v1", domain.ScopeUnrelated)
	result, err = symmetric.handler.Handle(t.Context(),
		coverageCommand(t, "ENTER_SHADOW_RUN", "pilot-scope/v0", domain.ScopeUnrelated))
	if err != nil {
		t.Fatalf("symmetric handle: %v", err)
	}
	if result.Outcome() != application.ReviewRecorded || symmetric.relations.saves != 0 {
		t.Fatalf("对称事实被重登：outcome=%s saves=%d", result.Outcome(), symmetric.relations.saves)
	}
}

// Covers: 声明装不成边未受理——自指边（评审范围对自己声明）与命令内同前代自相矛盾
// 都在构造门被拒，决定不落库。
func TestAMalformedCoverageDeclarationIsNotAccepted(t *testing.T) {
	selfEdge := newCoverageFixture(t)
	refused, err := selfEdge.handler.Handle(t.Context(),
		coverageCommand(t, "ENTER_SHADOW_RUN", "pilot-scope/v1", domain.ScopeInheritsSuspensions))
	if err != nil {
		t.Fatalf("self-edge handle: %v", err)
	}
	if refused.Outcome() != application.ReviewNotAccepted || selfEdge.reviews.saved != 0 {
		t.Fatalf("自指边被收下了：outcome=%s saved=%d", refused.Outcome(), selfEdge.reviews.saved)
	}

	contradictory := newCoverageFixture(t)
	command := reviewCommand(t, "ENTER_SHADOW_RUN")
	command.Coverage = []domain.ScopeCoverageDeclaration{
		{Predecessor: mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v0"), Kind: domain.ScopeInheritsSuspensions},
		{Predecessor: mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v0"), Kind: domain.ScopeUnrelated},
	}
	refused, err = contradictory.handler.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("contradictory handle: %v", err)
	}
	if refused.Outcome() != application.ReviewNotAccepted || contradictory.reviews.saved != 0 {
		t.Fatalf("一次决定对同一对版本说了两句话还被收下：outcome=%s saved=%d",
			refused.Outcome(), contradictory.reviews.saved)
	}
}

// Covers: 边登记失败不翻已落库的决定但留续办引用；分岔期间第三态保守作答由读侧担保，
// 这里证重放路凭同一命令把边补上、补上后续办引用清空。
func TestAFailedCoverageSaveLeavesAContinuationAndReplayHeals(t *testing.T) {
	fixture := newCoverageFixture(t)
	fixture.relations.saveErr = errors.New("relation store unreachable")
	command := coverageCommand(t, "ENTER_SHADOW_RUN", "pilot-scope/v0", domain.ScopeInheritsSuspensions)

	first, err := fixture.handler.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.ReviewRecorded {
		t.Fatalf("outcome = %s；边登记失败不得翻决定", first.Outcome())
	}
	if first.CoverageContinuation() == "" {
		t.Fatal("边登记失败没有留续办引用——决定与关系分岔无处可知")
	}
	if fixture.reviews.saved != 1 {
		t.Fatalf("saved = %d", fixture.reviews.saved)
	}

	fixture.relations.saveErr = nil
	replay, err := fixture.handler.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.ReviewExistingDecision || replay.CoverageContinuation() != "" {
		t.Fatalf("重放没有补上边：outcome=%s continuation=%q", replay.Outcome(), replay.CoverageContinuation())
	}
	if fixture.relations.saves != 1 {
		t.Fatalf("补登后边数 = %d, 想要 1", fixture.relations.saves)
	}
}

// Covers: 关系店故障译成未决——预检读不到册面时不能假装没有相悖边继续落决定。
func TestACoverageStoreOutageAnswersUndecided(t *testing.T) {
	fixture := newCoverageFixture(t)
	fixture.relations.findErr = errors.New("relation store is down")

	result, err := fixture.handler.Handle(t.Context(),
		coverageCommand(t, "ENTER_SHADOW_RUN", "pilot-scope/v0", domain.ScopeInheritsSuspensions))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReviewUndecided || fixture.reviews.saved != 0 {
		t.Fatalf("outcome=%s saved=%d, 想要 UNDECIDED 且决定不落", result.Outcome(), fixture.reviews.saved)
	}
}
