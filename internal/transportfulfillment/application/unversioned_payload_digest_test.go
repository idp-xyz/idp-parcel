package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// unversionedDigestCase 是一口在 TFC-1 之前入库的一条记录。first 首次提交并交回替身里那条记录的
// 摘要与改写它的办法；随后把摘要改写成 unversioned，再走 replay（同内容，必须答已有结果）与
// conflict（同身份异内容，必须答内容冲突）。三步共用一个替身。
//
// unversioned 是基 1121ba61 上的旧代码对 first 那条命令落下的摘要，写成定值而不在这里现算：
// 拿恢复后的函数现算，恢复得不一致也照样绿。
type unversionedDigestCase struct {
	name        string
	unversioned string
	first       func(t *testing.T) (string, func(string))
	replay      func(t *testing.T)
	conflict    func(t *testing.T)
}

// onlyDigest 交回替身里唯一那条记录的摘要，与把它改写成别的串的办法。
func onlyDigest[K comparable, V any](t *testing.T, records map[K]V, digest func(*V) *string) (string, func(string)) {
	t.Helper()
	if len(records) != 1 {
		t.Fatalf("替身里有 %d 条记录，want 1", len(records))
	}
	for key, record := range records {
		return *digest(&record), func(rewritten string) {
			*digest(&record) = rewritten
			records[key] = record
		}
	}
	return "", nil
}

// chainTailDigest 交回场外揽收登记册里唯一那条版本链链尾（当前版）的摘要，与改写它的办法。
func chainTailDigest(t *testing.T, registry *pickupRegistryDouble) (string, func(string)) {
	t.Helper()
	if len(registry.records) != 1 {
		t.Fatalf("登记册里有 %d 条版本链，want 1", len(registry.records))
	}
	for key, chain := range registry.records {
		tail := len(chain) - 1
		return chain[tail].ContentDigest, func(rewritten string) {
			registry.records[key][tail].ContentDigest = rewritten
		}
	}
	return "", nil
}

func wantOutcome[O comparable](t *testing.T, got O, err error, want O) {
	t.Helper()
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got != want {
		t.Fatalf("outcome = %v, want %v", got, want)
	}
}

func unversionedDigestCases(t *testing.T) []unversionedDigestCase {
	t.Helper()
	return []unversionedDigestCase{
		dispositionUnversionedCase(t),
		commissionUnversionedCase(t),
		bookingUnversionedCase(t),
		bookingAnswerUnversionedCase(t),
		pickupAttemptUnversionedCase(t),
		scheduleUnversionedCase(t),
		poolUnversionedCase(t),
		effectiveDeliveryUnversionedCase(t),
		pickupRegistrationUnversionedCase(t),
		pickupCorrectionUnversionedCase(t),
		handoverUnversionedCase(t),
		journeyUnversionedCase(t),
	}
}

// Covers: ADR-0014「规范化版本不同不是冲突」——TFC-1 之前入库的记录存的是无版本摘要，同一条命令
// 重放按无版本那一版重算再比，答已有结果；同身份换内容仍是内容冲突。比摘要的定形口逐口走一遍；
// 交接更正口不比摘要（重放与撞键都走版本键），不在此列。
func TestRecordsStoredBeforeTFC1AreComparedUnderTheirOwnShape(t *testing.T) {
	for _, testCase := range unversionedDigestCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			_, rewrite := testCase.first(t)
			rewrite(testCase.unversioned)
			testCase.replay(t)
			testCase.conflict(t)
		})
	}
}

// Covers: 认不出的形状版本不答冲突——按该处读失败的既有答复作答（委托口是存储未决）。
func TestAnUnknownStoredShapeIsUndecidedNotAConflict(t *testing.T) {
	fixture := newCommissionFixture(t)
	command := submitCommissionCommand(t)
	first, err := fixture.handler.SubmitCommission(context.Background(), command)
	wantOutcome(t, first.Outcome(), err, application.CommissionSubmitted)
	_, rewrite := onlyDigest(t, fixture.commissions.records,
		func(record *ports.TransportCommissionRecord) *string { return &record.ContentDigest })
	rewrite("TFC-9:0000000000000000000000000000000000000000000000000000000000000000")

	result, err := fixture.handler.SubmitCommission(context.Background(), command)
	wantOutcome(t, result.Outcome(), err, application.CommissionUndecided)
	if result.UndecidedReason() != application.CommissionStoreUnavailable {
		t.Fatalf("reason = %q", result.UndecidedReason())
	}
}

func dispositionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newAcceptDispositionFixture(t)
	command := acceptDispositionCommand(t)
	handle := func(t *testing.T, command application.AcceptRegulatoryDispositionCommand, want application.AcceptDispositionOutcome) {
		t.Helper()
		result, err := fixture.handler.Handle(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "ACCEPT_REGULATORY_DISPOSITION",
		unversioned: "1c3e3a3f3209f1b3",
		first: func(t *testing.T) (string, func(string)) {
			handle(t, command, application.DispositionDecided)
			return onlyDigest(t, fixture.acceptances.records,
				func(record *ports.DispositionAcceptanceRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { handle(t, command, application.DispositionExistingDecision) },
		conflict: func(t *testing.T) {
			changed := acceptDispositionCommand(t)
			changed.MovementAuthority = "movement-authority-2"
			handle(t, changed, application.DispositionDecisionConflict)
		},
	}
}

func commissionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newCommissionFixture(t)
	command := submitCommissionCommand(t)
	submit := func(t *testing.T, command application.SubmitCommissionCommand, want application.CommissionOutcome) {
		t.Helper()
		result, err := fixture.handler.SubmitCommission(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "SUBMIT_COMMISSION",
		unversioned: "afc54c1c10d616e555bf2ca66e3b12b053c22edf229898c6a56b00009830be79",
		first: func(t *testing.T) (string, func(string)) {
			submit(t, command, application.CommissionSubmitted)
			return onlyDigest(t, fixture.commissions.records,
				func(record *ports.TransportCommissionRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { submit(t, command, application.CommissionExistingResult) },
		conflict: func(t *testing.T) {
			changed := submitCommissionCommand(t)
			changed.Agreement = "agreement-snapshot-2"
			submit(t, changed, application.CommissionConflict)
		},
	}
}

func bookingUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newCommissionFixture(t)
	command := submitBookingCommand(t)
	submit := func(t *testing.T, command application.SubmitBookingCommand, want application.CommissionOutcome) {
		t.Helper()
		result, err := fixture.handler.SubmitBooking(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "SUBMIT_BOOKING",
		unversioned: "976bf4be9af7daa957ed89ba0a928effb1a58a926bc6e41bc04a0e011a2dd555",
		first: func(t *testing.T) (string, func(string)) {
			submit(t, command, application.BookingSubmitted)
			return onlyDigest(t, fixture.bookings.records,
				func(record *ports.BookingRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { submit(t, command, application.BookingExistingResult) },
		conflict: func(t *testing.T) {
			changed := submitBookingCommand(t)
			changed.Quantity = 120
			submit(t, changed, application.BookingConflict)
		},
	}
}

func bookingAnswerUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newCommissionFixture(t)
	command := answerBookingCommand(t, domain.BookingAccepted)
	answer := func(t *testing.T, command application.AnswerBookingCommand, want application.CommissionOutcome) {
		t.Helper()
		result, err := fixture.handler.AnswerBooking(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "ANSWER_BOOKING",
		unversioned: "26681fe16aea102b12f9887e6bf7b60cd4511678da29104d245506cfab4b0b0e",
		first: func(t *testing.T) (string, func(string)) {
			submitted, err := fixture.handler.SubmitBooking(context.Background(), submitBookingCommand(t))
			wantOutcome(t, submitted.Outcome(), err, application.BookingSubmitted)
			answer(t, command, application.BookingAnswered)
			return onlyDigest(t, fixture.answers.records,
				func(record *ports.BookingAnswerRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { answer(t, command, application.BookingAnswerExists) },
		conflict: func(t *testing.T) {
			answer(t, answerBookingCommand(t, domain.BookingRefused), application.BookingAnswerConflict)
		},
	}
}

func pickupAttemptUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newPickupFixture(t)
	command := pickupCommand(t, "source-legacy-1", "attempt-1", successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"))
	handle := func(t *testing.T, command application.PerformOffsitePickupCommand, want application.PickupOutcome) {
		t.Helper()
		result, err := fixture.handler.Handle(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "PERFORM_OFFSITE_PICKUP",
		unversioned: "980d428f613d924d9b47c8a2f6ba580a73842a5c07cc3e20e21cc1f73158a481",
		first: func(t *testing.T) (string, func(string)) {
			handle(t, command, application.PickupAttemptRecorded)
			return onlyDigest(t, fixture.store.records,
				func(record *ports.PickupAttemptRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { handle(t, command, application.PickupExistingResult) },
		conflict: func(t *testing.T) {
			changed := pickupCommand(t, "source-legacy-1", "attempt-1", successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-9"))
			handle(t, changed, application.PickupSourceConflict)
		},
	}
}

func scheduleUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newOpportunityFixture(t)
	command := application.EstablishScheduleCommand{
		TenantID:  opportunityTenant(t),
		Schedule:  "schedule-1",
		Direction: "US-WEST/EXPORT",
		DepartsAt: scheduleDepartsAt,
	}
	establish := func(t *testing.T, command application.EstablishScheduleCommand, want application.OpportunityOutcome) {
		t.Helper()
		result, err := fixture.handler.EstablishSchedule(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "ESTABLISH_SCHEDULE",
		unversioned: "19878af2c3c7559a4d5bd400a42c196c81e4013b26b8036b06d6543a4bab74b4",
		first: func(t *testing.T) (string, func(string)) {
			establish(t, command, application.ScheduleEstablished)
			return onlyDigest(t, fixture.schedules.records,
				func(record *ports.ScheduleRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { establish(t, command, application.ScheduleExistingResult) },
		conflict: func(t *testing.T) {
			changed := command
			changed.Direction = "US-EAST/EXPORT"
			establish(t, changed, application.ScheduleConflict)
		},
	}
}

func poolUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newOpportunityFixture(t)
	command := application.EstablishPoolCommand{
		TenantID: opportunityTenant(t),
		Pool:     "pool-1",
		Schedule: "schedule-1",
		Unit:     "kg",
		Capacity: 100,
	}
	establish := func(t *testing.T, command application.EstablishPoolCommand, want application.OpportunityOutcome) {
		t.Helper()
		result, err := fixture.handler.EstablishPool(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "ESTABLISH_POOL",
		unversioned: "1ef67c6d60b18c39b3a7b8ffbc0fe3d5a15596028de121783485a96e9692101e",
		first: func(t *testing.T) (string, func(string)) {
			establish(t, command, application.PoolEstablished)
			return onlyDigest(t, fixture.pools.records,
				func(record *ports.CapacityPoolRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { establish(t, command, application.PoolExistingResult) },
		conflict: func(t *testing.T) {
			changed := command
			changed.Capacity = 120
			establish(t, changed, application.PoolConflict)
		},
	}
}

func effectiveDeliveryUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newDeliveryFixture(t)
	command := registerCommand(t)
	register := func(t *testing.T, command application.RegisterEffectiveDeliveryCommand, want application.DeliveryRegistrationOutcome) {
		t.Helper()
		result, err := fixture.handler.Register(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "REGISTER_EFFECTIVE_DELIVERY",
		unversioned: "2f13a0ca2853553e65d6f1b41deec828518640d937016af03f23be8610ef2960",
		first: func(t *testing.T) (string, func(string)) {
			register(t, command, application.DeliveryRegistered)
			return onlyDigest(t, fixture.store.records,
				func(record *ports.EffectiveDeliveryRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { register(t, command, application.DeliveryExistingVersion) },
		conflict: func(t *testing.T) {
			changed := registerCommand(t)
			changed.Proof = "pod-2"
			register(t, changed, application.DeliveryRegistrationConflict)
		},
	}
}

func pickupRegistrationUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newPickupRegFixture(t)
	command := pickupRegistrationCommand(t)
	register := func(t *testing.T, command application.RegisterOffsitePickupCommand, want application.PickupRegistrationOutcome) {
		t.Helper()
		result, err := fixture.handler.Register(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "REGISTER_OFFSITE_PICKUP",
		unversioned: "6034f9554e87ee4cba5de2293ae7295275bcb0445f3b51d3d20cf2a063e398ab",
		first: func(t *testing.T) (string, func(string)) {
			register(t, command, application.PickupRegistered)
			return chainTailDigest(t, fixture.registry)
		},
		replay: func(t *testing.T) { register(t, command, application.PickupExistingVersion) },
		conflict: func(t *testing.T) {
			changed := pickupRegistrationCommand(t)
			changed.Control = "TRANSPORT-CONTROL/TF-9"
			register(t, changed, application.PickupRegistrationConflict)
		},
	}
}

// pickupCorrectionUnversionedCase 比的是当前版：同一份更正再来一次时，前版已被它更正过，当前版的
// 内容等于这份更正才是重放。
func pickupCorrectionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newPickupRegFixture(t)
	command := pickupCorrectionCommand(t, "pickup-result/v1")
	correct := func(t *testing.T, command application.CorrectOffsitePickupCommand, want application.PickupRegistrationOutcome) {
		t.Helper()
		result, err := fixture.handler.Correct(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "CORRECT_OFFSITE_PICKUP",
		unversioned: "898262807e242f51ca293ecf54c49f8691d0bef12ccb728a895d53121da5dcf0",
		first: func(t *testing.T) (string, func(string)) {
			seedRegisteredPickup(t, fixture)
			correct(t, command, application.PickupCorrected)
			return chainTailDigest(t, fixture.registry)
		},
		replay: func(t *testing.T) { correct(t, command, application.PickupExistingVersion) },
		conflict: func(t *testing.T) {
			changed := pickupCorrectionCommand(t, "pickup-result/v1")
			changed.Control = "TRANSPORT-CONTROL/TF-1-THIRD-OPINION"
			correct(t, changed, application.PickupRegistrationConflict)
		},
	}
}

func handoverUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newHandoverFixture(t)
	command := registerHandoverCommand(t)
	register := func(t *testing.T, command application.RegisterTransportHandoverCommand, want application.HandoverRegistrationOutcome) {
		t.Helper()
		result, err := fixture.handler.Register(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "REGISTER_TRANSPORT_HANDOVER",
		unversioned: "ce748b001b582166475c82743ba05fa163afaefd439dcc396c8a3f3f84281cac",
		first: func(t *testing.T) (string, func(string)) {
			register(t, command, application.HandoverRegistered)
			return onlyDigest(t, fixture.registry.records,
				func(record *ports.TransportHandoverRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { register(t, command, application.HandoverExistingVersion) },
		conflict: func(t *testing.T) {
			changed := registerHandoverCommand(t)
			changed.ReleasingEvidence = "evidence-release-2"
			register(t, changed, application.HandoverRegistrationConflict)
		},
	}
}

func journeyUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newJourneyFixture(t)
	command := startJourneyCommand(t, domain.RegulatoryDispositionDecision)
	start := func(t *testing.T, command application.StartAlternateJourneyCommand, want application.JourneyStartOutcome) {
		t.Helper()
		result, err := fixture.handler.Handle(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "START_ALTERNATE_JOURNEY",
		unversioned: "ccb86db61dd99d1eeb9b9a3018e1ed2ec246481f9777f402929b01bdd24d4d25",
		first: func(t *testing.T) (string, func(string)) {
			start(t, command, application.JourneyStarted)
			return onlyDigest(t, fixture.store.records,
				func(record *ports.AlternateJourneyRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { start(t, command, application.JourneyExistingResult) },
		conflict: func(t *testing.T) {
			changed := startJourneyCommand(t, domain.RegulatoryDispositionDecision)
			changed.Journey = "journey-return-2"
			start(t, changed, application.JourneyStartConflict)
		},
	}
}
