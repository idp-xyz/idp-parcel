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

var (
	commissionSentAt = time.Date(2026, 8, 13, 15, 0, 0, 0, time.UTC)
	bookingSentAt    = time.Date(2026, 8, 13, 15, 30, 0, 0, time.UTC)
	commissionNowAt  = time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)
)

type commissionStoreDouble struct {
	records map[string]ports.TransportCommissionRecord
	findErr error
	saves   int
}

func newCommissionStore() *commissionStoreDouble {
	return &commissionStoreDouble{records: map[string]ports.TransportCommissionRecord{}}
}

func commissionStoreKey(key ports.TransportCommissionKey) string {
	return key.TenantID.String() + "|" + key.Commission.String()
}

func (double *commissionStoreDouble) FindByKey(
	_ context.Context,
	key ports.TransportCommissionKey,
) (ports.TransportCommissionRecord, bool, error) {
	if double.findErr != nil {
		return ports.TransportCommissionRecord{}, false, double.findErr
	}
	record, found := double.records[commissionStoreKey(key)]
	return record, found, nil
}

func (double *commissionStoreDouble) Save(
	_ context.Context,
	record ports.TransportCommissionRecord,
) (ports.CommissionSaveOutcome, error) {
	double.saves++
	if _, exists := double.records[commissionStoreKey(record.Key)]; exists {
		return ports.CommissionAlreadyRegistered, nil
	}
	double.records[commissionStoreKey(record.Key)] = record
	return ports.CommissionSaved, nil
}

func (double *commissionStoreDouble) Replace(
	_ context.Context,
	record ports.TransportCommissionRecord,
) (bool, error) {
	if _, exists := double.records[commissionStoreKey(record.Key)]; !exists {
		return false, nil
	}
	double.records[commissionStoreKey(record.Key)] = record
	return true, nil
}

type bookingStoreDouble struct {
	records map[string]ports.BookingRecord
	findErr error
}

func newBookingStore() *bookingStoreDouble {
	return &bookingStoreDouble{records: map[string]ports.BookingRecord{}}
}

func bookingStoreKey(key ports.BookingKey) string {
	return key.TenantID.String() + "|" + key.Booking.String()
}

func (double *bookingStoreDouble) FindByKey(
	_ context.Context,
	key ports.BookingKey,
) (ports.BookingRecord, bool, error) {
	if double.findErr != nil {
		return ports.BookingRecord{}, false, double.findErr
	}
	record, found := double.records[bookingStoreKey(key)]
	return record, found, nil
}

func (double *bookingStoreDouble) Save(
	_ context.Context,
	record ports.BookingRecord,
) (ports.BookingSaveOutcome, error) {
	if _, exists := double.records[bookingStoreKey(record.Key)]; exists {
		return ports.BookingAlreadyRegistered, nil
	}
	double.records[bookingStoreKey(record.Key)] = record
	return ports.BookingSaved, nil
}

type answerStoreDouble struct {
	records map[string]ports.BookingAnswerRecord
	findErr error
	saves   int
}

func newAnswerStore() *answerStoreDouble {
	return &answerStoreDouble{records: map[string]ports.BookingAnswerRecord{}}
}

func (double *answerStoreDouble) FindByKey(
	_ context.Context,
	key ports.BookingKey,
) (ports.BookingAnswerRecord, bool, error) {
	if double.findErr != nil {
		return ports.BookingAnswerRecord{}, false, double.findErr
	}
	record, found := double.records[bookingStoreKey(key)]
	return record, found, nil
}

func (double *answerStoreDouble) Save(
	_ context.Context,
	record ports.BookingAnswerRecord,
) (ports.BookingAnswerSaveOutcome, error) {
	double.saves++
	if _, exists := double.records[bookingStoreKey(record.Key)]; exists {
		return ports.BookingAlreadyAnswered, nil
	}
	double.records[bookingStoreKey(record.Key)] = record
	return ports.BookingAnswerSaved, nil
}

type commissionHandoffDouble struct {
	intents []ports.TransportCommissionIntent
	err     error
}

func (double *commissionHandoffDouble) HandOffTransportCommission(
	_ context.Context,
	intent ports.TransportCommissionIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type commissionClock struct{ at time.Time }

func (clock commissionClock) Now() time.Time { return clock.at }

type commissionFixture struct {
	commissions *commissionStoreDouble
	bookings    *bookingStoreDouble
	answers     *answerStoreDouble
	handoff     *commissionHandoffDouble
	handler     *application.CommissionTransportHandler
}

func newCommissionFixture(t *testing.T) *commissionFixture {
	t.Helper()
	fixture := &commissionFixture{
		commissions: newCommissionStore(),
		bookings:    newBookingStore(),
		answers:     newAnswerStore(),
		handoff:     &commissionHandoffDouble{},
	}
	fixture.handler = application.NewCommissionTransportHandler(application.CommissionTransportDeps{
		Commissions: fixture.commissions,
		Bookings:    fixture.bookings,
		Answers:     fixture.answers,
		Downstream:  fixture.handoff,
		Clock:       commissionClock{at: commissionNowAt},
	})
	return fixture
}

func submitCommissionCommand(t *testing.T) application.SubmitCommissionCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.SubmitCommissionCommand{
		TenantID:       tenant,
		Commission:     "commission-1",
		Provider:       "partner-1",
		Agreement:      "agreement-snapshot-1",
		Conditions:     "conditions-snapshot-1",
		Role:           "role-snapshot-1",
		Responsibility: "responsibility-snapshot-1",
		Members:        []string{"parcel-1"},
		SubmittedAt:    commissionSentAt,
	}
}

func submitBookingCommand(t *testing.T) application.SubmitBookingCommand {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	return application.SubmitBookingCommand{
		TenantID:    tenant,
		Booking:     "booking-1",
		Commission:  "commission-1",
		Quantity:    100,
		Unit:        "kg",
		RequestedAt: bookingSentAt,
	}
}

func answerBookingCommand(t *testing.T, outcome domain.CarrierAcceptanceOutcome) application.AnswerBookingCommand {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	command := application.AnswerBookingCommand{
		TenantID:   tenant,
		Booking:    "booking-1",
		Acceptance: "acceptance-1",
		Outcome:    outcome,
		DecidedAt:  bookingSentAt.Add(time.Hour),
	}
	if outcome == domain.BookingAccepted {
		command.Quantity = 80
	} else {
		command.Basis = "basis-" + outcome.String()
	}
	return command
}

// Covers: CONTEXT 243「保存……协议、条件、角色和责任依据快照，不修改商业版本」的
// 编排面（UC-TF-003 预定链前半）——四快照冻结随记录保全；同一委托标识只提交一次，
// 重放返原、异快照同键冲突；意图交 SA 成本预期源（UC-SA-002 输入行的协议快照来源）。
func TestACommissionSubmitsOnceWithItsSnapshots(t *testing.T) {
	fixture := newCommissionFixture(t)
	command := submitCommissionCommand(t)

	first, err := fixture.handler.SubmitCommission(context.Background(), command)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if first.Outcome() != application.CommissionSubmitted {
		t.Fatalf("outcome = %q, want COMMISSION_SUBMITTED", first.Outcome())
	}
	record, _ := first.Commission()
	if record.Commission.Agreement().String() != "agreement-snapshot-1" {
		t.Fatal("协议快照没有随委托保全")
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1（SA 成本预期源）", len(fixture.handoff.intents))
	}

	replay, err := fixture.handler.SubmitCommission(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.CommissionExistingResult || fixture.commissions.saves != 1 {
		t.Fatalf("outcome = %q saves = %d", replay.Outcome(), fixture.commissions.saves)
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（重放重发同一份）", len(fixture.handoff.intents))
	}

	t.Run("a different snapshot under the same identity is a conflict", func(t *testing.T) {
		flipped := submitCommissionCommand(t)
		flipped.Agreement = "agreement-snapshot-2"
		result, err := fixture.handler.SubmitCommission(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict submit: %v", err)
		}
		if result.Outcome() != application.CommissionConflict {
			t.Fatalf("outcome = %q, want COMMISSION_CONFLICT", result.Outcome())
		}
	})
}

// Covers: CONTEXT「承运接受……不等于容量已经预占、载运对象已经分配或实际承运商已经
// 收寄」与「只有接受成约；拒绝/失效/撤回分格带原因」的编排面——一次订舱一个应答：
// 已接受的订舱不能再被拒绝（同键异应答冲突不覆盖，原接受保留）；同应答重放返原；
// 应答四格完备性由 FormCarrierAcceptance 把门（超量接受、拒绝免因都未受理）。
func TestABookingAnswerCannotBeOverwritten(t *testing.T) {
	fixture := newCommissionFixture(t)
	if _, err := fixture.handler.SubmitBooking(context.Background(), submitBookingCommand(t)); err != nil {
		t.Fatalf("submit booking: %v", err)
	}

	accepted, err := fixture.handler.AnswerBooking(context.Background(), answerBookingCommand(t, domain.BookingAccepted))
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	if accepted.Outcome() != application.BookingAnswered {
		t.Fatalf("outcome = %q, want BOOKING_ANSWERED", accepted.Outcome())
	}
	answer, _ := accepted.Answer()
	if !answer.Acceptance.Binds() || answer.Acceptance.AcceptedQuantity() != 80 {
		t.Fatalf("acceptance = %+v（部分接受保留数量）", answer.Acceptance)
	}

	t.Run("a refusal after acceptance is a conflict, not an overwrite", func(t *testing.T) {
		refusal := answerBookingCommand(t, domain.BookingRefused)
		result, err := fixture.handler.AnswerBooking(context.Background(), refusal)
		if err != nil {
			t.Fatalf("refuse after accept: %v", err)
		}
		if result.Outcome() != application.BookingAnswerConflict {
			t.Fatalf("outcome = %q, want ANSWER_CONFLICT（已接受不能再拒绝）", result.Outcome())
		}
		kept, _ := fixture.handler.AnswerBooking(context.Background(), answerBookingCommand(t, domain.BookingAccepted))
		keptAnswer, _ := kept.Answer()
		if !keptAnswer.Acceptance.Binds() {
			t.Fatal("冲突覆盖了原接受")
		}
	})

	t.Run("the same answer replays to the original", func(t *testing.T) {
		replay, err := fixture.handler.AnswerBooking(context.Background(), answerBookingCommand(t, domain.BookingAccepted))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.BookingAnswerExists || fixture.answers.saves != 1 {
			t.Fatalf("outcome = %q saves = %d", replay.Outcome(), fixture.answers.saves)
		}
	})

	t.Run("an over-quantity acceptance is not accepted", func(t *testing.T) {
		over := answerBookingCommand(t, domain.BookingAccepted)
		over.Booking = "booking-2"
		overBooking := submitBookingCommand(t)
		overBooking.Booking = "booking-2"
		if _, err := fixture.handler.SubmitBooking(context.Background(), overBooking); err != nil {
			t.Fatalf("submit booking-2: %v", err)
		}
		over.Quantity = 101
		result, err := fixture.handler.AnswerBooking(context.Background(), over)
		if err != nil {
			t.Fatalf("answer: %v", err)
		}
		if result.Outcome() != application.CommissionNotAccepted {
			t.Fatalf("outcome = %q（接受量超过申请量，领域把门）", result.Outcome())
		}
	})

	t.Run("answering an absent booking is not accepted", func(t *testing.T) {
		missing := answerBookingCommand(t, domain.BookingAccepted)
		missing.Booking = "booking-9"
		result, err := fixture.handler.AnswerBooking(context.Background(), missing)
		if err != nil {
			t.Fatalf("answer: %v", err)
		}
		if result.Outcome() != application.CommissionNotAccepted {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})
}

// Covers: CONTEXT「首个有效出发或移动事实形成前可以取消……之后只能形成中断、改降、
// 折返或其他实际结果」与 195「尚未消耗的运输委托或订舱范围可以取消」的编排面——
// 未开始取消成功；已开始 → TRANSPORT_STARTED 业务负向（ErrTransportAlreadyStarted
// 哨兵分格，恢复动作是按事实形成实际结果而非改单）；已取消重放返原不二取。
func TestCancellationOnlyReachesUnstartedCommissions(t *testing.T) {
	fixture := newCommissionFixture(t)
	if _, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t)); err != nil {
		t.Fatalf("submit: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")

	cancelled, err := fixture.handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
		TenantID:    tenant,
		Commission:  "commission-1",
		CancelledAt: commissionSentAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Outcome() != application.CommissionCancelled {
		t.Fatalf("outcome = %q, want COMMISSION_CANCELLED", cancelled.Outcome())
	}

	t.Run("a cancelled commission replays to the original", func(t *testing.T) {
		replay, err := fixture.handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
			TenantID:    tenant,
			Commission:  "commission-1",
			CancelledAt: commissionSentAt.Add(2 * time.Hour),
		})
		if err != nil {
			t.Fatalf("replay cancel: %v", err)
		}
		if replay.Outcome() != application.CommissionExistingResult {
			t.Fatalf("outcome = %q, want EXISTING_COMMISSION（不二取）", replay.Outcome())
		}
	})

	t.Run("a started commission cannot be cancelled", func(t *testing.T) {
		startedFixture := newCommissionFixture(t)
		if _, err := startedFixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t)); err != nil {
			t.Fatalf("submit: %v", err)
		}
		key := ports.TransportCommissionKey{TenantID: tenant}
		key.Commission, _ = domain.NewTransportCommissionReference("commission-1")
		record := startedFixture.commissions.records[commissionStoreKey(key)]
		basis, _ := domain.NewParticipationBasisReference("OFFSITE-PICKUP/pickup-result/parcel-1/v1")
		started, err := record.Commission.MarkTransportStarted(basis, commissionSentAt.Add(30*time.Minute))
		if err != nil {
			t.Fatalf("mark started: %v", err)
		}
		record.Commission = started
		startedFixture.commissions.records[commissionStoreKey(key)] = record

		result, err := startedFixture.handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
			TenantID:    tenant,
			Commission:  "commission-1",
			CancelledAt: commissionSentAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("cancel started: %v", err)
		}
		if result.Outcome() != application.CommissionTransportStarted {
			t.Fatalf("outcome = %q, want TRANSPORT_STARTED（只能按事实形成中断等实际结果）", result.Outcome())
		}
	})

	t.Run("cancelling an absent commission is not accepted", func(t *testing.T) {
		result, err := fixture.handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
			TenantID:    tenant,
			Commission:  "commission-9",
			CancelledAt: commissionSentAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("cancel absent: %v", err)
		}
		if result.Outcome() != application.CommissionNotAccepted {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})
}

// Covers: ADR-0029（按恢复动作分格——依赖故障归未决且指名等谁）、ADR-0031（写入
// 代数封闭，代数外是编程错误）与 ADR-0043（投递失败不翻结果、重放重发同一份）在
// 本编排的恢复面；未决原因集封闭。
func TestCommissionRecoveryDiscipline(t *testing.T) {
	t.Run("store failures are undecided with their reasons", func(t *testing.T) {
		fixture := newCommissionFixture(t)
		fixture.commissions.findErr = errors.New("store down")
		result, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t))
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if result.UndecidedReason() != application.CommissionStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		bookingFixture := newCommissionFixture(t)
		bookingFixture.bookings.findErr = errors.New("store down")
		result, err = bookingFixture.handler.SubmitBooking(context.Background(), submitBookingCommand(t))
		if err != nil {
			t.Fatalf("submit booking: %v", err)
		}
		if result.UndecidedReason() != application.BookingStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newCommissionFixture(t)
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t))
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if first.Outcome() != application.CommissionSubmitted || first.CommissionHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.CommissionHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.CommissionHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q（重放重发同一份）", len(fixture.handoff.intents), replay.CommissionHandoffReference())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.CommissionUndecidedReason{
			application.CommissionStoreUnavailable, application.BookingStoreUnavailable, application.AnswerStoreUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.CommissionUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第四个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
