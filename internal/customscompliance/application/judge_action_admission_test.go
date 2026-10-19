package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 准入编排的替身用例（隔离合成 S）。三源的折法由领域用例钉，这里钉编排自己的事：读了哪几口、哪一口读不动
// 停在哪一格、重放命中已有记录、门禁停点译成哪种读数。真库那一半在 adapters/postgres。

var admissionAppAt = time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)

type zoneViewDouble struct {
	zones map[string]string
	err   error
}

func (double *zoneViewDouble) LoadNodeCustomsZone(
	_ context.Context,
	_ domain.TenantID,
	node domain.NetworkNodeReference,
	_ time.Time,
) (ports.NodeCustomsZoneEntry, bool, error) {
	if double.err != nil {
		return ports.NodeCustomsZoneEntry{}, false, double.err
	}
	zone, ok := double.zones[node.String()]
	if !ok {
		return ports.NodeCustomsZoneEntry{}, false, nil
	}
	built, err := domain.NewCustomsZoneReference(zone)
	return ports.NodeCustomsZoneEntry{Node: node, Zone: built}, true, err
}

type objectScopeViewDouble struct {
	scope domain.ObjectCustomsScope
	err   error
}

func (double *objectScopeViewDouble) LoadObjectCustomsScope(
	context.Context, domain.TenantID, domain.DeclaredParcelReference,
) (domain.ObjectCustomsScope, error) {
	return double.scope, double.err
}

type restrictionViewDouble struct {
	asked []string
	err   error
}

func (double *restrictionViewDouble) ListByScope(
	_ context.Context, _ domain.TenantID, scope domain.DecisionScopeReference,
) ([]domain.RegulatoryRestriction, error) {
	double.asked = append(double.asked, scope.String())
	return nil, double.err
}

type releaseViewDouble struct {
	releases []domain.ReleaseReading
	err      error
}

func (double *releaseViewDouble) ListReleaseOutcomes(
	context.Context, domain.TenantID, domain.DecisionScopeReference,
) ([]domain.ReleaseReading, error) {
	return double.releases, double.err
}

type admissionStoreDouble struct {
	recorded map[string]time.Time
	saveErr  error
	findErr  error
}

func (double *admissionStoreDouble) SaveActionAdmissionJudgement(
	_ context.Context, _ domain.TenantID, id string, judgement domain.ActionAdmissionJudgement,
) (ports.ActionAdmissionSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.ActionAdmissionSaveOutcomeInvalid, double.saveErr
	}
	if _, ok := double.recorded[id]; ok {
		return ports.ActionAdmissionAlreadyRecorded, nil
	}
	double.recorded[id] = judgement.JudgedAt()
	return ports.ActionAdmissionSaved, nil
}

func (double *admissionStoreDouble) FindActionAdmissionJudgedAt(
	_ context.Context, _ domain.TenantID, id string,
) (time.Time, bool, error) {
	if double.findErr != nil {
		return time.Time{}, false, double.findErr
	}
	judgedAt, ok := double.recorded[id]
	return judgedAt, ok, nil
}

type mutableClock struct{ at time.Time }

func (clock *mutableClock) Now() time.Time { return clock.at }

type admissionFixture struct {
	zones        *zoneViewDouble
	scopes       *objectScopeViewDouble
	restrictions *restrictionViewDouble
	conditions   *gateConditionViewDouble
	dutyRules    *dutyRuleViewDouble
	verification *currentDutyVerificationDouble
	releases     *releaseViewDouble
	store        *admissionStoreDouble
	clock        *mutableClock
}

func newAdmissionFixture() *admissionFixture {
	return &admissionFixture{
		zones: &zoneViewDouble{zones: map[string]string{
			"SYN-NODE-SHA-HUB": "SYN-ZONE-CN", "SYN-NODE-SZX-GATE": "SYN-ZONE-CN", "SYN-NODE-SIN-HUB": "SYN-ZONE-SG",
		}},
		scopes:       &objectScopeViewDouble{},
		restrictions: &restrictionViewDouble{},
		conditions:   &gateConditionViewDouble{configured: true},
		dutyRules:    &dutyRuleViewDouble{rule: domain.DutyPaymentNotAPrecondition(), found: true},
		verification: &currentDutyVerificationDouble{},
		releases:     &releaseViewDouble{},
		store:        &admissionStoreDouble{recorded: map[string]time.Time{}},
		clock:        &mutableClock{at: admissionAppAt.Add(time.Minute)},
	}
}

func (fixture *admissionFixture) handler(t *testing.T) *application.JudgeActionAdmissionHandler {
	t.Helper()
	handler, err := application.NewJudgeActionAdmissionHandler(application.JudgeActionAdmissionDeps{
		Zones:             fixture.zones,
		Scopes:            fixture.scopes,
		Restrictions:      fixture.restrictions,
		Conditions:        fixture.conditions,
		DutyRules:         fixture.dutyRules,
		DutyVerifications: fixture.verification,
		Releases:          fixture.releases,
		Store:             fixture.store,
		Clock:             fixture.clock,
	})
	if err != nil {
		t.Fatalf("构造准入编排：%v", err)
	}
	return handler
}

func (fixture *admissionFixture) inUnit(t *testing.T) {
	fixture.scopes.scope.Units = []domain.UnitScope{{
		Scope:     mustValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-CN-EXPORT-01"),
		Procedure: mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-CN-EXPORT"),
	}}
}

func admissionCommand(t *testing.T, from, to string) application.JudgeActionAdmissionCommand {
	t.Helper()
	intent, err := domain.NewDepartureIntent(
		mustValue(t, domain.NewNetworkNodeReference, from), mustValue(t, domain.NewNetworkNodeReference, to))
	if err != nil {
		t.Fatalf("构造出发意图：%v", err)
	}
	return application.JudgeActionAdmissionCommand{
		TenantID: mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Object:   mustValue(t, domain.NewDeclaredParcelReference, "SYN-PCL-01"),
		Intent:   intent,
		At:       admissionAppAt,
	}
}

func judged(t *testing.T, result application.JudgeActionAdmissionResult) (domain.ActionAdmissionJudgement, string) {
	t.Helper()
	judgement, id, ok := result.Judgement()
	if !ok {
		t.Fatalf("判断没形成：%s / %s", result.Outcome(), result.UnreadableReason())
	}
	return judgement, id
}

// 同一请求在同样的三源上重放：命中已记录的那份，标识与首次判断时刻都不变。
func TestAdmissionJudgementIsRecordedOnceAndReplaysToTheSameRecord(t *testing.T) {
	fixture := newAdmissionFixture()
	handler := fixture.handler(t)
	command := admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")

	first, err := handler.Handle(t.Context(), command)
	if err != nil || first.Outcome() != application.ActionAdmissionJudged {
		t.Fatalf("首判 = %s，err = %v", first.Outcome(), err)
	}
	judgement, id := judged(t, first)
	if judgement.Answer() != domain.AdmissionNotGuarded || len(judgement.Findings()) != 1 ||
		judgement.Findings()[0].Action != domain.LoadingDeparture {
		t.Fatalf("同区出发、对象不在册：%s %+v", judgement.Answer(), judgement.Findings())
	}
	// 对象自身也是限制范围：直接落在包裹上的限制不必经申报单元。
	if len(fixture.restrictions.asked) != 1 || fixture.restrictions.asked[0] != "SYN-PCL-01" {
		t.Fatalf("限制该按对象自身问：%v", fixture.restrictions.asked)
	}

	fixture.clock.at = fixture.clock.at.Add(time.Hour)
	replay, err := handler.Handle(t.Context(), command)
	if err != nil || replay.Outcome() != application.ActionAdmissionExisting {
		t.Fatalf("重放 = %s，err = %v", replay.Outcome(), err)
	}
	replayed, replayID := judged(t, replay)
	if replayID != id || !replayed.JudgedAt().Equal(judgement.JudgedAt()) {
		t.Fatalf("重放要回到同一份：%s@%s vs %s@%s", replayID, replayed.JudgedAt(), id, judgement.JudgedAt())
	}
	if len(fixture.store.recorded) != 1 {
		t.Fatalf("重放不该再记一份：%d", len(fixture.store.recorded))
	}
}

// 三源一变，判断换标识、记新的一份——旧的那份不被覆盖。
func TestAdmissionJudgementTakesANewIdentityWhenASourceChanges(t *testing.T) {
	fixture := newAdmissionFixture()
	fixture.inUnit(t)
	handler := fixture.handler(t)
	command := admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")

	before, err := handler.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("首判：%v", err)
	}
	beforeJudgement, beforeID := judged(t, before)
	if beforeJudgement.Answer() != domain.AdmissionNotGuarded {
		t.Fatalf("空清单门禁 = %s", beforeJudgement.Answer())
	}

	fixture.conditions.findings = []domain.PreconditionFinding{finding(t, "SYN-PRE-EXPORT-DECL-RELEASED", domain.PreconditionMet)}
	after, err := handler.Handle(t.Context(), command)
	if err != nil || after.Outcome() != application.ActionAdmissionJudged {
		t.Fatalf("门禁变了再判 = %s，err = %v", after.Outcome(), err)
	}
	afterJudgement, afterID := judged(t, after)
	if afterID == beforeID || afterJudgement.Answer() != domain.AdmissionBlocked {
		t.Fatalf("门禁满足却没放行，要换标识答阻断：%s %s", afterID, afterJudgement.Answer())
	}
	if len(fixture.store.recorded) != 2 {
		t.Fatalf("两份判断都该在：%d", len(fixture.store.recorded))
	}
}

// 门禁编排的停点译成领域读数：业务停点进判断，依赖读不动不进判断。
func TestGateStopsBecomeReadingsAndDependencyFailuresStopTheJudgement(t *testing.T) {
	fixture := newAdmissionFixture()
	fixture.inUnit(t)
	fixture.conditions.configured = false
	result, err := fixture.handler(t).Handle(t.Context(), admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"))
	if err != nil {
		t.Fatalf("判：%v", err)
	}
	judgement, _ := judged(t, result)
	if judgement.Answer() != domain.AdmissionNotConfigured ||
		judgement.Findings()[0].Bases[0].Cause != domain.CauseGateCatalogNotRegistered {
		t.Fatalf("目录未登记 = %s %+v", judgement.Answer(), judgement.Findings())
	}

	fixture = newAdmissionFixture()
	fixture.inUnit(t)
	fixture.dutyRules.rule = acceptFullNoDeltaValid(t)
	result, err = fixture.handler(t).Handle(t.Context(), admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"))
	if err != nil {
		t.Fatalf("判：%v", err)
	}
	judgement, _ = judged(t, result)
	if judgement.Answer() != domain.AdmissionUndecided ||
		judgement.Findings()[0].Bases[0].Cause != domain.CauseDutyVerificationAbsent {
		t.Fatalf("没有付款核对 = %s %+v", judgement.Answer(), judgement.Findings())
	}
}

func TestEachUnreadableSourceStopsBeforeTheFold(t *testing.T) {
	failure := errors.New("dependency down")
	tests := map[string]struct {
		fail func(*testing.T, *admissionFixture)
		want application.ActionAdmissionUnreadableReason
	}{
		"区域":   {func(_ *testing.T, f *admissionFixture) { f.zones.err = failure }, application.NodeZoneViewUnavailable},
		"对象范围": {func(_ *testing.T, f *admissionFixture) { f.scopes.err = failure }, application.ObjectScopeViewUnavailable},
		"限制":   {func(_ *testing.T, f *admissionFixture) { f.restrictions.err = failure }, application.RestrictionViewUnavailable},
		"门禁目录": {func(_ *testing.T, f *admissionFixture) { f.conditions.err = failure }, application.AdmissionGateConditionsUnavailable},
		"付款规则": {func(_ *testing.T, f *admissionFixture) { f.dutyRules.err = failure }, application.AdmissionDutyPaymentRuleUnavailable},
		"付款核对": {func(t *testing.T, f *admissionFixture) {
			f.dutyRules.rule = acceptFullNoDeltaValid(t)
			f.verification.err = failure
		}, application.AdmissionDutyVerificationsUnavailable},
		"放行":   {func(_ *testing.T, f *admissionFixture) { f.releases.err = failure }, application.ReleaseOutcomeViewUnavailable},
		"判断册写": {func(_ *testing.T, f *admissionFixture) { f.store.saveErr = failure }, application.ActionAdmissionStoreUnavailable},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newAdmissionFixture()
			fixture.inUnit(t)
			test.fail(t, fixture)
			result, err := fixture.handler(t).Handle(t.Context(), admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"))
			if err != nil {
				t.Fatalf("依赖读不动不是编程错误：%v", err)
			}
			if result.Outcome() != application.ActionAdmissionUnreadable || result.UnreadableReason() != test.want {
				t.Fatalf("= %s / %s，要读不动 / %s", result.Outcome(), result.UnreadableReason(), test.want)
			}
			if _, _, ok := result.Judgement(); ok {
				t.Fatal("读不动不该交出判断")
			}
		})
	}
}

func TestAdmissionRefusesARequestMissingItsCoordinates(t *testing.T) {
	handler := newAdmissionFixture().handler(t)
	complete := admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")
	for name, command := range map[string]application.JudgeActionAdmissionCommand{
		"缺租户":   {Object: complete.Object, Intent: complete.Intent, At: complete.At},
		"缺对象":   {TenantID: complete.TenantID, Intent: complete.Intent, At: complete.At},
		"缺意图":   {TenantID: complete.TenantID, Object: complete.Object, At: complete.At},
		"缺动作时点": {TenantID: complete.TenantID, Object: complete.Object, Intent: complete.Intent},
	} {
		result, err := handler.Handle(t.Context(), command)
		if err != nil || result.Outcome() != application.ActionAdmissionNotAccepted {
			t.Fatalf("%s：%s，err = %v", name, result.Outcome(), err)
		}
	}
}

func TestTheAdmissionHandlerNamesWhichDependencyIsMissing(t *testing.T) {
	if _, err := application.NewJudgeActionAdmissionHandler(application.JudgeActionAdmissionDeps{}); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("缺依赖：err = %v", err)
	}
}
