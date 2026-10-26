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

// 本文件钉授权改路决定（ADR-0284；票 docs-consistency/code/29）：先用真复核编排在失效线上形成一份改路建议，再由授权角色据它
// 择一候选。复核与决定共用同一组替身，决定形成的计划经复核册替身的 FormedPlans 进路由历史，与真库并读决定册同一口径。

// decidedAt 是授权角色提交决定的那一刻，晚于建议所在那次复核的触发。
var decidedAt = reassessedAt.Add(3 * time.Hour)

const decidingOperator = "OPERATOR/https://issuer.example#ops-7"

var baselineGeo = domain.NewGeoResolutionProjection(
	domain.NewGeoResolutionSide("CN", true, "200000", true),
	domain.NewGeoResolutionSide("SG", true, "018956", true),
)

// rerouteDecisionStoreDouble 是决定册替身：一份建议一份决定；过时照真库的口径在复核册替身上找「另一次、不早于」的复核。
type rerouteDecisionStoreDouble struct {
	reassessments *reassessmentStoreDouble
	bySuggestion  map[domain.RequestCorrelationID]ports.AuthorizedRerouteDecision
	findErr       error
	overtakenErr  error
	saveErr       error
	saves         int
	// beforeSave 在落册那一刻插进来，演另一笔事务抢先落了同一份建议的决定。
	beforeSave func()
}

func newRerouteDecisionStore(reassessments *reassessmentStoreDouble) *rerouteDecisionStoreDouble {
	store := &rerouteDecisionStoreDouble{
		reassessments: reassessments,
		bySuggestion:  map[domain.RequestCorrelationID]ports.AuthorizedRerouteDecision{},
	}
	reassessments.decided = func(key domain.InitialRouteJudgmentKey) []domain.InitialRoutePlan {
		var plans []domain.InitialRoutePlan
		for _, decision := range store.bySuggestion {
			if decision.Key == key {
				plans = append(plans, decision.Decision.NewPlan())
			}
		}
		return plans
	}
	return store
}

func (double *rerouteDecisionStoreDouble) FindBySuggestion(
	_ context.Context,
	_ domain.TenantID,
	suggestion domain.RequestCorrelationID,
) (ports.AuthorizedRerouteDecision, bool, error) {
	if double.findErr != nil {
		return ports.AuthorizedRerouteDecision{}, false, double.findErr
	}
	decision, found := double.bySuggestion[suggestion]
	return decision, found, nil
}

func (double *rerouteDecisionStoreDouble) Save(
	_ context.Context,
	decision ports.AuthorizedRerouteDecision,
) (ports.AuthorizedRerouteDecisionSaveOutcome, error) {
	if double.beforeSave != nil {
		double.beforeSave()
	}
	if double.saveErr != nil {
		return ports.AuthorizedRerouteDecisionSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.bySuggestion[decision.Suggestion]; exists {
		return ports.AuthorizedRerouteDecisionAlreadyRecorded, nil
	}
	double.bySuggestion[decision.Suggestion] = decision
	double.saves++
	return ports.AuthorizedRerouteDecisionSaved, nil
}

func (double *rerouteDecisionStoreDouble) SuggestionOvertaken(
	_ context.Context,
	key domain.InitialRouteJudgmentKey,
	suggestion domain.RequestCorrelationID,
	at time.Time,
) (bool, error) {
	if double.overtakenErr != nil {
		return false, double.overtakenErr
	}
	for correlation, record := range double.reassessments.byCorrelation {
		if correlation != suggestion && record.Key == key && !record.TriggerOccurredAt.Before(at) {
			return true, nil
		}
	}
	return false, nil
}

// baselineGeoDouble 交回受理基线那一版的投影；判断键各维默认与复核夹具同一组。
type baselineGeoDouble struct {
	answer ports.AcceptanceBaselineGeo
	found  bool
	err    error
	asked  int
}

func newBaselineGeo(t *testing.T) *baselineGeoDouble {
	t.Helper()
	key := reassessKey(t)
	return &baselineGeoDouble{
		answer: ports.AcceptanceBaselineGeo{
			CustomerAccountID:  key.CustomerAccountID,
			ShipmentRequestID:  key.ShipmentRequestID,
			AcceptanceBaseline: key.AcceptanceBaseline,
			Geo:                baselineGeo,
		},
		found: true,
	}
}

func (double *baselineGeoDouble) LoadAcceptanceBaselineGeo(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DeclaredParcelID,
) (ports.AcceptanceBaselineGeo, bool, error) {
	double.asked++
	return double.answer, double.found, double.err
}

type decideFixture struct {
	*reassessFixture
	decide    *application.DecideRerouteHandler
	decisions *rerouteDecisionStoreDouble
	baselines *baselineGeoDouble
	ids       *routeIdentityDouble
	// suggestion 是建议所在那次复核的触发关联。
	suggestion domain.RequestCorrelationID
}

// newDecideFixture 在失效线上形成一份改路建议：交进不在计划上的节点，原计划失效；候选 2 便宜 900 而阈值 1000，只成建议。
// 阈值是本测试交入的租户取值。
func newDecideFixture(t *testing.T) *decideFixture {
	t.Helper()
	fixture := rerouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = declareCostImprovement(cheaperAlternative(t, 100), 1000)
	result := handleOK(t, fixture, reassessCommand(t, "node-actual-elsewhere"))
	record, _ := result.Record()
	if result.Outcome() != application.ReassessedPlanLapsed || !record.HasSuggestion {
		t.Fatalf("夹具没形成建议：outcome = %q suggestion = %v", result.Outcome(), record.HasSuggestion)
	}
	return wireDecide(t, fixture, record.Correlation)
}

func wireDecide(t *testing.T, fixture *reassessFixture, suggestion domain.RequestCorrelationID) *decideFixture {
	t.Helper()
	decide := &decideFixture{
		reassessFixture: fixture,
		decisions:       newRerouteDecisionStore(fixture.store),
		baselines:       newBaselineGeo(t),
		ids:             &routeIdentityDouble{next: 6},
		suggestion:      suggestion,
	}
	decide.decide = application.NewDecideRerouteHandler(application.DecideRerouteDeps{
		Reassessments: fixture.store,
		Decisions:     decide.decisions,
		Applicability: fixture.applicability,
		Baselines:     decide.baselines,
		Evidence:      fixture.evidence,
		Costs:         fixture.costs,
		Identities:    decide.ids,
		Clock:         fixedClock{at: decidedAt},
	})
	return decide
}

func (fixture *decideFixture) command(t *testing.T, candidate string) application.DecideRerouteCommand {
	t.Helper()
	return application.DecideRerouteCommand{
		Tenant:     value(t, domain.NewTenantID, "tenant-1"),
		Parcel:     value(t, domain.NewDeclaredParcelID, "parcel-1"),
		Suggestion: fixture.suggestion,
		Candidate:  value(t, domain.NewCandidateID, candidate),
		Basis:      value(t, domain.NewRerouteDecisionBasisReference, "SYN-APPROVAL-7"),
		DecidedBy:  value(t, domain.NewAuthorizedRoleReference, decidingOperator),
	}
}

func (fixture *decideFixture) decideOK(t *testing.T, command application.DecideRerouteCommand) application.DecideRerouteResult {
	t.Helper()
	result, err := fixture.decide.Decide(context.Background(), command)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return result
}

// Covers: ADR-0284 决定一、三、四；CONTEXT「授权角色接受建议 → 形成改路决定」——授权角色据当前的建议择一合格候选：形成
// 授权角色方式的改路决定与新计划，决定方是会话里认证出的操作者，原计划是失效的那一份，触发是建议的触发；新计划从决定的
// 时刻起生效、判断时刻同取，生效节点是新计划首节点；证据与候选成本按受理基线那一版投影取。原计划的失效照留，适用性不再动。
func TestAnAuthorizedRoleDecidesACurrentSuggestion(t *testing.T) {
	fixture := newDecideFixture(t)
	appliedBefore := len(fixture.applicability.saved)

	result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
	if result.Outcome() != application.RerouteDecided {
		t.Fatalf("outcome = %q reason = %q, want REROUTE_DECIDED", result.Outcome(), result.UndecidedReason())
	}
	recorded, present := result.Decision()
	if !present || fixture.decisions.saves != 1 {
		t.Fatalf("decision present = %v saves = %d", present, fixture.decisions.saves)
	}
	decision := recorded.Decision
	if decision.Mode() != domain.AuthorizedRoleReroute {
		t.Fatalf("mode = %q, want AUTHORIZED_ROLE", decision.Mode())
	}
	if by, present := decision.DecidedBy(); !present || by.String() != decidingOperator {
		t.Fatalf("decided by = %q present = %v", by, present)
	}
	if decision.OriginalPlan().String() != "plan-1/v1" || decision.Trigger().String() != "trigger-1" {
		t.Fatalf("original plan = %q trigger = %q", decision.OriginalPlan(), decision.Trigger())
	}
	plan := decision.NewPlan()
	if plan.SelectedCandidate().String() != "candidate-2" || plan.Version().String() == "plan-1/v1" {
		t.Fatalf("new plan selected = %q version = %q", plan.SelectedCandidate(), plan.Version())
	}
	if !plan.EffectiveFrom().Equal(decidedAt) || !plan.JudgedAt().Equal(decidedAt) || !decision.DecidedAt().Equal(decidedAt) {
		t.Fatalf("effective from = %s judged at = %s decided at = %s, want all %s",
			plan.EffectiveFrom(), plan.JudgedAt(), decision.DecidedAt(), decidedAt)
	}
	if decision.EffectiveFromNode().String() != "node-origin" {
		t.Fatalf("effective node = %q, want 新计划首节点", decision.EffectiveFromNode())
	}
	if recorded.Candidate.String() != "candidate-2" || recorded.Basis.String() != "SYN-APPROVAL-7" ||
		recorded.Suggestion != fixture.suggestion || recorded.Key != reassessKey(t) {
		t.Fatalf("recorded = %+v", recorded)
	}
	if fixture.evidence.carried.Geo.ContentDigest() != baselineGeo.ContentDigest() {
		t.Fatalf("取证据时的投影 = %q，want 受理基线那一版", fixture.evidence.carried.Geo.ContentDigest())
	}
	if last := fixture.costs.carried[len(fixture.costs.carried)-1]; last.Geo.ContentDigest() != baselineGeo.ContentDigest() {
		t.Fatalf("取成本时的投影 = %q，want 受理基线那一版", last.Geo.ContentDigest())
	}
	if len(fixture.applicability.saved) != appliedBefore {
		t.Fatalf("适用性多写了 %d 次; 原计划已失效，新计划没有登记行即当前有效", len(fixture.applicability.saved)-appliedBefore)
	}
}

// Covers: ADR-0284 决定五——决定形成的计划进路由历史：下一次复核从它起判，在它的计划节点上仍适用、引用它的版本。
func TestTheNextReassessmentStartsFromTheDecidedPlan(t *testing.T) {
	fixture := newDecideFixture(t)
	decided := fixture.decideOK(t, fixture.command(t, "candidate-2"))
	recorded, _ := decided.Decision()

	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
	next := reassessCommand(t, "node-origin")
	next.Trigger.Correlation = value(t, domain.NewRequestCorrelationID, "trigger-2")
	next.Trigger.OccurredAt = decidedAt.Add(time.Hour)
	result := handleOK(t, fixture.reassessFixture, next)
	if result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE", result.Outcome())
	}
	record, _ := result.Record()
	if record.ReviewedPlan != recorded.Decision.NewPlan().Version() {
		t.Fatalf("reviewed plan = %q, want 决定形成的 %q", record.ReviewedPlan, recorded.Decision.NewPlan().Version())
	}
}

// Covers: ADR-0284 决定二「过时」——建议所在那次复核之后，同一判断键上另有一次复核（更晚，或同一时刻）：建议过时，答
// SUGGESTION_SUPERSEDED，不形成决定，也不去读证据。
func TestASuggestionOvertakenByAnotherReassessmentIsSuperseded(t *testing.T) {
	for name, at := range map[string]time.Time{
		"later":              reassessedAt.Add(time.Minute),
		"same business time": reassessedAt,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDecideFixture(t)
			other := value(t, domain.NewRequestCorrelationID, "trigger-later")
			fixture.store.byCorrelation[other] = ports.ReassessmentRecord{
				Correlation:       other,
				Key:               reassessKey(t),
				Conclusion:        ports.ReassessmentPlanLapsed,
				CandidateState:    ports.NoQualifiedCandidates,
				TriggerOccurredAt: at,
				ReassessedAt:      reassessedAt.Add(time.Hour),
			}
			loaded := fixture.evidence.loaded

			result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
			if result.Outcome() != application.RerouteSuggestionSuperseded {
				t.Fatalf("outcome = %q, want SUGGESTION_SUPERSEDED", result.Outcome())
			}
			if fixture.decisions.saves != 0 || fixture.evidence.loaded != loaded {
				t.Fatalf("saves = %d evidence loads = %d; 过时的建议不往下判", fixture.decisions.saves, fixture.evidence.loaded-loaded)
			}
		})
	}
}

// Covers: ADR-0284 决定二「已决定」——同一份建议第二次决定（另一位授权角色，或同一位重放），答已决定并交回原决定，不再落册。
func TestASecondDecisionOnTheSameSuggestionAnswersAlreadyDecided(t *testing.T) {
	fixture := newDecideFixture(t)
	first := fixture.decideOK(t, fixture.command(t, "candidate-2"))
	firstDecision, _ := first.Decision()

	second := fixture.command(t, "candidate-1")
	second.DecidedBy = value(t, domain.NewAuthorizedRoleReference, "OPERATOR/https://issuer.example#ops-8")
	result := fixture.decideOK(t, second)
	if result.Outcome() != application.RerouteSuggestionAlreadyDecided {
		t.Fatalf("outcome = %q, want SUGGESTION_ALREADY_DECIDED", result.Outcome())
	}
	existing, present := result.Decision()
	if !present || existing.Decision.NewPlan().Version() != firstDecision.Decision.NewPlan().Version() {
		t.Fatalf("existing = %+v present = %v; 交回的应是先成的那一份", existing, present)
	}
	if fixture.decisions.saves != 1 {
		t.Fatalf("saves = %d, want 1", fixture.decisions.saves)
	}
}

// Covers: ADR-0284 决定五「并发」——锁外看着没有决定、落册时另一笔已抢先落了同一份建议：撞键读回赢家，答已决定。
func TestALosingConcurrentDecisionReadsBackTheWinner(t *testing.T) {
	fixture := newDecideFixture(t)
	winner := fixture.command(t, "candidate-1")
	fixture.decisions.beforeSave = func() {
		fixture.decisions.beforeSave = nil
		if _, err := fixture.decide.Decide(context.Background(), winner); err != nil {
			t.Fatalf("winner decide: %v", err)
		}
	}
	result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
	if result.Outcome() != application.RerouteSuggestionAlreadyDecided {
		t.Fatalf("outcome = %q, want SUGGESTION_ALREADY_DECIDED", result.Outcome())
	}
	existing, _ := result.Decision()
	if existing.Candidate.String() != "candidate-1" || fixture.decisions.saves != 1 {
		t.Fatalf("existing candidate = %q saves = %d; 只有一笔成立", existing.Candidate, fixture.decisions.saves)
	}
}

// Covers: ADR-0284 决定二「建议不存在」——没有这个关联的复核、那次复核没有建议、建议不是指名的包裹，都答 SUGGESTION_NOT_FOUND。
func TestASuggestionThatIsNotThereIsNotFound(t *testing.T) {
	fixture := newDecideFixture(t)

	unknown := fixture.command(t, "candidate-2")
	unknown.Suggestion = value(t, domain.NewRequestCorrelationID, "trigger-unknown")

	plain := value(t, domain.NewRequestCorrelationID, "trigger-plain")
	fixture.store.byCorrelation[plain] = ports.ReassessmentRecord{
		Correlation: plain, Key: reassessKey(t), Conclusion: ports.ReassessmentPlanLapsed,
		CandidateState: ports.NoQualifiedCandidates, TriggerOccurredAt: reassessedAt.Add(-time.Minute),
	}
	noSuggestion := fixture.command(t, "candidate-2")
	noSuggestion.Suggestion = plain

	otherParcel := fixture.command(t, "candidate-2")
	otherParcel.Parcel = value(t, domain.NewDeclaredParcelID, "parcel-2")

	for name, command := range map[string]application.DecideRerouteCommand{
		"unknown correlation": unknown, "no suggestion": noSuggestion, "other parcel": otherParcel,
	} {
		if result := fixture.decideOK(t, command); result.Outcome() != application.RerouteSuggestionNotFound {
			t.Errorf("%s: outcome = %q, want SUGGESTION_NOT_FOUND", name, result.Outcome())
		}
	}
	if fixture.decisions.saves != 0 {
		t.Fatalf("saves = %d", fixture.decisions.saves)
	}
}

// Covers: ADR-0284 决定二「不在建议之列」——所择候选不在建议的候选里，答 CANDIDATE_NOT_SUGGESTED。
func TestACandidateOutsideTheSuggestionIsRefused(t *testing.T) {
	fixture := newDecideFixture(t)
	result := fixture.decideOK(t, fixture.command(t, "candidate-9"))
	if result.Outcome() != application.RerouteCandidateNotSuggested {
		t.Fatalf("outcome = %q, want CANDIDATE_NOT_SUGGESTED", result.Outcome())
	}
	if fixture.decisions.saves != 0 || fixture.ids.next != 6 {
		t.Fatalf("saves = %d ids = %d; 拒了不落册、不取号", fixture.decisions.saves, fixture.ids.next)
	}
}

// Covers: ADR-0284 决定二「此刻重判」；CONTEXT「人工决定同样不得绕过硬限制」——建议形成之后，所择候选被有效硬限制淘汰：答
// CANDIDATE_RESTRICTED、带淘汰依据，不形成决定。
func TestACandidateRestrictedSinceTheSuggestionIsRefused(t *testing.T) {
	fixture := newDecideFixture(t)
	evidence := fixture.evidence.byParcel["parcel-1"]
	evidence.HardConstraints = append(evidence.HardConstraints, restrictionOn(t, "candidate-2", "CUSTOMS-ELIGIBILITY/CC-REF-11"))
	fixture.evidence.byParcel["parcel-1"] = evidence

	result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
	if result.Outcome() != application.RerouteCandidateRestricted {
		t.Fatalf("outcome = %q, want CANDIDATE_RESTRICTED", result.Outcome())
	}
	if result.CandidateReason() != "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-ELIGIBILITY/CC-REF-11" {
		t.Fatalf("reason = %q", result.CandidateReason())
	}
	if fixture.decisions.saves != 0 || fixture.ids.next != 6 {
		t.Fatalf("saves = %d ids = %d; 拒了不落册、不取号", fixture.decisions.saves, fixture.ids.next)
	}
}

// Covers: ADR-0284 决定二「此刻重判」其余两格——被其余层次淘汰或已不在候选空间答 CANDIDATE_NOT_QUALIFIED；证据未知答
// CANDIDATE_EVIDENCE_UNKNOWN；各带依据。
func TestACandidateNoLongerQualifiedOrUnknownIsRefused(t *testing.T) {
	cases := map[string]struct {
		mutate  func(t *testing.T, evidence ports.InitialRouteEvidence) ports.InitialRouteEvidence
		outcome application.DecideRerouteOutcome
		reason  string
	}{
		"gone from the candidate space": {
			mutate: func(t *testing.T, evidence ports.InitialRouteEvidence) ports.InitialRouteEvidence {
				evidence.ServiceAreas = coveringAreas(t, "candidate-1")
				evidence.Paths = evidence.Paths[:1]
				return evidence
			},
			outcome: application.RerouteCandidateNotQualified,
			reason:  "NOT_IN_CANDIDATE_SPACE",
		},
		"path not executable": {
			mutate: func(t *testing.T, evidence ports.InitialRouteEvidence) ports.InitialRouteEvidence {
				finding, err := domain.NewPathExecutability(domain.PathExecutabilitySpec{
					Candidate: value(t, domain.NewCandidateID, "candidate-2"),
					Outcome:   domain.PathNotExecutable,
					Schedule:  value(t, domain.NewScheduleVersionReference, "CLOSURE/CONN-9"),
				})
				if err != nil {
					t.Fatalf("new path executability: %v", err)
				}
				evidence.PathExecutability = append(evidence.PathExecutability, finding)
				return evidence
			},
			outcome: application.RerouteCandidateNotQualified,
			reason:  "PATH_NOT_EXECUTABLE/CLOSURE/CONN-9",
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
			outcome: application.RerouteCandidateEvidenceUnknown,
			reason:  "HARD_CONSTRAINT_STATUS_UNKNOWN",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDecideFixture(t)
			fixture.evidence.byParcel["parcel-1"] = testCase.mutate(t, fixture.evidence.byParcel["parcel-1"])
			result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
			if result.Outcome() != testCase.outcome || result.CandidateReason() != testCase.reason {
				t.Fatalf("outcome = %q reason = %q, want %q %q", result.Outcome(), result.CandidateReason(),
					testCase.outcome, testCase.reason)
			}
			if fixture.decisions.saves != 0 {
				t.Fatalf("saves = %d", fixture.decisions.saves)
			}
		})
	}
}

// Covers: ADR-0284 决定二——判断时点没有适用的网络证据，答 REROUTE_EVIDENCE_NOT_CONFIGURED，不是未决。
func TestADecisionWithoutConfiguredEvidenceAnswersNotConfigured(t *testing.T) {
	fixture := newDecideFixture(t)
	fixture.evidence.unconfigured = map[string]bool{"parcel-1": true}
	result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
	if result.Outcome() != application.RerouteEvidenceNotConfigured {
		t.Fatalf("outcome = %q, want REROUTE_EVIDENCE_NOT_CONFIGURED", result.Outcome())
	}
}

// Covers: ADR-0284 决定二「只有依赖读不动答未决」——各依赖读不动答 UNDECIDED 带原因与续办引用，不落册。
func TestDependenciesThatCannotBeReadLeaveTheDecisionUndecided(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]struct {
		break_ func(fixture *decideFixture)
		reason string
	}{
		"judgment key lock": {func(fixture *decideFixture) { fixture.store.latestErr = boom }, "REASSESSMENT_STORE_UNAVAILABLE"},
		"overtaken read":    {func(fixture *decideFixture) { fixture.decisions.overtakenErr = boom }, "REASSESSMENT_STORE_UNAVAILABLE"},
		"decision register": {func(fixture *decideFixture) { fixture.decisions.findErr = boom }, "DECISION_STORE_UNAVAILABLE"},
		"decision write":    {func(fixture *decideFixture) { fixture.decisions.saveErr = boom }, "DECISION_STORE_UNAVAILABLE"},
		"applicability":     {func(fixture *decideFixture) { fixture.applicability.findErr = boom }, "ROUTING_HISTORY_UNAVAILABLE"},
		"parcel shipment": {func(fixture *decideFixture) {
			fixture.baselines.err = errors.Join(ports.ErrAcceptanceBaselineGeoUnreadable, boom)
		}, "ACCEPTED_PARCEL_UNREADABLE"},
		"evidence":    {func(fixture *decideFixture) { fixture.evidence.errs["parcel-1"] = boom }, "REROUTE_EVIDENCE_UNAVAILABLE"},
		"cost source": {func(fixture *decideFixture) { fixture.costs.err = boom }, "REROUTE_COST_SOURCE_UNAVAILABLE"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDecideFixture(t)
			testCase.break_(fixture)
			result := fixture.decideOK(t, fixture.command(t, "candidate-2"))
			if result.Outcome() != application.RerouteDecisionUndecided || result.UndecidedReason().String() != testCase.reason {
				t.Fatalf("outcome = %q reason = %q, want UNDECIDED %s", result.Outcome(), result.UndecidedReason(), testCase.reason)
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("未决没带续办引用")
			}
			if fixture.decisions.saves != 0 {
				t.Fatalf("saves = %d", fixture.decisions.saves)
			}
		})
	}
}

// Covers: ADR-0284 决定三——受理基线那一版读回来与建议的判断键对不上，或读成「不是当前已接受包裹」，是不变量破了：响亮报错，
// 不进未决、不落册。
func TestABaselineThatDisagreesWithTheSuggestionIsLoud(t *testing.T) {
	for name, breakIt := range map[string]func(*decideFixture){
		"not accepted": func(fixture *decideFixture) { fixture.baselines.found = false },
		"other baseline": func(fixture *decideFixture) {
			fixture.baselines.answer.AcceptanceBaseline = value(t, domain.NewAcceptanceBaselineReference, "baseline-1/v2")
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDecideFixture(t)
			breakIt(fixture)
			if _, err := fixture.decide.Decide(context.Background(), fixture.command(t, "candidate-2")); err == nil {
				t.Fatal("decide answered; want a loud error")
			}
			if fixture.decisions.saves != 0 {
				t.Fatalf("saves = %d", fixture.decisions.saves)
			}
		})
	}
}

// Covers: AT-NR-084 的决定那一半（ADR-0249 决定二、ADR-0284 决定四）——揽收复核形成的建议，授权角色在包裹交进首个节点之前
// 决定：新计划从决定的时刻起生效，生效节点是包裹将交进的首个节点（新计划首节点）。阈值 0 是本测试交入的租户取值。
func TestAPickupSuggestionIsDecidedFromTheFirstNode(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = restrictedSelectedCandidate(t,
		declareCostImprovement(cheaperAlternative(t, 100), 0))
	lapsed := handleOK(t, fixture, pickupCommand(t))
	suggestion, _ := lapsed.Record()
	if !suggestion.HasSuggestion {
		t.Fatal("揽收失效没形成建议")
	}
	decide := wireDecide(t, fixture, suggestion.Correlation)

	// 原计划所选的候选在建议里就被限制淘汰了：选它答不在建议之列，人工决定不绕过。
	refused := decide.decideOK(t, decide.command(t, "candidate-1"))
	if refused.Outcome() != application.RerouteCandidateNotSuggested {
		t.Fatalf("outcome = %q, want CANDIDATE_NOT_SUGGESTED", refused.Outcome())
	}

	result := decide.decideOK(t, decide.command(t, "candidate-2"))
	if result.Outcome() != application.RerouteDecided {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	recorded, _ := result.Decision()
	if recorded.Decision.EffectiveFromNode().String() != "node-origin" ||
		!recorded.Decision.NewPlan().EffectiveFrom().Equal(decidedAt) {
		t.Fatalf("effective node = %q from = %s", recorded.Decision.EffectiveFromNode(), recorded.Decision.NewPlan().EffectiveFrom())
	}
}

func restrictionOn(t *testing.T, candidate, restriction string) domain.HardConstraintFinding {
	t.Helper()
	finding, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
		Candidate:   value(t, domain.NewCandidateID, candidate),
		Outcome:     domain.RestrictionApplies,
		Restriction: value(t, domain.NewRestrictionReference, restriction),
	})
	if err != nil {
		t.Fatalf("new restriction finding: %v", err)
	}
	return finding
}
