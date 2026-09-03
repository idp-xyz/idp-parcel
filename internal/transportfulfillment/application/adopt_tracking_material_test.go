package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉外部轨迹收编执行器（label-channel/16）。判据取自 ADR-0102 与 CONTEXT「外部承运轨迹
// 事实」生命周期节：源未给发生时间 → 留痕不成事实；有效时间无规则 → 待判断且不提供给 VE；
// 同（源，源事件）再到 → 重复投递；源声明更正 → 新版本回指当前版；凭证不认识 → 留痕。

var (
	trackingOccurredAt  = time.Date(2026, 9, 3, 8, 30, 0, 0, time.UTC)
	trackingReceivedAt  = time.Date(2026, 9, 3, 8, 31, 12, 0, time.UTC)
	trackingRecordedAt  = time.Date(2026, 9, 3, 8, 31, 13, 0, time.UTC)
	trackingRegistryOff = errors.New("外部承运轨迹事实登记册不可用")
)

type trackingFactRegistryDouble struct {
	records []ports.ExternalTrackingFactRecord
	findErr error
	saveErr error
}

func (double *trackingFactRegistryDouble) FindByKey(
	_ context.Context,
	key ports.ExternalTrackingFactKey,
) (ports.ExternalTrackingFactRecord, bool, error) {
	if double.findErr != nil {
		return ports.ExternalTrackingFactRecord{}, false, double.findErr
	}
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return ports.ExternalTrackingFactRecord{}, false, nil
}

func (double *trackingFactRegistryDouble) FindBySourceEvent(
	_ context.Context,
	tenant domain.TenantID,
	source domain.TrackingSourceReference,
	event domain.SourceEventReference,
) (ports.ExternalTrackingFactRecord, bool, error) {
	if double.findErr != nil {
		return ports.ExternalTrackingFactRecord{}, false, double.findErr
	}
	for _, record := range double.records {
		given, has := record.Fact.SourceEvent()
		if has && record.Key.TenantID == tenant && record.Fact.Source() == source && given == event {
			return record, true, nil
		}
	}
	return ports.ExternalTrackingFactRecord{}, false, nil
}

func (double *trackingFactRegistryDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	fact domain.ExternalTrackingFactReference,
) (ports.ExternalTrackingFactRecord, bool, error) {
	if double.findErr != nil {
		return ports.ExternalTrackingFactRecord{}, false, double.findErr
	}
	superseded := map[domain.ExternalTrackingFactVersion]bool{}
	for _, record := range double.records {
		if prior, has := record.Fact.Supersedes(); has {
			superseded[prior] = true
		}
	}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Fact == fact && !superseded[record.Key.Version] {
			return record, true, nil
		}
	}
	return ports.ExternalTrackingFactRecord{}, false, nil
}

func (double *trackingFactRegistryDouble) Save(
	_ context.Context,
	record ports.ExternalTrackingFactRecord,
) (ports.ExternalTrackingFactSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.ExternalTrackingFactSaveOutcomeInvalid, double.saveErr
	}
	for _, existing := range double.records {
		if existing.Key == record.Key {
			return ports.ExternalTrackingFactAlreadyRegistered, nil
		}
	}
	double.records = append(double.records, record)
	return ports.ExternalTrackingFactSaved, nil
}

type trackingIdentityDouble struct{ facts, versions int }

func (double *trackingIdentityDouble) NextExternalTrackingFactReference(context.Context) (domain.ExternalTrackingFactReference, error) {
	double.facts++
	return domain.NewExternalTrackingFactReference(fmt.Sprintf("EXTF-%012d", double.facts))
}

func (double *trackingIdentityDouble) NextExternalTrackingFactVersion(context.Context) (domain.ExternalTrackingFactVersion, error) {
	double.versions++
	return domain.NewExternalTrackingFactVersion(fmt.Sprintf("EXTV-%012d", double.versions))
}

type credentialResolverDouble struct {
	objects    map[string]string
	resolution ports.CredentialResolution
	err        error
}

func (double credentialResolverDouble) ResolveCredential(
	_ context.Context,
	_ domain.TenantID,
	credential domain.ExternalCarrierCredentialReference,
) (domain.CarriedObjectReference, ports.CredentialResolution, error) {
	if double.err != nil {
		return domain.CarriedObjectReference{}, ports.CredentialResolutionInvalid, double.err
	}
	if double.resolution != ports.CredentialResolutionInvalid {
		return domain.CarriedObjectReference{}, double.resolution, nil
	}
	object, known := double.objects[credential.String()]
	if !known {
		return domain.CarriedObjectReference{}, ports.CredentialUnknown, nil
	}
	reference, err := domain.NewCarriedObjectReference(object)
	return reference, ports.CredentialResolved, err
}

type effectiveTimeRulesDouble struct {
	ruling ports.EffectiveTimeRuling
	err    error
	inputs []ports.EffectiveTimeRuleInput
}

func (double *effectiveTimeRulesDouble) JudgeEffectiveTime(
	_ context.Context,
	input ports.EffectiveTimeRuleInput,
) (ports.EffectiveTimeRuling, error) {
	double.inputs = append(double.inputs, input)
	if double.err != nil {
		return ports.EffectiveTimeRuling{}, double.err
	}
	return double.ruling, nil
}

type unadoptedLedgerDouble struct {
	entries []ports.UnadoptedTrackingMaterial
	err     error
}

func (double *unadoptedLedgerDouble) RecordUnadopted(_ context.Context, entry ports.UnadoptedTrackingMaterial) error {
	if double.err != nil {
		return double.err
	}
	double.entries = append(double.entries, entry)
	return nil
}

type trackingHandoffDouble struct {
	intents []ports.ExternalTrackingFactHandoffIntent
	err     error
}

func (double *trackingHandoffDouble) HandOffExternalTrackingFact(_ context.Context, intent ports.ExternalTrackingFactHandoffIntent) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type trackingClock struct{ at time.Time }

func (clock trackingClock) Now() time.Time { return clock.at }

type adoptionFixture struct {
	registry *trackingFactRegistryDouble
	identity *trackingIdentityDouble
	rules    *effectiveTimeRulesDouble
	ledger   *unadoptedLedgerDouble
	handoff  *trackingHandoffDouble
	handler  *application.AdoptTrackingMaterialHandler
}

func newAdoptionFixture(resolver ports.ExternalCarrierCredentialResolver) *adoptionFixture {
	fixture := &adoptionFixture{
		registry: &trackingFactRegistryDouble{},
		identity: &trackingIdentityDouble{},
		rules:    &effectiveTimeRulesDouble{ruling: ports.EffectiveTimeRuling{Outcome: ports.EffectiveTimeRuleAbsent}},
		ledger:   &unadoptedLedgerDouble{},
		handoff:  &trackingHandoffDouble{},
	}
	fixture.handler = application.NewAdoptTrackingMaterialHandler(application.AdoptTrackingMaterialDeps{
		Facts:       fixture.registry,
		Identities:  fixture.identity,
		Credentials: resolver,
		Rules:       fixture.rules,
		Ledger:      fixture.ledger,
		Downstream:  fixture.handoff,
		Clock:       trackingClock{at: trackingRecordedAt},
	})
	return fixture
}

func knownCredentials() credentialResolverDouble {
	return credentialResolverDouble{objects: map[string]string{"carrier-x/1Z999": "PCL-1"}}
}

func ruleApplied(t *testing.T, at time.Time) ports.EffectiveTimeRuling {
	t.Helper()
	rule, err := domain.NewEffectiveTimeRuleReference("aggregator-a/effective-time", "v3")
	if err != nil {
		t.Fatalf("规则引用：%v", err)
	}
	return ports.EffectiveTimeRuling{Outcome: ports.EffectiveTimeRuleApplied, Rule: rule, EffectiveAt: at}
}

func trackingMaterial(t *testing.T, event string) ports.TrackingMaterial {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return ports.TrackingMaterial{
		Source:          "aggregator-a",
		Subject:         ports.TrackingSubject{Tenant: tenant, CredentialReference: "carrier-x/1Z999"},
		SourceEventID:   event,
		OccurredAt:      ports.SourceTime{Given: true, At: trackingOccurredAt},
		ReceivedAt:      trackingReceivedAt,
		StatusReference: "IN_TRANSIT",
		PayloadDigest:   "sha256:abc",
	}
}

func TestMaterialWithoutARuleIsAdoptedPendingAndNotHandedToVisibility(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())

	result, err := fixture.handler.Adopt(t.Context(), trackingMaterial(t, "evt-1"))
	if err != nil {
		t.Fatalf("收编：%v", err)
	}
	if result.Outcome() != application.TrackingFactAdoptedPendingJudgment {
		t.Fatalf("无规则应认领为待判断：%s", result.Outcome())
	}
	record, has := result.Record()
	if !has {
		t.Fatal("认领结果应带记录")
	}
	if _, judged := record.Fact.EffectiveAt(); judged {
		t.Fatal("没有规则却判出了有效时间——那就是默认等于发生时间那一格")
	}
	if !record.Fact.OccurredAt().Equal(trackingOccurredAt) || !record.Fact.ReceivedAt().Equal(trackingReceivedAt) {
		t.Fatalf("发生/接收时间没有各归各位：%s / %s", record.Fact.OccurredAt(), record.Fact.ReceivedAt())
	}
	if record.Fact.Object().String() != "PCL-1" {
		t.Fatalf("凭证应解析到载运对象：%q", record.Fact.Object())
	}
	if record.Fact.Status().String() != "IN_TRANSIT" {
		t.Fatalf("原始状态词应原样保存不解释：%q", record.Fact.Status())
	}
	if len(fixture.handoff.intents) != 0 {
		t.Fatal("待判断的事实不得提供给 visibility-exception")
	}
	if len(fixture.ledger.entries) != 0 {
		t.Fatal("成了事实的素材不该同时留痕")
	}
}

func TestMaterialJudgedByARegisteredRuleIsAdoptedAndHandedToVisibility(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	fixture.rules.ruling = ruleApplied(t, trackingReceivedAt)

	result, err := fixture.handler.Adopt(t.Context(), trackingMaterial(t, "evt-1"))
	if err != nil {
		t.Fatalf("收编：%v", err)
	}
	if result.Outcome() != application.TrackingFactAdopted {
		t.Fatalf("有规则应认领并判断：%s", result.Outcome())
	}
	record, _ := result.Record()
	at, judged := record.Fact.EffectiveAt()
	if !judged || !at.Equal(trackingReceivedAt) {
		t.Fatalf("有效时间应按规则形成：%s judged=%v", at, judged)
	}
	rule, byRule := record.Fact.Effective().Rule()
	if !byRule || rule.Version() != "v3" {
		t.Fatalf("事实上应记下采用的规则版本：%q byRule=%v", rule.Version(), byRule)
	}
	if len(fixture.handoff.intents) != 1 || fixture.handoff.intents[0].Record.Key != record.Key {
		t.Fatalf("判断过的事实应交给 visibility-exception 一次：%d", len(fixture.handoff.intents))
	}
	if len(fixture.rules.inputs) != 1 || fixture.rules.inputs[0].Status.String() != "IN_TRANSIT" {
		t.Fatalf("规则应看到素材自己的状态词：%+v", fixture.rules.inputs)
	}
}

func TestMaterialWhoseSourceGaveNoOccurrenceTimeIsLeftUnadopted(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	fixture.rules.ruling = ruleApplied(t, trackingReceivedAt)
	material := trackingMaterial(t, "evt-1")
	material.OccurredAt = ports.SourceTime{}

	result, err := fixture.handler.Adopt(t.Context(), material)
	if err != nil {
		t.Fatalf("收编：%v", err)
	}
	if result.Outcome() != application.TrackingMaterialLeftUnadopted {
		t.Fatalf("源未给发生时间应留痕不收编：%s", result.Outcome())
	}
	if result.UnadoptedReason() != ports.UnadoptedOccurredAtNotGiven {
		t.Fatalf("留痕理由应为源未给发生时间：%s", result.UnadoptedReason())
	}
	if len(fixture.registry.records) != 0 {
		t.Fatal("没有发生时间的素材进了事实库——接收时间顶替了发生时间")
	}
	if len(fixture.ledger.entries) != 1 {
		t.Fatalf("应留痕一条：%d", len(fixture.ledger.entries))
	}
	entry := fixture.ledger.entries[0]
	if entry.Reason != ports.UnadoptedOccurredAtNotGiven || entry.SourceEvent != "evt-1" || entry.PayloadDigest != "sha256:abc" {
		t.Fatalf("留痕应原样记源给了什么：%+v", entry)
	}
	if !entry.ReceivedAt.Equal(trackingReceivedAt) || !entry.RecordedAt.Equal(trackingRecordedAt) {
		t.Fatalf("留痕应带接收时间与落痕时间：%s / %s", entry.ReceivedAt, entry.RecordedAt)
	}
	if len(fixture.handoff.intents) != 0 {
		t.Fatal("留痕的素材不得交给 visibility-exception")
	}
}

func TestTheSameSourceEventArrivingAgainIsADuplicateDelivery(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	fixture.rules.ruling = ruleApplied(t, trackingReceivedAt)
	first, _ := fixture.handler.Adopt(t.Context(), trackingMaterial(t, "evt-1"))
	firstRecord, _ := first.Record()

	again := trackingMaterial(t, "evt-1")
	again.ReceivedAt = trackingReceivedAt.Add(time.Hour)
	second, err := fixture.handler.Adopt(t.Context(), again)
	if err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if second.Outcome() != application.TrackingMaterialDuplicate {
		t.Fatalf("同（源，源事件）再到应答重复投递：%s", second.Outcome())
	}
	secondRecord, _ := second.Record()
	if secondRecord.Key != firstRecord.Key {
		t.Fatal("重复投递应交回原结果，不形成第二条事实")
	}
	if len(fixture.registry.records) != 1 {
		t.Fatalf("事实库里应只有一条：%d", len(fixture.registry.records))
	}
}

func TestMaterialWithoutASourceEventIsNeverDeduplicated(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	if _, err := fixture.handler.Adopt(t.Context(), trackingMaterial(t, "")); err != nil {
		t.Fatalf("第一条：%v", err)
	}
	result, err := fixture.handler.Adopt(t.Context(), trackingMaterial(t, ""))
	if err != nil {
		t.Fatalf("第二条：%v", err)
	}
	if result.Outcome() != application.TrackingFactAdoptedPendingJudgment {
		t.Fatalf("源未给事件标识的素材不按内容判重，两条各成一条事实：%s", result.Outcome())
	}
	if len(fixture.registry.records) != 2 {
		t.Fatalf("应有两条事实：%d", len(fixture.registry.records))
	}
	if _, given := fixture.registry.records[0].Fact.SourceEvent(); given {
		t.Fatal("源未给事件标识，事实上却铸了一个——那是代铸")
	}
}

func TestASourceDeclaredCorrectionFormsANewVersionOfTheCorrectedFact(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	fixture.rules.ruling = ruleApplied(t, trackingReceivedAt)
	original, _ := fixture.handler.Adopt(t.Context(), trackingMaterial(t, "evt-1"))
	originalRecord, _ := original.Record()

	correction := trackingMaterial(t, "evt-2")
	correction.CorrectionOf = "evt-1"
	correction.StatusReference = "DELIVERED"
	correction.OccurredAt = ports.SourceTime{Given: true, At: trackingOccurredAt.Add(2 * time.Hour)}
	result, err := fixture.handler.Adopt(t.Context(), correction)
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	if result.Outcome() != application.TrackingFactAdopted {
		t.Fatalf("更正应认领：%s", result.Outcome())
	}
	corrected, _ := result.Record()
	if corrected.Key.Fact != originalRecord.Key.Fact {
		t.Fatal("更正应是同一条事实的新版本，不是另一条事实")
	}
	if corrected.Key.Version == originalRecord.Key.Version {
		t.Fatal("更正必须换版本")
	}
	prior, has := corrected.Fact.Supersedes()
	if !has || prior != originalRecord.Key.Version {
		t.Fatalf("新版本应回指被更正的当前版：%q has=%v", prior, has)
	}
	declared, _ := corrected.Fact.CorrectionOf()
	if declared.String() != "evt-1" {
		t.Fatalf("源的更正声明应原样登记：%q", declared)
	}
	if event, _ := corrected.Fact.SourceEvent(); event.String() != "evt-2" {
		t.Fatalf("新版本携带更正素材自己的源事件标识：%q", event)
	}
	if corrected.Fact.Status().String() != "DELIVERED" {
		t.Fatalf("更正后的状态词应原样保存、不解释：%q", corrected.Fact.Status())
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("两个判断过的版本各交一次：%d", len(fixture.handoff.intents))
	}
}

func TestACorrectionOfAnUnknownSourceEventIsAdoptedWithTheDeclarationKept(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	correction := trackingMaterial(t, "evt-2")
	correction.CorrectionOf = "evt-never-seen"

	result, err := fixture.handler.Adopt(t.Context(), correction)
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	record, _ := result.Record()
	if _, linked := record.Fact.Supersedes(); linked {
		t.Fatal("被更正的事件本上下文不认识，不得回指任何版本")
	}
	if declared, has := record.Fact.CorrectionOf(); !has || declared.String() != "evt-never-seen" {
		t.Fatalf("源声明仍应原样登记：%q has=%v", declared, has)
	}
}

func TestAnUnknownCredentialLeavesTheMaterialUnadopted(t *testing.T) {
	fixture := newAdoptionFixture(credentialResolverDouble{objects: map[string]string{}})

	result, err := fixture.handler.Adopt(t.Context(), trackingMaterial(t, "evt-1"))
	if err != nil {
		t.Fatalf("收编：%v", err)
	}
	if result.Outcome() != application.TrackingMaterialLeftUnadopted || result.UnadoptedReason() != ports.UnadoptedCredentialUnknown {
		t.Fatalf("凭证不认识应留痕：%s / %s", result.Outcome(), result.UnadoptedReason())
	}
	if len(fixture.ledger.entries) != 1 || fixture.ledger.entries[0].Credential != "carrier-x/1Z999" {
		t.Fatalf("留痕应带凭证引用：%+v", fixture.ledger.entries)
	}
}

func TestAnUnconfiguredCredentialRegistryIsUndecidedNotUnadopted(t *testing.T) {
	fixture := newAdoptionFixture(credentialResolverDouble{resolution: ports.CredentialRegistryUnconfigured})

	result, err := fixture.handler.Adopt(t.Context(), trackingMaterial(t, "evt-1"))
	if err != nil {
		t.Fatalf("收编：%v", err)
	}
	if result.Outcome() != application.TrackingAdoptionUndecided || result.UndecidedReason() != application.TrackingCredentialRegistryUnconfigured {
		t.Fatalf("凭证登记册未配置是本上下文的缺口，应未决而不是留痕：%s / %s", result.Outcome(), result.UndecidedReason())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决应带续办引用")
	}
	if len(fixture.ledger.entries) != 0 {
		t.Fatal("本上下文的缺口不该记成素材的缺陷")
	}
}

func TestARegistryOutageIsUndecided(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	fixture.registry.findErr = trackingRegistryOff

	result, err := fixture.handler.Adopt(t.Context(), trackingMaterial(t, "evt-1"))
	if err != nil {
		t.Fatalf("收编：%v", err)
	}
	if result.Outcome() != application.TrackingAdoptionUndecided || result.UndecidedReason() != application.TrackingFactRegistryUnavailable {
		t.Fatalf("登记册不可用应未决：%s / %s", result.Outcome(), result.UndecidedReason())
	}
}

func TestAHandoffFailureKeepsTheFactAndLeavesAContinuation(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	fixture.rules.ruling = ruleApplied(t, trackingReceivedAt)
	fixture.handoff.err = errors.New("outbox 不可用")

	result, err := fixture.handler.Adopt(t.Context(), trackingMaterial(t, "evt-1"))
	if err != nil {
		t.Fatalf("收编：%v", err)
	}
	if result.Outcome() != application.TrackingFactAdopted {
		t.Fatalf("投递失败不翻结果：%s", result.Outcome())
	}
	if result.HandoffReference() == "" {
		t.Fatal("意图没交出去应留续办引用")
	}
}

func TestMalformedMaterialIsNotAccepted(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	blankStatus := trackingMaterial(t, "evt-1")
	blankStatus.StatusReference = "  "
	result, err := fixture.handler.Adopt(t.Context(), blankStatus)
	if err != nil {
		t.Fatalf("收编：%v", err)
	}
	if result.Outcome() != application.TrackingMaterialNotAccepted {
		t.Fatalf("没有状态词的素材不成形，应未受理：%s", result.Outcome())
	}
	if len(fixture.ledger.entries) != 0 {
		t.Fatal("未受理不是留痕——它是调用方的输入错误")
	}
}

func TestEveryAdoptionOutcomeAndReasonHasAName(t *testing.T) {
	for _, outcome := range []application.TrackingAdoptionOutcome{
		application.TrackingFactAdopted, application.TrackingFactAdoptedPendingJudgment,
		application.TrackingMaterialDuplicate, application.TrackingMaterialLeftUnadopted,
		application.TrackingMaterialNotAccepted, application.TrackingAdoptionUndecided,
	} {
		if outcome.String() == "" {
			t.Fatalf("结果 %d 没有名字", outcome)
		}
	}
	for _, reason := range []application.TrackingAdoptionUndecidedReason{
		application.TrackingFactRegistryUnavailable, application.TrackingIdentityUnavailable,
		application.TrackingCredentialResolverUnavailable, application.TrackingCredentialRegistryUnconfigured,
		application.TrackingRulesUnavailable, application.TrackingLedgerUnavailable,
	} {
		if reason.String() == "" {
			t.Fatalf("理由 %d 没有名字", reason)
		}
	}
}
