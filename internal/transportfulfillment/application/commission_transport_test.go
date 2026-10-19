package application_test

import (
	"context"
	"errors"
	"sort"
	"strings"
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

// answerStoreDouble 照迁移 0029 的三条唯一约束办：同一答复标识、同一订舱第二份候补、第二份定局都答已有应答。
type answerStoreDouble struct {
	records map[string]ports.BookingAnswerRecord
	findErr error
	saves   int
}

func newAnswerStore() *answerStoreDouble {
	return &answerStoreDouble{records: map[string]ports.BookingAnswerRecord{}}
}

func answerStoreKey(record ports.BookingAnswerRecord) string {
	return bookingStoreKey(record.Key) + "|" + record.Acceptance.Acceptance().String()
}

func (double *answerStoreDouble) FindAnswers(
	_ context.Context,
	key ports.BookingKey,
) ([]ports.BookingAnswerRecord, error) {
	if double.findErr != nil {
		return nil, double.findErr
	}
	var answers []ports.BookingAnswerRecord
	for _, record := range double.records {
		if bookingStoreKey(record.Key) == bookingStoreKey(key) {
			answers = append(answers, record)
		}
	}
	sort.Slice(answers, func(i, j int) bool {
		return answers[i].Acceptance.DecidedAt().Before(answers[j].Acceptance.DecidedAt())
	})
	return answers, nil
}

func (double *answerStoreDouble) Save(
	_ context.Context,
	record ports.BookingAnswerRecord,
) (ports.BookingAnswerSaveOutcome, error) {
	double.saves++
	if _, exists := double.records[answerStoreKey(record)]; exists {
		return ports.BookingAlreadyAnswered, nil
	}
	for _, stored := range double.records {
		if bookingStoreKey(stored.Key) == bookingStoreKey(record.Key) && stored.Acceptance.Settles() == record.Acceptance.Settles() {
			return ports.BookingAlreadyAnswered, nil
		}
	}
	double.records[answerStoreKey(record)] = record
	return ports.BookingAnswerSaved, nil
}

// answersOf 交 booking 名下在册的答复数。
func (double *answerStoreDouble) answersOf(booking string) int {
	count := 0
	for _, record := range double.records {
		if record.Key.Booking.String() == booking {
			count++
		}
	}
	return count
}

// scheduleReaderDouble 是班次登记的只读替身：只认登记过的（租户，班次）。
type scheduleReaderDouble struct {
	registered map[string]bool
	findErr    error
}

func (double *scheduleReaderDouble) FindByKey(
	_ context.Context,
	key ports.ScheduleKey,
) (ports.ScheduleRecord, bool, error) {
	if double.findErr != nil {
		return ports.ScheduleRecord{}, false, double.findErr
	}
	if !double.registered[key.TenantID.String()+"|"+key.Schedule.String()] {
		return ports.ScheduleRecord{}, false, nil
	}
	return ports.ScheduleRecord{Key: key}, true, nil
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

// agreementSourceDouble 是协议核对口的替身：默认在册，带回快照与法人；记下每次被问的参数。
type agreementSourceDouble struct {
	check ports.SupplierAgreementCheck
	err   error
	asked []agreementQuestion
}

type agreementQuestion struct {
	agreement domain.SupplierAgreementVersion
	provider  domain.ServiceProviderReference
	at        time.Time
}

func registeredAgreement(t *testing.T) ports.SupplierAgreementCheck {
	t.Helper()
	snapshot, _ := domain.NewAgreementSnapshotReference("agreement-1/v1")
	legalEntity, _ := domain.NewProcurementLegalEntityReference("legal-entity-1")
	supplier, _ := domain.NewServiceProviderReference("partner-1")
	return ports.SupplierAgreementCheck{
		Standing:    ports.SupplierAgreementRegistered,
		Snapshot:    snapshot,
		LegalEntity: legalEntity,
		Supplier:    supplier,
	}
}

func (double *agreementSourceDouble) CheckSupplierAgreement(
	_ context.Context,
	_ domain.TenantID,
	agreement domain.SupplierAgreementVersion,
	provider domain.ServiceProviderReference,
	at time.Time,
) (ports.SupplierAgreementCheck, error) {
	double.asked = append(double.asked, agreementQuestion{agreement: agreement, provider: provider, at: at})
	if double.err != nil {
		return ports.SupplierAgreementCheck{}, double.err
	}
	return double.check, nil
}

type commissionClock struct{ at time.Time }

func (clock commissionClock) Now() time.Time { return clock.at }

type commissionFixture struct {
	commissions *commissionStoreDouble
	bookings    *bookingStoreDouble
	answers     *answerStoreDouble
	schedules   *scheduleReaderDouble
	agreements  *agreementSourceDouble
	handoff     *commissionHandoffDouble
	handler     *application.CommissionTransportHandler
}

func newCommissionFixture(t *testing.T) *commissionFixture {
	t.Helper()
	fixture := &commissionFixture{
		commissions: newCommissionStore(),
		bookings:    newBookingStore(),
		answers:     newAnswerStore(),
		schedules:   &scheduleReaderDouble{registered: map[string]bool{"tenant-1|schedule-1": true}},
		agreements:  &agreementSourceDouble{check: registeredAgreement(t)},
		handoff:     &commissionHandoffDouble{},
	}
	fixture.handler = application.NewCommissionTransportHandler(application.CommissionTransportDeps{
		Commissions: fixture.commissions,
		Bookings:    fixture.bookings,
		Answers:     fixture.answers,
		Schedules:   fixture.schedules,
		Agreements:  fixture.agreements,
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
		TenantID:         tenant,
		Commission:       "commission-1",
		Provider:         "partner-1",
		AgreementObject:  "agreement-1",
		AgreementVersion: "v1",
		Conditions:       "conditions-snapshot-1",
		Role:             "role-snapshot-1",
		Responsibility:   "responsibility-snapshot-1",
		Members:          []string{"parcel-1"},
		SubmittedAt:      commissionSentAt,
	}
}

func submitBookingCommand(t *testing.T) application.SubmitBookingCommand {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	return application.SubmitBookingCommand{
		TenantID:    tenant,
		Booking:     "booking-1",
		Commission:  "commission-1",
		Schedule:    "schedule-1",
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
		Source:     "partner-1",
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

// Covers: `AT-TF-108`（编排面）、TF CONTEXT「Boundaries and ownership」「保存……协议、条件、角色和责任依据快照，不修改商业版本」的
// 编排面（UC-TF-004 步 2、3）——协议快照与采购责任法人取自核对结果、不由登记方给，随记录保全；
// 核协议用的是登记方指名的那一版、服务方与提交时刻；同一委托标识只提交一次，重放返原、
// 异内容同键冲突；「委托已提交」意图随提交交出、重放重发同一份。
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
	if record.Commission.Agreement().String() != "agreement-1/v1" || record.Commission.LegalEntity().String() != "legal-entity-1" {
		t.Fatalf("协议快照与法人没有从核对结果随委托保全：%q %q", record.Commission.Agreement(), record.Commission.LegalEntity())
	}
	if len(fixture.agreements.asked) != 1 {
		t.Fatalf("协议核对 %d 次，want 1", len(fixture.agreements.asked))
	}
	asked := fixture.agreements.asked[0]
	if asked.agreement.Object() != "agreement-1" || asked.agreement.Version() != "v1" ||
		asked.provider.String() != "partner-1" || !asked.at.Equal(commissionSentAt) {
		t.Fatalf("核协议问的不是登记方指名的那一版、服务方与提交时刻：%+v", asked)
	}
	if !strings.HasPrefix(record.ContentDigest, "TFC-1:") {
		t.Fatalf("委托摘要 = %q，want TFC-1 形", record.ContentDigest)
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
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
	if len(fixture.agreements.asked) != 1 {
		t.Fatalf("重放又问了一次协议：核对 %d 次", len(fixture.agreements.asked))
	}

	t.Run("a replay answers the original even after the agreement has terminated", func(t *testing.T) {
		fixture.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementTerminated}
		defer func() { fixture.agreements.check = registeredAgreement(t) }()
		result, err := fixture.handler.SubmitCommission(context.Background(), command)
		if err != nil {
			t.Fatalf("replay after termination: %v", err)
		}
		if result.Outcome() != application.CommissionExistingResult {
			t.Fatalf("outcome = %q, want EXISTING_COMMISSION（重放判在核协议之前）", result.Outcome())
		}
	})

	t.Run("a different agreement version under the same identity is a conflict", func(t *testing.T) {
		flipped := submitCommissionCommand(t)
		flipped.AgreementVersion = "v2"
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
	fixture := newBookedFixture(t)

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

	t.Run("answering an absent booking is its own cell", func(t *testing.T) {
		missing := answerBookingCommand(t, domain.BookingAccepted)
		missing.Booking = "booking-9"
		result, err := fixture.handler.AnswerBooking(context.Background(), missing)
		if err != nil {
			t.Fatalf("answer: %v", err)
		}
		if result.Outcome() != application.BookingNotFound {
			t.Fatalf("outcome = %q, want BOOKING_NOT_FOUND（续办是核订舱标识，不是改载荷）", result.Outcome())
		}
	})
}

// newBookedFixture 交回一份已提交委托（服务方 partner-1）并在登记过的班次 schedule-1 上订了舱（booking-1）的编排。
func newBookedFixture(t *testing.T) *commissionFixture {
	t.Helper()
	fixture := newCommissionFixture(t)
	if result, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t)); err != nil || result.Outcome() != application.CommissionSubmitted {
		t.Fatalf("submit commission: %q %v", result.Outcome(), err)
	}
	if result, err := fixture.handler.SubmitBooking(context.Background(), submitBookingCommand(t)); err != nil || result.Outcome() != application.BookingSubmitted {
		t.Fatalf("submit booking: %q %v", result.Outcome(), err)
	}
	return fixture
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
			application.SupplierAgreementUnavailable, application.ScheduleStoreUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 5 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.CommissionUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("名单之外的未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
