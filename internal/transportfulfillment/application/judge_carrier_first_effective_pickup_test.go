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

// 本文件证「显式判断实际承运商首次有效收寄」编排（lc/31 做法 3；ADR-0135 决定四 / 五 / 六 / 七 / 八）的结果代数
// 与段那一半：依据读回与不可用、首次判据、待确认两支与身份登记后凭同一依据形成、进段与实际承运商判断首版、
// 更正的替代 / 失效与参与重派生、段引用缺席照登照发。夹具全为合成引用（S 级）。

var (
	carrierPickupEffectiveAt = time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	carrierPickupNow         = time.Date(2026, 9, 10, 9, 30, 0, 0, time.UTC)
)

// carrierPickupRegistryDouble 是 ports.CarrierFirstEffectivePickupRegistry 的替身：按版本行存、读时走重建门。
type carrierPickupRegistryDouble struct {
	rows    []domain.RehydrateCarrierFirstEffectivePickupSpec
	findErr error
	saveErr error
}

func (double *carrierPickupRegistryDouble) rehydrate(row domain.RehydrateCarrierFirstEffectivePickupSpec) (ports.CarrierFirstEffectivePickupRecord, error) {
	pickup, err := domain.RehydrateCarrierFirstEffectivePickup(row)
	if err != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, err
	}
	return ports.CarrierFirstEffectivePickupRecord{
		Key:        ports.CarrierFirstEffectivePickupKey{TenantID: row.TenantID, Fact: row.Fact, Version: row.Version},
		Pickup:     pickup,
		RecordedAt: row.JudgedAt,
	}, nil
}

func (double *carrierPickupRegistryDouble) superseded(row domain.RehydrateCarrierFirstEffectivePickupSpec) bool {
	for _, other := range double.rows {
		if other.Fact == row.Fact && other.Supersedes == row.Version {
			return true
		}
	}
	return false
}

func (double *carrierPickupRegistryDouble) FindByKey(_ context.Context, key ports.CarrierFirstEffectivePickupKey) (ports.CarrierFirstEffectivePickupRecord, bool, error) {
	if double.findErr != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, false, double.findErr
	}
	for _, row := range double.rows {
		if row.TenantID == key.TenantID && row.Fact == key.Fact && row.Version == key.Version {
			record, err := double.rehydrate(row)
			return record, err == nil, err
		}
	}
	return ports.CarrierFirstEffectivePickupRecord{}, false, nil
}

func (double *carrierPickupRegistryDouble) FindCurrentByObject(_ context.Context, tenant domain.TenantID, object domain.CarriedObjectReference) (ports.CarrierFirstEffectivePickupRecord, bool, error) {
	if double.findErr != nil {
		return ports.CarrierFirstEffectivePickupRecord{}, false, double.findErr
	}
	for _, row := range double.rows {
		if row.TenantID == tenant && row.Object == object && !double.superseded(row) {
			record, err := double.rehydrate(row)
			return record, err == nil, err
		}
	}
	return ports.CarrierFirstEffectivePickupRecord{}, false, nil
}

func (double *carrierPickupRegistryDouble) ListByObject(_ context.Context, tenant domain.TenantID, object domain.CarriedObjectReference) ([]ports.CarrierFirstEffectivePickupRecord, error) {
	var records []ports.CarrierFirstEffectivePickupRecord
	for _, row := range double.rows {
		if row.TenantID == tenant && row.Object == object {
			record, err := double.rehydrate(row)
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		}
	}
	return records, nil
}

func (double *carrierPickupRegistryDouble) Save(_ context.Context, record ports.CarrierFirstEffectivePickupRecord) (ports.CarrierPickupSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.CarrierPickupSaveOutcomeInvalid, double.saveErr
	}
	pickup := record.Pickup
	row := domain.RehydrateCarrierFirstEffectivePickupSpec{
		TenantID: pickup.TenantID(), Object: pickup.Object(), Fact: pickup.Fact(), Version: pickup.Version(),
		Result: pickup.Result(), JudgedAt: pickup.JudgedAt(), Bases: pickup.Bases(), Material: pickup.Material(),
	}
	if carrier, identified := pickup.Carrier(); identified {
		row.Carrier = carrier
	}
	if at, present := pickup.OccurredAt(); present {
		row.OccurredAt = at
	}
	if reason, pending := pickup.PendingReason(); pending {
		row.Reason = reason
	}
	if prior, has := pickup.Supersedes(); has {
		row.Supersedes = prior
	}
	for _, existing := range double.rows {
		if existing.Fact == row.Fact && existing.Version == row.Version {
			return ports.CarrierPickupAlreadyRegistered, nil
		}
		// 一对象一链：第二条首登（不回指）撞既有首登。
		if row.Supersedes.String() == "" && existing.Supersedes.String() == "" && existing.TenantID == row.TenantID && existing.Object == row.Object {
			return ports.CarrierPickupAlreadyRegistered, nil
		}
		if row.Supersedes.String() != "" && existing.Fact == row.Fact && existing.Supersedes == row.Supersedes {
			return ports.CarrierPickupAlreadyRegistered, nil
		}
	}
	double.rows = append(double.rows, row)
	return ports.CarrierPickupSaved, nil
}

type carrierPickupIdentityDouble struct{ facts, versions int }

func (double *carrierPickupIdentityDouble) NextCarrierFirstEffectivePickupReference(context.Context) (domain.CarrierFirstEffectivePickupReference, error) {
	double.facts++
	return domain.NewCarrierFirstEffectivePickupReference(fmt.Sprintf("CFEP-%d", double.facts))
}

func (double *carrierPickupIdentityDouble) NextCarrierFirstEffectivePickupVersion(context.Context) (domain.CarrierFirstEffectivePickupVersion, error) {
	double.versions++
	return domain.NewCarrierFirstEffectivePickupVersion(fmt.Sprintf("CFEV-%d", double.versions))
}

type carrierPickupHandoffDouble struct {
	intents []ports.CarrierFirstEffectivePickupHandoffIntent
	err     error
}

func (double *carrierPickupHandoffDouble) HandOffCarrierFirstEffectivePickup(_ context.Context, intent ports.CarrierFirstEffectivePickupHandoffIntent) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type carrierPickupFixture struct {
	pickups   *carrierPickupRegistryDouble
	facts     *trackingFactRegistryDouble
	segments  *segmentRegistryDouble
	judgments *judgmentRegistryDouble
	directory *identityDirectoryDouble
	handoff   *carrierPickupHandoffDouble
	clock     *movingClock
	handler   *application.JudgeCarrierFirstEffectivePickupHandler
}

func newCarrierPickupFixture(t *testing.T, registered ...domain.CarrierSubject) *carrierPickupFixture {
	t.Helper()
	fixture := &carrierPickupFixture{
		pickups:   &carrierPickupRegistryDouble{},
		facts:     &trackingFactRegistryDouble{},
		segments:  newSegmentRegistry(),
		judgments: newJudgmentRegistry(),
		directory: newIdentityDirectory(registered...),
		handoff:   &carrierPickupHandoffDouble{},
		clock:     &movingClock{at: carrierPickupNow},
	}
	carrierJudgments := application.NewFormActualCarrierJudgmentHandler(application.FormActualCarrierJudgmentDeps{
		Segments: fixture.segments, Judgments: fixture.judgments, Identities: fixture.directory, Clock: fixture.clock,
	})
	fixture.handler = application.NewJudgeCarrierFirstEffectivePickupHandler(application.JudgeCarrierFirstEffectivePickupDeps{
		Pickups:          fixture.pickups,
		Identities:       &carrierPickupIdentityDouble{},
		Downstream:       fixture.handoff,
		Evidence:         fixture.facts,
		Directory:        fixture.directory,
		Segments:         fixture.segments,
		Judgments:        fixture.judgments,
		CarrierJudgments: carrierJudgments,
		Clock:            fixture.clock,
	})
	return fixture
}

// addTrackingFact 往轨迹事实登记册里放一代：有效时间按 effective 给（零值即待判断），supersedes 非空即回指前版。
func (fixture *carrierPickupFixture) addTrackingFact(t *testing.T, fact, version, object string, effective time.Time, supersedes string) {
	t.Helper()
	spec := domain.ExternalTrackingFactSpec{
		TenantID:   mustTenant(t, "tenant-1"),
		Fact:       mustRefValue(t, domain.NewExternalTrackingFactReference, fact),
		Version:    mustRefValue(t, domain.NewExternalTrackingFactVersion, version),
		Source:     mustRefValue(t, domain.NewTrackingSourceReference, "carrier-x-direct"),
		Credential: mustRefValue(t, domain.NewExternalCarrierCredentialReference, "carrier-x/1Z999"),
		Object:     mustRefValue(t, domain.NewCarriedObjectReference, object),
		Status:     mustRefValue(t, domain.NewRawStatusReference, "PICKED_UP"),
		OccurredAt: carrierPickupEffectiveAt.Add(-5 * time.Minute),
		ReceivedAt: carrierPickupEffectiveAt.Add(time.Minute),
		Effective:  domain.PendingEffectiveTime(),
	}
	if !effective.IsZero() {
		judged, err := domain.JudgeEffectiveTimeExplicitly(effective)
		if err != nil {
			t.Fatalf("有效时间判断：%v", err)
		}
		spec.Effective = judged
	}
	if supersedes != "" {
		spec.CorrectionOf = mustRefValue(t, domain.NewSourceEventReference, "evt-prior")
		spec.Supersedes = mustRefValue(t, domain.NewExternalTrackingFactVersion, supersedes)
	}
	adopted, err := domain.AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("认领轨迹事实夹具：%v", err)
	}
	fixture.facts.records = append(fixture.facts.records, ports.ExternalTrackingFactRecord{
		Key:  ports.ExternalTrackingFactKey{TenantID: spec.TenantID, Fact: spec.Fact, Version: spec.Version},
		Fact: adopted, RecordedAt: spec.ReceivedAt,
	})
}

func mustTenant(t *testing.T, raw string) domain.TenantID {
	t.Helper()
	return mustRefValue(t, domain.NewTenantID, raw)
}

func mustRefValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("%q：%v", raw, err)
	}
	return value
}

func trackingPickupCommand(t *testing.T) application.JudgeCarrierFirstEffectivePickupCommand {
	t.Helper()
	return application.JudgeCarrierFirstEffectivePickupCommand{
		TenantID:          mustTenant(t, "tenant-1"),
		Object:            "PCL-1",
		Source:            domain.TrustedChannelCallback,
		EvidenceReference: "EXTF-1",
		EvidenceVersion:   "EXTV-1",
		ExpressesControl:  true,
		SubjectKind:       domain.ExternalCarrierParty,
		SubjectReference:  "party/carrier-x",
		Segment:           "SEG-X",
	}
}

func (fixture *carrierPickupFixture) judge(t *testing.T, command application.JudgeCarrierFirstEffectivePickupCommand) application.JudgeCarrierFirstEffectivePickupResult {
	t.Helper()
	result, err := fixture.handler.Judge(t.Context(), command)
	if err != nil {
		t.Fatalf("judge：%v", err)
	}
	return result
}

// Covers: ADR-0135 决定四已形成 + 决定五进段与判断首版 + 决定七意图——一条有效时间已判断的轨迹事实、承运主体在册、
// 对象无当前参与 → 已形成；业务时间取有效时间；对象凭第三格进段；实际承运商判断当前版为已识别、依据即收寄依据；
// 意图交出一份带（租户，事实，版本，对象）。
func TestAJudgedTrackingFactNamingARegisteredCarrierFormsTheFirstPickupAndEntersTheSegment(t *testing.T) {
	fixture := newCarrierPickupFixture(t, carrierX(t))
	fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")

	result := fixture.judge(t, trackingPickupCommand(t))
	if result.Outcome() != application.CarrierPickupFormedOutcome {
		t.Fatalf("outcome = %s，期望 PICKUP_FORMED", result.Outcome())
	}
	record, present := result.Record()
	if !present || !record.Pickup.Formed() {
		t.Fatalf("应带回已形成的记录")
	}
	if at, _ := record.Pickup.OccurredAt(); !at.Equal(carrierPickupEffectiveAt) {
		t.Fatalf("业务时间应取依据已判断的有效时间：%s", at)
	}
	if result.HandoffReference() != "" || len(fixture.handoff.intents) != 1 || fixture.handoff.intents[0].Record.Key != record.Key {
		t.Fatalf("意图应交出恰一份且指向新版本：%q %d", result.HandoffReference(), len(fixture.handoff.intents))
	}
	if result.SegmentContinuationReference() != "" || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("段那一半不应欠账或被拒：%q %s", result.SegmentContinuationReference(), result.SegmentEntryRefusal())
	}
	segment, found, err := fixture.segments.FindByKey(t.Context(), ports.FulfillmentSegmentKey{TenantID: mustTenant(t, "tenant-1"), Segment: mustRefValue(t, domain.NewFulfillmentSegmentReference, "SEG-X")})
	if err != nil || !found {
		t.Fatalf("段没有立起来：%v %v", err, found)
	}
	participation, in := segment.Segment.ParticipationFor(mustRefValue(t, domain.NewCarriedObjectReference, "PCL-1"))
	if !in || participation.EntryKind() != domain.EnteredByCarrierFirstEffectivePickup || participation.EntryBasis().String() != "CARRIER-FIRST-EFFECTIVE-PICKUP/"+record.Key.Version.String() {
		t.Fatalf("参与起点不是第三格 / 依据不对：%+v", participation)
	}
	if result.CarrierJudgmentContinuationReference() != "" {
		t.Fatalf("判断那一半不应欠账：%q", result.CarrierJudgmentContinuationReference())
	}
	judgment, found, err := fixture.judgments.FindByKey(t.Context(), judgmentKeyFixture(t, "tenant-1", "SEG-X"))
	if err != nil || !found {
		t.Fatalf("判断读不回：%v %v", err, found)
	}
	subject, identified := judgment.Judgment.Current().Verdict().Identified()
	if !identified || subject != carrierX(t) {
		t.Fatalf("判断当前版应为已识别 carrier-x：%v %+v", identified, subject)
	}
	if bases := judgment.Judgment.Current().Bases(); len(bases) != 1 || bases[0].Reference().String() != "EXTF-1" {
		t.Fatalf("判断依据应即收寄依据：%+v", bases)
	}
}

// Covers: 同一依据重放答`已在册`不追加；段引用缺席时事实照登、意图照发、不进段。
func TestAReplayAnswersAlreadyRecordedAndAMissingSegmentReferenceStillRegistersAndHandsOff(t *testing.T) {
	fixture := newCarrierPickupFixture(t, carrierX(t))
	fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
	command := trackingPickupCommand(t)
	command.Segment = ""

	first := fixture.judge(t, command)
	if first.Outcome() != application.CarrierPickupFormedOutcome || len(fixture.handoff.intents) != 1 {
		t.Fatalf("首判 = %s / 意图 %d", first.Outcome(), len(fixture.handoff.intents))
	}
	if keys, _ := fixture.segments.FindActiveSegments(t.Context(), mustTenant(t, "tenant-1"), mustRefValue(t, domain.NewCarriedObjectReference, "PCL-1")); len(keys) != 0 {
		t.Fatalf("段引用缺席不应进段")
	}
	replay := fixture.judge(t, command)
	if replay.Outcome() != application.CarrierPickupAlreadyRecorded || len(fixture.pickups.rows) != 1 || len(fixture.handoff.intents) != 1 {
		t.Fatalf("重放 = %s，行数 %d，意图 %d", replay.Outcome(), len(fixture.pickups.rows), len(fixture.handoff.intents))
	}
}

// Covers: ADR-0135 决定四「非首次」——对象已凭场外揽收进段时，新到的承运证据不构成首次有效收寄、不落版本、不发意图。
func TestEvidenceForAnObjectAlreadyUnderControlIsNotAFirstPickup(t *testing.T) {
	fixture := newCarrierPickupFixture(t, carrierX(t))
	establishment := newJudgmentOnEstablishmentFixture(t)
	pickupCommand := pickupRegistrationCommand(t)
	pickupCommand.Segment = "segment-1"
	if _, err := establishment.handler.Register(t.Context(), pickupCommand); err != nil {
		t.Fatalf("先经场外揽收进段：%v", err)
	}
	fixture.segments = establishment.segments
	fixture.handler = application.NewJudgeCarrierFirstEffectivePickupHandler(application.JudgeCarrierFirstEffectivePickupDeps{
		Pickups: fixture.pickups, Identities: &carrierPickupIdentityDouble{}, Downstream: fixture.handoff, Evidence: fixture.facts,
		Directory: fixture.directory, Segments: fixture.segments, Judgments: fixture.judgments, Clock: fixture.clock,
	})
	fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", pickupCommand.Object, carrierPickupEffectiveAt, "")
	command := trackingPickupCommand(t)
	command.Object = pickupCommand.Object

	result := fixture.judge(t, command)
	if result.Outcome() != application.CarrierPickupNotFirst || len(fixture.pickups.rows) != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatalf("outcome = %s，行数 %d，意图 %d", result.Outcome(), len(fixture.pickups.rows), len(fixture.handoff.intents))
	}
}

// bringUnderControl 让对象凭场外揽收进段，造出首次判据要看的「当前有效参与」——与
// TestEvidenceForAnObjectAlreadyUnderControlIsNotAFirstPickup 走同一条路，只是共用本夹具的段登记册，
// 好让「先待确认、后在控」这个先后顺序在同一份夹具上摆出来。
func (fixture *carrierPickupFixture) bringUnderControl(t *testing.T, object string) {
	t.Helper()
	handler := application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
		Pickups:    newPickupRegistry(),
		Segments:   fixture.segments,
		Judgments:  fixture.judgments,
		Versions:   &pickupRegVersionFactory{},
		Downstream: &pickupRegHandoffDouble{},
		Clock:      pickupRegClock{at: pickupRegisteredAt},
	})
	command := pickupRegistrationCommand(t)
	command.Object = object
	command.Segment = "segment-1"
	if _, err := handler.Register(t.Context(), command); err != nil {
		t.Fatalf("凭场外揽收进段：%v", err)
	}
}

// Covers: lc/33 判据 1 / ADR-0135 决定四「非首次……不形成版本」——链尾待确认期间对象已凭别的控制事实进段，
// 之后到达的每一条本会让链尾成为已形成的入口都得先问首次判据：在控就答`非首次`，不落版本、不发意图，
// 待确认链尾留原样（失效也是一版，且领域只让已形成失效）。
func TestAPendingChainDoesNotFormWhenTheObjectIsAlreadyUnderControl(t *testing.T) {
	tenant := mustTenant(t, "tenant-1")
	object := mustRefValue(t, domain.NewCarriedObjectReference, "PCL-1")

	holdPending := func(t *testing.T) *carrierPickupFixture {
		t.Helper()
		fixture := newCarrierPickupFixture(t)
		fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
		if pending := fixture.judge(t, trackingPickupCommand(t)); pending.Outcome() != application.CarrierPickupPendingOutcome {
			t.Fatalf("前置：outcome = %s，期望 PICKUP_PENDING", pending.Outcome())
		}
		fixture.bringUnderControl(t, "PCL-1")
		return fixture
	}
	assertNotFirstAndUntouched := func(t *testing.T, fixture *carrierPickupFixture, result application.JudgeCarrierFirstEffectivePickupResult) {
		t.Helper()
		if result.Outcome() != application.CarrierPickupNotFirst {
			t.Fatalf("outcome = %s，期望 NOT_FIRST", result.Outcome())
		}
		if len(fixture.pickups.rows) != 1 || len(fixture.handoff.intents) != 0 {
			t.Fatalf("不该落版本或发意图：行数 %d，意图 %d", len(fixture.pickups.rows), len(fixture.handoff.intents))
		}
		current, found, err := fixture.pickups.FindCurrentByObject(t.Context(), tenant, object)
		if err != nil || !found || current.Pickup.Result() != domain.CarrierPickupPending {
			t.Fatalf("待确认链尾应留原样：%v %v", err, found)
		}
	}

	t.Run("the same basis after the carrier is registered", func(t *testing.T) {
		fixture := holdPending(t)
		fixture.directory.registered[carrierX(t).Kind().String()+"|"+carrierX(t).Reference()] = true
		assertNotFirstAndUntouched(t, fixture, fixture.judge(t, trackingPickupCommand(t)))
	})

	t.Run("another basis naming a registered carrier", func(t *testing.T) {
		fixture := holdPending(t)
		fixture.addTrackingFact(t, "EXTF-2", "EXTV-1", "PCL-1", carrierPickupEffectiveAt.Add(time.Minute), "")
		carrierY := mustSubject(t, domain.ExternalCarrierParty, "party/carrier-y")
		fixture.directory.registered[carrierY.Kind().String()+"|"+carrierY.Reference()] = true
		other := trackingPickupCommand(t)
		other.EvidenceReference = "EXTF-2"
		other.SubjectReference = carrierY.Reference()
		assertNotFirstAndUntouched(t, fixture, fixture.judge(t, other))
	})

	t.Run("a correction of the pending basis that still reads as a pickup", func(t *testing.T) {
		fixture := holdPending(t)
		fixture.addTrackingFact(t, "EXTF-1", "EXTV-2", "PCL-1", carrierPickupEffectiveAt.Add(time.Minute), "EXTV-1")
		fixture.directory.registered[carrierX(t).Kind().String()+"|"+carrierX(t).Reference()] = true
		corrected := trackingPickupCommand(t)
		corrected.EvidenceVersion = "EXTV-2"
		assertNotFirstAndUntouched(t, fixture, fixture.judge(t, corrected))
	})
}

// Covers: ADR-0135 决定三——有效时间待判断的轨迹事实不得作依据 → 依据不可用；指名不存在的一代或别的对象 → 未受理。
func TestAPendingEffectiveTimeMakesTheBasisUnavailable(t *testing.T) {
	fixture := newCarrierPickupFixture(t, carrierX(t))
	fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", time.Time{}, "")
	if result := fixture.judge(t, trackingPickupCommand(t)); result.Outcome() != application.CarrierPickupBasisUnavailable {
		t.Fatalf("outcome = %s，期望 BASIS_UNAVAILABLE", result.Outcome())
	}
	missing := trackingPickupCommand(t)
	missing.EvidenceVersion = "EXTV-404"
	if result := fixture.judge(t, missing); result.Outcome() != application.CarrierPickupNotAccepted {
		t.Fatalf("不存在的一代 = %s，期望 INPUT_NOT_ACCEPTED", result.Outcome())
	}
	other := trackingPickupCommand(t)
	other.Object = "PCL-2"
	if result := fixture.judge(t, other); result.Outcome() != application.CarrierPickupNotAccepted {
		t.Fatalf("说的是别的对象 = %s，期望 INPUT_NOT_ACCEPTED", result.Outcome())
	}
	if len(fixture.pickups.rows) != 0 {
		t.Fatalf("三格都不该落版本")
	}
}

// Covers: CONTEXT 生命周期——承运主体身份未登记 → 待确认版本（不发意图、不进段）；登记后凭同一依据 → 已形成版本回指
// 待确认前版，此时才进段、才发意图。
func TestAnUnregisteredCarrierHoldsPendingUntilRegistrationThenFormsOnTheSameBasis(t *testing.T) {
	fixture := newCarrierPickupFixture(t)
	fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")

	pending := fixture.judge(t, trackingPickupCommand(t))
	if pending.Outcome() != application.CarrierPickupPendingOutcome {
		t.Fatalf("outcome = %s，期望 PICKUP_PENDING", pending.Outcome())
	}
	record, _ := pending.Record()
	if reason, _ := record.Pickup.PendingReason(); reason != domain.PickupCarrierIdentityNotRegistered || record.Pickup.Material() != "party/carrier-x" {
		t.Fatalf("待确认原因 / 素材不对：%s %q", reason, record.Pickup.Material())
	}
	if len(fixture.handoff.intents) != 0 {
		t.Fatalf("待确认不提供")
	}
	if keys, _ := fixture.segments.FindActiveSegments(t.Context(), mustTenant(t, "tenant-1"), mustRefValue(t, domain.NewCarriedObjectReference, "PCL-1")); len(keys) != 0 {
		t.Fatalf("待确认不进段")
	}

	fixture.directory.registered[carrierX(t).Kind().String()+"|"+carrierX(t).Reference()] = true
	fixture.clock.at = carrierPickupNow.Add(time.Hour)
	formed := fixture.judge(t, trackingPickupCommand(t))
	if formed.Outcome() != application.CarrierPickupFormedOutcome {
		t.Fatalf("登记后 = %s，期望 PICKUP_FORMED", formed.Outcome())
	}
	formedRecord, _ := formed.Record()
	if prior, has := formedRecord.Pickup.Supersedes(); !has || prior != record.Key.Version || formedRecord.Key.Fact != record.Key.Fact {
		t.Fatalf("已形成版本应回指待确认前版并沿用事实身份")
	}
	if len(fixture.handoff.intents) != 1 || len(fixture.pickups.rows) != 2 {
		t.Fatalf("意图 %d，行数 %d", len(fixture.handoff.intents), len(fixture.pickups.rows))
	}
	if keys, _ := fixture.segments.FindActiveSegments(t.Context(), mustTenant(t, "tenant-1"), mustRefValue(t, domain.NewCarriedObjectReference, "PCL-1")); len(keys) != 1 {
		t.Fatalf("已形成后应进段")
	}
}

// Covers: CONTEXT 生命周期——链尾待确认、另一来源指名不同主体 → 待确认（来源冲突），全部依据保留；同一主体的第二条证据仍是身份未登记。
func TestTwoUnregisteredBasesNamingDifferentCarriersConflict(t *testing.T) {
	fixture := newCarrierPickupFixture(t)
	fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
	fixture.addTrackingFact(t, "EXTF-2", "EXTV-1", "PCL-1", carrierPickupEffectiveAt.Add(time.Minute), "")
	fixture.addTrackingFact(t, "EXTF-3", "EXTV-1", "PCL-1", carrierPickupEffectiveAt.Add(2*time.Minute), "")
	fixture.judge(t, trackingPickupCommand(t))

	same := trackingPickupCommand(t)
	same.EvidenceReference = "EXTF-2"
	sameResult := fixture.judge(t, same)
	if sameResult.Outcome() != application.CarrierPickupPendingOutcome {
		t.Fatalf("同主体第二条 = %s", sameResult.Outcome())
	}
	if sameRecord, _ := sameResult.Record(); func() domain.PendingPickupReason { r, _ := sameRecord.Pickup.PendingReason(); return r }() != domain.PickupCarrierIdentityNotRegistered {
		t.Fatalf("同主体第二条仍应是身份未登记")
	}

	other := trackingPickupCommand(t)
	other.EvidenceReference = "EXTF-3"
	other.SubjectReference = "party/carrier-y"
	result := fixture.judge(t, other)
	record, _ := result.Record()
	if reason, _ := record.Pickup.PendingReason(); result.Outcome() != application.CarrierPickupPendingOutcome || reason != domain.PickupSourceConflict {
		t.Fatalf("不同主体 = %s / %s，期望待确认（来源冲突）", result.Outcome(), reason)
	}
	if len(record.Pickup.Bases()) != 3 {
		t.Fatalf("全部依据应保留：%d", len(record.Pickup.Bases()))
	}
}

// Covers: ADR-0135 决定四「不构成」——读法说证据不表达取得控制且不是更正：什么都不留。
func TestEvidenceReadAsNotExpressingControlLeavesNothing(t *testing.T) {
	fixture := newCarrierPickupFixture(t, carrierX(t))
	fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
	command := trackingPickupCommand(t)
	command.ExpressesControl = false
	if result := fixture.judge(t, command); result.Outcome() != application.CarrierPickupNotAPickup || len(fixture.pickups.rows) != 0 {
		t.Fatalf("outcome = %s，行数 %d", result.Outcome(), len(fixture.pickups.rows))
	}
}

// Covers: ADR-0135 决定六——依据被源更正：更正后仍表达收寄 → 替代版本（业务时间随新一代）+ 替代参与版本；
// 不再表达 → 失效版本 + 失效参与版本、对象在段内无有效参与；更正的不是链尾依据 → 依据不是当前依据。
func TestACorrectedBasisSupersedesOrVoidsThePickupAndRederivesTheParticipation(t *testing.T) {
	object := mustRefValue(t, domain.NewCarriedObjectReference, "PCL-1")
	segmentKey := ports.FulfillmentSegmentKey{TenantID: mustTenant(t, "tenant-1"), Segment: mustRefValue(t, domain.NewFulfillmentSegmentReference, "SEG-X")}

	t.Run("still a pickup after correction", func(t *testing.T) {
		fixture := newCarrierPickupFixture(t, carrierX(t))
		fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
		first := fixture.judge(t, trackingPickupCommand(t))
		firstRecord, _ := first.Record()
		later := carrierPickupEffectiveAt.Add(15 * time.Minute)
		fixture.addTrackingFact(t, "EXTF-1", "EXTV-2", "PCL-1", later, "EXTV-1")

		corrected := trackingPickupCommand(t)
		corrected.EvidenceVersion = "EXTV-2"
		result := fixture.judge(t, corrected)
		if result.Outcome() != application.CarrierPickupSupersededOutcome {
			t.Fatalf("outcome = %s，期望 PICKUP_SUPERSEDED", result.Outcome())
		}
		record, _ := result.Record()
		if prior, _ := record.Pickup.Supersedes(); prior != firstRecord.Key.Version {
			t.Fatalf("替代版本没有回指前版")
		}
		if at, _ := record.Pickup.OccurredAt(); !at.Equal(later) {
			t.Fatalf("业务时间应随新一代：%s", at)
		}
		if len(fixture.handoff.intents) != 2 {
			t.Fatalf("两代各一份意图：%d", len(fixture.handoff.intents))
		}
		segment, _, _ := fixture.segments.FindByKey(t.Context(), segmentKey)
		current, _ := segment.Segment.ParticipationFor(object)
		if current.EntryBasis().String() != "CARRIER-FIRST-EFFECTIVE-PICKUP/"+record.Key.Version.String() || !current.EnteredAt().Equal(later) || current.Voided() {
			t.Fatalf("参与应被替代版本接替：%+v", current)
		}
	})

	t.Run("no longer a pickup after correction", func(t *testing.T) {
		fixture := newCarrierPickupFixture(t, carrierX(t))
		fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
		fixture.judge(t, trackingPickupCommand(t))
		fixture.addTrackingFact(t, "EXTF-1", "EXTV-2", "PCL-1", carrierPickupEffectiveAt, "EXTV-1")

		corrected := trackingPickupCommand(t)
		corrected.EvidenceVersion = "EXTV-2"
		corrected.ExpressesControl = false
		result := fixture.judge(t, corrected)
		if result.Outcome() != application.CarrierPickupVoidedOutcome {
			t.Fatalf("outcome = %s，期望 PICKUP_VOIDED", result.Outcome())
		}
		record, _ := result.Record()
		if !record.Pickup.Voided() || len(fixture.handoff.intents) != 2 {
			t.Fatalf("失效版本应登记并提供：%v %d", record.Pickup.Voided(), len(fixture.handoff.intents))
		}
		segment, _, _ := fixture.segments.FindByKey(t.Context(), segmentKey)
		current, _ := segment.Segment.ParticipationFor(object)
		if !current.Voided() || segment.Segment.ActiveParticipations() != 0 {
			t.Fatalf("参与应为失效版本且段内无有效参与：%+v", current)
		}
		if _, found, _ := fixture.pickups.FindCurrentByObject(t.Context(), mustTenant(t, "tenant-1"), object); !found {
			t.Fatalf("链尾应仍在册（失效版本）")
		}
	})

	t.Run("a correction of a version that is not the current basis", func(t *testing.T) {
		fixture := newCarrierPickupFixture(t, carrierX(t))
		fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
		fixture.judge(t, trackingPickupCommand(t))
		fixture.addTrackingFact(t, "EXTF-1", "EXTV-2", "PCL-1", carrierPickupEffectiveAt, "EXTV-0")

		corrected := trackingPickupCommand(t)
		corrected.EvidenceVersion = "EXTV-2"
		if result := fixture.judge(t, corrected); result.Outcome() != application.CarrierPickupBasisNotCurrent || len(fixture.pickups.rows) != 1 {
			t.Fatalf("outcome = %s，行数 %d", result.Outcome(), len(fixture.pickups.rows))
		}
	})
}

// Covers: 已形成之后另一来源到达 → 非首次（不收回、不落版本），那是段级实际承运商判断的事（ADR-0135 决定六）。
func TestAnotherSourceAfterFormationIsNotFirstAndDoesNotRetract(t *testing.T) {
	fixture := newCarrierPickupFixture(t, carrierX(t))
	fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
	fixture.addTrackingFact(t, "EXTF-2", "EXTV-1", "PCL-1", carrierPickupEffectiveAt.Add(time.Hour), "")
	fixture.judge(t, trackingPickupCommand(t))
	other := trackingPickupCommand(t)
	other.EvidenceReference = "EXTF-2"
	other.SubjectReference = "party/carrier-y"
	if result := fixture.judge(t, other); result.Outcome() != application.CarrierPickupNotFirst || len(fixture.pickups.rows) != 1 {
		t.Fatalf("outcome = %s，行数 %d", result.Outcome(), len(fixture.pickups.rows))
	}
}

// Covers: 依赖缺席 / 读不通各答`未决`并带续办引用，不折成业务答案（ADR-0029）。
func TestUnavailableDependenciesAnswerUndecided(t *testing.T) {
	fixture := newCarrierPickupFixture(t, carrierX(t))
	fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
	fixture.pickups.findErr = errors.New("登记册不可用")
	result := fixture.judge(t, trackingPickupCommand(t))
	if result.Outcome() != application.CarrierPickupUndecided || result.UndecidedReason() != application.CarrierPickupRegistryUnavailable || result.ContinuationReference() == "" {
		t.Fatalf("outcome = %s / %s / %q", result.Outcome(), result.UndecidedReason(), result.ContinuationReference())
	}
	fixture.pickups.findErr = nil
	fixture.directory.err = errors.New("身份读口不可用")
	if result := fixture.judge(t, trackingPickupCommand(t)); result.Outcome() != application.CarrierPickupUndecided || result.UndecidedReason() != application.CarrierPickupIdentityDirectoryUnavailable {
		t.Fatalf("身份读口不可用 = %s / %s", result.Outcome(), result.UndecidedReason())
	}
}
