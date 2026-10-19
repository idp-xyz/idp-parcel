package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证运输收费发生项登记的订舱与取消两种原因，以及事实带着委托时采购上下文从委托取（票 demo-chain-gaps/28，ADR-0207
// 决定一、二与文末补记）：订舱的依据是已接受的承运答复，取消的依据是尚未开始范围的订舱取消；订舱属于委托，实际履约段
// 采用了委托，采购上下文就取委托的，登记方可以不给，给了而不一致答冲突。夹具全是合成串，证据只记 S。

var (
	bkSubmittedAt = time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	bkRequestedAt = time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	bkAcceptedAt  = time.Date(2026, 10, 18, 4, 0, 0, 0, time.UTC)
	bkCancelledAt = time.Date(2026, 10, 18, 5, 0, 0, 0, time.UTC)
)

const (
	bkCommission = "SYN-COMMISSION-TRUNK-01"
	bkBooking    = "SYN-BOOKING-TRUNK-01"
	bkAcceptance = "SYN-ANSWER-TRUNK-01"
)

type adoptionViewDouble struct {
	records map[domain.FulfillmentSegmentReference]ports.SegmentCommissionAdoptionRecord
	err     error
}

func (double adoptionViewDouble) FindBySegment(
	_ context.Context, key ports.SegmentCommissionAdoptionKey,
) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	record, found := double.records[key.Segment]
	return record, found, double.err
}

type commissionViewDouble struct {
	records map[domain.TransportCommissionReference]ports.TransportCommissionRecord
	err     error
}

func (double commissionViewDouble) FindByKey(
	_ context.Context, key ports.TransportCommissionKey,
) (ports.TransportCommissionRecord, bool, error) {
	record, found := double.records[key.Commission]
	return record, found, double.err
}

type bookingViewDouble struct {
	records map[domain.BookingReference]ports.BookingRecord
	err     error
}

func (double bookingViewDouble) FindByKey(_ context.Context, key ports.BookingKey) (ports.BookingRecord, bool, error) {
	record, found := double.records[key.Booking]
	return record, found, double.err
}

type bookingAnswerViewDouble struct {
	records map[domain.BookingReference]ports.BookingAnswerRecord
	err     error
}

func (double bookingAnswerViewDouble) FindByKey(_ context.Context, key ports.BookingKey) (ports.BookingAnswerRecord, bool, error) {
	record, found := double.records[key.Booking]
	return record, found, double.err
}

// commissionShape 是一份干线委托的可变几格：开始与取消、提交时有没有记下协议版本（迁移 TF 0038 之前的委托没有）。
type commissionShape struct {
	startedAt   time.Time
	cancelledAt time.Time
	legacy      bool
}

func trunkCommission(t *testing.T, shape commissionShape) ports.TransportCommissionRecord {
	t.Helper()
	tenant := occurrenceValue(t, domain.NewTenantID, occurrenceTenant)
	spec := domain.RehydrateTransportCommissionSpec{
		TenantID:       tenant,
		Commission:     occurrenceValue(t, domain.NewTransportCommissionReference, bkCommission),
		Provider:       occurrenceValue(t, domain.NewServiceProviderReference, trunkProvider),
		Agreement:      occurrenceValue(t, domain.NewAgreementSnapshotReference, trunkSnapshot),
		LegalEntity:    occurrenceValue(t, domain.NewProcurementLegalEntityReference, trunkLegalEntity),
		Conditions:     occurrenceValue(t, domain.NewConditionsSnapshotReference, "SYN-COND-TRUNK-01"),
		Role:           occurrenceValue(t, domain.NewRoleSnapshotReference, "SYN-ROLE-TRUNK-01"),
		Responsibility: occurrenceValue(t, domain.NewResponsibilitySnapshotReference, "SYN-RESP-TRUNK-01"),
		Members: []domain.CarriedObjectReference{
			occurrenceValue(t, domain.NewCarriedObjectReference, trunkParcel),
			occurrenceValue(t, domain.NewCarriedObjectReference, trunkSecondParcel),
		},
		SubmittedAt: bkSubmittedAt,
		CancelledAt: shape.cancelledAt,
	}
	if !shape.startedAt.IsZero() {
		spec.StartedAt = shape.startedAt
		spec.StartedBasis = occurrenceValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SZX-START")
	}
	if !shape.legacy {
		version, err := domain.NewSupplierAgreementVersion(trunkAgreement, trunkAgreementTag)
		if err != nil {
			t.Fatal(err)
		}
		spec.AgreementVersion = version
	}
	commission, err := domain.RehydrateTransportCommission(spec)
	if err != nil {
		t.Fatalf("委托夹具：%v", err)
	}
	return ports.TransportCommissionRecord{
		Key:        ports.TransportCommissionKey{TenantID: tenant, Commission: commission.Commission()},
		Commission: commission,
	}
}

// trunkBooking 是干线委托下的一次订舱；cancelledAt 非零时已取消。
func trunkBooking(t *testing.T, requestedAt, cancelledAt time.Time) ports.BookingRecord {
	t.Helper()
	tenant := occurrenceValue(t, domain.NewTenantID, occurrenceTenant)
	booking, err := domain.SubmitBookingRequest(domain.BookingRequestSpec{
		TenantID:    tenant,
		Booking:     occurrenceValue(t, domain.NewBookingReference, bkBooking),
		Commission:  occurrenceValue(t, domain.NewTransportCommissionReference, bkCommission),
		Schedule:    occurrenceValue(t, domain.NewScheduleReference, "SYN-SCHED-SZX-SIN-DAILY/2026-10-21"),
		Quantity:    120,
		Unit:        occurrenceValue(t, domain.NewQuantityUnitReference, "KG"),
		RequestedAt: requestedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cancelledAt.IsZero() {
		if booking, err = booking.Cancel(cancelledAt); err != nil {
			t.Fatal(err)
		}
	}
	return ports.BookingRecord{Key: ports.BookingKey{TenantID: tenant, Booking: booking.Booking()}, Booking: booking}
}

func trunkAnswer(t *testing.T, booking ports.BookingRecord, outcome domain.CarrierAcceptanceOutcome, decidedAt time.Time) ports.BookingAnswerRecord {
	t.Helper()
	quantity, basis := int64(120), domain.AcceptanceBasisReference{}
	if outcome != domain.BookingAccepted {
		quantity = 0
		basis = occurrenceValue(t, domain.NewAcceptanceBasisReference, "SYN-CARRIER-REASON-01")
	}
	answer, err := domain.FormCarrierAcceptance(booking.Booking,
		occurrenceValue(t, domain.NewCarrierAcceptanceReference, bkAcceptance), outcome, quantity, basis, decidedAt)
	if err != nil {
		t.Fatalf("答复夹具：%v", err)
	}
	return ports.BookingAnswerRecord{Key: booking.Key, Acceptance: answer}
}

// bookingRig 立一台按订舱登记的夹具：干线委托（记下了 v2）、一次订舱、一份接受；协议声明订舱与取消都按订舱计。
func bookingRig(t *testing.T, commission ports.TransportCommissionRecord, booking ports.BookingRecord, answer *ports.BookingAnswerRecord) *occurrenceRig {
	t.Helper()
	rig := newOccurrenceRig(t)
	rig.agreements.check.ChargeConditions = declaredConditions(t,
		declaredCondition{domain.BookingOccurrence, true, domain.PerBookingQuantity},
		declaredCondition{domain.CancellationOccurrence, true, domain.PerBookingQuantity},
		declaredCondition{domain.ActualFulfillmentOccurrence, true, domain.PerPieceQuantity})
	rig.commissions = commissionViewDouble{records: map[domain.TransportCommissionReference]ports.TransportCommissionRecord{
		commission.Key.Commission: commission,
	}}
	rig.bookings = bookingViewDouble{records: map[domain.BookingReference]ports.BookingRecord{booking.Key.Booking: booking}}
	if answer != nil {
		rig.answers = bookingAnswerViewDouble{records: map[domain.BookingReference]ports.BookingAnswerRecord{booking.Key.Booking: *answer}}
	}
	return rig
}

func acceptedBookingRig(t *testing.T) *occurrenceRig {
	t.Helper()
	booking := trunkBooking(t, bkRequestedAt, time.Time{})
	answer := trunkAnswer(t, booking, domain.BookingAccepted, bkAcceptedAt)
	return bookingRig(t, trunkCommission(t, commissionShape{}), booking, &answer)
}

// bookingCommand 是一次按订舱的登记：采购上下文不给，取委托的。
func bookingCommand(t *testing.T, reason domain.ChargeOccurrenceReason, objects ...string) application.RegisterChargeOccurrenceCommand {
	t.Helper()
	if len(objects) == 0 {
		objects = []string{trunkParcel}
	}
	return application.RegisterChargeOccurrenceCommand{
		TenantID:   occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Registrant: registrant,
		Reason:     reason,
		Booking:    bkBooking,
		Objects:    objects,
		Journey:    "SYN-JRN-" + trunkParcel,
	}
}

func registeredOccurrence(t *testing.T, result application.RegisterChargeOccurrenceResult) domain.TransportChargeOccurrence {
	t.Helper()
	if result.Outcome() != application.ChargeOccurrenceRegistered {
		t.Fatalf("outcome/detail = %q/%q, want REGISTERED", result.Outcome(), result.Detail())
	}
	record, present := result.Record()
	if !present {
		t.Fatal("已登记却不带记录")
	}
	return record.Occurrence
}

// Covers: AT-TF-206——订舱被承运方接受之后登订舱发生项：依据是那一份接受（订舱与答复两段），范围是那一次订舱，业务时间是
// 接受的答复时刻；采购上下文不给，服务方、协议快照与采购责任法人取自订舱所属的委托，按委托记下的那一版问 PC、在业务时间
// 核；协议按订舱计，数量 1、单位 BOOKING。
func TestAnAcceptedBookingRegistersABookingOccurrenceFromItsCommission(t *testing.T) {
	rig := acceptedBookingRig(t)
	occurrence := registeredOccurrence(t, rig.register(t, bookingCommand(t, domain.BookingOccurrence, trunkParcel, trunkSecondParcel)))
	quantity, unit := occurrence.Quantity()
	if occurrence.Reason() != domain.BookingOccurrence ||
		occurrence.FactBasis().String() != "CARRIER-ACCEPTANCE/"+bkBooking+"/"+bkAcceptance ||
		occurrence.Scope().String() != bkBooking ||
		!occurrence.OccurredAt().Equal(bkAcceptedAt) ||
		occurrence.Provider().String() != trunkProvider ||
		occurrence.Agreement().String() != trunkSnapshot ||
		occurrence.LegalEntity().String() != trunkLegalEntity ||
		quantity != 1 || unit.String() != "BOOKING" {
		t.Fatalf("订舱发生项 = %+v", occurrence)
	}
	if len(rig.agreements.asked) != 1 || rig.agreements.asked[0] != trunkAgreement+" "+trunkAgreementTag ||
		rig.agreements.provider[0] != trunkProvider || !rig.agreements.askedAt[0].Equal(bkAcceptedAt) {
		t.Fatalf("协议核对 = %v %v %v，want 委托记下的那一版、委托的服务方、答复时刻", rig.agreements.asked, rig.agreements.provider, rig.agreements.askedAt)
	}
	if len(rig.handoff.intents) != 1 {
		t.Fatalf("意图 %d 份，want 1", len(rig.handoff.intents))
	}

	// 同一订舱原样再登答已存在。
	if replay := rig.register(t, bookingCommand(t, domain.BookingOccurrence, trunkSecondParcel, trunkParcel)); replay.Outcome() != application.ChargeOccurrenceExisting {
		t.Fatalf("重登 outcome = %q", replay.Outcome())
	}

	// 按订舱计的一项盖住整次订舱：先登一件，再登另一件（成员不相交）答范围已计过，一次订舱只计一次。
	split := acceptedBookingRig(t)
	registeredOccurrence(t, split.register(t, bookingCommand(t, domain.BookingOccurrence, trunkParcel)))
	if again := split.register(t, bookingCommand(t, domain.BookingOccurrence, trunkSecondParcel)); again.Outcome() != application.ChargeOccurrenceConflict ||
		again.Detail() != application.ScopeAlreadyCharged {
		t.Fatalf("换成员 outcome/detail = %q/%q", again.Outcome(), again.Detail())
	}
}

// Covers: AT-TF-207——订舱的事实不成立各答一格未受理，读不动答未决，都不写：没有答复与只有候补是还没定局；拒绝、失效、
// 撤回是没成约；成员不在委托里；订舱不在册。
func TestABookingOccurrenceNeedsASettledAcceptanceOverCommissionMembers(t *testing.T) {
	booking := trunkBooking(t, bkRequestedAt, time.Time{})
	commission := trunkCommission(t, commissionShape{})
	waitlisted := trunkAnswer(t, booking, domain.BookingWaitlisted, bkAcceptedAt)
	refused := trunkAnswer(t, booking, domain.BookingRefused, bkAcceptedAt)
	withdrawn := trunkAnswer(t, booking, domain.BookingWithdrawn, bkAcceptedAt)
	accepted := trunkAnswer(t, booking, domain.BookingAccepted, bkAcceptedAt)
	for name, testCase := range map[string]struct {
		answer  *ports.BookingAnswerRecord
		objects []string
		mutate  func(*occurrenceRig)
		outcome application.ChargeOccurrenceOutcome
		detail  application.ChargeOccurrenceDetail
	}{
		"没有答复":    {nil, nil, nil, application.ChargeOccurrenceInputNotAccepted, application.BookingNotAnswered},
		"只有候补":    {&waitlisted, nil, nil, application.ChargeOccurrenceInputNotAccepted, application.BookingNotAnswered},
		"拒绝":      {&refused, nil, nil, application.ChargeOccurrenceInputNotAccepted, application.BookingNotAccepted},
		"撤回":      {&withdrawn, nil, nil, application.ChargeOccurrenceInputNotAccepted, application.BookingNotAccepted},
		"成员不在委托里": {&accepted, []string{"SYN-PARCEL-OUTSIDE"}, nil, application.ChargeOccurrenceInputNotAccepted, application.ObjectNotInCommission},
		"订舱不在册": {&accepted, nil, func(rig *occurrenceRig) { rig.bookings = bookingViewDouble{} },
			application.ChargeOccurrenceInputNotAccepted, application.ChargeFactNotFound},
		"订舱读不动": {&accepted, nil, func(rig *occurrenceRig) { rig.bookings = bookingViewDouble{err: errors.New("bookings down")} },
			application.ChargeOccurrenceUndecided, application.FactSourceUnavailable},
		"答复读不动": {&accepted, nil, func(rig *occurrenceRig) { rig.answers = bookingAnswerViewDouble{err: errors.New("answers down")} },
			application.ChargeOccurrenceUndecided, application.FactSourceUnavailable},
		"委托读不动": {&accepted, nil, func(rig *occurrenceRig) { rig.commissions = commissionViewDouble{err: errors.New("commissions down")} },
			application.ChargeOccurrenceUndecided, application.ProcurementSourceUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			rig := bookingRig(t, commission, booking, testCase.answer)
			if testCase.mutate != nil {
				testCase.mutate(rig)
			}
			result := rig.register(t, bookingCommand(t, domain.BookingOccurrence, testCase.objects...))
			if result.Outcome() != testCase.outcome || result.Detail() != testCase.detail {
				t.Fatalf("outcome/detail = %q/%q, want %q/%q", result.Outcome(), result.Detail(), testCase.outcome, testCase.detail)
			}
			if testCase.outcome == application.ChargeOccurrenceUndecided && result.Continuation() == "" {
				t.Fatal("未决不带续办引用")
			}
			if len(rig.agreements.asked) != 0 {
				t.Fatal("事实不成立不该去问协议")
			}
			assertNothingWritten(t, rig)
		})
	}

	// 订舱在册而委托读不回是数据违约：答「不在册」会让登记方去核一份确实存在的订舱，上抛。
	rig := bookingRig(t, commission, booking, &accepted)
	rig.commissions = commissionViewDouble{}
	if _, err := rig.handler().Register(t.Context(), bookingCommand(t, domain.BookingOccurrence)); !errors.Is(err, application.ErrBookingWithoutCommission) {
		t.Fatalf("err = %v, want ErrBookingWithoutCommission", err)
	}
	assertNothingWritten(t, rig)
}

// Covers: AT-TF-208——尚未开始范围的订舱取消形成取消发生项：依据是那一次订舱取消，范围是那一次订舱，业务时间是取消时刻；
// 同一订舱上先登了订舱发生项，取消发生项照样形成，两项各是各的事实，取消不删订舱那一项。
func TestACancellationOfAnUnstartedBookingRegistersBesideTheBookingOccurrence(t *testing.T) {
	booking := trunkBooking(t, bkRequestedAt, bkCancelledAt)
	answer := trunkAnswer(t, booking, domain.BookingAccepted, bkAcceptedAt)
	rig := bookingRig(t, trunkCommission(t, commissionShape{}), booking, &answer)

	booked := registeredOccurrence(t, rig.register(t, bookingCommand(t, domain.BookingOccurrence)))
	cancelled := registeredOccurrence(t, rig.register(t, bookingCommand(t, domain.CancellationOccurrence)))
	quantity, unit := cancelled.Quantity()
	if cancelled.Reason() != domain.CancellationOccurrence ||
		cancelled.FactBasis().String() != "BOOKING-CANCELLATION/"+bkBooking ||
		cancelled.Scope().String() != bkBooking ||
		!cancelled.OccurredAt().Equal(bkCancelledAt) ||
		cancelled.Occurrence() == booked.Occurrence() ||
		quantity != 1 || unit.String() != "BOOKING" {
		t.Fatalf("取消发生项 = %+v", cancelled)
	}
	if rig.registry.saves != 2 || len(rig.handoff.intents) != 2 {
		t.Fatalf("落 %d 项、交 %d 份，want 两项各一份", rig.registry.saves, len(rig.handoff.intents))
	}
}

// Covers: AT-TF-209——取消发生项只认尚未开始范围的取消，按业务时间判，与 ADR-0204 决定三同一边界：订舱没取消；取消那一刻还
// 没接受（接受晚于取消）；委托下的运输已经开始、这次订舱在开始之前提出、取消不早于开始（同一刻算开始在先），都答未受理。
// 开始之后才为剩余范围提出的订舱、或取消早于开始，照常形成。已取消而还没定局（没有答复或只有候补）答未定局，不答未被接受：
// 取消不封答复口，取消之前作出、之后送达的接受仍会落册。
func TestACancellationOccurrenceOnlyStopsAnUnstartedScopeByBusinessTime(t *testing.T) {
	startedAfterCancel := bkCancelledAt.Add(time.Hour)
	startedBeforeCancel := bkCancelledAt.Add(-30 * time.Minute)
	for name, testCase := range map[string]struct {
		requestedAt time.Time
		cancelledAt time.Time
		acceptedAt  time.Time
		startedAt   time.Time
		detail      application.ChargeOccurrenceDetail
	}{
		"未取消":    {bkRequestedAt, time.Time{}, bkAcceptedAt, time.Time{}, application.BookingNotCancelled},
		"接受晚于取消": {bkRequestedAt, bkCancelledAt, bkCancelledAt.Add(time.Minute), time.Time{}, application.BookingNotAccepted},
		"开始之前提出、开始之后取消":  {bkRequestedAt, bkCancelledAt, bkAcceptedAt, startedBeforeCancel, application.CancelledAfterTransportStart},
		"开始与取消同一刻":       {bkRequestedAt, bkCancelledAt, bkAcceptedAt, bkCancelledAt, application.CancelledAfterTransportStart},
		"取消早于开始":         {bkRequestedAt, bkCancelledAt, bkAcceptedAt, startedAfterCancel, application.ChargeOccurrenceDetailNone},
		"开始之后为剩余范围提出的订舱": {bkRequestedAt, bkCancelledAt, bkAcceptedAt, bkRequestedAt.Add(-time.Minute), application.ChargeOccurrenceDetailNone},
	} {
		t.Run(name, func(t *testing.T) {
			booking := trunkBooking(t, testCase.requestedAt, testCase.cancelledAt)
			answer := trunkAnswer(t, booking, domain.BookingAccepted, testCase.acceptedAt)
			rig := bookingRig(t, trunkCommission(t, commissionShape{startedAt: testCase.startedAt}), booking, &answer)
			result := rig.register(t, bookingCommand(t, domain.CancellationOccurrence))
			if testCase.detail == application.ChargeOccurrenceDetailNone {
				registeredOccurrence(t, result)
				return
			}
			if result.Outcome() != application.ChargeOccurrenceInputNotAccepted || result.Detail() != testCase.detail {
				t.Fatalf("outcome/detail = %q/%q, want INPUT_NOT_ACCEPTED/%q", result.Outcome(), result.Detail(), testCase.detail)
			}
			assertNothingWritten(t, rig)
		})
	}

	cancelled := trunkBooking(t, bkRequestedAt, bkCancelledAt)
	waitlisted := trunkAnswer(t, cancelled, domain.BookingWaitlisted, bkAcceptedAt)
	for name, answer := range map[string]*ports.BookingAnswerRecord{"已取消、没有答复": nil, "已取消、只有候补": &waitlisted} {
		t.Run(name, func(t *testing.T) {
			rig := bookingRig(t, trunkCommission(t, commissionShape{}), cancelled, answer)
			result := rig.register(t, bookingCommand(t, domain.CancellationOccurrence))
			if result.Outcome() != application.ChargeOccurrenceInputNotAccepted || result.Detail() != application.BookingNotAnswered {
				t.Fatalf("outcome/detail = %q/%q, want INPUT_NOT_ACCEPTED/%q", result.Outcome(), result.Detail(), application.BookingNotAnswered)
			}
			assertNothingWritten(t, rig)
		})
	}
}

// Covers: AT-TF-210——订舱属于委托，采购上下文取委托的：登记方给的服务方、协议对象与版本与委托一致照常登记；任一格不一致、
// 或声明自营，答冲突、细格「采购上下文与委托不一致」，回显那份委托，不问协议、不写。
func TestADeclaredProcurementContextMustAgreeWithTheBookingsCommission(t *testing.T) {
	for name, testCase := range map[string]struct {
		mutate   func(*application.RegisterChargeOccurrenceCommand)
		conflict bool
	}{
		"三格都一致": {func(command *application.RegisterChargeOccurrenceCommand) {
			command.Provider, command.AgreementObject, command.AgreementVersion = trunkProvider, trunkAgreement, trunkAgreementTag
		}, false},
		"服务方不同": {func(command *application.RegisterChargeOccurrenceCommand) {
			command.Provider = "SYN-PARTY-CARRIER-LM-01"
		}, true},
		"版本不同": {func(command *application.RegisterChargeOccurrenceCommand) {
			command.AgreementObject, command.AgreementVersion = trunkAgreement, "v1"
		}, true},
		"声明自营": {func(command *application.RegisterChargeOccurrenceCommand) {
			command.SelfOperated = true
		}, true},
	} {
		t.Run(name, func(t *testing.T) {
			rig := acceptedBookingRig(t)
			command := bookingCommand(t, domain.BookingOccurrence)
			testCase.mutate(&command)
			result := rig.register(t, command)
			if !testCase.conflict {
				registeredOccurrence(t, result)
				return
			}
			echoed, present := result.Commission()
			if result.Outcome() != application.ChargeOccurrenceConflict || result.Detail() != application.ProcurementContextConflict ||
				!present || echoed.Commission.Commission().String() != bkCommission {
				t.Fatalf("outcome/detail = %q/%q，回显委托 %v", result.Outcome(), result.Detail(), present)
			}
			if len(rig.agreements.asked) != 0 {
				t.Fatal("冲突不该去问协议")
			}
			assertNothingWritten(t, rig)
		})
	}
}

// adoptedTrunkRig 立一台实际履约夹具：干线段的参与已结束，段已采用干线委托（记下了 v2）。
func adoptedTrunkRig(t *testing.T, commission ports.TransportCommissionRecord) *occurrenceRig {
	t.Helper()
	rig := newOccurrenceRig(t)
	tenant := occurrenceValue(t, domain.NewTenantID, occurrenceTenant)
	segment := occurrenceValue(t, domain.NewFulfillmentSegmentReference, trunkSegment)
	adoption, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID:   tenant,
		Segment:    segment,
		Commission: commission.Key.Commission,
		Provider:   occurrenceValue(t, domain.NewServiceProviderReference, trunkProvider),
		StartBasis: occurrenceValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SZX-"+trunkParcel),
		StartedAt:  occurrenceEnteredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	rig.adoptions = adoptionViewDouble{records: map[domain.FulfillmentSegmentReference]ports.SegmentCommissionAdoptionRecord{
		segment: {Key: ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: segment}, Adoption: adoption},
	}}
	rig.commissions = commissionViewDouble{records: map[domain.TransportCommissionReference]ports.TransportCommissionRecord{
		commission.Key.Commission: commission,
	}}
	return rig
}

// Covers: AT-TF-211——实际履约段已采用委托时，登记方可以省略采购上下文，服务方、协议与法人取委托快照；给了而不一致答冲突并
// 回显委托；一致照常。段采用了委托就不问实际承运商判断：成本来源跟着采购走，承运商判断即便判成自营法人也不答不适用。没有
// 采用连线的段照旧要登记方给出，缺了答缺采购上下文。
func TestAnAdoptedSegmentTakesItsProcurementContextFromTheCommission(t *testing.T) {
	commission := trunkCommission(t, commissionShape{startedAt: occurrenceEnteredAt})
	omitted := actualFulfillmentCommand(t)
	omitted.Provider, omitted.AgreementObject, omitted.AgreementVersion = "", "", ""

	rig := adoptedTrunkRig(t, commission)
	rig.judgments = judgedCarrier(t, domain.OwnOperatingLegalEntity, trunkLegalEntity)
	occurrence := registeredOccurrence(t, rig.register(t, omitted))
	if occurrence.Provider().String() != trunkProvider || occurrence.Agreement().String() != trunkSnapshot ||
		occurrence.LegalEntity().String() != trunkLegalEntity || !occurrence.OccurredAt().Equal(occurrenceEndedAt) {
		t.Fatalf("从委托取的采购上下文 = %+v", occurrence)
	}
	if len(rig.agreements.asked) != 1 || rig.agreements.asked[0] != trunkAgreement+" "+trunkAgreementTag ||
		!rig.agreements.askedAt[0].Equal(occurrenceEndedAt) {
		t.Fatalf("协议核对 = %v %v", rig.agreements.asked, rig.agreements.askedAt)
	}

	given := adoptedTrunkRig(t, commission)
	registeredOccurrence(t, given.register(t, actualFulfillmentCommand(t)))

	conflicting := adoptedTrunkRig(t, commission)
	other := actualFulfillmentCommand(t)
	other.Provider = "SYN-PARTY-CARRIER-LM-01"
	result := conflicting.register(t, other)
	if echoed, present := result.Commission(); result.Outcome() != application.ChargeOccurrenceConflict ||
		result.Detail() != application.ProcurementContextConflict || !present || echoed.Commission.Provider().String() != trunkProvider {
		t.Fatalf("不一致 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
	assertNothingWritten(t, conflicting)

	unadopted := newOccurrenceRig(t)
	if result := unadopted.register(t, omitted); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
		result.Detail() != application.ProcurementContextMissing {
		t.Fatalf("没有连线 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
}

// Covers: AT-TF-212——采购上下文取自委托时，协议照旧在业务时间核、照那一版的声明判条件：委托提交时协议有效、业务发生时已
// 不支持采购答未生效；那一版没声明订舱答未配置并带回那一版，都不写。
func TestACommissionsAgreementIsCheckedAtBusinessTimeAndByItsDeclaredConditions(t *testing.T) {
	rig := acceptedBookingRig(t)
	rig.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementNotEffective}
	if result := rig.register(t, bookingCommand(t, domain.BookingOccurrence)); result.Outcome() != application.ChargeOccurrenceAgreementNotEffective {
		t.Fatalf("业务时间已不生效 outcome = %q", result.Outcome())
	}
	assertNothingWritten(t, rig)

	undeclared := acceptedBookingRig(t)
	undeclared.agreements.check.ChargeConditions = declaredConditions(t,
		declaredCondition{domain.ActualFulfillmentOccurrence, true, domain.PerPieceQuantity})
	result := undeclared.register(t, bookingCommand(t, domain.BookingOccurrence))
	if result.Outcome() != application.ChargeOccurrenceNotConfigured || result.Agreement() != trunkSnapshot {
		t.Fatalf("没声明订舱 outcome/agreement = %q/%q", result.Outcome(), result.Agreement())
	}
	assertNothingWritten(t, undeclared)
}

// Covers: AT-TF-213——委托没记下协议对象与版本（迁移 TF 0038 之前提交的）：服务方仍取委托的，协议要登记方给出，缺了答缺
// 采购上下文；给出而核回的快照不是委托上那一份、或那一版不在册、或约定的供应商不是委托的服务方，都答冲突回显委托；未生效
// 照协议否定格答；是同一份照常登记。委托记着那一版时，核回别的快照、不在册或供应商不符都是两边数据走岔了，上抛。
func TestALegacyCommissionNeedsTheRegistrantToNameItsAgreement(t *testing.T) {
	booking := trunkBooking(t, bkRequestedAt, time.Time{})
	answer := trunkAnswer(t, booking, domain.BookingAccepted, bkAcceptedAt)
	legacy := trunkCommission(t, commissionShape{legacy: true})

	missing := bookingRig(t, legacy, booking, &answer)
	if result := missing.register(t, bookingCommand(t, domain.BookingOccurrence)); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
		result.Detail() != application.ProcurementContextMissing {
		t.Fatalf("缺协议 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}

	named := bookingCommand(t, domain.BookingOccurrence)
	named.AgreementObject, named.AgreementVersion = trunkAgreement, trunkAgreementTag
	registeredOccurrence(t, bookingRig(t, legacy, booking, &answer).register(t, named))

	otherVersion := bookingRig(t, legacy, booking, &answer)
	otherVersion.agreements.check.Snapshot = occurrenceValue(t, domain.NewAgreementSnapshotReference, trunkAgreement+"/v1")
	named.AgreementVersion = "v1"
	if result := otherVersion.register(t, named); result.Outcome() != application.ChargeOccurrenceConflict ||
		result.Detail() != application.ProcurementContextConflict {
		t.Fatalf("另一版 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
	assertNothingWritten(t, otherVersion)

	for standing, want := range map[ports.SupplierAgreementStanding]application.ChargeOccurrenceOutcome{
		ports.SupplierAgreementNotRegistered:    application.ChargeOccurrenceConflict,
		ports.SupplierAgreementProviderMismatch: application.ChargeOccurrenceConflict,
		ports.SupplierAgreementNotEffective:     application.ChargeOccurrenceAgreementNotEffective,
	} {
		t.Run("报的那一版答 "+standing.String(), func(t *testing.T) {
			rig := bookingRig(t, legacy, booking, &answer)
			rig.agreements.check = ports.SupplierAgreementCheck{Standing: standing}
			result := rig.register(t, named)
			if result.Outcome() != want {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), want)
			}
			if want == application.ChargeOccurrenceConflict {
				if result.Detail() != application.ProcurementContextConflict {
					t.Fatalf("detail = %q", result.Detail())
				}
				if echoed, ok := result.Commission(); !ok || echoed.Key != legacy.Key {
					t.Fatalf("冲突没回显委托：%+v/%v", echoed.Key, ok)
				}
			}
			assertNothingWritten(t, rig)
		})
	}

	for _, standing := range []ports.SupplierAgreementStanding{
		ports.SupplierAgreementNotRegistered, ports.SupplierAgreementProviderMismatch,
	} {
		rig := acceptedBookingRig(t)
		rig.agreements.check = ports.SupplierAgreementCheck{Standing: standing}
		if _, err := rig.handler().Register(t.Context(), bookingCommand(t, domain.BookingOccurrence)); err == nil ||
			!strings.Contains(err.Error(), standing.String()) {
			t.Fatalf("委托记着的那一版答 %s err = %v，want 上抛", standing, err)
		}
		assertNothingWritten(t, rig)
	}

	drifted := acceptedBookingRig(t)
	drifted.agreements.check.Snapshot = occurrenceValue(t, domain.NewAgreementSnapshotReference, trunkAgreement+"/v9")
	if _, err := drifted.handler().Register(t.Context(), bookingCommand(t, domain.BookingOccurrence)); err == nil ||
		!strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("快照走岔 err = %v，want 上抛", err)
	}
	assertNothingWritten(t, drifted)
}

// Covers: AT-TF-214——采用连线或委托读不动答未决、带续办引用；连线指名的委托读不回是数据违约，上抛；都不写。
func TestAnUnreadableAdoptionOrCommissionIsUndecided(t *testing.T) {
	commission := trunkCommission(t, commissionShape{startedAt: occurrenceEnteredAt})
	for name, mutate := range map[string]func(*occurrenceRig){
		"连线读不动": func(rig *occurrenceRig) { rig.adoptions.err = errors.New("adoptions down") },
		"委托读不动": func(rig *occurrenceRig) { rig.commissions.err = errors.New("commissions down") },
	} {
		t.Run(name, func(t *testing.T) {
			rig := adoptedTrunkRig(t, commission)
			mutate(rig)
			result := rig.register(t, actualFulfillmentCommand(t))
			if result.Outcome() != application.ChargeOccurrenceUndecided || result.Detail() != application.ProcurementSourceUnavailable ||
				result.Continuation() == "" {
				t.Fatalf("outcome/detail = %q/%q", result.Outcome(), result.Detail())
			}
			assertNothingWritten(t, rig)
		})
	}

	rig := adoptedTrunkRig(t, commission)
	rig.commissions = commissionViewDouble{}
	if _, err := rig.handler().Register(t.Context(), actualFulfillmentCommand(t)); !errors.Is(err, application.ErrAdoptionWithoutCommission) {
		t.Fatalf("err = %v, want ErrAdoptionWithoutCommission", err)
	}
	assertNothingWritten(t, rig)
}
