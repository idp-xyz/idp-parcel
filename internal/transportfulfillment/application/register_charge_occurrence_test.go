package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证运输收费发生项登记用例的公共一半与实际履约那一种原因（票 demo-chain-gaps/27，ADR-0207）：事实取自段
// 内已结束的参与，身份由事实派生，采购上下文经协议核对口取法人与快照，缺了答未受理而不答不适用；自营只认显式
// 声明与实际承运商判断；协议四个否定格各成一格；重登同内容答已存在、异内容答冲突；落册与交意图在同一次调用里。
// 夹具全是合成串，证据只记 S。

var (
	occurrenceEnteredAt = time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC)
	occurrenceEndedAt   = time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)
	occurrenceNow       = time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
)

const (
	occurrenceTenant   = "SYN-TENANT-01"
	trunkSegment       = "SYN-SEG-SZX-SIN-01"
	trunkProvider      = "SYN-PARTY-CARRIER-TRUNK-01"
	trunkAgreement     = "SYN-SUPPLIER-TRUNK-CN-SG-01"
	trunkAgreementTag  = "v1"
	trunkSnapshot      = "SYN-SUPPLIER-TRUNK-CN-SG-01/v1"
	trunkLegalEntity   = "SYN-LE-01"
	trunkParcel        = "SYN-PARCEL-01"
	trunkSecondParcel  = "SYN-PARCEL-02"
	conditionAssertion = "干线协议 v1：段到段运输一次计一段，已在 SIN-HUB 交接"
	registrant         = "https://dex.syn.example#SYN-OPERATOR-CONFIG-01"
)

func occurrenceValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

type occurrenceRegistryDouble struct {
	records     map[ports.ChargeOccurrenceKey]ports.ChargeOccurrenceRecord
	saveErr     error
	findErr     error
	coverageErr error
	saves       int
}

func newOccurrenceRegistry() *occurrenceRegistryDouble {
	return &occurrenceRegistryDouble{records: map[ports.ChargeOccurrenceKey]ports.ChargeOccurrenceRecord{}}
}

func (double *occurrenceRegistryDouble) FindByKey(
	_ context.Context, key ports.ChargeOccurrenceKey,
) (ports.ChargeOccurrenceRecord, bool, error) {
	if double.findErr != nil {
		return ports.ChargeOccurrenceRecord{}, false, double.findErr
	}
	record, found := double.records[key]
	return record, found, nil
}

// FindCovering 照真册的读法：同租户、同原因、同事实依据、不是同一身份、成员有交集。替身里没有修订版，当前版即首版。
func (double *occurrenceRegistryDouble) FindCovering(
	_ context.Context, query ports.ChargeOccurrenceCoverageQuery,
) (ports.ChargeOccurrenceRecord, bool, error) {
	if double.coverageErr != nil {
		return ports.ChargeOccurrenceRecord{}, false, double.coverageErr
	}
	asked := map[domain.CarriedObjectReference]bool{}
	for _, member := range query.Members {
		asked[member] = true
	}
	for key, record := range double.records {
		occurrence := record.Occurrence
		if key.TenantID != query.TenantID || key.Occurrence == query.Except ||
			occurrence.Reason() != query.Reason || occurrence.FactBasis() != query.FactBasis {
			continue
		}
		for _, member := range occurrence.Members() {
			if asked[member] {
				return record, true, nil
			}
		}
	}
	return ports.ChargeOccurrenceRecord{}, false, nil
}

func (double *occurrenceRegistryDouble) Save(
	_ context.Context, record ports.ChargeOccurrenceRecord,
) (ports.ChargeOccurrenceSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.ChargeOccurrenceSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.records[record.Key]; exists {
		return ports.ChargeOccurrenceAlreadyRegistered, nil
	}
	double.saves++
	double.records[record.Key] = record
	return ports.ChargeOccurrenceSaved, nil
}

type occurrenceHandoffDouble struct {
	intents []ports.ChargeOccurrenceIntent
	err     error
}

func (double *occurrenceHandoffDouble) HandOffChargeOccurrence(_ context.Context, intent ports.ChargeOccurrenceIntent) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type segmentViewDouble struct {
	record ports.FulfillmentSegmentRecord
	found  bool
	err    error
}

func (double segmentViewDouble) FindByKey(_ context.Context, _ ports.FulfillmentSegmentKey) (ports.FulfillmentSegmentRecord, bool, error) {
	return double.record, double.found, double.err
}

type judgmentViewDouble struct {
	record ports.ActualCarrierJudgmentRecord
	found  bool
	err    error
}

func (double judgmentViewDouble) FindByKey(_ context.Context, _ ports.ActualCarrierJudgmentKey) (ports.ActualCarrierJudgmentRecord, bool, error) {
	return double.record, double.found, double.err
}

type chargeAgreementSourceDouble struct {
	check    ports.SupplierAgreementCheck
	err      error
	askedAt  []time.Time
	provider []string
}

func (double *chargeAgreementSourceDouble) CheckSupplierAgreement(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SupplierAgreementVersion,
	provider domain.ServiceProviderReference,
	at time.Time,
) (ports.SupplierAgreementCheck, error) {
	double.askedAt = append(double.askedAt, at)
	double.provider = append(double.provider, provider.String())
	return double.check, double.err
}

func chargeRegisteredAgreement(t *testing.T) *chargeAgreementSourceDouble {
	t.Helper()
	return &chargeAgreementSourceDouble{check: ports.SupplierAgreementCheck{
		Standing:    ports.SupplierAgreementRegistered,
		Snapshot:    occurrenceValue(t, domain.NewAgreementSnapshotReference, trunkSnapshot),
		LegalEntity: occurrenceValue(t, domain.NewProcurementLegalEntityReference, trunkLegalEntity),
		Supplier:    occurrenceValue(t, domain.NewServiceProviderReference, trunkProvider),
	}}
}

type occurrenceClock struct{}

func (occurrenceClock) Now() time.Time { return occurrenceNow }

// endedParticipation 是一条凭交接入场、以下一次交接离场的参与；ended 为假时仍在场。
func participationRow(t *testing.T, object string, ended bool) domain.RehydrateParticipationSpec {
	t.Helper()
	row := domain.RehydrateParticipationSpec{
		Object:     occurrenceValue(t, domain.NewCarriedObjectReference, object),
		EntryKind:  domain.EnteredByTransportHandover,
		EntryBasis: occurrenceValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SZX-"+object),
		EnteredAt:  occurrenceEnteredAt,
	}
	if ended {
		row.EndKind = domain.EndedByNextHandover
		row.EndBasis = occurrenceValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SIN-"+object)
		row.EndedAt = occurrenceEndedAt
	}
	return row
}

func trunkSegmentWith(t *testing.T, rows ...domain.RehydrateParticipationSpec) segmentViewDouble {
	t.Helper()
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Segment:        occurrenceValue(t, domain.NewFulfillmentSegmentReference, trunkSegment),
		Participations: rows,
	})
	if err != nil {
		t.Fatalf("段夹具：%v", err)
	}
	return segmentViewDouble{
		record: ports.FulfillmentSegmentRecord{
			Key:     ports.FulfillmentSegmentKey{TenantID: segment.TenantID(), Segment: segment.Segment()},
			Segment: segment,
		},
		found: true,
	}
}

func judgedCarrier(t *testing.T, kind domain.CarrierSubjectKind, reference string) judgmentViewDouble {
	t.Helper()
	subject, err := domain.NewCarrierSubject(kind, reference)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := domain.NewCarrierEvidence(domain.CarrierEvidenceSpec{
		Source:     domain.HandedOverToCarrier,
		Reference:  occurrenceValue(t, domain.NewCarrierEvidenceReference, "SYN-HO-SZX-"+trunkParcel),
		OccurredAt: occurrenceEnteredAt,
		Subject:    subject,
	})
	if err != nil {
		t.Fatal(err)
	}
	judgment, err := domain.RehydrateActualCarrierJudgment(domain.RehydrateActualCarrierJudgmentSpec{
		TenantID:      occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Segment:       occurrenceValue(t, domain.NewFulfillmentSegmentReference, trunkSegment),
		EstablishedAt: occurrenceEnteredAt,
		Versions: []domain.RehydrateActualCarrierJudgmentVersionSpec{
			{Sequence: 1, Pending: domain.NoQualifiedCarrierEvidence, BusinessTime: occurrenceEnteredAt, FormedAt: occurrenceEnteredAt},
			{Sequence: 2, Subject: subject, BusinessTime: occurrenceEnteredAt, FormedAt: occurrenceEnteredAt, Bases: []domain.CarrierEvidence{evidence}},
		},
	})
	if err != nil {
		t.Fatalf("判断夹具：%v", err)
	}
	return judgmentViewDouble{record: ports.ActualCarrierJudgmentRecord{Judgment: judgment}, found: true}
}

type occurrenceRig struct {
	registry   *occurrenceRegistryDouble
	handoff    *occurrenceHandoffDouble
	agreements *chargeAgreementSourceDouble
	segments   segmentViewDouble
	judgments  judgmentViewDouble
	pickups    ports.FailedAttemptSource
	deliveries ports.DeliveryAttemptView
}

func newOccurrenceRig(t *testing.T) *occurrenceRig {
	t.Helper()
	return &occurrenceRig{
		registry:   newOccurrenceRegistry(),
		handoff:    &occurrenceHandoffDouble{},
		agreements: chargeRegisteredAgreement(t),
		segments:   trunkSegmentWith(t, participationRow(t, trunkParcel, true)),
	}
}

func (rig *occurrenceRig) handler() *application.RegisterChargeOccurrenceHandler {
	return application.NewRegisterChargeOccurrenceHandler(application.RegisterChargeOccurrenceDeps{
		PickupAttempts:   rig.pickups,
		DeliveryAttempts: rig.deliveries,
		Segments:         rig.segments,
		CarrierJudgments: rig.judgments,
		Agreements:       rig.agreements,
		Occurrences:      rig.registry,
		Handoff:          rig.handoff,
		Clock:            occurrenceClock{},
	})
}

func (rig *occurrenceRig) register(t *testing.T, command application.RegisterChargeOccurrenceCommand) application.RegisterChargeOccurrenceResult {
	t.Helper()
	result, err := rig.handler().Register(t.Context(), command)
	if err != nil {
		t.Fatalf("登记不该上抛：%v", err)
	}
	return result
}

func actualFulfillmentCommand(t *testing.T, objects ...string) application.RegisterChargeOccurrenceCommand {
	t.Helper()
	if len(objects) == 0 {
		objects = []string{trunkParcel}
	}
	return application.RegisterChargeOccurrenceCommand{
		TenantID:           occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Registrant:         registrant,
		Reason:             domain.ActualFulfillmentOccurrence,
		Segment:            trunkSegment,
		Objects:            objects,
		Journey:            "SYN-JRN-" + trunkParcel,
		Provider:           trunkProvider,
		AgreementObject:    trunkAgreement,
		AgreementVersion:   trunkAgreementTag,
		Quantity:           1,
		Unit:               "SEGMENT",
		ConditionAssertion: conditionAssertion,
	}
}

func assertNothingWritten(t *testing.T, rig *occurrenceRig) {
	t.Helper()
	if rig.registry.saves != 0 || len(rig.handoff.intents) != 0 {
		t.Fatalf("不该落册也不该交意图：落 %d 条，交 %d 份", rig.registry.saves, len(rig.handoff.intents))
	}
}

// Covers: AT-TF-120——干线段参与以下一次交接结束，登记方给出服务方、协议版本与断言：依据与范围取段、业务时间取
// 参与终点、法人与协议快照取自协议核对口（快照是「对象/版本」，ADR-0208 决定六），协议按业务时间核；身份由事实
// 派生、首版 V1；同一次调用交出一份意图，意图里就是落册的那一份。
func TestAnEndedTrunkParticipationRegistersAnActualFulfillmentOccurrence(t *testing.T) {
	rig := newOccurrenceRig(t)
	result := rig.register(t, actualFulfillmentCommand(t))
	if result.Outcome() != application.ChargeOccurrenceRegistered {
		t.Fatalf("outcome = %q detail = %q, want REGISTERED", result.Outcome(), result.Detail())
	}
	record, present := result.Record()
	if !present {
		t.Fatal("已登记却没有回显发生项")
	}
	occurrence := record.Occurrence
	if occurrence.Reason() != domain.ActualFulfillmentOccurrence ||
		occurrence.FactBasis().String() != "FULFILLMENT-SEGMENT/"+trunkSegment ||
		occurrence.Scope().String() != trunkSegment {
		t.Fatalf("原因 / 依据 / 范围 = %q / %q / %q", occurrence.Reason(), occurrence.FactBasis(), occurrence.Scope())
	}
	if !occurrence.OccurredAt().Equal(occurrenceEndedAt) {
		t.Fatalf("业务时间 = %v，应取参与终点 %v（不取登记时刻）", occurrence.OccurredAt(), occurrenceEndedAt)
	}
	if occurrence.LegalEntity().String() != trunkLegalEntity || occurrence.Agreement().String() != trunkSnapshot ||
		occurrence.Provider().String() != trunkProvider {
		t.Fatalf("采购三件 = %q / %q / %q", occurrence.LegalEntity(), occurrence.Agreement(), occurrence.Provider())
	}
	if assertion, asserted := occurrence.Condition().Assertion(); !asserted || assertion != conditionAssertion {
		t.Fatalf("断言没有原样留痕：%q %v", assertion, asserted)
	}
	if by, asserted := occurrence.Condition().AssertedBy(); !asserted || by.String() != registrant {
		t.Fatalf("断言人没有留痕：%q %v，应是做出这一次登记决定的操作者", by, asserted)
	}
	if record.Key.Validity.String() != "V1" || record.Key.Occurrence != occurrence.Occurrence() {
		t.Fatalf("键 = %+v", record.Key)
	}
	if len(rig.agreements.askedAt) != 1 || !rig.agreements.askedAt[0].Equal(occurrenceEndedAt) {
		t.Fatalf("协议核对时点 = %v，应是业务时间", rig.agreements.askedAt)
	}
	if !record.RecordedAt.Equal(occurrenceNow) {
		t.Fatalf("登记时刻 = %v", record.RecordedAt)
	}
	if len(rig.handoff.intents) != 1 || rig.handoff.intents[0].Record.Key != record.Key {
		t.Fatalf("意图 = %+v，want 恰一份且就是落册的那一份", rig.handoff.intents)
	}
}

// Covers: ADR-0207 决定四——身份由（租户、原因、事实依据、成员范围）派生：成员换个顺序报来是同一件事，同一段上
// 换一个成员范围是另一件事。
func TestTheOccurrenceIdentityIsDerivedFromItsFactAndMemberSet(t *testing.T) {
	both := trunkSegmentWith(t, participationRow(t, trunkParcel, true), participationRow(t, trunkSecondParcel, true))

	first := newOccurrenceRig(t)
	first.segments = both
	forward, _ := first.register(t, actualFulfillmentCommand(t, trunkParcel, trunkSecondParcel)).Record()

	second := newOccurrenceRig(t)
	second.segments = both
	backward, _ := second.register(t, actualFulfillmentCommand(t, trunkSecondParcel, trunkParcel)).Record()

	third := newOccurrenceRig(t)
	third.segments = both
	single, _ := third.register(t, actualFulfillmentCommand(t, trunkParcel)).Record()

	if forward.Key != backward.Key {
		t.Fatalf("成员换序得出两个身份：%v / %v", forward.Key.Occurrence, backward.Key.Occurrence)
	}
	if forward.Key.Occurrence == single.Key.Occurrence {
		t.Fatal("成员范围不同却落到同一个身份")
	}
}

// Covers: AT-TF-121——参与仍在场（没有终点）或对象不在段里，不形成发生项，各答各的未受理；段读不动答未决。
func TestAnOccurrenceNeedsAnEndedParticipationInTheSegment(t *testing.T) {
	cases := map[string]struct {
		segments segmentViewDouble
		objects  []string
		outcome  application.ChargeOccurrenceOutcome
		detail   application.ChargeOccurrenceDetail
	}{
		"still in the segment": {
			segments: trunkSegmentWith(t, participationRow(t, trunkParcel, false)),
			outcome:  application.ChargeOccurrenceInputNotAccepted, detail: application.ParticipationNotEnded,
		},
		"not a participant": {
			segments: trunkSegmentWith(t, participationRow(t, trunkParcel, true)),
			objects:  []string{trunkSecondParcel},
			outcome:  application.ChargeOccurrenceInputNotAccepted, detail: application.ObjectNotParticipating,
		},
		"no such segment": {
			segments: segmentViewDouble{},
			outcome:  application.ChargeOccurrenceInputNotAccepted, detail: application.ChargeFactNotFound,
		},
		"segment unreadable": {
			segments: segmentViewDouble{err: errors.New("segments down")},
			outcome:  application.ChargeOccurrenceUndecided, detail: application.FactSourceUnavailable,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			rig := newOccurrenceRig(t)
			rig.segments = testCase.segments
			result := rig.register(t, actualFulfillmentCommand(t, testCase.objects...))
			if result.Outcome() != testCase.outcome || result.Detail() != testCase.detail {
				t.Fatalf("outcome/detail = %q/%q, want %q/%q", result.Outcome(), result.Detail(), testCase.outcome, testCase.detail)
			}
			if testCase.outcome == application.ChargeOccurrenceUndecided && result.Continuation() == "" {
				t.Fatal("未决没有续办引用")
			}
			assertNothingWritten(t, rig)
		})
	}
}

// Covers: AT-TF-122——缺采购上下文答未受理，**绝不答不适用**（ADR-0098 决定二）；不适用只给显式声明自营与实际
// 承运商判为自营法人两格（决定三），都不落册；声明自营又给了服务方是自相矛盾。
func TestMissingProcurementIsNotAcceptedWhileOnlyDeclaredOrJudgedSelfOperationIsNotApplicable(t *testing.T) {
	missing := actualFulfillmentCommand(t)
	missing.AgreementVersion = ""
	declared := actualFulfillmentCommand(t)
	declared.SelfOperated, declared.Provider, declared.AgreementObject, declared.AgreementVersion = true, "", "", ""
	contradicted := actualFulfillmentCommand(t)
	contradicted.SelfOperated = true

	cases := map[string]struct {
		command   application.RegisterChargeOccurrenceCommand
		judgments judgmentViewDouble
		outcome   application.ChargeOccurrenceOutcome
		detail    application.ChargeOccurrenceDetail
	}{
		"procurement context missing": {command: missing,
			outcome: application.ChargeOccurrenceInputNotAccepted, detail: application.ProcurementContextMissing},
		"self operation declared": {command: declared,
			outcome: application.ChargeOccurrenceNotApplicable, detail: application.SelfOperatedDeclared},
		"self operation declared with a provider": {command: contradicted,
			outcome: application.ChargeOccurrenceInputNotAccepted, detail: application.SelfOperationContradicted},
		"actual carrier judged own legal entity": {command: actualFulfillmentCommand(t),
			judgments: judgedCarrier(t, domain.OwnOperatingLegalEntity, trunkLegalEntity),
			outcome:   application.ChargeOccurrenceNotApplicable, detail: application.ActualCarrierJudgedSelfOperated},
		"carrier judgment unreadable": {command: actualFulfillmentCommand(t),
			judgments: judgmentViewDouble{err: errors.New("judgments down")},
			outcome:   application.ChargeOccurrenceUndecided, detail: application.CarrierJudgmentUnavailable},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			rig := newOccurrenceRig(t)
			rig.judgments = testCase.judgments
			result := rig.register(t, testCase.command)
			if result.Outcome() != testCase.outcome || result.Detail() != testCase.detail {
				t.Fatalf("outcome/detail = %q/%q, want %q/%q", result.Outcome(), result.Detail(), testCase.outcome, testCase.detail)
			}
			assertNothingWritten(t, rig)
			if len(rig.agreements.askedAt) != 0 {
				t.Fatal("不该走到协议核对")
			}
		})
	}

	t.Run("an external carrier judgment does not make it self operated", func(t *testing.T) {
		rig := newOccurrenceRig(t)
		rig.judgments = judgedCarrier(t, domain.ExternalCarrierParty, trunkProvider)
		if outcome := rig.register(t, actualFulfillmentCommand(t)).Outcome(); outcome != application.ChargeOccurrenceRegistered {
			t.Fatalf("outcome = %q, want REGISTERED", outcome)
		}
	})

	t.Run("a carrier judged self operated after registration is a conflict", func(t *testing.T) {
		rig := newOccurrenceRig(t)
		rig.register(t, actualFulfillmentCommand(t))
		rig.judgments = judgedCarrier(t, domain.OwnOperatingLegalEntity, trunkLegalEntity)
		result := rig.register(t, actualFulfillmentCommand(t))
		if result.Outcome() != application.ChargeOccurrenceConflict {
			t.Fatalf("outcome = %q, want CONFLICT", result.Outcome())
		}
		if _, present := result.Record(); !present {
			t.Fatal("冲突没有回显册上原有的那一项")
		}
	})

	t.Run("declaring self operation over a registered occurrence is a conflict", func(t *testing.T) {
		rig := newOccurrenceRig(t)
		rig.register(t, actualFulfillmentCommand(t))
		result := rig.register(t, declared)
		if result.Outcome() != application.ChargeOccurrenceConflict {
			t.Fatalf("outcome = %q, want CONFLICT", result.Outcome())
		}
		if _, present := result.Record(); !present {
			t.Fatal("冲突没有回显册上原有的那一项")
		}
	})
}

// Covers: AT-TF-123——协议四个否定格各成一格，不形成；服务方不符带回协议约定的供应商；核对口读不动答未决。
func TestEachAgreementRefusalIsItsOwnOutcome(t *testing.T) {
	cases := map[ports.SupplierAgreementStanding]application.ChargeOccurrenceOutcome{
		ports.SupplierAgreementNotRegistered:    application.ChargeOccurrenceAgreementNotRegistered,
		ports.SupplierAgreementNotEffective:     application.ChargeOccurrenceAgreementNotEffective,
		ports.SupplierAgreementTerminated:       application.ChargeOccurrenceAgreementTerminated,
		ports.SupplierAgreementProviderMismatch: application.ChargeOccurrenceProviderMismatch,
	}
	for standing, want := range cases {
		t.Run(standing.String(), func(t *testing.T) {
			rig := newOccurrenceRig(t)
			rig.agreements.check = ports.SupplierAgreementCheck{Standing: standing}
			if standing == ports.SupplierAgreementProviderMismatch {
				rig.agreements.check.Supplier = occurrenceValue(t, domain.NewServiceProviderReference, "SYN-PARTY-CARRIER-LASTMILE-01")
			}
			result := rig.register(t, actualFulfillmentCommand(t))
			if result.Outcome() != want || result.Outcome().String() != standing.String() {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), want)
			}
			if standing == ports.SupplierAgreementProviderMismatch && result.AgreedSupplier() != "SYN-PARTY-CARRIER-LASTMILE-01" {
				t.Fatalf("服务方不符没带回约定的供应商：%q", result.AgreedSupplier())
			}
			assertNothingWritten(t, rig)
		})
	}

	t.Run("unreadable", func(t *testing.T) {
		rig := newOccurrenceRig(t)
		rig.agreements.err = errors.New("party commercial down")
		result := rig.register(t, actualFulfillmentCommand(t))
		if result.Outcome() != application.ChargeOccurrenceUndecided || result.Detail() != application.ChargeSupplierAgreementUnavailable ||
			result.Continuation() == "" {
			t.Fatalf("outcome/detail/continuation = %q/%q/%q", result.Outcome(), result.Detail(), result.Continuation())
		}
		assertNothingWritten(t, rig)
	})
}

// Covers: AT-TF-124——同一事实同一成员范围重登：同内容答已存在、回显原项、重交同一份意图（信封按 ID 认领，不多出
// 第二封）；异内容（数量不同）答冲突、回显原项、不交意图、不落第二条。
func TestReRegisteringAnswersExistingOrConflict(t *testing.T) {
	rig := newOccurrenceRig(t)
	first := rig.register(t, actualFulfillmentCommand(t))
	original, _ := first.Record()

	replay := rig.register(t, actualFulfillmentCommand(t))
	if replay.Outcome() != application.ChargeOccurrenceExisting {
		t.Fatalf("重放 outcome = %q, want EXISTING", replay.Outcome())
	}
	if echoed, _ := replay.Record(); echoed.Key != original.Key || !echoed.RecordedAt.Equal(original.RecordedAt) {
		t.Fatalf("重放回显的不是原项：%+v", echoed.Key)
	}
	if len(rig.handoff.intents) != 2 || rig.handoff.intents[1].Record.Key != original.Key {
		t.Fatalf("重放应重交同一份意图：%d 份", len(rig.handoff.intents))
	}

	// 另一位操作者照原样再报：断言人是登记决定的出处、不是内容，仍答已存在，回显的是原断言人。
	byAnother := actualFulfillmentCommand(t)
	byAnother.Registrant = "https://dex.syn.example#SYN-OPERATOR-CONFIG-02"
	again := rig.register(t, byAnother)
	if again.Outcome() != application.ChargeOccurrenceExisting {
		t.Fatalf("另一位操作者重报 outcome = %q, want EXISTING", again.Outcome())
	}
	if echoed, _ := again.Record(); echoed.Key != original.Key {
		t.Fatal("另一位操作者重报没有回显原项")
	} else if by, _ := echoed.Occurrence.Condition().AssertedBy(); by.String() != registrant {
		t.Fatalf("回显的断言人 = %q，应仍是原断言人", by)
	}

	changed := actualFulfillmentCommand(t)
	changed.Quantity = 2
	conflict := rig.register(t, changed)
	if conflict.Outcome() != application.ChargeOccurrenceConflict {
		t.Fatalf("异内容 outcome = %q, want CONFLICT", conflict.Outcome())
	}
	if echoed, _ := conflict.Record(); echoed.Key != original.Key {
		t.Fatal("冲突没有回显原项")
	}
	if rig.registry.saves != 1 || len(rig.handoff.intents) != 3 {
		t.Fatalf("冲突不该落第二条也不该交意图：落 %d 条，交 %d 份", rig.registry.saves, len(rig.handoff.intents))
	}
}

// Covers: AT-TF-125——同一段上先登了一个成员范围，再登一个与它有交集的另一范围：身份不同，却答冲突、带
// MEMBERS_ALREADY_COVERED、回显先登的那一项，不落第二条；没有交集的另一范围照常登成。声明自营的范围与已登的
// 外部发生项有交集，同样答冲突而不答不适用。交集读不动答未决。
func TestMembersAlreadyChargedOnTheSameFactAreAConflict(t *testing.T) {
	twoParcels := func(t *testing.T) *occurrenceRig {
		rig := newOccurrenceRig(t)
		rig.segments = trunkSegmentWith(t, participationRow(t, trunkParcel, true), participationRow(t, trunkSecondParcel, true))
		return rig
	}

	t.Run("an overlapping member set", func(t *testing.T) {
		rig := twoParcels(t)
		first := rig.register(t, actualFulfillmentCommand(t, trunkParcel))
		original, _ := first.Record()
		result := rig.register(t, actualFulfillmentCommand(t, trunkParcel, trunkSecondParcel))
		if result.Outcome() != application.ChargeOccurrenceConflict || result.Detail() != application.MembersAlreadyCovered {
			t.Fatalf("outcome/detail = %q/%q, want CONFLICT/MEMBERS_ALREADY_COVERED", result.Outcome(), result.Detail())
		}
		if echoed, present := result.Record(); !present || echoed.Key != original.Key {
			t.Fatal("冲突没有回显先登的那一项")
		}
		if rig.registry.saves != 1 || len(rig.handoff.intents) != 1 {
			t.Fatalf("不该落第二条也不该交第二份意图：落 %d 条，交 %d 份", rig.registry.saves, len(rig.handoff.intents))
		}
	})

	t.Run("a disjoint member set", func(t *testing.T) {
		rig := twoParcels(t)
		rig.register(t, actualFulfillmentCommand(t, trunkParcel))
		if outcome := rig.register(t, actualFulfillmentCommand(t, trunkSecondParcel)).Outcome(); outcome != application.ChargeOccurrenceRegistered {
			t.Fatalf("outcome = %q, want REGISTERED", outcome)
		}
	})

	t.Run("declaring self operation over a covered member", func(t *testing.T) {
		rig := twoParcels(t)
		rig.register(t, actualFulfillmentCommand(t, trunkParcel, trunkSecondParcel))
		declared := actualFulfillmentCommand(t, trunkParcel)
		declared.SelfOperated, declared.Provider, declared.AgreementObject, declared.AgreementVersion = true, "", "", ""
		result := rig.register(t, declared)
		if result.Outcome() != application.ChargeOccurrenceConflict || result.Detail() != application.MembersAlreadyCovered {
			t.Fatalf("outcome/detail = %q/%q, want CONFLICT/MEMBERS_ALREADY_COVERED", result.Outcome(), result.Detail())
		}
	})

	t.Run("an unreadable coverage", func(t *testing.T) {
		rig := twoParcels(t)
		rig.registry.coverageErr = errors.New("registry down")
		result := rig.register(t, actualFulfillmentCommand(t))
		if result.Outcome() != application.ChargeOccurrenceUndecided || result.Detail() != application.OccurrenceRegistryUnavailable ||
			result.Continuation() == "" {
			t.Fatalf("outcome/detail/continuation = %q/%q/%q", result.Outcome(), result.Detail(), result.Continuation())
		}
		assertNothingWritten(t, rig)
	})
}

// Covers: ADR-0207 决定五——落册或交意图任一答不出都答未决：装配点据此整笔回滚，不留一份落了册而结算等不到的发生项。
func TestAFailingRegistryOrHandoffIsUndecided(t *testing.T) {
	registryDown := newOccurrenceRig(t)
	registryDown.registry.saveErr = errors.New("registry down")
	if result := registryDown.register(t, actualFulfillmentCommand(t)); result.Outcome() != application.ChargeOccurrenceUndecided ||
		result.Detail() != application.OccurrenceRegistryUnavailable {
		t.Fatalf("册读不动 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}

	handoffDown := newOccurrenceRig(t)
	handoffDown.handoff.err = errors.New("outbox down")
	if result := handoffDown.register(t, actualFulfillmentCommand(t)); result.Outcome() != application.ChargeOccurrenceUndecided ||
		result.Detail() != application.OccurrenceHandoffUnavailable || result.Continuation() == "" {
		t.Fatalf("交意图失败 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
}

// Covers: 协议核对口答「在册」却交回不成形的法人与快照，是适配器缺陷，不是请求有错——上抛 error，不答未受理，
// 否则登记方会去改一份本来没错的请求；什么都不写。
func TestAnAgreementCheckHandingBackNoLegalEntityIsRaised(t *testing.T) {
	rig := newOccurrenceRig(t)
	rig.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementRegistered}
	result, err := rig.handler().Register(t.Context(), actualFulfillmentCommand(t))
	if err == nil {
		t.Fatalf("应上抛，却答 %q/%q", result.Outcome(), result.Detail())
	}
	assertNothingWritten(t, rig)
}

// Covers: 订舱与取消两种原因的事实依据随票 demo-chain-gaps/28 接上，在那之前如实答未受理；断言缺席同样不形成——
// 首版的发生条件只由登记者断言作答（ADR-0207 决定三）。
func TestReasonsWithoutAFactPathAndUnassertedConditionsAreNotAccepted(t *testing.T) {
	for _, reason := range []domain.ChargeOccurrenceReason{domain.BookingOccurrence, domain.CancellationOccurrence} {
		rig := newOccurrenceRig(t)
		command := actualFulfillmentCommand(t)
		command.Reason = reason
		if result := rig.register(t, command); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
			result.Detail() != application.ReasonNotRegistrable {
			t.Fatalf("%s outcome/detail = %q/%q", reason, result.Outcome(), result.Detail())
		}
	}

	rig := newOccurrenceRig(t)
	command := actualFulfillmentCommand(t)
	command.ConditionAssertion = "   "
	if result := rig.register(t, command); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
		result.Detail() != application.ConditionNotAsserted {
		t.Fatalf("未断言 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
	assertNothingWritten(t, rig)

	// 没有断言人就没有人为条件作答：操作者取自认证结果，缺了是入口漏了一道，答坏请求。
	anonymous := actualFulfillmentCommand(t)
	anonymous.Registrant = " "
	if result := rig.register(t, anonymous); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
		result.Detail() != application.ChargeRequestMalformed {
		t.Fatalf("无断言人 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
	assertNothingWritten(t, rig)
}
