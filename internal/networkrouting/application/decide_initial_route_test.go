package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉最低成本并列的授权裁决（ADR-0309；票 docs-consistency/code/97）：并列入账的登记行由登记替身给出，授权角色在并列的那几家
// 之间择一，编排按此刻的证据重判、形成首个计划。判断键锁由复核册替身承担，与授权改路决定同一把。

// tieDecidedAt 是授权角色提交裁决的那一刻，晚于委托接受与并列入账。
var tieDecidedAt = routeJudgedAt.Add(5 * time.Hour)

// notFormedReadDouble 是未形成登记的读替身：按判断键存最新一次。
type notFormedReadDouble struct {
	records map[domain.InitialRouteJudgmentKey]ports.InitialRouteNotFormedRecord
	err     error
}

func (double *notFormedReadDouble) FindNotFormed(
	_ context.Context,
	key domain.InitialRouteJudgmentKey,
) (ports.InitialRouteNotFormedRecord, bool, error) {
	if double.err != nil {
		return ports.InitialRouteNotFormedRecord{}, false, double.err
	}
	record, found := double.records[key]
	return record, found, nil
}

func (double *notFormedReadDouble) FindLatestNotFormedOfParcel(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (ports.InitialRouteNotFormedRecord, bool, error) {
	if double.err != nil {
		return ports.InitialRouteNotFormedRecord{}, false, double.err
	}
	for key, record := range double.records {
		if key.TenantID == tenant && key.DeclaredParcelID == parcel {
			return record, true, nil
		}
	}
	return ports.InitialRouteNotFormedRecord{}, false, nil
}

type tieDecisionDouble struct {
	recorded []ports.InitialRouteTieDecision
	err      error
}

func (double *tieDecisionDouble) Record(_ context.Context, decision ports.InitialRouteTieDecision) error {
	if double.err != nil {
		return double.err
	}
	double.recorded = append(double.recorded, decision)
	return nil
}

type tieFixture struct {
	decide     *application.DecideInitialRouteHandler
	locks      *reassessmentStoreDouble
	notFormed  *notFormedReadDouble
	routes     *routeStoreDouble
	decisions  *tieDecisionDouble
	baselines  *baselineGeoDouble
	evidence   *routeEvidenceDouble
	costs      *carriedRecordingCosts
	downstream *routeDownstreamDouble
	ids        *routeIdentityDouble
}

// newTieFixture 落一行并列登记：两家候选都合格、打平在 4200 CNY，证据与登记那一刻同形。
func newTieFixture(t *testing.T) *tieFixture {
	t.Helper()
	key := reassessKey(t)
	fixture := &tieFixture{
		locks: newReassessmentStore(),
		notFormed: &notFormedReadDouble{records: map[domain.InitialRouteJudgmentKey]ports.InitialRouteNotFormedRecord{
			key: {
				Key:         key,
				Correlation: value(t, domain.NewRequestCorrelationID, "route-handoff-1"),
				Reason:      ports.NotFormedCandidatesTied,
				Tied: []domain.CandidateID{
					value(t, domain.NewCandidateID, "candidate-1"), value(t, domain.NewCandidateID, "candidate-2"),
				},
				AcceptedAt: routeJudgedAt.Add(-time.Hour),
				SettledAt:  routeJudgedAt,
			},
		}},
		routes:     newRouteStore(),
		decisions:  &tieDecisionDouble{},
		baselines:  newBaselineGeo(t),
		evidence:   &routeEvidenceDouble{byParcel: map[string]ports.InitialRouteEvidence{"parcel-1": tiedEvidence(t)}},
		costs:      &carriedRecordingCosts{},
		downstream: &routeDownstreamDouble{},
		ids:        &routeIdentityDouble{next: 6},
	}
	fixture.decide = application.NewDecideInitialRouteHandler(application.DecideInitialRouteDeps{
		Locks:      fixture.locks,
		NotFormed:  fixture.notFormed,
		Routes:     fixture.routes,
		Decisions:  fixture.decisions,
		Baselines:  fixture.baselines,
		Evidence:   fixture.evidence,
		Costs:      fixture.costs,
		Downstream: fixture.downstream,
		Identities: fixture.ids,
		Clock:      fixedClock{at: tieDecidedAt},
	})
	return fixture
}

func (fixture *tieFixture) command(t *testing.T, candidate string) application.DecideInitialRouteCommand {
	t.Helper()
	return application.DecideInitialRouteCommand{
		Tenant:    value(t, domain.NewTenantID, "tenant-1"),
		Parcel:    value(t, domain.NewDeclaredParcelID, "parcel-1"),
		Candidate: value(t, domain.NewCandidateID, candidate),
		Basis:     value(t, domain.NewRerouteDecisionBasisReference, "SYN-APPROVAL-97"),
		DecidedBy: value(t, domain.NewAuthorizedRoleReference, decidingOperator),
	}
}

func (fixture *tieFixture) decideOK(t *testing.T, command application.DecideInitialRouteCommand) application.DecideInitialRouteResult {
	t.Helper()
	result, err := fixture.decide.Decide(context.Background(), command)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return result
}

// assertNothingWritten 钉拒与未决什么都不留：不落判断库、不落裁决证据、不交意图、不取号。
func (fixture *tieFixture) assertNothingWritten(t *testing.T) {
	t.Helper()
	if fixture.routes.saved != 0 || len(fixture.decisions.recorded) != 0 || len(fixture.downstream.intents) != 0 || fixture.ids.next != 6 {
		t.Fatalf("saved = %d decisions = %d intents = %d ids = %d; 拒与未决不留任何写入",
			fixture.routes.saved, len(fixture.decisions.recorded), len(fixture.downstream.intents), fixture.ids.next)
	}
}

// Covers: AT-NR-100（ADR-0309 决定二、三）；CONTEXT「并列交授权角色裁决」——授权角色在并列的那几家之间择一：形成这件包裹的首个当前有效
// 计划，从决定的时刻起生效、判断时刻同取；计划落判断库，裁决证据记所择候选、依据与决定方，发布意图带并列入账那次交接的关联；
// 证据与候选成本按受理基线那一版投影取。
func TestAnAuthorizedRoleDecidesALowestCostTie(t *testing.T) {
	fixture := newTieFixture(t)

	result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
	if result.Outcome() != application.InitialRouteTieDecided {
		t.Fatalf("outcome = %q reason = %q, want INITIAL_ROUTE_DECIDED", result.Outcome(), result.UndecidedReason())
	}
	plan, present := result.Plan()
	if !present || plan.SelectedCandidate().String() != "candidate-2" || plan.Key() != reassessKey(t) {
		t.Fatalf("plan present = %v selected = %q", present, plan.SelectedCandidate())
	}
	if !plan.EffectiveFrom().Equal(tieDecidedAt) || !plan.JudgedAt().Equal(tieDecidedAt) {
		t.Fatalf("effective from = %s judged at = %s, want both %s（决定的时刻，不是委托接受的时刻）",
			plan.EffectiveFrom(), plan.JudgedAt(), tieDecidedAt)
	}
	stored, found := fixture.routes.records[reassessKey(t)]
	if !found || !stored.HasPlan || stored.Plan.Version() != plan.Version() || fixture.routes.saved != 1 {
		t.Fatalf("判断库 found = %v saved = %d", found, fixture.routes.saved)
	}
	if len(fixture.decisions.recorded) != 1 {
		t.Fatalf("裁决证据 %d 行, want 1", len(fixture.decisions.recorded))
	}
	recorded := fixture.decisions.recorded[0]
	if recorded.Candidate.String() != "candidate-2" || recorded.Basis.String() != "SYN-APPROVAL-97" ||
		recorded.DecidedBy.String() != decidingOperator || recorded.PlanVersion != plan.Version() ||
		!recorded.DecidedAt.Equal(tieDecidedAt) || recorded.Key != reassessKey(t) {
		t.Fatalf("recorded = %+v", recorded)
	}
	if len(fixture.downstream.intents) != 1 || fixture.downstream.intents[0].Correlation.String() != "route-handoff-1" ||
		fixture.downstream.intents[0].Record.Plan.Version() != plan.Version() {
		t.Fatalf("intents = %+v, want 一份、带并列入账那次交接的关联", fixture.downstream.intents)
	}
	if fixture.evidence.carried.Geo.ContentDigest() != baselineGeo.ContentDigest() {
		t.Fatalf("取证据时的投影 = %q，want 受理基线那一版", fixture.evidence.carried.Geo.ContentDigest())
	}
	if last := fixture.costs.carried[len(fixture.costs.carried)-1]; last.Geo.ContentDigest() != baselineGeo.ContentDigest() {
		t.Fatalf("取成本时的投影 = %q，want 受理基线那一版", last.Geo.ContentDigest())
	}
	// 并列裁决是初始路由的延续，包裹事实仍取客户声明，不因裁决发生在接受之后而改取物理事实。
	if last := fixture.costs.points[len(fixture.costs.points)-1]; last.Kind() != domain.PreRouteFactsPoint {
		t.Fatalf("取成本的判断点 = %v，want 预路由", last.Kind())
	}
}

// Covers: AT-NR-101（ADR-0309 决定二「已有结果」）——这件包裹的判断库已有行（先成的裁决、并列之后重判选出了唯一一条，或无路由）：答
// INITIAL_ROUTE_ALREADY_FORMED 并交回那一行，不重判、不落第二份、不再交意图。
func TestADecisionOnAParcelWithAnInitialRouteAnswersTheExistingResult(t *testing.T) {
	fixture := newTieFixture(t)
	first := fixture.decideOK(t, fixture.command(t, "candidate-1"))
	if first.Outcome() != application.InitialRouteTieDecided {
		t.Fatalf("first outcome = %q", first.Outcome())
	}
	evidenceLoads := fixture.evidence.loaded

	second := fixture.decideOK(t, fixture.command(t, "candidate-2"))
	if second.Outcome() != application.InitialRouteAlreadyFormed {
		t.Fatalf("outcome = %q, want INITIAL_ROUTE_ALREADY_FORMED", second.Outcome())
	}
	existing, present := second.Existing()
	if !present || !existing.HasPlan || existing.Plan.SelectedCandidate().String() != "candidate-1" {
		t.Fatalf("existing present = %v plan = %+v; 交回的是先成的那一份", present, existing)
	}
	if fixture.routes.saved != 1 || len(fixture.decisions.recorded) != 1 || len(fixture.downstream.intents) != 1 ||
		fixture.evidence.loaded != evidenceLoads {
		t.Fatalf("saved = %d decisions = %d intents = %d evidence loads %d→%d; 已有结果不重判不重落",
			fixture.routes.saved, len(fixture.decisions.recorded), len(fixture.downstream.intents), evidenceLoads, fixture.evidence.loaded)
	}
}

// Covers: AT-NR-101（ADR-0309 决定二「没有并列登记」）——包裹没有未形成登记，或登记最近一次停住的不是并列：答 NO_TIE_REGISTERED，后者带
// 它停住的原因，授权角色据此知道卡在哪一格；什么都不写。
func TestADecisionWithoutATieRegistrationIsRefused(t *testing.T) {
	t.Run("no registration", func(t *testing.T) {
		fixture := newTieFixture(t)
		fixture.notFormed.records = map[domain.InitialRouteJudgmentKey]ports.InitialRouteNotFormedRecord{}
		result := fixture.decideOK(t, fixture.command(t, "candidate-1"))
		if result.Outcome() != application.InitialRouteNoTieRegistered || result.RegisteredReason() != ports.InitialRouteNotFormedReasonInvalid {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.RegisteredReason())
		}
		fixture.assertNothingWritten(t)
	})
	t.Run("last stopped elsewhere", func(t *testing.T) {
		fixture := newTieFixture(t)
		key := reassessKey(t)
		record := fixture.notFormed.records[key]
		record.Reason = ports.NotFormedCandidateCostsPending
		record.Tied = nil
		fixture.notFormed.records[key] = record
		result := fixture.decideOK(t, fixture.command(t, "candidate-1"))
		if result.Outcome() != application.InitialRouteNoTieRegistered ||
			result.RegisteredReason() != ports.NotFormedCandidateCostsPending {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.RegisteredReason())
		}
		fixture.assertNothingWritten(t)
	})
}

// Covers: AT-NR-101（ADR-0309 决定二「所择不在并列里」）——所择候选不在并列的那几家里（哪怕它此刻合格）：答 CANDIDATE_NOT_TIED。裁并列
// 不是随意改选，要选并列之外的候选是另一种决定，不在这一口。
func TestACandidateOutsideTheTieIsRefused(t *testing.T) {
	fixture := newTieFixture(t)
	result := fixture.decideOK(t, fixture.command(t, "candidate-3"))
	if result.Outcome() != application.InitialRouteCandidateNotTied {
		t.Fatalf("outcome = %q, want CANDIDATE_NOT_TIED", result.Outcome())
	}
	fixture.assertNothingWritten(t)
}

// Covers: AT-NR-102（ADR-0309 决定二「此刻重判」）；CONTEXT「人工决定同样不得绕过硬限制」——登记之后所择候选被有效硬限制淘汰答
// CANDIDATE_RESTRICTED，被其余层次淘汰或已不在候选空间答 CANDIDATE_NOT_QUALIFIED，证据未知答 CANDIDATE_EVIDENCE_UNKNOWN，各带依据；
// 什么都不写。
func TestAChosenCandidateThatNoLongerHoldsIsRefusedWithItsReason(t *testing.T) {
	cases := map[string]struct {
		mutate  func(t *testing.T, evidence ports.InitialRouteEvidence) ports.InitialRouteEvidence
		outcome application.DecideInitialRouteOutcome
		reason  string
	}{
		"restricted": {
			mutate: func(t *testing.T, evidence ports.InitialRouteEvidence) ports.InitialRouteEvidence {
				evidence.HardConstraints = append(evidence.HardConstraints, restrictionOn(t, "candidate-2", "CUSTOMS-ELIGIBILITY/CC-REF-11"))
				return evidence
			},
			outcome: application.InitialRouteCandidateRestricted,
			reason:  "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-ELIGIBILITY/CC-REF-11",
		},
		"gone from the candidate space": {
			mutate: func(t *testing.T, evidence ports.InitialRouteEvidence) ports.InitialRouteEvidence {
				evidence.ServiceAreas = coveringAreas(t, "candidate-1")
				evidence.Paths = evidence.Paths[:1]
				return evidence
			},
			outcome: application.InitialRouteCandidateNotQualified,
			reason:  "NOT_IN_CANDIDATE_SPACE",
		},
		"restriction status unknown": {
			mutate: func(t *testing.T, evidence ports.InitialRouteEvidence) ports.InitialRouteEvidence {
				finding, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
					Candidate: value(t, domain.NewCandidateID, "candidate-2"),
					Outcome:   domain.ConstraintStatusUnknown,
					Missing:   value(t, domain.NewEvidenceGapReference, "CUSTOMS_CATALOGUE_EMPTY"),
					Reassess:  value(t, domain.NewReassessmentCondition, "WHEN_CUSTOMS_CATALOGUE_REGISTERED"),
				})
				if err != nil {
					t.Fatalf("new unknown finding: %v", err)
				}
				evidence.HardConstraints = append(evidence.HardConstraints, finding)
				return evidence
			},
			outcome: application.InitialRouteCandidateEvidenceUnknown,
			reason:  "HARD_CONSTRAINT_STATUS_UNKNOWN",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newTieFixture(t)
			fixture.evidence.byParcel["parcel-1"] = testCase.mutate(t, fixture.evidence.byParcel["parcel-1"])
			result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
			if result.Outcome() != testCase.outcome || result.CandidateReason() != testCase.reason {
				t.Fatalf("outcome = %q reason = %q, want %q %q", result.Outcome(), result.CandidateReason(),
					testCase.outcome, testCase.reason)
			}
			fixture.assertNothingWritten(t)
		})
	}
}

// Covers: AT-NR-102（ADR-0309 决定二「证据未配置」与「未决」）——判断时点没有适用的网络证据答 INITIAL_ROUTE_EVIDENCE_NOT_CONFIGURED；依赖读不动
// （判断键锁、登记、判断库、受理基线、证据）答 UNDECIDED 带原因与续办引用；都什么也不写。受理基线读成别的包裹、或不是当前已接受包裹是
// 不变量破了，响亮报错，不折成业务答案。
func TestTheTieDecisionNamesWhatItCannotAnswer(t *testing.T) {
	t.Run("evidence not configured", func(t *testing.T) {
		fixture := newTieFixture(t)
		fixture.evidence.unconfigured = map[string]bool{"parcel-1": true}
		result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
		if result.Outcome() != application.InitialRouteEvidenceNotConfigured {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		fixture.assertNothingWritten(t)
	})
	unreadable := errors.New("down")
	for name, arrange := range map[string]struct {
		set    func(fixture *tieFixture)
		reason application.DecideInitialRouteUndecidedReason
	}{
		"register": {func(f *tieFixture) { f.notFormed.err = unreadable }, application.TieRegisterUnavailable},
		"lock":     {func(f *tieFixture) { f.locks.latestErr = unreadable }, application.TieJudgmentLockUnavailable},
		"routes":   {func(f *tieFixture) { f.routes.findErr = unreadable }, application.TieRouteStoreUnavailable},
		"baseline": {func(f *tieFixture) { f.baselines.err = ports.ErrAcceptanceBaselineGeoUnreadable }, application.TieAcceptedParcelUnreadable},
		"evidence": {func(f *tieFixture) { f.evidence.errs = map[string]error{"parcel-1": unreadable} }, application.TieEvidenceUnavailable},
	} {
		t.Run("undecided "+name, func(t *testing.T) {
			fixture := newTieFixture(t)
			arrange.set(fixture)
			result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
			if result.Outcome() != application.InitialRouteDecisionUndecided || result.UndecidedReason() != arrange.reason ||
				result.ContinuationReference().String() == "" {
				t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
			}
			fixture.assertNothingWritten(t)
		})
	}
	t.Run("baseline disagrees", func(t *testing.T) {
		fixture := newTieFixture(t)
		fixture.baselines.answer.AcceptanceBaseline = value(t, domain.NewAcceptanceBaselineReference, "baseline-9/v1")
		if _, err := fixture.decide.Decide(context.Background(), fixture.command(t, "candidate-2")); !errors.Is(err, application.ErrTieBaselineInconsistent) {
			t.Fatalf("err = %v, want ErrTieBaselineInconsistent", err)
		}
		fixture.baselines.found = false
		if _, err := fixture.decide.Decide(context.Background(), fixture.command(t, "candidate-2")); !errors.Is(err, application.ErrTieBaselineInconsistent) {
			t.Fatalf("not accepted: err = %v, want ErrTieBaselineInconsistent", err)
		}
	})
}

// Covers: AT-NR-100（ADR-0309 决定三「同一条提交边界」）——发布意图交不出去，裁决整笔报错、交给事务壳回滚，不像初始路由形成那样只留续办引用：
// 这里一次只有一件包裹，没有同一交接里别的包裹要保。
func TestAFailedHandoffFailsTheWholeTieDecision(t *testing.T) {
	fixture := newTieFixture(t)
	fixture.downstream.err = errors.New("outbox down")
	if _, err := fixture.decide.Decide(context.Background(), fixture.command(t, "candidate-2")); err == nil {
		t.Fatal("意图没交出去却答了成立")
	}
}
