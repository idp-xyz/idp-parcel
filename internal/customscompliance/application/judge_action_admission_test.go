package application_test

import (
	"context"
	"errors"
	"fmt"
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
	// tenantErr 只让「租户登没登过区域」那一问读不动；anyAsked 数它被问了几次。
	tenantErr error
	anyAsked  int
}

// AnyNodeCustomsZoneInForce 按替身里登了几个节点作答：登了任一节点即「已登过区域」。
func (double *zoneViewDouble) AnyNodeCustomsZoneInForce(context.Context, domain.TenantID, time.Time) (bool, error) {
	double.anyAsked++
	if double.err != nil {
		return false, double.err
	}
	if double.tenantErr != nil {
		return false, double.tenantErr
	}
	return len(double.zones) > 0, nil
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

type internalRestrictionViewDouble struct {
	asked []string
	err   error
}

func (double *internalRestrictionViewDouble) ListInternalByScope(
	_ context.Context, _ domain.TenantID, scope domain.DecisionScopeReference,
) ([]domain.InternalRestriction, error) {
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
	internal     *internalRestrictionViewDouble
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
		internal:     &internalRestrictionViewDouble{},
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
		Zones:                fixture.zones,
		Scopes:               fixture.scopes,
		Restrictions:         fixture.restrictions,
		InternalRestrictions: fixture.internal,
		Conditions:           fixture.conditions,
		DutyRules:            fixture.dutyRules,
		DutyVerifications:    fixture.verification,
		Releases:             fixture.releases,
		Store:                fixture.store,
		Clock:                fixture.clock,
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
// Covers: AT-CC-423 的编排半边（替身册；判断册一行、Outbox 不增在真库用例）。
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
// Covers: AT-CC-420 的编排半边——目录未登记的停点译成未配置读数。
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

// Covers: AT-CC-424（编排半边：替身依赖逐口读不动）。
func TestEachUnreadableSourceStopsBeforeTheFold(t *testing.T) {
	failure := errors.New("dependency down")
	tests := map[string]struct {
		fail func(*testing.T, *admissionFixture)
		want application.ActionAdmissionUnreadableReason
	}{
		"区域":     {func(_ *testing.T, f *admissionFixture) { f.zones.err = failure }, application.NodeZoneViewUnavailable},
		"对象范围":   {func(_ *testing.T, f *admissionFixture) { f.scopes.err = failure }, application.ObjectScopeViewUnavailable},
		"限制":     {func(_ *testing.T, f *admissionFixture) { f.restrictions.err = failure }, application.RestrictionViewUnavailable},
		"内部合规限制": {func(_ *testing.T, f *admissionFixture) { f.internal.err = failure }, application.InternalRestrictionViewUnavailable},
		"门禁目录":   {func(_ *testing.T, f *admissionFixture) { f.conditions.err = failure }, application.AdmissionGateConditionsUnavailable},
		"付款规则":   {func(_ *testing.T, f *admissionFixture) { f.dutyRules.err = failure }, application.AdmissionDutyPaymentRuleUnavailable},
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

// 起讫两端都没登区域时编排另问一句租户登没登过区域，答案交领域折（ADR-0300）：登过的租户，不在册对象的跨区那一道答
// 未配置并指名未登的节点；一条都没登的租户这一道不列。一端已登时不问——领域从读数认得租户登过；判得出跨区时也不问。
// 问不动停在区域那一口的读不动，不拿「没登」顶替。
// Covers: AT-CC-426、AT-CC-427 的编排半边。
func TestTheTenantsZoneRegisterIsAskedOnlyWhenCrossZoneIsUndetermined(t *testing.T) {
	crossFinding := func(t *testing.T, judgement domain.ActionAdmissionJudgement) domain.ActionAdmissionFinding {
		t.Helper()
		for _, finding := range judgement.Findings() {
			if finding.Action == domain.CrossCustomsMovement {
				return finding
			}
		}
		t.Fatalf("判断里没有跨区那一道：%+v", judgement.Findings())
		return domain.ActionAdmissionFinding{}
	}
	handle := func(t *testing.T, fixture *admissionFixture, from, to string) application.JudgeActionAdmissionResult {
		t.Helper()
		result, err := fixture.handler(t).Handle(t.Context(), admissionCommand(t, from, to))
		if err != nil {
			t.Fatalf("判：%v", err)
		}
		return result
	}

	oneEnd := newAdmissionFixture()
	judgement, _ := judged(t, handle(t, oneEnd, "SYN-NODE-SZX-GATE", "SYN-NODE-KUL-HUB"))
	if judgement.Answer() != domain.AdmissionNotConfigured || oneEnd.zones.anyAsked != 0 {
		t.Fatalf("起点已登、终点未登 = %s %+v，问了 %d 次，要未配置、不问", judgement.Answer(), judgement.Findings(), oneEnd.zones.anyAsked)
	}
	if finding := crossFinding(t, judgement); finding.Bases[0].Cause != domain.CauseNodeZoneNotRegistered ||
		len(finding.Bases[0].Sources) != 1 || finding.Bases[0].Sources[0] != "SYN-NODE-KUL-HUB" {
		t.Fatalf("未配置要指名未登的节点：%+v", finding)
	}

	bothEnds := newAdmissionFixture()
	judgement, _ = judged(t, handle(t, bothEnds, "SYN-NODE-KUL-HUB", "SYN-NODE-PEN-HUB"))
	if judgement.Answer() != domain.AdmissionNotConfigured || bothEnds.zones.anyAsked != 1 {
		t.Fatalf("登过区域的租户、两端都未登 = %s %+v，问了 %d 次，要未配置、问一次", judgement.Answer(), judgement.Findings(), bothEnds.zones.anyAsked)
	}
	if sources := crossFinding(t, judgement).Bases[0].Sources; len(sources) != 2 ||
		sources[0] != "SYN-NODE-KUL-HUB" || sources[1] != "SYN-NODE-PEN-HUB" {
		t.Fatalf("两端都未登要两个都指名：%v", sources)
	}

	domestic := newAdmissionFixture()
	domestic.zones.zones = map[string]string{}
	if judgement, _ := judged(t, handle(t, domestic, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")); judgement.Answer() != domain.AdmissionNotGuarded ||
		len(judgement.Findings()) != 1 || domestic.zones.anyAsked != 1 {
		t.Fatalf("一条区域都没登的租户 = %s %+v，问了 %d 次", judgement.Answer(), judgement.Findings(), domestic.zones.anyAsked)
	}

	determined := newAdmissionFixture()
	handle(t, determined, "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	if determined.zones.anyAsked != 0 {
		t.Fatalf("判得出跨区时不该问租户登没登过区域：问了 %d 次", determined.zones.anyAsked)
	}

	failing := newAdmissionFixture()
	failing.zones.tenantErr = errors.New("dependency down")
	result := handle(t, failing, "SYN-NODE-KUL-HUB", "SYN-NODE-PEN-HUB")
	if result.Outcome() != application.ActionAdmissionUnreadable || result.UnreadableReason() != application.NodeZoneViewUnavailable {
		t.Fatalf("问不动 = %s / %s，要读不动 / %s", result.Outcome(), result.UnreadableReason(), application.NodeZoneViewUnavailable)
	}
	if result := handle(t, failing, "SYN-NODE-SZX-GATE", "SYN-NODE-KUL-HUB"); result.Outcome() != application.ActionAdmissionJudged {
		t.Fatalf("一端已登时不问，那一口坏着也照常形成判断：%s / %s", result.Outcome(), result.UnreadableReason())
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

// 判断册拒收这次写入本身（词表外、缺事务）是编程或接线错，原样上抛；只有库读写不动才答读不动——前者重投不会好。
// Covers: AT-CC-424「判断册写口拒收这次写入本身不答读不动」那一句。
func TestAStoreRejectionIsAProgrammingErrorNotAnUnreadableStore(t *testing.T) {
	fixture := newAdmissionFixture()
	fixture.store.saveErr = fmt.Errorf("wrapped: %w", ports.ErrActionAdmissionJudgementRejected)
	result, err := fixture.handler(t).Handle(t.Context(), admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"))
	if !errors.Is(err, ports.ErrActionAdmissionJudgementRejected) {
		t.Fatalf("拒收要原样上抛：%s / %v", result.Outcome(), err)
	}
}

// 认定清单折不出结论：目录读口交回了领域构造造不出的认定，归那一口读不动，不是 Go error、也不折进五格。
// Covers: AT-CC-424「门禁目录交回折不出结论的认定」那一支。
func TestFindingsThatDoNotFoldNameTheGateConditionView(t *testing.T) {
	fixture := newAdmissionFixture()
	fixture.inUnit(t)
	fixture.conditions.findings = []domain.PreconditionFinding{{
		Precondition: mustValue(t, domain.NewPreconditionReference, "SYN-PRE-EXPORT-DECL-RELEASED"),
		State:        domain.PreconditionStateInvalid,
	}}
	result, err := fixture.handler(t).Handle(t.Context(), admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE"))
	if err != nil {
		t.Fatalf("读口交回坏行不是编程错误：%v", err)
	}
	if result.Outcome() != application.ActionAdmissionUnreadable ||
		result.UnreadableReason() != application.AdmissionGateConditionsUnavailable {
		t.Fatalf("= %s / %s，要读不动 / %s", result.Outcome(), result.UnreadableReason(), application.AdmissionGateConditionsUnavailable)
	}
}

// 动作时点按微秒入标识：库只存到微秒，消费方读回时点重问不带亚微秒位，仍要命中首次那一份。
// Covers: AT-CC-423「执行方读回微秒精度的动作时点再问」那一支（编排半边）。
func TestAdmissionAtIsTruncatedToTheMicrosecondTheStoreKeeps(t *testing.T) {
	fixture := newAdmissionFixture()
	handler := fixture.handler(t)
	precise := admissionCommand(t, "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")
	precise.At = admissionAppAt.Add(1234567 * time.Nanosecond)

	first, err := handler.Handle(t.Context(), precise)
	if err != nil || first.Outcome() != application.ActionAdmissionJudged {
		t.Fatalf("首判 = %s，err = %v", first.Outcome(), err)
	}
	judgement, id := judged(t, first)
	if want := admissionAppAt.Add(1234 * time.Microsecond); !judgement.At().Equal(want) {
		t.Fatalf("判断记下的时点 = %s，要截到微秒 %s", judgement.At(), want)
	}

	readBack := precise
	readBack.At = admissionAppAt.Add(1234 * time.Microsecond)
	replay, err := handler.Handle(t.Context(), readBack)
	if err != nil || replay.Outcome() != application.ActionAdmissionExisting {
		t.Fatalf("读回时点重问 = %s，err = %v", replay.Outcome(), err)
	}
	if _, replayID := judged(t, replay); replayID != id {
		t.Fatalf("同一次询问派生了两个标识：%s vs %s", replayID, id)
	}
}
