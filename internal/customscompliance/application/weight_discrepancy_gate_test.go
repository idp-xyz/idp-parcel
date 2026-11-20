package application_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

func TestWeightDiscrepancyStopsAreNamedAndDoNotDefaultToMet(t *testing.T) {
	rule := mustWeightRule(t, "0.5", "KG")
	cases := []struct {
		name   string
		mutate func(*gateFixture)
		want   application.VerifyGateUndecidedReason
	}{
		{"规则未配置", func(f *gateFixture) { f.weightRules.found = false }, application.WeightDiscrepancyGateRuleNotConfigured},
		{"规则读口故障", func(f *gateFixture) { f.weightRules.err = errors.New("weight rule down") }, application.WeightDiscrepancyGateRuleViewUnavailable},
		{"规则旁另登认定", func(f *gateFixture) {
			f.conditions.findings = []domain.PreconditionFinding{{
				Precondition: domain.WeightDiscrepancyPrecondition,
				State:        domain.PreconditionMet,
			}}
		}, application.WeightDiscrepancyFindingRegisteredBesideRule},
		{"提交版本未形成", func(f *gateFixture) {
			f.weightRules.rule = rule
			f.submissions.found = false
		}, application.DeclarationSubmissionAbsent},
		{"提交版本读口故障", func(f *gateFixture) {
			f.weightRules.rule = rule
			f.submissions.err = errors.New("submission down")
		}, application.DeclarationSubmissionViewUnavailable},
		{"成员没有实测", func(f *gateFixture) {
			f.weightRules.rule = rule
			f.submissions.record = weightSubmissionRecord(t, "2.10", "SYN-WEIGH-FIX")
			f.submissions.found = true
		}, application.GateMemberMeasurementAbsent},
		{"成员实测定不下", func(f *gateFixture) {
			f.weightRules.rule = rule
			f.submissions.record = weightSubmissionRecord(t, "2.10", "SYN-WEIGH-FIX")
			f.submissions.found = true
			f.memberWeights.byParcel = map[string]ports.MeasuredWeightReading{
				"parcel-a": {Answer: ports.MeasuredWeightUndetermined, Cause: "TWO_AT_ONE_INSTANT"},
			}
		}, application.GateMemberMeasurementUndetermined},
		{"实测读口故障", func(f *gateFixture) {
			f.weightRules.rule = rule
			f.submissions.record = weightSubmissionRecord(t, "2.10", "SYN-WEIGH-FIX")
			f.submissions.found = true
			f.memberWeights.err = errors.New("measurement down")
		}, application.GateMemberMeasurementViewUnavailable},
		{"单位换不出", func(f *gateFixture) {
			f.weightRules.rule = rule
			f.submissions.record = weightSubmissionRecord(t, "2.10", "SYN-WEIGH-FIX")
			f.submissions.found = true
			f.memberWeights.byParcel = map[string]ports.MeasuredWeightReading{
				"parcel-a": {Answer: ports.MeasuredWeightOne, GrossWeight: "2.40", GrossWeightUnit: "CM", MeasurementSource: "SYN-WEIGH-CM"},
			}
		}, application.GateMemberMeasurementUndetermined},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newGateFixture(t)
			testCase.mutate(fixture)
			result, err := fixture.handler.Handle(t.Context(), gateCommand(t))
			if err != nil {
				t.Fatalf("停点被当成编程错误：%v", err)
			}
			if result.Outcome() != application.GateVerificationUndecided || result.UndecidedReason() != testCase.want {
				t.Fatalf("= %s / %s，要未决 / %s", result.Outcome(), result.UndecidedReason(), testCase.want)
			}
			if fixture.store.saved != 0 {
				t.Fatal("停点落了册")
			}
		})
	}
}

func TestAWeightDiscrepancyOutsideToleranceIsRecordedAndAReplayHitsTheSameReading(t *testing.T) {
	fixture := newGateFixture(t)
	fixture.weightRules.rule = mustWeightRule(t, "0.50", "KG")
	fixture.submissions.record = weightSubmissionRecord(t, "2.10", "SYN-WEIGH-FIX")
	fixture.submissions.found = true
	fixture.memberWeights.byParcel = map[string]ports.MeasuredWeightReading{
		"parcel-a": {Answer: ports.MeasuredWeightOne, GrossWeight: "3.00", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-OVER"},
	}
	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "SYN-PRE-EXPORT-DECL-RELEASED", domain.PreconditionMet),
	}
	over, err := fixture.handler.Handle(t.Context(), gateCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	gate, ok := over.Gate()
	reading, has := gate.WeightDiscrepancy()
	if over.Outcome() != application.GateVerificationRecorded || !ok || gate.Conclusion() != domain.GatePartiallyMet ||
		!has || reading.State != domain.PreconditionUnmet || reading.Members[0].WithinTolerance || reading.MaxDelta != "0.5" {
		t.Fatalf("超容差 = %s %s %+v", over.Outcome(), gate.Conclusion(), reading)
	}

	again, err := fixture.handler.Handle(t.Context(), gateCommand(t))
	if err != nil || again.Outcome() != application.GateVerificationExisting || fixture.store.saved != 1 {
		t.Fatalf("同实测重放 = %s saved=%d err=%v", again.Outcome(), fixture.store.saved, err)
	}

	fixture.memberWeights.byParcel["parcel-a"] = ports.MeasuredWeightReading{
		Answer: ports.MeasuredWeightOne, GrossWeight: "2.40", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-IN",
	}
	within, err := fixture.handler.Handle(t.Context(), gateCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	withinGate, _ := within.Gate()
	withinReading, _ := withinGate.WeightDiscrepancy()
	if within.Outcome() != application.GateVerificationRecorded || withinGate.Conclusion() != domain.GateMet ||
		!withinReading.Members[0].WithinTolerance || fixture.store.saved != 2 {
		t.Fatalf("容差内 = %s %s %+v saved=%d", within.Outcome(), withinGate.Conclusion(), withinReading, fixture.store.saved)
	}
}

func TestWeightDiscrepancyAnswersBlockedUndecidedOrAdmitted(t *testing.T) {
	over := weightAdmission(t, "3.00", "SYN-WEIGH-OVER", false, nil)
	overJudgement, _ := judged(t, over)
	overFinding := findingOfAdmission(t, overJudgement, domain.LoadingDeparture)
	if overJudgement.Answer() != domain.AdmissionBlocked || overFinding.Bases[0].Cause != domain.CauseGateUnmet ||
		!containsSource(overFinding.Bases[0].Sources, "WEIGHT_DISCREPANCY") || overJudgement.Answer().Admits() {
		t.Fatalf("超容差无放行 = %s %+v", overJudgement.Answer(), overFinding)
	}

	partialFixture := newWeightAdmission(t, "3.00", "SYN-WEIGH-OVER", false)
	partialFixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "SYN-PRE-EXPORT-DECL-RELEASED", domain.PreconditionMet),
	}
	partial, err := partialFixture.handler(t).Handle(t.Context(), admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"))
	if err != nil {
		t.Fatal(err)
	}
	partialJudgement, _ := judged(t, partial)
	partialFinding := findingOfAdmission(t, partialJudgement, domain.LoadingDeparture)
	if partialJudgement.Answer() != domain.AdmissionBlocked || partialFinding.Bases[0].Cause != domain.CauseGatePartiallyMet ||
		!containsSource(partialFinding.Bases[0].Sources, "WEIGHT_DISCREPANCY") {
		t.Fatalf("部分满足 = %s %+v", partialJudgement.Answer(), partialFinding)
	}

	released := weightAdmission(t, "3.00", "SYN-WEIGH-OVER", true, nil)
	releasedJudgement, _ := judged(t, released)
	releasedFinding := findingOfAdmission(t, releasedJudgement, domain.LoadingDeparture)
	if releasedJudgement.Answer() != domain.AdmissionUndecided || releasedFinding.Bases[0].Cause != domain.CauseGateReleaseDiscrepancy ||
		releasedJudgement.Answer().Admits() {
		t.Fatalf("已放行但超容差 = %s %+v admits=%v", releasedJudgement.Answer(), releasedFinding, releasedJudgement.Answer().Admits())
	}

	admitted := weightAdmission(t, "2.40", "SYN-WEIGH-IN", true, nil)
	admittedJudgement, _ := judged(t, admitted)
	if admittedJudgement.Answer() != domain.AdmissionAdmitted || !admittedJudgement.Answer().Admits() {
		t.Fatalf("容差内且已全部放行 = %s", admittedJudgement.Answer())
	}
}

func TestAdmissionNamesTheWeightStops(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*admissionFixture)
		answer domain.AdmissionAnswer
		cause  domain.AdmissionCause
		reason application.ActionAdmissionUnreadableReason
	}{
		{"规则未登", func(f *admissionFixture) { f.weightRules.found = false }, domain.AdmissionNotConfigured, domain.CauseWeightDiscrepancyRuleNotRegistered, 0},
		{"规则旁另登认定", func(f *admissionFixture) {
			f.conditions.findings = []domain.PreconditionFinding{{
				Precondition: domain.WeightDiscrepancyPrecondition, State: domain.PreconditionMet,
			}}
		}, domain.AdmissionNotConfigured, domain.CauseWeightDiscrepancyFindingBesideRule, 0},
		{"提交版本未形成", func(f *admissionFixture) { f.weightRules.rule = mustWeightRule(t, "0.5", "KG") }, domain.AdmissionUndecided, domain.CauseDeclarationSubmissionAbsent, 0},
		{"没有实测", func(f *admissionFixture) {
			f.weightRules.rule = mustWeightRule(t, "0.5", "KG")
			f.submissions.record = weightSubmissionRecord(t, "2.10", "SYN-WEIGH-FIX")
			f.submissions.found = true
		}, domain.AdmissionUndecided, domain.CauseMemberMeasurementAbsent, 0},
		{"实测定不下", func(f *admissionFixture) {
			f.weightRules.rule = mustWeightRule(t, "0.5", "KG")
			f.submissions.record = weightSubmissionRecord(t, "2.10", "SYN-WEIGH-FIX")
			f.submissions.found = true
			f.memberWeights.byParcel = map[string]ports.MeasuredWeightReading{
				"parcel-a": {Answer: ports.MeasuredWeightUndetermined},
			}
		}, domain.AdmissionUndecided, domain.CauseMemberMeasurementUndetermined, 0},
		{"规则读口故障", func(f *admissionFixture) { f.weightRules.err = errors.New("down") }, 0, 0, application.AdmissionWeightDiscrepancyRuleUnavailable},
		{"提交版本读口故障", func(f *admissionFixture) {
			f.weightRules.rule = mustWeightRule(t, "0.5", "KG")
			f.submissions.err = errors.New("down")
		}, 0, 0, application.AdmissionDeclarationSubmissionUnavailable},
		{"实测读口故障", func(f *admissionFixture) {
			f.weightRules.rule = mustWeightRule(t, "0.5", "KG")
			f.submissions.record = weightSubmissionRecord(t, "2.10", "SYN-WEIGH-FIX")
			f.submissions.found = true
			f.memberWeights.err = errors.New("down")
		}, 0, 0, application.AdmissionMemberMeasurementUnavailable},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newAdmissionFixture()
			fixture.inUnit(t)
			testCase.mutate(fixture)
			result, err := fixture.handler(t).Handle(t.Context(), admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"))
			if err != nil {
				t.Fatal(err)
			}
			if testCase.reason != 0 {
				if result.Outcome() != application.ActionAdmissionUnreadable || result.UnreadableReason() != testCase.reason {
					t.Fatalf("= %s / %s，要读不动 / %s", result.Outcome(), result.UnreadableReason(), testCase.reason)
				}
				return
			}
			judgement, _ := judged(t, result)
			finding := findingOfAdmission(t, judgement, domain.LoadingDeparture)
			if judgement.Answer() != testCase.answer || finding.Bases[0].Cause != testCase.cause {
				t.Fatalf("= %s %+v，要 %s / %s", judgement.Answer(), finding, testCase.answer, testCase.cause)
			}
		})
	}
}

func weightAdmission(t *testing.T, measured, source string, withRelease bool, mutate func(*admissionFixture)) application.JudgeActionAdmissionResult {
	t.Helper()
	fixture := newWeightAdmission(t, measured, source, withRelease)
	if mutate != nil {
		mutate(fixture)
	}
	result, err := fixture.handler(t).Handle(t.Context(), admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func newWeightAdmission(t *testing.T, measured, source string, withRelease bool) *admissionFixture {
	t.Helper()
	fixture := newAdmissionFixture()
	fixture.inUnit(t)
	fixture.weightRules.rule = mustWeightRule(t, "0.5", "KG")
	fixture.submissions.record = weightSubmissionRecord(t, "2.10", "SYN-WEIGH-FIX")
	fixture.submissions.found = true
	fixture.memberWeights.byParcel = map[string]ports.MeasuredWeightReading{
		"parcel-a": {Answer: ports.MeasuredWeightOne, GrossWeight: measured, GrossWeightUnit: "KG", MeasurementSource: source},
	}
	if withRelease {
		release, err := domain.ReceiveReleaseOutcome(domain.FullRelease,
			mustValue(t, domain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY-CN"),
			mustValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-CN-EXPORT-01"),
			"", admissionAppAt.Add(-time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		fixture.releases.releases = []domain.ReleaseReading{{
			Source: "SYN-RELEASE-01", Release: release, OccurredAt: admissionAppAt.Add(-time.Hour),
		}}
	}
	return fixture
}

func findingOfAdmission(t *testing.T, judgement domain.ActionAdmissionJudgement, action domain.GuardedAction) domain.ActionAdmissionFinding {
	t.Helper()
	for _, finding := range judgement.Findings() {
		if finding.Action == action {
			return finding
		}
	}
	t.Fatalf("没有 %s 的答复", action)
	return domain.ActionAdmissionFinding{}
}

func containsSource(sources []string, want string) bool {
	for _, source := range sources {
		if source == want {
			return true
		}
	}
	return false
}

func mustWeightRule(t *testing.T, delta, unit string) domain.WeightDiscrepancyGateRule {
	t.Helper()
	rule, err := domain.AllowWeightDeltaUpTo(delta, unit)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func weightSubmissionRecord(t *testing.T, gross, source string) ports.DeclarationSubmissionRecord {
	t.Helper()
	parcel := mustValue(t, domain.NewDeclaredParcelReference, "parcel-a")
	unit, err := domain.FormDeclarationUnit(
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		mustValue(t, domain.NewCustomsCaseID, "case-1"),
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		[]domain.DeclaredParcelReference{parcel})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC)
	readiness, err := domain.JudgeReady(unit.ID(), mustValue(t, domain.NewReadinessBasisReference, "readiness-1"), at)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := domain.GrantSubmissionAuthority(unit.ID(), mustValue(t, domain.NewSubmissionAuthorityReference, "authority-1"), at)
	if err != nil {
		t.Fatal(err)
	}
	weight, err := domain.NewMemberMeasuredWeight(parcel, gross, "KG", source, at)
	if err != nil {
		t.Fatal(err)
	}
	version, err := domain.FixSubmissionVersion(domain.CustomsSubmissionVersionSpec{
		ID:            mustValue(t, domain.NewSubmissionVersionID, "submission-1"),
		Unit:          unit,
		Dossier:       mustValue(t, domain.NewDossierSnapshotReference, "dossier-1"),
		Roles:         mustValue(t, domain.NewRoleSnapshotReference, "roles-1"),
		Readiness:     readiness,
		Authorization: authorization,
		FixedAt:       at.Add(time.Hour),
		MemberWeights: []domain.MemberMeasuredWeight{weight},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ports.DeclarationSubmissionRecord{Version: version}
}
