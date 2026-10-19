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

// 本文件证实际履约发生项随参与结束自动形成（ADR-0218）：交接或有效交付信封的键 → 这件事实结束的那一条参与 → 段须已采用委托
// → 定旅程 → 既有登记用例；以及两层答复怎样折成对信封的处置（入账 / 重投 / 缺陷）。登记用例的各道门由它自己的用例证，这里接的
// 是真登记用例（替身只在它的读口与册上），证的是自动形成不重判、不漏判、不推断自营。夹具全是合成串，证据只记 S。

type formationHandoversStub struct {
	records map[ports.TransportHandoverKey]ports.TransportHandoverRecord
	err     error
}

func (stub formationHandoversStub) FindByKey(
	_ context.Context, key ports.TransportHandoverKey,
) (ports.TransportHandoverRecord, bool, error) {
	record, found := stub.records[key]
	return record, found, stub.err
}

// formationSegmentsStub 照段登记册的读法：按对象列出它有过参与的各段，再按键读整段。
type formationSegmentsStub struct {
	records []ports.FulfillmentSegmentRecord
	listErr error
	findErr error
	missing bool
}

func (stub formationSegmentsStub) FindSegmentsForObject(
	_ context.Context, _ domain.TenantID, object domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	if stub.listErr != nil {
		return nil, stub.listErr
	}
	var keys []ports.FulfillmentSegmentKey
	for _, record := range stub.records {
		if len(record.Segment.ParticipationHistory(object)) > 0 {
			keys = append(keys, record.Key)
		}
	}
	return keys, nil
}

func (stub formationSegmentsStub) FindByKey(
	_ context.Context, key ports.FulfillmentSegmentKey,
) (ports.FulfillmentSegmentRecord, bool, error) {
	if stub.findErr != nil {
		return ports.FulfillmentSegmentRecord{}, false, stub.findErr
	}
	for _, record := range stub.records {
		if record.Key == key && !stub.missing {
			return record, true, nil
		}
	}
	return ports.FulfillmentSegmentRecord{}, false, nil
}

type formationJourneysStub struct {
	records []ports.AlternateJourneyRecord
	err     error
}

func (stub formationJourneysStub) FindByMember(
	_ context.Context, _ domain.TenantID, _ domain.CarriedObjectReference,
) ([]ports.AlternateJourneyRecord, error) {
	return stub.records, stub.err
}

// registrationSpy 记下交给登记用例的命令；inner 为空时交回 canned。
type registrationSpy struct {
	inner    application.ChargeOccurrenceRegistration
	canned   application.RegisterChargeOccurrenceResult
	commands []application.RegisterChargeOccurrenceCommand
}

func (spy *registrationSpy) Register(
	ctx context.Context, command application.RegisterChargeOccurrenceCommand,
) (application.RegisterChargeOccurrenceResult, error) {
	spy.commands = append(spy.commands, command)
	if spy.inner == nil {
		return spy.canned, nil
	}
	return spy.inner.Register(ctx, command)
}

// formationRig 把自动形成编排接到一台发生项登记夹具上：段、采用连线、委托、协议与发生项册都是那台夹具的，编排与人工登记
// 读的是同一组。
type formationRig struct {
	occurrences *occurrenceRig
	handovers   formationHandoversStub
	segments    formationSegmentsStub
	journeys    formationJourneysStub
	spy         *registrationSpy
}

func newFormationRig(t *testing.T, occurrences *occurrenceRig) *formationRig {
	t.Helper()
	rig := &formationRig{
		occurrences: occurrences,
		handovers:   formationHandoversStub{records: map[ports.TransportHandoverKey]ports.TransportHandoverRecord{}},
		segments:    formationSegmentsStub{records: []ports.FulfillmentSegmentRecord{occurrences.segments.record}},
	}
	rig.spy = &registrationSpy{inner: occurrences.handler()}
	return rig
}

func (rig *formationRig) handler() *application.FormActualFulfillmentOnParticipationEndHandler {
	return application.NewFormActualFulfillmentOnParticipationEndHandler(application.FormActualFulfillmentOnParticipationEndDeps{
		Handovers:    rig.handovers,
		Segments:     rig.segments,
		Adoptions:    rig.occurrences.adoptions,
		Journeys:     rig.journeys,
		Registration: rig.spy,
	})
}

// handedOverAtSIN 登一版交接：对象在 SIN 被交出，版本与 participationRow 记下的终点依据是同一串，裁决由调用方定。
func (rig *formationRig) handedOverAtSIN(t *testing.T, object string, verdict domain.HandoverVerdict) application.FormActualFulfillmentOnHandoverCommand {
	t.Helper()
	spec := domain.TransportHandoverSpec{
		TenantID:          occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Object:            occurrenceValue(t, domain.NewCarriedObjectReference, object),
		Scope:             occurrenceValue(t, domain.NewHandoverScopeReference, "SYN-HANDOVER-SCOPE-SIN"),
		ReleasedBy:        occurrenceValue(t, domain.NewHandoverPartyReference, trunkProvider),
		ReceivedBy:        occurrenceValue(t, domain.NewHandoverPartyReference, "SYN-NODE-SIN-HUB"),
		Verdict:           verdict,
		ReleasingEvidence: occurrenceValue(t, domain.NewHandoverEvidenceReference, "SYN-EVIDENCE-RELEASE-SIN"),
		ReceivingEvidence: occurrenceValue(t, domain.NewHandoverEvidenceReference, "SYN-EVIDENCE-RECEIVE-SIN"),
		Rule:              occurrenceValue(t, domain.NewHandoverRuleReference, "SYN-HANDOVER-RULE/v1"),
		Version:           occurrenceValue(t, domain.NewHandoverResultVersion, "SYN-HO-SIN-"+object),
		JudgedAt:          occurrenceEndedAt,
	}
	if verdict != domain.ObjectHandedOver {
		spec.Basis = occurrenceValue(t, domain.NewHandoverBasisReference, "SYN-REFUSAL-BASIS-SIN")
	}
	handover, err := domain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("交接夹具：%v", err)
	}
	key := ports.TransportHandoverKey{TenantID: spec.TenantID, Object: spec.Object, Scope: spec.Scope, Version: spec.Version}
	rig.handovers.records[key] = ports.TransportHandoverRecord{Key: key, Handover: handover}
	return application.FormActualFulfillmentOnHandoverCommand{
		TenantID: spec.TenantID, Object: object, Scope: spec.Scope.String(), Version: spec.Version.String(),
	}
}

func (rig *formationRig) onHandover(t *testing.T, command application.FormActualFulfillmentOnHandoverCommand) application.FormActualFulfillmentResult {
	t.Helper()
	result, err := rig.handler().OnHandover(t.Context(), command)
	if err != nil {
		t.Fatalf("交接那一路不该上抛：%v", err)
	}
	return result
}

func assertFormationDisposition(t *testing.T, result application.FormActualFulfillmentResult, want application.ActualFulfillmentFormationDisposition) {
	t.Helper()
	if got := result.Disposition(); got != want {
		t.Fatalf("disposition = %s（outcome=%s reason=%s 登记=%s/%s）, want %s", got, result.Outcome(), result.UndecidedReason(),
			result.Registration().Outcome(), result.Registration().Detail(), want)
	}
}

func adoptedCommission(t *testing.T) ports.TransportCommissionRecord {
	t.Helper()
	return trunkCommission(t, commissionShape{startedAt: occurrenceEnteredAt})
}

// Covers: AT-TF-246——干线段已采用委托，对象在 SIN 以`已交接`交出、参与随之结束：交接信封一到，经既有登记用例形成实际履约
// 发生项。成员恰是这个对象，依据与范围取段、业务时间取参与终点，采购上下文取委托，旅程按包裹派生 `JRN/<包裹>`，登记者是本
// 上下文的系统身份；落册与已形成意图在同一次调用里，处置入账。
func TestAHandoverEndingAnAdoptedSegmentsParticipationFormsTheOccurrence(t *testing.T) {
	occurrences := adoptedTrunkRig(t, adoptedCommission(t))
	rig := newFormationRig(t, occurrences)

	result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
	if result.Outcome() != application.FormationRegistrationAnswered || result.Segment() != trunkSegment {
		t.Fatalf("outcome = %s segment = %s", result.Outcome(), result.Segment())
	}
	occurrence := registeredOccurrence(t, result.Registration())
	members := occurrence.Members()
	if len(members) != 1 || members[0].String() != trunkParcel || occurrence.Journey().String() != "JRN/"+trunkParcel ||
		occurrence.Reason() != domain.ActualFulfillmentOccurrence || occurrence.Scope().String() != trunkSegment ||
		!occurrence.OccurredAt().Equal(occurrenceEndedAt) || occurrence.Provider().String() != trunkProvider ||
		occurrence.Agreement().String() != trunkSnapshot || occurrence.LegalEntity().String() != trunkLegalEntity {
		t.Fatalf("自动形成的发生项 = %+v", occurrence)
	}
	if registeredBy := occurrence.Condition().RegisteredBy().String(); registeredBy != application.ActualFulfillmentFormationRegistrant {
		t.Fatalf("登记者 = %q, want 本上下文的系统身份", registeredBy)
	}
	command := rig.spy.commands[0]
	if command.Provider != "" || command.AgreementObject != "" || command.AgreementVersion != "" || command.SelfOperated {
		t.Fatalf("自动形成不该报采购上下文、不该声明自营：%+v", command)
	}
	if occurrences.registry.saves != 1 || len(occurrences.handoff.intents) != 1 {
		t.Fatalf("落 %d 条、交 %d 份意图, want 各 1", occurrences.registry.saves, len(occurrences.handoff.intents))
	}
	assertFormationDisposition(t, result, application.FormationSettled)
}

// Covers: AT-TF-247——同一段的参与由有效交付结束：交付信封所指的那一版结束了它，照样形成；POD 更正出的新版本没有结束任何参与，
// 入账不形成第二项。
func TestAnEffectiveDeliveryEndingAnAdoptedSegmentsParticipationFormsTheOccurrence(t *testing.T) {
	occurrences := adoptedTrunkRig(t, adoptedCommission(t))
	row := participationRow(t, trunkParcel, false)
	row.EndKind = domain.EndedByEffectiveDelivery
	row.EndBasis = occurrenceValue(t, domain.NewParticipationBasisReference, "EFFECTIVE-DELIVERY/SYN-DRV-01")
	row.EndedAt = occurrenceEndedAt
	occurrences.segments = trunkSegmentWith(t, row)
	rig := newFormationRig(t, occurrences)

	delivered := func(version string) application.FormActualFulfillmentResult {
		result, err := rig.handler().OnDelivery(t.Context(), application.FormActualFulfillmentOnDeliveryCommand{
			TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
			Object:   trunkParcel,
			Attempt:  "SYN-ATTEMPT-DELIVERY-01",
			Version:  version,
		})
		if err != nil {
			t.Fatalf("交付那一路不该上抛：%v", err)
		}
		return result
	}
	first := delivered("SYN-DRV-01")
	registeredOccurrence(t, first.Registration())
	assertFormationDisposition(t, first, application.FormationSettled)

	corrected := delivered("SYN-DRV-02")
	if corrected.Outcome() != application.FormationNoParticipationEnded {
		t.Fatalf("POD 更正那一封 outcome = %s, want NO_PARTICIPATION_ENDED", corrected.Outcome())
	}
	assertFormationDisposition(t, corrected, application.FormationSettled)
	if occurrences.registry.saves != 1 {
		t.Fatalf("更正那一封多落了一项：共 %d 项", occurrences.registry.saves)
	}
}

// Covers: AT-TF-248——段没有采用委托：不形成、不推断自营。实际承运商判断哪怕判成自营法人，也不交给登记用例去答不适用；零写入，
// 入账。
func TestASegmentWithoutAnAdoptionFormsNothingAndInfersNoSelfOperation(t *testing.T) {
	occurrences := newOccurrenceRig(t)
	occurrences.judgments = judgedCarrier(t, domain.OwnOperatingLegalEntity, trunkLegalEntity)
	rig := newFormationRig(t, occurrences)

	result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
	if result.Outcome() != application.FormationSegmentNotAdopted || result.Segment() != trunkSegment {
		t.Fatalf("outcome = %s segment = %s, want SEGMENT_NOT_ADOPTED 并指名那一段", result.Outcome(), result.Segment())
	}
	if len(rig.spy.commands) != 0 {
		t.Fatalf("没有采用连线却交给了登记用例 %d 次", len(rig.spy.commands))
	}
	assertNothingWritten(t, occurrences)
	assertFormationDisposition(t, result, application.FormationSettled)
}

// Covers: AT-TF-249——采用的那一版协议没声明实际履约的收费发生条件答`未配置`、声明不计费答不计费：都入账、不落册。
func TestAnAgreementThatDoesNotChargeOrDeclareActualFulfillmentSettlesWithoutAnOccurrence(t *testing.T) {
	for name, conditions := range map[string]domain.AgreementChargeConditions{
		"未声明": declaredConditions(t, declaredCondition{domain.FailedAttemptOccurrence, true, domain.PerAttemptQuantity}),
		"不计费": declaredConditions(t, declaredCondition{domain.ActualFulfillmentOccurrence, false, 0}),
	} {
		t.Run(name, func(t *testing.T) {
			occurrences := adoptedTrunkRig(t, adoptedCommission(t))
			occurrences.agreements.check.ChargeConditions = conditions
			rig := newFormationRig(t, occurrences)
			result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
			if outcome := result.Registration().Outcome(); outcome != application.ChargeOccurrenceNotConfigured &&
				outcome != application.ChargeOccurrenceNotCharged {
				t.Fatalf("登记 outcome = %s", outcome)
			}
			assertNothingWritten(t, occurrences)
			assertFormationDisposition(t, result, application.FormationSettled)
		})
	}
}

// Covers: AT-TF-250——人工登记与自动形成落在同一身份上：旅程写派生拼写时，先自动后人工、先人工后自动都答`已存在`，回显先到的
// 那一份（登记者不进内容比对）；人工先写了另一个旅程，自动那一拍答冲突，入账，册上那一份不动。
func TestManualAndAutomaticRegistrationOfTheSameIdentityAnswerExisting(t *testing.T) {
	manual := func() application.RegisterChargeOccurrenceCommand {
		command := actualFulfillmentCommand(t)
		command.Provider, command.AgreementObject, command.AgreementVersion = "", "", ""
		command.Journey = "JRN/" + trunkParcel
		return command
	}

	t.Run("先自动后人工", func(t *testing.T) {
		occurrences := adoptedTrunkRig(t, adoptedCommission(t))
		rig := newFormationRig(t, occurrences)
		registeredOccurrence(t, rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver)).Registration())
		again := occurrences.register(t, manual())
		record, _ := again.Record()
		if again.Outcome() != application.ChargeOccurrenceExisting ||
			record.Occurrence.Condition().RegisteredBy().String() != application.ActualFulfillmentFormationRegistrant {
			t.Fatalf("人工再登 = %s，登记者 %q", again.Outcome(), record.Occurrence.Condition().RegisteredBy())
		}
	})
	t.Run("先人工后自动", func(t *testing.T) {
		occurrences := adoptedTrunkRig(t, adoptedCommission(t))
		registeredOccurrence(t, occurrences.register(t, manual()))
		rig := newFormationRig(t, occurrences)
		result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
		record, _ := result.Registration().Record()
		if result.Registration().Outcome() != application.ChargeOccurrenceExisting || record.Occurrence.Condition().RegisteredBy().String() != registrant {
			t.Fatalf("自动那一拍 = %s，登记者 %q", result.Registration().Outcome(), record.Occurrence.Condition().RegisteredBy())
		}
		assertFormationDisposition(t, result, application.FormationSettled)
		if occurrences.registry.saves != 1 {
			t.Fatalf("册上 %d 项, want 1", occurrences.registry.saves)
		}
	})
	t.Run("人工写了另一个旅程", func(t *testing.T) {
		occurrences := adoptedTrunkRig(t, adoptedCommission(t))
		other := manual()
		other.Journey = "SYN-JRN-" + trunkParcel
		registeredOccurrence(t, occurrences.register(t, other))
		rig := newFormationRig(t, occurrences)
		result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
		if result.Registration().Outcome() != application.ChargeOccurrenceConflict {
			t.Fatalf("自动那一拍 = %s, want CONFLICT", result.Registration().Outcome())
		}
		assertFormationDisposition(t, result, application.FormationSettled)
		if occurrences.registry.saves != 1 {
			t.Fatalf("册上 %d 项, want 1", occurrences.registry.saves)
		}
	})
}

// Covers: AT-TF-251——这件事实没有结束任何参与：拒收与待确认不转出控制（不问段）；`已交接`而对象不在任何段里、或在段里还没
// 被它结束，都入账，不交登记用例。
func TestAFactThatEndsNoParticipationSettlesWithoutRegistration(t *testing.T) {
	for name, verdict := range map[string]domain.HandoverVerdict{"拒收": domain.HandoverRefused, "待确认": domain.HandoverPendingConfirmation} {
		t.Run(name, func(t *testing.T) {
			rig := newFormationRig(t, adoptedTrunkRig(t, adoptedCommission(t)))
			command := rig.handedOverAtSIN(t, trunkParcel, verdict)
			rig.segments.listErr = errors.New("段登记册不该被问")
			result := rig.onHandover(t, command)
			if result.Outcome() != application.FormationHandoverNotHandedOver {
				t.Fatalf("outcome = %s, want HANDOVER_NOT_HANDED_OVER", result.Outcome())
			}
			assertFormationDisposition(t, result, application.FormationSettled)
		})
	}
	t.Run("对象不在段里", func(t *testing.T) {
		rig := newFormationRig(t, adoptedTrunkRig(t, adoptedCommission(t)))
		result := rig.onHandover(t, rig.handedOverAtSIN(t, "SYN-PARCEL-ELSEWHERE", domain.ObjectHandedOver))
		if result.Outcome() != application.FormationNoParticipationEnded || len(rig.spy.commands) != 0 {
			t.Fatalf("outcome = %s，交登记 %d 次", result.Outcome(), len(rig.spy.commands))
		}
		assertFormationDisposition(t, result, application.FormationSettled)
	})
	t.Run("参与还在场", func(t *testing.T) {
		occurrences := adoptedTrunkRig(t, adoptedCommission(t))
		occurrences.segments = trunkSegmentWith(t, participationRow(t, trunkParcel, false))
		rig := newFormationRig(t, occurrences)
		result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
		if result.Outcome() != application.FormationNoParticipationEnded {
			t.Fatalf("outcome = %s, want NO_PARTICIPATION_ENDED", result.Outcome())
		}
	})
}

// Covers: AT-TF-252——对象在参与入场之前已开始一趟退运旅程：发生项挂在那一趟上，不挂派生拼写。
func TestTheOccurrenceJoinsTheAlternateJourneyStartedBeforeTheParticipation(t *testing.T) {
	occurrences := adoptedTrunkRig(t, adoptedCommission(t))
	rig := newFormationRig(t, occurrences)
	journey, err := domain.FormAlternateJourney(domain.AlternateJourneySpec{
		TenantID:        occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Journey:         occurrenceValue(t, domain.NewJourneyReference, "SYN-JRN-RETURN-"+trunkParcel),
		Purpose:         domain.ReturnJourneyPurpose,
		OriginalJourney: occurrenceValue(t, domain.NewJourneyReference, "JRN/"+trunkParcel),
		BasisKind:       domain.ServiceDispositionDecision,
		Basis:           occurrenceValue(t, domain.NewDispositionBasisReference, "SYN-DISPOSITION-RETURN-01"),
		Members:         []domain.CarriedObjectReference{occurrenceValue(t, domain.NewCarriedObjectReference, trunkParcel)},
		StartedAt:       occurrenceEnteredAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	rig.journeys.records = []ports.AlternateJourneyRecord{{Journey: journey}}

	result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
	if occurrence := registeredOccurrence(t, result.Registration()); occurrence.Journey().String() != "SYN-JRN-RETURN-"+trunkParcel {
		t.Fatalf("旅程 = %s, want 那一趟退运", occurrence.Journey())
	}
}

// Covers: AT-TF-253——读不动就重投：交接册读不动或还读不回、段登记册、采用连线、旅程册读不动，登记用例的未决（协议核对口读
// 不动）；都不落册。
func TestUnreadableDependenciesAreRetriedWithoutWriting(t *testing.T) {
	unreadable := errors.New("读不动")
	cases := map[string]struct {
		breakIt func(rig *formationRig, command *application.FormActualFulfillmentOnHandoverCommand)
		reason  application.ActualFulfillmentFormationUndecidedReason
	}{
		"交接册读不动": {func(rig *formationRig, _ *application.FormActualFulfillmentOnHandoverCommand) {
			rig.handovers.err = unreadable
		}, application.FormationHandoverRegistryUnavailable},
		"交接还读不回": {func(_ *formationRig, command *application.FormActualFulfillmentOnHandoverCommand) {
			command.Version = "SYN-HO-NOT-YET"
		}, application.FormationHandoverNotVisible},
		"段登记册列不出": {func(rig *formationRig, _ *application.FormActualFulfillmentOnHandoverCommand) {
			rig.segments.listErr = unreadable
		}, application.FormationSegmentRegistryUnavailable},
		"段登记册读不动": {func(rig *formationRig, _ *application.FormActualFulfillmentOnHandoverCommand) {
			rig.segments.findErr = unreadable
		}, application.FormationSegmentRegistryUnavailable},
		"采用连线读不动": {func(rig *formationRig, _ *application.FormActualFulfillmentOnHandoverCommand) {
			adoptions := rig.occurrences.adoptions
			adoptions.err = unreadable
			rig.occurrences.adoptions = adoptions
		}, application.FormationAdoptionRegistryUnavailable},
		"旅程册读不动": {func(rig *formationRig, _ *application.FormActualFulfillmentOnHandoverCommand) {
			rig.journeys.err = unreadable
		}, application.FormationJourneyRegistryUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			occurrences := adoptedTrunkRig(t, adoptedCommission(t))
			rig := newFormationRig(t, occurrences)
			command := rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver)
			tc.breakIt(rig, &command)
			result := rig.onHandover(t, command)
			if result.Outcome() != application.FormationUndecided || result.UndecidedReason() != tc.reason {
				t.Fatalf("outcome = %s reason = %s, want 未决 %s", result.Outcome(), result.UndecidedReason(), tc.reason)
			}
			assertFormationDisposition(t, result, application.FormationRetry)
			assertNothingWritten(t, occurrences)
		})
	}

	t.Run("协议核对口读不动", func(t *testing.T) {
		occurrences := adoptedTrunkRig(t, adoptedCommission(t))
		occurrences.agreements.err = unreadable
		rig := newFormationRig(t, occurrences)
		result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
		if result.Registration().Outcome() != application.ChargeOccurrenceUndecided ||
			result.Registration().Detail() != application.ChargeSupplierAgreementUnavailable {
			t.Fatalf("登记 = %s/%s", result.Registration().Outcome(), result.Registration().Detail())
		}
		assertFormationDisposition(t, result, application.FormationRetry)
		assertNothingWritten(t, occurrences)
	})
}

// Covers: AT-TF-254——登记用例答的业务格照入账：迁移 0038 之前提交的委托没记下协议版本，答缺采购上下文；委托那一版在业务
// 时间未生效，照协议否定格答；协议按段计时同一段第二件包裹的参与结束答范围已计过——第一件已盖住整段。
func TestRegistrationBusinessAnswersSettle(t *testing.T) {
	t.Run("委托没记下协议版本", func(t *testing.T) {
		occurrences := adoptedTrunkRig(t, trunkCommission(t, commissionShape{startedAt: occurrenceEnteredAt, legacy: true}))
		rig := newFormationRig(t, occurrences)
		result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
		if result.Registration().Outcome() != application.ChargeOccurrenceInputNotAccepted ||
			result.Registration().Detail() != application.ProcurementContextMissing {
			t.Fatalf("登记 = %s/%s", result.Registration().Outcome(), result.Registration().Detail())
		}
		assertFormationDisposition(t, result, application.FormationSettled)
		assertNothingWritten(t, occurrences)
	})
	t.Run("委托那一版在业务时间未生效", func(t *testing.T) {
		occurrences := adoptedTrunkRig(t, adoptedCommission(t))
		occurrences.agreements.check.Standing = ports.SupplierAgreementNotEffective
		rig := newFormationRig(t, occurrences)
		result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
		if result.Registration().Outcome() != application.ChargeOccurrenceAgreementNotEffective {
			t.Fatalf("登记 = %s", result.Registration().Outcome())
		}
		assertFormationDisposition(t, result, application.FormationSettled)
		assertNothingWritten(t, occurrences)
	})
	t.Run("按段计的第二件", func(t *testing.T) {
		occurrences := adoptedTrunkRig(t, adoptedCommission(t))
		occurrences.agreements.check.ChargeConditions = declaredConditions(t,
			declaredCondition{domain.ActualFulfillmentOccurrence, true, domain.PerSegmentQuantity})
		occurrences.segments = trunkSegmentWith(t, participationRow(t, trunkParcel, true), participationRow(t, trunkSecondParcel, true))
		rig := newFormationRig(t, occurrences)
		registeredOccurrence(t, rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver)).Registration())
		second := rig.onHandover(t, rig.handedOverAtSIN(t, trunkSecondParcel, domain.ObjectHandedOver))
		if second.Registration().Outcome() != application.ChargeOccurrenceConflict ||
			second.Registration().Detail() != application.ScopeAlreadyCharged {
			t.Fatalf("第二件 = %s/%s, want CONFLICT/SCOPE_ALREADY_CHARGED", second.Registration().Outcome(), second.Registration().Detail())
		}
		assertFormationDisposition(t, second, application.FormationSettled)
	})
}

// Covers: AT-TF-255——本上下文自己的缺陷响亮失败：自家键过不了自家门、同一件事实在两个段里都结束了参与、旅程册同一刻开始
// 两趟、登记用例答出本路不该见的格（不适用、缺采购上下文以外的未受理、协议不在册、供应商不符、采购上下文冲突）。
func TestFormationDefectsAreNeitherSettledNorRetried(t *testing.T) {
	t.Run("自家键不受理", func(t *testing.T) {
		rig := newFormationRig(t, adoptedTrunkRig(t, adoptedCommission(t)))
		result := rig.onHandover(t, application.FormActualFulfillmentOnHandoverCommand{
			TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant), Object: trunkParcel})
		assertFormationDisposition(t, result, application.FormationDefect)
		delivered, err := rig.handler().OnDelivery(t.Context(), application.FormActualFulfillmentOnDeliveryCommand{
			TenantID: occurrenceValue(t, domain.NewTenantID, occurrenceTenant), Object: trunkParcel, Attempt: "SYN-ATTEMPT-01"})
		if err != nil || delivered.Outcome() != application.FormationNotAccepted {
			t.Fatalf("交付缺版本 = %s %v", delivered.Outcome(), err)
		}
		assertFormationDisposition(t, delivered, application.FormationDefect)
	})
	t.Run("两段都被同一件事实结束", func(t *testing.T) {
		occurrences := adoptedTrunkRig(t, adoptedCommission(t))
		rig := newFormationRig(t, occurrences)
		twin := occurrences.segments.record
		twin.Key.Segment = occurrenceValue(t, domain.NewFulfillmentSegmentReference, "SYN-SEG-TWIN")
		rig.segments.records = append(rig.segments.records, twin)
		if _, err := rig.handler().OnHandover(t.Context(), rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver)); !errors.Is(err, application.ErrParticipationEndedInSeveralSegments) {
			t.Fatalf("err = %v, want ErrParticipationEndedInSeveralSegments", err)
		}
	})
	t.Run("按对象列得出、按键读不回", func(t *testing.T) {
		rig := newFormationRig(t, adoptedTrunkRig(t, adoptedCommission(t)))
		rig.segments.missing = true
		if _, err := rig.handler().OnHandover(t.Context(), rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver)); err == nil {
			t.Fatal("段登记册自相矛盾却没上抛")
		}
	})
	t.Run("旅程同一刻开始两趟", func(t *testing.T) {
		rig := newFormationRig(t, adoptedTrunkRig(t, adoptedCommission(t)))
		for _, name := range []string{"SYN-JRN-A", "SYN-JRN-B"} {
			journey, err := domain.FormAlternateJourney(domain.AlternateJourneySpec{
				TenantID:        occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
				Journey:         occurrenceValue(t, domain.NewJourneyReference, name),
				Purpose:         domain.AlternateJourneyPurpose,
				OriginalJourney: occurrenceValue(t, domain.NewJourneyReference, "JRN/"+trunkParcel),
				BasisKind:       domain.ServiceDispositionDecision,
				Basis:           occurrenceValue(t, domain.NewDispositionBasisReference, "SYN-DISPOSITION-"+name),
				Members:         []domain.CarriedObjectReference{occurrenceValue(t, domain.NewCarriedObjectReference, trunkParcel)},
				StartedAt:       occurrenceEnteredAt.Add(-time.Hour),
			})
			if err != nil {
				t.Fatal(err)
			}
			rig.journeys.records = append(rig.journeys.records, ports.AlternateJourneyRecord{Journey: journey})
		}
		if _, err := rig.handler().OnHandover(t.Context(), rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver)); !errors.Is(err, domain.ErrAmbiguousJourney) {
			t.Fatalf("err = %v, want ErrAmbiguousJourney", err)
		}
	})
	for name, canned := range map[string]application.RegisterChargeOccurrenceResult{
		"不适用":       cannedRegistration(t, application.ChargeOccurrenceNotApplicable),
		"参与未结束的未受理": cannedRegistration(t, application.ChargeOccurrenceInputNotAccepted),
		"协议不在册":     cannedRegistration(t, application.ChargeOccurrenceAgreementNotRegistered),
		"供应商不符":     cannedRegistration(t, application.ChargeOccurrenceProviderMismatch),
		"采购上下文冲突":   cannedRegistration(t, application.ChargeOccurrenceConflict),
		"集外一格":      {},
	} {
		t.Run(name, func(t *testing.T) {
			rig := newFormationRig(t, adoptedTrunkRig(t, adoptedCommission(t)))
			rig.spy = &registrationSpy{canned: canned}
			result := rig.onHandover(t, rig.handedOverAtSIN(t, trunkParcel, domain.ObjectHandedOver))
			assertFormationDisposition(t, result, application.FormationDefect)
		})
	}
}

// cannedRegistration 造一份登记用例的答复，只为证处置怎样折它：经真登记用例答出来——不适用由声明自营得出，未受理由参与未结束
// 得出，协议不在册与供应商不符由人工报协议的那一路得出，冲突取段已采用委托而报来另一家服务方的采购上下文冲突。这几格本路都
// 走不到，只能借人工那一路的命令造出来。
func cannedRegistration(t *testing.T, outcome application.ChargeOccurrenceOutcome) application.RegisterChargeOccurrenceResult {
	t.Helper()
	rig := newOccurrenceRig(t)
	command := actualFulfillmentCommand(t)
	switch outcome {
	case application.ChargeOccurrenceNotApplicable:
		command.SelfOperated = true
		command.Provider, command.AgreementObject, command.AgreementVersion = "", "", ""
	case application.ChargeOccurrenceInputNotAccepted:
		rig.segments = trunkSegmentWith(t, participationRow(t, trunkParcel, false))
	case application.ChargeOccurrenceAgreementNotRegistered:
		rig.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementNotRegistered}
	case application.ChargeOccurrenceProviderMismatch:
		rig.agreements.check = ports.SupplierAgreementCheck{
			Standing: ports.SupplierAgreementProviderMismatch,
			Supplier: occurrenceValue(t, domain.NewServiceProviderReference, "SYN-PARTY-CARRIER-LASTMILE-01"),
		}
	case application.ChargeOccurrenceConflict:
		rig = adoptedTrunkRig(t, adoptedCommission(t))
		command.Provider = "SYN-PARTY-CARRIER-LM-01"
	}
	result := rig.register(t, command)
	if result.Outcome() != outcome {
		t.Fatalf("夹具答复 = %s, want %s", result.Outcome(), outcome)
	}
	if outcome == application.ChargeOccurrenceConflict && result.Detail() != application.ProcurementContextConflict {
		t.Fatalf("夹具细格 = %s, want PROCUREMENT_CONTEXT_CONFLICT", result.Detail())
	}
	return result
}
